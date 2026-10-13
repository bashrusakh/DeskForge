package main

import (
	"errors"
	"strings"
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

type legacyCustomPresetWithJSON struct {
	Id         uint   `gorm:"primaryKey"`
	UserId     uint   `gorm:"not null"`
	Name       string `gorm:"size:128;not null"`
	CustomJson string
}

func (legacyCustomPresetWithJSON) TableName() string {
	return "custom_presets"
}

func TestMigrateSchemaAndRecordVersionRejectsNonStringCustomBrandingFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Version{}, &legacyCustomPresetWithJSON{}); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
	if err := db.Create(&legacyCustomPresetWithJSON{UserId: 7, Name: "forged", CustomJson: `{"website_url":123}`}).Error; err != nil {
		t.Fatalf("seed forged legacy preset: %v", err)
	}
	// An explicit JSON null is present-but-non-string: the typed field reader
	// rejects it exactly like any other non-string value, so the preflight
	// must fail closed on it too instead of skipping it.
	if err := db.Create(&legacyCustomPresetWithJSON{UserId: 9, Name: "nulled", CustomJson: `{"website_url":null}`}).Error; err != nil {
		t.Fatalf("seed nulled legacy preset: %v", err)
	}

	migrateCalled := false
	err = migrateSchemaAndRecordVersion(db, DatabaseVersion, func(db *gorm.DB) error {
		migrateCalled = true
		return db.AutoMigrate(&model.CustomPreset{})
	})
	if !errors.Is(err, errInvalidCustomBrandingFields) {
		t.Fatalf("migrateSchemaAndRecordVersion() error = %v, want non-string branding field error", err)
	}
	if migrateCalled {
		t.Fatal("AutoMigrate ran after branding preflight failure")
	}
	var count int64
	if err := db.Model(&model.Version{}).Count(&count).Error; err != nil {
		t.Fatalf("count database versions: %v", err)
	}
	if count != 0 {
		t.Fatalf("database version count = %d, want 0 after branding preflight failure", count)
	}
}

func TestMigrateSchemaAndRecordVersionAcceptsStringCustomBrandingFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Version{}, &legacyCustomPresetWithJSON{}); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
	if err := db.Create(&legacyCustomPresetWithJSON{
		UserId:     7,
		Name:       "branding",
		CustomJson: `{"website_url":"https://www.example.com","privacy_policy_url":"https://www.example.com/privacy"}`,
	}).Error; err != nil {
		t.Fatalf("seed preset: %v", err)
	}
	// Absent branding keys must remain allowed even when unrelated fields are
	// present: only a present non-string/null value fails the gate.
	if err := db.Create(&legacyCustomPresetWithJSON{UserId: 9, Name: "absent-fields", CustomJson: `{"app_name":"rustqs","version":"1.0"}`}).Error; err != nil {
		t.Fatalf("seed absent-field preset: %v", err)
	}
	// An opaque secret-bearing row must not block the preflight: it is not a
	// plain-JSON value and carries no plaintext branding names.
	if err := db.Create(&legacyCustomPresetWithJSON{UserId: 8, Name: "encrypted", CustomJson: "Salted__opaque-ciphertext"}).Error; err != nil {
		t.Fatalf("seed encrypted preset: %v", err)
	}

	if err := migrateSchemaAndRecordVersion(db, DatabaseVersion, func(db *gorm.DB) error {
		return db.AutoMigrate(&model.CustomPreset{})
	}); err != nil {
		t.Fatalf("migrateSchemaAndRecordVersion() error = %v", err)
	}
	var versions []model.Version
	if err := db.Find(&versions).Error; err != nil {
		t.Fatalf("read database version: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != DatabaseVersion {
		t.Fatalf("database versions = %#v, want one version %d", versions, DatabaseVersion)
	}
}

func TestMigrateSchemaAndRecordVersionCustomBrandingPreflight(t *testing.T) {
	// Every case seeds one legacy custom_presets row (the table named by the
	// original preflight test) with the given custom_json, then runs the real
	// migration gate at DatabaseVersion (>= the branding migration version) and
	// asserts whether it fails closed with errInvalidCustomBrandingFields.
	overLongURL := "https://download.example/" + strings.Repeat("a", 2049)
	overLongCompany := strings.Repeat("x", 257)
	cases := []struct {
		name       string
		customJSON string
		abort      bool
	}{
		// download_url is newly validated by this PR; a stored value the typed
		// boundary rejects must stop the migration instead of passing it.
		{"download-relative", `{"download_url":"example.com/app"}`, true},
		{"download-mixed-case-scheme", `{"download_url":"HTTP://x"}`, true},
		{"download-punctuation-host", `{"download_url":"https://."}`, true},
		{"download-over-length", `{"download_url":"` + overLongURL + `"}`, true},
		// company_name is newly validated too (control characters and length).
		{"company-control-char", `{"company_name":"Acme\nLtd"}`, true},
		{"company-over-length", `{"company_name":"` + overLongCompany + `"}`, true},
		// A present non-string / explicit null for the new fields.
		{"download-non-string", `{"download_url":123}`, true},
		{"company-null", `{"company_name":null}`, true},
		{"app-logo-non-string", `{"app_logo_url":["/upload/logo.png"]}`, true},
		// A branding image key shares the dispatch control-character rule.
		{"privacy-screen-control-char", `{"privacy_screen_url":"bad\nvalue"}`, true},
		// The existing website_url tightening is preserved: a format-invalid
		// stored string now fails too, not just a non-string value.
		{"website-mixed-case-scheme", `{"website_url":"HTTP://x"}`, true},
		// Valid branding values, absent keys, and unrelated invalid fields must
		// all pass: the preflight must not over-block a row for a non-branding
		// field, nor run the whole typed boundary over legacy rows.
		{"valid-branding", `{"company_name":"Acme Ltd","download_url":"http://download.example/app","website_url":"https://www.example.com","privacy_policy_url":"https://www.example.com/privacy","app_icon_url":"/upload/icon.png","app_logo_url":"/upload/logo.png","privacy_screen_url":"/upload/privacy.png"}`, false},
		{"absent-keys", `{"app_name":"rustqs","version":"1.0"}`, false},
		{"unrelated-invalid-only", `{"server_ip":"bad host","direction":"sideways"}`, false},
		{"unrelated-invalid-with-valid-branding", `{"website_url":"https://www.example.com","direction":"sideways"}`, false},
		// A substring LIKE match without an exact branding key (the row is
		// selected, but only "company_name_note" is present) plus an unrelated
		// invalid field must still pass: only exact branding keys are inspected.
		{"unrelated-invalid-selected-via-substring", `{"direction":"sideways","company_name_note":"x"}`, false},
		// A branding key written with JSON \uXXXX escapes decodes to the name
		// the typed boundary reads, so a raw-text prefilter would miss the row
		// and record migration success over a value the typed boundary later
		// rejects. Inspection of the decoded JSON must still fail closed.
		{"escaped-company-key-non-string", `{"company\u005fname":123}`, true},
		{"escaped-company-key-over-length", `{"company\u005fname":"` + overLongCompany + `"}`, true},
		{"escaped-website-key-invalid-url", `{"website\u005furl":"HTTP://x"}`, true},
		{"escaped-privacy-key-control-char", `{"privacy\u005fpolicy\u005furl":"https://example.com/\u0086"}`, true},
		// A C1 control character (U+0086) is not whitespace and url.ParseRequestURI
		// accepts it, so the branding URL seam must also apply the dispatch
		// control-character rule to the URL text fields.
		{"website-url-c1-control", `{"website_url":"https://example.com/\u0086"}`, true},
		// An escaped key that only resembles a branding key after decoding is an
		// unrelated field and must not over-block the row.
		{"escaped-unrelated-key-only", `{"company\u005fname\u005fnote":"x","direction":"sideways"}`, false},
		// An encrypted row carries branding names only inside opaque ciphertext
		// and must stay skipped rather than be decoded as plaintext.
		{"encrypted-skipped", "Salted__opaque-website_url-download_url-company_name", false},
		{"encrypted-prefix-skipped", "enc:v1:not-valid-json-website_url", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			if err := db.AutoMigrate(&model.Version{}, &legacyCustomPresetWithJSON{}); err != nil {
				t.Fatalf("create legacy tables: %v", err)
			}
			if err := db.Create(&legacyCustomPresetWithJSON{UserId: 7, Name: "seed", CustomJson: tc.customJSON}).Error; err != nil {
				t.Fatalf("seed legacy preset: %v", err)
			}

			migrateCalled := false
			err = migrateSchemaAndRecordVersion(db, DatabaseVersion, func(db *gorm.DB) error {
				migrateCalled = true
				return nil
			})
			if tc.abort {
				if !errors.Is(err, errInvalidCustomBrandingFields) {
					t.Fatalf("migrateSchemaAndRecordVersion() error = %v, want errInvalidCustomBrandingFields", err)
				}
				if migrateCalled {
					t.Fatal("AutoMigrate ran after branding preflight failure")
				}
			} else if err != nil {
				t.Fatalf("migrateSchemaAndRecordVersion() error = %v, want accepted", err)
			} else if !migrateCalled {
				t.Fatal("AutoMigrate was not called for an accepted row")
			}

			var count int64
			if err := db.Model(&model.Version{}).Count(&count).Error; err != nil {
				t.Fatalf("count database versions: %v", err)
			}
			wantCount := int64(1)
			if tc.abort {
				wantCount = 0
			}
			if count != wantCount {
				t.Fatalf("database version count = %d, want %d", count, wantCount)
			}
		})
	}
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
