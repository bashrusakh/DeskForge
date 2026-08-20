package model

import (
	"rustdesk-server/api/utils"

	"gorm.io/gorm"
)

type CustomPreset struct {
	IdModel
	UserId     uint   `json:"user_id" gorm:"default:0;not null;"`
	Name       string `json:"name" gorm:"size:128;default:'';not null;"`
	Platform   string `json:"platform" gorm:"size:32;default:'';not null;"`
	Version    string `json:"version" gorm:"size:32;default:'';not null;"`
	AppName    string `json:"app_name" gorm:"size:128;default:'';not null;"`
	CustomJson string `json:"-" gorm:"type:text;"`
	// PreservePermanentPassword is request intent only; it is never stored or serialized.
	PreservePermanentPassword bool `json:"-" gorm:"-"`
	TimeModel
}

type CustomPresetList struct {
	CustomPresets []*CustomPreset `json:"list"`
	Pagination
}

// CustomPresetSafe is the administrative response view. Its custom_json is
// limited to canonical non-secret settings.
type CustomPresetSafe struct {
	Id                   uint   `json:"id"`
	UserId               uint   `json:"user_id"`
	Name                 string `json:"name"`
	Platform             string `json:"platform"`
	Version              string `json:"version"`
	AppName              string `json:"app_name"`
	CustomJson           string `json:"custom_json"`
	HasPermanentPassword bool   `json:"has_permanent_password"`
	TimeModel
}

// CustomPresetSafeList is the paginated administrative response view.
type CustomPresetSafeList struct {
	CustomPresets []*CustomPresetSafe `json:"list"`
	Pagination
}

// Safe returns a response-only view that cannot serialize raw custom_json.
func (c *CustomPreset) Safe() *CustomPresetSafe {
	if c == nil {
		return nil
	}
	return &CustomPresetSafe{
		Id:                   c.Id,
		UserId:               c.UserId,
		Name:                 c.Name,
		Platform:             c.Platform,
		Version:              c.Version,
		AppName:              c.AppName,
		CustomJson:           utils.RedactCustomBuilderJSON(c.CustomJson),
		HasPermanentPassword: utils.CustomBuilderJSONHasPermanentPassword(c.CustomJson),
		TimeModel:            c.TimeModel,
	}
}

// Safe returns a paginated response-only view for administrative consumers.
func (l *CustomPresetList) Safe() *CustomPresetSafeList {
	if l == nil {
		return nil
	}
	view := &CustomPresetSafeList{Pagination: l.Pagination}
	if l.CustomPresets != nil {
		view.CustomPresets = make([]*CustomPresetSafe, 0, len(l.CustomPresets))
		for _, preset := range l.CustomPresets {
			view.CustomPresets = append(view.CustomPresets, preset.Safe())
		}
	}
	return view
}

// --- BUGS.md B-008: non-empty permanent_password lies inside custom_json.
// Secret-bearing JSON is encrypted at rest; non-secret typed JSON keeps its
// existing representation so core non-secret operations remain available. ----

func (c *CustomPreset) BeforeSave(tx *gorm.DB) error {
	if err := utils.ValidateCustomBuilderJSONFields(c.CustomJson); err != nil {
		return err
	}
	customJSON, err := utils.EncryptCustomBuilderJSON(c.CustomJson)
	if err != nil {
		return err
	}
	c.CustomJson = customJSON
	return err
}

func (c *CustomPreset) AfterSave(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.DecryptSecret(c.CustomJson)
	return err
}

func (c *CustomPreset) AfterFind(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.DecryptSecret(c.CustomJson)
	return err
}
