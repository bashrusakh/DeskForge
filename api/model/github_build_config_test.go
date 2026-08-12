package model

import (
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGithubBuildConfigAutoMigrateAddsWorkflowApprovalAndPreservesLegacyRead(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "model-workflow-approval-test")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&GithubBuildConfig{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if !db.Migrator().HasColumn(&GithubBuildConfig{}, "workflow_ref_approved") {
		t.Fatal("AutoMigrate() did not add workflow_ref_approved")
	}
	if !db.Migrator().HasColumn(&GithubBuildConfig{}, "workflow_ref_provider_verified") {
		t.Fatal("AutoMigrate() did not add workflow_ref_provider_verified")
	}
	if !db.Migrator().HasColumn(&GithubBuildConfig{}, "workflow_ref_approval_sha") {
		t.Fatal("AutoMigrate() did not add workflow_ref_approval_sha")
	}

	legacy := &GithubBuildConfig{IdModel: IdModel{Id: 1}, Repo: "owner/repo", Branch: "master"}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("create legacy config: %v", err)
	}
	var loaded GithubBuildConfig
	if err := db.First(&loaded, 1).Error; err != nil {
		t.Fatalf("read legacy config: %v", err)
	}
	if loaded.WorkflowRefApproved {
		t.Fatal("legacy config unexpectedly read as approved")
	}
	view, err := json.Marshal(loaded.Safe())
	if err != nil {
		t.Fatalf("marshal safe config: %v", err)
	}
	encoded := string(view)
	if !strings.Contains(encoded, `"workflow_ref":""`) || strings.Contains(encoded, `"branch"`) {
		t.Fatalf("safe legacy config = %s, want no mutable ref in safe view", encoded)
	}
}
