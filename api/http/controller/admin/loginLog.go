package admin

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"gorm.io/gorm"
	"strconv"
)

type LoginLog struct {
}

// Detail 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param id path int true "ID"
// @Success 200 {object} response.Response{data=model.LoginLog}
// @Failure 500 {object} response.Response
// @Router /admin/login_log/detail/{id} [get]
// @Security token
func (ct *LoginLog) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	u := service.AllService.LoginLogService.InfoById(uint(iid))
	if u.Id > 0 {
		response.Success(c, u)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
	return
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
// @Router /admin/login_log/list [get]
// @Security token
func (ct *LoginLog) List(c *gin.Context) {
	query := &admin.LoginLogQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.LoginLogService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		if query.UserId > 0 {
			tx.Where("user_id = ?", query.UserId)
		}
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
// @Router /admin/login_log/delete [post]
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
	if l.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.LoginLogService.Delete(l)
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
// @Router /admin/login_log/batchDelete [post]
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

	err := service.AllService.LoginLogService.BatchDelete(f.Ids)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
	return
}
