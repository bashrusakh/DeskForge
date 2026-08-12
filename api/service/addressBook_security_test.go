package service

import (
	"database/sql"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"rustdesk-server/api/model"
)

func TestAddressBookUpdateAllPreservesCredentialsOmittedBySafeResponse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer func(db *sql.DB) { _ = db.Close() }(sqlDB)
	if err := db.AutoMigrate(&model.AddressBook{}); err != nil {
		t.Fatalf("migrate address book: %v", err)
	}
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	stored := &model.AddressBook{Id: "peer-id", Password: "password-secret", Hash: "hash-credential", Alias: "old"}
	if err := db.Create(stored).Error; err != nil {
		t.Fatalf("create address book: %v", err)
	}
	update := &model.AddressBook{RowId: stored.RowId, Id: stored.Id, Alias: "new"}
	if err := (&AddressBookService{}).UpdateAll(update); err != nil {
		t.Fatalf("UpdateAll() error = %v", err)
	}

	var loaded model.AddressBook
	if err := db.First(&loaded, stored.RowId).Error; err != nil {
		t.Fatalf("reload address book: %v", err)
	}
	if loaded.Alias != "new" || loaded.Password != stored.Password || loaded.Hash != stored.Hash {
		t.Fatalf("updated address book = %#v, want public fields updated and credentials preserved", loaded)
	}
}
