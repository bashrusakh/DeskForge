package admin

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"os"
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

// AdminConfig ADMIN
// @Tags ADMIN
// @Summary ADMIN
// @Description ADMIN
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/config/admin [get]
// @Security token
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
