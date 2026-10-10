package admin

import "rustdesk-server/api/model"

type CustomBuildForm struct {
	Id         uint   `json:"id"`
	Name       string `json:"name" validate:"required"`
	Platform   string `json:"platform" validate:"required"`
	Version    string `json:"version" validate:"required"`
	AppName    string `json:"app_name"`
	CustomJson string `json:"custom_json"`
	// PresetId is the optional source-preset reference. It is request-only
	// provenance input: the server resolves it against presets owned by the
	// current user and persists the resolved id/name snapshot; a provided id
	// that does not resolve rejects the create. nil means no source preset.
	PresetId *uint  `json:"preset_id"`
	BuildRef string `json:"build_ref" swaggerignore:"true"`
}

func (f *CustomBuildForm) ToCustomBuild() *model.CustomBuild {
	b := &model.CustomBuild{
		Name:       f.Name,
		Platform:   f.Platform,
		Version:    f.Version,
		AppName:    f.AppName,
		CustomJson: f.CustomJson,
	}
	if f.PresetId != nil {
		b.PresetId = *f.PresetId
	}
	return b
}

type CustomBuildQuery struct {
	PageQuery
}
