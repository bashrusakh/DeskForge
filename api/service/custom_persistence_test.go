package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCustomPersistenceMethodsRejectUnsupportedPlatformBeforeDB(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "build create",
			call: func() error {
				return (&CustomBuildService{}).Create(&model.CustomBuild{Platform: "macos"})
			},
		},
		{
			name: "preset create",
			call: func() error {
				return (&CustomPresetService{}).Create(&model.CustomPreset{Platform: "macos"})
			},
		},
		{
			name: "preset update",
			call: func() error {
				return (&CustomPresetService{}).Update(&model.CustomPreset{Platform: "macos"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil {
				t.Fatal("error = nil, want unsupported platform validation error")
			}
			if !IsClientValidationError(err) {
				t.Fatalf("error type = %T, want ClientValidationError", err)
			}
			if got := err.Error(); got != `platform has unsupported value "macos"` {
				t.Errorf("error = %q, want unsupported platform message", got)
			}
		})
	}
}

func TestCustomBuildCreateCanonicalizesBeforePersistence(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Name:       "typed-build",
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "deskforge",
		CustomJson: `{"server_ip":"id.example:21116","key":"public-key","relay_server":"relay.example:21117","enable_audio":false}`,
	}
	normalized, err := (&CustomBuildService{}).CreateNormalized(build)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var persisted map[string]any
	if err := json.Unmarshal([]byte(build.CustomJson), &persisted); err != nil {
		t.Fatalf("canonical custom_json is invalid: %v", err)
	}
	assertCanonicalPersistedJSON(t, persisted)
	if normalized.DispatchParams["server"] != "id.example:21116" || persisted["relay_server"] != "relay.example:21117" {
		t.Fatalf("endpoint literals were not preserved: persisted=%#v dispatch=%#v", persisted, normalized.DispatchParams)
	}
	if persisted["enable_audio"] != false {
		t.Fatalf("explicit false was not preserved: %#v", persisted)
	}

	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read persisted build: %v", err)
	}
	if stored.CustomJson != build.CustomJson {
		t.Fatalf("stored custom_json = %q, returned = %q", stored.CustomJson, build.CustomJson)
	}
}

func TestCustomPersistenceRejectsInvalidPayloadBeforeWrite(t *testing.T) {
	db := newCustomPersistenceDB(t)
	var before int64
	if err := db.Model(&model.CustomBuild{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{
		`{"custom_txt":"raw"}`,
		`{"permissions_type":"not-supported"}`,
		`{"enable_audio":"false"}`,
	} {
		buildErr := (&CustomBuildService{}).Create(&model.CustomBuild{
			Name:       "invalid-build",
			Platform:   "linux",
			Version:    "1.2.3",
			CustomJson: payload,
		})
		if buildErr == nil || !IsClientValidationError(buildErr) {
			t.Fatalf("build Create(%s) error = %v, want ClientValidationError", payload, buildErr)
		}
	}
	var after int64
	if err := db.Model(&model.CustomBuild{}).Count(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("invalid build changed row count from %d to %d", before, after)
	}

	presetErr := (&CustomPresetService{}).Create(&model.CustomPreset{
		UserId:     7,
		Name:       "invalid-preset",
		Platform:   "linux",
		Version:    "1.2.3",
		CustomJson: `{"permissions_type":"not-supported"}`,
	})
	if presetErr == nil || !IsClientValidationError(presetErr) {
		t.Fatalf("preset Create() error = %v, want ClientValidationError", presetErr)
	}
	var presetCount int64
	if err := db.Model(&model.CustomPreset{}).Count(&presetCount).Error; err != nil {
		t.Fatal(err)
	}
	if presetCount != 0 {
		t.Fatalf("invalid preset changed row count to %d", presetCount)
	}
}

func TestDirectCustomSavesRequireExactCanonicalTypedFields(t *testing.T) {
	db := newCustomPersistenceDB(t)
	invalidPayloads := []string{
		`{"server-ip":"id.example:21116"}`,
		`{"enable-audio":true}`,
		`{"server_ip":{"value":"id.example:21116"}}`,
		`{"enable_audio":"false"}`,
		`{"enable_audio":null}`,
		`{"future_field":"value"}`,
		`{"app_name":"internal-record-field"}`,
	}
	for index, payload := range invalidPayloads {
		t.Run(fmt.Sprintf("build-%d", index), func(t *testing.T) {
			err := (&CustomBuildService{}).Create(&model.CustomBuild{
				Name:       "invalid-build",
				Platform:   "windows",
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: payload,
			})
			if err == nil || !IsClientValidationError(err) {
				t.Fatalf("Create(%s) error = %v, want ClientValidationError", payload, err)
			}
		})
		t.Run(fmt.Sprintf("preset-%d", index), func(t *testing.T) {
			err := (&CustomPresetService{}).Create(&model.CustomPreset{
				UserId:     uint(index + 1),
				Name:       fmt.Sprintf("invalid-preset-%d", index),
				Platform:   "windows",
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: payload,
			})
			if err == nil || !IsClientValidationError(err) {
				t.Fatalf("Create(%s) error = %v, want ClientValidationError", payload, err)
			}
		})
	}

	build := &model.CustomBuild{
		Name:       "valid-build",
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{"server_ip":"id.example:21116","enable_audio":false,"company_name":"DeskForge"}`,
	}
	if err := (&CustomBuildService{}).Create(build); err != nil {
		t.Fatalf("valid build Create() error = %v", err)
	}
	preset := &model.CustomPreset{
		UserId:     100,
		Name:       "valid-preset",
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{"relay_server":"relay.example:21117","enable_terminal":true}`,
	}
	if err := (&CustomPresetService{}).Create(preset); err != nil {
		t.Fatalf("valid preset Create() error = %v", err)
	}
	var buildCount, presetCount int64
	if err := db.Model(&model.CustomBuild{}).Count(&buildCount).Error; err != nil {
		t.Fatalf("count valid builds: %v", err)
	}
	if err := db.Model(&model.CustomPreset{}).Count(&presetCount).Error; err != nil {
		t.Fatalf("count valid presets: %v", err)
	}
	if buildCount != 1 || presetCount != 1 {
		t.Fatalf("valid direct saves counts = builds %d, presets %d; want one each", buildCount, presetCount)
	}
}

func TestCustomPresetCreateUpdateCanonicalizeAndPreserveUpsert(t *testing.T) {
	db := newCustomPersistenceDB(t)
	service := &CustomPresetService{}
	preset := &model.CustomPreset{
		UserId:     9,
		Name:       "shared-name",
		Platform:   "android",
		Version:    "1.0.0",
		AppName:    "rustqs",
		CustomJson: `{"android_app_id":"com.example.rustqs","enable_audio":false}`,
	}
	if err := service.Create(preset); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	firstID := preset.Id
	var created map[string]any
	if err := json.Unmarshal([]byte(preset.CustomJson), &created); err != nil {
		t.Fatal(err)
	}
	assertCanonicalPersistedJSON(t, created)

	replacement := &model.CustomPreset{
		UserId:     9,
		Name:       "shared-name",
		Platform:   "android",
		Version:    "2.0.0",
		AppName:    "rustqs",
		CustomJson: `{"android_app_id":"com.example.client","relay_server":"relay.example:21117"}`,
	}
	if err := service.Create(replacement); err != nil {
		t.Fatalf("upsert Create() error = %v", err)
	}
	if replacement.Id != firstID {
		t.Fatalf("upsert id = %d, want existing id %d", replacement.Id, firstID)
	}
	var count int64
	if err := db.Model(&model.CustomPreset{}).Where("user_id = ? AND name = ?", 9, "shared-name").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("upsert row count = %d, want 1", count)
	}

	replacement.CustomJson = `{"android_app_id":"com.example.client"}`
	if err := service.Update(replacement); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	var stored model.CustomPreset
	if err := db.First(&stored, firstID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CustomJson != replacement.CustomJson || stored.AppName != "rustqs" {
		t.Fatalf("valid Android custom_json was not saved without clearing the required output name: %#v", stored)
	}
	replacement.CustomJson = `{"custom_txt":"raw"}`
	if err := service.Update(replacement); err == nil || !IsClientValidationError(err) {
		t.Fatalf("invalid preset Update() error = %v, want ClientValidationError", err)
	}
	var unchanged model.CustomPreset
	if err := db.First(&unchanged, firstID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.CustomJson != `{"android_app_id":"com.example.client"}` {
		t.Fatalf("invalid preset update changed persisted custom_json to %q", unchanged.CustomJson)
	}
}

func TestCustomPresetRedactedPasswordUpdateSemantics(t *testing.T) {
	tests := []struct {
		name          string
		incomingJSON  string
		preserve      bool
		seedExisting  bool
		useCreate     bool
		wantPassword  string
		wantEncrypted bool
	}{
		{
			name:          "preserve redacted password",
			incomingJSON:  `{"enable_audio":false,"hide_cm":false,"permanent_password":""}`,
			preserve:      true,
			seedExisting:  true,
			useCreate:     true,
			wantPassword:  "old-password",
			wantEncrypted: true,
		},
		{
			name:          "preserve redacted hide cm password",
			incomingJSON:  `{"enable_audio":false,"hide_cm":true,"permanent_password":""}`,
			preserve:      true,
			seedExisting:  true,
			useCreate:     true,
			wantPassword:  "old-password",
			wantEncrypted: true,
		},
		{
			name:          "replace with non-empty password",
			incomingJSON:  `{"enable_audio":false,"hide_cm":false,"permanent_password":"new-password"}`,
			preserve:      true,
			seedExisting:  true,
			wantPassword:  "new-password",
			wantEncrypted: true,
		},
		{
			name:          "direct clear",
			incomingJSON:  `{"enable_audio":false,"hide_cm":false,"permanent_password":""}`,
			preserve:      false,
			seedExisting:  true,
			wantPassword:  "",
			wantEncrypted: false,
		},
		{
			name:          "new preset",
			incomingJSON:  `{"enable_audio":false,"hide_cm":false,"permanent_password":""}`,
			preserve:      true,
			seedExisting:  false,
			wantPassword:  "",
			wantEncrypted: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(utils.SecretEncryptionKeyEnv, "custom-preset-test-key")
			db := newCustomPersistenceDB(t)
			presetService := &CustomPresetService{}
			var preset *model.CustomPreset
			if test.seedExisting {
				preset = &model.CustomPreset{
					UserId:     7,
					Name:       "redacted-preset",
					Platform:   "windows",
					Version:    "1.2.3",
					AppName:    "rustqs",
					CustomJson: `{"enable_audio":true,"hide_cm":false,"permanent_password":"old-password"}`,
				}
				if err := presetService.Create(preset); err != nil {
					t.Fatalf("seed Create() error = %v", err)
				}
				if test.useCreate {
					preset = &model.CustomPreset{
						UserId:                    7,
						Name:                      "redacted-preset",
						Platform:                  "windows",
						Version:                   "1.2.3",
						AppName:                   "rustqs",
						CustomJson:                test.incomingJSON,
						PreservePermanentPassword: test.preserve,
					}
					if err := presetService.Create(preset); err != nil {
						t.Fatalf("upsert Create() error = %v", err)
					}
				} else {
					preset.CustomJson = test.incomingJSON
					preset.PreservePermanentPassword = test.preserve
					if err := presetService.Update(preset); err != nil {
						t.Fatalf("Update() error = %v", err)
					}
				}
			} else {
				preset = &model.CustomPreset{
					UserId:                    7,
					Name:                      "new-preset",
					Platform:                  "windows",
					Version:                   "1.2.3",
					AppName:                   "rustqs",
					CustomJson:                test.incomingJSON,
					PreservePermanentPassword: test.preserve,
				}
				if err := presetService.Create(preset); err != nil {
					t.Fatalf("Create() error = %v", err)
				}
			}

			loaded, err := presetService.Info(preset.Id)
			if err != nil {
				t.Fatalf("Info() error = %v", err)
			}
			var fields map[string]any
			if err := json.Unmarshal([]byte(loaded.CustomJson), &fields); err != nil {
				t.Fatalf("stored custom_json is invalid: %v", err)
			}
			password, _ := fields["permanent_password"].(string)
			if password != test.wantPassword {
				t.Fatalf("permanent_password = %q, want %q", password, test.wantPassword)
			}
			if raw := rawCustomJSON(t, db, "custom_presets", preset.Id); utils.IsEncryptedSecret(raw) != test.wantEncrypted {
				t.Fatalf("encrypted-at-rest = %v for raw custom_json %q, want %v", utils.IsEncryptedSecret(raw), raw, test.wantEncrypted)
			}
		})
	}
}

func TestCustomPresetPreservePasswordRequiresExistingStoredPassword(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "custom-preset-test-key")
	newCustomPersistenceDB(t)
	presetService := &CustomPresetService{}

	preset := &model.CustomPreset{
		UserId:     7,
		Name:       "hide-cm-preset",
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{"hide_cm":true,"permanent_password":"old-password"}`,
	}
	if err := presetService.Create(preset); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	preset.CustomJson = `{"hide_cm":true,"permanent_password":""}`
	preset.PreservePermanentPassword = true
	if err := presetService.Update(preset); err != nil {
		t.Fatalf("preserving existing password on hide_cm update: %v", err)
	}

	newPreset := &model.CustomPreset{
		UserId:                    7,
		Name:                      "new-hide-cm-preset",
		Platform:                  "windows",
		Version:                   "1.2.3",
		AppName:                   "rustqs",
		CustomJson:                `{"hide_cm":true,"permanent_password":""}`,
		PreservePermanentPassword: true,
	}
	if err := presetService.Create(newPreset); err == nil || !IsClientValidationError(err) {
		t.Fatalf("new hide_cm preset error = %v, want ClientValidationError", err)
	}
}

func TestCustomPresetAndroidAppIDRequiredForCreateAndUpdate(t *testing.T) {
	tests := []struct {
		name       string
		customJSON string
		wantErr    bool
	}{
		{name: "missing", customJSON: `{}`, wantErr: true},
		{name: "invalid", customJSON: `{"android_app_id":"Com.Example.Client"}`, wantErr: true},
		{name: "valid", customJSON: `{"android_app_id":"com.example.client"}`},
	}

	for _, test := range tests {
		t.Run("create/"+test.name, func(t *testing.T) {
			db := newCustomPersistenceDB(t)
			preset := &model.CustomPreset{
				UserId:     31,
				Name:       "android-create-" + test.name,
				Platform:   string(PlatformAndroid),
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: test.customJSON,
			}
			err := (&CustomPresetService{}).Create(preset)
			if (err != nil) != test.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr && !IsClientValidationError(err) {
				t.Fatalf("Create() error type = %T, want ClientValidationError", err)
			}
			var count int64
			if err := db.Model(&model.CustomPreset{}).Where("name = ?", preset.Name).Count(&count).Error; err != nil {
				t.Fatalf("count presets: %v", err)
			}
			if count != map[bool]int64{true: 0, false: 1}[test.wantErr] {
				t.Fatalf("preset count = %d, want %d", count, map[bool]int64{true: 0, false: 1}[test.wantErr])
			}
		})

		t.Run("update/"+test.name, func(t *testing.T) {
			db := newCustomPersistenceDB(t)
			preset := &model.CustomPreset{
				UserId:     32,
				Name:       "android-update-" + test.name,
				Platform:   string(PlatformAndroid),
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: `{"android_app_id":"com.example.original"}`,
			}
			if err := (&CustomPresetService{}).Create(preset); err != nil {
				t.Fatalf("seed Create() error = %v", err)
			}
			preset.CustomJson = test.customJSON
			err := (&CustomPresetService{}).Update(preset)
			if (err != nil) != test.wantErr {
				t.Fatalf("Update() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr && !IsClientValidationError(err) {
				t.Fatalf("Update() error type = %T, want ClientValidationError", err)
			}
			var stored model.CustomPreset
			if err := db.First(&stored, preset.Id).Error; err != nil {
				t.Fatalf("read preset: %v", err)
			}
			wantJSON := test.customJSON
			if test.wantErr {
				wantJSON = `{"android_app_id":"com.example.original"}`
			}
			if stored.CustomJson != wantJSON {
				t.Fatalf("stored custom_json = %q, want %q", stored.CustomJson, wantJSON)
			}
		})
	}
}

func TestCustomPresetLegacyAndroidReadAndMigrationCompatibility(t *testing.T) {
	db := newCustomPersistenceDB(t)
	legacyJSON := `{"enable_audio":true}`
	legacy := &model.CustomPreset{
		UserId:     41,
		Name:       "legacy-android",
		Platform:   string(PlatformAndroid),
		Version:    "1.0.0",
		AppName:    "rustqs",
		CustomJson: legacyJSON,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(legacy).Error; err != nil {
		t.Fatalf("seed legacy preset: %v", err)
	}

	service := &CustomPresetService{}
	loaded, err := service.Info(legacy.Id)
	if err != nil {
		t.Fatalf("read legacy preset: %v", err)
	}
	if loaded.CustomJson != legacyJSON {
		t.Fatalf("legacy read custom_json = %q, want %q", loaded.CustomJson, legacyJSON)
	}
	list, err := service.ListByUser(1, 10, legacy.UserId)
	if err != nil {
		t.Fatalf("list legacy preset: %v", err)
	}
	if len(list.CustomPresets) != 1 || list.CustomPresets[0].CustomJson != legacyJSON {
		t.Fatalf("legacy list = %#v, want one unchanged preset", list.CustomPresets)
	}

	if _, err := NormalizeCustomBuildJSON(legacyJSON, BuildRecordContext{
		Platform:                 legacy.Platform,
		AppName:                  legacy.AppName,
		Version:                  legacy.Version,
		AllowMissingAndroidAppID: true,
	}); err != nil {
		t.Fatalf("legacy migration normalization rejected missing Android identity: %v", err)
	}
}

func TestCustomPresetWindowsAndLinuxRemainValidWithoutAndroidAppID(t *testing.T) {
	for _, platform := range []Platform{PlatformWindows, PlatformLinux} {
		t.Run(string(platform), func(t *testing.T) {
			newCustomPersistenceDB(t)
			preset := &model.CustomPreset{
				UserId:     51,
				Name:       string(platform) + "-preset",
				Platform:   string(platform),
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: `{"enable_audio":false}`,
			}
			service := &CustomPresetService{}
			if err := service.Create(preset); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			preset.CustomJson = `{"enable_audio":true}`
			if err := service.Update(preset); err != nil {
				t.Fatalf("Update() error = %v", err)
			}
		})
	}
}

func TestCustomBuildProgressUpdateDoesNotModifyCustomJSON(t *testing.T) {
	db := newCustomPersistenceDB(t)
	legacyJSON := `{"legacy_field":"opaque","custom_txt":"legacy"}`
	build := &model.CustomBuild{
		Name:             "legacy-build",
		Platform:         "windows",
		Version:          "legacy",
		AppName:          "legacy-app",
		CustomJson:       legacyJSON,
		Status:           model.CustomBuildStatusBuilding,
		BuildLog:         "initial",
		FileSize:         42,
		GithubRunId:      99,
		GithubArtifactID: 7,
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(build).Error; err != nil {
		t.Fatalf("seed legacy build: %v", err)
	}
	recordValidPublication(t, build)
	if err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
		BuildID:            build.Id,
		ExpectedRunID:      build.GithubRunId,
		ExpectedArtifactID: build.GithubArtifactID,
		Status:             model.CustomBuildStatusDone,
		BuildLog:           "completed",
		FileSize:           0,
	}); err != nil {
		t.Fatalf("UpdateProgress() error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.CustomBuildStatusDone || stored.BuildLog != "completed" || stored.FileSize != 0 || stored.GithubRunId != 99 || stored.PublicationRecordedAt <= 0 {
		t.Fatalf("progress fields were not updated, including zero values: %#v", stored)
	}
	if stored.CustomJson != legacyJSON || stored.Platform != "windows" || stored.Version != "1.2.3" || stored.AppName != "legacy-app" {
		t.Fatalf("progress update changed user-authored fields: %#v", stored)
	}
}

func TestCustomBuildDeleteRemovesRowEvenWhenArtifactCleanupFails(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusDone}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	previousRemove := removeBuildOutputDir
	removeBuildOutputDir = func(string) error { return errors.New("simulated cleanup failure") }
	t.Cleanup(func() { removeBuildOutputDir = previousRemove })

	err := (&CustomBuildService{}).Delete(build)
	var cleanupPending *CustomBuildCleanupPending
	if !errors.As(err, &cleanupPending) {
		t.Fatalf("Delete() error = %T %v, want cleanup-pending status", err, err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("read build after successful DB delete: error = %v, want record-not-found", err)
	}
	tombstone := buildOutputDeletionTombstonePath(filepath.Dir(BuildOutputDir(build.Id)), build.Id)
	if _, err := os.Stat(tombstone); err != nil {
		t.Fatalf("cleanup tombstone stat = %v, want retained marker", err)
	}
}

func TestCustomBuildDeleteCreatesAndRemovesCleanupTombstone(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusDone}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0700); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "artifact.bin"), []byte("artifact"), 0600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	tombstone := buildOutputDeletionTombstonePath(filepath.Dir(outDir), build.Id)
	callbackName := "test:check-custom-build-delete-tombstone-" + strings.ReplaceAll(t.Name(), "/", "-")
	if err := db.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if _, err := os.Stat(tombstone); err != nil {
			tx.AddError(fmt.Errorf("deletion tombstone was not created before DB delete: %w", err))
		}
	}); err != nil {
		t.Fatalf("register tombstone callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Delete().Remove(callbackName) })

	if err := (&CustomBuildService{}).Delete(build); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if _, err := os.Stat(tombstone); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup tombstone stat = %v, want removed", err)
	}
	if _, err := os.Stat(outDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output directory stat = %v, want removed", err)
	}
}

func TestCustomBuildDeleteReturnsPendingWhenTombstoneRemovalFails(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusDone}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	previousRemove := removeBuildOutputDeletionTombstone
	removeBuildOutputDeletionTombstone = func(string) error { return errors.New("simulated tombstone removal failure") }
	t.Cleanup(func() { removeBuildOutputDeletionTombstone = previousRemove })

	err := (&CustomBuildService{}).Delete(build)
	var cleanupPending *CustomBuildCleanupPending
	if !errors.As(err, &cleanupPending) {
		t.Fatalf("Delete() error = %T %v, want cleanup-pending status", err, err)
	}
	tombstone := buildOutputDeletionTombstonePath(filepath.Dir(BuildOutputDir(build.Id)), build.Id)
	if _, err := os.Stat(tombstone); err != nil {
		t.Fatalf("cleanup tombstone stat = %v, want retained marker", err)
	}
}

func TestCustomBuildDeleteRetainsRowAndArtifactWhenDBDeleteFails(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusDone}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := BuildOutputDir(build.Id)
	artifact := filepath.Join(outDir, "rustqs.exe")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(artifact, []byte("must remain accessible"), 0600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	wantErr := errors.New("simulated database delete failure")
	callbackName := "test:fail-custom-build-delete-" + strings.ReplaceAll(t.Name(), "/", "-")
	if err := db.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		tx.AddError(wantErr)
	}); err != nil {
		t.Fatalf("register delete failure callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Delete().Remove(callbackName) })

	if err := (&CustomBuildService{}).Delete(build); !errors.Is(err, wantErr) {
		t.Fatalf("Delete() error = %v, want %v", err, wantErr)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build after failed DB delete: %v", err)
	}
	if contents, err := os.ReadFile(artifact); err != nil || string(contents) != "must remain accessible" {
		t.Fatalf("artifact after failed DB delete = %q, error = %v; want accessible artifact", contents, err)
	}
	tombstone := buildOutputDeletionTombstonePath(filepath.Dir(outDir), build.Id)
	if _, err := os.Stat(tombstone); err != nil {
		t.Fatalf("cleanup tombstone after failed DB delete = %v, want marker", err)
	}
}

func TestCustomBuildDeleteDoesNotClassifyPostDeleteDatabaseUncertaintyAsCleanupPending(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusDone}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := (&CustomBuildService{}).Delete(build); err != nil {
		var cleanupPending *CustomBuildCleanupPending
		if errors.As(err, &cleanupPending) {
			t.Fatalf("Delete() error = %T %v, cleanup-pending must only represent filesystem failures after DB delete", err, err)
		}
		t.Fatalf("Delete() error = %v, want nil after successful DB delete and filesystem cleanup", err)
	}
}

func TestCustomBuildDeleteReturnsOrdinaryErrorWhenTombstonePrecreationFails(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusDone}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	previousOutputDir := BuildOutputDir
	BuildOutputDir = func(uint) string { return filepath.Join(t.TempDir(), "invalid\x00root", "1") }
	t.Cleanup(func() { BuildOutputDir = previousOutputDir })

	err := (&CustomBuildService{}).Delete(build)
	var cleanupPending *CustomBuildCleanupPending
	if errors.As(err, &cleanupPending) {
		t.Fatalf("Delete() error = %T %v, tombstone precreation failure must remain ordinary", err, err)
	}
	if err == nil {
		t.Fatal("Delete() error = nil, want tombstone precreation failure")
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build after tombstone precreation failure: %v", err)
	}
}

func TestRecordPublishedOutputValidatesContentAndWritesMarkerOnce(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Status:              model.CustomBuildStatusExtracting,
		Platform:            "windows",
		Version:             "1.2.3",
		AppName:             "rustqs",
		BuildRef:            strings.Repeat("a", 40),
		SourceTag:           "1.2.3",
		AssetsRelease:       "offline-assets-1.2.3",
		AssetsReleaseID:     12,
		GithubRunId:         17,
		GithubProvider:      "github",
		GithubRepo:          "owner/repo",
		GithubWorkflow:      "workflow.yml",
		WorkflowSelector:    defaultWorkflowExecutionRef,
		GithubRef:           strings.Repeat("a", 40),
		GithubArtifactName:  "artifact",
		GithubArtifactID:    42,
		GithubRunUrl:        "https://api.github.com/repos/owner/repo/actions/runs/17",
		GithubHtmlUrl:       "https://github.com/owner/repo/actions/runs/17",
		AssetsReleaseAssets: string(mustTestReleaseAssetsJSON(t)),
	}
	producerManifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "published"})
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	decoyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(decoyDir, "rustqs.exe"), []byte("caller-selected"), 0600); err != nil {
		t.Fatalf("write decoy output: %v", err)
	}
	if err := (&CustomBuildService{}).RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID, producerManifest); err == nil {
		t.Fatal("RecordPublishedOutput() error = nil for missing canonical output, want validation failure")
	}

	outDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create canonical output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("published"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	service := &CustomBuildService{}
	if err := service.RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID, producerManifest); err != nil {
		t.Fatalf("RecordPublishedOutput() valid output error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read marked build: %v", err)
	}
	if stored.PublicationRecordedAt <= 0 {
		t.Fatalf("publication marker = %d, want positive timestamp", stored.PublicationRecordedAt)
	}
	if !validPublishedDigest(stored.PublishedDigest) {
		t.Fatalf("published digest = %q, want SHA-256 hex", stored.PublishedDigest)
	}
	if storedManifest, err := ProducerManifestFromStoredJSON(stored.ProducerManifestJSON); err != nil {
		t.Fatalf("stored producer manifest: %v", err)
	} else if err := ValidateProducerManifestForBuild(storedManifest, &stored); err != nil {
		t.Fatalf("stored producer manifest identity: %v", err)
	}
	marker := stored.PublicationRecordedAt
	digest := stored.PublishedDigest
	if err := service.RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID, producerManifest); err != nil {
		t.Fatalf("RecordPublishedOutput() idempotent retry error = %v", err)
	}
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read retried build: %v", err)
	}
	if stored.PublicationRecordedAt != marker {
		t.Fatalf("publication marker changed from %d to %d", marker, stored.PublicationRecordedAt)
	}
	if stored.PublishedDigest != digest {
		t.Fatalf("published digest changed from %q to %q", digest, stored.PublishedDigest)
	}
}

func TestRecordPublishedOutputRejectsPartialProvenance(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Status:           model.CustomBuildStatusExtracting,
		Platform:         "windows",
		GithubRunId:      17,
		GithubArtifactID: 42,
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("published"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	if err := (&CustomBuildService{}).RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID); err == nil {
		t.Fatal("RecordPublishedOutput() error = nil for partial provenance")
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.PublicationRecordedAt != 0 || stored.PublishedDigest != "" {
		t.Fatalf("partial provenance recorded publication proof: %#v", stored)
	}
}

func TestPublishedOutputDigestMismatchFailsClosed(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Status:              model.CustomBuildStatusExtracting,
		Platform:            "windows",
		Version:             "1.2.3",
		AppName:             "rustqs",
		BuildRef:            strings.Repeat("a", 40),
		SourceTag:           "1.2.3",
		AssetsRelease:       "offline-assets-1.2.3",
		AssetsReleaseID:     12,
		GithubRunId:         18,
		GithubProvider:      "github",
		GithubRepo:          "owner/repo",
		GithubWorkflow:      "workflow.yml",
		WorkflowSelector:    defaultWorkflowExecutionRef,
		GithubRef:           strings.Repeat("a", 40),
		GithubArtifactName:  "artifact",
		GithubArtifactID:    42,
		GithubRunUrl:        "https://api.github.com/repos/owner/repo/actions/runs/18",
		GithubHtmlUrl:       "https://github.com/owner/repo/actions/runs/18",
		AssetsReleaseAssets: string(mustTestReleaseAssetsJSON(t)),
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	path := filepath.Join(outDir, "rustqs.exe")
	if err := os.WriteFile(path, []byte("published"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	service := &CustomBuildService{}
	producerManifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "published"})
	if err := service.RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID, producerManifest); err != nil {
		t.Fatalf("RecordPublishedOutput() error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read recorded build: %v", err)
	}
	if err := os.WriteFile(path, []byte("mutated!"), 0600); err != nil {
		t.Fatalf("mutate output: %v", err)
	}
	stored.Status = model.CustomBuildStatusDone
	if _, err := ValidatePublishedOutputDigest(&stored); err == nil {
		t.Fatal("ValidatePublishedOutputDigest() error = nil after output mutation")
	}
}

func TestPublishedOutputDigestBindsImmutablePublicationIdentity(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Status:              model.CustomBuildStatusExtracting,
		Platform:            "windows",
		Version:             "1.2.3",
		AppName:             "rustqs",
		BuildRef:            strings.Repeat("a", 40),
		SourceTag:           "1.2.3",
		AssetsRelease:       "offline-assets-1.2.3",
		AssetsReleaseID:     12,
		GithubRunId:         18,
		GithubProvider:      "github",
		GithubRepo:          "owner/repo",
		GithubWorkflow:      "workflow.yml",
		WorkflowSelector:    defaultWorkflowExecutionRef,
		GithubRef:           strings.Repeat("a", 40),
		GithubArtifactName:  "artifact",
		GithubArtifactID:    42,
		GithubRunUrl:        "https://api.github.com/repos/owner/repo/actions/runs/18",
		GithubHtmlUrl:       "https://github.com/owner/repo/actions/runs/18",
		AssetsReleaseAssets: string(mustTestReleaseAssetsJSON(t)),
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("published"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	producerManifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "published"})
	if err := (&CustomBuildService{}).RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID, producerManifest); err != nil {
		t.Fatalf("record publication: %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read recorded build: %v", err)
	}
	for name, mutate := range map[string]func(*model.CustomBuild){
		"run":      func(b *model.CustomBuild) { b.GithubRunId++ },
		"artifact": func(b *model.CustomBuild) { b.GithubArtifactID++ },
		"repo":     func(b *model.CustomBuild) { b.GithubRepo = "owner/other" },
		"name":     func(b *model.CustomBuild) { b.GithubArtifactName = "other-artifact" },
		"ref":      func(b *model.CustomBuild) { b.GithubRef = strings.Repeat("b", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			mutated := stored
			mutate(&mutated)
			if _, err := ValidatePublishedOutputProof(&mutated); err == nil {
				t.Fatal("ValidatePublishedOutputProof() succeeded after immutable publication identity mutation")
			}
		})
	}
}

func TestProviderPublicationProofRequiresStoredManifestAndExactOutput(t *testing.T) {
	cases := []struct {
		name           string
		storedManifest func(*model.CustomBuild) string
		mutateOutput   func(*testing.T, string)
	}{
		{
			name: "missing stored manifest",
			storedManifest: func(*model.CustomBuild) string {
				return ""
			},
		},
		{
			name: "legacy v1 stored manifest",
			storedManifest: func(build *model.CustomBuild) string {
				encoded, err := json.Marshal(ProducerManifest{
					Schema:          ProducerManifestSchema,
					SchemaVersion:   1,
					Platform:        build.Platform,
					AppName:         build.AppName,
					OutputFilenames: []string{"rustqs.exe"},
					SourceSHA:       build.BuildRef,
					WorkflowSHA:     build.GithubRef,
					WorkflowRef:     build.WorkflowSelector,
					Version:         build.Version,
					DigestScope:     producerManifestLegacyDigestScope,
					Files:           []ProducerManifestFile{{Name: "rustqs.exe", SHA256: strings.Repeat("a", 64)}},
				})
				if err != nil {
					t.Fatalf("marshal legacy producer manifest: %v", err)
				}
				return string(encoded)
			},
		},
		{
			name: "malformed stored manifest",
			storedManifest: func(*model.CustomBuild) string {
				return `{"schema":`
			},
		},
		{
			name: "identity-mismatched stored manifest",
			storedManifest: func(build *model.CustomBuild) string {
				manifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "published"})
				manifest.SourceSHA = strings.Repeat("b", 40)
				encoded, err := manifest.StoredJSON()
				if err != nil {
					t.Fatalf("store identity-mismatched producer manifest: %v", err)
				}
				return encoded
			},
		},
		{
			name: "missing final file",
			storedManifest: func(build *model.CustomBuild) string {
				return mustStoredProducerManifestJSON(t, build, map[string]string{"rustqs.exe": "published"})
			},
			mutateOutput: func(t *testing.T, outputDir string) {
				t.Helper()
				if err := os.Remove(filepath.Join(outputDir, "rustqs.exe")); err != nil {
					t.Fatalf("remove final output: %v", err)
				}
			},
		},
		{
			name: "mutated final file",
			storedManifest: func(build *model.CustomBuild) string {
				return mustStoredProducerManifestJSON(t, build, map[string]string{"rustqs.exe": "published"})
			},
			mutateOutput: func(t *testing.T, outputDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(outputDir, "rustqs.exe"), []byte("mutated"), 0600); err != nil {
					t.Fatalf("mutate final output: %v", err)
				}
			},
		},
		{
			name: "extra final file",
			storedManifest: func(build *model.CustomBuild) string {
				return mustStoredProducerManifestJSON(t, build, map[string]string{"rustqs.exe": "published"})
			},
			mutateOutput: func(t *testing.T, outputDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(outputDir, "helper.dll"), []byte("extra"), 0600); err != nil {
					t.Fatalf("write extra final output: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newCustomPersistenceDB(t)
			build := &model.CustomBuild{
				Status:              model.CustomBuildStatusExtracting,
				Platform:            "windows",
				Version:             "1.2.3",
				AppName:             "rustqs",
				BuildRef:            strings.Repeat("a", 40),
				SourceTag:           "1.2.3",
				AssetsRelease:       "offline-assets-1.2.3",
				AssetsReleaseID:     12,
				GithubRunId:         17,
				GithubProvider:      "github",
				GithubRepo:          "owner/repo",
				GithubWorkflow:      "workflow.yml",
				WorkflowSelector:    defaultWorkflowExecutionRef,
				GithubRef:           strings.Repeat("a", 40),
				GithubArtifactName:  "artifact",
				GithubArtifactID:    42,
				GithubRunUrl:        "https://api.github.com/repos/owner/repo/actions/runs/17",
				GithubHtmlUrl:       "https://github.com/owner/repo/actions/runs/17",
				AssetsReleaseAssets: string(mustTestReleaseAssetsJSON(t)),
			}
			build.ProducerManifestJSON = tc.storedManifest(build)
			if err := db.Create(build).Error; err != nil {
				t.Fatalf("create provider build: %v", err)
			}
			outputDir := BuildOutputDir(build.Id)
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				t.Fatalf("create canonical output: %v", err)
			}
			if err := os.WriteFile(filepath.Join(outputDir, "rustqs.exe"), []byte("published"), 0600); err != nil {
				t.Fatalf("write canonical output: %v", err)
			}
			if tc.mutateOutput != nil {
				tc.mutateOutput(t, outputDir)
			}

			err := (&CustomBuildService{}).RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID)
			var persistenceErr *BuildProgressPersistenceError
			if err == nil || !errors.As(err, &persistenceErr) {
				t.Fatalf("RecordPublishedOutput() error = %T %v, want typed proof rejection", err, err)
			}
			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read rejected publication: %v", err)
			}
			if stored.PublicationRecordedAt != 0 || stored.PublishedDigest != "" {
				t.Fatalf("rejected provider publication blessed marker/digest: %#v", stored)
			}

			stored.PublicationRecordedAt = 1
			stored.PublishedDigest = strings.Repeat("a", 64)
			if _, err := ValidatePublishedOutputProof(&stored); err == nil {
				t.Fatal("ValidatePublishedOutputProof() error = nil for invalid provider proof")
			}
			if err := db.Model(&model.CustomBuild{}).Where("id = ?", build.Id).Updates(map[string]any{
				"publication_recorded_at": stored.PublicationRecordedAt,
				"published_digest":        stored.PublishedDigest,
			}).Error; err != nil {
				t.Fatalf("seed invalid stored proof: %v", err)
			}
			_, recoveryErr := (&CustomBuildService{}).ValidateRecordedPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID)
			var recoveryPersistenceErr *BuildProgressPersistenceError
			if recoveryErr == nil || !errors.As(recoveryErr, &recoveryPersistenceErr) {
				t.Fatalf("ValidateRecordedPublishedOutput() error = %T %v, want typed proof rejection", recoveryErr, recoveryErr)
			}
		})
	}
}

func TestIdentitylessPublishedOutputProofRemainsManifestOptionalAndNonPublic(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Status:   model.CustomBuildStatusDone,
		Platform: "windows",
		AppName:  "rustqs",
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create legacy build: %v", err)
	}
	outputDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("create legacy output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "rustqs.exe"), []byte("legacy"), 0600); err != nil {
		t.Fatalf("write legacy output: %v", err)
	}
	digest, err := PublishedOutputDigest(build)
	if err != nil {
		t.Fatalf("PublishedOutputDigest() legacy output: %v", err)
	}
	build.PublicationRecordedAt = 1
	build.PublishedDigest = digest
	if _, err := ValidatePublishedOutputProof(build); err != nil {
		t.Fatalf("ValidatePublishedOutputProof() legacy manifest-optional output: %v", err)
	}
	if _, _, err := ValidateCompletedPublishedOutput(build); err == nil {
		t.Fatal("ValidateCompletedPublishedOutput() accepted identity-less legacy output as public")
	}
}

func TestPublishedOutputDigestIsStableForEntryOrder(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Platform: "linux", AppName: "rustqs"}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(contents), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("z.bin", "last")
	write("a.bin", "first")
	first, err := PublishedOutputDigest(build)
	if err != nil {
		t.Fatalf("first digest: %v", err)
	}
	if err := os.RemoveAll(outDir); err != nil {
		t.Fatalf("remove output: %v", err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("recreate output directory: %v", err)
	}
	write("a.bin", "first")
	write("z.bin", "last")
	second, err := PublishedOutputDigest(build)
	if err != nil {
		t.Fatalf("second digest: %v", err)
	}
	if first != second {
		t.Fatalf("published digest changed with entry order: %q != %q", first, second)
	}
}

func TestCustomBuildProgressUpdateTreatsZeroRowsAsPersistenceError(t *testing.T) {
	newCustomPersistenceDB(t)

	err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
		BuildID:            404,
		ExpectedRunID:      404,
		ExpectedArtifactID: 1,
		Status:             model.CustomBuildStatusDone,
		BuildLog:           "completed",
		FileSize:           1,
	})
	if err == nil {
		t.Fatal("UpdateProgress() error = nil, want persistence error for missing build")
	}
	var persistenceErr *BuildProgressPersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("UpdateProgress() error = %T %v, want BuildProgressPersistenceError", err, err)
	}
}

func TestCustomBuildProgressDoneRequiresStoredArtifactIdentity(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Status:           model.CustomBuildStatusExtracting,
		Platform:         "windows",
		AppName:          "rustqs",
		GithubRunId:      909,
		GithubArtifactID: 0,
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}

	err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
		BuildID:            build.Id,
		ExpectedRunID:      build.GithubRunId,
		ExpectedArtifactID: 0,
		Status:             model.CustomBuildStatusDone,
		BuildLog:           "must not complete without artifact identity",
	})
	var persistenceErr *BuildProgressPersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("UpdateProgress() error = %T %v, want guarded persistence error", err, err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build after rejected completion: %v", err)
	}
	if stored.Status != model.CustomBuildStatusExtracting || stored.BuildLog != "" {
		t.Fatalf("rejected completion changed build: %#v", stored)
	}

	if err := db.Model(&model.CustomBuild{}).Where("id = ?", build.Id).Update("github_artifact_id", 42).Error; err != nil {
		t.Fatalf("store artifact identity: %v", err)
	}
	build.GithubArtifactID = 42
	recordValidPublication(t, build)
	err = (&CustomBuildService{}).UpdateProgress(BuildProgress{
		BuildID:            build.Id,
		ExpectedRunID:      build.GithubRunId,
		ExpectedArtifactID: 99,
		Status:             model.CustomBuildStatusDone,
		BuildLog:           "wrong artifact must not complete",
	})
	if err == nil {
		t.Fatal("UpdateProgress() with wrong artifact identity = nil, want guarded persistence error")
	}
	var persistenceErrWrongArtifact *BuildProgressPersistenceError
	if !errors.As(err, &persistenceErrWrongArtifact) {
		t.Fatalf("wrong artifact error = %T %v, want guarded persistence error", err, err)
	}
	if err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
		BuildID:            build.Id,
		ExpectedRunID:      build.GithubRunId,
		ExpectedArtifactID: 42,
		Status:             model.CustomBuildStatusDone,
		BuildLog:           "completed",
	}); err != nil {
		t.Fatalf("UpdateProgress() with stored artifact identity = %v", err)
	}
}

func TestCustomBuildProgressDoneRequiresRecordedPublication(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusExtracting, Platform: "windows", AppName: "rustqs", GithubRunId: 909, GithubArtifactID: 42}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
		BuildID:            build.Id,
		ExpectedRunID:      build.GithubRunId,
		ExpectedArtifactID: build.GithubArtifactID,
		Status:             model.CustomBuildStatusDone,
		BuildLog:           "published without marker",
	})
	var persistenceErr *BuildProgressPersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("UpdateProgress() error = %T %v, want publication marker rejection", err, err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusExtracting || stored.BuildLog != "" {
		t.Fatalf("marker rejection changed build: %#v", stored)
	}
}

func TestCustomBuildProgressBoundsBuildLog(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, Platform: "windows", AppName: "rustqs", GithubRunId: 909, GithubArtifactID: 42}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
		BuildID:       build.Id,
		ExpectedRunID: build.GithubRunId,
		Status:        model.CustomBuildStatusBuilding,
		BuildLog:      strings.Repeat("x", MaxBuildLogBytes+100),
	}); err != nil {
		t.Fatalf("UpdateProgress() error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if len(stored.BuildLog) != MaxBuildLogBytes {
		t.Fatalf("stored build log length = %d, want %d", len(stored.BuildLog), MaxBuildLogBytes)
	}
}

func TestCustomBuildProgressRejectsStaleRunUpdates(t *testing.T) {
	db := newCustomPersistenceDB(t)
	cases := []struct {
		name          string
		status        string
		storedRunID   int64
		staleRunID    int64
		attemptStatus string
		wantStatus    string
	}{
		{
			name:          "done row cannot be overwritten",
			status:        model.CustomBuildStatusDone,
			storedRunID:   101,
			staleRunID:    100,
			attemptStatus: model.CustomBuildStatusFailed,
			wantStatus:    model.CustomBuildStatusDone,
		},
		{
			name:          "failed row cannot be overwritten",
			status:        model.CustomBuildStatusFailed,
			storedRunID:   202,
			staleRunID:    201,
			attemptStatus: model.CustomBuildStatusDone,
			wantStatus:    model.CustomBuildStatusFailed,
		},
		{
			name:          "another run cannot update building row",
			status:        model.CustomBuildStatusBuilding,
			storedRunID:   303,
			staleRunID:    304,
			attemptStatus: model.CustomBuildStatusDone,
			wantStatus:    model.CustomBuildStatusBuilding,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build := &model.CustomBuild{
				Status:           tc.status,
				GithubRunId:      tc.storedRunID,
				GithubArtifactID: 1,
			}
			if err := db.Create(build).Error; err != nil {
				t.Fatalf("create build: %v", err)
			}
			err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
				BuildID:            build.Id,
				ExpectedRunID:      tc.staleRunID,
				ExpectedArtifactID: 1,
				Status:             tc.attemptStatus,
				BuildLog:           "stale update",
			})
			var persistenceErr *BuildProgressPersistenceError
			if !errors.As(err, &persistenceErr) {
				t.Fatalf("UpdateProgress() error = %T %v, want typed zero-row persistence error", err, err)
			}
			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read build: %v", err)
			}
			if stored.Status != tc.wantStatus || stored.BuildLog != "" {
				t.Fatalf("stale update changed row: status=%q log=%q", stored.Status, stored.BuildLog)
			}
		})
	}
}

func TestCustomBuildProgressSupportsDownloadLifecycleStatesWithRunGuard(t *testing.T) {
	db := newCustomPersistenceDB(t)
	for _, current := range []string{model.CustomBuildStatusDownloading, model.CustomBuildStatusExtracting} {
		t.Run(current, func(t *testing.T) {
			build := &model.CustomBuild{Status: current, Platform: "windows", AppName: "rustqs", GithubRunId: 909, GithubArtifactID: 42}
			if err := db.Create(build).Error; err != nil {
				t.Fatalf("create build: %v", err)
			}
			if current == model.CustomBuildStatusDownloading || current == model.CustomBuildStatusExtracting {
				recordValidPublication(t, build)
			}
			if err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
				BuildID: build.Id, ExpectedRunID: build.GithubRunId,
				ExpectedArtifactID: build.GithubArtifactID,
				Status:             model.CustomBuildStatusDone, BuildLog: "published",
			}); err != nil {
				t.Fatalf("UpdateProgress() lifecycle transition error = %v", err)
			}
			staleErr := (&CustomBuildService{}).UpdateProgress(BuildProgress{
				BuildID: build.Id, ExpectedRunID: build.GithubRunId - 1,
				Status: model.CustomBuildStatusFailed, BuildLog: "stale",
			})
			var persistenceErr *BuildProgressPersistenceError
			if !errors.As(staleErr, &persistenceErr) {
				t.Fatalf("stale lifecycle update error = %T %v, want guarded persistence error", staleErr, staleErr)
			}
		})
	}
}

func TestCustomBuildProgressCannotTransitionLinuxOrAndroidToDone(t *testing.T) {
	db := newCustomPersistenceDB(t)
	for _, platform := range []string{"linux", "android"} {
		t.Run(platform, func(t *testing.T) {
			build := &model.CustomBuild{Status: model.CustomBuildStatusExtracting, Platform: platform, GithubRunId: 909, GithubArtifactID: 42}
			if err := db.Create(build).Error; err != nil {
				t.Fatalf("create build: %v", err)
			}
			err := (&CustomBuildService{}).UpdateProgress(BuildProgress{
				BuildID: build.Id, ExpectedRunID: build.GithubRunId,
				ExpectedArtifactID: build.GithubArtifactID,
				Status:             model.CustomBuildStatusDone, BuildLog: "must remain non-done",
			})
			var unavailable *ProductionCapabilityUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("UpdateProgress() error = %T %v, want capability-unavailable error", err, err)
			}
			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read build: %v", err)
			}
			if stored.Status != model.CustomBuildStatusExtracting {
				t.Fatalf("status after rejected completion = %q, want extracting", stored.Status)
			}
		})
	}
}

func TestCustomBuildPendingFailureUsesExplicitUndispatchedGuard(t *testing.T) {
	db := newCustomPersistenceDB(t)
	pending := &model.CustomBuild{Status: model.CustomBuildStatusPending}
	building := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 505}
	if err := db.Create(pending).Error; err != nil {
		t.Fatalf("create pending build: %v", err)
	}
	if err := db.Create(building).Error; err != nil {
		t.Fatalf("create building build: %v", err)
	}
	if err := (&CustomBuildService{}).UpdatePendingFailure(BuildProgress{
		BuildID:  pending.Id,
		Status:   model.CustomBuildStatusFailed,
		BuildLog: "pre-dispatch failure",
	}); err != nil {
		t.Fatalf("UpdatePendingFailure() error = %v", err)
	}
	err := (&CustomBuildService{}).UpdatePendingFailure(BuildProgress{
		BuildID:  building.Id,
		Status:   model.CustomBuildStatusFailed,
		BuildLog: "must not bypass run guard",
	})
	var persistenceErr *BuildProgressPersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("building pending-failure error = %T %v, want typed zero-row persistence error", err, err)
	}
	var storedPending, storedBuilding model.CustomBuild
	if err := db.First(&storedPending, pending.Id).Error; err != nil {
		t.Fatalf("read pending build: %v", err)
	}
	if err := db.First(&storedBuilding, building.Id).Error; err != nil {
		t.Fatalf("read building build: %v", err)
	}
	if storedPending.Status != model.CustomBuildStatusFailed || storedBuilding.Status != model.CustomBuildStatusBuilding {
		t.Fatalf("pending guard results: pending=%q building=%q", storedPending.Status, storedBuilding.Status)
	}
}

func TestCustomBuildUpdateValidatedRejectsZeroIDBeforePersistence(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Platform:   "linux",
		Version:    "1.2.3",
		CustomJson: `{"enable_audio":true}`,
	}

	err := (&CustomBuildService{}).UpdateValidated(build)
	if err == nil || !IsClientValidationError(err) {
		t.Fatalf("UpdateValidated() error = %v, want ClientValidationError", err)
	}
	if got := err.Error(); got != "build id is required" {
		t.Fatalf("UpdateValidated() error = %q, want build id validation error", got)
	}
	var count int64
	if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
		t.Fatalf("count builds after rejected update: %v", err)
	}
	if count != 0 {
		t.Fatalf("zero-id update inserted %d row(s)", count)
	}
}

func TestCustomBuildUpdateValidatedAllowlistPreservesProvenance(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Name:               "validated-build",
		Platform:           "linux",
		Version:            "1.2.3",
		AppName:            "rustqs",
		CustomJson:         `{"enable_audio":true}`,
		Status:             model.CustomBuildStatusBuilding,
		FileSize:           42,
		GithubRunId:        77,
		GithubProvider:     "github",
		GithubRepo:         "owner/repo",
		GithubWorkflow:     "workflow.yml",
		GithubRef:          "ref",
		GithubArtifactName: "artifact",
		GithubArtifactID:   88,
		GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/77",
		GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/77",
		GithubSourceSha:    strings.Repeat("a", 40),
	}
	if _, err := (&CustomBuildService{}).CreateNormalized(build); err != nil {
		t.Fatalf("CreateNormalized() error = %v", err)
	}

	build.CustomJson = ""
	build.Status = ""
	build.FileSize = 0
	if err := (&CustomBuildService{}).UpdateValidated(build); err != nil {
		t.Fatalf("UpdateValidated() error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read validated build: %v", err)
	}
	if stored.CustomJson != "" || stored.Status != model.CustomBuildStatusBuilding || stored.FileSize != 42 {
		t.Fatalf("validated zero values were not saved: %#v", stored)
	}
	if stored.GithubRunId != 77 || stored.GithubArtifactID != 88 || stored.GithubRepo != "owner/repo" || stored.GithubSourceSha != strings.Repeat("a", 40) {
		t.Fatalf("validated update erased immutable provenance: %#v", stored)
	}

	build.CustomJson = `{"enable_audio":"false"}`
	if err := (&CustomBuildService{}).UpdateValidated(build); err == nil || !IsClientValidationError(err) {
		t.Fatalf("invalid UpdateValidated() error = %v, want ClientValidationError", err)
	}
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build after rejected update: %v", err)
	}
	if stored.CustomJson != "" {
		t.Fatalf("rejected validated update changed custom_json to %q", stored.CustomJson)
	}
}

func TestCustomBuildUpdateValidatedProviderBackedIdentityFieldsAreImmutable(t *testing.T) {
	const completeWindowsJSON = `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`
	cases := []struct {
		name      string
		platform  string
		appName   string
		wantError string
	}{
		{name: "platform", platform: "linux", appName: "rustqs", wantError: "immutable build platform cannot be changed"},
		{name: "app name", platform: "windows", appName: "other-client", wantError: "immutable build app name cannot be changed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newCustomPersistenceDB(t)
			stored := &model.CustomBuild{
				Platform:   "windows",
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: completeWindowsJSON,
				BuildRef:   strings.Repeat("a", 40),
			}
			if err := db.Create(stored).Error; err != nil {
				t.Fatalf("seed provider-backed build: %v", err)
			}

			candidate := *stored
			candidate.Platform = tc.platform
			candidate.AppName = tc.appName
			err := (&CustomBuildService{}).UpdateValidated(&candidate)
			if err == nil || !IsClientValidationError(err) {
				t.Fatalf("UpdateValidated() error = %v, want ClientValidationError", err)
			}
			if got := err.Error(); got != tc.wantError {
				t.Fatalf("UpdateValidated() error = %q, want %q", got, tc.wantError)
			}

			var unchanged model.CustomBuild
			if err := db.First(&unchanged, stored.Id).Error; err != nil {
				t.Fatalf("read unchanged provider-backed build: %v", err)
			}
			if unchanged.Platform != stored.Platform || unchanged.AppName != stored.AppName || unchanged.CustomJson != stored.CustomJson {
				t.Fatalf("rejected provider-backed identity update mutated row: %#v", unchanged)
			}
		})
	}
}

func TestCustomBuildUpdateValidatedProviderBackedWindowsRequiresProductionFields(t *testing.T) {
	const completeWindowsJSON = `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`
	const providerSourceRef = "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name       string
		platform   string
		version    string
		appName    string
		customJSON string
	}{
		{name: "missing platform", platform: "", version: "1.2.3", appName: "rustqs", customJSON: completeWindowsJSON},
		{name: "whitespace version", platform: "windows", version: " \t ", appName: "rustqs", customJSON: completeWindowsJSON},
		{name: "whitespace app name", platform: "windows", version: "1.2.3", appName: " \t ", customJSON: completeWindowsJSON},
		{name: "missing server endpoint", platform: "windows", version: "1.2.3", appName: "rustqs", customJSON: `{"key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`},
		{name: "missing public key", platform: "windows", version: "1.2.3", appName: "rustqs", customJSON: `{"server_ip":"id.example:21116","api_server":"https://api.example","relay_server":"relay.example:21117"}`},
		{name: "missing API endpoint", platform: "windows", version: "1.2.3", appName: "rustqs", customJSON: `{"server_ip":"id.example:21116","key":"public-key","relay_server":"relay.example:21117"}`},
		{name: "missing relay endpoint", platform: "windows", version: "1.2.3", appName: "rustqs", customJSON: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example"}`},
		{name: "hide cm whitespace password", platform: "windows", version: "1.2.3", appName: "rustqs", customJSON: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117","hide_cm":true,"permanent_password":" \t "}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newCustomPersistenceDB(t)
			stored := &model.CustomBuild{
				Platform:   "windows",
				Version:    "1.2.3",
				AppName:    "rustqs",
				CustomJson: completeWindowsJSON,
				BuildRef:   providerSourceRef,
			}
			if err := db.Create(stored).Error; err != nil {
				t.Fatalf("seed provider-backed build: %v", err)
			}
			candidate := *stored
			candidate.Platform = tc.platform
			candidate.Version = tc.version
			candidate.AppName = tc.appName
			candidate.CustomJson = tc.customJSON
			if err := (&CustomBuildService{}).UpdateValidated(&candidate); err == nil || !IsClientValidationError(err) {
				t.Fatalf("UpdateValidated() error = %v, want client validation error", err)
			}
			var unchanged model.CustomBuild
			if err := db.First(&unchanged, stored.Id).Error; err != nil {
				t.Fatalf("read unchanged provider-backed build: %v", err)
			}
			if unchanged.Platform != stored.Platform || unchanged.Version != stored.Version || unchanged.AppName != stored.AppName || unchanged.CustomJson != stored.CustomJson {
				t.Fatalf("rejected provider-backed update mutated row: %#v", unchanged)
			}
		})
	}
}

func TestCustomBuildUpdateValidatedAllowsIdentityLessLegacyWindowsDraft(t *testing.T) {
	db := newCustomPersistenceDB(t)
	legacy := &model.CustomBuild{
		Platform:   "windows",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: `{}`,
	}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("seed identity-less legacy build: %v", err)
	}
	legacy.Platform = "linux"
	legacy.AppName = "other-client"
	legacy.CustomJson = `{}`
	if err := (&CustomBuildService{}).UpdateValidated(legacy); err != nil {
		t.Fatalf("UpdateValidated() legacy error = %v, want permissive update", err)
	}
	var updated model.CustomBuild
	if err := db.First(&updated, legacy.Id).Error; err != nil {
		t.Fatalf("read updated legacy build: %v", err)
	}
	if updated.Platform != "linux" || updated.AppName != "other-client" || updated.CustomJson != `{}` {
		t.Fatalf("legacy fields = %#v, want permissive platform/app name/json update", updated)
	}
}

func TestCustomBuildUpdateValidatedActiveProviderIdentityRequiresProductionFields(t *testing.T) {
	db := newCustomPersistenceDB(t)
	stored := &model.CustomBuild{
		Platform:       "windows",
		Version:        "1.2.3",
		AppName:        "rustqs",
		CustomJson:     `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`,
		GithubProvider: "github",
	}
	if err := db.Create(stored).Error; err != nil {
		t.Fatalf("seed active-provider build: %v", err)
	}
	candidate := *stored
	candidate.CustomJson = `{"key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`
	if err := (&CustomBuildService{}).UpdateValidated(&candidate); err == nil || !IsClientValidationError(err) {
		t.Fatalf("UpdateValidated() error = %v, want client validation error", err)
	}
}

func newCustomPersistenceDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.CustomBuild{}, &model.CustomPreset{}); err != nil {
		t.Fatalf("migrate test models: %v", err)
	}
	previousDB := DB
	previousOutputDir := BuildOutputDir
	DB = db
	outputRoot := t.TempDir()
	BuildOutputDir = func(id uint) string { return filepath.Join(outputRoot, "output", fmt.Sprintf("%d", id)) }
	t.Cleanup(func() {
		DB = previousDB
		BuildOutputDir = previousOutputDir
		_ = sqlDB.Close()
	})
	return db
}

func mustTestReleaseAssetsJSON(t *testing.T) []byte {
	t.Helper()
	assetsJSON, err := json.Marshal(testReleaseAssets())
	if err != nil {
		t.Fatalf("marshal release assets: %v", err)
	}
	return assetsJSON
}

func mustStoredProducerManifestJSON(t *testing.T, build *model.CustomBuild, contents map[string]string) string {
	t.Helper()
	stored, err := producerManifestForBuild(build, contents).StoredJSON()
	if err != nil {
		t.Fatalf("store producer manifest: %v", err)
	}
	return stored
}

func recordValidPublication(t *testing.T, build *model.CustomBuild) {
	t.Helper()
	assetsJSON := mustTestReleaseAssetsJSON(t)
	if build.Version == "" || build.Version == "legacy" {
		build.Version = "1.2.3"
	}
	if build.BuildRef == "" {
		build.BuildRef = strings.Repeat("a", 40)
	}
	if build.SourceTag == "" {
		build.SourceTag = build.Version
	}
	if build.AssetsRelease == "" {
		build.AssetsRelease = "offline-assets-" + build.Version
	}
	if build.AssetsReleaseID == 0 {
		build.AssetsReleaseID = 12
	}
	if build.GithubProvider == "" {
		build.GithubProvider = "github"
	}
	if build.GithubRepo == "" {
		build.GithubRepo = "owner/repo"
	}
	if build.GithubWorkflow == "" {
		build.GithubWorkflow = "workflow.yml"
	}
	if build.WorkflowSelector == "" {
		build.WorkflowSelector = defaultWorkflowExecutionRef
	}
	if build.GithubRef == "" {
		build.GithubRef = build.BuildRef
	}
	if build.GithubArtifactName == "" {
		build.GithubArtifactName = "artifact"
	}
	if build.GithubRunUrl == "" {
		build.GithubRunUrl = fmt.Sprintf("https://api.github.com/repos/%s/actions/runs/%d", build.GithubRepo, build.GithubRunId)
	}
	if build.GithubHtmlUrl == "" {
		build.GithubHtmlUrl = fmt.Sprintf("https://github.com/%s/actions/runs/%d", build.GithubRepo, build.GithubRunId)
	}
	if err := DB.Model(&model.CustomBuild{}).Where("id = ?", build.Id).Updates(map[string]any{
		"version": build.Version, "build_ref": build.BuildRef, "source_tag": build.SourceTag,
		"assets_release": build.AssetsRelease, "assets_release_id": build.AssetsReleaseID,
		"github_provider": build.GithubProvider, "github_repo": build.GithubRepo,
		"github_workflow": build.GithubWorkflow, "workflow_selector": build.WorkflowSelector, "github_ref": build.GithubRef,
		"assets_release_assets": string(assetsJSON),
		"github_artifact_name":  build.GithubArtifactName, "github_run_url": build.GithubRunUrl,
		"github_html_url": build.GithubHtmlUrl,
	}).Error; err != nil {
		t.Fatalf("seed publication provenance: %v", err)
	}
	outDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create canonical output: %v", err)
	}
	appName := build.AppName
	if appName == "" {
		appName = "rustqs"
	}
	if err := os.WriteFile(filepath.Join(outDir, appName+".exe"), []byte("published"), 0600); err != nil {
		t.Fatalf("write published output: %v", err)
	}
	producerManifest := producerManifestForBuild(build, map[string]string{appName + ".exe": "published"})
	if err := (&CustomBuildService{}).RecordPublishedOutput(build.Id, build.GithubRunId, build.GithubArtifactID, producerManifest); err != nil {
		t.Fatalf("RecordPublishedOutput() error = %v", err)
	}
}

func assertCanonicalPersistedJSON(t *testing.T, value map[string]any) {
	t.Helper()
	if value["enable_audio"] == nil {
		t.Error("canonical output omitted supported enable_audio")
	}
	for _, key := range []string{
		"app_name", "version", "platform", "build_id", "future_field", "dead_field", "custom_txt",
	} {
		if _, ok := value[key]; ok {
			t.Errorf("canonical output contains excluded field %q", key)
		}
	}
}

func TestCustomPresetLegacyUpdateRejectsUnknownFieldWithoutMutation(t *testing.T) {
	db := newCustomPersistenceDB(t)
	for _, test := range []struct {
		name string
		json string
	}{
		{name: "unknown legacy field", json: `{"enable_audio":false,"unknown_legacy_field":"not-guaranteed"}`},
		{name: "raw custom txt", json: `{"custom_txt":"legacy-base64"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			legacy := &model.CustomPreset{
				UserId:     12,
				Name:       "legacy-preset-" + test.name,
				Platform:   "windows",
				Version:    "1.0.0",
				CustomJson: test.json,
			}
			if err := db.Session(&gorm.Session{SkipHooks: true}).Create(legacy).Error; err != nil {
				t.Fatalf("seed legacy preset: %v", err)
			}
			loaded, err := (&CustomPresetService{}).Info(legacy.Id)
			if err != nil {
				t.Fatalf("read legacy preset: %v", err)
			}
			if loaded.CustomJson != test.json {
				t.Fatalf("legacy read changed custom_json before update: %q", loaded.CustomJson)
			}
			loaded.Version = "2.0.0"
			err = (&CustomPresetService{}).Update(loaded)
			if err == nil || !IsClientValidationError(err) {
				t.Fatalf("legacy preset Update() error = %v, want ClientValidationError", err)
			}
			var unchanged model.CustomPreset
			if err := db.First(&unchanged, legacy.Id).Error; err != nil {
				t.Fatalf("read rejected preset: %v", err)
			}
			if unchanged.CustomJson != test.json || unchanged.Version != "1.0.0" {
				t.Fatalf("rejected update mutated row: %#v", unchanged)
			}
		})
	}
}

func TestCustomPresetUpdateClearsPreviouslyPopulatedField(t *testing.T) {
	newCustomPersistenceDB(t)
	service := &CustomPresetService{}
	preset := &model.CustomPreset{
		UserId:     21,
		Name:       "clearable-preset",
		Platform:   "windows",
		Version:    "1.0.0",
		AppName:    "rustqs",
		CustomJson: `{"company_name":"DeskForge","server_ip":"id.example:21116"}`,
	}
	if err := service.Create(preset); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	preset.Version = "2.0.0"
	preset.CustomJson = `{"company_name":"","server_ip":""}`
	if err := service.Update(preset); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	loaded, err := service.Info(preset.Id)
	if err != nil {
		t.Fatalf("read updated preset: %v", err)
	}
	if loaded.CustomJson != preset.CustomJson {
		t.Fatalf("loaded canonical custom_json = %q, want %q", loaded.CustomJson, preset.CustomJson)
	}
	normalized, err := NormalizeCustomBuildJSON(loaded.CustomJson, BuildRecordContext{Platform: loaded.Platform, AppName: "rustqs", Version: loaded.Version})
	if err != nil {
		t.Fatalf("NormalizeCustomBuildJSON() error = %v", err)
	}
	if normalized.Spec.CompanyName != "" || normalized.Spec.ServerIP != "" {
		t.Fatalf("cleared values retained stale state: %#v", normalized.Spec)
	}
	persisted := decodeJSON(t, loaded.CustomJson)
	for _, key := range []string{"company_name", "server_ip"} {
		if value, ok := persisted[key]; !ok || value != "" {
			t.Errorf("persisted[%q] = %#v, present = %v, want empty value", key, value, ok)
		}
	}
}

func TestValidateCustomBuildInput(t *testing.T) {
	tests := []struct {
		name       string
		platform   string
		customJSON string
		wantErr    bool
	}{
		{name: "supported empty payload", platform: "linux"},
		{name: "windows complete payload", platform: "windows", customJSON: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`},
		{name: "supported typed payload", platform: "linux", customJSON: `{"enable_audio":false}`},
		{name: "windows missing server endpoint", platform: "windows", customJSON: `{"key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`, wantErr: true},
		{name: "windows missing public key", platform: "windows", customJSON: `{"server_ip":"id.example:21116","api_server":"https://api.example","relay_server":"relay.example:21117"}`, wantErr: true},
		{name: "windows missing API URL", platform: "windows", customJSON: `{"server_ip":"id.example:21116","key":"public-key","relay_server":"relay.example:21117"}`, wantErr: true},
		{name: "windows missing relay endpoint", platform: "windows", customJSON: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example"}`, wantErr: true},
		{name: "windows empty required values", platform: "windows", customJSON: `{"server_ip":"","key":"","api_server":"","relay_server":""}`, wantErr: true},
		{name: "android requires package identity", platform: "android", wantErr: true},
		{name: "unsupported platform", platform: "macos", customJSON: `{}`, wantErr: true},
		{name: "malformed payload", platform: "android", customJSON: `{`, wantErr: true},
		{name: "raw custom txt bypass", platform: "windows", customJSON: `{"custom_txt":"raw"}`, wantErr: true},
		{name: "wrong typed field", platform: "windows", customJSON: `{"enable_audio":"false"}`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCustomBuildInput(test.platform, test.customJSON, "app", "1.2.3")
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateCustomBuildInput() error = %v, wantErr %v", err, test.wantErr)
			}
			if err != nil && !IsClientValidationError(err) {
				t.Fatalf("error type = %T, want ClientValidationError", err)
			}
		})
	}
}

func TestValidateCustomBuildInputUsesUserAuthoredRecordFields(t *testing.T) {
	const completeWindowsJSON = `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`
	for _, test := range []struct {
		name     string
		platform string
		appName  string
		version  string
	}{
		{name: "empty AppName", platform: "windows", version: "1.2.3"},
		{name: "unsupported platform", platform: "macos", appName: "DeskForge", version: "1.2.3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCustomBuildInput(test.platform, completeWindowsJSON, test.appName, test.version)
			if err == nil {
				t.Fatal("ValidateCustomBuildInput() error = nil, want typed record-field rejection")
			}
			if !IsClientValidationError(err) {
				t.Fatalf("error type = %T, want ClientValidationError", err)
			}
		})
	}
}

func TestCustomBuildCreateRejectsMissingAndroidAppIDBeforePersistence(t *testing.T) {
	db := newCustomPersistenceDB(t)
	build := &model.CustomBuild{
		Name:       "android-build",
		Platform:   "android",
		Version:    "1.2.3",
		AppName:    "rustqs",
		CustomJson: "",
	}

	err := (&CustomBuildService{}).Create(build)
	if err == nil || !IsClientValidationError(err) {
		t.Fatalf("Create() error = %v, want typed Android identity validation", err)
	}
	var count int64
	if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
		t.Fatalf("count builds: %v", err)
	}
	if count != 0 {
		t.Fatalf("missing Android identity created %d build row(s)", count)
	}
}
