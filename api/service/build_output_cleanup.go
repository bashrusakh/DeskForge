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

	"rustdesk-server/api/model"

	"gorm.io/gorm"
)

var githubArtifactTempPattern = regexp.MustCompile(`^deskforge-artifact-[A-Za-z0-9]{6}\.(?:part|zip)$`)
var buildOutputNestedTempPattern = regexp.MustCompile(`^\.(?:[0-9]+-)?(?:download|archive|artifact)-[A-Za-z0-9._-]+\.(?:part|zip)$`)
var buildOutputRecoveryPattern = regexp.MustCompile(`^\.artifact-recovery-[A-Za-z0-9._-]+$`)
var buildOutputSnapshotPattern = regexp.MustCompile(`^\.[0-9]+-snapshot-[A-Za-z0-9._-]+$`)
var buildOutputDeletionTombstonePattern = regexp.MustCompile(`^\.deskforge-build-delete-([1-9][0-9]*)\.tombstone$`)
var buildOutputDeletionTombstoneTempPattern = regexp.MustCompile(`^\.deskforge-build-delete-[A-Za-z0-9]{6}\.tmp$`)

const maxNestedBuildOutputTempEntries = 256
const maxBuildOutputDeletionTombstones = 256

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

func buildOutputDeletionTombstonePath(outputRoot string, id uint) string {
	return filepath.Join(outputRoot, fmt.Sprintf(".deskforge-build-delete-%d.tombstone", id))
}

// ensureBuildOutputDeletionTombstone durably records deletion intent before
// the database row is removed. Existing valid markers are reused so retries do
// not create a second intent.
func ensureBuildOutputDeletionTombstone(outputRoot string, id uint) (string, error) {
	if outputRoot == "" || id == 0 {
		return "", errors.New("build deletion tombstone requires an output root and positive id")
	}
	if err := os.MkdirAll(outputRoot, 0700); err != nil {
		return "", fmt.Errorf("create build output root for deletion tombstone: %w", err)
	}
	rootInfo, err := os.Lstat(outputRoot)
	if err != nil {
		return "", fmt.Errorf("inspect build output root for deletion tombstone: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("build output root for deletion tombstone is not a regular directory")
	}
	path := buildOutputDeletionTombstonePath(outputRoot, id)
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", errors.New("build deletion tombstone is not a regular file")
		}
		if err := validateBuildOutputDeletionTombstone(path, id); err != nil {
			return "", err
		}
		if err := os.Chmod(path, 0600); err != nil {
			return "", fmt.Errorf("restrict build deletion tombstone permissions: %w", err)
		}
		return path, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect build deletion tombstone: %w", err)
	}
	file, err := os.CreateTemp(outputRoot, ".deskforge-build-delete-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create build deletion tombstone: %w", err)
	}
	tempPath := file.Name()
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("restrict build deletion tombstone permissions: %w", err)
	}
	content := []byte(fmt.Sprintf("%d\n", id))
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("write build deletion tombstone: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("sync build deletion tombstone: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("close build deletion tombstone: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		if _, statErr := os.Lstat(path); statErr == nil {
			_ = os.Remove(tempPath)
			if validateErr := validateBuildOutputDeletionTombstone(path, id); validateErr != nil {
				return "", validateErr
			}
			return path, nil
		}
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("publish build deletion tombstone: %w", err)
	}
	if err := syncBuildOutputDirectory(outputRoot); err != nil {
		return "", fmt.Errorf("sync build output root after deletion tombstone: %w", err)
	}
	return path, nil
}

func syncBuildOutputDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func validateBuildOutputDeletionTombstone(path string, id uint) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read build deletion tombstone: %w", err)
	}
	if string(content) != fmt.Sprintf("%d\n", id) {
		return errors.New("build deletion tombstone does not match its filename")
	}
	return nil
}

var removeBuildOutputDeletionTombstone = os.Remove

func removeBuildOutputDeletionTombstoneIfPresent(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("build deletion tombstone is not a regular file")
	}
	if err := removeBuildOutputDeletionTombstone(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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
	cleanupErr := sweepBuildOutputDeletionTombstones(outputRoot, entries, now, ttl, activeBuildIDs)
	for _, entry := range entries {
		if entry.IsDir() {
			buildID, buildDir := parseBuildOutputDirectory(entry.Name())
			if buildDir {
				if _, active := activeBuildIDs[buildID]; active {
					continue
				}
				path := filepath.Join(outputRoot, entry.Name())
				if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
					continue
				}
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

func sweepBuildOutputDeletionTombstones(outputRoot string, entries []os.DirEntry, now time.Time, ttl time.Duration, activeBuildIDs map[uint]struct{}) error {
	var cleanupErr error
	candidates := 0
	for _, entry := range entries {
		if buildOutputDeletionTombstoneTempPattern.MatchString(entry.Name()) {
			path := filepath.Join(outputRoot, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect build deletion tombstone temp %q: %w", path, err))
				}
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || now.Sub(info.ModTime()) < ttl {
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove stale build deletion tombstone temp %q: %w", path, err))
			}
			continue
		}
		matches := buildOutputDeletionTombstonePattern.FindStringSubmatch(entry.Name())
		if len(matches) != 2 {
			continue
		}
		candidates++
		if candidates > maxBuildOutputDeletionTombstones {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("build output deletion tombstone limit exceeded: %d", maxBuildOutputDeletionTombstones))
			break
		}
		parsed, err := strconv.ParseUint(matches[1], 10, 0)
		if err != nil || parsed == 0 {
			continue
		}
		buildID := uint(parsed)
		if _, active := activeBuildIDs[buildID]; active {
			continue
		}
		path := filepath.Join(outputRoot, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("inspect build deletion tombstone %q: %w", path, err))
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		if now.Sub(info.ModTime()) < ttl {
			continue
		}
		if err := validateBuildOutputDeletionTombstone(path, buildID); err != nil {
			continue
		}
		if DB == nil {
			cleanupErr = errors.Join(cleanupErr, errors.New("database is unavailable for build deletion tombstone cleanup"))
			continue
		}
		cleanupErr = errors.Join(cleanupErr, func() error {
			buildOutputLifecycleMu.Lock()
			defer buildOutputLifecycleMu.Unlock()

			// Recheck immediately under the shared lifecycle lock. This is the
			// final authority check before exact output and marker removal.
			var build model.CustomBuild
			err = DB.First(&build, buildID).Error
			if err == nil {
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("verify deleted custom build %d: %w", buildID, err)
			}
			outputDir := filepath.Join(outputRoot, strconv.FormatUint(parsed, 10))
			outputInfo, err := os.Lstat(outputDir)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("inspect deleted custom build output %q: %w", outputDir, err)
			}
			if err == nil {
				if outputInfo.Mode()&os.ModeSymlink != 0 || !outputInfo.IsDir() {
					return nil
				}
				if err := removeBuildOutputDir(outputDir); err != nil {
					return fmt.Errorf("remove deleted custom build output %q: %w", outputDir, err)
				}
				if _, err := os.Lstat(outputDir); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("verify deleted custom build output %q: %w", outputDir, err)
				}
			}
			if err := removeBuildOutputDeletionTombstoneIfPresent(path); err != nil {
				return fmt.Errorf("remove build deletion tombstone %q: %w", path, err)
			}
			return nil
		}())
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
