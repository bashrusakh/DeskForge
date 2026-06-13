package middleware

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/service"
)

func RustAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		//fmt.Println(c.Request.URL, c.Request.Header)
		//HTTP_AUTHORIZATION
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}
		if len(token) <= 7 {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}
		//tokenпјЊBearer {token}

		token = token[7:]

		//token

		//jwt key
		if len(global.Jwt.Key) > 0 {
			uid, _ := service.AllService.UserService.VerifyJWT(token)
			if uid == 0 {
				c.JSON(401, gin.H{
					"error": "Unauthorized",
				})
				c.Abort()
				return
			}
		}

		user, ut := service.AllService.UserService.InfoByAccessToken(token)
		if user.Id == 0 {
			c.JSON(401, gin.H{
				"error": "Unauthorized",
			})
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

		service.AllService.UserService.AutoRefreshAccessToken(ut)

		c.Next()
	}
}
