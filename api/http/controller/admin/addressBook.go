package admin

import (
	"encoding/json"
	_ "encoding/json"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/service"
	"strconv"
)

type AddressBook struct {
}

// Detail is retained for internal callers but is not registered by the router.
func (ct *AddressBook) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	t := service.AllService.AddressBookService.InfoByRowId(uint(iid))
	if t.RowId > 0 {
		response.Success(c, t.Safe())
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
// @Param body body admin.AddressBookForm true "Address book entry form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/address_book/create [post]
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
	if t.UserId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
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

// BatchCreate
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param body body admin.AddressBookForm true "Address book batch-create form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/address_book/batchCreate [post]
// @Security token
func (ct *AddressBook) BatchCreate(c *gin.Context) {
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
	ul := len(f.UserIds)

	if ul == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if ul > 1 {

		f.Tags = []string{}

		f.CollectionId = 0
	}

	/*for _, fu := range f.UserIds {
		if fu == 0 {
			continue
		}
		for _, ft := range f.Tags {
			exTag := service.AllService.TagService.InfoByUserIdAndNameAndCollectionId(fu, ft, 0)
			if exTag.Id == 0 {
				service.AllService.TagService.Create(&model.Tag{
					UserId: fu,
					Name:   ft,
				})
			}
		}
	}*/
	ts := f.ToAddressBooks()
	for _, t := range ts {
		if t.UserId == 0 {
			continue
		}
		ex := service.AllService.AddressBookService.InfoByUserIdAndIdAndCid(t.UserId, t.Id, t.CollectionId)
		if ex.RowId == 0 {
			service.AllService.AddressBookService.Create(t)
		}
	}

	response.Success(c, nil)
}

// List
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param page query int false "Page number for the address book list"
// @Param page_size query int false "Number of entries per address book page"
// @Param user_id query int false "id"
// @Param is_my query int false "Filter for address books owned by the current user"
// @Success 200 {object} response.Response{data=model.AddressBookSafeList} "Redacted address-book list envelope"
// @Failure 500 {object} response.Response
// @Router /admin/address_book/list [get]
// @Security token
func (ct *AddressBook) List(c *gin.Context) {
	query := &admin.AddressBookQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.AddressBookService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		tx.Preload("Collection", func(txc *gorm.DB) *gorm.DB {
			return txc.Select("id,name")
		})
		if query.Id != "" {
			tx.Where("id like ?", "%"+query.Id+"%")
		}
		if query.UserId > 0 {
			tx.Where("user_id = ?", query.UserId)
		}
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

// Update
// @Tags
// @Summary
// @Description
// @Accept  json
// @Produce  json
// @Param body body admin.AddressBookForm true "Address book entry update form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/address_book/update [post]
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
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	ex := service.AllService.AddressBookService.InfoByRowId(f.RowId)
	if ex.RowId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
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
// @Param body body admin.AddressBookForm true "Address book entry deletion form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/address_book/delete [post]
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
	t := service.AllService.AddressBookService.InfoByRowId(f.RowId)
	if t.RowId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.AddressBookService.Delete(t)
	if err == nil {
		response.Success(c, nil)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
}

// ShareByWebClient
// @Tags
// @Summary
// @Description Authenticated web-client share route; requires the admin API token but not AdminPrivilege.
// @Accept  json
// @Produce  json
// @Param body body admin.ShareByWebClientForm true "Web client address book sharing form payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/address_book/shareByWebClient [post]
// @Security token
func (ct *AddressBook) ShareByWebClient(c *gin.Context) {
	f := &admin.ShareByWebClientForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}

	u := service.AllService.UserService.CurUser(c)
	ab := service.AllService.AddressBookService.InfoByUserIdAndId(u.Id, f.Id)
	if ab.RowId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	m := f.ToShareRecord()
	m.UserId = u.Id
	err := service.AllService.AddressBookService.ShareByWebClient(m)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, &gin.H{
		"share_token": m.ShareToken,
	})
}

// BatchCreateFromPeers
// @Tags
// @Summary
// @Description Admin-only batch creation of address-book entries from existing peers.
// @Accept  json
// @Produce  json
// @Param body body admin.BatchCreateFromPeersForm true "Peer IDs and address-book destination payload"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/address_book/batchCreateFromPeers [post]
// @Security token
func (ct *AddressBook) BatchCreateFromPeers(c *gin.Context) {
	f := &admin.BatchCreateFromPeersForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	if f.UserId == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	if f.CollectionId != 0 {
		collection := service.AllService.AddressBookService.CollectionInfoById(f.CollectionId)
		if collection.Id == 0 {
			response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
			return
		}
	}

	pl := int64(len(f.PeerIds))
	peers := service.AllService.PeerService.List(1, uint(pl), func(tx *gorm.DB) {
		tx.Where("row_id in ?", f.PeerIds)
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
		ab.UserId = f.UserId
		ex := service.AllService.AddressBookService.InfoByUserIdAndIdAndCid(f.UserId, ab.Id, ab.CollectionId)
		if ex.RowId != 0 {
			continue
		}
		service.AllService.AddressBookService.Create(ab)
	}
	response.Success(c, nil)
}
