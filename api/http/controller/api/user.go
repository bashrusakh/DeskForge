package api

import (
	"github.com/gin-gonic/gin"
	apiResp "rustdesk-server/api/http/response/api"
	"rustdesk-server/api/service"
	"net/http"
)

type User struct {
}

// currentUser 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Success 200 {object} apiResp.UserPayload
// @Failure 500 {object} response.Response
// @Router /currentUser [get]
// @Security token
//func (u *User) currentUser(c *gin.Context) {
//	user := service.AllService.UserService.CurUser(c)
//	up := (&apiResp.UserPayload{}).FromName(user)
//	c.JSON(http.StatusOK, up)
//}

// Info 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Success 200 {object} apiResp.UserPayload
// @Failure 500 {object} response.Response
// @Router /currentUser [get]
// @Security token
func (u *User) Info(c *gin.Context) {
	user := service.AllService.UserService.CurUser(c)
	up := (&apiResp.UserPayload{}).FromUser(user)
	c.JSON(http.StatusOK, up)
}
