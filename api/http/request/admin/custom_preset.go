package admin

import "rustdesk-server/api/model"

type CustomPresetForm struct {
	Id         uint   `json:"id"`
	Name       string `json:"name" validate:"required"`
	Platform   string `json:"platform" validate:"required"`
	Version    string `json:"version" validate:"required"`
	AppName    string `json:"app_name"`
	CustomJson string `json:"custom_json"`
}

func (f *CustomPresetForm) ToCustomPreset() *model.CustomPreset {
	return &model.CustomPreset{
		Name:       f.Name,
		Platform:   f.Platform,
		Version:    f.Version,
		AppName:    f.AppName,
		CustomJson: f.CustomJson,
	}
}

func (f *CustomPresetForm) FromCustomPreset(p *model.CustomPreset) *CustomPresetForm {
	f.Id = p.Id
	f.Name = p.Name
	f.Platform = p.Platform
	f.Version = p.Version
	f.AppName = p.AppName
	f.CustomJson = p.CustomJson
	return f
}

type CustomPresetQuery struct {
	PageQuery
}
