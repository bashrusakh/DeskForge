package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

func failCustomValidation(c *gin.Context, err error) {
	response.FailStatus(c, http.StatusBadRequest, 101, customValidationMessage(err))
}

func customValidationMessage(err error) string {
	const genericMessage = "custom build request is invalid"

	if err == nil {
		return genericMessage
	}
	var validationErr *service.ClientValidationError
	if errors.As(err, &validationErr) {
		if validationErr == nil || validationErr.Err == nil {
			return genericMessage
		}
		err = validationErr.Err
	}

	detail := err.Error()
	switch {
	case strings.Contains(detail, "server_ip is required"),
		strings.Contains(detail, "server_ip must not contain surrounding whitespace"):
		return "server endpoint is required"
	case strings.Contains(detail, "key is required"):
		return "public key is required"
	case strings.Contains(detail, "api_server is required"),
		strings.Contains(detail, "api_server must not contain surrounding whitespace"):
		return "API URL is required"
	case strings.Contains(detail, "relay_server is required"),
		strings.Contains(detail, "relay_server must not contain surrounding whitespace"):
		return "relay endpoint is required"
	case strings.Contains(detail, "app_name is required"):
		return "AppName is required"
	case strings.Contains(detail, "permanent_password is required"):
		return "permanent password is required"
	case strings.Contains(detail, "invalid version format"),
		strings.Contains(detail, "invalid display version"),
		strings.Contains(detail, "version has invalid or unsafe format"):
		return "invalid version format"
	case strings.Contains(detail, "required custom build field"):
		return "required custom build field is missing"
	case strings.Contains(detail, "not available in configured repository"),
		strings.Contains(detail, "does not match resolved catalog identity"),
		(strings.Contains(detail, "source tag") && strings.Contains(detail, "does not match version")):
		return "selected version is not available in the catalog"
	case strings.Contains(detail, "platform has unsupported value"):
		return "unsupported platform"
	case strings.Contains(detail, "build_ref is system-derived"):
		return "build_ref is system-derived and cannot be supplied"
	default:
		return genericMessage
	}
}

func failCustomServiceError(c *gin.Context, err error) bool {
	if service.IsSecretEncryptionConfigurationError(err) {
		response.FailStatus(c, http.StatusServiceUnavailable, 101, "secret encryption is not configured")
		return true
	}
	if !service.IsClientValidationError(err) {
		return false
	}
	failCustomValidation(c, err)
	return true
}

// validateCustomPlatform is the request-time persistence gate for custom
// builds and presets. The service owns the closed platform domain; callers
// must run this gate before any Create, Update, or upsert operation.
func validateCustomPlatform(c *gin.Context, value string) bool {
	if err := service.ValidateCustomPlatform(value); err != nil {
		failCustomValidation(c, err)
		return false
	}
	return true
}
