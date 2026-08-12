package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var githubArtifactTempPattern = regexp.MustCompile(`^deskforge-artifact-[A-Za-z0-9]{6}\.(?:part|zip)$`)
var buildOutputNestedTempPattern = regexp.MustCompile(`^\.(?:[0-9]+-)?(?:download|archive|artifact)-[A-Za-z0-9._-]+\.(?:part|zip)$`)
var buildOutputRecoveryPattern = regexp.MustCompile(`^\.artifact-recovery-[A-Za-z0-9._-]+$`)
var buildOutputSnapshotPattern = regexp.MustCompile(`^\.[0-9]+-snapshot-[A-Za-z0-9._-]+$`)

const maxNestedBuildOutputTempEntries = 256

var activeGithubArtifactTemps = struct {
	sync.Mutex
	paths map[string]struct{}
}{
	paths: make(map[string]struct{}),
}

var activeBuildOutputSnapshots = struct {
	sync.Mutex
	paths map[string]struct{}
}{
	paths: make(map[string]struct{}),
}

// ProtectBuildOutputSnapshot marks an export source snapshot as active until
// its caller has finished packaging and removed it.
func ProtectBuildOutputSnapshot(path string) {
	path = filepath.Clean(path)
	activeBuildOutputSnapshots.Lock()
	activeBuildOutputSnapshots.paths[path] = struct{}{}
	activeBuildOutputSnapshots.Unlock()
}

// ReleaseBuildOutputSnapshot removes active protection after an export
// snapshot has been cleaned up. It is safe to call for an unknown path.
func ReleaseBuildOutputSnapshot(path string) {
	path = filepath.Clean(path)
	activeBuildOutputSnapshots.Lock()
	delete(activeBuildOutputSnapshots.paths, path)
	activeBuildOutputSnapshots.Unlock()
}

func isProtectedBuildOutputSnapshot(path string) bool {
	path = filepath.Clean(path)
	activeBuildOutputSnapshots.Lock()
	_, protected := activeBuildOutputSnapshots.paths[path]
	activeBuildOutputSnapshots.Unlock()
	return protected
}

// ProtectGithubArtifactTemp marks a provider archive as owned by an active
// download until the caller explicitly releases it.
func ProtectGithubArtifactTemp(path string) {
	path = filepath.Clean(path)
	activeGithubArtifactTemps.Lock()
	activeGithubArtifactTemps.paths[path] = struct{}{}
	activeGithubArtifactTemps.Unlock()
}

// ReleaseGithubArtifactTemp removes a provider archive from the active-path
// protection set after its caller has finished with it. It is safe to call for
// paths that were not created by DownloadArtifact.
func ReleaseGithubArtifactTemp(path string) {
	path = filepath.Clean(path)
	activeGithubArtifactTemps.Lock()
	delete(activeGithubArtifactTemps.paths, path)
	activeGithubArtifactTemps.Unlock()
}

func isProtectedGithubArtifactTemp(path string) bool {
	path = filepath.Clean(path)
	activeGithubArtifactTemps.Lock()
	_, protected := activeGithubArtifactTemps.paths[path]
	activeGithubArtifactTemps.Unlock()
	return protected
}

// SweepBuildOutputTemps removes only stale temporary artifact files and
// staging directories directly below outputRoot. Final numeric build
// directories are never candidates. activeBuildIDs protects temporary paths
// for builds that are currently in an asynchronous lifecycle state.
func SweepBuildOutputTemps(outputRoot string, now time.Time, ttl time.Duration, activeBuildIDs map[uint]struct{}) error {
	if outputRoot == "" {
		return errors.New("build output root is required")
	}
	if ttl <= 0 {
		return errors.New("build output temporary-file TTL must be positive")
	}

	rootInfo, err := os.Lstat(outputRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect build output root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("build output root is not a regular directory")
	}

	entries, err := os.ReadDir(outputRoot)
	if err != nil {
		return fmt.Errorf("read build output root: %w", err)
	}
	var cleanupErr error
	for _, entry := range entries {
		if entry.IsDir() {
			buildID, buildDir := parseBuildOutputDirectory(entry.Name())
			if buildDir {
				if _, active := activeBuildIDs[buildID]; active {
					continue
				}
				path := filepath.Join(outputRoot, entry.Name())
				if err := sweepNestedBuildOutputTemps(path, now, ttl); err != nil {
					cleanupErr = errors.Join(cleanupErr, err)
				}
				continue
			}
		}
		buildID, candidate := buildOutputTempCandidate(entry.Name(), entry.IsDir())
		if !candidate {
			continue
		}
		if buildID > 0 {
			if _, active := activeBuildIDs[buildID]; active {
				continue
			}
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect stale build output temp %q: %w", entry.Name(), err))
			continue
		}
		if now.Sub(info.ModTime()) < ttl {
			continue
		}
		path := filepath.Join(outputRoot, entry.Name())
		if isProtectedBuildOutputSnapshot(path) {
			continue
		}
		if isProtectedGithubArtifactTemp(path) {
			continue
		}
		pathInfo, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect cleanup target %q: %w", path, err))
			continue
		}
		if pathInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if pathInfo.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove stale build output temp %q: %w", path, err))
		}
	}
	return cleanupErr
}

func parseBuildOutputDirectory(name string) (uint, bool) {
	buildID, err := strconv.ParseUint(name, 10, 0)
	return uint(buildID), err == nil && buildID > 0
}

// sweepNestedBuildOutputTemps is deliberately one level deep except for an
// explicitly service-owned recovery directory. The numeric directory is a
// published-output boundary, so ordinary files are never candidates. A hard
// entry bound prevents a malformed build directory or recovery tree from
// turning startup cleanup into an unbounded traversal.
func sweepNestedBuildOutputTemps(buildDir string, now time.Time, ttl time.Duration) error {
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return fmt.Errorf("read build output temp directory %q: %w", buildDir, err)
	}
	if len(entries) > maxNestedBuildOutputTempEntries {
		return fmt.Errorf("build output temp directory %q contains too many entries: %d", buildDir, len(entries))
	}
	var cleanupErr error
	for _, entry := range entries {
		isNestedTemp := buildOutputNestedTempPattern.MatchString(entry.Name())
		isRecoveryPath := buildOutputRecoveryPattern.MatchString(entry.Name())
		if !isNestedTemp && !isRecoveryPath {
			continue
		}
		path := filepath.Join(buildDir, entry.Name())
		if isProtectedGithubArtifactTemp(path) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect stale nested build output temp %q: %w", path, err))
			continue
		}
		if now.Sub(info.ModTime()) < ttl {
			continue
		}
		pathInfo, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect nested build output cleanup target %q: %w", path, err))
			continue
		}
		if pathInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if pathInfo.IsDir() {
			if !isRecoveryPath {
				continue
			}
			removed, err := removeBoundedRecoveryDirectory(path, now, ttl)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove stale nested artifact recovery %q: %w", path, err))
			}
			if !removed {
				continue
			}
			continue
		}
		if !pathInfo.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove stale nested build output temp %q: %w", path, err))
		}
	}
	return cleanupErr
}

func removeBoundedRecoveryDirectory(path string, now time.Time, ttl time.Duration) (bool, error) {
	entries := 0
	stale := true
	err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > maxNestedBuildOutputTempEntries {
			return fmt.Errorf("artifact recovery directory contains too many entries: %d", entries)
		}
		if entry.Type()&os.ModeSymlink == 0 {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if now.Sub(info.ModTime()) < ttl {
				stale = false
			}
		}
		if current != path && entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if !stale {
		return false, nil
	}
	return true, os.RemoveAll(path)
}

// SweepGithubArtifactTemps removes only stale provider download archives from
// tempRoot. The exact CreateTemp filename shape is required so unrelated temp
// files are never swept. Archives still owned by an active download remain
// protected until ReleaseGithubArtifactTemp is called.
func SweepGithubArtifactTemps(tempRoot string, now time.Time, ttl time.Duration) error {
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	if ttl <= 0 {
		return errors.New("GitHub artifact temporary-file TTL must be positive")
	}

	rootInfo, err := os.Lstat(tempRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect GitHub artifact temp root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("GitHub artifact temp root is not a regular directory")
	}

	entries, err := os.ReadDir(tempRoot)
	if err != nil {
		return fmt.Errorf("read GitHub artifact temp root: %w", err)
	}
	var cleanupErr error
	for _, entry := range entries {
		if entry.IsDir() || !githubArtifactTempPattern.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(tempRoot, entry.Name())
		if isProtectedGithubArtifactTemp(path) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect stale GitHub artifact temp %q: %w", entry.Name(), err))
			continue
		}
		if now.Sub(info.ModTime()) < ttl {
			continue
		}
		pathInfo, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect GitHub artifact cleanup target %q: %w", path, err))
			continue
		}
		if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove stale GitHub artifact temp %q: %w", path, err))
		}
	}
	return cleanupErr
}

func buildOutputTempCandidate(name string, isDir bool) (uint, bool) {
	if buildOutputSnapshotPattern.MatchString(name) {
		buildID, _, _ := strings.Cut(strings.TrimPrefix(name, "."), "-snapshot-")
		parsed, err := strconv.ParseUint(buildID, 10, 0)
		return uint(parsed), err == nil && parsed > 0 && isDir
	}
	if !strings.HasPrefix(name, ".") {
		return 0, false
	}
	rest := strings.TrimPrefix(name, ".")
	dash := strings.IndexByte(rest, '-')
	if dash <= 0 {
		return 0, false
	}
	buildID, err := strconv.ParseUint(rest[:dash], 10, 0)
	if err != nil || buildID == 0 {
		return 0, false
	}
	kind := rest[dash+1:]
	if strings.HasPrefix(kind, "artifact-recovery-") {
		return uint(buildID), buildOutputRecoveryPattern.MatchString("." + kind)
	}
	if strings.HasPrefix(kind, "artifact-") {
		if isDir {
			return uint(buildID), true
		}
		return uint(buildID), strings.HasSuffix(kind, ".part") || strings.HasSuffix(kind, ".zip")
	}
	if strings.HasPrefix(kind, "download-") || strings.HasPrefix(kind, "archive-") {
		return uint(buildID), strings.HasSuffix(kind, ".part") || strings.HasSuffix(kind, ".zip")
	}
	return 0, false
}
