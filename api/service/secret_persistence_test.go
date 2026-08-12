package service

import (
	"errors"
	"strings"
	"testing"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSecretBearingGithubConfigSaveFailsWithoutKeyBeforeCreatingRow(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	db := newSecretPersistenceDB(t, &model.GithubBuildConfig{})

	err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{
		Repo:       "owner/repo",
		Token:      "github_pat_secret",
		PayloadKey: "payload-secret",
	})
	var keyErr *utils.SecretEncryptionKeyError
	if !errors.As(err, &keyErr) {
		t.Fatalf("Save() error = %T %v, want SecretEncryptionKeyError", err, err)
	}
	var count int64
	if err := db.Model(&model.GithubBuildConfig{}).Count(&count).Error; err != nil {
		t.Fatalf("count configs: %v", err)
	}
	if count != 0 {
		t.Fatalf("missing-key config save created %d row(s)", count)
	}
}

func TestSecretBearingCustomCreateAndUpdateFailWithoutKeyBeforeRowMutation(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "")

	t.Run("custom build create", func(t *testing.T) {
		db := newSecretPersistenceDB(t, &model.CustomBuild{})
		build := &model.CustomBuild{Platform: "windows", Version: "1.2.3", AppName: "rustqs", CustomJson: `{"permanent_password":"build-secret"}`}
		if _, err := (&CustomBuildService{}).CreateNormalized(build); err == nil {
			t.Fatal("CustomBuild create succeeded without encryption key")
		}
		assertRowCount(t, db, &model.CustomBuild{}, 0)
	})

	t.Run("custom build update", func(t *testing.T) {
		db := newSecretPersistenceDB(t, &model.CustomBuild{})
		build := &model.CustomBuild{Platform: "windows", Version: "1.2.3", AppName: "rustqs", CustomJson: `{"enable_audio":true}`}
		if _, err := (&CustomBuildService{}).CreateNormalized(build); err != nil {
			t.Fatalf("seed custom build: %v", err)
		}
		build.CustomJson = `{"permanent_password":"build-secret"}`
		if err := (&CustomBuildService{}).UpdateValidated(build); err == nil {
			t.Fatal("CustomBuild update succeeded without encryption key")
		}
		assertRowCount(t, db, &model.CustomBuild{}, 1)
		if raw := rawCustomJSON(t, db, "custom_builds", build.Id); raw != `{"enable_audio":true}` {
			t.Fatalf("failed custom build update changed stored JSON to %q", raw)
		}
	})

	t.Run("custom preset create", func(t *testing.T) {
		db := newSecretPersistenceDB(t, &model.CustomPreset{})
		preset := &model.CustomPreset{UserId: 1, Name: "secret", Platform: "windows", Version: "1.2.3", AppName: "rustqs", CustomJson: `{"permanent_password":"preset-secret"}`}
		if err := (&CustomPresetService{}).Create(preset); err == nil {
			t.Fatal("CustomPreset create succeeded without encryption key")
		}
		assertRowCount(t, db, &model.CustomPreset{}, 0)
	})

	t.Run("custom preset update", func(t *testing.T) {
		db := newSecretPersistenceDB(t, &model.CustomPreset{})
		preset := &model.CustomPreset{UserId: 1, Name: "secret", Platform: "windows", Version: "1.2.3", AppName: "rustqs", CustomJson: `{}`}
		if err := (&CustomPresetService{}).Create(preset); err != nil {
			t.Fatalf("seed custom preset: %v", err)
		}
		preset.CustomJson = `{"permanent_password":"preset-secret"}`
		if err := (&CustomPresetService{}).Update(preset); err == nil {
			t.Fatal("CustomPreset update succeeded without encryption key")
		}
		assertRowCount(t, db, &model.CustomPreset{}, 1)
		if raw := rawCustomJSON(t, db, "custom_presets", preset.Id); raw != `{}` {
			t.Fatalf("failed custom preset update changed stored JSON to %q", raw)
		}
	})
}

func TestSecretBearingPersistenceEncryptsAtRestAndReadsPlaintext(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "persistence-key")
	db := newSecretPersistenceDB(t, &model.GithubBuildConfig{}, &model.CustomBuild{}, &model.CustomPreset{})

	config := &model.GithubBuildConfig{Repo: "owner/repo", Token: "github_pat_secret", PayloadKey: "payload-secret"}
	if err := db.Create(config).Error; err != nil {
		t.Fatalf("create GitHub config: %v", err)
	}
	var rawConfig struct {
		Token      string
		PayloadKey string
	}
	if err := db.Table("github_build_configs").Select("token, payload_key").Where("id = ?", config.Id).Scan(&rawConfig).Error; err != nil {
		t.Fatalf("read raw config: %v", err)
	}
	if !utils.IsEncryptedSecret(rawConfig.Token) || !utils.IsEncryptedSecret(rawConfig.PayloadKey) {
		t.Fatalf("GitHub secrets were not encrypted at rest: %#v", rawConfig)
	}
	var loadedConfig model.GithubBuildConfig
	if err := db.First(&loadedConfig, config.Id).Error; err != nil {
		t.Fatalf("read GitHub config: %v", err)
	}
	if loadedConfig.Token != "github_pat_secret" || loadedConfig.PayloadKey != "payload-secret" {
		t.Fatalf("decrypted GitHub config = %#v", loadedConfig)
	}

	build := &model.CustomBuild{
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{"permanent_password":"builder-secret","enable_audio":true}`,
	}
	if _, err := (&CustomBuildService{}).CreateNormalized(build); err != nil {
		t.Fatalf("create custom build: %v", err)
	}
	rawBuild := rawCustomJSON(t, db, "custom_builds", build.Id)
	if !utils.IsEncryptedSecret(rawBuild) || strings.Contains(rawBuild, "builder-secret") {
		t.Fatalf("custom build secret was not protected at rest: %q", rawBuild)
	}
	loadedBuild, err := (&CustomBuildService{}).Info(build.Id)
	if err != nil {
		t.Fatalf("read custom build: %v", err)
	}
	if got := loadedBuild.CustomJson; got != build.CustomJson {
		t.Fatalf("custom build read = %q, want %q", got, build.CustomJson)
	}

	preset := &model.CustomPreset{
		UserId:     7,
		Name:       "secret-preset",
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{"permanent_password":"preset-secret"}`,
	}
	if err := (&CustomPresetService{}).Create(preset); err != nil {
		t.Fatalf("create custom preset: %v", err)
	}
	rawPreset := rawCustomJSON(t, db, "custom_presets", preset.Id)
	if !utils.IsEncryptedSecret(rawPreset) || strings.Contains(rawPreset, "preset-secret") {
		t.Fatalf("custom preset secret was not protected at rest: %q", rawPreset)
	}
	loadedPreset, err := (&CustomPresetService{}).Info(preset.Id)
	if err != nil {
		t.Fatalf("read custom preset: %v", err)
	}
	if got := loadedPreset.CustomJson; got != preset.CustomJson {
		t.Fatalf("custom preset read = %q, want %q", got, preset.CustomJson)
	}
}

func TestLegacyCustomSecretReadsAndResavesAsCiphertext(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	db := newSecretPersistenceDB(t, &model.CustomBuild{}, &model.CustomPreset{})
	legacyJSON := `{"permanent_password":"legacy-secret","enable_audio":true}`

	legacyBuild := &model.CustomBuild{Platform: "windows", Version: "1.2.3", AppName: "rustqs", CustomJson: legacyJSON}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(legacyBuild).Error; err != nil {
		t.Fatalf("seed legacy custom build: %v", err)
	}
	loadedBuild, err := (&CustomBuildService{}).Info(legacyBuild.Id)
	if err != nil {
		t.Fatalf("read legacy custom build: %v", err)
	}
	if loadedBuild.CustomJson != legacyJSON {
		t.Fatalf("legacy custom build read = %q", loadedBuild.CustomJson)
	}
	loadedBuild.Name = "migrated"
	if err := (&CustomBuildService{}).UpdateValidated(loadedBuild); err == nil {
		t.Fatal("legacy custom build re-save without key succeeded")
	}
	if raw := rawCustomJSON(t, db, "custom_builds", legacyBuild.Id); raw != legacyJSON {
		t.Fatalf("failed legacy update changed raw custom build to %q", raw)
	}

	legacyPreset := &model.CustomPreset{
		UserId:     7,
		Name:       "legacy-preset",
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: legacyJSON,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(legacyPreset).Error; err != nil {
		t.Fatalf("seed legacy custom preset: %v", err)
	}
	loadedPreset, err := (&CustomPresetService{}).Info(legacyPreset.Id)
	if err != nil {
		t.Fatalf("read legacy custom preset: %v", err)
	}
	if got := loadedPreset.CustomJson; got != legacyJSON {
		t.Fatalf("legacy custom preset read = %q", got)
	}

	t.Setenv(utils.SecretEncryptionKeyEnv, "migration-key")
	loadedBuild, err = (&CustomBuildService{}).Info(legacyBuild.Id)
	if err != nil {
		t.Fatalf("reread legacy custom build: %v", err)
	}
	if err := (&CustomBuildService{}).UpdateValidated(loadedBuild); err != nil {
		t.Fatalf("legacy custom build re-save with key: %v", err)
	}
	if raw := rawCustomJSON(t, db, "custom_builds", legacyBuild.Id); !utils.IsEncryptedSecret(raw) {
		t.Fatalf("migrated custom build is not encrypted: %q", raw)
	}
	loadedPreset, err = (&CustomPresetService{}).Info(legacyPreset.Id)
	if err != nil {
		t.Fatalf("reread legacy custom preset: %v", err)
	}
	if err := (&CustomPresetService{}).Update(loadedPreset); err != nil {
		t.Fatalf("legacy custom preset re-save with key: %v", err)
	}
	if raw := rawCustomJSON(t, db, "custom_presets", legacyPreset.Id); !utils.IsEncryptedSecret(raw) {
		t.Fatalf("migrated custom preset is not encrypted: %q", raw)
	}
}

func TestLegacyGithubSecretsReadAndResaveAsCiphertext(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	db := newSecretPersistenceDB(t, &model.GithubBuildConfig{})
	legacy := &model.GithubBuildConfig{
		IdModel:    model.IdModel{Id: 1},
		Repo:       "owner/repo",
		Token:      "legacy-pat",
		PayloadKey: "legacy-payload-key",
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(legacy).Error; err != nil {
		t.Fatalf("seed legacy GitHub config: %v", err)
	}
	var loaded model.GithubBuildConfig
	if err := db.First(&loaded, legacy.Id).Error; err != nil {
		t.Fatalf("read legacy GitHub config: %v", err)
	}
	if loaded.Token != legacy.Token || loaded.PayloadKey != legacy.PayloadKey {
		t.Fatalf("legacy GitHub config read = %#v", loaded)
	}
	t.Setenv(utils.SecretEncryptionKeyEnv, "github-migration-key")
	if err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{Repo: loaded.Repo}); err != nil {
		t.Fatalf("re-save legacy GitHub config: %v", err)
	}
	var raw struct {
		Token      string
		PayloadKey string
	}
	if err := db.Table("github_build_configs").Select("token, payload_key").Where("id = ?", legacy.Id).Scan(&raw).Error; err != nil {
		t.Fatalf("read migrated raw GitHub config: %v", err)
	}
	if !utils.IsEncryptedSecret(raw.Token) || !utils.IsEncryptedSecret(raw.PayloadKey) {
		t.Fatalf("legacy GitHub secrets were not migrated on re-save: %#v", raw)
	}
}

func TestNonSecretCustomRecordsRemainAllowedWithoutKey(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	db := newSecretPersistenceDB(t, &model.CustomBuild{}, &model.CustomPreset{})
	build := &model.CustomBuild{Platform: "windows", Version: "1.2.3", AppName: "rustqs", CustomJson: `{"enable_audio":true}`}
	if _, err := (&CustomBuildService{}).CreateNormalized(build); err != nil {
		t.Fatalf("non-secret custom build rejected without key: %v", err)
	}
	if raw := rawCustomJSON(t, db, "custom_builds", build.Id); raw != build.CustomJson {
		t.Fatalf("non-secret custom build raw JSON = %q, want %q", raw, build.CustomJson)
	}

	preset := &model.CustomPreset{UserId: 1, Name: "plain", Platform: "windows", Version: "1.2.3", AppName: "rustqs", CustomJson: `{}`}
	if err := (&CustomPresetService{}).Create(preset); err != nil {
		t.Fatalf("empty non-secret preset rejected without key: %v", err)
	}
}

func TestDirectCustomModelSaveRejectsUnknownNeutralFields(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	db := newSecretPersistenceDB(t, &model.CustomBuild{}, &model.CustomPreset{})
	for _, field := range []string{"cookie", "jwt", "signing_key"} {
		t.Run(field+" build", func(t *testing.T) {
			err := db.Create(&model.CustomBuild{
				Platform:   "windows",
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: `{"` + field + `":"must-not-be-plaintext"}`,
			}).Error
			if err == nil || !strings.Contains(err.Error(), "unsupported custom field") {
				t.Fatalf("direct build save error = %v, want unsupported custom field rejection", err)
			}
		})
		t.Run(field+" preset", func(t *testing.T) {
			err := db.Create(&model.CustomPreset{
				Platform:   "windows",
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: `{"` + field + `":"must-not-be-plaintext"}`,
			}).Error
			if err == nil || !strings.Contains(err.Error(), "unsupported custom field") {
				t.Fatalf("direct preset save error = %v, want unsupported custom field rejection", err)
			}
		})
	}
}

func TestDirectCustomModelSaveAcceptsCanonicalPublicFields(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	db := newSecretPersistenceDB(t, &model.CustomBuild{}, &model.CustomPreset{})
	build := &model.CustomBuild{
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{"server_ip":"id.example:21116","enable_audio":true}`,
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("direct canonical build save: %v", err)
	}
	if raw := rawCustomJSON(t, db, "custom_builds", build.Id); raw != build.CustomJson {
		t.Fatalf("canonical build JSON = %q, want %q", raw, build.CustomJson)
	}
	preset := &model.CustomPreset{
		Platform:   "android",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{"theme":"dark","enable_audio":false}`,
	}
	if err := db.Create(preset).Error; err != nil {
		t.Fatalf("direct canonical preset save: %v", err)
	}
	if raw := rawCustomJSON(t, db, "custom_presets", preset.Id); raw != preset.CustomJson {
		t.Fatalf("canonical preset JSON = %q, want %q", raw, preset.CustomJson)
	}
}

func TestCustomReadErrorsAreReturnedAndRowsAreNotPartiallyLoaded(t *testing.T) {
	for _, test := range []struct {
		name  string
		seed  func(*gorm.DB) error
		check func(*testing.T)
	}{
		{
			name: "custom build missing key",
			seed: func(db *gorm.DB) error {
				return db.Session(&gorm.Session{SkipHooks: true}).Create(&model.CustomBuild{
					Platform: "windows", Version: "1.2.3", CustomJson: "enc:v1:not-readable",
				}).Error
			},
			check: func(t *testing.T) {
				buildService := &CustomBuildService{}
				if got, err := buildService.Info(1); err == nil || got != nil {
					t.Fatalf("CustomBuild.Info() = %#v, %v; want nil and decryption error", got, err)
				}
				if got, err := buildService.List(1, 10); err == nil || got != nil {
					t.Fatalf("CustomBuild.List() = %#v, %v; want nil and decryption error", got, err)
				}
			},
		},
		{
			name: "custom preset missing key",
			seed: func(db *gorm.DB) error {
				return db.Session(&gorm.Session{SkipHooks: true}).Create(&model.CustomPreset{
					UserId: 7, Name: "broken", Platform: "windows", Version: "1.2.3", CustomJson: "enc:v1:not-readable",
				}).Error
			},
			check: func(t *testing.T) {
				presetService := &CustomPresetService{}
				if got, err := presetService.Info(1); err == nil || got != nil {
					t.Fatalf("CustomPreset.Info() = %#v, %v; want nil and decryption error", got, err)
				}
				if got, err := presetService.List(1, 10); err == nil || got != nil {
					t.Fatalf("CustomPreset.List() = %#v, %v; want nil and decryption error", got, err)
				}
				if got, err := presetService.ListByUser(1, 10, 7); err == nil || got != nil {
					t.Fatalf("CustomPreset.ListByUser() = %#v, %v; want nil and decryption error", got, err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(utils.SecretEncryptionKeyEnv, "")
			db := newSecretPersistenceDB(t, &model.CustomBuild{}, &model.CustomPreset{})
			if err := test.seed(db); err != nil {
				t.Fatalf("seed broken row: %v", err)
			}
			test.check(t)
		})
	}
}

func TestCustomReadMalformedCiphertextReturnsError(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "malformed-read-key")
	db := newSecretPersistenceDB(t, &model.CustomBuild{}, &model.CustomPreset{})
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&model.CustomBuild{
		Platform: "windows", Version: "1.2.3", CustomJson: "enc:v1:not-readable",
	}).Error; err != nil {
		t.Fatalf("seed malformed build: %v", err)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&model.CustomPreset{
		UserId: 7, Name: "malformed", Platform: "windows", Version: "1.2.3", CustomJson: "enc:v1:not-readable",
	}).Error; err != nil {
		t.Fatalf("seed malformed preset: %v", err)
	}
	if got, err := (&CustomBuildService{}).Info(1); err == nil || got != nil {
		t.Fatalf("CustomBuild.Info() = %#v, %v; want nil and malformed-ciphertext error", got, err)
	}
	if got, err := (&CustomBuildService{}).List(1, 10); err == nil || got != nil {
		t.Fatalf("CustomBuild.List() = %#v, %v; want nil and malformed-ciphertext error", got, err)
	}
	if got, err := (&CustomPresetService{}).Info(1); err == nil || got != nil {
		t.Fatalf("CustomPreset.Info() = %#v, %v; want nil and malformed-ciphertext error", got, err)
	}
	if got, err := (&CustomPresetService{}).ListByUser(1, 10, 7); err == nil || got != nil {
		t.Fatalf("CustomPreset.ListByUser() = %#v, %v; want nil and malformed-ciphertext error", got, err)
	}
}

func newSecretPersistenceDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate secret persistence models: %v", err)
	}
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	return db
}

func rawCustomJSON(t *testing.T, db *gorm.DB, table string, id uint) string {
	t.Helper()
	var row struct {
		CustomJSON string `gorm:"column:custom_json"`
	}
	if err := db.Table(table).Select("custom_json").Where("id = ?", id).Scan(&row).Error; err != nil {
		t.Fatalf("read raw custom JSON: %v", err)
	}
	return row.CustomJSON
}

func assertRowCount(t *testing.T, db *gorm.DB, value any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(value).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != want {
		t.Fatalf("row count = %d, want %d", count, want)
	}
}
