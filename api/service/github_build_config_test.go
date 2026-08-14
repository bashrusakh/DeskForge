package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/pbkdf2"
	"rustdesk-server/api/model"
)

type githubRoundTripFunc func(*http.Request) (*http.Response, error)

type countingGithubReader struct {
	remaining int
	read      int
}

func (r *countingGithubReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	r.read += n
	return n, nil
}

func (f githubRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withGithubTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := ghClient.Transport
	ghClient.Transport = transport
	t.Cleanup(func() { ghClient.Transport = previous })
}

func githubResponse(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func githubConfig() *model.GithubBuildConfig {
	return &model.GithubBuildConfig{
		Repo:                        "owner/repo",
		Token:                       "github_pat_test",
		PayloadKey:                  "payload-key",
		Branch:                      "refs/tags/workflow-v1",
		WorkflowRefApproved:         true,
		WorkflowRefProviderVerified: true,
		WorkflowRefApprovalSHA:      strings.Repeat("f", 40),
	}
}

func githubVersionIdentity() VersionIdentity {
	return VersionIdentity{
		Repo:           "owner/repo",
		DisplayVersion: "1.2.3",
		BuildRef:       "0123456789abcdef0123456789abcdef01234567",
		SourceTag:      "1.2.3",
		WorkflowRef:    "refs/tags/workflow-v1",
		WorkflowSHA:    strings.Repeat("f", 40),
		AssetsRelease:  AssetsRelease{ID: 12, TagName: "offline-assets-1.2.3", Assets: testReleaseAssets()},
	}
}

func testReleaseAssets() []ReleaseAsset {
	return []ReleaseAsset{
		{ID: 101, Name: "windows-x64-release.zip", Digest: "sha256:" + strings.Repeat("1", 64)},
		{ID: 102, Name: "usbmmidd_v2.zip", Digest: "sha256:" + strings.Repeat("2", 64)},
		{ID: 103, Name: "rustdesk_printer_driver_v4-1.4.zip", Digest: "sha256:" + strings.Repeat("3", 64)},
		{ID: 104, Name: "printer_driver_adapter.zip", Digest: "sha256:" + strings.Repeat("4", 64)},
	}
}

func testReleaseDetails(id int64, tag string) string {
	return fmt.Sprintf(`{"id":%d,"tag_name":%q,"assets":[{"id":101,"name":"windows-x64-release.zip","digest":"sha256:%s"},{"id":102,"name":"usbmmidd_v2.zip","digest":"sha256:%s"},{"id":103,"name":"rustdesk_printer_driver_v4-1.4.zip","digest":"sha256:%s"},{"id":104,"name":"printer_driver_adapter.zip","digest":"sha256:%s"}]}`,
		id, tag, strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64), strings.Repeat("4", 64))
}

func testWorkflowFileResponse(workflow string) string {
	return testWorkflowFileResponseWithContent(workflow, "on:\n  workflow_dispatch:\n")
}

func testWorkflowFileResponseWithContent(workflow, content string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	return fmt.Sprintf(`{"type":"file","path":".github/workflows/%s","sha":"%s","encoding":"base64","content":%q}`, workflow, strings.Repeat("c", 40), encoded)
}

func testWorkflowStateResponse() string { return `{"state":"active"}` }

func testWorkflowBranchRefResponse(sha string) string {
	return `{"ref":"refs/heads/rustqs/min-test","object":{"sha":"` + sha + `","type":"commit"}}`
}

// TestCompareSemver проверяет упорядочивание compareSemver для стабильных
// и pre-release версий: numeric > numeric-equal-non-numeric ("1.4.8" > "1.4.8-beta"),
// trailing-segments-равны только если non-zero, pre-release между равными numeric
// сегментами ("1.4.8-beta" < "1.4.8-beta.1").
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.4.8", "1.4.7", 1},
		{"1.4.7", "1.4.8", -1},
		{"1.4.8", "1.4.8", 0},
		{"1.4.8-beta", "1.4.8", -1},
		{"1.4.8", "1.4.8-beta", 1},
		{"1.4.10-beta", "1.4.9", 1},
		{"1.4.9", "1.4.10-beta", -1},
		{"1.4.8-beta", "1.4.8-beta.1", -1},
		{"1.4.8-beta.1", "1.4.8-beta", 1},
		{"1.4.8-beta", "1.4.8-beta", 0},
	}
	for _, c := range cases {
		got := compareSemver(c.a, c.b)
		if got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestGeneratePayloadKeyUsesRawURLBase64(t *testing.T) {
	svc := &GithubBuildConfigService{}
	keys := make(map[string]struct{})
	for i := 0; i < 8; i++ {
		key, err := svc.GeneratePayloadKey()
		if err != nil {
			t.Fatalf("GeneratePayloadKey() error = %v", err)
		}
		if len(key) != 43 {
			t.Fatalf("generated key length = %d, want 43", len(key))
		}
		decoded, err := base64.RawURLEncoding.DecodeString(key)
		if err != nil {
			t.Fatalf("generated key is not raw URL base64: %v", err)
		}
		if len(decoded) != 32 {
			t.Fatalf("decoded key length = %d, want 32", len(decoded))
		}
		for _, char := range key {
			if !(char >= 'A' && char <= 'Z') && !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' && char != '_' {
				t.Fatalf("generated key contains non-URL-base64 character %q", char)
			}
		}
		keys[key] = struct{}{}
	}
	if len(keys) < 2 {
		t.Fatal("generated payload keys were not random across samples")
	}
}

func TestEncryptPayloadAuthenticatesNewEnvelopeBeforeDecrypting(t *testing.T) {
	svc := &GithubBuildConfigService{}
	encoded, err := svc.EncryptPayload("payload-key", map[string]any{"version": "1.2.3", "server": "id.example"})
	if err != nil {
		t.Fatalf("EncryptPayload() error = %v", err)
	}
	decoded, err := svc.DecryptPayload("payload-key", encoded)
	if err != nil || decoded["version"] != "1.2.3" {
		t.Fatalf("DecryptPayload() = %#v, %v", decoded, err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	for _, index := range []int{len(raw) - 1, len(payloadEnvelopeMagic) + payloadSaltSize} {
		t.Run(fmt.Sprintf("tamper-%d", index), func(t *testing.T) {
			mutated := append([]byte(nil), raw...)
			mutated[index] ^= 0x01
			if _, err := svc.DecryptPayload("payload-key", base64.StdEncoding.EncodeToString(mutated)); err == nil {
				t.Fatal("DecryptPayload() accepted a tampered authenticated envelope")
			}
		})
	}
}

func TestDecryptPayloadReadsLegacySaltedPayload(t *testing.T) {
	const passphrase = "legacy-payload-key"
	plaintext := []byte(`{"version":"1.2.3","server":"id.example:21116"}`)
	salt := []byte("12345678")
	derived := pbkdf2.Key([]byte(passphrase), salt, legacyPayloadPBKDF2Iters, 48, sha256.New)
	padded := append(append([]byte(nil), plaintext...), bytes.Repeat([]byte{byte(aes.BlockSize - len(plaintext)%aes.BlockSize)}, aes.BlockSize-len(plaintext)%aes.BlockSize)...)
	block, err := aes.NewCipher(derived[:32])
	if err != nil {
		t.Fatalf("create legacy cipher: %v", err)
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, derived[32:48]).CryptBlocks(ciphertext, padded)
	raw := append([]byte("Salted__"), salt...)
	raw = append(raw, ciphertext...)

	decoded, err := (&GithubBuildConfigService{}).DecryptPayload(passphrase, base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("DecryptPayload() legacy Salted__ error = %v", err)
	}
	if decoded["version"] != "1.2.3" || decoded["server"] != "id.example:21116" {
		t.Fatalf("legacy Salted__ payload = %#v", decoded)
	}
}

// TestGetAvailableVersionsCtxCancel подтверждает, что отменённый контекст
// возвращается моментально через select перед DoChan, не заводя singleflight
// shared goroutine (герметично — без реального запроса к GitHub API).
func TestGetAvailableVersionsCtxCancel(t *testing.T) {
	s := &GithubBuildConfigService{}

	// Сбросить кеш, чтобы не было cache-hit.
	resetVersionCatalogCache()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем заранее

	start := time.Now()
	versions, err := s.GetAvailableVersions(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got err=%v versions=%v", err, versions)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("GetAvailableVersions did not honor ctx cancellation: took %v", elapsed)
	}
	// Не сбрасываем releasesCache: при cancelled ctx singleflight goroutine
	// не заводится — кеш не мутируется, тест герметичен.
}

func TestDispatchBuildUsesExactRunDetailsWithoutPolling(t *testing.T) {
	identity := githubVersionIdentity()
	config := githubConfig()
	config.Branch = identity.WorkflowRef
	var requests int32
	var listRequests int32
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&requests, 1)
		if strings.Contains(req.URL.Path, "/runs") {
			atomic.AddInt32(&listRequests, 1)
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/tags/protection") {
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		}
		if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/") {
			return githubResponse(http.StatusOK, strings.Replace(testProtectedRulesetResponse("workflow-*"), `"id":1`, `"id":1`, 1), nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/rulesets") {
			return testRulesetResponse(req, "workflow-*"), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("e", 40)+`","type":"tag"}}`, nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("e", 40)) {
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("e", 40)+`","object":{"sha":"`+identity.WorkflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/ref/heads/workflow-v1") {
			return githubResponse(http.StatusNotFound, `{"message":"branch not found"}`, nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/contents/.github/workflows/rustqs-windows-min-test.yml") {
			if req.URL.Query().Get("ref") != identity.WorkflowSHA {
				return githubResponse(http.StatusBadRequest, `{"message":"workflow readiness did not use exact SHA"}`, nil), nil
			}
			return githubResponse(http.StatusOK, testWorkflowFileResponse("rustqs-windows-min-test.yml"), nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/actions/workflows/rustqs-windows-min-test.yml") {
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		}
		if req.Method != http.MethodPost || !strings.HasSuffix(req.URL.Path, "/actions/workflows/rustqs-windows-min-test.yml/dispatches") {
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
		if payload["return_run_details"] != true {
			return githubResponse(http.StatusBadRequest, `{"message":"return_run_details missing"}`, nil), nil
		}
		if payload["ref"] != workflowDispatchRef(identity.WorkflowRef) {
			return githubResponse(http.StatusBadRequest, `{"message":"workflow selector was not used"}`, nil), nil
		}
		return githubResponse(http.StatusOK, `{"workflow_run_id":12345,"run_url":"https://api.github.com/repos/owner/repo/actions/runs/12345","html_url":"https://github.com/owner/repo/actions/runs/12345"}`, nil), nil
	}))

	result, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, identity, string(PlatformWindows), map[string]any{"server": "id.example", "key": validRustDeskPublicKey})
	if err != nil {
		t.Fatalf("DispatchBuild() error = %v", err)
	}
	if result.WorkflowRunID != 12345 || result.RunURL == "" || result.HTMLURL == "" {
		t.Fatalf("unexpected exact dispatch result: %#v", result)
	}
	if atomic.LoadInt32(&requests) != 8 || atomic.LoadInt32(&listRequests) != 0 {
		t.Fatalf("dispatch made unexpected policy/readiness/polling requests: requests=%d list=%d", requests, listRequests)
	}
	if config.Branch != identity.WorkflowRef {
		t.Fatalf("dispatch changed selector audit provenance: got %q, want %q", config.Branch, identity.WorkflowRef)
	}
}

func TestDispatchBuildBindsEncryptedSourceSHAAndImmutableWorkflowSHA(t *testing.T) {
	identity := githubVersionIdentity()
	config := githubConfig()
	config.Branch = identity.WorkflowRef
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/tags/protection") {
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		}
		if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/") {
			return testRulesetResponse(req, "workflow-*"), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/rulesets") {
			return testRulesetResponse(req, "workflow-*"), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("e", 40)+`","type":"tag"}}`, nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("e", 40)) {
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("e", 40)+`","object":{"sha":"`+identity.WorkflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/ref/heads/workflow-v1") {
			return githubResponse(http.StatusNotFound, `{"message":"branch not found"}`, nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/contents/.github/workflows/rustqs-windows-min-test.yml") {
			if req.URL.Query().Get("ref") != identity.WorkflowSHA {
				return githubResponse(http.StatusBadRequest, `{"message":"workflow readiness did not use exact SHA"}`, nil), nil
			}
			return githubResponse(http.StatusOK, testWorkflowFileResponse("rustqs-windows-min-test.yml"), nil), nil
		}
		if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/actions/workflows/rustqs-windows-min-test.yml") {
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		}
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
		if payload["ref"] != workflowDispatchRef(identity.WorkflowRef) {
			return githubResponse(http.StatusBadRequest, `{"message":"wrong workflow selector"}`, nil), nil
		}
		inputs, ok := payload["inputs"].(map[string]any)
		if !ok {
			return githubResponse(http.StatusBadRequest, `{"message":"missing encrypted inputs"}`, nil), nil
		}
		decoded, err := decryptTestPayload(inputs["enc_payload"].(string), "payload-key")
		if err != nil {
			return nil, err
		}
		if decoded["version"] != identity.DisplayVersion || decoded["source_sha"] != identity.BuildRef || decoded["workflow_repo"] != identity.Repo {
			return githubResponse(http.StatusBadRequest, `{"message":"source identity was not bound"}`, nil), nil
		}
		if decoded["release_repo"] != identity.Repo {
			return githubResponse(http.StatusBadRequest, `{"message":"release repository was not bound"}`, nil), nil
		}
		if decoded["assets_release_id"] != float64(identity.AssetsRelease.ID) || decoded["assets_release_tag"] != identity.AssetsRelease.TagName {
			return githubResponse(http.StatusBadRequest, `{"message":"release identity was not bound"}`, nil), nil
		}
		assets, ok := decoded["release_assets"].([]any)
		if !ok || len(assets) != len(identity.AssetsRelease.Assets) {
			return githubResponse(http.StatusBadRequest, `{"message":"release asset metadata was not bound"}`, nil), nil
		}
		for index, expected := range identity.AssetsRelease.Assets {
			asset, ok := assets[index].(map[string]any)
			if !ok || asset["id"] != float64(expected.ID) || asset["name"] != expected.Name || asset["digest"] != expected.Digest {
				return githubResponse(http.StatusBadRequest, `{"message":"release asset metadata changed"}`, nil), nil
			}
		}
		return githubResponse(http.StatusOK, `{"workflow_run_id":12347,"run_url":"https://api.github.com/repos/owner/repo/actions/runs/12347","html_url":"https://github.com/owner/repo/actions/runs/12347"}`, nil), nil
	}))

	if _, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, identity, string(PlatformWindows), map[string]any{
		"key":               validRustDeskPublicKey,
		"release_repo":      "attacker/other-repo",
		"assets_release_id": float64(999),
		"release_assets":    []ReleaseAsset{{ID: 999, Name: "forged.zip", Digest: "sha256:" + strings.Repeat("f", 64)}},
	}); err != nil {
		t.Fatalf("DispatchBuild() error = %v", err)
	}
	if config.Branch != identity.WorkflowRef {
		t.Fatalf("dispatch changed selector audit provenance: got %q, want %q", config.Branch, identity.WorkflowRef)
	}
}

func TestDispatchBuildRejectsUnsafeOrUnknownRawParamsBeforeProvider(t *testing.T) {
	identity := githubVersionIdentity()
	for _, test := range []struct {
		name   string
		params map[string]any
	}{
		{name: "path unsafe app name", params: map[string]any{"app_name": "../escape"}},
		{name: "control custom text", params: map[string]any{"custom_txt": "safe\nunsafe"}},
		{name: "path unsafe version", params: map[string]any{"version": "../../etc"}},
		{name: "wrong version type", params: map[string]any{"version": 1.4}},
		{name: "unknown raw field", params: map[string]any{"raw_internal": "value"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("unexpected provider request for rejected raw dispatch params")
				return nil, nil
			}))
			if _, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), githubConfig(), identity, string(PlatformWindows), test.params); err == nil {
				t.Fatal("DispatchBuild() error = nil, want raw dispatch parameter rejection")
			}
		})
	}
}

func TestReleaseAssetPayloadRejectsMalformedResolvedRepository(t *testing.T) {
	identity := githubVersionIdentity()
	identity.Repo = "owner/repo?ref=mutable"
	if _, err := releaseAssetPayloadForIdentity(identity); err == nil {
		t.Fatal("releaseAssetPayloadForIdentity() error = nil, want malformed repository rejection")
	}
}

func decryptTestPayload(encoded, passphrase string) (map[string]any, error) {
	return (&GithubBuildConfigService{}).DecryptPayload(passphrase, encoded)
}

func TestDispatchBuildRejectsMissingWorkflowSelector(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected provider request: %s", req.URL.Path)
		return nil, nil
	}))
	identity := githubVersionIdentity()
	identity.WorkflowRef = ""
	if _, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), githubConfig(), identity, string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey}); err == nil {
		t.Fatal("DispatchBuild() error = nil, want missing selector rejection")
	}
}

func TestDispatchBuildRequiresConfiguredWorkflowSelectorIdentity(t *testing.T) {
	identity := githubVersionIdentity()
	for _, tc := range []struct {
		name   string
		branch string
	}{
		{name: "tampered tag selector", branch: "refs/tags/workflow-v2"},
		{name: "unknown legacy selector", branch: "legacy-ref"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatalf("unexpected provider request for mismatched workflow identity: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}))
			config := githubConfig()
			config.Branch = tc.branch
			if _, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, identity, string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey}); err == nil {
				t.Fatal("DispatchBuild() error = nil, want configured-selector identity rejection")
			}
		})
	}
}

func TestDispatchBuildRejectsMissingOrInvalidWorkflowSHA(t *testing.T) {
	cases := []struct {
		name string
		sha  string
	}{
		{name: "missing", sha: ""},
		{name: "invalid", sha: strings.Repeat("g", 40)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatalf("unexpected provider request: %s", req.URL.Path)
				return nil, nil
			}))
			identity := githubVersionIdentity()
			identity.WorkflowSHA = tc.sha
			if _, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), githubConfig(), identity, string(PlatformWindows), nil); err == nil {
				t.Fatal("DispatchBuild() error = nil, want fail-closed workflow SHA rejection")
			}
		})
	}
}

func TestDispatchBuildRejectsInvalidReleaseAssetIdentity(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected provider request: %s", req.URL.Path)
		return nil, nil
	}))
	identity := githubVersionIdentity()
	identity.AssetsRelease.Assets[0].Digest = "sha256:not-a-digest"
	if _, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), githubConfig(), identity, string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey}); err == nil {
		t.Fatal("DispatchBuild() error = nil, want invalid release asset rejection")
	}
}

func TestDispatchBuildRejectsUnvalidatedPlatformsAtServiceBoundary(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected provider request for unavailable platform")
		return nil, nil
	}))
	for _, platform := range []string{string(PlatformLinux), string(PlatformAndroid)} {
		t.Run(platform, func(t *testing.T) {
			result, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), githubConfig(), githubVersionIdentity(), platform, nil)
			var unavailable *ProductionCapabilityUnavailableError
			if result != nil || !errors.As(err, &unavailable) {
				t.Fatalf("DispatchBuild(%q) = %#v, %T %v; want capability rejection", platform, result, err, err)
			}
		})
	}
}

func TestDispatchBuildRejectsMissingMalformedZeroAnd204(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "missing run id", status: http.StatusOK, body: `{"run_url":"https://api.github.com/runs/1","html_url":"https://github.com/runs/1"}`},
		{name: "zero run id", status: http.StatusOK, body: `{"workflow_run_id":0,"run_url":"https://api.github.com/runs/1","html_url":"https://github.com/runs/1"}`},
		{name: "malformed body", status: http.StatusOK, body: `{"workflow_run_id":`},
		{name: "unexpected 204", status: http.StatusNoContent, body: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identity := githubVersionIdentity()
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/tags/protection") {
					return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
				}
				if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/") {
					return testRulesetResponse(req, "workflow-*"), nil
				}
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/rulesets") {
					return testRulesetResponse(req, "workflow-*"), nil
				}
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
					return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("e", 40)+`","type":"tag"}}`, nil), nil
				}
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("e", 40)) {
					return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("e", 40)+`","object":{"sha":"`+identity.WorkflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
				}
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/git/ref/heads/workflow-v1") {
					return githubResponse(http.StatusNotFound, `{"message":"branch not found"}`, nil), nil
				}
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/contents/.github/workflows/rustqs-windows-min-test.yml") {
					return githubResponse(http.StatusOK, testWorkflowFileResponse("rustqs-windows-min-test.yml"), nil), nil
				}
				if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/actions/workflows/rustqs-windows-min-test.yml") {
					return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
				}
				return githubResponse(tc.status, tc.body, nil), nil
			}))
			result, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), githubConfig(), identity, string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey})
			if err == nil || result != nil {
				t.Fatalf("DispatchBuild() = %#v, %v; want terminal error", result, err)
			}
			if !IsGithubTerminal(err) {
				t.Fatalf("DispatchBuild() error is not terminal: %T %v", err, err)
			}
			if tc.status == http.StatusNoContent {
				var contractErr *GithubContractError
				if !errors.As(err, &contractErr) {
					t.Fatalf("204 dispatch error = %T %v, want GithubContractError", err, err)
				}
			}
		})
	}
}

func TestGithubRESTStatusClassificationIsBoundedAndRedacted(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		headers   http.Header
		retryable bool
		terminal  bool
	}{
		{name: "401", status: http.StatusUnauthorized, terminal: true},
		{name: "403 ordinary", status: http.StatusForbidden, terminal: true},
		{name: "403 rate limit", status: http.StatusForbidden, headers: http.Header{"X-Ratelimit-Remaining": []string{"0"}}, retryable: true},
		{name: "403 retry after", status: http.StatusForbidden, headers: http.Header{"Retry-After": []string{"30"}}, retryable: true},
		{name: "404", status: http.StatusNotFound, terminal: true},
		{name: "409", status: http.StatusConflict, terminal: true},
		{name: "422", status: http.StatusUnprocessableEntity, terminal: true},
		{name: "429", status: http.StatusTooManyRequests, retryable: true},
		{name: "500", status: http.StatusInternalServerError, retryable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(*http.Request) (*http.Response, error) {
				body := strings.Repeat(`{"message":"token=github_pat_secret","enc_payload":"encrypted-payload"}`, 200)
				return githubResponse(tc.status, body, tc.headers), nil
			}))
			_, err := (&GithubBuildConfigService{}).ghReq(context.Background(), githubConfig(), http.MethodGet, "/repos/owner/repo", nil, http.StatusOK)
			var apiErr *GithubAPIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("ghReq() error = %T %v, want GithubAPIError", err, err)
			}
			if apiErr.Retryable != tc.retryable || apiErr.Terminal != tc.terminal {
				t.Fatalf("classification = retryable:%v terminal:%v, want retryable:%v terminal:%v", apiErr.Retryable, apiErr.Terminal, tc.retryable, tc.terminal)
			}
			if len(apiErr.Body) > githubErrorBodyLimit || strings.Contains(apiErr.Body, "github_pat_secret") || strings.Contains(apiErr.Body, "encrypted-payload") {
				t.Fatalf("unsafe/unbounded API body: len=%d body=%q", len(apiErr.Body), apiErr.Body)
			}
		})
	}
}

func TestGithubTransportErrorsAreRetryable(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}))
	_, err := (&GithubBuildConfigService{}).ghReq(context.Background(), githubConfig(), http.MethodGet, "/repos/owner/repo", nil, http.StatusOK)
	var transportErr *GithubTransportError
	if !errors.As(err, &transportErr) || !IsGithubRetryable(err) || IsGithubTerminal(err) {
		t.Fatalf("transport classification = %T retryable=%v terminal=%v err=%v", err, IsGithubRetryable(err), IsGithubTerminal(err), err)
	}
}

func TestGithubRequestRejectsUnsafeRepoBeforeTransport(t *testing.T) {
	var requests int32
	withGithubTransport(t, githubRoundTripFunc(func(*http.Request) (*http.Response, error) {
		atomic.AddInt32(&requests, 1)
		return githubResponse(http.StatusOK, `{}`, nil), nil
	}))
	for _, repo := range []string{"../repo", "owner/..", "./repo", ".", "..", `owner\\repo`} {
		t.Run(repo, func(t *testing.T) {
			cfg := githubConfig()
			cfg.Repo = repo
			_, err := (&GithubBuildConfigService{}).ghReq(context.Background(), cfg, http.MethodGet, "/repos/"+repo, nil, http.StatusOK)
			if err == nil {
				t.Fatalf("ghReq(%q) error = nil, want repository validation error", repo)
			}
		})
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("unsafe repository reached transport %d time(s)", got)
	}
}

func TestGithubConfigSaveRejectsUnsafeRepoBeforeDatabaseAccess(t *testing.T) {
	for _, repo := range []string{"../repo", "owner/..", "./repo", ".", "..", `owner\\repo`, "owner/repo?ref=main"} {
		t.Run(repo, func(t *testing.T) {
			err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{Repo: repo})
			if err == nil {
				t.Fatalf("Save(%q) error = nil, want repository validation error", repo)
			}
		})
	}
}

func TestDispatchBuildRejectsMissingProviderAsTypedConfigurationError(t *testing.T) {
	identity := githubVersionIdentity()
	for _, config := range []*model.GithubBuildConfig{
		nil,
		{Repo: "owner/repo", Token: "", PayloadKey: "payload-key"},
		{Repo: "owner/repo", Token: "github_pat_test", PayloadKey: ""},
		{Repo: "", Token: "github_pat_test", PayloadKey: "payload-key"},
	} {
		_, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, identity, string(PlatformWindows), nil)
		var providerErr *GithubProviderConfigurationError
		if !errors.As(err, &providerErr) {
			t.Fatalf("DispatchBuild(%#v) error = %T %v, want GithubProviderConfigurationError", config, err, err)
		}
	}
}

func TestDispatchBuildRejectsNilOrEmptyParamsBeforeDFP1OrProviderPost(t *testing.T) {
	for _, test := range []struct {
		name   string
		params map[string]any
	}{
		{name: "nil params"},
		{name: "empty params", params: map[string]any{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests int32
			var posts int32
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				atomic.AddInt32(&requests, 1)
				if req.Method == http.MethodPost {
					atomic.AddInt32(&posts, 1)
				}
				t.Fatalf("unexpected provider request before missing public-key gate: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}))
			_, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), githubConfig(), githubVersionIdentity(), string(PlatformWindows), test.params)
			var providerErr *GithubProviderConfigurationError
			if !errors.As(err, &providerErr) || !strings.Contains(err.Error(), "public key") {
				t.Fatalf("DispatchBuild() error = %T %v, want typed public-key gate", err, err)
			}
			if requests != 0 || posts != 0 {
				t.Fatalf("DispatchBuild() requests=%d posts=%d, want no provider or DFP1 dispatch", requests, posts)
			}
		})
	}
}

func TestGithubResponseReadsAreBounded(t *testing.T) {
	reader := &countingGithubReader{remaining: githubErrorBodyLimit * 10}
	body, truncated, err := readGithubBody(reader, githubErrorBodyLimit)
	if err != nil || !truncated {
		t.Fatalf("readGithubBody() = len:%d truncated:%v err:%v", len(body), truncated, err)
	}
	if len(body) != githubErrorBodyLimit+1 || reader.read != githubErrorBodyLimit+1 {
		t.Fatalf("readGithubBody() consumed unbounded body: returned=%d read=%d", len(body), reader.read)
	}
}

func TestGithubRedactionCoversSensitiveValueCutAtBodyLimit(t *testing.T) {
	prefix := `{"enc_payload":"`
	body := []byte(prefix + strings.Repeat("s", githubErrorBodyLimit-len(prefix)+10))
	bounded, truncated, err := readGithubBody(strings.NewReader(string(body)), githubErrorBodyLimit)
	if err != nil || !truncated {
		t.Fatalf("readGithubBody() = truncated:%v err:%v, want truncated body", truncated, err)
	}
	message := redactGithubBody(bounded)
	if strings.Contains(message, "s") {
		t.Fatalf("truncated sensitive field leaked in redacted body: %q", message)
	}
	if !strings.Contains(message, "[REDACTED]") {
		t.Fatalf("truncated sensitive field was not redacted: %q", message)
	}
}

func TestGithubRedactionHandlesSensitiveValuesCutAtBoundedInput(t *testing.T) {
	cases := []struct {
		name         string
		field        string
		secret       string
		secretPrefix string
	}{
		{name: "enc payload", field: `{"enc_payload":"`, secret: strings.Repeat("e", 64), secretPrefix: "eeee"},
		{name: "token", field: `{"token":"`, secret: "token-boundary-secret", secretPrefix: "token-boundary"},
		{name: "authorization", field: "Authorization: Bearer ", secret: "authorization-boundary-secret", secretPrefix: "authorization-boundary"},
		{name: "PAT", field: "PAT=", secret: "github_pat_boundary_secret", secretPrefix: "github_pat_boundary"},
		{name: "payload key", field: "payload-key=", secret: "payload-key-boundary-secret", secretPrefix: "payload-key-boundary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			padding := githubErrorBodyLimit - len(tc.field) - 1
			body := []byte(strings.Repeat("x", padding) + tc.field + tc.secret)
			bounded, truncated, err := readGithubBody(strings.NewReader(string(body)), githubErrorBodyLimit)
			if err != nil || !truncated || len(bounded) != githubErrorBodyLimit+1 {
				t.Fatalf("readGithubBody() = len:%d truncated:%v err:%v", len(bounded), truncated, err)
			}
			message := redactGithubBody(bounded)
			if strings.Contains(message, tc.secretPrefix) {
				t.Fatalf("sensitive prefix leaked after bounded redaction: %q", message)
			}
		})
	}
}

func TestGithubErrorDetailIsBoundedAndDoesNotFormatNestedProviderData(t *testing.T) {
	err := fmt.Errorf("outer: %w", &GithubProviderConfigurationError{Cause: fmt.Errorf(
		"provider response https://api.github.com/repos/owner/repo/refs/tags/private: %w",
		&GithubAPIError{
			StatusCode: http.StatusForbidden,
			Terminal:   true,
			Body:       `{"message":"token=github_pat_secret enc_payload=private-payload"}`,
		},
	)})
	detail := GithubErrorDetail(err)
	if len(detail) > githubDiagnosticLimit {
		t.Fatalf("GithubErrorDetail() length = %d, want <= %d", len(detail), githubDiagnosticLimit)
	}
	for _, forbidden := range []string{
		"api.github.com",
		"refs/tags/private",
		"github_pat_secret",
		"private-payload",
		"token=",
		"enc_payload",
	} {
		if strings.Contains(detail, forbidden) {
			t.Fatalf("GithubErrorDetail() leaked %q: %q", forbidden, detail)
		}
	}
	if !strings.Contains(detail, "status=403") || !strings.Contains(detail, "GitHub provider request failed") {
		t.Fatalf("GithubErrorDetail() = %q, want safe status/classification", detail)
	}
}

func TestGithubRESTSuccessPathsUseExpectedStatusBeforeDecode(t *testing.T) {
	publicKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	artifactPayload := serviceArtifactZip(t, "zip-bytes")
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/actions/runs/7"):
			return githubResponse(http.StatusOK, `{"status":"in_progress","conclusion":null}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/artifacts"):
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"artifact"}]}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/artifacts/42/zip"):
			return githubResponse(http.StatusOK, string(artifactPayload), nil), nil
		case strings.HasSuffix(req.URL.Path, "/releases"):
			return githubResponse(http.StatusOK, `[{"id":12,"tag_name":"offline-assets-1.2.3"}]`, nil), nil
		case req.URL.Path == "/repos/owner/repo/releases/12":
			return githubResponse(http.StatusOK, testReleaseDetails(12, "offline-assets-1.2.3"), nil), nil
		case req.URL.Path == "/repos/owner/repo/git/ref/tags/1.2.3":
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/1.2.3","object":{"sha":"0123456789abcdef0123456789abcdef01234567","type":"commit"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/actions/secrets/public-key"):
			return githubResponse(http.StatusOK, `{"key_id":"key-id","key":"`+publicKey+`"}`, nil), nil
		case strings.Contains(req.URL.Path, "/actions/secrets/"):
			return githubResponse(http.StatusNoContent, "", nil), nil
		case req.URL.Path == "/repos/owner/repo":
			return githubResponse(http.StatusOK, `{}`, nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	s := &GithubBuildConfigService{}
	cfg := githubConfig()
	status, conclusion, err := s.RunStatus(context.Background(), cfg, 7)
	if err != nil || status != "in_progress" || conclusion != "" {
		t.Fatalf("RunStatus() = %q, %q, %v", status, conclusion, err)
	}
	artifact, err := s.DownloadArtifact(context.Background(), cfg, 7, 0, "artifact")
	if err != nil {
		t.Fatalf("DownloadArtifact() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(artifact.ArchivePath) })
	contents, err := os.ReadFile(artifact.ArchivePath)
	if err != nil || !bytes.Equal(contents, artifactPayload) || artifact.ArtifactID != 42 || artifact.ArtifactName != "artifact" || artifact.Size != int64(len(contents)) {
		t.Fatalf("DownloadArtifact() = %#v, contents=%q, err=%v", artifact, contents, err)
	}
	versions, err := s.fetchReleasesWithConfig(context.Background(), cfg, WorkflowExecutionIdentity{Ref: defaultWorkflowExecutionRef, SHA: strings.Repeat("f", 40)})
	if err != nil || len(versions) != 1 || versions[0].DisplayVersion != "1.2.3" {
		t.Fatalf("fetchReleasesWithConfig() = %#v, %v", versions, err)
	}
	if err := s.TestConnectionError(cfg); err != nil {
		t.Fatalf("TestConnectionError() = %v", err)
	}
	if err := s.SetWorkflowSecret(cfg); err != nil {
		t.Fatalf("SetWorkflowSecret() = %v", err)
	}
}

func TestRunStatusDetailsCapturesExactHeadSHA(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/repos/owner/repo/actions/runs/7" {
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
		return githubResponse(http.StatusOK, `{"status":"completed","conclusion":"success","head_sha":"0123456789abcdef0123456789abcdef01234567"}`, nil), nil
	}))
	details, err := (&GithubBuildConfigService{}).RunStatusDetails(context.Background(), githubConfig(), 7)
	if err != nil {
		t.Fatalf("RunStatusDetails() error = %v", err)
	}
	if details.Status != "completed" || details.Conclusion != "success" || details.SourceSHA != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("RunStatusDetails() = %#v, want exact status/conclusion/head_sha", details)
	}
}

func TestRunStatusDetailsRejectsUnboundedOrInvalidHeadSHA(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return githubResponse(http.StatusOK, `{"status":"completed","conclusion":"failure","head_sha":"not-a-sha"}`, nil), nil
	}))
	_, err := (&GithubBuildConfigService{}).RunStatusDetails(context.Background(), githubConfig(), 7)
	var contractErr *GithubContractError
	if !errors.As(err, &contractErr) || !strings.Contains(err.Error(), "head_sha") {
		t.Fatalf("RunStatusDetails() error = %T %v, want bounded head_sha contract rejection", err, err)
	}
}

func TestDownloadArtifactRequiresExactRequestedName(t *testing.T) {
	var zipRequests int32
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/artifacts") {
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"different-artifact"}]}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/artifacts/42/zip") {
			atomic.AddInt32(&zipRequests, 1)
			return githubResponse(http.StatusOK, "must-not-download", nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
	}))

	_, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), githubConfig(), 7, 0, "requested-artifact")
	var unavailable *GithubArtifactUnavailableError
	if !errors.As(err, &unavailable) || !IsGithubTerminal(err) || IsGithubRetryable(err) {
		t.Fatalf("DownloadArtifact() error = %T %v, want typed terminal availability error", err, err)
	}
	if atomic.LoadInt32(&zipRequests) != 0 {
		t.Fatal("DownloadArtifact() downloaded an arbitrary sole artifact")
	}
}

func TestGithubEndpointErrorsRemainTypedAcrossCallers(t *testing.T) {
	type endpointCase struct {
		name string
		call func(*GithubBuildConfigService, *model.GithubBuildConfig) error
	}
	endpoints := []endpointCase{
		{name: "run status", call: func(s *GithubBuildConfigService, c *model.GithubBuildConfig) error {
			_, _, err := s.RunStatus(context.Background(), c, 7)
			return err
		}},
		{name: "artifact list", call: func(s *GithubBuildConfigService, c *model.GithubBuildConfig) error {
			_, err := s.DownloadArtifact(context.Background(), c, 7, 0, "artifact")
			return err
		}},
		{name: "release list", call: func(s *GithubBuildConfigService, c *model.GithubBuildConfig) error {
			_, err := s.fetchReleasesWithConfig(context.Background(), c, WorkflowExecutionIdentity{Ref: defaultWorkflowExecutionRef, SHA: strings.Repeat("f", 40)})
			return err
		}},
		{name: "test connection", call: func(s *GithubBuildConfigService, c *model.GithubBuildConfig) error {
			return s.TestConnectionError(c)
		}},
		{name: "secret public key", call: func(s *GithubBuildConfigService, c *model.GithubBuildConfig) error {
			return s.SetWorkflowSecret(c)
		}},
	}
	statuses := []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "401", status: http.StatusUnauthorized},
		{name: "403", status: http.StatusForbidden},
		{name: "404", status: http.StatusNotFound},
		{name: "410", status: http.StatusGone},
		{name: "409", status: http.StatusConflict},
		{name: "422", status: http.StatusUnprocessableEntity},
		{name: "429", status: http.StatusTooManyRequests, retryable: true},
		{name: "500", status: http.StatusInternalServerError, retryable: true},
		{name: "408", status: http.StatusRequestTimeout, retryable: true},
		{name: "425", status: http.StatusTooEarly, retryable: true},
	}
	for _, endpoint := range endpoints {
		for _, status := range statuses {
			t.Run(endpoint.name+"/"+status.name, func(t *testing.T) {
				withGithubTransport(t, githubRoundTripFunc(func(*http.Request) (*http.Response, error) {
					return githubResponse(status.status, `{"message":"bounded failure"}`, nil), nil
				}))
				err := endpoint.call(&GithubBuildConfigService{}, githubConfig())
				var apiErr *GithubAPIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("error = %T %v, want GithubAPIError", err, err)
				}
				if apiErr.Retryable != status.retryable || apiErr.Terminal == status.retryable {
					t.Fatalf("classification = retryable:%v terminal:%v for status %d", apiErr.Retryable, apiErr.Terminal, status.status)
				}
			})
		}
	}
}

func TestGithubPollErrorPolicyDoesNotRetryTerminalErrors(t *testing.T) {
	terminal := &GithubAPIError{StatusCode: http.StatusUnauthorized, Terminal: true}
	retryable := &GithubAPIError{StatusCode: http.StatusTooManyRequests, Retryable: true}
	if IsGithubRetryable(terminal) || githubPollErrorActionForService(terminal) != false {
		t.Fatalf("terminal API error was classified as retryable")
	}
	if !IsGithubRetryable(retryable) || githubPollErrorActionForService(retryable) != true {
		t.Fatalf("retryable API error was not retained as retryable")
	}
}

func githubPollErrorActionForService(err error) bool {
	return IsGithubRetryable(err) && !IsGithubTerminal(err)
}
