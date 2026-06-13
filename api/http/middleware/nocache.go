package middleware

import "github.com/gin-gonic/gin"

// NoCache выставляет заголовки, запрещающие браузеру/прокси кешировать ответ.
// Применяется к эндпоинтам со статусом фоновых задач (custom_build, github_build),
// чтобы UI не показывал устаревший статус после F5.
func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}
