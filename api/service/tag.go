package service

import (
	"rustdesk-server/api/model"
	"gorm.io/gorm"
)

type TagService struct {
}

func (s *TagService) Info(id uint) *model.Tag {
	p := &model.Tag{}
	DB.Where("id = ?", id).First(p)
	return p
}
func (s *TagService) InfoByUserIdAndNameAndCollectionId(userid uint, name string, cid uint) *model.Tag {
	p := &model.Tag{}
	DB.Where("user_id = ? and name = ? and collection_id = ?", userid, name, cid).First(p)
	return p
}

func (s *TagService) ListByUserId(userId uint) (res *model.TagList) {
	res = s.List(1, 1000, func(tx *gorm.DB) {
		tx.Where("user_id = ?", userId)
	})
	return
}
func (s *TagService) ListByUserIdAndCollectionId(userId, cid uint) (res *model.TagList) {
	res = s.List(1, 1000, func(tx *gorm.DB) {
		tx.Where("user_id = ? and collection_id = ?", userId, cid)
		tx.Order("name asc")
	})
	return
}
func (s *TagService) UpdateTags(userId uint, tags map[string]uint) {
	tx := DB.Begin()
	//tag
	var allTags []*model.Tag
	tx.Where("user_id = ?", userId).Find(&allTags)
	for _, t := range allTags {
		if _, ok := tags[t.Name]; !ok {

			tx.Delete(t)
		} else {
			if tags[t.Name] != t.Color {

				t.Color = tags[t.Name]
				tx.Save(t)
			}

			delete(tags, t.Name)
		}
	}

	for tag, color := range tags {
		t := &model.Tag{}
		t.Name = tag
		t.Color = color
		t.UserId = userId
		tx.Create(t)
	}
	tx.Commit()
}

// InfoById id
func (s *TagService) InfoById(id uint) *model.Tag {
	u := &model.Tag{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (s *TagService) List(page, pageSize uint, where func(tx *gorm.DB)) (res *model.TagList) {
	res = &model.TagList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	tx := DB.Model(&model.Tag{})
	if where != nil {
		where(tx)
	}
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize))
	tx.Find(&res.Tags)
	return
}

// Create 
func (s *TagService) Create(u *model.Tag) error {
	res := DB.Create(u).Error
	return res
}
func (s *TagService) Delete(u *model.Tag) error {
	return DB.Delete(u).Error
}

// Update 
func (s *TagService) Update(u *model.Tag) error {
	return DB.Model(u).Select("*").Omit("created_at").Updates(u).Error
}
