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
func (us *GroupService) Delete(u *model.Group) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("group_id = ?", u.Id).Update("group_id", 0).Error; err != nil {
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
func (us *GroupService) DeviceGroupDelete(u *model.DeviceGroup) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Peer{}).Where("device_group_id = ?", u.Id).Update("device_group_id", 0).Error; err != nil {
			return err
		}
		return tx.Delete(u).Error
	})
}

func (us *GroupService) DeviceGroupUpdate(u *model.DeviceGroup) error {
	return DB.Model(u).Updates(u).Error
}
