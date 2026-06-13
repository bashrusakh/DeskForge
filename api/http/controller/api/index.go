package api

import (
	"github.com/gin-gonic/gin"
	requstform "rustdesk-server/api/http/request/api"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"net/http"
	"time"
)

type Index struct {
}

// Index 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router / [get]
func (i *Index) Index(c *gin.Context) {
	response.Success(
		c,
		"Hello Gwen",
	)
}

// Heartbeat 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Success 200 {object} nil
// @Failure 500 {object} response.Response
// @Router /heartbeat [post]
func (i *Index) Heartbeat(c *gin.Context) {
	info := &requstform.PeerInfoInHeartbeat{}
	err := c.ShouldBindJSON(info)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if info.Uuid == "" {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	peer := service.AllService.PeerService.FindById(info.Id)
	if peer == nil || peer.RowId == 0 {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	//40s
	if time.Now().Unix()-peer.LastOnlineTime >= 30 {
		upp := &model.Peer{RowId: peer.RowId, LastOnlineTime: time.Now().Unix(), LastOnlineIp: c.ClientIP()}
		service.AllService.PeerService.Update(upp)
	}
	c.JSON(http.StatusOK, gin.H{})
}

// Version 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /version [get]
func (i *Index) Version(c *gin.Context) {
	//resources/version
	v := service.AllService.AppService.GetAppVersion()
	response.Success(
		c,
		v,
	)
}
