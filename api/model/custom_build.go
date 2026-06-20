package model

import (
	"rustdesk-server/api/utils"

	"gorm.io/gorm"
)

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
	// GithubRunId — id рана GitHub Actions, если билд диспетчился туда. Нужен для
	// возобновления `pollAndDownload` после рестарта api (BUGS.md B-003). 0 = file-queue
	// или ещё не диспетчен.
	GithubRunId int64 `json:"github_run_id" gorm:"default:0;not null;"`
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

// --- BUGS.md B-008: permanent_password лежит внутри custom_json. Шифруем весь
// JSON-блоб at rest; вызывающий код видит открытый JSON как раньше. ------------

func (c *CustomBuild) BeforeSave(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.EncryptSecret(c.CustomJson)
	return err
}

func (c *CustomBuild) AfterSave(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.DecryptSecret(c.CustomJson)
	return err
}

func (c *CustomBuild) AfterFind(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.DecryptSecret(c.CustomJson)
	return err
}
