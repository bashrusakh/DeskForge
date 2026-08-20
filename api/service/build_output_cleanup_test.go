package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rustdesk-server/api/model"

	"gorm.io/gorm"
)

func TestSweepBuildOutputTempsRemovesOnlyStaleInactiveTemps(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(2_000, 0)
	ttl := time.Hour
	old := now.Add(-2 * ttl)
	fresh := now.Add(-ttl / 2)

	staleStaging := filepath.Join(root, ".7-artifact-stale")
	if err := os.MkdirAll(staleStaging, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleStaging, "partial.bin"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	staleArchive := filepath.Join(root, ".8-download-stale.zip.part")
	if err := os.WriteFile(staleArchive, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	staleCompletedArchive := filepath.Join(root, ".8-download-stale.zip")
	if err := os.WriteFile(staleCompletedArchive, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	protectedCurrent := filepath.Join(root, ".12-download-current.zip.part")
	if err := os.WriteFile(protectedCurrent, []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	freshStaging := filepath.Join(root, ".9-artifact-fresh")
	if err := os.Mkdir(freshStaging, 0755); err != nil {
		t.Fatal(err)
	}
	activeStaging := filepath.Join(root, ".10-artifact-active")
	if err := os.Mkdir(activeStaging, 0755); err != nil {
		t.Fatal(err)
	}
	currentOutput := filepath.Join(root, "11")
	if err := os.Mkdir(currentOutput, 0755); err != nil {
		t.Fatal(err)
	}
	staleNestedPart := filepath.Join(currentOutput, ".download-interrupted.zip.part")
	if err := os.WriteFile(staleNestedPart, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	staleNestedArchive := filepath.Join(currentOutput, ".archive-interrupted.zip")
	if err := os.WriteFile(staleNestedArchive, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	staleNestedRecovery := filepath.Join(currentOutput, ".artifact-recovery-interrupted")
	if err := os.MkdirAll(staleNestedRecovery, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleNestedRecovery, "temporary-secret.bin"), []byte("recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	freshNestedRecovery := filepath.Join(currentOutput, ".artifact-recovery-active")
	if err := os.MkdirAll(freshNestedRecovery, 0755); err != nil {
		t.Fatal(err)
	}
	freshRecoveryFile := filepath.Join(freshNestedRecovery, "current.bin")
	if err := os.WriteFile(freshRecoveryFile, []byte("active recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	activeNested := filepath.Join(activeStaging, ".download-active.zip.part")
	if err := os.WriteFile(activeNested, []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	staleSnapshot := filepath.Join(root, ".13-snapshot-interrupted")
	if err := os.MkdirAll(staleSnapshot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleSnapshot, "secret.bin"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	activeSnapshot := filepath.Join(root, ".14-snapshot-current")
	if err := os.MkdirAll(activeSnapshot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeSnapshot, "current.bin"), []byte("active snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	freshSnapshot := filepath.Join(root, ".15-snapshot-fresh")
	if err := os.MkdirAll(freshSnapshot, 0700); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(root, ".not-a-build-temp")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{staleStaging, staleArchive, staleCompletedArchive, protectedCurrent, staleNestedPart, staleNestedArchive, staleNestedRecovery, activeNested, staleSnapshot, activeSnapshot} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(filepath.Join(staleNestedRecovery, "temporary-secret.bin"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(freshNestedRecovery, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(freshRecoveryFile, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(freshStaging, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(activeStaging, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(freshSnapshot, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	ProtectGithubArtifactTemp(protectedCurrent)
	t.Cleanup(func() { ReleaseGithubArtifactTemp(protectedCurrent) })
	ProtectBuildOutputSnapshot(activeSnapshot)
	t.Cleanup(func() { ReleaseBuildOutputSnapshot(activeSnapshot) })
	if err := os.Chtimes(currentOutput, old, old); err != nil {
		t.Fatal(err)
	}

	if err := SweepBuildOutputTemps(root, now, ttl, map[uint]struct{}{10: {}}); err != nil {
		t.Fatalf("SweepBuildOutputTemps() error = %v", err)
	}
	for _, path := range []string{staleStaging, staleArchive, staleCompletedArchive, staleNestedPart, staleNestedArchive, staleNestedRecovery, staleSnapshot} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale temp %q stat = %v, want removed", path, err)
		}
	}
	for _, path := range []string{freshStaging, activeStaging, activeNested, currentOutput, freshNestedRecovery, freshRecoveryFile, protectedCurrent, activeSnapshot, freshSnapshot, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected/fresh path %q stat = %v, want preserved", path, err)
		}
	}
}

func TestSweepGithubArtifactTempsRemovesOnlyStaleUnprotectedArchives(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(2_000, 0)
	ttl := time.Hour
	old := now.Add(-2 * ttl)
	fresh := now.Add(-ttl / 2)

	stalePart := filepath.Join(root, "deskforge-artifact-abcdef.part")
	staleArchive := filepath.Join(root, "deskforge-artifact-123456.zip")
	freshPart := filepath.Join(root, "deskforge-artifact-fedcba.part")
	activeArchive := filepath.Join(root, "deskforge-artifact-654321.zip")
	unrelatedPrefix := filepath.Join(root, "deskforge-artifact-provider.part")
	unrelatedFile := filepath.Join(root, "other-service-abcdef.part")
	for _, path := range []string{stalePart, staleArchive, freshPart, activeArchive, unrelatedPrefix, unrelatedFile} {
		if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{stalePart, staleArchive, activeArchive, unrelatedPrefix, unrelatedFile} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(freshPart, fresh, fresh); err != nil {
		t.Fatal(err)
	}

	ProtectGithubArtifactTemp(activeArchive)
	t.Cleanup(func() { ReleaseGithubArtifactTemp(activeArchive) })
	if err := SweepGithubArtifactTemps(root, now, ttl); err != nil {
		t.Fatalf("SweepGithubArtifactTemps() error = %v", err)
	}
	for _, path := range []string{stalePart, staleArchive} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale provider temp %q stat = %v, want removed", path, err)
		}
	}
	for _, path := range []string{freshPart, activeArchive, unrelatedPrefix, unrelatedFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("fresh/protected/unrelated path %q stat = %v, want preserved", path, err)
		}
	}
}

func TestSweepBuildOutputDeletionTombstonesRetriesOnlyDeletedBuilds(t *testing.T) {
	db := newCustomPersistenceDB(t)
	root := t.TempDir()
	now := time.Unix(2_000, 0)
	ttl := time.Hour
	old := now.Add(-2 * ttl)
	fresh := now.Add(-ttl / 2)

	deletedID := uint(41)
	rowID := uint(42)
	activeID := uint(43)
	for _, id := range []uint{deletedID, rowID, activeID} {
		marker := buildOutputDeletionTombstonePath(root, id)
		if err := os.WriteFile(marker, []byte(fmt.Sprintf("%d\n", id)), 0600); err != nil {
			t.Fatalf("write tombstone %d: %v", id, err)
		}
		if err := os.Chtimes(marker, old, old); err != nil {
			t.Fatalf("age tombstone %d: %v", id, err)
		}
		outDir := filepath.Join(root, fmt.Sprint(id))
		if err := os.MkdirAll(outDir, 0700); err != nil {
			t.Fatalf("create output %d: %v", id, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, "artifact.bin"), []byte("artifact"), 0600); err != nil {
			t.Fatalf("write output %d: %v", id, err)
		}
	}
	if err := db.Create(&model.CustomBuild{IdModel: model.IdModel{Id: rowID}}).Error; err != nil {
		t.Fatalf("create retained row: %v", err)
	}

	if err := SweepBuildOutputTemps(root, now, ttl, map[uint]struct{}{activeID: {}}); err != nil {
		t.Fatalf("SweepBuildOutputTemps() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, fmt.Sprint(deletedID)),
		buildOutputDeletionTombstonePath(root, deletedID),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted build path %q stat = %v, want removed", path, err)
		}
	}
	for _, id := range []uint{rowID, activeID} {
		for _, path := range []string{filepath.Join(root, fmt.Sprint(id)), buildOutputDeletionTombstonePath(root, id)} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("protected build path %q stat = %v, want preserved", path, err)
			}
		}
	}

	freshID := uint(44)
	freshMarker := buildOutputDeletionTombstonePath(root, freshID)
	if err := os.WriteFile(freshMarker, []byte(fmt.Sprintf("%d\n", freshID)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(freshMarker, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	if err := SweepBuildOutputTemps(root, now, ttl, nil); err != nil {
		t.Fatalf("fresh tombstone sweep error = %v", err)
	}
	if _, err := os.Stat(freshMarker); err != nil {
		t.Fatalf("fresh tombstone stat = %v, want preserved", err)
	}
}

func TestSweepBuildOutputDeletionTombstonesWaitsForLifecycleLock(t *testing.T) {
	db := newCustomPersistenceDB(t)
	root := t.TempDir()
	now := time.Unix(2_000, 0)
	ttl := time.Hour
	buildID := uint(61)
	if err := db.Create(&model.CustomBuild{IdModel: model.IdModel{Id: buildID}}).Error; err != nil {
		t.Fatalf("create retained row: %v", err)
	}
	marker := buildOutputDeletionTombstonePath(root, buildID)
	if err := os.WriteFile(marker, []byte(fmt.Sprintf("%d\n", buildID)), 0600); err != nil {
		t.Fatalf("write tombstone: %v", err)
	}
	if err := os.Chtimes(marker, now.Add(-2*ttl), now.Add(-2*ttl)); err != nil {
		t.Fatalf("age tombstone: %v", err)
	}

	buildOutputLifecycleMu.Lock()
	result := make(chan error, 1)
	go func() { result <- SweepBuildOutputTemps(root, now, ttl, nil) }()
	select {
	case err := <-result:
		buildOutputLifecycleMu.Unlock()
		t.Fatalf("sweep completed while lifecycle lock was held: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	buildOutputLifecycleMu.Unlock()
	if err := <-result; err != nil {
		t.Fatalf("SweepBuildOutputTemps() error = %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("retained-row tombstone stat = %v, want preserved", err)
	}
}

func TestSweepBuildOutputDeletionTombstonesSkipsMalformedAndFailsClosed(t *testing.T) {
	newCustomPersistenceDB(t)
	root := t.TempDir()
	now := time.Unix(2_000, 0)
	ttl := time.Hour
	old := now.Add(-2 * ttl)
	malformed := filepath.Join(root, ".deskforge-build-delete-51.tombstone")
	if err := os.WriteFile(malformed, []byte("not-51\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(malformed, old, old); err != nil {
		t.Fatal(err)
	}
	unknownNumeric := filepath.Join(root, "52")
	if err := os.MkdirAll(unknownNumeric, 0700); err != nil {
		t.Fatal(err)
	}
	if err := SweepBuildOutputTemps(root, now, ttl, nil); err != nil {
		t.Fatalf("malformed tombstone sweep error = %v", err)
	}
	for _, path := range []string{malformed, unknownNumeric} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("malformed/unknown path %q stat = %v, want preserved", path, err)
		}
	}

	newClosedCustomPersistenceDB(t)
	closedMarker := filepath.Join(root, ".deskforge-build-delete-53.tombstone")
	if err := os.WriteFile(closedMarker, []byte("53\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(closedMarker, old, old); err != nil {
		t.Fatal(err)
	}
	if err := SweepBuildOutputTemps(root, now, ttl, nil); err == nil {
		t.Fatal("closed database sweep error = nil, want fail-closed error")
	}
	if _, err := os.Stat(closedMarker); err != nil {
		t.Fatalf("closed database marker stat = %v, want preserved", err)
	}
}

func newClosedCustomPersistenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newCustomPersistenceDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	return db
}
