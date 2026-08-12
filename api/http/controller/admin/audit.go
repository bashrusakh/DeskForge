package admin

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"gorm.io/gorm"
)

type Audit struct {
}

// ConnList 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false "Page number for audit connection records"
// @Param page_size query int false "Number of audit connection records per page"
// @Param peer_id query int false "Peer ID filter for audit connections"
// @Param from_peer query int false "Source peer ID filter for audit connections"
// @Success 200 {object} response.Response{data=model.AuditConnList}
// @Failure 500 {object} response.Response
// @Router /admin/audit_conn/list [get]
// @Security token
func (a *Audit) ConnList(c *gin.Context) {
	query := &admin.AuditQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.AuditService.AuditConnList(query.Page, query.PageSize, func(tx *gorm.DB) {
		if query.PeerId != "" {
			tx.Where("peer_id like ?", "%"+query.PeerId+"%")
		}
		if query.FromPeer != "" {
			tx.Where("from_peer like ?", "%"+query.FromPeer+"%")
		}
		tx.Order("id desc")
	})
	response.Success(c, res)
}

// ConnDelete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body model.AuditConn true "Audit connection deletion form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_conn/delete [post]
// @Security token
func (a *Audit) ConnDelete(c *gin.Context) {
	f := &model.AuditConn{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.Id
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	l := service.AllService.AuditService.ConnInfoById(f.Id)
	if l.Id > 0 {
		err := service.AllService.AuditService.DeleteAuditConn(l)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, err.Error())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// BatchConnDelete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.AuditConnLogIds true "Audit connection log ID batch deletion payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_conn/batchDelete [post]
// @Security token
func (a *Audit) BatchConnDelete(c *gin.Context) {
	f := &admin.AuditConnLogIds{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(f.Ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	err := service.AllService.AuditService.BatchDeleteAuditConn(f.Ids)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
	return
}

// FileList 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false "Page number for audit file records"
// @Param page_size query int false "Number of audit file records per page"
// @Param peer_id query int false "Peer ID filter for audit file records"
// @Param from_peer query int false "Source peer ID filter for audit file records"
// @Success 200 {object} response.Response{data=model.AuditFileList}
// @Failure 500 {object} response.Response
// @Router /admin/audit_file/list [get]
// @Security token
func (a *Audit) FileList(c *gin.Context) {
	query := &admin.AuditQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.AuditService.AuditFileList(query.Page, query.PageSize, func(tx *gorm.DB) {
		if query.PeerId != "" {
			tx.Where("peer_id like ?", "%"+query.PeerId+"%")
		}
		if query.FromPeer != "" {
			tx.Where("from_peer like ?", "%"+query.FromPeer+"%")
		}
		tx.Order("id desc")
	})
	response.Success(c, res)
}

// FileDelete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body model.AuditFile true "Audit file deletion form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_file/delete [post]
// @Security token
func (a *Audit) FileDelete(c *gin.Context) {
	f := &model.AuditFile{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.Id
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	l := service.AllService.AuditService.FileInfoById(f.Id)
	if l.Id > 0 {
		err := service.AllService.AuditService.DeleteAuditFile(l)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, err.Error())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// BatchFileDelete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.AuditFileLogIds true "Audit file log ID batch deletion payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/audit_file/batchDelete [post]
// @Security token
func (a *Audit) BatchFileDelete(c *gin.Context) {
	f := &admin.AuditFileLogIds{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(f.Ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	err := service.AllService.AuditService.BatchDeleteAuditFile(f.Ids)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
	return
}
