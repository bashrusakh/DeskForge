package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/utils"
)

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
