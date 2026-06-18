package service

import (
	"rustdesk-server/api/model"
	"gorm.io/gorm"
)

type GroupService struct {
}

// InfoById id
func (us *GroupService) InfoById(id uint) *model.Group {
	u := &model.Group{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (us *GroupService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.GroupList) {
	res = &model.GroupList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Group{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Groups)
	return
}

// Create 
func (us *GroupService) Create(u *model.Group) error {
	res := DB.Create(u).Error
	return res
}
// Delete removes a Group and nulls out the foreign-key reference from any
// child rows in a single transaction so we don't leave orphan group_id values.
// NOTE: the peers table reuses the same group_id column for both Group and
// DeviceGroup (see DeviceGroupDelete and api/http/controller/api/group.go) —
// we null the peer rows whose group_id matches u.Id even though the column is
// shared. Deleting either kind of group cleans up its references.
func (us *GroupService) Delete(u *model.Group) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("group_id = ?", u.Id).Update("group_id", 0).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Peer{}).Where("group_id = ?", u.Id).Update("group_id", 0).Error; err != nil {
			return err
		}
		return tx.Delete(u).Error
	})
}

// Update 
func (us *GroupService) Update(u *model.Group) error {
	return DB.Model(u).Updates(u).Error
}

// DeviceGroupInfoById id
func (us *GroupService) DeviceGroupInfoById(id uint) *model.DeviceGroup {
	u := &model.DeviceGroup{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (us *GroupService) DeviceGroupList(page, pageSize uint, where func(tx *gorm.DB)) (res *model.DeviceGroupList) {
	res = &model.DeviceGroupList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.DeviceGroup{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.DeviceGroups)
	return
}

func (us *GroupService) DeviceGroupCreate(u *model.DeviceGroup) error {
	res := DB.Create(u).Error
	return res
}
// DeviceGroupDelete removes a DeviceGroup and nulls out the peers that
// reference it. The peers table does NOT have a dedicated device_group_id
// column — peer.group_id is overloaded to point at either a Group or a
// DeviceGroup row (see api/http/controller/api/group.go and the peer admin UI
// which populates the group_id dropdown from /device_group/list). Until that
// schema overload is split into two columns, deleting either kind of group
// clears the shared reference.
func (us *GroupService) DeviceGroupDelete(u *model.DeviceGroup) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Peer{}).Where("group_id = ?", u.Id).Update("group_id", 0).Error; err != nil {
			return err
		}
		return tx.Delete(u).Error
	})
}

func (us *GroupService) DeviceGroupUpdate(u *model.DeviceGroup) error {
	return DB.Model(u).Updates(u).Error
}
