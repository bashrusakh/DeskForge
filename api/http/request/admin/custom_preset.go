package admin

import "rustdesk-server/api/model"

type CustomPresetForm struct {
	Id                        uint   `json:"id"`
	Name                      string `json:"name" validate:"required"`
	Platform                  string `json:"platform" validate:"required"`
	Version                   string `json:"version" validate:"required"`
	AppName                   string `json:"app_name"`
	CustomJson                string `json:"custom_json"`
	PreservePermanentPassword bool   `json:"preserve_permanent_password" swaggerignore:"true"`
	BuildRef                  string `json:"build_ref" swaggerignore:"true"`
}

func (f *CustomPresetForm) ToCustomPreset() *model.CustomPreset {
	return &model.CustomPreset{
		Name:                      f.Name,
		Platform:                  f.Platform,
		Version:                   f.Version,
		AppName:                   f.AppName,
		CustomJson:                f.CustomJson,
		PreservePermanentPassword: f.PreservePermanentPassword,
	}
}

type CustomPresetQuery struct {
	PageQuery
}
