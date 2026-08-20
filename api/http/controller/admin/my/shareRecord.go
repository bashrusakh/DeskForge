package my

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

type ShareRecord struct {
}

// List
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param page query int false "Page number for personal share records"
// @Param page_size query int false "Number of personal share records per page"
// @Success 200 {object} response.Response{data=model.ShareRecordSafeList} "Redacted personal share-record list envelope"
// @Failure 500 {object} response.Response
// @Router /admin/my/share_record/list [get]
// @Security token
func (sr *ShareRecord) List(c *gin.Context) {
	query := &admin.PageQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	res := service.AllService.ShareRecordService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		tx.Where("user_id = ?", u.Id)
	})
	response.Success(c, res.Safe())
}

// Delete
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param body body admin.ShareRecordForm true "Personal share record deletion form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/share_record/delete [post]
// @Security token
func (sr *ShareRecord) Delete(c *gin.Context) {
	f := &admin.ShareRecordForm{}
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
	u := service.AllService.UserService.CurUser(c)
	i := service.AllService.ShareRecordService.InfoById(f.Id)
	if i.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if i.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.ShareRecordService.Delete(i)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
}

// BatchDelete
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param body body admin.PeerShareRecordBatchDeleteForm true "id"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/share_record/batchDelete [post]
// @Security token
func (sr *ShareRecord) BatchDelete(c *gin.Context) {
	f := &admin.PeerShareRecordBatchDeleteForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(f.Ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	u := service.AllService.UserService.CurUser(c)
	var l int64
	l = int64(len(f.Ids))
	res := service.AllService.ShareRecordService.List(1, uint(l), func(tx *gorm.DB) {
		tx.Where("user_id = ?", u.Id)
		tx.Where("id in ?", f.Ids)
	})
	if res.Total != l {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.ShareRecordService.BatchDelete(f.Ids)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}
