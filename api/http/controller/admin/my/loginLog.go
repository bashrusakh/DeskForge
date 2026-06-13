package my

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"gorm.io/gorm"
)

type LoginLog struct {
}

// List 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param page_size query int false ""
// @Param user_id query int false "ID"
// @Success 200 {object} response.Response{data=model.LoginLogList}
// @Failure 500 {object} response.Response
// @Router /admin/my/login_log/list [get]
// @Security token
func (ct *LoginLog) List(c *gin.Context) {
	query := &admin.LoginLogQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	res := service.AllService.LoginLogService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		tx.Where("user_id = ? and is_deleted = ?", u.Id, model.IsDeletedNo)
		tx.Order("id desc")
	})
	response.Success(c, res)
}

// Delete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body model.LoginLog true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/login_log/delete [post]
// @Security token
func (ct *LoginLog) Delete(c *gin.Context) {
	f := &model.LoginLog{}
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
	l := service.AllService.LoginLogService.InfoById(f.Id)
	if l.Id == 0 || l.IsDeleted == model.IsDeletedYes {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	u := service.AllService.UserService.CurUser(c)
	if l.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.LoginLogService.SoftDelete(l)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
}

// BatchDelete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.LoginLogIds true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/login_log/batchDelete [post]
// @Security token
func (ct *LoginLog) BatchDelete(c *gin.Context) {
	f := &admin.LoginLogIds{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if len(f.Ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	u := service.AllService.UserService.CurUser(c)
	err := service.AllService.LoginLogService.BatchSoftDelete(u.Id, f.Ids)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
	return
}
