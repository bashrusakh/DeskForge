package service

import (
	"rustdesk-server/api/model"
	"gorm.io/gorm"
)

type ShareRecordService struct {
}

// InfoById id
func (srs *ShareRecordService) InfoById(id uint) *model.ShareRecord {
	u := &model.ShareRecord{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (srs *ShareRecordService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.ShareRecordList) {
	res = &model.ShareRecordList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.ShareRecord{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.ShareRecords)
	return
}

// Create 
func (srs *ShareRecordService) Create(u *model.ShareRecord) error {
	res := DB.Create(u).Error
	return res
}
func (srs *ShareRecordService) Delete(u *model.ShareRecord) error {
	return DB.Delete(u).Error
}

// Update 
func (srs *ShareRecordService) Update(u *model.ShareRecord) error {
	return DB.Model(u).Updates(u).Error
}

func (srs *ShareRecordService) BatchDelete(ids []uint) error {
	return DB.Where("id in (?)", ids).Delete(&model.ShareRecord{}).Error
}
