package middleware

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

// BackendUserAuth 
func BackendUserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {

		token := c.GetHeader("api-token")
		if token == "" {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}
		user, ut := service.AllService.UserService.InfoByAccessToken(token)
		if user.Id == 0 {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}

		if !service.AllService.UserService.CheckUserEnable(user) {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		c.Set("curUser", user)
		c.Set("token", token)
		//1,token
		service.AllService.UserService.AutoRefreshAccessToken(ut)

		c.Next()
	}
}
