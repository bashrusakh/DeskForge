package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"rustdesk-server/api/global"
	"rustdesk-server/api/http/middleware"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"rustdesk-server/api/utils"
)

func TestDispatchTestResponseIncludesProviderRunDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	htmlURL := "https://github.com/owner/repo/actions/runs/12345"
	runURL := "https://api.github.com/repos/owner/repo/actions/runs/12345"
	response.Success(context, dispatchTestResponse(&service.GithubDispatchResult{
		WorkflowRunID: 12345,
		RunURL:        runURL,
		HTMLURL:       htmlURL,
	}))
	if recorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload struct {
		Data struct {
			RunID   int64  `json:"run_id"`
			RunURL  string `json:"run_url"`
			HTMLURL string `json:"html_url"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid dispatch response: %v", err)
	}

	if payload.Data.RunID != 12345 {
		t.Errorf("run_id = %d, want 12345", payload.Data.RunID)
	}
	if payload.Data.RunURL != runURL {
		t.Errorf("run_url = %q, want %q", payload.Data.RunURL, runURL)
	}
	if payload.Data.HTMLURL != htmlURL {
		t.Errorf("html_url = %q, want %q", payload.Data.HTMLURL, htmlURL)
	}
	if payload.Data.Status != "dispatched" {
		t.Errorf("status = %q, want dispatched", payload.Data.Status)
	}
	wantMessage := "Smoke-test build dispatched. Check status at " + htmlURL
	if payload.Data.Message != wantMessage {
		t.Errorf("message = %q, want %q", payload.Data.Message, wantMessage)
	}
}

func TestDispatchTestParamsUseTypedConfiguredValues(t *testing.T) {
	params, err := normalizeDispatchTestParams("id.example:21116", "public-key", "1.4.8")
	if err != nil {
		t.Fatalf("normalizeDispatchTestParams() error = %v", err)
	}
	customTxt, ok := params["custom_txt"].(service.NormalizedCustomTxt)
	if !ok || customTxt.Value() != "" || params["server"] != "id.example:21116" || params["key"] != "public-key" || params["app_name"] != "deskforge-smoketest" {
		t.Fatalf("normalized smoke-test params = %#v", params)
	}

	for _, test := range []struct {
		name   string
		server string
		key    string
	}{
		{name: "server newline", server: "id.example:21116\n"},
		{name: "key newline", server: "id.example:21116", key: "public\nkey"},
		{name: "key nul", server: "id.example:21116", key: "public\x00key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeDispatchTestParams(test.server, test.key, "1.4.8"); err == nil {
				t.Fatal("normalizeDispatchTestParams() error = nil, want unsafe configured value rejection")
			}
		})
	}

}

func TestGithubSecretPersistenceErrorUsesServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	if !failGithubConfigError(context, &utils.SecretEncryptionKeyError{}) {
		t.Fatal("failGithubConfigError() did not handle secret configuration error")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("error status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(recorder.Body.String(), "github_pat_secret") {
		t.Fatalf("secret value leaked in error response: %s", recorder.Body.String())
	}
}

func TestGithubProviderErrorsUseTypedSafeResponses(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		want   string
	}{
		{name: "missing provider config", err: &service.GithubProviderConfigurationError{Cause: errors.New("payload key is not configured")}, status: http.StatusServiceUnavailable, want: "GitHub build provider is not configured"},
		{name: "workflow approval", err: &service.WorkflowRefApprovalError{Reason: "selector rustqs/workflows is not approved"}, status: http.StatusPreconditionFailed, want: "workflow reference approval is required"},
		{name: "provider response", err: &service.GithubContractError{Operation: "dispatch", Cause: errors.New("invalid response")}, status: http.StatusBadGateway, want: "GitHub provider response was invalid"},
		{name: "malformed provider response with sensitive details", err: &service.GithubContractError{Operation: "dispatch refs/tags/workflow-v1", Cause: errors.New(`response body https://api.github.com/repos/owner/repo?token=github_pat_secret enc_payload=ciphertext`)}, status: http.StatusBadGateway, want: "GitHub provider response was invalid"},
		{name: "ruleset bypass metadata permission", err: &service.GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New(`ruleset bypass metadata is missing or not visible: https://api.github.com/repos/owner/repo/rulesets/1 token=github_pat_ruleset_secret`)}, status: http.StatusBadGateway, want: "GitHub workflow tag verification requires a PAT with Administration: write and repository access"},
		{name: "provider authentication with sensitive body", err: &service.GithubAPIError{StatusCode: http.StatusUnauthorized, Terminal: true, Body: `{"message":"token=github_pat_secret enc_payload=ciphertext"}`}, status: http.StatusBadGateway, want: "GitHub authentication failed; verify the configured PAT"},
		{name: "provider permission with sensitive body", err: &service.GithubAPIError{StatusCode: http.StatusForbidden, Terminal: true, Body: `{"message":"token=github_pat_secret enc_payload=ciphertext"}`}, status: http.StatusBadGateway, want: "GitHub access was denied; verify the PAT permissions and repository access"},
		{name: "provider resource not found", err: &service.GithubAPIError{StatusCode: http.StatusNotFound, Terminal: true}, status: http.StatusBadGateway, want: "GitHub repository or workflow resource was not found; verify the repository name and PAT access"},
		{name: "provider temporary failure", err: &service.GithubAPIError{StatusCode: http.StatusBadGateway, Retryable: true}, status: http.StatusBadGateway, want: "GitHub provider is temporarily unavailable or rate-limited; retry shortly"},
		{name: "unknown internal workflow error", err: errors.New(`workflow refs/tags/workflow-v1 failed at https://api.github.com/repos/owner/repo: ciphertext`), status: http.StatusInternalServerError, want: "GitHub build operation failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			if !failGithubConfigError(context, test.err) {
				t.Fatal("failGithubConfigError() did not handle typed provider error")
			}
			if recorder.Code != test.status {
				t.Fatalf("error status = %d, want %d", recorder.Code, test.status)
			}
			body := recorder.Body.String()
			forbidden := []string{"payload key is not configured", "api.github.com", "owner/repo", "refs/tags/workflow-v1", "github_pat_secret", "github_pat_ruleset_secret", "ciphertext", "enc_payload"}
			if !strings.Contains(body, test.want) {
				t.Fatalf("missing safe provider response %q: %s", test.want, body)
			}
			for _, value := range forbidden {
				if strings.Contains(body, value) {
					t.Fatalf("unsafe provider response contains %q: %s", value, body)
				}
			}
		})
	}

}

func TestGithubNestedProviderErrorsPreserveSpecificClassification(t *testing.T) {
	for _, test := range []struct {
		name   string
		inner  error
		status int
		want   string
	}{
		{
			name:   "workflow policy inside provider configuration",
			inner:  &service.WorkflowRefApprovalError{Reason: "mutable selector"},
			status: http.StatusPreconditionFailed,
			want:   "workflow reference approval is required",
		},
		{
			name:   "capability inside provider configuration",
			inner:  &service.ProductionCapabilityUnavailableError{Platform: "linux", Capability: "completion"},
			status: http.StatusServiceUnavailable,
			want:   "production build capability is unavailable",
		},
		{
			name:   "transport inside provider configuration",
			inner:  &service.GithubTransportError{Operation: "GET https://api.github.com/private/ref", Cause: errors.New("token=github_pat_nested")},
			status: http.StatusServiceUnavailable,
			want:   "GitHub provider is unavailable",
		},
		{
			name:   "contract inside provider configuration",
			inner:  &service.GithubContractError{Operation: "run refs/tags/private", Cause: errors.New("response body contains secret")},
			status: http.StatusBadGateway,
			want:   "GitHub provider response was invalid",
		},
		{
			name:   "API inside provider configuration",
			inner:  &service.GithubAPIError{StatusCode: http.StatusForbidden, Terminal: true, Body: `{"message":"payload=secret"}`},
			status: http.StatusBadGateway,
			want:   "GitHub access was denied; verify the PAT permissions and repository access",
		},
		{
			name:   "artifact contract inside provider configuration",
			inner:  &service.GithubArtifactUnavailableError{RunID: 7, ArtifactName: "private-artifact"},
			status: http.StatusBadGateway,
			want:   "requested GitHub artifact is unavailable",
		},
		{
			name:   "validation inside provider configuration",
			inner:  &service.ClientValidationError{Err: errors.New("platform has unsupported value \"linux\"")},
			status: http.StatusBadRequest,
			want:   "custom build request is invalid",
		},
		{
			name:   "secret configuration inside provider configuration",
			inner:  &utils.SecretEncryptionKeyError{},
			status: http.StatusServiceUnavailable,
			want:   "secret encryption is not configured",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			err := fmt.Errorf("outer provider wrapper: %w", &service.GithubProviderConfigurationError{
				Cause: fmt.Errorf("inner workflow wrapper: %w", test.inner),
			})
			if !failGithubConfigError(context, err) {
				t.Fatal("failGithubConfigError() did not handle nested provider error")
			}
			if recorder.Code != test.status {
				t.Fatalf("HTTP status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			body := recorder.Body.String()
			if !strings.Contains(body, test.want) {
				t.Fatalf("response = %s, want safe message %q", body, test.want)
			}
			for _, forbidden := range []string{"api.github.com", "refs/tags/private", "github_pat_nested", "payload=secret", "response body contains secret", "platform has unsupported"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("nested provider response leaked %q: %s", forbidden, body)
				}
			}
		})
	}
}

func TestWorkflowRefApprovalEndpointRequiresAdminAndReturnsSafeStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Setenv(utils.SecretEncryptionKeyEnv, "workflow-approval-controller-test")
	if err := db.AutoMigrate(&model.GithubBuildConfig{}); err != nil {
		t.Fatalf("migrate config: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo"}).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}
	previousTransport := http.DefaultTransport
	http.DefaultTransport = githubApprovalRoundTripper(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/tags/protection") {
			t.Fatalf("legacy tag-protection endpoint was requested: %s %s", req.Method, req.URL.Path)
		}
		body := `{"message":"unexpected endpoint"}`
		status := http.StatusNotFound
		if strings.HasSuffix(req.URL.Path, "/rulesets/1") {
			status = http.StatusOK
			body = `{"id":1,"name":"workflow-protection","source_type":"Repository","source":"owner/repo","created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z","target":"tag","enforcement":"active","bypass_actors":[],"current_user_can_bypass":"never","conditions":{"ref_name":{"include":["refs/tags/workflow-*"],"exclude":[]}},"rules":[{"type":"tag_name_pattern","parameters":{"name":"tag_name","negate":false,"operator":"starts_with","pattern":"workflow-"}},{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}]}`
		} else if strings.HasSuffix(req.URL.Path, "/rulesets") {
			status = http.StatusOK
			body = `[{"id":1}]`
		} else if strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
			status = http.StatusOK
			body = `{"ref":"refs/tags/workflow-v1","object":{"sha":"` + strings.Repeat("b", 40) + `","type":"tag"}}`
		} else if strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("b", 40)) {
			status = http.StatusOK
			body = `{"sha":"` + strings.Repeat("b", 40) + `","object":{"sha":"` + strings.Repeat("a", 40) + `","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`
		} else if strings.Contains(req.URL.Path, "/contents/.github/workflows/") {
			status = http.StatusOK
			body = `{"type":"file","path":".github/workflows/rustqs-windows.yml","sha":"` + strings.Repeat("c", 40) + `","encoding":"base64","content":"b246CiAgd29ya2Zsb3dfZGlzcGF0Y2g6Cg=="}`
		} else if strings.Contains(req.URL.Path, "/actions/workflows/") {
			status = http.StatusOK
			body = `{"state":"active"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	previousDB, previousServices, previousLocalizer, previousLogger := service.DB, service.AllService, global.Localizer, global.Logger
	service.DB = db
	service.AllService = &service.Service{
		UserService:              &service.UserService{},
		GithubBuildConfigService: &service.GithubBuildConfigService{},
	}
	global.Localizer = testManifestLocalizer
	global.Logger = logrus.New()
	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousServices
		global.Localizer = previousLocalizer
		global.Logger = previousLogger
	})

	for _, tc := range []struct {
		name     string
		user     *model.User
		code     int
		wantHTTP int
	}{
		{name: "non-admin", user: &model.User{IsAdmin: boolPointer(false)}, code: 403, wantHTTP: http.StatusOK},
		{name: "admin", user: &model.User{IsAdmin: boolPointer(true)}, code: 0, wantHTTP: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newWorkflowApprovalRouter(tc.user)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/admin/github_build_config/approve_workflow_ref", strings.NewReader(`{"confirm":true,"workflow_tag":"workflow-v1"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantHTTP {
				t.Fatalf("HTTP status = %d, want %d: %s", recorder.Code, tc.wantHTTP, recorder.Body.String())
			}
			var envelope struct {
				Code int `json:"code"`
				Data struct {
					WorkflowRef string `json:"workflow_ref"`
					Approved    bool   `json:"workflow_ref_approved"`
					Status      string `json:"workflow_ref_status"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("approval response JSON: %v", err)
			}
			if envelope.Code != tc.code {
				t.Fatalf("response code = %d, want %d: %s", envelope.Code, tc.code, recorder.Body.String())
			}
			if tc.code == 0 && (envelope.Data.WorkflowRef != "workflow-v1" || !envelope.Data.Approved || envelope.Data.Status != "approved") {
				t.Fatalf("safe approval data = %#v, want approved effective selector", envelope.Data)
			}
		})
	}

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
		wantCode   int
	}{
		{name: "confirm required", body: `{"confirm":false}`, wantStatus: http.StatusOK, wantCode: 101},
		{name: "invalid selector", body: `{"confirm":true,"workflow_tag":"../bad"}`, wantStatus: http.StatusBadRequest, wantCode: 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newWorkflowApprovalRouter(&model.User{IsAdmin: boolPointer(true)})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/admin/github_build_config/approve_workflow_ref", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("HTTP status = %d, want %d: %s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			var envelope struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("approval response JSON: %v", err)
			}
			if envelope.Code != tc.wantCode {
				t.Fatalf("response code = %d, want %d: %s", envelope.Code, tc.wantCode, recorder.Body.String())
			}
		})
	}
}

type githubApprovalRoundTripper func(*http.Request) (*http.Response, error)

func (f githubApprovalRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDispatchTestRequiresWorkflowRefApprovalBeforeProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(utils.SecretEncryptionKeyEnv, "workflow-dispatch-approval-test")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.GithubBuildConfig{}); err != nil {
		t.Fatalf("migrate config: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{
		IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "github_pat_test", PayloadKey: "payload-key",
	}).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}
	previousDB, previousServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{GithubBuildConfigService: &service.GithubBuildConfigService{}}
	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousServices
	})
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/admin/github_build_config/dispatch_test", strings.NewReader(`{"confirm":true}`))
	context.Request.Header.Set("Content-Type", "application/json")
	(&GithubBuildConfig{}).DispatchTest(context)
	if recorder.Code != http.StatusPreconditionFailed || strings.Contains(recorder.Body.String(), "owner/repo") {
		t.Fatalf("unapproved dispatch response status/body = %d/%s, want safe precondition failure", recorder.Code, recorder.Body.String())
	}
}

func newWorkflowApprovalRouter(user *model.User) *gin.Engine {
	router := gin.New()
	group := router.Group("/api/admin")
	if user != nil {
		group.Use(func(c *gin.Context) {
			c.Set("curUser", user)
			c.Next()
		})
	}
	approvalGroup := group.Group("/github_build_config").Use(middleware.AdminPrivilege())
	approvalGroup.POST("/approve_workflow_ref", (&GithubBuildConfig{}).ApproveWorkflowRef)
	return router
}

func TestGenerateKeyReturnsOneTimeCopyValueAndSafeConfigHidesIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(utils.SecretEncryptionKeyEnv, "generate-key-test")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.GithubBuildConfig{}); err != nil {
		t.Fatalf("migrate config: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{GithubBuildConfigService: &service.GithubBuildConfigService{}}
	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousServices
		_ = sqlDB.Close()
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/admin/github_build_config/generate_key", nil)
	(&GithubBuildConfig{}).GenerateKey(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("generate key status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			PayloadKey string `json:"payload_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid generate key response: %v", err)
	}
	if payload.Data.PayloadKey == "" {
		t.Fatal("generate key response omitted the one-time payload key")
	}

	stored, err := service.AllService.GithubBuildConfigService.Get()
	if err != nil {
		t.Fatalf("reload generated config: %v", err)
	}
	encoded, err := json.Marshal(stored.Safe())
	if err != nil {
		t.Fatalf("marshal safe generated config: %v", err)
	}
	if strings.Contains(string(encoded), payload.Data.PayloadKey) || strings.Contains(string(encoded), `"payload_key"`) {
		t.Fatalf("safe config exposed one-time payload key: %s", encoded)
	}
}
