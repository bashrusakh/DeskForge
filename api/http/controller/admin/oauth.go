package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	adminReq "rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

type Oauth struct {
}

// Info
func (o *Oauth) Info(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	v := service.AllService.OauthService.GetOauthCache(code)
	if v == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	response.Success(c, v)
}

func (o *Oauth) ToBind(c *gin.Context) {
	f := &adminReq.BindOauthForm{}
	err := c.ShouldBindJSON(f)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)

	utr := service.AllService.UserService.UserThirdInfo(u.Id, f.Op)
	if utr.Id > 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "OauthHasBindOtherUser"))
		return
	}

	err, state, verifier, nonce, url := service.AllService.OauthService.BeginAuth(f.Op)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, err.Error()))
		return
	}

	service.AllService.OauthService.SetOauthCache(state, &service.OauthCacheItem{
		Action:   service.OauthActionTypeBind,
		Op:       f.Op,
		UserId:   u.Id,
		Verifier: verifier,
		Nonce:    nonce,
	}, 5*60)

	response.Success(c, gin.H{
		"code": state,
		"url":  url,
	})
}

// Confirm 
func (o *Oauth) Confirm(c *gin.Context) {
	j := &adminReq.OauthConfirmForm{}
	err := c.ShouldBindJSON(j)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if j.Code == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	v := service.AllService.OauthService.GetOauthCache(j.Code)
	if v == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OauthExpired"))
		return
	}
	u := service.AllService.UserService.CurUser(c)
	v.UserId = u.Id
	service.AllService.OauthService.SetOauthCache(j.Code, v, 0)
	response.Success(c, v)
}

func (o *Oauth) BindConfirm(c *gin.Context) {
	j := &adminReq.OauthConfirmForm{}
	err := c.ShouldBindJSON(j)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if j.Code == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	oauthService := service.AllService.OauthService
	oauthCache := oauthService.GetOauthCache(j.Code)
	if oauthCache == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OauthExpired"))
		return
	}
	oauthUser := oauthCache.ToOauthUser()
	user := service.AllService.UserService.CurUser(c)
	err = oauthService.BindOauthUser(user.Id, oauthUser, oauthCache.Op)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "BindFail"))
		return
	}

	oauthCache.UserId = user.Id
	oauthService.SetOauthCache(j.Code, oauthCache, 0)
	response.Success(c, oauthCache)
}

func (o *Oauth) Unbind(c *gin.Context) {
	f := &adminReq.UnBindOauthForm{}
	err := c.ShouldBindJSON(f)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	utr := service.AllService.UserService.UserThirdInfo(u.Id, f.Op)
	if utr.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err = service.AllService.OauthService.UnBindOauthUser(u.Id, f.Op)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// Detail Oauth
// @Tags Oauth
// @Summary Oauth
// @Description Oauth
// @Accept  json
// @Produce  json
// @Param id path int true "ID"
// @Success 200 {object} response.Response{data=model.Oauth}
// @Failure 500 {object} response.Response
// @Router /admin/oauth/detail/{id} [get]
// @Security token
func (o *Oauth) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	u := service.AllService.OauthService.InfoById(uint(iid))
	if u.Id > 0 {
		response.Success(c, u)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
	return
}

// Create Oauth
// @Tags Oauth
// @Summary Oauth
// @Description Oauth
// @Accept  json
// @Produce  json
// @Param body body admin.OauthForm true "Oauth"
// @Success 200 {object} response.Response{data=model.Oauth}
// @Failure 500 {object} response.Response
// @Router /admin/oauth/create [post]
// @Security token
func (o *Oauth) Create(c *gin.Context) {
	f := &admin.OauthForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToOauth()
	err := u.FormatOauthInfo()
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	ex := service.AllService.OauthService.InfoByOp(u.Op)
	if ex.Id > 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemExists"))
		return
	}
	err = service.AllService.OauthService.Create(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// List 
// @Tags Oauth
// @Summary Oauth
// @Description Oauth
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param page_size query int false ""
// @Success 200 {object} response.Response{data=model.OauthList}
// @Failure 500 {object} response.Response
// @Router /admin/oauth/list [get]
// @Security token
func (o *Oauth) List(c *gin.Context) {
	query := &admin.PageQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.OauthService.List(query.Page, query.PageSize, nil)
	response.Success(c, res)
}

// Update 
// @Tags Oauth
// @Summary Oauth
// @Description Oauth
// @Accept  json
// @Produce  json
// @Param body body admin.OauthForm true "Oauth"
// @Success 200 {object} response.Response{data=model.OauthList}
// @Failure 500 {object} response.Response
// @Router /admin/oauth/update [post]
// @Security token
func (o *Oauth) Update(c *gin.Context) {
	f := &admin.OauthForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToOauth()
	err := service.AllService.OauthService.Update(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 
// @Tags Oauth
// @Summary Oauth
// @Description Oauth
// @Accept  json
// @Produce  json
// @Param body body admin.OauthForm true "Oauth"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/oauth/delete [post]
// @Security token
func (o *Oauth) Delete(c *gin.Context) {
	f := &admin.OauthForm{}
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
	u := service.AllService.OauthService.InfoById(f.Id)
	if u.Id > 0 {
		err := service.AllService.OauthService.Delete(u)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, err.Error())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}
