package middleware

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"github.com/sirupsen/logrus"
)

// Logger 
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		global.Logger.WithFields(
			logrus.Fields{
				"uri":    c.Request.URL,
				"ip":     c.ClientIP(),
				"method": c.Request.Method,
			}).Debug("Request")
		c.Next()
	}
}
