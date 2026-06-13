package admin

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
	"strconv"
)

type DeviceGroup struct {
}

// Detail 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param id path int true "ID"
// @Success 200 {object} response.Response{data=model.Group}
// @Failure 500 {object} response.Response
// @Router /admin/device_group/detail/{id} [get]
// @Security token
func (ct *DeviceGroup) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	u := service.AllService.GroupService.DeviceGroupInfoById(uint(iid))
	if u.Id > 0 {
		response.Success(c, u)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
	return
}

// Create 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.DeviceGroupForm true ""
// @Success 200 {object} response.Response{data=model.DeviceGroup}
// @Failure 500 {object} response.Response
// @Router /admin/device_group/create [post]
// @Security token
func (ct *DeviceGroup) Create(c *gin.Context) {
	f := &admin.DeviceGroupForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToDeviceGroup()
	err := service.AllService.GroupService.DeviceGroupCreate(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// List 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param page_size query int false ""
// @Success 200 {object} response.Response{data=model.GroupList}
// @Failure 500 {object} response.Response
// @Router /admin/device_group/list [get]
// @Security token
func (ct *DeviceGroup) List(c *gin.Context) {
	query := &admin.PageQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.GroupService.DeviceGroupList(query.Page, query.PageSize, nil)
	response.Success(c, res)
}

// Update 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.DeviceGroupForm true ""
// @Success 200 {object} response.Response{data=model.Group}
// @Failure 500 {object} response.Response
// @Router /admin/device_group/update [post]
// @Security token
func (ct *DeviceGroup) Update(c *gin.Context) {
	f := &admin.DeviceGroupForm{}
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
	u := f.ToDeviceGroup()
	err := service.AllService.GroupService.DeviceGroupUpdate(u)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// Delete 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.DeviceGroupForm true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/device_group/delete [post]
// @Security token
func (ct *DeviceGroup) Delete(c *gin.Context) {
	f := &admin.DeviceGroupForm{}
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
	u := service.AllService.GroupService.DeviceGroupInfoById(f.Id)
	if u.Id > 0 {
		err := service.AllService.GroupService.DeviceGroupDelete(u)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}
