package model

type CustomBuild struct {
	IdModel
	UserId      uint   `json:"user_id" gorm:"default:0;not null;"`
	Name        string `json:"name" gorm:"size:128;default:'';not null;"`
	Platform    string `json:"platform" gorm:"size:32;default:'';not null;"`
	Version     string `json:"version" gorm:"size:32;default:'';not null;"`
	Status      string `json:"status" gorm:"size:32;default:'pending';not null;"`
	AppName     string `json:"app_name" gorm:"size:128;default:'';not null;"`
	CustomJson  string `json:"custom_json" gorm:"type:text;"`
	BuildLog    string `json:"build_log" gorm:"type:text;"`
	FileSize    int64  `json:"file_size" gorm:"default:0;not null;"`
	DownloadKey string `json:"download_key" gorm:"size:64;default:'';not null;"`
	TimeModel
}

type CustomBuildList struct {
	CustomBuilds []*CustomBuild `json:"list"`
	Pagination
}

const (
	CustomBuildStatusPending   = "pending"
	CustomBuildStatusBuilding  = "building"
	CustomBuildStatusDone      = "done"
	CustomBuildStatusFailed    = "failed"
)
