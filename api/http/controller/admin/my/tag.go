package my

import (
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
	"gorm.io/gorm"
)

type Tag struct{}

// List 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param page_size query int false ""
// @Param is_my query int false ""
// @Param user_id query int false "id"
// @Success 200 {object} response.Response{data=model.TagList}
// @Failure 500 {object} response.Response
// @Router /admin/my/tag/list [get]
// @Security token
func (ct *Tag) List(c *gin.Context) {
	query := &admin.TagQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	query.UserId = int(u.Id)
	res := service.AllService.TagService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		tx.Preload("Collection", func(txc *gorm.DB) *gorm.DB {
			return txc.Select("id,name")
		})
		tx.Where("user_id = ?", query.UserId)
		if query.CollectionId != nil && *query.CollectionId >= 0 {
			tx.Where("collection_id = ?", query.CollectionId)
		}
	})
	response.Success(c, res)
}

// Create 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.TagForm true ""
// @Success 200 {object} response.Response{data=model.Tag}
// @Failure 500 {object} response.Response
// @Router /admin/my/tag/create [post]
// @Security token
func (ct *Tag) Create(c *gin.Context) {
	f := &admin.TagForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	t := f.ToTag()
	u := service.AllService.UserService.CurUser(c)
	t.UserId = u.Id
	err := service.AllService.TagService.Create(t)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// Update 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.TagForm true ""
// @Success 200 {object} response.Response{data=model.Tag}
// @Failure 500 {object} response.Response
// @Router /admin/my/tag/update [post]
// @Security token
func (ct *Tag) Update(c *gin.Context) {
	f := &admin.TagForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	u := service.AllService.UserService.CurUser(c)
	if f.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	ex := service.AllService.TagService.InfoById(f.Id)
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if ex.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}

	t := f.ToTag()
	if t.CollectionId > 0 && !service.AllService.AddressBookService.CheckCollectionOwner(t.UserId, t.CollectionId) {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	err := service.AllService.TagService.Update(t)
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
// @Param body body admin.TagForm true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/tag/delete [post]
// @Security token
func (ct *Tag) Delete(c *gin.Context) {
	f := &admin.TagForm{}
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
	ex := service.AllService.TagService.InfoById(f.Id)
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	u := service.AllService.UserService.CurUser(c)
	if ex.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	err := service.AllService.TagService.Delete(ex)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, err.Error())
	return
}
