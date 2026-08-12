package main

import (
	"errors"
	"testing"

	"rustdesk-server/api/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
