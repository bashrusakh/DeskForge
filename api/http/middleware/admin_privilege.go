package middleware

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

// AdminPrivilege ...
func AdminPrivilege() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := service.AllService.UserService.CurUser(c)

		if !service.AllService.UserService.IsAdmin(u) {
			response.Fail(c, 403, response.TranslateMsg(c, "NoAccess"))
			c.Abort()
			return
		}

		c.Next()
	}
}
