package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"rustdesk-server/api/model"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/pbkdf2"
	"gorm.io/gorm"
)

// GithubBuildConfigService — singleton-настройки + криптография + дёрганье GitHub API.
// См. PLAN.md §8.8.5.
type GithubBuildConfigService struct{}

// Get возвращает singleton-запись настроек. Если её нет — создаёт пустую с id=1.
func (s *GithubBuildConfigService) Get() (*model.GithubBuildConfig, error) {
	c := &model.GithubBuildConfig{}
	err := DB.First(c, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Id = 1
		if err := DB.Create(c).Error; err != nil {
			return nil, err
		}
		return c, nil
	}
	return c, err
}

// Save обновляет настройки.
//   - Repo / WorkflowFilename: всегда копируем (включая пустое значение — это
//     валидный способ очистить настройку).
//   - Branch / Token / PayloadKey: пустая строка означает «оставить как есть».
//     Это удобство UI: показывать секреты нельзя, а Branch имеет осмысленный
//     default (`master`), и обнулять его пустым полем формы — почти всегда
//     случайность (см. BUGS.md B-010).
func (s *GithubBuildConfigService) Save(in *model.GithubBuildConfig) error {
	cur, err := s.Get()
	if err != nil {
		return err
	}
	cur.Repo = in.Repo
	cur.WorkflowFilename = in.WorkflowFilename
	if in.Branch != "" {
		cur.Branch = in.Branch
	}
	if in.Token != "" {
		cur.Token = in.Token
	}
	if in.PayloadKey != "" {
		cur.PayloadKey = in.PayloadKey
	}
	return DB.Save(cur).Error
}

// GeneratePayloadKey — 32 случайных байта → base64-URL без padding (≈43 char).
// Совместимо с тем, как ключ был выпущен в PowerShell на этапе (5).
func (s *GithubBuildConfigService) GeneratePayloadKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// убираем +/= для удобства (как в PS-скрипте)
	out := base64.StdEncoding.EncodeToString(buf)
	clean := make([]byte, 0, len(out))
	for i := 0; i < len(out); i++ {
		c := out[i]
		if c == '+' || c == '/' || c == '=' {
			continue
		}
		clean = append(clean, c)
	}
	return string(clean), nil
}

// EncryptPayload шифрует JSON-карту в base64-блоб, совместимый с
// `openssl enc -aes-256-cbc -pbkdf2 -pass pass:<key>` (формат: "Salted__"+salt(8)+ct).
// Используется для workflow input enc_payload (шаг 5 PLAN §8.8.3b).
func (s *GithubBuildConfigService) EncryptPayload(passphrase string, data map[string]any) (string, error) {
	if passphrase == "" {
		return "", errors.New("encryption passphrase is empty")
	}
	plain, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	// salt 8 байт (как у openssl)
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// PBKDF2-HMAC-SHA256 iter=10000, length=48 → 32 ключ + 16 IV
	derived := pbkdf2.Key([]byte(passphrase), salt, 10000, 48, sha256.New)
	key := derived[:32]
	iv := derived[32:48]

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// PKCS#7 padding
	bs := aes.BlockSize
	padLen := bs - len(plain)%bs
	padded := append(plain, bytes.Repeat([]byte{byte(padLen)}, padLen)...)

	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)

	var out bytes.Buffer
	out.WriteString("Salted__")
	out.Write(salt)
	out.Write(ct)
	return base64.StdEncoding.EncodeToString(out.Bytes()), nil
}

// --- GitHub REST API helpers ---

const githubAPI = "https://api.github.com"

func (s *GithubBuildConfigService) ghReq(ctx context.Context, c *model.GithubBuildConfig, method, path string, body any) (*http.Response, error) {
	var br io.Reader
	if body != nil {
		j, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		br = bytes.NewReader(j)
	}
	req, err := http.NewRequestWithContext(ctx, method, githubAPI+path, br)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if br != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Без жёсткого Timeout — таймаут контролируется через ctx у каждого вызова
	// (иначе скачивание большого артефакта обрезалось бы на 30s).
	return ghClient.Do(req)
}

var ghClient = &http.Client{}

// --- Available versions cache (offline-assets releases) ---

type ghVersionsCache struct {
	versions []string
	cachedAt time.Time
	mu       sync.Mutex
}

var (
	releasesCache    ghVersionsCache
	releasesCacheTTL = 5 * time.Minute
	// fallbackCacheTTL — короткий TTL для fallback-кэша: при недоступности API
	// хотим быстро вернуться к реальным данным после восстановления.
	fallbackCacheTTL = 30 * time.Second
)

// fallbackVersions возвращается, если GitHub API недоступен.
func fallbackVersions() []string {
	return []string{"1.4.8", "1.4.7"}
}

// GetAvailableVersions возвращает список версий RustDesk, доступных для сборки
// custom-клиента. Версии извлекаются из GitHub-релизов форка bashrusakh/rustdesk
// с тегами "offline-assets-*". Результат кешируется на 5 минут.
//
// Если GitHub API недоступен, возвращается fallback-список.
func (s *GithubBuildConfigService) GetAvailableVersions(ctx context.Context) ([]string, error) {
	// 1) Проверить кеш
	releasesCache.mu.Lock()
	if releasesCache.versions != nil && time.Since(releasesCache.cachedAt) < releasesCacheTTL {
		versions := releasesCache.versions
		releasesCache.mu.Unlock()
		return versions, nil
	}
	releasesCache.mu.Unlock()

	// 2) Запросить GitHub API
	versions, err := s.fetchReleases(ctx)
	if err != nil {
		// fallback при недоступности API — кешируем с коротким TTL,
		// чтобы после восстановления API быстро получить реальные данные.
		fallback := fallbackVersions()
		releasesCache.mu.Lock()
		releasesCache.versions = fallback
		// Сдвигаем cachedAt назад на (releasesCacheTTL - fallbackCacheTTL),
		// чтобы time.Since(cachedAt) быстро превысил releasesCacheTTL.
		releasesCache.cachedAt = time.Now().Add(-(releasesCacheTTL - fallbackCacheTTL))
		releasesCache.mu.Unlock()
		log.Warnf("fetchReleases failed, using fallback for %s: %v", fallbackCacheTTL, err)
		return fallback, err
	}

	// 3) Закешировать и вернуть
	releasesCache.mu.Lock()
	releasesCache.versions = versions
	releasesCache.cachedAt = time.Now()
	releasesCache.mu.Unlock()
	return versions, nil
}

// fetchReleases делает HTTP-запрос к GitHub API и возвращает отсортированные
// по semver (по убыванию) версии из тегов "offline-assets-*".
func (s *GithubBuildConfigService) fetchReleases(ctx context.Context) ([]string, error) {
	gcfg, err := s.Get()
	if err != nil {
		// Только ErrRecordNotFound допустим (конфиг ещё не задан — используем public API).
		// Любая другая ошибка — реальная проблема (DB unavailable и т.п.),
		// её надо увидеть, а не тихо провалиться на public rate limit.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get config: %w", err)
		}
	}

	// TODO: handle pagination via Link header if releases exceed 100. Реальных релизов <10,
	// но при росте выше 100 будут silently omitted. Нужен fetcher с follow Link header.
	path := "/repos/bashrusakh/rustdesk/releases?per_page=100"
	var resp *http.Response

	if gcfg != nil && gcfg.Token != "" {
		resp, err = s.ghReq(ctx, gcfg, "GET", path, nil)
	} else {
		// Без PAT — публичный запрос (ниже rate limit, но работает)
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, "GET", githubAPI+path, nil)
		if err == nil {
			req.Header.Set("Accept", "application/vnd.github+json")
			req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
			resp, err = ghClient.Do(req)
		}
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	// Фильтр: только offline-assets-*
	prefix := "offline-assets-"
	var versions []string
	for _, r := range releases {
		if strings.HasPrefix(r.TagName, prefix) {
			v := strings.TrimPrefix(r.TagName, prefix)
			if v != "" {
				versions = append(versions, v)
			}
		}
	}

	// Сортировка semver по убыванию
	sort.Slice(versions, func(i, j int) bool {
		return compareSemver(versions[i], versions[j]) > 0
	})

	return versions, nil
}

// compareSemver сравнивает две semver-строки (например "1.4.8" и "1.4.7").
// Возвращает >0 если a > b, <0 если a < b, 0 если равны.
// Pre-release сегменты (например "1.4.8-beta") считаются МЕНЬШЕ release ("1.4.8").
func compareSemver(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) < n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		va, errA := strconv.Atoi(pa[i])
		vb, errB := strconv.Atoi(pb[i])
		if errA != nil || errB != nil {
			// non-numeric сегмент — pre-release. numeric > non-numeric.
			if errA == nil && errB != nil {
				return 1
			}
			if errA != nil && errB == nil {
				return -1
			}
			// оба non-numeric — лексикографически (стабильно, но редко встречается)
			if cmp := strings.Compare(pa[i], pb[i]); cmp != 0 {
				return cmp
			}
			continue
		}
		if va != vb {
			return va - vb
		}
	}
// Все совпавшие сегменты равны. Больше сегментов = выше версия ТОЛЬКО если
// дополнительные сегменты non-zero. По semver trailing zero сегменты эквивалентны
// ("1.4.8.0" == "1.4.8"). Если количество равно — финальное сравнение
// non-numeric хвостов: non-numeric < numeric ("1.4.8-beta" < "1.4.8").
	switch {
	case len(pa) > len(pb):
		// Extra segments у a — greater только если хоть один non-zero
		for i := n; i < len(pa); i++ {
			if v, err := strconv.Atoi(pa[i]); err == nil && v != 0 {
				return 1
			}
		}
		return 0
	case len(pa) < len(pb):
		// Extra segments у b — greater только если хоть один non-zero
		for i := n; i < len(pb); i++ {
			if v, err := strconv.Atoi(pb[i]); err == nil && v != 0 {
				return -1
			}
		}
		return 0
	default:
		// одинаковая длина — сравнить последние сегменты на non-numeric
		hasPreA := hasNonNumeric(pa[len(pa)-1])
		hasPreB := hasNonNumeric(pb[len(pb)-1])
		if hasPreA == hasPreB {
			return 0
		}
		if hasPreA {
			return -1
		}
		return 1
	}
}

// hasNonNumeric — true если сегмент содержит не только цифры
// (например "8-beta", "rc1").
func hasNonNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return true
		}
	}
	return false
}

// TestConnection — read-only вызов GET /repos/{repo}: проверяет PAT, scope, доступ к репо.
// Возвращает (ok, message).
func (s *GithubBuildConfigService) TestConnection(c *model.GithubBuildConfig) (bool, string) {
	if c.Token == "" {
		return false, "token not set"
	}
	if c.Repo == "" {
		return false, "repo not set (expected owner/name)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := s.ghReq(ctx, c, "GET", "/repos/"+c.Repo, nil)
	if err != nil {
		return false, "request failed: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return true, "ok"
	}
	b, _ := io.ReadAll(resp.Body)
	return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(b))
}

// SetWorkflowSecret — кладёт/обновляет PayloadKey в GitHub Secrets форка как
// WORKFLOW_PAYLOAD_KEY. Использует libsodium sealed box (NaCl crypto_box_seal):
// GitHub отдаёт публичный X25519 ключ репо, мы шифруем значение, оно никогда не
// уходит в открытом виде. Требует у PAT scope `Secrets: read & write` на репо.
func (s *GithubBuildConfigService) SetWorkflowSecret(c *model.GithubBuildConfig) error {
	if c.Token == "" || c.Repo == "" {
		return errors.New("token/repo required")
	}
	if c.PayloadKey == "" {
		return errors.New("payload_key is empty (Generate or paste one first)")
	}
	return s.putGithubSecret(c, "/repos/"+c.Repo, "WORKFLOW_PAYLOAD_KEY", c.PayloadKey)
}

// SetSyncPatSecret кладёт/обновляет PAT в GitHub Secrets текущего настроенного
// репозитория как GH_PAT. Используется sync-workflows.yml для доступа к форку
// из CI. Тот же sealed box механизм, что и SetWorkflowSecret.
func (s *GithubBuildConfigService) SetSyncPatSecret(c *model.GithubBuildConfig) error {
	if c.Token == "" {
		return errors.New("token is empty — save a PAT first")
	}
	if c.Repo == "" {
		return errors.New("repo is not set")
	}
	return s.putGithubSecret(c, "/repos/"+c.Repo, "GH_PAT", c.Token)
}

// putGithubSecret — общая логика encrypt-and-PUT секрета в GitHub Actions Secrets.
// Шаги:
//   GET  {repoPath}/actions/secrets/public-key  → {key_id, key (base64 32B)}
//   PUT  {repoPath}/actions/secrets/{secretName}  body {encrypted_value, key_id}
func (s *GithubBuildConfigService) putGithubSecret(c *model.GithubBuildConfig, repoPath, secretName, plaintext string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1) забрать публичный ключ репо
	resp, err := s.ghReq(ctx, c, "GET", repoPath+"/actions/secrets/public-key", nil)
	if err != nil {
		return fmt.Errorf("get public-key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("get public-key HTTP %d: %s", resp.StatusCode, string(b))
	}
	var pk struct {
		KeyId string `json:"key_id"`
		Key   string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pk); err != nil {
		return fmt.Errorf("decode public-key: %w", err)
	}
	keyBytes, err := base64.StdEncoding.DecodeString(pk.Key)
	if err != nil {
		return fmt.Errorf("decode public-key base64: %w", err)
	}
	if len(keyBytes) != 32 {
		return fmt.Errorf("public-key length unexpected: %d (want 32)", len(keyBytes))
	}
	var peerPub [32]byte
	copy(peerPub[:], keyBytes)

	// 2) sealed box: эфемерная пара + шифрование значения
	sealed, err := box.SealAnonymous(nil, []byte(plaintext), &peerPub, rand.Reader)
	if err != nil {
		return fmt.Errorf("sealed box: %w", err)
	}
	encValue := base64.StdEncoding.EncodeToString(sealed)

	// 3) PUT секрет
	body := map[string]string{
		"encrypted_value": encValue,
		"key_id":          pk.KeyId,
	}
	putResp, err := s.ghReq(ctx, c, "PUT",
		repoPath+"/actions/secrets/"+secretName, body)
	if err != nil {
		return fmt.Errorf("put secret: %w", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated && putResp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("put secret HTTP %d: %s", putResp.StatusCode, string(b))
	}
	return nil
}

// DispatchBuild — workflow_dispatch с зашифрованным payload.
// params — {server, key, app_name, custom_txt}. Возвращает (runId, error).
// runId получается отдельным запросом /actions/runs?per_page=1 после dispatch (GitHub
// не возвращает id напрямую — приходится поллить).
func (s *GithubBuildConfigService) DispatchBuild(ctx context.Context, c *model.GithubBuildConfig, params map[string]any) (int64, error) {
	if c.Token == "" || c.Repo == "" || c.WorkflowFilename == "" {
		return 0, errors.New("GithubBuildConfig: token/repo/workflow_filename required")
	}
	enc, err := s.EncryptPayload(c.PayloadKey, params)
	if err != nil {
		return 0, fmt.Errorf("encrypt: %w", err)
	}
	ref := c.Branch
	if ref == "" {
		ref = "rustqs/min-test"
	}
	body := map[string]any{
		"ref": ref,
		"inputs": map[string]string{
			"enc_payload": enc,
		},
	}
	path := fmt.Sprintf("/repos/%s/actions/workflows/%s/dispatches", c.Repo, c.WorkflowFilename)
	resp, err := s.ghReq(ctx, c, "POST", path, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("dispatch HTTP %d: %s", resp.StatusCode, string(b))
	}
	// найти id свежезапущенного рана (GitHub индексирует не моментально).
	// ВАЖНО: принимаем только ран, созданный не раньше момента dispatch (минус
	// небольшой допуск на рассинхрон часов сервера и GitHub). Иначе per_page=1
	// вернёт ПРЕДЫДУЩИЙ ран этого воркфлоу, пока новый не проиндексирован, и UI
	// покажет ссылку на чужую сборку.
	dispatchedAt := time.Now().UTC().Add(-10 * time.Second)
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		listPath := fmt.Sprintf("/repos/%s/actions/workflows/%s/runs?per_page=1&branch=%s",
			c.Repo, c.WorkflowFilename, ref)
		rr, err := s.ghReq(ctx, c, "GET", listPath, nil)
		if err != nil {
			continue
		}
		var data struct {
			WorkflowRuns []struct {
				Id        int64  `json:"id"`
				Status    string `json:"status"`
				CreatedAt string `json:"created_at"`
			} `json:"workflow_runs"`
		}
		_ = json.NewDecoder(rr.Body).Decode(&data)
		rr.Body.Close()
		if len(data.WorkflowRuns) > 0 {
			run := data.WorkflowRuns[0]
			created, perr := time.Parse(time.RFC3339, run.CreatedAt)
			// если created_at непарсится — не блокируемся, берём как есть;
			// иначе ждём появления именно нового рана.
			if perr != nil || !created.Before(dispatchedAt) {
				return run.Id, nil
			}
		}
	}
	return 0, errors.New("dispatch ok but run id not found after polling")
}

// RunStatus — GET /actions/runs/{id}: возвращает (status, conclusion).
func (s *GithubBuildConfigService) RunStatus(ctx context.Context, c *model.GithubBuildConfig, runId int64) (status, conclusion string, err error) {
	resp, err := s.ghReq(ctx, c, "GET", fmt.Sprintf("/repos/%s/actions/runs/%d", c.Repo, runId), nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var data struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}
	return data.Status, data.Conclusion, nil
}

// DownloadArtifact — скачивает zip артефакта по имени, возвращает []byte (zip-байты).
// Используется после успешного завершения сборки.
func (s *GithubBuildConfigService) DownloadArtifact(ctx context.Context, c *model.GithubBuildConfig, runId int64, artifactName string) ([]byte, error) {
	resp, err := s.ghReq(ctx, c, "GET", fmt.Sprintf("/repos/%s/actions/runs/%d/artifacts", c.Repo, runId), nil)
	if err != nil {
		return nil, err
	}
	var data struct {
		Artifacts []struct {
			Id   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"artifacts"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&data)
	resp.Body.Close()
	var aid int64
	if artifactName != "" {
		for _, a := range data.Artifacts {
			if a.Name == artifactName {
				aid = a.Id
				break
			}
		}
	}
	// AU-L-011: не завязываемся жёстко на имя артефакта. Если имя не задано или
	// не найдено, но ран произвёл ровно один артефакт — берём его.
	if aid == 0 && len(data.Artifacts) == 1 {
		aid = data.Artifacts[0].Id
	}
	if aid == 0 {
		names := make([]string, 0, len(data.Artifacts))
		for _, a := range data.Artifacts {
			names = append(names, a.Name)
		}
		return nil, fmt.Errorf("artifact %q not found in run %d (available: %v)", artifactName, runId, names)
	}
	// /artifacts/{id}/zip → 302 redirect → CDN. http.Client сам не следует на скачивание
	// большого файла — но в нашем случае GitHub отдаёт 302 на signed URL.
	zipPath := fmt.Sprintf("/repos/%s/actions/artifacts/%d/zip", c.Repo, aid)
	rr, err := s.ghReq(ctx, c, "GET", zipPath, nil)
	if err != nil {
		return nil, err
	}
	defer rr.Body.Close()
	if rr.StatusCode != 200 {
		b, _ := io.ReadAll(rr.Body)
		return nil, fmt.Errorf("artifact zip HTTP %d: %s", rr.StatusCode, string(b))
	}
	return io.ReadAll(rr.Body)
}

