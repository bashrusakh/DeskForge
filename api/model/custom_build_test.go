package model

import (
	"testing"

	"rustdesk-server/api/utils"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// customBuildBeforePresetSchema matches CustomBuild as it existed at
// origin/main@69776a12a4e0e0643b26c992df68f8ea5a70ed90, before preset
// provenance was added. Keep this schema in sync with that historical model.
type customBuildBeforePresetSchema struct {
	IdModel
	UserId                uint   `json:"user_id" gorm:"default:0;not null;"`
	Name                  string `json:"name" gorm:"size:128;default:'';not null;"`
	Platform              string `json:"platform" gorm:"size:32;default:'';not null;"`
	Version               string `json:"version" gorm:"size:32;default:'';not null;"`
	Status                string `json:"status" gorm:"size:32;default:'pending';not null;"`
	AppName               string `json:"app_name" gorm:"size:128;default:'';not null;"`
	CustomJson            string `json:"-" gorm:"type:text;"`
	BuildLog              string `json:"build_log" gorm:"type:text;"`
	FileSize              int64  `json:"file_size" gorm:"default:0;not null;"`
	DownloadKey           string `json:"-" gorm:"size:64;default:'';not null;"`
	DownloadKeyExpiresAt  int64  `json:"download_key_expires_at" gorm:"default:0;not null;"`
	GithubRunId           int64  `json:"github_run_id" gorm:"default:0;not null;"`
	GithubProvider        string `json:"github_provider" gorm:"size:32;default:'';not null;"`
	GithubRepo            string `json:"github_repo" gorm:"size:128;default:'';not null;"`
	GithubWorkflow        string `json:"github_workflow" gorm:"size:128;default:'';not null;"`
	WorkflowSelector      string `json:"-" gorm:"size:128;default:'';not null;"`
	GithubRef             string `json:"github_ref" gorm:"size:128;default:'';not null;"`
	GithubArtifactName    string `json:"github_artifact_name" gorm:"size:128;default:'';not null;"`
	GithubArtifactID      int64  `json:"github_artifact_id" gorm:"default:0;not null;"`
	GithubRunUrl          string `json:"github_run_url" gorm:"size:512;default:'';not null;"`
	GithubHtmlUrl         string `json:"github_html_url" gorm:"size:512;default:'';not null;"`
	GithubSourceSha       string `json:"github_source_sha" gorm:"size:128;default:'';not null;"`
	PublicationRecordedAt int64  `json:"-" gorm:"default:0;not null;"`
	PublishedDigest       string `json:"-" gorm:"size:64;default:'';not null;"`
	ProducerManifestJSON  string `json:"-" gorm:"type:text;"`
	BuildRef              string `json:"-" gorm:"size:128;default:'';not null;"`
	SourceTag             string `json:"-" gorm:"size:128;default:'';not null;"`
	AssetsRelease         string `json:"-" gorm:"size:128;default:'';not null;"`
	AssetsReleaseID       int64  `json:"-" gorm:"default:0;not null;"`
	AssetsReleaseAssets   string `json:"-" gorm:"type:text;"`
	TimeModel
}

func (customBuildBeforePresetSchema) TableName() string { return "custom_builds" }

func TestCustomBuildAutoMigrateAddsProvenanceAndReadsLegacyRows(t *testing.T) {
	t.Setenv(utils.SecretEncryptionKeyEnv, "legacy-migration-test-key")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite handle: %v", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&customBuildBeforePresetSchema{}); err != nil {
		t.Fatalf("create pre-preset schema: %v", err)
	}
	legacyJSON := `{"enable_audio":true,"permanent_password":"legacy-secret"}`
	encryptedJSON, err := utils.EncryptCustomBuilderJSON(legacyJSON)
	if err != nil {
		t.Fatalf("encrypt legacy custom_json: %v", err)
	}
	legacy := &customBuildBeforePresetSchema{
		IdModel: IdModel{Id: 42}, UserId: 7, Name: "legacy build", Platform: "windows",
		Version: "1.2.3", Status: CustomBuildStatusDone, AppName: "legacy-app",
		CustomJson: encryptedJSON, BuildLog: "legacy build log", FileSize: 12345,
		DownloadKey: "legacy-download-key", DownloadKeyExpiresAt: 987654,
		GithubRunId: 77, GithubProvider: "github", GithubRepo: "owner/repo",
		GithubWorkflow: "build.yml", WorkflowSelector: "refs/heads/main",
		GithubRef: "0123456789abcdef", GithubArtifactName: "client.zip",
		GithubArtifactID: 88, GithubRunUrl: "https://example.test/run/77",
		GithubHtmlUrl: "https://example.test/artifact/88", GithubSourceSha: "abcdef0123456789",
		PublicationRecordedAt: 123456, PublishedDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProducerManifestJSON: `{"schema":1}`, BuildRef: "build-ref", SourceTag: "v1.2.3",
		AssetsRelease: "release-v1.2.3", AssetsReleaseID: 99,
		AssetsReleaseAssets: `[{"id":1,"name":"client.zip"}]`,
	}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	if err := db.AutoMigrate(&CustomBuild{}); err != nil {
		t.Fatalf("candidate AutoMigrate() error = %v", err)
	}
	for _, column := range []string{"preset_id", "preset_name"} {
		if !db.Migrator().HasColumn(&CustomBuild{}, column) {
			t.Errorf("candidate AutoMigrate() missing column %q", column)
		}
	}

	var loaded CustomBuild
	if err := db.First(&loaded, legacy.Id).Error; err != nil {
		t.Fatalf("read migrated legacy row: %v", err)
	}
	if loaded.UserId != legacy.UserId || loaded.Name != legacy.Name || loaded.Platform != legacy.Platform ||
		loaded.Version != legacy.Version || loaded.Status != legacy.Status || loaded.AppName != legacy.AppName ||
		loaded.CustomJson != legacyJSON || loaded.BuildLog != legacy.BuildLog || loaded.FileSize != legacy.FileSize ||
		loaded.DownloadKey != legacy.DownloadKey || loaded.DownloadKeyExpiresAt != legacy.DownloadKeyExpiresAt ||
		loaded.GithubRunId != legacy.GithubRunId || loaded.GithubProvider != legacy.GithubProvider ||
		loaded.GithubRepo != legacy.GithubRepo || loaded.GithubWorkflow != legacy.GithubWorkflow ||
		loaded.WorkflowSelector != legacy.WorkflowSelector || loaded.GithubRef != legacy.GithubRef ||
		loaded.GithubArtifactName != legacy.GithubArtifactName || loaded.GithubArtifactID != legacy.GithubArtifactID ||
		loaded.GithubRunUrl != legacy.GithubRunUrl || loaded.GithubHtmlUrl != legacy.GithubHtmlUrl ||
		loaded.GithubSourceSha != legacy.GithubSourceSha || loaded.PublicationRecordedAt != legacy.PublicationRecordedAt ||
		loaded.PublishedDigest != legacy.PublishedDigest || loaded.ProducerManifestJSON != legacy.ProducerManifestJSON ||
		loaded.BuildRef != legacy.BuildRef || loaded.SourceTag != legacy.SourceTag ||
		loaded.AssetsRelease != legacy.AssetsRelease || loaded.AssetsReleaseID != legacy.AssetsReleaseID ||
		loaded.AssetsReleaseAssets != legacy.AssetsReleaseAssets {
		t.Fatalf("AutoMigrate() did not preserve populated legacy fields: %#v", loaded)
	}
	if loaded.PresetId != 0 || loaded.PresetName != "" {
		t.Fatalf("legacy row provenance = (%d, %q), want (0, empty)", loaded.PresetId, loaded.PresetName)
	}
	if safe := loaded.Safe(); safe == nil || safe.PresetId != 0 || safe.PresetName != "" {
		t.Fatalf("legacy safe view provenance = %#v, want (0, empty) for UI dash", safe)
	}

	newBuild := &CustomBuild{
		UserId: 7, Name: "preset build", Platform: "linux", Version: "2.0.0",
		Status: CustomBuildStatusPending, AppName: "preset-app", CustomJson: `{"enable_audio":false}`,
		PresetId: 19, PresetName: "release preset",
	}
	if err := db.Create(newBuild).Error; err != nil {
		t.Fatalf("create new preset snapshot after migration: %v", err)
	}
	var created CustomBuild
	if err := db.First(&created, newBuild.Id).Error; err != nil {
		t.Fatalf("read new preset snapshot: %v", err)
	}
	if created.PresetId != newBuild.PresetId || created.PresetName != newBuild.PresetName {
		t.Fatalf("new preset snapshot = (%d, %q), want (%d, %q)", created.PresetId, created.PresetName, newBuild.PresetId, newBuild.PresetName)
	}
}
