package my

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
)

type AddressBook struct{}

// List
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param page query int false "Page number for the personal address book list"
// @Param page_size query int false "Number of personal address book entries per page"
// @Param user_id query int false "id"
// @Success 200 {object} response.Response{data=model.AddressBookSafeList} "Redacted personal address-book list envelope"
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
	response.Success(c, res.Safe())
}

// Create
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param body body admin.AddressBookForm true "Personal address book entry form payload"
// @Success 200 {object} response.Response
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
// @Param body body admin.AddressBookForm true "Personal address book entry update form payload"
// @Success 200 {object} response.Response
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
// @Param body body admin.AddressBookForm true "Personal address book entry deletion form payload"
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

// BatchCreateFromPeers
// @Tags
// @Summary
// @Description Personal batch creation of address-book entries from the current user's peers.
// @Accept  json
// @Produce  json
// @Param body body admin.BatchCreateFromPeersForm true "Peer IDs and address-book destination payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/address_book/batchCreateFromPeers [post]
// @Security token
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

// BatchUpdateTags changes tags for the current user's address-book entries.
// @Tags AddressBook
// @Summary Update personal address-book tags
// @Description Authenticated users may update tags on their own address-book entries.
// @Accept json
// @Produce json
// @Param body body admin.BatchUpdateTagsForm true "Address-book tag update payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/my/address_book/batchUpdateTags [post]
// @Security token
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
