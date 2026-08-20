package service

import (
	"sync"
	"testing"
	"time"

	"rustdesk-server/api/model"

	"gorm.io/gorm"
)

func newCustomPresetForUpsert(userID uint, name, version, customJSON string) *model.CustomPreset {
	return &model.CustomPreset{
		UserId:     userID,
		Name:       name,
		Platform:   "windows",
		Version:    version,
		AppName:    "rustqs",
		CustomJson: customJSON,
	}
}

func TestCustomPresetCreateUpsertsSameOwnerName(t *testing.T) {
	db := newCustomPersistenceDB(t)
	service := &CustomPresetService{}

	first := newCustomPresetForUpsert(7, "shared", "1.0.0", `{"enable_audio":true}`)
	if err := service.Create(first); err != nil {
		t.Fatalf("create first preset: %v", err)
	}
	unrelated := newCustomPresetForUpsert(8, "other", "1.0.0", `{"enable_audio":true}`)
	if err := service.Create(unrelated); err != nil {
		t.Fatalf("create unrelated preset: %v", err)
	}

	replacement := newCustomPresetForUpsert(7, "shared", "2.0.0", `{"enable_audio":false}`)
	replacement.IdModel = model.IdModel{Id: unrelated.Id}
	if err := service.Create(replacement); err != nil {
		t.Fatalf("upsert replacement preset: %v", err)
	}
	if replacement.Id != first.Id {
		t.Fatalf("replacement ID = %d, want existing row ID %d", replacement.Id, first.Id)
	}

	var rows []model.CustomPreset
	if err := db.Where("user_id = ? AND name = ?", 7, "shared").Find(&rows).Error; err != nil {
		t.Fatalf("read owner/name rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("owner/name row count = %d, want 1", len(rows))
	}
	if rows[0].Id != first.Id || rows[0].Version != "2.0.0" {
		t.Fatalf("persisted replacement = %#v, want ID %d and version 2.0.0", rows[0], first.Id)
	}
	var stillUnrelated model.CustomPreset
	if err := db.First(&stillUnrelated, unrelated.Id).Error; err != nil {
		t.Fatalf("read unrelated preset: %v", err)
	}
	if stillUnrelated.UserId != unrelated.UserId || stillUnrelated.Name != unrelated.Name {
		t.Fatalf("unrelated preset was changed: %#v", stillUnrelated)
	}
}

func TestCustomPresetCreateAllowsSameNameForDifferentUsers(t *testing.T) {
	db := newCustomPersistenceDB(t)
	service := &CustomPresetService{}

	first := newCustomPresetForUpsert(7, "shared", "1.0.0", `{"enable_audio":true}`)
	second := newCustomPresetForUpsert(8, "shared", "1.0.0", `{"enable_audio":false}`)
	if err := service.Create(first); err != nil {
		t.Fatalf("create first user preset: %v", err)
	}
	if err := service.Create(second); err != nil {
		t.Fatalf("create second user preset: %v", err)
	}
	if first.Id == 0 || second.Id == 0 || first.Id == second.Id {
		t.Fatalf("preset IDs = %d, %d; want distinct persisted rows", first.Id, second.Id)
	}
	var count int64
	if err := db.Model(&model.CustomPreset{}).Where("name = ?", "shared").Count(&count).Error; err != nil {
		t.Fatalf("count same-name presets: %v", err)
	}
	if count != 2 {
		t.Fatalf("same-name preset count = %d, want 2", count)
	}
}

func TestCustomPresetCreateConcurrentSameOwnerNameKeepsOneRow(t *testing.T) {
	db := newCustomPersistenceDB(t)
	const callbackName = "test:custom-preset-upsert-barrier"
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWrites := func() {
		releaseOnce.Do(func() { close(release) })
	}
	if err := db.Callback().Create().Before("gorm:begin_transaction").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "custom_presets" {
			return
		}
		ready <- struct{}{}
		<-release
	}); err != nil {
		t.Fatalf("register create barrier: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	var writes sync.WaitGroup
	t.Cleanup(func() {
		releaseWrites()
		writes.Wait()
	})

	type result struct {
		preset *model.CustomPreset
		err    error
	}
	results := make(chan result, 2)
	service := &CustomPresetService{}
	for _, preset := range []*model.CustomPreset{
		newCustomPresetForUpsert(7, "concurrent", "1.0.0", `{"enable_audio":true}`),
		newCustomPresetForUpsert(7, "concurrent", "2.0.0", `{"enable_audio":false}`),
	} {
		writes.Add(1)
		go func(preset *model.CustomPreset) {
			defer writes.Done()
			results <- result{preset: preset, err: service.Create(preset)}
		}(preset)
	}

	for range 2 {
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent writes did not reach the create barrier")
		}
	}
	releaseWrites()

	completed := make([]result, 0, 2)
	for range 2 {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("concurrent Create() error = %v", result.err)
			}
			completed = append(completed, result)
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent Create() did not complete")
		}
	}

	var rows []model.CustomPreset
	if err := db.Where("user_id = ? AND name = ?", 7, "concurrent").Find(&rows).Error; err != nil {
		t.Fatalf("read concurrent owner/name rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("concurrent owner/name row count = %d, want 1", len(rows))
	}
	for _, result := range completed {
		if result.preset.Id != rows[0].Id {
			t.Fatalf("returned ID = %d, want persisted row ID %d", result.preset.Id, rows[0].Id)
		}
	}
}
