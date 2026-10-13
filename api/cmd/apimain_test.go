package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"rustdesk-server/api/config"
	"rustdesk-server/api/global"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"

	"github.com/sirupsen/logrus"
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
	// The branding image preflight checks stored data only and never consults a
	// public origin, so no origin is configured for this test: a server-relative
	// image is valid stored data and must pass, while a data-invalid stored value
	// still fails closed.
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
		// The image keys are checked as stored data with no public origin
		// configured: a server-relative path is valid and passes (resolving it is
		// the build-spec producer's concern), while a data-invalid stored value —
		// a non-http(s) scheme, a host-less URL, or a literal host outside the
		// public-address policy — is still refused. A valid absolute image URL
		// stays accepted.
		{"image-relative-accepted-no-origin", `{"app_icon_url":"/upload/icon.png"}`, false},
		{"image-relative-logo-accepted-no-origin", `{"app_logo_url":"/upload/logo.png"}`, false},
		{"image-file-scheme", `{"privacy_screen_url":"file:///etc/passwd"}`, true},
		{"image-missing-host", `{"app_icon_url":"https:///x.png"}`, true},
		{"image-loopback-host", `{"app_logo_url":"http://127.0.0.1:21114/logo.png"}`, true},
		// The public-address policy also refuses the special-purpose ranges the
		// old loopback/private/link-local check accepted: unspecified, shared/
		// CGNAT, documentation, benchmarking, multicast, reserved, and the IPv6
		// documentation range, while a public literal stays accepted.
		{"image-unspecified-host", `{"app_icon_url":"http://0.0.0.0/i.png"}`, true},
		{"image-shared-address-host", `{"app_logo_url":"http://100.64.0.1/i.png"}`, true},
		{"image-documentation-host", `{"privacy_screen_url":"http://192.0.2.1/i.png"}`, true},
		{"image-benchmarking-host", `{"app_icon_url":"http://198.18.0.1/i.png"}`, true},
		{"image-multicast-host", `{"app_logo_url":"http://224.0.0.1/i.png"}`, true},
		{"image-broadcast-host", `{"app_icon_url":"http://255.255.255.255/i.png"}`, true},
		{"image-documentation-v6-host", `{"app_logo_url":"http://[2001:db8::1]/i.png"}`, true},
		{"image-public-literal-accepted", `{"privacy_screen_url":"http://8.8.8.8/i.png"}`, false},
		{"image-absolute-valid", `{"privacy_screen_url":"https://cdn.example/i.png"}`, false},
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

// preUpgradeTipVersion is the DatabaseVersion that `main` carried before this
// change (283->284 added the branding preflight). An install already at this
// version is exactly the population the preflight must now reach, so the
// startup regression tests seed a real Version row at this literal value
// rather than at the current DatabaseVersion constant.
const preUpgradeTipVersion = 284

// runStartupGate exercises the real process startup path — DatabaseAutoUpdate
// against global.DB — with a seeded sqlite database, restoring the
// process-global state the gate depends on afterwards. It is deliberately not
// a direct call to migrateSchemaAndRecordVersion: the defect this guards
// against lives in the version gate around Migrate, above the helper.
func runStartupGate(t *testing.T, seed func(*gorm.DB)) error {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	seed(db)

	prevDB, prevLogger, prevGormType := global.DB, global.Logger, global.Config.Gorm.Type
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	global.DB = db
	global.Logger = logger
	global.Config.Gorm.Type = config.TypeSqlite
	t.Cleanup(func() {
		global.DB = prevDB
		global.Logger = prevLogger
		global.Config.Gorm.Type = prevGormType
	})
	return DatabaseAutoUpdate()
}

// seedLegacyCustomJSON inserts one plaintext custom_json row the way a legacy
// install would carry it: hooks skipped so the boundary/encryption layer does
// not rewrite or reject the stored value before the preflight can inspect it.
func seedLegacyCustomJSON(t *testing.T, db *gorm.DB, table string, customJSON string) {
	t.Helper()
	var err error
	switch table {
	case "custom_builds":
		err = db.Session(&gorm.Session{SkipHooks: true}).
			Create(&model.CustomBuild{UserId: 1, Name: "legacy", CustomJson: customJSON}).Error
	case "custom_presets":
		err = db.Session(&gorm.Session{SkipHooks: true}).
			Create(&model.CustomPreset{UserId: 1, Name: "legacy", CustomJson: customJSON}).Error
	default:
		t.Fatalf("unsupported seed table %q", table)
	}
	if err != nil {
		t.Fatalf("seed %s row: %v", table, err)
	}
}

// TestDatabaseAutoUpdateBrandingPreflightOnExistingVersion284 is the real
// startup regression: an install whose Version row is 284 (the pre-PR tip)
// must still run the branding preflight during DatabaseAutoUpdate and fail
// closed on a persisted branding value the shared seam rejects. Against the
// pre-fix code the `v.Version < DatabaseVersion` gate was 284 < 284 = false, so
// Migrate never ran, no error was returned, and this test fails.
func TestDatabaseAutoUpdateBrandingPreflightOnExistingVersion284(t *testing.T) {
	cases := []struct {
		name       string
		table      string
		customJSON string
	}{
		// A control character in company_name is rejected by validateCompanyName
		// independent of the separate image-URL format rules in flight.
		{"custom-builds-control-char", "custom_builds", `{"company_name":"Acme\nLtd"}`},
		// A present non-string value is rejected by the seam regardless of field.
		{"custom-builds-non-string", "custom_builds", `{"company_name":123}`},
		{"custom-presets-control-char", "custom_presets", `{"company_name":"Acme\nLtd"}`},
		{"custom-presets-non-string", "custom_presets", `{"company_name":123}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var db *gorm.DB
			err := runStartupGate(t, func(seeded *gorm.DB) {
				db = seeded
				if err := seeded.AutoMigrate(&model.Version{}, &model.CustomBuild{}, &model.CustomPreset{}); err != nil {
					t.Fatalf("create install tables: %v", err)
				}
				if err := seeded.Create(&model.Version{Version: preUpgradeTipVersion}).Error; err != nil {
					t.Fatalf("seed version %d: %v", preUpgradeTipVersion, err)
				}
				seedLegacyCustomJSON(t, seeded, tc.table, tc.customJSON)
			})
			if !errors.Is(err, errInvalidCustomBrandingFields) {
				t.Fatalf("DatabaseAutoUpdate() error = %v, want errInvalidCustomBrandingFields", err)
			}
			var versions []model.Version
			if err := db.Find(&versions).Error; err != nil {
				t.Fatalf("read database versions: %v", err)
			}
			if len(versions) != 1 || versions[0].Version != preUpgradeTipVersion {
				t.Fatalf("database versions = %#v, want only the existing version %d after fail-closed preflight", versions, preUpgradeTipVersion)
			}
		})
	}
}

// TestDatabaseAutoUpdateUpgradesExistingVersion284WithValidBranding is the
// clean counterpart: a 284 install whose persisted branding values are
// accepted upgrades and records the new DatabaseVersion.
func TestDatabaseAutoUpdateUpgradesExistingVersion284WithValidBranding(t *testing.T) {
	var db *gorm.DB
	err := runStartupGate(t, func(seeded *gorm.DB) {
		db = seeded
		if err := seeded.AutoMigrate(&model.Version{}, &model.CustomBuild{}, &model.CustomPreset{}); err != nil {
			t.Fatalf("create install tables: %v", err)
		}
		if err := seeded.Create(&model.Version{Version: preUpgradeTipVersion}).Error; err != nil {
			t.Fatalf("seed version %d: %v", preUpgradeTipVersion, err)
		}
		valid := `{"company_name":"Acme Ltd","website_url":"https://www.example.com"}`
		seedLegacyCustomJSON(t, seeded, "custom_builds", valid)
		seedLegacyCustomJSON(t, seeded, "custom_presets", valid)
	})
	if err != nil {
		t.Fatalf("DatabaseAutoUpdate() error = %v, want accepted upgrade", err)
	}
	var latest model.Version
	if err := db.Last(&latest).Error; err != nil {
		t.Fatalf("read latest database version: %v", err)
	}
	if latest.Version != DatabaseVersion {
		t.Fatalf("latest database version = %d, want %d after a 284 install upgraded", latest.Version, DatabaseVersion)
	}
	if DatabaseVersion <= preUpgradeTipVersion {
		t.Fatalf("DatabaseVersion = %d must exceed the pre-upgrade tip %d for this test to be meaningful", DatabaseVersion, preUpgradeTipVersion)
	}
}

// TestDatabaseAutoUpdateUpgradesExistingVersion284WithStoredRelativeImages is
// the regression for the migration/startup abort: a 284 install that stored
// server-relative branding image values ("/upload/...") and has no public origin
// configured must upgrade and record the new DatabaseVersion. Resolving those
// values against a public origin belongs to the build-spec producer boundary, so
// the stored-data preflight must not fail closed on a deployment configuration
// condition. Against the pre-fix seam the image case consulted the public origin
// and aborted, so this test fails there.
func TestDatabaseAutoUpdateUpgradesExistingVersion284WithStoredRelativeImages(t *testing.T) {
	// No public origin is configured: the stored-data seam must not consult one.
	previousServiceConfig := service.Config
	service.Config = nil
	t.Cleanup(func() { service.Config = previousServiceConfig })

	var db *gorm.DB
	err := runStartupGate(t, func(seeded *gorm.DB) {
		db = seeded
		if err := seeded.AutoMigrate(&model.Version{}, &model.CustomBuild{}, &model.CustomPreset{}); err != nil {
			t.Fatalf("create install tables: %v", err)
		}
		if err := seeded.Create(&model.Version{Version: preUpgradeTipVersion}).Error; err != nil {
			t.Fatalf("seed version %d: %v", preUpgradeTipVersion, err)
		}
		relative := `{"app_icon_url":"/upload/icon.png","app_logo_url":"/upload/logo.png","privacy_screen_url":"/upload/privacy.png"}`
		seedLegacyCustomJSON(t, seeded, "custom_builds", relative)
		seedLegacyCustomJSON(t, seeded, "custom_presets", relative)
	})
	if err != nil {
		t.Fatalf("DatabaseAutoUpdate() error = %v, want an accepted upgrade for stored relative images with no public origin", err)
	}
	var latest model.Version
	if err := db.Last(&latest).Error; err != nil {
		t.Fatalf("read latest database version: %v", err)
	}
	if latest.Version != DatabaseVersion {
		t.Fatalf("latest database version = %d, want %d after a 284 install upgraded", latest.Version, DatabaseVersion)
	}
}

// TestDatabaseAutoUpdateBrandingPreflightGate confirms the version gate the
// fix relies on: an install below the pre-upgrade tip and a fresh database run
// the branding preflight, while an install already at the current version does
// not re-inspect stored rows.
func TestDatabaseAutoUpdateBrandingPreflightGate(t *testing.T) {
	badRow := `{"company_name":"Acme\nLtd"}`
	cases := []struct {
		name            string
		seedVersion     *uint
		wantInvalidRows bool
	}{
		{"fresh-database-runs-preflight", nil, true},
		{"version-283-runs-preflight", ptr(uint(283)), true},
		{"version-284-runs-preflight", ptr(uint(preUpgradeTipVersion)), true},
		{"version-285-skips-preflight", ptr(uint(DatabaseVersion)), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runStartupGate(t, func(seeded *gorm.DB) {
				tables := []any{&model.CustomBuild{}}
				if tc.seedVersion != nil {
					tables = append(tables, &model.Version{})
				}
				if err := seeded.AutoMigrate(tables...); err != nil {
					t.Fatalf("create install tables: %v", err)
				}
				if tc.seedVersion != nil {
					if err := seeded.Create(&model.Version{Version: *tc.seedVersion}).Error; err != nil {
						t.Fatalf("seed version %d: %v", *tc.seedVersion, err)
					}
				}
				seedLegacyCustomJSON(t, seeded, "custom_builds", badRow)
			})
			if tc.wantInvalidRows && !errors.Is(err, errInvalidCustomBrandingFields) {
				t.Fatalf("DatabaseAutoUpdate() error = %v, want errInvalidCustomBrandingFields", err)
			}
			if !tc.wantInvalidRows && err != nil {
				t.Fatalf("DatabaseAutoUpdate() error = %v, want an already-current install left untouched", err)
			}
		})
	}
}

func ptr[T any](value T) *T {
	return &value
}
