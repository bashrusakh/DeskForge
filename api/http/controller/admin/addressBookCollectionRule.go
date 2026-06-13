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

type AddressBookCollectionRule struct {
}

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
// @Param collection_id query int false "id"
// @Success 200 {object} response.Response{data=model.AddressBookCollectionList}
// @Failure 500 {object} response.Response
// @Router /admin/address_book_collection_rule/list [get]
// @Security token
func (abcr *AddressBookCollectionRule) List(c *gin.Context) {
	query := &admin.AddressBookCollectionRuleQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	res := service.AllService.AddressBookService.ListRules(query.Page, query.PageSize, func(tx *gorm.DB) {
		if query.UserId > 0 {
			tx.Where("user_id = ?", query.UserId)
		}
		if query.CollectionId > 0 {
			tx.Where("collection_id = ?", query.CollectionId)
		}
	})
	response.Success(c, res)
}

// Detail 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param id path int true "ID"
// @Success 200 {object} response.Response{data=model.AddressBookCollectionRule}
// @Failure 500 {object} response.Response
// @Router /admin/address_book_collection_rule/detail/{id} [get]
// @Security token
func (abcr *AddressBookCollectionRule) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	t := service.AllService.AddressBookService.RuleInfoById(uint(iid))
	if t.Id > 0 {
		response.Success(c, t)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// Create 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body model.AddressBookCollectionRule true ""
// @Success 200 {object} response.Response{data=model.AddressBookCollection}
// @Failure 500 {object} response.Response
// @Router /admin/address_book_collection_rule/create [post]
// @Security token
func (abcr *AddressBookCollectionRule) Create(c *gin.Context) {
	f := &model.AddressBookCollectionRule{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	if f.Type != model.ShareAddressBookRuleTypePersonal && f.Type != model.ShareAddressBookRuleTypeGroup {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	t := f
	msg, res := abcr.CheckForm(t)
	if !res {
		response.Fail(c, 101, response.TranslateMsg(c, msg))
		return
	}
	err := service.AllService.AddressBookService.CreateRule(t)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

func (abcr *AddressBookCollectionRule) CheckForm(t *model.AddressBookCollectionRule) (string, bool) {
	if t.UserId == 0 {
		return "ParamsError", false
	}
	if t.CollectionId > 0 && !service.AllService.AddressBookService.CheckCollectionOwner(t.UserId, t.CollectionId) {
		return "ParamsError", false
	}

	//check to_id
	if t.Type == model.ShareAddressBookRuleTypePersonal {
		if t.ToId == t.UserId {
			return "CannotShareToSelf", false
		}
		tou := service.AllService.UserService.InfoById(t.ToId)
		if tou.Id == 0 {
			return "ItemNotFound", false
		}
	} else if t.Type == model.ShareAddressBookRuleTypeGroup {
		tog := service.AllService.GroupService.InfoById(t.ToId)
		if tog.Id == 0 {
			return "ItemNotFound", false
		}
	} else {
		return "ParamsError", false
	}

	ex := service.AllService.AddressBookService.RuleInfoByToIdAndCid(t.Type, t.ToId, t.CollectionId)
	if t.Id == 0 && ex.Id > 0 {
		return "ItemExists", false
	}
	if t.Id > 0 && ex.Id > 0 && t.Id != ex.Id {
		return "ItemExists", false
	}
	return "", true
}

// Update 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body model.AddressBookCollectionRule true ""
// @Success 200 {object} response.Response{data=model.AddressBookCollection}
// @Failure 500 {object} response.Response
// @Router /admin/address_book_collection_rule/update [post]
// @Security token
func (abcr *AddressBookCollectionRule) Update(c *gin.Context) {
	f := &model.AddressBookCollectionRule{}
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
	t := f
	msg, res := abcr.CheckForm(t)
	if !res {
		response.Fail(c, 101, response.TranslateMsg(c, msg))
		return
	}
	err := service.AllService.AddressBookService.UpdateRule(t)
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
// @Param body body model.AddressBookCollectionRule true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/address_book_collection_rule/delete [post]
// @Security token
func (abcr *AddressBookCollectionRule) Delete(c *gin.Context) {
	f := &model.AddressBookCollectionRule{}
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
	ex := service.AllService.AddressBookService.RuleInfoById(f.Id)
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.AddressBookService.DeleteRule(ex)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
}
