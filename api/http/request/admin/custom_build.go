package admin

import "rustdesk-server/api/model"

type CustomBuildForm struct {
	Id         uint   `json:"id"`
	Name       string `json:"name" validate:"required"`
	Platform   string `json:"platform" validate:"required"`
	Version    string `json:"version" validate:"required"`
	AppName    string `json:"app_name"`
	CustomJson string `json:"custom_json"`
}

func (f *CustomBuildForm) ToCustomBuild() *model.CustomBuild {
	return &model.CustomBuild{
		Name:       f.Name,
		Platform:   f.Platform,
		Version:    f.Version,
		AppName:    f.AppName,
		CustomJson: f.CustomJson,
	}
}

type CustomBuildQuery struct {
	PageQuery
}
