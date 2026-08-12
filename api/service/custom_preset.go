package service

import (
	"fmt"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"
)

type CustomPresetService struct{}

func (ps *CustomPresetService) List(page, pageSize uint) (*model.CustomPresetList, error) {
	res := &model.CustomPresetList{}
	tx := DB.Model(&model.CustomPreset{})
	if err := tx.Count(&res.Total).Error; err != nil {
		return nil, err
	}
	if err := tx.Scopes(Paginate(page, pageSize)).Order("id desc").Find(&res.CustomPresets).Error; err != nil {
		return nil, err
	}
	return res, nil
}

func (ps *CustomPresetService) ListByUser(page, pageSize, userId uint) (*model.CustomPresetList, error) {
	res := &model.CustomPresetList{}
	tx := DB.Model(&model.CustomPreset{}).Where("user_id = ?", userId)
	if err := tx.Count(&res.Total).Error; err != nil {
		return nil, err
	}
	if err := tx.Scopes(Paginate(page, pageSize)).Order("id desc").Find(&res.CustomPresets).Error; err != nil {
		return nil, err
	}
	return res, nil
}

func (ps *CustomPresetService) Info(id uint) (*model.CustomPreset, error) {
	p := &model.CustomPreset{}
	if err := DB.Where("id = ?", id).First(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// Create — upsert по (user_id, name): если запись с таким именем у юзера уже есть,
// перезаписывает её содержимое (§8.9 «Save as preset → перезаписывать при совпадении»).
// Иначе создаёт новую. Поле Id у входящего p при upsert будет установлено на найденный.
func (ps *CustomPresetService) Create(p *model.CustomPreset) error {
	if err := validateCustomPresetFields(p, false); err != nil {
		return err
	}
	if err := ValidateDirectCustomBuilderJSON(p.CustomJson); err != nil {
		return err
	}
	canonicalJSON, err := CanonicalizeCustomBuildJSON(p.CustomJson, BuildRecordContext{
		Platform: p.Platform,
		AppName:  p.AppName,
		Version:  p.Version,
	})
	if err != nil {
		return err
	}
	if err := utils.RequireSecretEncryptionForCustomBuilderJSON(canonicalJSON); err != nil {
		return err
	}
	p.CustomJson = canonicalJSON
	if p.Name != "" {
		existing := &model.CustomPreset{}
		err = DB.Where("user_id = ? AND name = ?", p.UserId, p.Name).First(existing).Error
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
	if err := validateCustomPresetFields(p, true); err != nil {
		return err
	}
	if err := ValidateDirectCustomBuilderJSON(p.CustomJson); err != nil {
		return err
	}
	canonicalJSON, err := CanonicalizeCustomBuildJSON(p.CustomJson, BuildRecordContext{
		BuildID:  p.Id,
		Platform: p.Platform,
		AppName:  p.AppName,
		Version:  p.Version,
	})
	if err != nil {
		return err
	}
	if err := utils.RequireSecretEncryptionForCustomBuilderJSON(canonicalJSON); err != nil {
		return err
	}
	p.CustomJson = canonicalJSON
	// Save is required here: Updates(struct) silently drops intentional zero
	// values such as an empty custom_json.
	return DB.Save(p).Error
}

func validateCustomPresetFields(p *model.CustomPreset, requireID bool) error {
	if err := ValidateCustomPlatform(p.Platform); err != nil {
		return err
	}
	if requireID && p.Id == 0 {
		return &ClientValidationError{Err: fmt.Errorf("preset id is required")}
	}
	if p.Name == "" {
		return &ClientValidationError{Err: fmt.Errorf("preset name is required")}
	}
	if p.Version == "" {
		return &ClientValidationError{Err: fmt.Errorf("preset version is required")}
	}
	if !utils.ValidateBuildVersion(p.Version) {
		return &ClientValidationError{Err: fmt.Errorf("invalid version format: %s", p.Version)}
	}
	return nil
}

func (ps *CustomPresetService) Delete(p *model.CustomPreset) error {
	return DB.Delete(p).Error
}
