package my

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
	"gorm.io/gorm"
)

type AddressBook struct{}

// List 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param page query int false ""
// @Param page_size query int false ""
// @Param user_id query int false "id"
// @Success 200 {object} response.Response{data=model.AddressBookList}
// @Failure 500 {object} response.Response
// @Router /admin/my/address_book/list [get]
// @Security token
func (ct *AddressBook) List(c *gin.Context) {
	query := &admin.AddressBookQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)
	query.UserId = int(u.Id)
	res := service.AllService.AddressBookService.List(query.Page, query.PageSize, func(tx *gorm.DB) {

		tx.Preload("Collection", func(txc *gorm.DB) *gorm.DB {
			return txc.Select("id,name")
		})
		if query.Id != "" {
			tx.Where("id like ?", "%"+query.Id+"%")
		}
		tx.Where("user_id = ?", query.UserId)
		if query.Username != "" {
			tx.Where("username like ?", "%"+query.Username+"%")
		}
		if query.Hostname != "" {
			tx.Where("hostname like ?", "%"+query.Hostname+"%")
		}
		if query.CollectionId != nil && *query.CollectionId >= 0 {
			tx.Where("collection_id = ?", query.CollectionId)
		}
	})

	abCIds := make([]uint, 0)
	for _, ab := range res.AddressBooks {
		abCIds = append(abCIds, ab.CollectionId)
	}
	response.Success(c, res)
}

// Create 
// @Tags 
// @Summary 
// @Description 
// @Accept  json
// @Produce  json
// @Param body body admin.AddressBookForm true ""
// @Success 200 {object} response.Response{data=model.AddressBook}
// @Failure 500 {object} response.Response
// @Router /admin/my/address_book/create [post]
// @Security token
func (ct *AddressBook) Create(c *gin.Context) {
	f := &admin.AddressBookForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	t := f.ToAddressBook()
	u := service.AllService.UserService.CurUser(c)
	t.UserId = u.Id
	if t.CollectionId > 0 && !service.AllService.AddressBookService.CheckCollectionOwner(t.UserId, t.CollectionId) {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	ex := service.AllService.AddressBookService.InfoByUserIdAndIdAndCid(t.UserId, t.Id, t.CollectionId)
	if ex.RowId > 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemExists"))
		return
	}

	err := service.AllService.AddressBookService.Create(t)
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
// @Param body body admin.AddressBookForm true ""
// @Success 200 {object} response.Response{data=model.AddressBook}
// @Failure 500 {object} response.Response
// @Router /admin/my/address_book/update [post]
// @Security token
func (ct *AddressBook) Update(c *gin.Context) {
	f := &admin.AddressBookForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	if f.RowId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	u := service.AllService.UserService.CurUser(c)
	if f.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}

	ex := service.AllService.AddressBookService.InfoByRowId(f.RowId)
	if ex.RowId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if ex.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	t := f.ToAddressBook()
	if t.CollectionId > 0 && !service.AllService.AddressBookService.CheckCollectionOwner(t.UserId, t.CollectionId) {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	err := service.AllService.AddressBookService.UpdateAll(t)
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
// @Param body body admin.AddressBookForm true ""
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/address_book/delete [post]
// @Security token
func (ct *AddressBook) Delete(c *gin.Context) {
	f := &admin.AddressBookForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.RowId
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	ex := service.AllService.AddressBookService.InfoByRowId(f.RowId)
	if ex.RowId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	u := service.AllService.UserService.CurUser(c)
	if ex.UserId != u.Id {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	err := service.AllService.AddressBookService.Delete(ex)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
	return
}
func (ct *AddressBook) BatchCreateFromPeers(c *gin.Context) {
	f := &admin.BatchCreateFromPeersForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)

	if f.CollectionId != 0 {
		collection := service.AllService.AddressBookService.CollectionInfoById(f.CollectionId)
		if collection.Id == 0 {
			response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
			return
		}
		if collection.UserId != u.Id {
			response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
			return
		}
	}
	if len(f.PeerIds) == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	pl := int64(len(f.PeerIds))
	peers := service.AllService.PeerService.List(1, uint(pl), func(tx *gorm.DB) {
		tx.Where("row_id in ?", f.PeerIds)
		tx.Where("user_id = ?", u.Id)
	})
	if peers.Total == 0 || pl != peers.Total {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}

	tags, _ := json.Marshal(f.Tags)
	for _, peer := range peers.Peers {
		ab := service.AllService.AddressBookService.FromPeer(peer)
		ab.Tags = tags
		ab.CollectionId = f.CollectionId
		ex := service.AllService.AddressBookService.InfoByUserIdAndIdAndCid(u.Id, ab.Id, ab.CollectionId)
		if ex.RowId != 0 {
			continue
		}
		service.AllService.AddressBookService.Create(ab)
	}
	response.Success(c, nil)
}

func (ct *AddressBook) BatchUpdateTags(c *gin.Context) {
	f := &admin.BatchUpdateTagsForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	u := service.AllService.UserService.CurUser(c)

	abs := service.AllService.AddressBookService.List(1, 999, func(tx *gorm.DB) {
		tx.Where("row_id in ?", f.RowIds)
		tx.Where("user_id = ?", u.Id)
	})
	if abs.Total == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.AddressBookService.BatchUpdateTags(abs.AddressBooks, f.Tags)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}
