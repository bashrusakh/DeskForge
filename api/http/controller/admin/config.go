package admin

import (
	"github.com/gin-gonic/gin"
	"os"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"strings"
)

type Config struct {
}

// ServerConfig RUSTDESK
// @Tags ADMIN
// @Summary RUSTDESK
// @Description ,webclientapi-server
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/server [get]
// @Security token
func (co *Config) ServerConfig(c *gin.Context) {
	cf := &response.ServerConfigResponse{
		IdServer:    global.Config.Rustdesk.IdServer,
		Key:         global.Config.Rustdesk.Key,
		RelayServer: global.Config.Rustdesk.RelayServer,
		ApiServer:   global.Config.Rustdesk.ApiServer,
	}
	response.Success(c, cf)
}

// AppConfig APP
// @Tags ADMIN
// @Summary APP
// @Description APP
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/app [get]
// @Security token
func (co *Config) AppConfig(c *gin.Context) {
	response.Success(c, &gin.H{
		"web_client": global.Config.App.WebClient,
	})
}

// AllConfig
// @Tags ADMIN
// @Summary Get all admin configuration
// @Description Admin-only configuration used by the admin panel.
// @Produce json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/all [get]
// @Security token
func (co *Config) AllConfig(c *gin.Context) {
	response.Success(c, &gin.H{
		"id_server":    global.Config.Rustdesk.IdServer,
		"relay_server": global.Config.Rustdesk.RelayServer,
		"api_server":   global.Config.Rustdesk.ApiServer,
		"key":          global.Config.Rustdesk.Key,
		"ws_host":      global.Config.Rustdesk.WsHost,
		"web_client":   global.Config.App.WebClient,
		"register":     global.Config.App.Register,
		"show_swagger": global.Config.App.ShowSwagger,
		"personal":     global.Config.Rustdesk.Personal,
		"token_expire": global.Config.App.TokenExpire.String(),
		"title":        global.Config.Admin.Title,
		"lang":         global.Config.Lang,
	})
}

// AdminConfig returns public admin branding, with optional authenticated greeting data.
// @Tags ADMIN
// @Summary Get public admin configuration
// @Description Public pre-authentication route returning the admin title and, when a valid api-token is supplied, the personalized greeting.
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/admin [get]
func (co *Config) AdminConfig(c *gin.Context) {

	u := &model.User{}
	token := c.GetHeader("api-token")
	if token != "" {
		u, _ = service.AllService.UserService.InfoByAccessToken(token)
		if !service.AllService.UserService.CheckUserEnable(u) {
			u.Id = 0
		}
	}

	if u.Id == 0 {
		response.Success(c, &gin.H{
			"title": global.Config.Admin.Title,
		})
		return
	}

	hello := global.Config.Admin.Hello
	if hello == "" {
		helloFile := global.Config.Admin.HelloFile
		if helloFile != "" {
			b, err := os.ReadFile(helloFile)
			if err == nil && len(b) > 0 {
				hello = string(b)
			}
		}
	}

	//replace {{username}} to username
	hello = strings.Replace(hello, "{{username}}", u.Username, -1)
	response.Success(c, &gin.H{
		"title": global.Config.Admin.Title,
		"hello": hello,
	})
}
