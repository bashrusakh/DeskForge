package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"rustdesk-server/api/utils"
)

type CustomPreset struct{}

func failCustomPresetRead(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if global.Logger != nil {
		global.Logger.Errorf("custom preset read failed: %v", err)
	}
	response.FailStatus(c, http.StatusInternalServerError, 101, "custom preset is unavailable")
}

// getOwnedPreset loads a preset by id and verifies it belongs to the current user.
// On any failure it writes the error response and returns ok=false, so callers
// just `return` instead of repeating the not-found / ownership checks.
func (p *CustomPreset) getOwnedPreset(c *gin.Context, id uint) (*model.CustomPreset, bool) {
	ex, err := service.AllService.CustomPresetService.Info(id)
	if err != nil {
		failCustomPresetRead(c, err)
		return nil, false
	}
	u := service.AllService.UserService.CurUser(c)
	if u == nil || ex.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return nil, false
	}
	return ex, true
}

// List returns the current user's redacted custom presets.
// @Tags CustomPreset
// @Summary List custom presets
// @Description Administrators with AdminPrivilege receive a paginated redacted custom-preset list.
// @Produce json
// @Param page query int false "Page number"
// @Param page_size query int false "Number of presets per page"
// @Success 200 {object} response.Response{data=model.CustomPresetSafeList}
// @Failure 500 {object} response.Response
// @Router /admin/custom_preset/list [get]
// @Security token
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
	res, err := service.AllService.CustomPresetService.ListByUser(uint(q.Page), uint(q.PageSize), u.Id)
	if err != nil {
		failCustomPresetRead(c, err)
		return
	}
	response.Success(c, res.Safe())
}

// Detail returns one redacted custom preset owned by the current user.
// @Tags CustomPreset
// @Summary Get a custom preset
// @Description Administrators with AdminPrivilege may read their own redacted custom preset.
// @Produce json
// @Param id path int true "Custom preset ID"
// @Success 200 {object} response.Response{data=model.CustomPresetSafe}
// @Failure 500 {object} response.Response
// @Router /admin/custom_preset/detail/{id} [get]
// @Security token
func (p *CustomPreset) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	preset, ok := p.getOwnedPreset(c, uint(iid))
	if !ok {
		return
	}
	response.Success(c, preset.Safe())
}

// Create stores a redacted custom preset for the current user.
// @Tags CustomPreset
// @Summary Create a custom preset
// @Description Administrators with AdminPrivilege may save a custom preset; provider-derived build references are resolved server-side.
// @Accept json
// @Produce json
// @Param body body admin.CustomPresetForm true "Custom-preset payload"
// @Success 200 {object} response.Response{data=model.CustomPresetSafe}
// @Failure 500 {object} response.Response
// @Router /admin/custom_preset/create [post]
// @Security token
func (p *CustomPreset) Create(c *gin.Context) {
	f := &admin.CustomPresetForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.BuildRef != "" {
		failCustomValidation(c, fmt.Errorf("build_ref is system-derived and cannot be supplied"))
		return
	}
	if !validateCustomPlatform(c, f.Platform) {
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
	if err := utils.RequireSecretEncryptionForCustomBuilderJSON(preset.CustomJson); err != nil {
		if failCustomServiceError(c, err) {
			return
		}
		response.FailStatus(c, http.StatusServiceUnavailable, 101, "secret encryption is not configured")
		return
	}

	if err := service.AllService.CustomPresetService.Create(preset); err != nil {
		if failCustomServiceError(c, err) {
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, preset.Safe())
}

// Update changes a redacted custom preset owned by the current user.
// @Tags CustomPreset
// @Summary Update a custom preset
// @Description Administrators with AdminPrivilege may update their own custom preset; provider-derived build references are resolved server-side.
// @Accept json
// @Produce json
// @Param body body admin.CustomPresetForm true "Custom-preset update payload"
// @Success 200 {object} response.Response{data=model.CustomPresetSafe}
// @Failure 500 {object} response.Response
// @Router /admin/custom_preset/update [post]
// @Security token
func (p *CustomPreset) Update(c *gin.Context) {
	f := &admin.CustomPresetForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.BuildRef != "" {
		failCustomValidation(c, fmt.Errorf("build_ref is system-derived and cannot be supplied"))
		return
	}
	if !validateCustomPlatform(c, f.Platform) {
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
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
	if err := utils.RequireSecretEncryptionForCustomBuilderJSON(ex.CustomJson); err != nil {
		if failCustomServiceError(c, err) {
			return
		}
		response.FailStatus(c, http.StatusServiceUnavailable, 101, "secret encryption is not configured")
		return
	}

	if err := service.AllService.CustomPresetService.Update(ex); err != nil {
		if failCustomServiceError(c, err) {
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, ex.Safe())
}

// Delete removes a custom preset owned by the current user.
// @Tags CustomPreset
// @Summary Delete a custom preset
// @Description Administrators with AdminPrivilege may delete their own custom preset.
// @Accept json
// @Produce json
// @Param body body admin.CustomPresetForm true "Custom-preset identifier"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/custom_preset/delete [post]
// @Security token
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
