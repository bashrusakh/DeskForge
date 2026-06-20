package service

import (
	"fmt"
	"os"
	"path/filepath"

	"rustdesk-server/api/model"
)

type CustomBuildService struct{}

// BuildOutputDir returns the on-disk path where the build agent writes
// artifacts for a given build id. The convention `/rdgen-data/output/<id>`
// is shared with docker/entrypoint-{linux,win}.sh and is referenced by the
// download handler in api/http/controller/admin/custom_build.go — keep this
// helper as the single source of truth so callers can't drift.
func BuildOutputDir(id uint) string {
	return filepath.Join("/rdgen-data", "output", fmt.Sprintf("%d", id))
}

func (is *CustomBuildService) List(page, pageSize uint) (res *model.CustomBuildList) {
	res = &model.CustomBuildList{}
	tx := DB.Model(&model.CustomBuild{})
	tx.Count(&res.Total)
	tx.Scopes(Paginate(page, pageSize)).Order("id desc").Find(&res.CustomBuilds)
	return
}

func (is *CustomBuildService) Info(id uint) *model.CustomBuild {
	u := &model.CustomBuild{}
	DB.Where("id = ?", id).First(u)
	return u
}

// Delete drops the DB row first, then best-effort removes the artifact
// directory. Filesystem cleanup happens after the DB delete so that a failed
// DB delete does not leave the user with a row pointing at missing files;
// the download handler already returns 404 if the directory is missing, so
// a failed cleanup is recoverable.
func (is *CustomBuildService) Delete(u *model.CustomBuild) error {
	id := u.Id
	if err := DB.Delete(u).Error; err != nil {
		return err
	}
	dir := BuildOutputDir(id)
	if err := os.RemoveAll(dir); err != nil {
		Logger.Warnf("custom_build: failed to remove artifact dir %s: %v", dir, err)
	}
	return nil
}

func (is *CustomBuildService) Create(u *model.CustomBuild) error {
	return DB.Create(u).Error
}

// Update — full-row save. Раньше использовался `Updates(struct)`, который в gorm
// тихо игнорирует zero-value поля (status="", file_size=0, github_run_id=0). Для
// pollAndDownload это уже ловило баги (BUGS.md B-011); переход на Save снимает мину.
func (is *CustomBuildService) Update(u *model.CustomBuild) error {
	return DB.Save(u).Error
}
