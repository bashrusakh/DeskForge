package middleware

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

func JwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {

		token := c.GetHeader("api-token")
		if token == "" {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}
		uid, err := global.Jwt.ParseToken(token)
		if err != nil {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}
		if uid == 0 {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}

		user := service.AllService.UserService.InfoById(uid)
		//user := &model.User{
		//	Id:       uid,
		//	Username: "",
		//}
		if user.Id == 0 {
			response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
			c.Abort()
			return
		}
		if !service.AllService.UserService.CheckUserEnable(user) {
			response.Fail(c, 101, response.TranslateMsg(c, "Banned"))
			c.Abort()
			return
		}
		c.Set("curUser", user)

		c.Next()
	}
}
