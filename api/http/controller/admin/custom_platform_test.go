package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/service"
	"rustdesk-server/api/utils"
)

func TestCustomValidationResponsesAreSafeAndActionable(t *testing.T) {
	const maliciousValue = "https://attacker.example/collect?token=secret-value"
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unknown detail uses generic message",
			err:  &service.ClientValidationError{Err: errors.New(`server has invalid endpoint "` + maliciousValue + `"`)},
			want: "custom build request is invalid",
		},
		{
			name: "invalid version remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("invalid version format: 1.4.8; " + maliciousValue)},
			want: "invalid version format",
		},
		{
			name: "unavailable catalog version remains actionable",
			err:  &service.ClientValidationError{Err: errors.New(`version "1.4.8" is not available in configured repository: ` + maliciousValue)},
			want: "selected version is not available in the catalog",
		},
		{
			name: "catalog identity mismatch remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("build version does not match resolved catalog identity")},
			want: "selected version is not available in the catalog",
		},
		{
			name: "unsupported platform remains actionable",
			err:  &service.ClientValidationError{Err: errors.New(`platform has unsupported value "plan9"`)},
			want: "unsupported platform",
		},
		{
			name: "missing server endpoint remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("server_ip is required for Windows builds")},
			want: "server endpoint is required",
		},
		{
			name: "whitespace server endpoint remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("server_ip must not contain surrounding whitespace")},
			want: "server endpoint is required",
		},
		{
			name: "missing public key remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("key is required for Windows builds")},
			want: "public key is required",
		},
		{
			name: "missing API URL remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("api_server is required for Windows builds")},
			want: "API URL is required",
		},
		{
			name: "whitespace API URL remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("api_server must not contain surrounding whitespace")},
			want: "API URL is required",
		},
		{
			name: "missing relay endpoint remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("relay_server is required for Windows builds")},
			want: "relay endpoint is required",
		},
		{
			name: "missing AppName remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("app_name is required")},
			want: "AppName is required",
		},
		{
			name: "missing permanent password remains actionable",
			err:  &service.ClientValidationError{Err: errors.New("permanent_password is required when hide_cm is true; value=secret-value")},
			want: "permanent password is required",
		},
		{
			name: "system-derived build ref remains actionable",
			err:  errors.New("build_ref is system-derived and cannot be supplied"),
			want: "build_ref is system-derived and cannot be supplied",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)

			failCustomValidation(context, test.err)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("validation status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			var payload struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("invalid validation response: %v", err)
			}
			if payload.Code != 101 {
				t.Fatalf("validation code = %d, want 101", payload.Code)
			}
			if payload.Message != test.want {
				t.Fatalf("validation message = %q, want %q", payload.Message, test.want)
			}
			if strings.Contains(recorder.Body.String(), maliciousValue) {
				t.Fatalf("validation response leaked malicious value: %s", recorder.Body.String())
			}
		})
	}
}

func TestFailCustomServiceErrorMapsClientValidationSafely(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	if !failCustomServiceError(context, &service.ClientValidationError{Err: errors.New("custom JSON contains https://attacker.example/private")}) {
		t.Fatal("failCustomServiceError() did not handle client validation error")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("validation status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if strings.Contains(recorder.Body.String(), "attacker.example/private") {
		t.Fatalf("client validation response leaked raw details: %s", recorder.Body.String())
	}
}

func TestCustomBuildCreateValidationResponsesAreSafe(t *testing.T) {
	previousValidator := global.Validator
	global.ApiInitValidator()
	t.Cleanup(func() { global.Validator = previousValidator })

	const maliciousValue = "https://attacker.example/raw?token=secret-value"
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "malformed JSON",
			body: `{"name":"DeskForge","platform":"windows","version":"1.2.3","app_name":"DeskForge","custom_json":"` + maliciousValue,
			want: "custom build request is invalid",
		},
		{
			name: "missing required field",
			body: `{"platform":"windows","version":"1.2.3","app_name":"DeskForge","custom_json":"` + maliciousValue + `"}`,
			want: "required custom build field is missing",
		},
		{
			name: "raw build reference",
			body: `{"name":"DeskForge","platform":"windows","version":"1.2.3","app_name":"DeskForge","build_ref":"` + maliciousValue + `"}`,
			want: "build_ref is system-derived and cannot be supplied",
		},
		{
			name: "unsafe version",
			body: `{"name":"DeskForge","platform":"windows","version":"1.2.3;` + maliciousValue + `","app_name":"DeskForge"}`,
			want: "invalid version format",
		},
		{
			name: "invalid custom JSON",
			body: `{"name":"DeskForge","platform":"windows","version":"1.2.3","app_name":"DeskForge","custom_json":"{\"server_ip\":\"` + maliciousValue + `"}`,
			want: "custom build request is invalid",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/custom_build/create", strings.NewReader(test.body))
			context.Request.Header.Set("Content-Type", "application/json")

			(&CustomBuild{}).Create(context)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("create validation status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			var payload struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("invalid create validation response: %v", err)
			}
			if payload.Code != 101 {
				t.Fatalf("create validation code = %d, want 101", payload.Code)
			}
			if payload.Message != test.want {
				t.Fatalf("create validation message = %q, want %q", payload.Message, test.want)
			}
			if strings.Contains(recorder.Body.String(), maliciousValue) {
				t.Fatalf("create validation response leaked raw value: %s", recorder.Body.String())
			}
		})
	}
}

func TestValidateCustomPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		platform   string
		wantOK     bool
		wantStatus int
	}{
		{name: "windows", platform: "windows", wantOK: true, wantStatus: http.StatusOK},
		{name: "linux", platform: "linux", wantOK: true, wantStatus: http.StatusOK},
		{name: "android", platform: "android", wantOK: true, wantStatus: http.StatusOK},
		{name: "macos", platform: "macos", wantStatus: http.StatusBadRequest},
		{name: "unknown", platform: "plan9", wantStatus: http.StatusBadRequest},
		{name: "empty", platform: "", wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)

			if got := validateCustomPlatform(context, test.platform); got != test.wantOK {
				t.Errorf("validateCustomPlatform(%q) = %v, want %v", test.platform, got, test.wantOK)
			}
			if recorder.Code != test.wantStatus {
				t.Errorf("validateCustomPlatform(%q) status = %d, want %d", test.platform, recorder.Code, test.wantStatus)
			}
			if !test.wantOK {
				var payload struct {
					Code int `json:"code"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
					t.Fatalf("invalid error response: %v", err)
				}
				if payload.Code != 101 {
					t.Errorf("error response code = %d, want 101", payload.Code)
				}
			}
		})
	}
}

func TestSecretPersistenceErrorUsesServiceUnavailableWithoutSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	secretValue := "must-not-appear"
	if !failCustomServiceError(context, &utils.SecretEncryptionKeyError{}) {
		t.Fatal("failCustomServiceError() did not handle secret configuration error")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("error status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(recorder.Body.String(), secretValue) {
		t.Fatalf("secret value leaked in error response: %s", recorder.Body.String())
	}
}
