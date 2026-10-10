package model

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCustomBuildAutoMigrateAddsProvenanceAndReadsLegacyRows(t *testing.T) {
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
	if err := db.AutoMigrate(&CustomBuild{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	for _, column := range []string{
		"github_provider", "github_repo", "github_workflow", "workflow_selector", "github_ref",
		"github_artifact_name", "github_artifact_id", "github_run_url", "github_html_url", "github_source_sha",
		"build_ref", "source_tag", "assets_release", "assets_release_id",
		"assets_release_assets",
		"publication_recorded_at", "published_digest",
		"producer_manifest_json",
		"preset_id", "preset_name",
	} {
		if !db.Migrator().HasColumn(&CustomBuild{}, column) {
			t.Errorf("AutoMigrate() missing provenance column %q", column)
		}
	}
	legacy := &CustomBuild{Status: CustomBuildStatusBuilding, GithubRunId: 77, DownloadKey: "legacy-download-key"}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("create legacy row: %v", err)
	}
	var loaded CustomBuild
	if err := db.First(&loaded, legacy.Id).Error; err != nil {
		t.Fatalf("read legacy row: %v", err)
	}
	if loaded.GithubRunId != 77 || loaded.DownloadKey != "legacy-download-key" || loaded.GithubRepo != "" || loaded.GithubSourceSha != "" || loaded.PublicationRecordedAt != 0 || loaded.PublishedDigest != "" {
		t.Fatalf("legacy row was not readable with empty provenance: %#v", loaded)
	}
	// Legacy rows predating source-preset provenance read as empty values, the
	// representation the admin UI renders as "—".
	if loaded.PresetId != 0 || loaded.PresetName != "" {
		t.Fatalf("legacy row gained source-preset provenance: preset_id=%d preset_name=%q", loaded.PresetId, loaded.PresetName)
	}
	if safe := loaded.Safe(); safe == nil || safe.PresetId != 0 || safe.PresetName != "" {
		t.Fatalf("legacy safe view gained source-preset provenance: %#v", safe)
	}
}
