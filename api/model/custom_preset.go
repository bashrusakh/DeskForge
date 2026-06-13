package model

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
