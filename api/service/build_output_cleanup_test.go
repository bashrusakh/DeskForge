package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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
