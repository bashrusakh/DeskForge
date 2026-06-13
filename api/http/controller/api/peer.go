package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	requstform "rustdesk-server/api/http/request/api"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
	"net/http"
)

type Peer struct {
}

// SysInfo
// @Tags System
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body requstform.PeerForm true ""
// @Success 200 {string} string "SYSINFO_UPDATED,ID_NOT_FOUND"
// @Failure 500 {object} response.ErrorResponse
// @Router /sysinfo [post]
func (p *Peer) SysInfo(c *gin.Context) {
	f := &requstform.PeerForm{}
	err := c.ShouldBindBodyWith(f, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	fpe := f.ToPeer()
	pe := service.AllService.PeerService.FindById(f.Id)
	if pe.RowId == 0 {
		pe = f.ToPeer()
		pe.UserId = service.AllService.UserService.FindLatestUserIdFromLoginLogByUuid(pe.Uuid, pe.Id)
		err = service.AllService.PeerService.Create(pe)
		if err != nil {
			response.Error(c, response.TranslateMsg(c, "OperationFailed")+err.Error())
			return
		}
	} else {
		if pe.UserId == 0 {
			pe.UserId = service.AllService.UserService.FindLatestUserIdFromLoginLogByUuid(pe.Uuid, pe.Id)
		}
		fpe.RowId = pe.RowId
		fpe.UserId = pe.UserId
		err = service.AllService.PeerService.Update(fpe)
		if err != nil {
			response.Error(c, response.TranslateMsg(c, "OperationFailed")+err.Error())
			return
		}
	}
	//SYSINFO_UPDATED 
	//ID_NOT_FOUND 

	c.String(http.StatusOK, "SYSINFO_UPDATED")
}

// SysInfoVer
// @Tags System
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Success 200 {string} string ""
// @Failure 500 {object} response.ErrorResponse
// @Router /sysinfo_ver [post]
func (p *Peer) SysInfoVer(c *gin.Context) {
	//resources/version
	v := service.AllService.AppService.GetAppVersion()
	// пјЊclient
	v = fmt.Sprintf("%s\n%s", v, service.AllService.AppService.GetStartTime())
	c.String(http.StatusOK, v)
}
