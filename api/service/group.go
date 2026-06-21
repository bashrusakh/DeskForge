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

// DefaultGroupId возвращает id системной группы по умолчанию (Type=GroupTypeDefault),
// в которую попадают новорегистрируемые пользователи. Раньше id=1 был зашит в коде
// (BUGS.md AU-L-015) — это ломается, если порядок создания групп иной. Фолбэк на 1
// сохраняет прежнее поведение, если группа почему-то не найдена.
func (us *GroupService) DefaultGroupId() uint {
	g := &model.Group{}
	if err := DB.Where("type = ?", model.GroupTypeDefault).Order("id asc").First(g).Error; err == nil && g.Id > 0 {
		return g.Id
	}
	return 1
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
// Delete removes a Group and nulls out user.group_id references in a transaction.
// peer.group_id is NOT touched here: it stores DeviceGroup IDs, never Group IDs
// (the peer admin UI populates the group dropdown from /device_group/list).
// Peer–DeviceGroup cleanup is handled exclusively by DeviceGroupDelete.
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
// DeviceGroupDelete removes a DeviceGroup and nulls out peer.group_id references
// in a transaction. peer.group_id stores DeviceGroup IDs (the peer admin UI
// populates the group dropdown from /device_group/list), so this is the correct
// and only place that clears peer–group assignments.
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
