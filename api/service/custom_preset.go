package service

import (
	"rustdesk-server/api/model"
)

type CustomPresetService struct{}

func (ps *CustomPresetService) List(page, pageSize uint) (res *model.CustomPresetList) {
	res = &model.CustomPresetList{}
	tx := DB.Model(&model.CustomPreset{})
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize)).Order("id desc").Find(&res.CustomPresets)
	return
}

func (ps *CustomPresetService) Info(id uint) *model.CustomPreset {
	p := &model.CustomPreset{}
	DB.Where("id = ?", id).First(p)
	return p
}

// Create — upsert по (user_id, name): если запись с таким именем у юзера уже есть,
// перезаписывает её содержимое (§8.9 «Save as preset → перезаписывать при совпадении»).
// Иначе создаёт новую. Поле Id у входящего p при upsert будет установлено на найденный.
func (ps *CustomPresetService) Create(p *model.CustomPreset) error {
	if p.Name != "" {
		existing := &model.CustomPreset{}
		err := DB.Where("user_id = ? AND name = ?", p.UserId, p.Name).First(existing).Error
		if err == nil && existing.Id > 0 {
			// найден → перезаписываем (Updates не трогает zero-value через struct,
			// поэтому используем Save)
			p.Id = existing.Id
			return DB.Save(p).Error
		}
	}
	return DB.Create(p).Error
}

func (ps *CustomPresetService) Update(p *model.CustomPreset) error {
	return DB.Model(p).Updates(p).Error
}

func (ps *CustomPresetService) Delete(p *model.CustomPreset) error {
	return DB.Delete(p).Error
}
