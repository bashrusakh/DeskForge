package main

import (
	"errors"
	"testing"

	"rustdesk-server/api/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyCustomPreset struct {
	Id     uint   `gorm:"primaryKey"`
	UserId uint   `gorm:"not null"`
	Name   string `gorm:"size:128;not null"`
}

func (legacyCustomPreset) TableName() string {
	return "custom_presets"
}

func TestMigrateSchemaAndRecordVersionFailsClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Version{}); err != nil {
		t.Fatalf("create version table: %v", err)
	}
	wantErr := errors.New("schema migration failed")
	if err := migrateSchemaAndRecordVersion(db, DatabaseVersion, func(*gorm.DB) error {
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("migrateSchemaAndRecordVersion() error = %v, want %v", err, wantErr)
	}
	var count int64
	if err := db.Model(&model.Version{}).Count(&count).Error; err != nil {
		t.Fatalf("count database versions: %v", err)
	}
	if count != 0 {
		t.Fatalf("database version count = %d, want 0 after migration failure", count)
	}
}

func TestMigrateSchemaAndRecordVersionAddsCustomPresetOwnerNameUniqueIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Version{}, &legacyCustomPreset{}); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
	if err := db.Create(&legacyCustomPreset{UserId: 7, Name: "shared"}).Error; err != nil {
		t.Fatalf("seed legacy preset: %v", err)
	}

	migrateCalled := false
	if err := migrateSchemaAndRecordVersion(db, DatabaseVersion, func(db *gorm.DB) error {
		migrateCalled = true
		return db.AutoMigrate(&model.CustomPreset{})
	}); err != nil {
		t.Fatalf("migrateSchemaAndRecordVersion() error = %v", err)
	}
	if !migrateCalled {
		t.Fatal("AutoMigrate was not called")
	}
	if !db.Migrator().HasIndex(&model.CustomPreset{}, model.CustomPresetUserNameUniqueIndex) {
		t.Fatalf("missing custom preset index %q", model.CustomPresetUserNameUniqueIndex)
	}
	if err := db.Create(&legacyCustomPreset{UserId: 7, Name: "shared"}).Error; err == nil {
		t.Fatal("duplicate owner/name preset insert succeeded after migration")
	}
	if err := db.Create(&legacyCustomPreset{UserId: 8, Name: "shared"}).Error; err != nil {
		t.Fatalf("same-name preset for another user failed: %v", err)
	}

	var versions []model.Version
	if err := db.Find(&versions).Error; err != nil {
		t.Fatalf("read database version: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != DatabaseVersion {
		t.Fatalf("database versions = %#v, want one version %d", versions, DatabaseVersion)
	}
}

func TestMigrateSchemaAndRecordVersionRejectsDuplicateCustomPresetNamesBeforeAutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Version{}, &legacyCustomPreset{}); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
	for range 2 {
		if err := db.Create(&legacyCustomPreset{UserId: 7, Name: "duplicate"}).Error; err != nil {
			t.Fatalf("seed duplicate legacy preset: %v", err)
		}
	}

	migrateCalled := false
	err = migrateSchemaAndRecordVersion(db, DatabaseVersion, func(db *gorm.DB) error {
		migrateCalled = true
		return db.AutoMigrate(&model.CustomPreset{})
	})
	if !errors.Is(err, errDuplicateCustomPresetNames) {
		t.Fatalf("migrateSchemaAndRecordVersion() error = %v, want duplicate-owner/name error", err)
	}
	if migrateCalled {
		t.Fatal("AutoMigrate ran after duplicate preflight failure")
	}
	if db.Migrator().HasIndex(&model.CustomPreset{}, model.CustomPresetUserNameUniqueIndex) {
		t.Fatalf("custom preset index %q was created despite duplicate data", model.CustomPresetUserNameUniqueIndex)
	}
	var count int64
	if err := db.Model(&model.Version{}).Count(&count).Error; err != nil {
		t.Fatalf("count database versions: %v", err)
	}
	if count != 0 {
		t.Fatalf("database version count = %d, want 0 after duplicate preflight failure", count)
	}
}
