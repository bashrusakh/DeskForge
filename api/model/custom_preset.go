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
	CustomJson string `json:"custom_json" gorm:"type:text;"`
	TimeModel
}

type CustomPresetList struct {
	CustomPresets []*CustomPreset `json:"list"`
	Pagination
}

// --- BUGS.md B-008: permanent_password лежит внутри custom_json. Шифруем весь
// JSON-блоб at rest; вызывающий код видит открытый JSON как раньше. ------------

func (c *CustomPreset) BeforeSave(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.EncryptSecret(c.CustomJson)
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
