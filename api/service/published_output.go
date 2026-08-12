package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"rustdesk-server/api/model"
)

const (
	maxPublishedOutputFiles      = 4096
	maxPublishedOutputFileBytes  = uint64(512 << 20)
	maxPublishedOutputTotalBytes = uint64(1 << 30)
)

type publishedOutputFile struct {
	name string
	size int64
}

const publishedOutputPublicationContext = "github-artifact-canonical-output-v2"

var (
	publishedOutputExportMu    sync.Mutex
	publishedOutputEntriesHook = func() {}
)

// WithPublishedOutputExportLock serializes bounded reads of canonical output
// with handoff and archive packaging. Writers outside this process still have
// to be detected by the caller's before/after digest comparison.
func WithPublishedOutputExportLock(operation func() error) error {
	if operation == nil {
		return errors.New("published output export operation is required")
	}
	publishedOutputExportMu.Lock()
	defer publishedOutputExportMu.Unlock()
	return operation()
}

// publishedOutputManifest validates the canonical flat output and computes a
// deterministic SHA-256 over the immutable publication identity followed by
// relative names, sizes, and contents in name order. The provider does not
// cryptographically sign this value; binding it here prevents a valid output
// from being reused for another build, run, or artifact.
func publishedOutputManifest(outputDir string, build *model.CustomBuild) (int64, string, error) {
	if outputDir == "" {
		return 0, "", errors.New("published output directory is required")
	}
	if build == nil {
		return 0, "", errors.New("build record is required")
	}
	info, err := os.Lstat(outputDir)
	if err != nil {
		return 0, "", fmt.Errorf("inspect final artifact output: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, "", errors.New("final artifact output is not a regular directory")
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return 0, "", fmt.Errorf("read final artifact output: %w", err)
	}
	if len(entries) == 0 {
		return 0, "", errors.New("final artifact output contains no published files")
	}
	if len(entries) > maxPublishedOutputFiles {
		return 0, "", fmt.Errorf("final artifact output contains too many files: %d", len(entries))
	}

	appName := build.AppName
	if err := ValidateOutputAppName(appName); err != nil {
		return 0, "", err
	}
	files := make([]publishedOutputFile, 0, len(entries))
	seenWindowsNames := make(map[string]string)
	var executableSize int64
	foundExecutable := false
	var totalBytes uint64
	for _, entry := range entries {
		if build.Platform == string(PlatformWindows) {
			if err := ValidateWindowsArtifactFilename(entry.Name()); err != nil {
				return 0, "", fmt.Errorf("invalid published Windows artifact %q: %w", entry.Name(), err)
			}
			nameKey := WindowsArtifactNameKey(entry.Name())
			if previous, exists := seenWindowsNames[nameKey]; exists {
				return 0, "", fmt.Errorf("published Windows artifact names collide case-insensitively: %q and %q", previous, entry.Name())
			}
			seenWindowsNames[nameKey] = entry.Name()
		}
		entryPath := filepath.Join(outputDir, entry.Name())
		entryInfo, err := os.Lstat(entryPath)
		if err != nil {
			return 0, "", fmt.Errorf("inspect final artifact %q: %w", entry.Name(), err)
		}
		if entryInfo.IsDir() || entryInfo.Mode()&os.ModeSymlink != 0 || !entryInfo.Mode().IsRegular() {
			return 0, "", fmt.Errorf("final artifact output contains non-regular entry %q", entry.Name())
		}
		if entryInfo.Size() < 0 || uint64(entryInfo.Size()) > maxPublishedOutputFileBytes {
			return 0, "", fmt.Errorf("final artifact %q exceeds published output file limit", entry.Name())
		}
		if totalBytes > maxPublishedOutputTotalBytes-uint64(entryInfo.Size()) {
			return 0, "", errors.New("final artifact output exceeds published aggregate limit")
		}
		totalBytes += uint64(entryInfo.Size())
		relativeName, err := filepath.Rel(outputDir, entryPath)
		if err != nil || relativeName == "." || strings.HasPrefix(relativeName, ".."+string(filepath.Separator)) || relativeName == ".." {
			return 0, "", fmt.Errorf("invalid published output name %q", entry.Name())
		}
		files = append(files, publishedOutputFile{name: filepath.ToSlash(relativeName), size: entryInfo.Size()})
		if strings.EqualFold(entry.Name(), appName+".exe") {
			executableSize = entryInfo.Size()
			foundExecutable = true
		}
	}
	if build.Platform == string(PlatformWindows) && !foundExecutable {
		return 0, "", fmt.Errorf("final artifact output is missing required %s.exe", appName)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	hasher := sha256.New()
	writeManifestString(hasher, publishedOutputPublicationContext)
	writeManifestUint64(hasher, uint64(build.Id))
	writeManifestUint64(hasher, uint64(build.GithubRunId))
	writeManifestUint64(hasher, uint64(build.GithubArtifactID))
	for _, identity := range []string{
		build.GithubProvider,
		build.GithubRepo,
		build.GithubWorkflow,
		build.WorkflowSelector,
		build.GithubRef,
		build.GithubArtifactName,
		build.GithubRunUrl,
		build.GithubHtmlUrl,
		build.GithubSourceSha,
		build.Platform,
		build.AppName,
		build.Version,
		build.BuildRef,
		build.SourceTag,
		build.AssetsRelease,
		build.AssetsReleaseAssets,
	} {
		writeManifestString(hasher, identity)
	}
	writeManifestUint64(hasher, uint64(build.AssetsReleaseID))
	for _, file := range files {
		writeManifestString(hasher, file.name)
		writeManifestUint64(hasher, uint64(file.size))
		opened, err := os.Open(filepath.Join(outputDir, filepath.FromSlash(file.name)))
		if err != nil {
			return 0, "", fmt.Errorf("open published artifact %q: %w", file.name, err)
		}
		openedInfo, statErr := opened.Stat()
		if statErr != nil {
			_ = opened.Close()
			return 0, "", fmt.Errorf("stat published artifact %q: %w", file.name, statErr)
		}
		if !openedInfo.Mode().IsRegular() || openedInfo.Size() != file.size {
			_ = opened.Close()
			return 0, "", fmt.Errorf("published artifact %q changed during manifest computation", file.name)
		}
		copied, copyErr := io.Copy(hasher, io.LimitReader(opened, file.size+1))
		closeErr := opened.Close()
		if copyErr != nil {
			return 0, "", fmt.Errorf("read published artifact %q: %w", file.name, copyErr)
		}
		if closeErr != nil {
			return 0, "", fmt.Errorf("close published artifact %q: %w", file.name, closeErr)
		}
		if copied != file.size {
			return 0, "", fmt.Errorf("published artifact %q changed during manifest computation", file.name)
		}
	}
	return executableSize, hex.EncodeToString(hasher.Sum(nil)), nil
}

func writeManifestString(hasher io.Writer, value string) {
	writeManifestUint64(hasher, uint64(len(value)))
	_, _ = io.WriteString(hasher, value)
}

func writeManifestUint64(hasher io.Writer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = hasher.Write(encoded[:])
}

func validPublishedDigest(digest string) bool {
	if len(digest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func publishedDigestMatches(stored, computed string) bool {
	if !validPublishedDigest(stored) || !validPublishedDigest(computed) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(stored)), []byte(strings.ToLower(computed))) == 1
}

// ValidatePublishedOutputDigest revalidates the canonical output and compares
// it with the write-once digest recorded for a completed build.
func ValidatePublishedOutputDigest(build *model.CustomBuild) (int64, error) {
	if build == nil {
		return 0, errors.New("build record is required")
	}
	if build.PublicationRecordedAt <= 0 || !validPublishedDigest(build.PublishedDigest) {
		return 0, errors.New("published output marker or digest is missing")
	}
	if _, err := CompletedBuildProvenanceFromRecord(build); err != nil {
		return 0, err
	}
	size, err := ValidatePublishedOutputProof(build)
	if err != nil {
		return 0, err
	}
	return size, nil
}

// ValidatePublishedOutputProof verifies the stored publication marker/digest
// against the current canonical output without requiring the row to be done.
// It is used by recovery and publication code that must not reuse an unproven
// output directory.
func ValidatePublishedOutputProof(build *model.CustomBuild) (int64, error) {
	if build == nil {
		return 0, errors.New("build record is required")
	}
	if build.PublicationRecordedAt <= 0 || !validPublishedDigest(build.PublishedDigest) {
		return 0, errors.New("published output marker or digest is missing")
	}
	size, digest, err := publishedOutputManifest(BuildOutputDir(build.Id), build)
	if err != nil {
		return 0, err
	}
	if !publishedDigestMatches(build.PublishedDigest, digest) {
		return 0, errors.New("published output digest does not match recorded digest")
	}
	return size, nil
}

// ValidateCompletedPublishedOutput is the shared public readiness predicate.
// Completion, immutable provider identity, stored publication proof, and the
// current output contents must all agree before a completed build is exposed.
func ValidateCompletedPublishedOutput(build *model.CustomBuild) (BuildProvenance, int64, error) {
	if build == nil {
		return BuildProvenance{}, 0, errors.New("build record is required")
	}
	if err := RequireProductionBuildCapability(build.Platform); err != nil {
		return BuildProvenance{}, 0, err
	}
	provenance, err := CompletedBuildProvenanceFromRecord(build)
	if err != nil {
		return BuildProvenance{}, 0, err
	}
	size, err := ValidatePublishedOutputProof(build)
	if err != nil {
		return BuildProvenance{}, 0, err
	}
	return provenance, size, nil
}

// PublishedOutputDigest computes the service-owned digest for a canonical
// output directory. Callers cannot provide an alternate output path.
func PublishedOutputDigest(build *model.CustomBuild) (string, error) {
	if build == nil {
		return "", errors.New("build record is required")
	}
	_, digest, err := publishedOutputManifest(BuildOutputDir(build.Id), build)
	return digest, err
}

// PublishedOutputFileEntries returns deterministic, secret-redacted output
// metadata for an already validated canonical output. custom_.txt is part of
// the service-owned publication digest when present, but its name, size, and
// hash are intentionally omitted from the operator handoff because its bytes
// carry private build settings.
func PublishedOutputFileEntries(build *model.CustomBuild) ([]BuildHandoffOutputFile, error) {
	if build == nil {
		return nil, errors.New("build record is required")
	}
	var files []BuildHandoffOutputFile
	err := WithPublishedOutputExportLock(func() error {
		outputDir := BuildOutputDir(build.Id)
		if _, _, err := publishedOutputManifest(outputDir, build); err != nil {
			return err
		}
		entries, err := os.ReadDir(outputDir)
		if err != nil {
			return fmt.Errorf("read published output entries: %w", err)
		}
		files = make([]BuildHandoffOutputFile, 0, len(entries))
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), "custom_.txt") {
				continue
			}
			path := filepath.Join(outputDir, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("inspect published output entry %q: %w", entry.Name(), err)
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("published output entry %q is not a regular file", entry.Name())
			}
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open published output entry %q: %w", entry.Name(), err)
			}
			hasher := sha256.New()
			copied, copyErr := io.Copy(hasher, io.LimitReader(file, int64(maxPublishedOutputFileBytes)+1))
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("hash published output entry %q: %w", entry.Name(), copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close published output entry %q: %w", entry.Name(), closeErr)
			}
			if copied != info.Size() {
				return fmt.Errorf("published output entry %q changed during handoff export", entry.Name())
			}
			files = append(files, BuildHandoffOutputFile{
				Name:   entry.Name(),
				Size:   copied,
				SHA256: hex.EncodeToString(hasher.Sum(nil)),
			})
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
		publishedOutputEntriesHook()
		_, digest, err := publishedOutputManifest(outputDir, build)
		if err != nil {
			return fmt.Errorf("recheck published output entries: %w", err)
		}
		if !publishedDigestMatches(build.PublishedDigest, digest) {
			return errors.New("published output changed during handoff export")
		}
		return nil
	})
	return files, err
}

// ValidateRecordedPublishedOutput verifies a write-once publication while a
// build is still active. It is the recovery-only path for failed completion
// persistence and requires exact run/artifact identity.
func (is *CustomBuildService) ValidateRecordedPublishedOutput(buildID uint, expectedRunID, expectedArtifactID int64) (int64, error) {
	if buildID == 0 || expectedRunID <= 0 || expectedArtifactID <= 0 {
		return 0, &BuildProgressPersistenceError{BuildID: buildID, Cause: errors.New("expected build, run, and artifact ids must be positive")}
	}
	var build model.CustomBuild
	if err := DB.Where("id = ? AND status IN ? AND github_run_id = ? AND github_artifact_id = ?", buildID, []string{
		model.CustomBuildStatusBuilding,
		model.CustomBuildStatusDownloading,
		model.CustomBuildStatusExtracting,
	}, expectedRunID, expectedArtifactID).First(&build).Error; err != nil {
		return 0, &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	provenance, err := BuildProvenanceFromRecord(&build)
	if err != nil {
		return 0, &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	if provenance.GithubRunID != expectedRunID || provenance.GithubArtifactID != expectedArtifactID {
		return 0, &BuildProgressPersistenceError{BuildID: buildID, Cause: errors.New("published output provenance does not match expected run and artifact")}
	}
	if err := is.requireCompletionCapability(buildID); err != nil {
		return 0, err
	}
	size, err := ValidatePublishedOutputProof(&build)
	if err != nil {
		return 0, &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	return size, nil
}
