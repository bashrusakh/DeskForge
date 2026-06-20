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
	"time"

	"rustdesk-server/api/model"

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
//
// Шаги:
//   GET  /repos/{repo}/actions/secrets/public-key  → {key_id, key (base64 32B)}
//   PUT  /repos/{repo}/actions/secrets/WORKFLOW_PAYLOAD_KEY  body {encrypted_value, key_id}
func (s *GithubBuildConfigService) SetWorkflowSecret(c *model.GithubBuildConfig) error {
	if c.Token == "" || c.Repo == "" {
		return errors.New("token/repo required")
	}
	if c.PayloadKey == "" {
		return errors.New("payload_key is empty (Generate or paste one first)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1) забрать публичный ключ репо
	resp, err := s.ghReq(ctx, c, "GET", "/repos/"+c.Repo+"/actions/secrets/public-key", nil)
	if err != nil {
		return fmt.Errorf("get public-key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
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
	sealed, err := box.SealAnonymous(nil, []byte(c.PayloadKey), &peerPub, rand.Reader)
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
		"/repos/"+c.Repo+"/actions/secrets/WORKFLOW_PAYLOAD_KEY", body)
	if err != nil {
		return fmt.Errorf("put secret: %w", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != 201 && putResp.StatusCode != 204 {
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
		ref = "master"
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
	for _, a := range data.Artifacts {
		if a.Name == artifactName {
			aid = a.Id
			break
		}
	}
	if aid == 0 {
		return nil, fmt.Errorf("artifact %q not found in run %d", artifactName, runId)
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

