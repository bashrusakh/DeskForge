package service

import (
	"rustdesk-server/api/model"
)

type CustomBuildService struct{}

func (is *CustomBuildService) List(page, pageSize uint) (res *model.CustomBuildList) {
	res = &model.CustomBuildList{}
	tx := DB.Model(&model.CustomBuild{})
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize)).Order("id desc").Find(&res.CustomBuilds)
	return
}

func (is *CustomBuildService) Info(id uint) *model.CustomBuild {
	u := &model.CustomBuild{}
	DB.Where("id = ?", id).First(u)
	return u
}

func (is *CustomBuildService) Delete(u *model.CustomBuild) error {
	return DB.Delete(u).Error
}

func (is *CustomBuildService) Create(u *model.CustomBuild) error {
	return DB.Create(u).Error
}

func (is *CustomBuildService) Update(u *model.CustomBuild) error {
	return DB.Model(u).Updates(u).Error
}
