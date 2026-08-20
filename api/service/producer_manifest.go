package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"
)

const (
	ProducerManifestFilename           = "manifest.txt"
	ProducerManifestSchema             = "deskforge.client-artifact"
	ProducerManifestVersion            = 2
	ProducerManifestDigestScope        = "sha256 covers public delivered output files; manifest.txt and declared private files are excluded"
	producerManifestLegacyDigestScope  = "sha256 covers delivered output files; manifest.txt is excluded"
	ProducerManifestVerificationScope  = "producer-reported source_sha, workflow_sha, workflow_ref, version, source_tree_sha, recursive submodule commits, and delivered output file names, sizes, and SHA-256 values"
	producerManifestLegacyScope        = "source_sha, workflow_sha, workflow_ref, version, source_tree_sha, recursive submodule commits, and delivered output file names, sizes, and SHA-256 values"
	ProducerManifestVerificationResult = "reported"
	ProducerManifestHandoffContract    = "deskforge.client-artifact-handoff-v1"
	MaxProducerManifestBytes           = 64 << 10
)

// ProducerManifest is the exact JSON schema emitted by the active RustDesk
// client workflows in manifest.txt. It is deliberately shared by all
// platforms; platform-specific output names are derived by
// ExpectedProducerOutputFilenames rather than parsed by individual callers.
type ProducerManifest struct {
	Schema               string                      `json:"schema"`
	ManifestSchema       string                      `json:"manifest_schema"`
	SchemaVersion        int                         `json:"schema_version"`
	Platform             string                      `json:"platform"`
	AppName              string                      `json:"app_name"`
	OutputFilenames      []string                    `json:"output_filenames"`
	SourceSHA            string                      `json:"source_sha"`
	WorkflowSHA          string                      `json:"workflow_sha"`
	WorkflowRef          string                      `json:"workflow_ref"`
	Version              string                      `json:"version"`
	SourceTreeSHA        string                      `json:"source_tree_sha"`
	Submodules           []ProducerManifestSubmodule `json:"submodules"`
	DigestScope          string                      `json:"digest_scope"`
	VerificationScope    string                      `json:"verification_scope"`
	VerificationResult   string                      `json:"verification_result"`
	PublicationTimestamp int64                       `json:"publication_timestamp"`
	HandoffContract      string                      `json:"handoff_contract"`
	Files                []ProducerManifestFile      `json:"files"`
	PrivateFilenames     []string                    `json:"private_filenames"`
}

type ProducerManifestFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type ProducerManifestSubmodule struct {
	Path      string `json:"path"`
	CommitSHA string `json:"commit_sha"`
}

// ParseProducerManifest parses one bounded producer manifest and rejects
// unknown fields, duplicate JSON keys, trailing values, and malformed JSON.
func ParseProducerManifest(data []byte) (ProducerManifest, error) {
	if len(data) == 0 || len(data) > MaxProducerManifestBytes {
		return ProducerManifest{}, fmt.Errorf("producer manifest must be between 1 and %d bytes", MaxProducerManifestBytes)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return ProducerManifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest ProducerManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ProducerManifest{}, fmt.Errorf("decode producer manifest: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ProducerManifest{}, errors.New("producer manifest contains trailing JSON")
		}
		return ProducerManifest{}, fmt.Errorf("decode producer manifest trailing data: %w", err)
	}
	if err := manifest.validateShape(); err != nil {
		return ProducerManifest{}, err
	}
	return manifest, nil
}

func (m ProducerManifest) validateShape() error {
	if m.Schema != ProducerManifestSchema {
		return fmt.Errorf("producer manifest schema must be %q", ProducerManifestSchema)
	}
	if m.SchemaVersion == 1 {
		return m.validateLegacyShape()
	}
	if m.SchemaVersion != ProducerManifestVersion || m.ManifestSchema != ProducerManifestSchema {
		return fmt.Errorf("producer manifest schema must be %q version %d", ProducerManifestSchema, ProducerManifestVersion)
	}
	if err := validateProducerPlatform(m.Platform); err != nil {
		return fmt.Errorf("producer manifest platform: %w", err)
	}
	if err := ValidateOutputAppName(m.AppName); err != nil {
		return fmt.Errorf("producer manifest app_name: %w", err)
	}
	if !utils.ValidateBuildVersion(m.Version) {
		return errors.New("producer manifest version is invalid")
	}
	if !validGithubSourceSHA(m.SourceSHA) || !validGithubSourceSHA(m.WorkflowSHA) {
		return errors.New("producer manifest source/workflow SHA must be 40-64 hexadecimal characters")
	}
	if m.WorkflowRef == "" || strings.TrimSpace(m.WorkflowRef) != m.WorkflowRef || strings.ContainsAny(m.WorkflowRef, "\r\n\x00") {
		return errors.New("producer manifest workflow_ref is invalid")
	}
	if m.DigestScope != ProducerManifestDigestScope && (m.DigestScope != producerManifestLegacyDigestScope || len(m.PrivateFilenames) != 0) {
		return fmt.Errorf("producer manifest digest_scope must be %q", ProducerManifestDigestScope)
	}
	if !validGithubSourceSHA(m.SourceTreeSHA) {
		return errors.New("producer manifest source_tree_sha must be 40-64 hexadecimal characters")
	}
	if m.Submodules == nil {
		return errors.New("producer manifest submodules must be present")
	}
	if err := validateProducerSubmodules(m.Submodules); err != nil {
		return err
	}
	if err := validateProducerPrivateFilenames(m.PrivateFilenames); err != nil {
		return err
	}
	if m.VerificationScope != ProducerManifestVerificationScope && m.VerificationScope != producerManifestLegacyScope {
		return fmt.Errorf("producer manifest verification_scope must be %q", ProducerManifestVerificationScope)
	}
	if m.VerificationResult != ProducerManifestVerificationResult && m.VerificationResult != "verified" {
		return fmt.Errorf("producer manifest verification_result must be %q", ProducerManifestVerificationResult)
	}
	if m.PublicationTimestamp <= 0 {
		return errors.New("producer manifest publication_timestamp must be positive")
	}
	if m.HandoffContract != ProducerManifestHandoffContract {
		return fmt.Errorf("producer manifest handoff_contract must be %q", ProducerManifestHandoffContract)
	}
	expected, err := ExpectedProducerOutputFilenames(m.Platform, m.AppName, m.Version)
	if err != nil {
		return err
	}
	if !sameStrings(m.OutputFilenames, expected) {
		return fmt.Errorf("producer manifest output_filenames = %v, want %v", m.OutputFilenames, expected)
	}
	if len(m.Files) != len(expected) {
		return fmt.Errorf("producer manifest files must contain exactly %d entries", len(expected))
	}
	seen := make(map[string]struct{}, len(m.Files))
	for index, file := range m.Files {
		if file.Name != expected[index] {
			return fmt.Errorf("producer manifest file %d name = %q, want %q", index, file.Name, expected[index])
		}
		if err := validateProducerFilenameForPlatform(file.Name, m.Platform); err != nil {
			return err
		}
		if file.Size < 0 {
			return fmt.Errorf("producer manifest file %q has a negative size", file.Name)
		}
		key := strings.ToLower(file.Name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("producer manifest contains duplicate or case-colliding file %q", file.Name)
		}
		seen[key] = struct{}{}
		if len(file.SHA256) != sha256.Size*2 || !isLowerHex(file.SHA256) {
			return fmt.Errorf("producer manifest file %q has an invalid SHA-256", file.Name)
		}
	}
	return nil
}

func (m ProducerManifest) validateLegacyShape() error {
	if m.ManifestSchema != "" || m.SourceTreeSHA != "" || m.Submodules != nil || m.VerificationScope != "" || m.VerificationResult != "" || m.PublicationTimestamp != 0 || m.HandoffContract != "" || m.PrivateFilenames != nil {
		return errors.New("legacy producer manifest contains v2-only fields")
	}
	if err := ValidateCustomPlatform(m.Platform); err != nil {
		return fmt.Errorf("producer manifest platform: %w", err)
	}
	if err := ValidateOutputAppName(m.AppName); err != nil {
		return fmt.Errorf("producer manifest app_name: %w", err)
	}
	if !utils.ValidateBuildVersion(m.Version) || !validGithubSourceSHA(m.SourceSHA) || !validGithubSourceSHA(m.WorkflowSHA) {
		return errors.New("legacy producer manifest identity is invalid")
	}
	if m.WorkflowRef == "" || strings.TrimSpace(m.WorkflowRef) != m.WorkflowRef || strings.ContainsAny(m.WorkflowRef, "\r\n\x00") {
		return errors.New("legacy producer manifest workflow_ref is invalid")
	}
	if m.DigestScope != producerManifestLegacyDigestScope {
		return fmt.Errorf("legacy producer manifest digest_scope must be %q", producerManifestLegacyDigestScope)
	}
	expected, err := ExpectedProducerOutputFilenames(m.Platform, m.AppName, m.Version)
	if err != nil {
		return err
	}
	if !sameStrings(m.OutputFilenames, expected) || len(m.Files) != len(expected) {
		return errors.New("legacy producer manifest output files are invalid")
	}
	for index, file := range m.Files {
		if file.Name != expected[index] || file.Size != 0 || len(file.SHA256) != sha256.Size*2 || !isLowerHex(file.SHA256) {
			return fmt.Errorf("legacy producer manifest file %q is invalid", file.Name)
		}
	}
	return nil
}

func validateProducerPrivateFilenames(filenames []string) error {
	if len(filenames) == 0 {
		return nil
	}
	if len(filenames) != 1 || filenames[0] != "custom_.txt" {
		return errors.New("producer manifest private_filenames may contain only custom_.txt")
	}
	return nil
}

func validateProducerPlatform(platform string) error {
	if platform == "bridge" {
		return nil
	}
	return ValidateCustomPlatform(platform)
}

func validateProducerSubmodules(submodules []ProducerManifestSubmodule) error {
	seen := make(map[string]struct{}, len(submodules))
	previous := ""
	for _, submodule := range submodules {
		if submodule.Path == "" || strings.TrimSpace(submodule.Path) != submodule.Path || strings.ContainsAny(submodule.Path, "\\\x00\r\n") || filepath.IsAbs(submodule.Path) || filepath.Clean(submodule.Path) != submodule.Path || submodule.Path == "." || submodule.Path == ".." || strings.HasPrefix(submodule.Path, "../") {
			return fmt.Errorf("producer manifest submodule path %q is invalid", submodule.Path)
		}
		if _, exists := seen[submodule.Path]; exists {
			return fmt.Errorf("producer manifest contains duplicate submodule path %q", submodule.Path)
		}
		if previous != "" && previous >= submodule.Path {
			return errors.New("producer manifest submodules must be sorted by path")
		}
		if !validGithubSourceSHA(submodule.CommitSHA) {
			return fmt.Errorf("producer manifest submodule %q commit SHA is invalid", submodule.Path)
		}
		seen[submodule.Path] = struct{}{}
		previous = submodule.Path
	}
	return nil
}

// StoredJSON returns the bounded, secret-free producer provenance persisted
// with a published provider artifact.
func (m ProducerManifest) StoredJSON() (string, error) {
	if m.SchemaVersion != ProducerManifestVersion {
		return "", errors.New("only the current producer manifest can be stored")
	}
	// v2 artifacts emitted before the producer/source evidence boundary used
	// "verified" for self-reported source-tree data. Keep parsing that v2
	// shape for compatibility, but never persist or export that label again.
	normalized := m
	normalized.DigestScope = ProducerManifestDigestScope
	normalized.VerificationScope = ProducerManifestVerificationScope
	normalized.VerificationResult = ProducerManifestVerificationResult
	if err := normalized.validateShape(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal producer manifest: %w", err)
	}
	return string(encoded), nil
}

// ProducerManifestFromStoredJSON parses the redacted producer provenance
// retained for deterministic handoff exports.
func ProducerManifestFromStoredJSON(value string) (ProducerManifest, error) {
	if value == "" {
		return ProducerManifest{}, errors.New("stored producer manifest is missing")
	}
	manifest, err := ParseProducerManifest([]byte(value))
	if err != nil {
		return ProducerManifest{}, fmt.Errorf("stored producer manifest: %w", err)
	}
	return manifest, nil
}

// ValidateProducerManifestForBuild compares producer-owned identity with the
// immutable build snapshot. The caller must use this before any output is
// published or exposed.
func ValidateProducerManifestForBuild(manifest ProducerManifest, build *model.CustomBuild) error {
	if build == nil {
		return errors.New("build record is required for producer manifest validation")
	}
	if err := manifest.validateShape(); err != nil {
		return err
	}
	if RequiresProducerManifest(build) && manifest.SchemaVersion != ProducerManifestVersion {
		return errors.New("provider artifact requires the current producer manifest contract")
	}
	if manifest.Platform != build.Platform || manifest.AppName != build.AppName || manifest.Version != build.Version {
		return errors.New("producer manifest platform, app_name, or version does not match stored build")
	}
	if !strings.EqualFold(manifest.SourceSHA, build.BuildRef) {
		return errors.New("producer manifest source_sha does not match stored source identity")
	}
	if !strings.EqualFold(manifest.WorkflowSHA, build.GithubRef) {
		return errors.New("producer manifest workflow_sha does not match stored workflow identity")
	}
	if !producerWorkflowRefMatches(manifest.WorkflowRef, build.WorkflowSelector) {
		return errors.New("producer manifest workflow_ref does not match stored workflow selector")
	}
	if build.ProducerManifestJSON != "" {
		stored, err := ProducerManifestFromStoredJSON(build.ProducerManifestJSON)
		if err != nil {
			return err
		}
		storedJSON, err := stored.StoredJSON()
		if err != nil {
			return err
		}
		candidateJSON, err := manifest.StoredJSON()
		if err != nil {
			return err
		}
		if storedJSON != candidateJSON {
			return errors.New("producer manifest does not match stored producer provenance")
		}
	}
	return nil
}

// GitHub exposes workflow_dispatch's run ref as refs/heads/<selector> (or
// refs/tags/<selector>), while DeskForge stores the exact selector sent in the
// dispatch request without that transport prefix. These are the only accepted
// equivalent representations; arbitrary ref inference is not allowed.
func producerWorkflowRefMatches(producer, stored string) bool {
	if producer == stored {
		return true
	}
	return strings.TrimPrefix(strings.TrimPrefix(producer, "refs/heads/"), "refs/tags/") == stored
}

// ExpectedProducerOutputFilenames is the one platform mapping shared by
// manifest validation and extraction. These names match the active workflow
// assertions exactly and preserve the existing artifact names.
func ExpectedProducerOutputFilenames(platform, appName, version string) ([]string, error) {
	if err := validateProducerPlatform(platform); err != nil {
		return nil, err
	}
	if err := ValidateOutputAppName(appName); err != nil {
		return nil, err
	}
	if !utils.ValidateBuildVersion(version) {
		return nil, errors.New("producer output version is invalid")
	}
	var names []string
	switch platform {
	case string(PlatformWindows):
		names = []string{appName + ".exe"}
	case string(PlatformLinux):
		// This is Python sorted() order used by rustqs-linux.yml: '-' sorts
		// before '.', so the RPM entry precedes the DEB entry.
		names = []string{appName + "-" + version + "-0.x86_64.rpm", appName + "-" + version + ".deb"}
	case string(PlatformAndroid):
		names = []string{appName + ".apk"}
	case "bridge":
		if appName != "rustdesk-bridge" {
			return nil, errors.New("bridge producer app_name must be rustdesk-bridge")
		}
		names = []string{
			"flutter/ios/Runner/bridge_generated.h",
			"flutter/lib/generated_bridge.dart",
			"flutter/lib/generated_bridge.freezed.dart",
			"flutter/macos/Runner/bridge_generated.h",
			"src/bridge_generated.io.rs",
			"src/bridge_generated.rs",
		}
	}
	return names, nil
}

// RequiresProducerManifest identifies new provider-run artifacts. Rows without
// a run and immutable identity are the explicit legacy compatibility boundary;
// any provider-run row must carry the active workflow manifest.
func RequiresProducerManifest(build *model.CustomBuild) bool {
	if build == nil {
		return false
	}
	return build.GithubRunId > 0 || build.BuildRef != "" || build.WorkflowSelector != "" || build.GithubRef != "" || build.GithubSourceSha != ""
}

// ValidateProducerManifestOutput verifies that staging contains exactly the
// manifest-declared regular files and that every declared SHA-256 matches the
// extracted bytes. manifest.txt is never part of the published output.
func ValidateProducerManifestOutput(manifest ProducerManifest, outputDir string) (int64, error) {
	if err := manifest.validateShape(); err != nil {
		return 0, err
	}
	if manifest.Platform == "bridge" {
		return validateNestedProducerManifestOutput(manifest, outputDir)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return 0, fmt.Errorf("read producer output: %w", err)
	}
	if len(entries) != len(manifest.Files)+len(manifest.PrivateFilenames) {
		return 0, fmt.Errorf("producer output contains %d entries, want %d", len(entries), len(manifest.Files)+len(manifest.PrivateFilenames))
	}
	expected := make(map[string]ProducerManifestFile, len(manifest.Files))
	for _, file := range manifest.Files {
		expected[file.Name] = file
	}
	private := make(map[string]struct{}, len(manifest.PrivateFilenames))
	for _, name := range manifest.PrivateFilenames {
		private[name] = struct{}{}
	}
	var total uint64
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(outputDir, entry.Name()))
		if err != nil {
			return 0, fmt.Errorf("inspect producer output %q: %w", entry.Name(), err)
		}
		if _, ok := expected[entry.Name()]; !ok {
			if _, isPrivate := private[entry.Name()]; isPrivate {
				if err := validatePrivateProducerOutputFile(entry.Name(), info); err != nil {
					return 0, err
				}
				if total > maxPublishedOutputTotalBytes-uint64(info.Size()) {
					return 0, errors.New("producer output exceeds aggregate size limit")
				}
				total += uint64(info.Size())
				continue
			}
			return 0, fmt.Errorf("producer output contains unexpected file %q", entry.Name())
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("producer output %q is not a regular file", entry.Name())
		}
		file, err := os.Open(filepath.Join(outputDir, entry.Name()))
		if err != nil {
			return 0, fmt.Errorf("open producer output %q: %w", entry.Name(), err)
		}
		hasher := sha256.New()
		copied, copyErr := io.Copy(hasher, io.LimitReader(file, int64(maxPublishedOutputFileBytes)+1))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return 0, fmt.Errorf("hash producer output %q: %w", entry.Name(), firstError(copyErr, closeErr))
		}
		if copied != info.Size() || uint64(copied) > maxPublishedOutputFileBytes {
			return 0, fmt.Errorf("producer output %q changed or exceeds size limit", entry.Name())
		}
		if expected[entry.Name()].Size != info.Size() {
			return 0, fmt.Errorf("producer output %q size does not match manifest", entry.Name())
		}
		if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expected[entry.Name()].SHA256) {
			return 0, fmt.Errorf("producer output %q does not match manifest SHA-256", entry.Name())
		}
		if total > maxPublishedOutputTotalBytes-uint64(copied) {
			return 0, errors.New("producer output exceeds aggregate size limit")
		}
		total += uint64(copied)
	}
	return int64(total), nil
}

func validateNestedProducerManifestOutput(manifest ProducerManifest, outputDir string) (int64, error) {
	rootInfo, err := os.Lstat(outputDir)
	if err != nil {
		return 0, fmt.Errorf("inspect producer output: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return 0, errors.New("producer output is not a regular directory")
	}
	expected := make(map[string]ProducerManifestFile, len(manifest.Files))
	for _, file := range manifest.Files {
		expected[file.Name] = file
	}
	seen := make(map[string]struct{}, len(expected))
	private := make(map[string]struct{}, len(manifest.PrivateFilenames))
	for _, name := range manifest.PrivateFilenames {
		private[name] = struct{}{}
	}
	var total uint64
	err = filepath.WalkDir(outputDir, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == outputDir {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("producer output %q contains a symlink", entry.Name())
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("producer output %q is not a regular file", entry.Name())
		}
		relative, relErr := filepath.Rel(outputDir, current)
		if relErr != nil {
			return relErr
		}
		name := filepath.ToSlash(relative)
		declared, ok := expected[name]
		if !ok {
			if _, isPrivate := private[name]; isPrivate {
				if err := validatePrivateProducerOutputFile(name, info); err != nil {
					return err
				}
				if total > maxPublishedOutputTotalBytes-uint64(info.Size()) {
					return errors.New("producer output exceeds aggregate size limit")
				}
				total += uint64(info.Size())
				seen[name] = struct{}{}
				return nil
			}
			return fmt.Errorf("producer output contains unexpected file %q", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("producer output contains duplicate file %q", name)
		}
		seen[name] = struct{}{}
		file, openErr := os.Open(current)
		if openErr != nil {
			return openErr
		}
		hasher := sha256.New()
		copied, copyErr := io.Copy(hasher, io.LimitReader(file, int64(maxPublishedOutputFileBytes)+1))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return firstError(copyErr, closeErr)
		}
		if copied != info.Size() || uint64(copied) > maxPublishedOutputFileBytes {
			return fmt.Errorf("producer output %q changed or exceeds size limit", name)
		}
		if declared.Size != info.Size() {
			return fmt.Errorf("producer output %q size does not match manifest", name)
		}
		if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), declared.SHA256) {
			return fmt.Errorf("producer output %q does not match manifest SHA-256", name)
		}
		if total > maxPublishedOutputTotalBytes-uint64(copied) {
			return errors.New("producer output exceeds aggregate size limit")
		}
		total += uint64(copied)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk producer output: %w", err)
	}
	if len(seen) != len(expected)+len(private) {
		return 0, fmt.Errorf("producer output contains %d files, want %d", len(seen), len(expected)+len(private))
	}
	return int64(total), nil
}

func validatePrivateProducerOutputFile(name string, info os.FileInfo) error {
	if name != "custom_.txt" {
		return fmt.Errorf("producer output contains undeclared private file %q", name)
	}
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("producer output %q is not a regular file", name)
	}
	if info.Size() < 0 || uint64(info.Size()) > maxPublishedOutputFileBytes {
		return fmt.Errorf("producer output %q exceeds size limit", name)
	}
	return nil
}

func validateProducerFilename(name string) error {
	if !utf8.ValidString(name) || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n") || strings.TrimSpace(name) != name {
		return fmt.Errorf("producer manifest contains unsafe filename %q", name)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("producer manifest filename %q is not flat", name)
	}
	return nil
}

func validateProducerFilenameForPlatform(name, platform string) error {
	if platform != "bridge" {
		return validateProducerFilename(name)
	}
	if !utf8.ValidString(name) || name == "" || strings.ContainsAny(name, "\\\x00\r\n") || strings.TrimSpace(name) != name || path.IsAbs(name) || strings.HasPrefix(name, "//") || windowsDrivePath(name) || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("producer manifest contains unsafe filename %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("producer manifest contains unsafe filename %q", name)
		}
	}
	return nil
}

func windowsDrivePath(name string) bool {
	return len(name) >= 2 && ((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z')) && name[1] == ':'
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func firstError(left, right error) error {
	if left != nil {
		return left
	}
	return right
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch delimiter := token.(type) {
		case json.Delim:
			switch delimiter {
			case '{':
				seen := map[string]struct{}{}
				for decoder.More() {
					key, err := decoder.Token()
					if err != nil {
						return err
					}
					keyString, ok := key.(string)
					if !ok {
						return errors.New("producer manifest object key is not a string")
					}
					if _, exists := seen[keyString]; exists {
						return fmt.Errorf("producer manifest contains duplicate JSON key %q", keyString)
					}
					seen[keyString] = struct{}{}
					if err := walk(); err != nil {
						return err
					}
				}
				_, err = decoder.Token()
				return err
			case '[':
				for decoder.More() {
					if err := walk(); err != nil {
						return err
					}
				}
				_, err = decoder.Token()
				return err
			}
		}
		return nil
	}
	if err := walk(); err != nil {
		return fmt.Errorf("validate producer manifest JSON: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("producer manifest contains trailing JSON")
		}
		return fmt.Errorf("validate producer manifest trailing JSON: %w", err)
	}
	return nil
}
