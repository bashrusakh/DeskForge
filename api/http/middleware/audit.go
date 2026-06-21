package middleware

import (
	"bytes"
	"io"

	"github.com/gin-gonic/gin"

	"rustdesk-server/api/global"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

const auditParamsMaxLen = 2000

// ServerCmdAudit — middleware журналирования админских server-команд
// (BUGS.md AU-S-001). Ставится ПОСЛЕ авторизации (curUser уже в контексте) на
// мутирующие маршруты /rustdesk/*. Пишет запись после обработки: кто, метод,
// путь, тело запроса (усечённое), IP и итоговый HTTP-статус.
func ServerCmdAudit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Считываем тело и кладём обратно, чтобы хендлер тоже его прочитал.
		var bodyCopy []byte
		if c.Request.Body != nil {
			bodyCopy, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyCopy))
		}

		c.Next()

		params := string(bodyCopy)
		if len(params) > auditParamsMaxLen {
			params = params[:auditParamsMaxLen] + "...(truncated)"
		}

		var userId uint
		var username string
		if u := service.AllService.UserService.CurUser(c); u != nil {
			userId = u.Id
			username = u.Username
		}

		entry := &model.ServerCmdAudit{
			UserId:   userId,
			Username: username,
			Method:   c.Request.Method,
			Path:     c.FullPath(),
			Params:   params,
			Ip:       c.ClientIP(),
			Status:   c.Writer.Status(),
		}
		if err := global.DB.Create(entry).Error; err != nil {
			global.Logger.Warnf("ServerCmdAudit: failed to record %s %s: %v",
				entry.Method, entry.Path, err)
		}
	}
}
