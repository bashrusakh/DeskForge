package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"

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
		normalizePersistedWorkflowExecutionRef(c)
		return c, nil
	}
	if err == nil {
		// Existing installations may still contain the old default "master".
		// Normalize only known owned representations in memory; unknown/mutable
		// values remain intact and are rejected by workflowExecutionRef.
		normalizePersistedWorkflowExecutionRef(c)
	}
	return c, err
}

// Save обновляет только глобальную конфигурацию, разрешённую оператору:
// repository, PAT и payload key. Legacy workflow/ref columns remain untouched;
// Branch is read only as the pre-existing execution-ref configuration.
func (s *GithubBuildConfigService) Save(in *model.GithubBuildConfig) error {
	if in == nil {
		return errors.New("GitHub build config is missing")
	}
	// Check before Get(): Get creates the singleton row when absent, but a
	// secret-bearing save without the at-rest key must not leave even an
	// incidental row behind.
	if (in.Token != "" && !utils.IsEncryptedSecret(in.Token)) ||
		(in.PayloadKey != "" && !utils.IsEncryptedSecret(in.PayloadKey)) {
		if err := utils.RequireSecretEncryptionKey(); err != nil {
			return err
		}
	}
	if in.Repo != "" {
		if err := validateGithubRepo(in.Repo); err != nil {
			return err
		}
	}
	cur, err := s.Get()
	if err != nil {
		return err
	}
	repoChanged := cur.Repo != in.Repo
	cur.Repo = in.Repo
	if repoChanged {
		cur.WorkflowRefApproved = false
		cur.WorkflowRefProviderVerified = false
		cur.WorkflowRefApprovalSHA = ""
	}
	if in.Token != "" {
		cur.Token = in.Token
	}
	if in.PayloadKey != "" {
		cur.PayloadKey = in.PayloadKey
	}
	if in.Token == "" && in.PayloadKey == "" {
		if utils.HasSecretEncryptionKey() {
			// Preserve the existing resave behavior when the key is available:
			// legacy plaintext secrets are migrated to encrypted storage.
			return DB.Save(cur).Error
		}
		updates := map[string]any{"repo": in.Repo, "branch": cur.Branch}
		if repoChanged {
			updates["workflow_ref_approved"] = false
			updates["workflow_ref_provider_verified"] = false
			updates["workflow_ref_approval_sha"] = ""
		}
		if err := updateGithubBuildConfigMetadata(cur.Id, "", updates); err != nil {
			return err
		}
		return nil
	}
	return DB.Save(cur).Error
}

// updateGithubBuildConfigMetadata changes only non-secret singleton metadata.
// UpdateColumns deliberately skips GithubBuildConfig's secret hooks so legacy
// plaintext PATs and payload keys are not reserialized during metadata changes.
// An expected repository, when supplied, prevents an approval read from being
// applied after a concurrent repository change.
func updateGithubBuildConfigMetadata(id uint, expectedRepo string, updates map[string]any) error {
	query := DB.Model(&model.GithubBuildConfig{}).Where("id = ?", id)
	if expectedRepo != "" {
		query = query.Where("repo = ?", expectedRepo)
	}
	result := query.UpdateColumns(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("GitHub build config metadata update affected %d rows: %w", result.RowsAffected, gorm.ErrRecordNotFound)
	}
	return nil
}

// GeneratePayloadKey — 32 случайных байта → base64-URL без padding (≈43 char).
// Совместимо с тем, как ключ был выпущен в PowerShell на этапе (5).
func (s *GithubBuildConfigService) GeneratePayloadKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

const (
	payloadEnvelopeMagic     = "DFP1"
	payloadSaltSize          = 16
	payloadMACSize           = sha256.Size
	payloadPBKDF2Iterations  = 100000
	payloadDerivedKeySize    = 32 + aes.BlockSize + sha256.Size
	legacyPayloadSaltSize    = 8
	legacyPayloadPBKDF2Iters = 10000
)

// EncryptPayload emits a versioned encrypt-then-MAC envelope. The AES-CBC
// ciphertext remains easy for the active shell workflows to decrypt, while a
// separate HMAC is verified before any new ciphertext is decrypted.
func (s *GithubBuildConfigService) EncryptPayload(passphrase string, data map[string]any) (string, error) {
	if passphrase == "" {
		return "", errors.New("encryption passphrase is empty")
	}
	plain, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	salt := make([]byte, payloadSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// PBKDF2-HMAC-SHA256 derives AES key, IV, and an independent MAC key.
	derived := pbkdf2.Key([]byte(passphrase), salt, payloadPBKDF2Iterations, payloadDerivedKeySize, sha256.New)
	key := derived[:32]
	iv := derived[32:48]
	macKey := derived[48:]

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

	var signed bytes.Buffer
	signed.WriteString(payloadEnvelopeMagic)
	signed.Write(salt)
	signed.Write(ct)
	tag := hmac.New(sha256.New, macKey)
	_, _ = tag.Write(signed.Bytes())

	var out bytes.Buffer
	out.Write(signed.Bytes())
	out.Write(tag.Sum(nil))
	return base64.StdEncoding.EncodeToString(out.Bytes()), nil
}

// DecryptPayload verifies and decrypts a new DFP1 envelope. Salted__ payloads
// are retained only as a clearly distinguishable legacy compatibility path;
// new payloads never fall back to unauthenticated CBC after a MAC failure.
func (s *GithubBuildConfigService) DecryptPayload(passphrase, encoded string) (map[string]any, error) {
	if passphrase == "" {
		return nil, errors.New("encryption passphrase is empty")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("encrypted payload has invalid base64")
	}
	var plain []byte
	switch {
	case len(raw) >= len(payloadEnvelopeMagic) && string(raw[:len(payloadEnvelopeMagic)]) == payloadEnvelopeMagic:
		if len(raw) <= len(payloadEnvelopeMagic)+payloadSaltSize+payloadMACSize {
			return nil, errors.New("encrypted payload envelope is truncated")
		}
		ciphertextEnd := len(raw) - payloadMACSize
		ciphertext := raw[len(payloadEnvelopeMagic)+payloadSaltSize : ciphertextEnd]
		if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
			return nil, errors.New("encrypted payload ciphertext is invalid")
		}
		derived := pbkdf2.Key([]byte(passphrase), raw[len(payloadEnvelopeMagic):len(payloadEnvelopeMagic)+payloadSaltSize], payloadPBKDF2Iterations, payloadDerivedKeySize, sha256.New)
		mac := hmac.New(sha256.New, derived[48:])
		_, _ = mac.Write(raw[:ciphertextEnd])
		if !hmac.Equal(mac.Sum(nil), raw[ciphertextEnd:]) {
			return nil, errors.New("encrypted payload authentication failed")
		}
		plain, err = decryptPayloadCBC(derived[:32], derived[32:48], ciphertext)
	case len(raw) >= len("Salted__")+legacyPayloadSaltSize && string(raw[:len("Salted__")]) == "Salted__":
		derived := pbkdf2.Key([]byte(passphrase), raw[len("Salted__"):len("Salted__")+legacyPayloadSaltSize], legacyPayloadPBKDF2Iters, 48, sha256.New)
		plain, err = decryptPayloadCBC(derived[:32], derived[32:48], raw[len("Salted__")+legacyPayloadSaltSize:])
	default:
		return nil, errors.New("encrypted payload envelope version is unsupported")
	}
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(plain, &data); err != nil {
		return nil, fmt.Errorf("decode encrypted payload JSON: %w", err)
	}
	return data, nil
}

func decryptPayloadCBC(key, iv, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("encrypted payload ciphertext is invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
	padding := int(plain[len(plain)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(plain) {
		return nil, errors.New("encrypted payload padding is invalid")
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return nil, errors.New("encrypted payload padding is invalid")
		}
	}
	return plain[:len(plain)-padding], nil
}

// --- GitHub REST API helpers ---

const githubAPI = "https://api.github.com"

const (
	githubErrorBodyLimit = 4 << 10
	githubJSONBodyLimit  = 4 << 20
)

// githubArtifactBodyLimit bounds the compressed artifact written to the
// temporary archive. It is a variable rather than a second caller-provided
// limit so every artifact download shares the same fail-closed boundary and
// hermetic tests can exercise the oversized path without allocating 256 MiB.
var githubArtifactBodyLimit int64 = 256 << 20

// Empty uses the process temp directory in production. Tests may inject a
// hermetic directory without changing the public service contract.
var githubArtifactTempDir string

// GithubAPIError describes a non-successful GitHub REST response without
// retaining an unbounded or sensitive response body.
type GithubAPIError struct {
	StatusCode         int
	Retryable          bool
	Terminal           bool
	Body               string
	RetryAfter         string
	RateLimitRemaining string
	RateLimitReset     string
}

func (e *GithubAPIError) Error() string {
	message := fmt.Sprintf("GitHub API HTTP %d", e.StatusCode)
	if e.Retryable {
		message += " (retryable)"
	}
	if e.Body != "" {
		message += ": " + e.Body
	}
	return message
}

// GithubTransportError wraps a request/response-body transport failure.
// Transport failures and context timeouts are retryable by default.
type GithubTransportError struct {
	Operation string
	Cause     error
}

func (e *GithubTransportError) Error() string {
	return fmt.Sprintf("GitHub %s failed: %v", e.Operation, e.Cause)
}

func (e *GithubTransportError) Unwrap() error { return e.Cause }

// GithubContractError means GitHub returned an otherwise successful response
// that does not satisfy the endpoint contract. It must never be guessed from.
type GithubContractError struct {
	Operation string
	Cause     error
}

func (e *GithubContractError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("GitHub %s response contract invalid", e.Operation)
	}
	return fmt.Sprintf("GitHub %s response contract invalid: %v", e.Operation, e.Cause)
}

func (e *GithubContractError) Unwrap() error { return e.Cause }

// GithubArtifactUnavailableError means the requested artifact name was not
// present in the completed run. It is terminal: selecting another artifact
// would violate the workflow's exact artifact contract.
type GithubArtifactUnavailableError struct {
	RunID        int64
	ArtifactName string
	Available    []string
}

type githubArtifactMetadata struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Expired bool   `json:"expired"`
}

func (e *GithubArtifactUnavailableError) Error() string {
	return fmt.Sprintf("artifact %q not found in run %d (available: %v)", e.ArtifactName, e.RunID, e.Available)
}

// IsGithubRetryable reports whether a GitHub failure may be retried.
func IsGithubRetryable(err error) bool {
	var apiErr *GithubAPIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable
	}
	var transportErr *GithubTransportError
	return errors.As(err, &transportErr)
}

// IsGithubTerminal reports whether a GitHub failure must fail the operation
// instead of being silently retried.
func IsGithubTerminal(err error) bool {
	var apiErr *GithubAPIError
	if errors.As(err, &apiErr) {
		return apiErr.Terminal
	}
	var contractErr *GithubContractError
	if errors.As(err, &contractErr) {
		return true
	}
	var artifactErr *GithubArtifactUnavailableError
	return errors.As(err, &artifactErr)
}

func classifyGithubStatus(status int, headers http.Header) (retryable, terminal bool) {
	switch {
	case status == http.StatusRequestTimeout,
		status == http.StatusTooEarly,
		status == http.StatusTooManyRequests,
		status >= http.StatusInternalServerError:
		return true, false
	case status == http.StatusForbidden:
		if headers.Get("X-RateLimit-Remaining") == "0" || headers.Get("Retry-After") != "" {
			return true, false
		}
		return false, true
	default:
		return false, true
	}
}

var (
	githubBearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
	githubTokenPattern  = regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9_]*|github_pat_[A-Za-z0-9_]*)`)
)

func redactGithubBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) > githubErrorBodyLimit+1 {
		body = body[:githubErrorBodyLimit+1]
	}
	// The input has already been bounded to limit+1 by readGithubBody. Scan the
	// extra byte before truncating so a sensitive value cut at the limit cannot
	// leak its prefix.
	sanitized := redactGithubSensitiveFields(body)
	message := githubBearerPattern.ReplaceAllString(string(sanitized), "Bearer [REDACTED]")
	message = githubTokenPattern.ReplaceAllString(message, "[REDACTED]")
	if len(message) > githubErrorBodyLimit {
		message = message[:githubErrorBodyLimit] + "…"
	}
	return strings.TrimSpace(message)
}

func isGithubSensitiveKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "token", "access_token", "authorization", "payload", "payload_key", "payload-key", "payloadkey", "pat", "github_pat", "secret", "password", "encrypted_value", "enc_payload":
		return true
	default:
		return false
	}
}

func githubFieldKey(body []byte, start int) (end int, key string, ok bool) {
	if start >= len(body) {
		return 0, "", false
	}
	if body[start] == '"' || body[start] == '\'' {
		quote := body[start]
		for i := start + 1; i < len(body); i++ {
			if body[i] == '\\' {
				i++
				continue
			}
			if body[i] == quote {
				return i + 1, string(body[start+1 : i]), true
			}
		}
		return 0, "", false
	}
	if !((body[start] >= 'a' && body[start] <= 'z') || (body[start] >= 'A' && body[start] <= 'Z') || body[start] == '_') {
		return 0, "", false
	}
	end = start + 1
	for end < len(body) {
		c := body[end]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			break
		}
		end++
	}
	return end, string(body[start:end]), true
}

func redactGithubSensitiveFields(body []byte) []byte {
	var sanitized bytes.Buffer
	sanitized.Grow(len(body))
	for i := 0; i < len(body); {
		keyEnd, key, ok := githubFieldKey(body, i)
		if !ok || !isGithubSensitiveKey(key) {
			sanitized.WriteByte(body[i])
			i++
			continue
		}
		valueStart := keyEnd
		for valueStart < len(body) && (body[valueStart] == ' ' || body[valueStart] == '\t' || body[valueStart] == '\r' || body[valueStart] == '\n') {
			valueStart++
		}
		if valueStart >= len(body) || (body[valueStart] != ':' && body[valueStart] != '=') {
			sanitized.WriteByte(body[i])
			i++
			continue
		}
		valueStart++
		for valueStart < len(body) && (body[valueStart] == ' ' || body[valueStart] == '\t' || body[valueStart] == '\r' || body[valueStart] == '\n') {
			valueStart++
		}
		sanitized.Write(body[i:valueStart])
		if valueStart == len(body) {
			sanitized.WriteString("[REDACTED]")
			i = valueStart
			continue
		}
		if body[valueStart] == '"' || body[valueStart] == '\'' {
			quote := body[valueStart]
			sanitized.WriteByte(quote)
			sanitized.WriteString("[REDACTED]")
			i = valueStart + 1
			for i < len(body) {
				if body[i] == '\\' {
					i += 2
					continue
				}
				if body[i] == quote {
					sanitized.WriteByte(quote)
					i++
					break
				}
				i++
			}
			// An unterminated value is intentionally consumed through the bounded
			// input, including the limit+1 byte.
			continue
		}
		sanitized.WriteString("[REDACTED]")
		i = valueStart
		for i < len(body) && body[i] != ',' && body[i] != '}' && body[i] != ']' && body[i] != '\r' && body[i] != '\n' {
			i++
		}
	}
	return sanitized.Bytes()
}

func readGithubBody(body io.Reader, limit int) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > limit {
		return data, true, nil
	}
	return data, false, nil
}

func decodeGithubJSON(resp *http.Response, operation string, dst any) error {
	body, truncated, err := readGithubBody(resp.Body, githubJSONBodyLimit)
	if err != nil {
		return &GithubTransportError{Operation: operation + " response body", Cause: err}
	}
	if truncated {
		return &GithubContractError{Operation: operation, Cause: errors.New("response body exceeds limit")}
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return &GithubContractError{Operation: operation, Cause: err}
	}
	return nil
}

func safeGithubURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "github.com" || host == "api.github.com"
}

// ghReq — общий helper для запросов к GitHub REST API с авторизацией из
// конфига. Если body == nil — GET; иначе — JSON-кодируется и Content-Type
// выставляется автоматически. Таймаут контролируется через ctx вызывающего.
func (s *GithubBuildConfigService) ghReq(ctx context.Context, c *model.GithubBuildConfig, method, path string, body any, expected ...int) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("GitHub build config is missing")
	}
	if err := validateGithubRepo(c.Repo); err != nil {
		return nil, err
	}
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
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if br != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Без жёсткого Timeout — таймаут контролируется через ctx у каждого вызова
	// (иначе скачивание большого артефакта обрезалось бы на 30s).
	resp, err := ghClient.Do(req)
	if err != nil {
		return nil, &GithubTransportError{Operation: method + " " + path, Cause: err}
	}
	for _, status := range expected {
		if resp.StatusCode == status {
			return resp, nil
		}
	}
	responseBody, _, readErr := readGithubBody(resp.Body, githubErrorBodyLimit)
	resp.Body.Close()
	if readErr != nil {
		return nil, &GithubTransportError{Operation: method + " " + path + " error response", Cause: readErr}
	}
	retryable, terminal := classifyGithubStatus(resp.StatusCode, resp.Header)
	return nil, &GithubAPIError{
		StatusCode:         resp.StatusCode,
		Retryable:          retryable,
		Terminal:           terminal,
		Body:               redactGithubBody(responseBody),
		RetryAfter:         resp.Header.Get("Retry-After"),
		RateLimitRemaining: resp.Header.Get("X-RateLimit-Remaining"),
		RateLimitReset:     resp.Header.Get("X-RateLimit-Reset"),
	}
}

var ghClient = &http.Client{}

// compareSemver сравнивает две semver-подобные строки: сначала numeric core,
// затем prerelease-сегменты. Release всегда старше prerelease, а
// prerelease вроде "1.4.10-beta" корректно сравнивается с более низкой
// numeric-версией "1.4.9".
func compareSemver(a, b string) int {
	coreA, preA := splitVersion(a)
	coreB, preB := splitVersion(b)

	pa := strings.Split(coreA, ".")
	pb := strings.Split(coreB, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		if i >= len(pa) {
			if v, err := strconv.Atoi(pb[i]); err == nil && v != 0 {
				return -1
			}
			continue
		}
		if i >= len(pb) {
			if v, err := strconv.Atoi(pa[i]); err == nil && v != 0 {
				return 1
			}
			continue
		}
		va, errA := strconv.Atoi(pa[i])
		vb, errB := strconv.Atoi(pb[i])
		if errA != nil || errB != nil {
			if cmp := strings.Compare(pa[i], pb[i]); cmp != 0 {
				return cmp
			}
			continue
		}
		if va != vb {
			return va - vb
		}
	}

	switch {
	case preA == preB:
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	default:
		return comparePrerelease(preA, preB)
	}
}

func splitVersion(v string) (core, pre string) {
	core, pre, ok := strings.Cut(v, "-")
	if !ok {
		return v, ""
	}
	return core, pre
}

func comparePrerelease(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) < n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		ai, aNum := prereleaseIdentifier(pa[i])
		bi, bNum := prereleaseIdentifier(pb[i])
		switch {
		case aNum && bNum:
			if ai != bi {
				return ai - bi
			}
		case aNum:
			return -1
		case bNum:
			return 1
		default:
			if cmp := strings.Compare(pa[i], pb[i]); cmp != 0 {
				return cmp
			}
		}
	}
	if len(pa) != len(pb) {
		return len(pa) - len(pb)
	}
	return 0
}

func prereleaseIdentifier(v string) (int, bool) {
	if n, err := strconv.Atoi(v); err == nil {
		return n, true
	}
	return 0, false
}

// TestConnection — read-only вызов GET /repos/{repo}: проверяет PAT, scope, доступ к репо.
// Возвращает (ok, message).
func (s *GithubBuildConfigService) TestConnection(c *model.GithubBuildConfig) (bool, string) {
	if err := s.TestConnectionError(c); err != nil {
		return false, githubErrorMessage(err)
	}
	return true, "ok"
}

// TestConnectionError is the typed-error variant used by callers that need to
// distinguish a retryable GitHub outage from a terminal configuration error.
func (s *GithubBuildConfigService) TestConnectionError(c *model.GithubBuildConfig) error {
	if c.Token == "" {
		return errors.New("token not set")
	}
	if c.Repo == "" {
		return errors.New("repo not set (expected owner/name)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	path, err := githubRepoPath(c.Repo, "")
	if err != nil {
		return err
	}
	resp, err := s.ghReq(ctx, c, "GET", path, nil, http.StatusOK)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func githubErrorMessage(err error) string {
	return GithubSafeErrorMessage(err)
}

// GithubErrorHTTPStatus returns the safe HTTP classification for a known
// provider/workflow error. Nested errors are checked before the provider
// configuration wrapper so a policy, capability, validation, transport, or
// provider-response failure cannot be flattened into "not configured".
func GithubErrorHTTPStatus(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	if IsSecretEncryptionConfigurationError(err) {
		return http.StatusServiceUnavailable, true
	}
	var validationErr *ClientValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, true
	}
	var approvalErr *WorkflowRefApprovalError
	if errors.As(err, &approvalErr) {
		return http.StatusPreconditionFailed, true
	}
	var capabilityErr *ProductionCapabilityUnavailableError
	if errors.As(err, &capabilityErr) {
		return http.StatusServiceUnavailable, true
	}
	var apiErr *GithubAPIError
	if errors.As(err, &apiErr) {
		return http.StatusBadGateway, true
	}
	var artifactErr *GithubArtifactUnavailableError
	if errors.As(err, &artifactErr) {
		return http.StatusBadGateway, true
	}
	var transportErr *GithubTransportError
	if errors.As(err, &transportErr) {
		return http.StatusServiceUnavailable, true
	}
	var contractErr *GithubContractError
	if errors.As(err, &contractErr) {
		return http.StatusBadGateway, true
	}
	var providerErr *GithubProviderConfigurationError
	if errors.As(err, &providerErr) {
		return http.StatusServiceUnavailable, true
	}
	return 0, false
}

const githubDiagnosticLimit = 512

const githubRulesetBypassMetadataUnavailableCause = "ruleset bypass metadata is missing or not visible"

func isGithubRulesetBypassMetadataUnavailable(err *GithubContractError) bool {
	return err != nil &&
		err.Operation == "verify repository ruleset detail" &&
		err.Cause != nil &&
		strings.Contains(err.Cause.Error(), githubRulesetBypassMetadataUnavailableCause)
}

// GithubErrorDetail returns a bounded, log-oriented diagnostic. It is
// deliberately reconstructed from typed classifications instead of formatting
// err.Error(): provider URLs, refs, response bodies, credentials, payloads,
// and nested implementation details therefore cannot enter logs or build logs.
func GithubErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	detail := GithubSafeErrorMessage(err)
	var apiErr *GithubAPIError
	if errors.As(err, &apiErr) {
		detail = fmt.Sprintf("%s (status=%d, retryable=%t, terminal=%t)", detail, apiErr.StatusCode, apiErr.Retryable, apiErr.Terminal)
	}
	if len(detail) > githubDiagnosticLimit {
		return detail[:githubDiagnosticLimit] + "…"
	}
	return detail
}

// GithubSafeErrorMessage maps GitHub workflow/provider failures to a bounded
// user-facing message. Provider URLs, selectors, response bodies, credentials,
// encrypted payloads, and internal causes remain outside the API response.
func GithubSafeErrorMessage(err error) string {
	if err == nil {
		return "GitHub build operation failed"
	}
	if IsSecretEncryptionConfigurationError(err) {
		return "secret encryption is not configured"
	}
	var validationErr *ClientValidationError
	if errors.As(err, &validationErr) {
		return "custom build request is invalid"
	}
	var approvalErr *WorkflowRefApprovalError
	if errors.As(err, &approvalErr) {
		return "workflow reference approval is required"
	}
	var capabilityErr *ProductionCapabilityUnavailableError
	if errors.As(err, &capabilityErr) {
		return "production build capability is unavailable"
	}
	var artifactErr *GithubArtifactUnavailableError
	if errors.As(err, &artifactErr) {
		return "requested GitHub artifact is unavailable"
	}
	var apiErr *GithubAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized:
			return "GitHub authentication failed; verify the configured PAT"
		case http.StatusForbidden:
			if apiErr.Retryable {
				return "GitHub rate limit reached; retry later"
			}
			return "GitHub access was denied; verify the PAT permissions and repository access"
		case http.StatusNotFound:
			return "GitHub repository or workflow resource was not found; verify the repository name and PAT access"
		default:
			if apiErr.Retryable {
				return "GitHub provider is temporarily unavailable or rate-limited; retry shortly"
			}
			return "GitHub provider rejected the request; verify the repository and PAT permissions"
		}
	}
	var transportErr *GithubTransportError
	if errors.As(err, &transportErr) {
		return "GitHub provider is unavailable"
	}
	var contractErr *GithubContractError
	if errors.As(err, &contractErr) {
		if isGithubRulesetBypassMetadataUnavailable(contractErr) {
			return "GitHub workflow tag verification requires a PAT with Administration: write and repository access"
		}
		return "GitHub provider response was invalid"
	}
	var providerErr *GithubProviderConfigurationError
	if errors.As(err, &providerErr) {
		return "GitHub build provider is not configured"
	}
	return "GitHub build operation failed"
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
	return s.putGithubSecret(c, "WORKFLOW_PAYLOAD_KEY", c.PayloadKey)
}

// putGithubSecret — общая логика encrypt-and-PUT секрета в GitHub Actions Secrets.
// Шаги:
//
//	GET  {repoPath}/actions/secrets/public-key  → {key_id, key (base64 32B)}
//	PUT  {repoPath}/actions/secrets/{secretName}  body {encrypted_value, key_id}
func (s *GithubBuildConfigService) putGithubSecret(c *model.GithubBuildConfig, secretName, plaintext string) error {
	repoPath, err := githubRepoPath(c.Repo, "")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1) забрать публичный ключ репо
	resp, err := s.ghReq(ctx, c, "GET", repoPath+"/actions/secrets/public-key", nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("get public-key: %w", err)
	}
	defer resp.Body.Close()
	var pk struct {
		KeyId string `json:"key_id"`
		Key   string `json:"key"`
	}
	if err := decodeGithubJSON(resp, "get public-key", &pk); err != nil {
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
		repoPath+"/actions/secrets/"+secretName, body, http.StatusCreated, http.StatusNoContent)
	if err != nil {
		return fmt.Errorf("put secret: %w", err)
	}
	defer putResp.Body.Close()
	// Both accepted statuses have no response contract to decode.
	return nil
}

// GithubDispatchResult is the exact identity returned by GitHub for a
// workflow_dispatch request. The run ID is provider-derived; callers must not
// accept or manufacture it from user input.
type GithubDispatchResult struct {
	WorkflowRunID int64  `json:"workflow_run_id"`
	RunURL        string `json:"run_url"`
	HTMLURL       string `json:"html_url"`
}

// releaseAssetDispatchPayload is the typed, provider-derived portion of the
// encrypted workflow payload. ReleaseRepo is projected from the resolved
// catalog identity rather than accepted from normal build parameters.
type releaseAssetDispatchPayload struct {
	ReleaseRepo      string         `json:"release_repo"`
	AssetsReleaseID  int64          `json:"assets_release_id"`
	AssetsReleaseTag string         `json:"assets_release_tag"`
	ReleaseAssets    []ReleaseAsset `json:"release_assets"`
}

func releaseAssetPayloadForIdentity(identity VersionIdentity) (releaseAssetDispatchPayload, error) {
	if err := validateGithubRepo(identity.Repo); err != nil {
		return releaseAssetDispatchPayload{}, fmt.Errorf("release asset repository is invalid: %w", err)
	}
	return releaseAssetDispatchPayload{
		ReleaseRepo:      identity.Repo,
		AssetsReleaseID:  identity.AssetsRelease.ID,
		AssetsReleaseTag: identity.AssetsRelease.TagName,
		ReleaseAssets:    identity.AssetsRelease.Assets,
	}, nil
}

// DispatchBuild dispatches a workflow using the validated branch/tag selector
// and returns the exact run details from GitHub. The resolved workflow SHA is
// checked during preparation and retained for polling; it is not sent as the
// workflow_dispatch ref. It is copied into the provider-owned public guard
// input and authenticated payload. This method deliberately does not poll or
// infer a run from a list response.
func (s *GithubBuildConfigService) DispatchBuild(ctx context.Context, c *model.GithubBuildConfig, identity VersionIdentity, platform string, params map[string]any) (*GithubDispatchResult, error) {
	if err := RequireProductionBuildCapability(platform); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, &GithubProviderConfigurationError{Cause: errors.New("configuration is missing")}
	}
	if c.Token == "" || c.Repo == "" || c.PayloadKey == "" {
		return nil, &GithubProviderConfigurationError{Cause: errors.New("repo, PAT, and payload key are required")}
	}
	// Direct callers must not reach provider policy reads, encryption, or a
	// dispatch POST when the typed public-key/input gate is not satisfied.
	normalizedParams, err := NormalizeWorkflowDispatchParams(platform, params)
	if err != nil {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: err}
	}
	if err := RequireDispatchPublicKey(normalizedParams); err != nil {
		return nil, err
	}
	if err := s.RequireWorkflowRefApproval(c); err != nil {
		return nil, err
	}
	workflow, err := WorkflowFilenameForPlatform(platform)
	if err != nil {
		return nil, err
	}
	configuredWorkflowRef, err := workflowExecutionRef(c)
	if err != nil {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: err}
	}
	if identity.WorkflowRef == "" || identity.WorkflowSHA == "" {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: errors.New("workflow selector and resolved execution SHA are required")}
	}
	if identity.WorkflowRef != configuredWorkflowRef {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: errors.New("workflow dispatch ref does not match the configured workflow selector")}
	}
	if err := identity.validate(); err != nil {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: err}
	}
	if identity.Repo != c.Repo {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: fmt.Errorf("version identity repo %q does not match configured repo", identity.Repo)}
	}
	releasePayload, err := releaseAssetPayloadForIdentity(identity)
	if err != nil {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: err}
	}
	// Re-check the mapped workflow immediately before dispatch. PrepareBuild
	// performs the same readiness check before persistence, while this second
	// check rejects stale identities for direct dispatch callers and provides
	// compensating protection against provider-side movement. A provider-side
	// TOCTOU window still remains after this check.
	if err := s.verifyWorkflowAvailable(ctx, c, workflow, identity.WorkflowSHA); err != nil {
		return nil, &GithubProviderConfigurationError{
			Cause: fmt.Errorf("mapped workflow %q is not ready: %w", workflow, err),
		}
	}
	// This is the dispatch primitive's final provider-policy check. It must be
	// immediately before encryption so a moved branch/tag or stale prepared
	// identity is rejected before a DFP1 payload is produced. This is
	// compensating protection, not an atomic binding of the later dispatch to
	// the resolved SHA.
	execution, err := s.validateWorkflowRefPolicy(ctx, c)
	if err != nil {
		return nil, &GithubProviderConfigurationError{Cause: fmt.Errorf("workflow reference policy is not verified: %w", err)}
	}
	if execution.Ref != identity.WorkflowRef || execution.SHA != identity.WorkflowSHA || !strings.EqualFold(execution.SHA, c.WorkflowRefApprovalSHA) {
		return nil, &GithubContractError{
			Operation: "workflow dispatch",
			Cause:     errors.New("workflow selector resolved to a different execution SHA"),
		}
	}
	// workflow_dispatch accepts a branch or tag selector, not a raw SHA. Keep
	// this explicit assertion at the dispatch boundary: the checks above reject
	// stale identities and provide compensating protection, but GitHub's short
	// tag selection is not atomically SHA-bound, so the selector/SHA TOCTOU risk
	// remains. Non-tag selectors must never be normalized here.
	dispatchRef, err := workflowDispatchTagSelector(identity.WorkflowRef)
	if err != nil {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: err}
	}
	dispatchParams := make(map[string]any, len(normalizedParams)+1)
	for key, value := range normalizedParams {
		dispatchParams[key] = value
	}
	// Version propagation is provider/workflow-owned. Never trust a caller's
	// duplicate or stale value over the immutable selected identity.
	dispatchParams["version"] = identity.DisplayVersion
	// Keep the source checkout identity available to the owned workflow without
	// replacing the execution ref used to locate its workflow definition.
	dispatchParams["source_sha"] = identity.BuildRef
	// Bind the authenticated payload to the configured repository. The workflow
	// compares this provider-derived value with github.repository, preserving
	// self-hosted fork support without trusting a hardcoded owner/name.
	dispatchParams["workflow_repo"] = c.Repo
	// Release asset identity is catalog/provider-derived. Overwrite any caller
	// value so raw/manual asset IDs or digests can never enter a normal build.
	dispatchParams["release_repo"] = releasePayload.ReleaseRepo
	dispatchParams["assets_release_id"] = releasePayload.AssetsReleaseID
	dispatchParams["assets_release_tag"] = releasePayload.AssetsReleaseTag
	dispatchParams["release_assets"] = releasePayload.ReleaseAssets
	// The execution SHA is provider-derived and must bind both dispatch layers.
	// NormalizeWorkflowDispatchParams deliberately rejects caller-authored
	// workflow_sha values, so this overwrite remains the only normal input path.
	dispatchParams["workflow_sha"] = identity.WorkflowSHA
	enc, err := s.EncryptPayload(c.PayloadKey, dispatchParams)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	body := map[string]any{
		// The resolved SHA is used by readiness and polling, not as this selector.
		"ref":                dispatchRef,
		"return_run_details": true,
		"inputs": map[string]string{
			"enc_payload":  enc,
			"workflow_sha": identity.WorkflowSHA,
		},
	}
	path, err := githubRepoPath(c.Repo, fmt.Sprintf("/actions/workflows/%s/dispatches", workflow))
	if err != nil {
		return nil, err
	}
	resp, err := s.ghReq(ctx, c, "POST", path, body, http.StatusOK)
	if err != nil {
		var apiErr *GithubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNoContent {
			return nil, &GithubContractError{Operation: "workflow dispatch", Cause: err}
		}
		return nil, err
	}
	defer resp.Body.Close()
	var result GithubDispatchResult
	if err := decodeGithubJSON(resp, "workflow dispatch", &result); err != nil {
		return nil, err
	}
	if result.WorkflowRunID <= 0 || !safeGithubURL(result.RunURL) || !safeGithubURL(result.HTMLURL) {
		return nil, &GithubContractError{Operation: "workflow dispatch", Cause: errors.New("expected nonzero workflow_run_id and safe run_url/html_url")}
	}
	return &result, nil
}

// GithubRunStatusDetails contains the exact provider-owned fields returned by
// a run-status response. New builds require SourceSHA to guard the immutable
// workflow execution commit.
type GithubRunStatusDetails struct {
	Status     string
	Conclusion string
	SourceSHA  string
}

// RunStatusDetails fetches the status response, including the exact run
// head_sha used to identify the workflow execution commit.
func (s *GithubBuildConfigService) RunStatusDetails(ctx context.Context, c *model.GithubBuildConfig, runId int64) (GithubRunStatusDetails, error) {
	path, err := githubRepoPath(c.Repo, fmt.Sprintf("/actions/runs/%d", runId))
	if err != nil {
		return GithubRunStatusDetails{}, err
	}
	resp, err := s.ghReq(ctx, c, "GET", path, nil, http.StatusOK)
	if err != nil {
		return GithubRunStatusDetails{}, err
	}
	defer resp.Body.Close()
	var data struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		HeadSHA    string `json:"head_sha"`
	}
	if err := decodeGithubJSON(resp, "run status", &data); err != nil {
		return GithubRunStatusDetails{}, err
	}
	if data.Status == "" {
		return GithubRunStatusDetails{}, &GithubContractError{Operation: "run status", Cause: errors.New("status is empty")}
	}
	if data.HeadSHA != "" && !validGithubSourceSHA(data.HeadSHA) {
		return GithubRunStatusDetails{}, &GithubContractError{Operation: "run status", Cause: errors.New("head_sha must be 40-64 hexadecimal characters")}
	}
	return GithubRunStatusDetails{Status: data.Status, Conclusion: data.Conclusion, SourceSHA: data.HeadSHA}, nil
}

// RunStatus preserves the existing status-only caller contract.
func (s *GithubBuildConfigService) RunStatus(ctx context.Context, c *model.GithubBuildConfig, runId int64) (status, conclusion string, err error) {
	details, err := s.RunStatusDetails(ctx, c, runId)
	if err != nil {
		return "", "", err
	}
	return details.Status, details.Conclusion, nil
}

func (s *GithubBuildConfigService) listRunArtifacts(ctx context.Context, c *model.GithubBuildConfig, runId int64) ([]githubArtifactMetadata, error) {
	path, err := githubRepoPath(c.Repo, fmt.Sprintf("/actions/runs/%d/artifacts", runId))
	if err != nil {
		return nil, err
	}
	resp, err := s.ghReq(ctx, c, "GET", path, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var data struct {
		Artifacts []githubArtifactMetadata `json:"artifacts"`
	}
	if err := decodeGithubJSON(resp, "list artifacts", &data); err != nil {
		resp.Body.Close()
		return nil, err
	}
	resp.Body.Close()
	return data.Artifacts, nil
}

// ResolveArtifactID selects the exact named artifact from the exact workflow
// run. A missing or expired name is terminal; there is deliberately no
// sole-artifact fallback.
func (s *GithubBuildConfigService) ResolveArtifactID(ctx context.Context, c *model.GithubBuildConfig, runId int64, artifactName string) (int64, error) {
	artifacts, err := s.listRunArtifacts(ctx, c, runId)
	if err != nil {
		return 0, err
	}
	var aid int64
	if artifactName != "" {
		for _, a := range artifacts {
			if a.Name == artifactName {
				if a.Expired {
					return 0, &GithubArtifactUnavailableError{RunID: runId, ArtifactName: artifactName}
				}
				if a.ID <= 0 {
					return 0, &GithubContractError{Operation: "list artifacts", Cause: fmt.Errorf("artifact %q has an invalid id", artifactName)}
				}
				if aid != 0 {
					return 0, &GithubContractError{Operation: "list artifacts", Cause: fmt.Errorf("artifact name %q is ambiguous", artifactName)}
				}
				aid = a.ID
			}
		}
	}
	if aid == 0 {
		names := make([]string, 0, len(artifacts))
		for _, a := range artifacts {
			names = append(names, a.Name)
		}
		return 0, &GithubArtifactUnavailableError{RunID: runId, ArtifactName: artifactName, Available: names}
	}
	return aid, nil
}

func (s *GithubBuildConfigService) verifyRunArtifact(ctx context.Context, c *model.GithubBuildConfig, runId, artifactID int64, artifactName string) error {
	artifacts, err := s.listRunArtifacts(ctx, c, runId)
	if err != nil {
		return err
	}
	var nameMatch *githubArtifactMetadata
	for i := range artifacts {
		a := &artifacts[i]
		if a.ID == artifactID {
			if a.Expired {
				return &GithubArtifactUnavailableError{RunID: runId, ArtifactName: artifactName, Available: []string{a.Name}}
			}
			if a.Name != artifactName {
				return &GithubContractError{Operation: "verify artifact identity", Cause: fmt.Errorf("stored artifact %d belongs to %q, not %q", artifactID, a.Name, artifactName)}
			}
			return nil
		}
		if a.Name == artifactName {
			copy := *a
			nameMatch = &copy
		}
	}
	if nameMatch != nil {
		return &GithubContractError{Operation: "verify artifact identity", Cause: fmt.Errorf("stored artifact id %d does not match run artifact id %d for %q", artifactID, nameMatch.ID, artifactName)}
	}
	return &GithubArtifactUnavailableError{RunID: runId, ArtifactName: artifactName}
}

// ArtifactDownload is the bounded hand-off between the GitHub service and the
// controller. The archive is always a temporary file; callers own cleanup of
// ArchivePath after validation/publication, while the identity is provider
// derived and can be guarded into build provenance.
type ArtifactDownload struct {
	ArchivePath  string
	ArtifactID   int64
	ArtifactName string
	Size         int64
}

// DownloadArtifact resolves the exact requested artifact when its stored ID
// is absent, then streams that exact ID to a temporary archive. It never
// selects a sole artifact, infers a path, or returns the ZIP in memory.
func (s *GithubBuildConfigService) DownloadArtifact(ctx context.Context, c *model.GithubBuildConfig, runId, artifactID int64, artifactName string) (ArtifactDownload, error) {
	if runId <= 0 {
		return ArtifactDownload{}, &GithubContractError{Operation: "download artifact", Cause: errors.New("github run id must be positive")}
	}
	if artifactName == "" {
		return ArtifactDownload{}, &GithubContractError{Operation: "download artifact", Cause: errors.New("artifact name is required")}
	}
	if artifactID > 0 {
		if err := s.verifyRunArtifact(ctx, c, runId, artifactID, artifactName); err != nil {
			return ArtifactDownload{}, err
		}
	} else {
		resolvedID, err := s.ResolveArtifactID(ctx, c, runId, artifactName)
		if err != nil {
			return ArtifactDownload{}, err
		}
		artifactID = resolvedID
	}
	if artifactID <= 0 {
		return ArtifactDownload{}, &GithubArtifactUnavailableError{RunID: runId, ArtifactName: artifactName}
	}

	zipPath, err := githubRepoPath(c.Repo, fmt.Sprintf("/actions/artifacts/%d/zip", artifactID))
	if err != nil {
		return ArtifactDownload{}, err
	}
	resp, err := s.ghReq(ctx, c, "GET", zipPath, nil, http.StatusOK)
	if err != nil {
		return ArtifactDownload{}, err
	}
	defer resp.Body.Close()

	limit := githubArtifactBodyLimit
	if limit <= 0 {
		return ArtifactDownload{}, &GithubContractError{Operation: "download artifact", Cause: errors.New("artifact response limit must be positive")}
	}
	expectedLength, hasContentLength := responseContentLength(resp)
	if hasContentLength && expectedLength > limit {
		return ArtifactDownload{}, &GithubContractError{Operation: "download artifact", Cause: errors.New("artifact response exceeds limit")}
	}

	part, err := os.CreateTemp(githubArtifactTempDir, "deskforge-artifact-*.part")
	if err != nil {
		return ArtifactDownload{}, &GithubTransportError{Operation: "create artifact temporary file", Cause: err}
	}
	partPath := part.Name()
	archivePath := strings.TrimSuffix(partPath, ".part") + ".zip"
	ProtectGithubArtifactTemp(partPath)
	ProtectGithubArtifactTemp(archivePath)
	keepArchive := false
	defer func() {
		if !keepArchive {
			_ = os.Remove(partPath)
			_ = os.Remove(archivePath)
			ReleaseGithubArtifactTemp(partPath)
			ReleaseGithubArtifactTemp(archivePath)
		}
	}()

	written, copyErr := copyContextBounded(ctx, part, resp.Body, limit+1)
	if copyErr != nil {
		_ = part.Close()
		return ArtifactDownload{}, &GithubTransportError{Operation: "download artifact response body", Cause: copyErr}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		_ = part.Close()
		return ArtifactDownload{}, &GithubTransportError{Operation: "download artifact response body", Cause: ctxErr}
	}
	if written > limit {
		_ = part.Close()
		return ArtifactDownload{}, &GithubContractError{Operation: "download artifact", Cause: errors.New("artifact response exceeds limit")}
	}
	if err := part.Sync(); err != nil {
		_ = part.Close()
		return ArtifactDownload{}, &GithubTransportError{Operation: "sync artifact temporary file", Cause: err}
	}
	if err := part.Close(); err != nil {
		return ArtifactDownload{}, &GithubTransportError{Operation: "close artifact temporary file", Cause: err}
	}
	if hasContentLength && written != expectedLength {
		return ArtifactDownload{}, &GithubContractError{Operation: "download artifact", Cause: fmt.Errorf("artifact response body length %d does not match Content-Length %d", written, expectedLength)}
	}
	if err := validateDownloadedArtifactArchive(partPath); err != nil {
		return ArtifactDownload{}, &GithubContractError{Operation: "download artifact", Cause: err}
	}
	if err := os.Rename(partPath, archivePath); err != nil {
		return ArtifactDownload{}, &GithubTransportError{Operation: "publish artifact temporary file", Cause: err}
	}
	ReleaseGithubArtifactTemp(partPath)
	keepArchive = true
	return ArtifactDownload{
		ArchivePath:  archivePath,
		ArtifactID:   artifactID,
		ArtifactName: artifactName,
		Size:         written,
	}, nil
}

// copyContextBounded keeps streaming bounded even when a non-conforming body
// repeatedly returns (0, nil). The context is checked between reads; the
// transport remains responsible for interrupting a blocked network read.
func copyContextBounded(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit < 0 {
		return 0, errors.New("copy limit must not be negative")
	}
	buf := make([]byte, 32*1024)
	var written int64
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			emptyReads = 0
			if err := ctx.Err(); err != nil {
				return written, err
			}
			writtenNow, writeErr := dst.Write(buf[:n])
			written += int64(writtenNow)
			if writeErr != nil {
				return written, writeErr
			}
			if writtenNow != n {
				return written, io.ErrShortWrite
			}
			if written > limit {
				return written, nil
			}
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return written, io.ErrNoProgress
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func responseContentLength(resp *http.Response) (int64, bool) {
	if raw := resp.Header.Get("Content-Length"); raw != "" {
		length, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && length >= 0 {
			return length, true
		}
		return 0, true
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength, true
	}
	return 0, false
}

func validateDownloadedArtifactArchive(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("invalid ZIP archive: %w", err)
	}
	if err := zr.Close(); err != nil {
		return fmt.Errorf("close ZIP archive: %w", err)
	}
	return nil
}
