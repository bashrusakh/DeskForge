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

type UserToken struct {
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
// @Success 200 {object} response.Response{data=model.UserTokenList}
// @Failure 500 {object} response.Response
// @Router /admin/user_token/list [get]
// @Security token
func (ct *UserToken) List(c *gin.Context) {
	query := &admin.LoginTokenQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.UserService.TokenList(query.Page, query.PageSize, func(tx *gorm.DB) {
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
// @Param body body model.UserToken true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user_token/delete [post]
// @Security token
func (ct *UserToken) Delete(c *gin.Context) {
	f := &model.UserToken{}
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
	l := service.AllService.UserService.TokenInfoById(f.Id)
	u := service.AllService.UserService.CurUser(c)
	if !service.AllService.UserService.IsAdmin(u) && l.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	if l.Id > 0 {
		err := service.AllService.UserService.DeleteToken(l)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, err.Error())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// BatchDelete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.UserTokenBatchDeleteForm true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user_token/batchDelete [post]
// @Security token
func (ct *UserToken) BatchDelete(c *gin.Context) {
	f := &admin.UserTokenBatchDeleteForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	ids := f.Ids
	if len(ids) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	err := service.AllService.UserService.BatchDeleteUserToken(ids)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
}
