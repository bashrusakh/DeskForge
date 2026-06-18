package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

type CustomPreset struct{}

// getOwnedPreset loads a preset by id and verifies it belongs to the current user.
// On any failure it writes the error response and returns ok=false, so callers
// just `return` instead of repeating the not-found / ownership checks.
func (p *CustomPreset) getOwnedPreset(c *gin.Context, id uint) (*model.CustomPreset, bool) {
	ex := service.AllService.CustomPresetService.Info(id)
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return nil, false
	}
	u := service.AllService.UserService.CurUser(c)
	if u == nil || ex.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return nil, false
	}
	return ex, true
}

func (p *CustomPreset) List(c *gin.Context) {
	q := &admin.CustomPresetQuery{}
	if err := c.ShouldBindQuery(q); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	if u == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "Unauthorized"))
		return
	}
	res := service.AllService.CustomPresetService.ListByUser(uint(q.Page), uint(q.PageSize), u.Id)
	response.Success(c, res)
}

func (p *CustomPreset) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	preset, ok := p.getOwnedPreset(c, uint(iid))
	if !ok {
		return
	}
	response.Success(c, preset)
}

func (p *CustomPreset) Create(c *gin.Context) {
	f := &admin.CustomPresetForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}

	user := service.AllService.UserService.CurUser(c)
	if user == nil {
		response.Fail(c, 101, response.TranslateMsg(c, "Unauthorized"))
		return
	}
	preset := f.ToCustomPreset()
	preset.UserId = user.Id

	if err := service.AllService.CustomPresetService.Create(preset); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, preset)
}

func (p *CustomPreset) Update(c *gin.Context) {
	f := &admin.CustomPresetForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	ex, ok := p.getOwnedPreset(c, f.Id)
	if !ok {
		return
	}
	ex.Name = f.Name
	ex.Platform = f.Platform
	ex.Version = f.Version
	ex.AppName = f.AppName
	ex.CustomJson = f.CustomJson

	if err := service.AllService.CustomPresetService.Update(ex); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, ex)
}

func (p *CustomPreset) Delete(c *gin.Context) {
	f := &admin.CustomPresetForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	ex, ok := p.getOwnedPreset(c, f.Id)
	if !ok {
		return
	}
	if err := service.AllService.CustomPresetService.Delete(ex); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}
