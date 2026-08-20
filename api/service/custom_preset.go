package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if p.PreservePermanentPassword && !hasNonEmptyPermanentPassword(p.CustomJson) {
		existing, err := findCustomPresetForPreservation(p)
		if err != nil {
			return err
		}
		if existing != nil {
			p.CustomJson, err = mergePreservedPermanentPassword(p.CustomJson, existing.CustomJson)
			if err != nil {
				return err
			}
		}
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

	// Insert with a zero primary key so a caller-provided ID cannot select an
	// unrelated primary-key conflict instead of the owner/name conflict target.
	candidate := *p
	candidate.Id = 0
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "name"}},
		UpdateAll: true,
	}).Create(&candidate).Error; err != nil {
		return err
	}

	// Do not depend on dialect-specific upsert ID reporting. Reload the one
	// persisted owner/name row and return its canonical, decrypted state.
	persisted := &model.CustomPreset{}
	if err := DB.Where("user_id = ? AND name = ?", p.UserId, p.Name).First(persisted).Error; err != nil {
		return fmt.Errorf("reload custom preset after upsert: %w", err)
	}
	preservePermanentPassword := p.PreservePermanentPassword
	*p = *persisted
	p.PreservePermanentPassword = preservePermanentPassword
	return nil
}

func (ps *CustomPresetService) Update(p *model.CustomPreset) error {
	if err := validateCustomPresetFields(p, true); err != nil {
		return err
	}
	if err := ValidateDirectCustomBuilderJSON(p.CustomJson); err != nil {
		return err
	}
	var err error
	if p.PreservePermanentPassword && !hasNonEmptyPermanentPassword(p.CustomJson) {
		stored := &model.CustomPreset{}
		if err := DB.First(stored, p.Id).Error; err != nil {
			return err
		}
		p.CustomJson, err = mergePreservedPermanentPassword(p.CustomJson, stored.CustomJson)
		if err != nil {
			return err
		}
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

func findCustomPresetForPreservation(p *model.CustomPreset) (*model.CustomPreset, error) {
	if p.Name == "" {
		return nil, nil
	}
	existing := &model.CustomPreset{}
	err := DB.Where("user_id = ? AND name = ?", p.UserId, p.Name).First(existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func hasNonEmptyPermanentPassword(customJSON string) bool {
	return utils.CustomBuilderJSONHasPermanentPassword(customJSON)
}

func mergePreservedPermanentPassword(incomingJSON, existingJSON string) (string, error) {
	if hasNonEmptyPermanentPassword(incomingJSON) {
		return incomingJSON, nil
	}
	var incoming map[string]any
	if strings.TrimSpace(incomingJSON) == "" {
		incoming = make(map[string]any)
	} else if err := json.Unmarshal([]byte(incomingJSON), &incoming); err != nil {
		return "", &ClientValidationError{Err: fmt.Errorf("invalid custom JSON: %w", err)}
	}
	if incoming == nil {
		return "", &ClientValidationError{Err: fmt.Errorf("custom JSON must be an object")}
	}
	if strings.TrimSpace(existingJSON) == "" {
		return incomingJSON, nil
	}
	var existing map[string]any
	if err := json.Unmarshal([]byte(existingJSON), &existing); err != nil {
		return "", fmt.Errorf("read existing permanent_password: %w", err)
	}
	password, ok := existing["permanent_password"].(string)
	if !ok || strings.TrimSpace(password) == "" {
		return incomingJSON, nil
	}
	incoming["permanent_password"] = password
	merged, err := json.Marshal(incoming)
	if err != nil {
		return "", fmt.Errorf("marshal preserved permanent_password: %w", err)
	}
	return string(merged), nil
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
