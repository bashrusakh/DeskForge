package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

func failCustomValidation(c *gin.Context, err error) {
	response.FailStatus(c, http.StatusBadRequest, 101, err.Error())
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
