package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"
)

type CustomBuildService struct{}

// IsSecretEncryptionConfigurationError reports whether an error means a
// secret-bearing persistence operation is unavailable because the canonical
// at-rest key is missing. The helper keeps HTTP boundaries independent of GORM
// hook details while preserving the typed root error for callers.
func IsSecretEncryptionConfigurationError(err error) bool {
	var keyErr *utils.SecretEncryptionKeyError
	return errors.As(err, &keyErr)
}

var removeBuildOutputDir = os.RemoveAll

// buildOutputLifecycleMu serializes the DB deletion boundary and exact output
// and marker removal with stale-tombstone cleanup. Normal SQLite/GORM model
// operation does not reuse deleted IDs, but this closes the in-process
// delete/sweep overlap that could otherwise remove a live path.
var buildOutputLifecycleMu sync.Mutex

// CustomBuildCleanupPending reports a successful authoritative DB deletion
// whose service-owned output directory still needs cleanup. Callers must not
// turn this into a failed deletion: the row is gone, while the filesystem
// condition remains observable for operator/sweeper handling.
type CustomBuildCleanupPending struct {
	Directory string
	Cause     error
}

func (e *CustomBuildCleanupPending) Error() string {
	return fmt.Sprintf("custom build deleted; artifact cleanup pending for %s: %v", e.Directory, e.Cause)
}

func (e *CustomBuildCleanupPending) Unwrap() error { return e.Cause }

// BuildOutputDir returns the on-disk path where the build agent writes
// artifacts for a given build id. The convention `/rdgen-data/output/<id>`
// is shared with docker/entrypoint-{linux,win}.sh and is referenced by the
// download handler in api/http/controller/admin/custom_build.go — keep this
// helper as the single source of truth so callers can't drift.
var BuildOutputDir = func(id uint) string {
	return filepath.Join("/rdgen-data", "output", fmt.Sprintf("%d", id))
}

func (is *CustomBuildService) List(page, pageSize uint) (*model.CustomBuildList, error) {
	res := &model.CustomBuildList{}
	tx := DB.Model(&model.CustomBuild{})
	if err := tx.Count(&res.Total).Error; err != nil {
		return nil, err
	}
	if err := tx.Scopes(Paginate(page, pageSize)).Order("id desc").Find(&res.CustomBuilds).Error; err != nil {
		return nil, err
	}
	return res, nil
}

func (is *CustomBuildService) Info(id uint) (*model.CustomBuild, error) {
	u := &model.CustomBuild{}
	if err := DB.Where("id = ?", id).First(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

// Delete records cleanup intent before deleting the DB row. A nil DB.Delete
// result is the authoritative deletion boundary: database errors remain ordinary
// failures, while only filesystem/tombstone errors after that boundary are
// reported as cleanup pending. The row remains authoritative before that point;
// cleanup after a successful delete cannot resurrect ownership.
func (is *CustomBuildService) Delete(u *model.CustomBuild) error {
	id := u.Id
	dir := BuildOutputDir(id)
	outputRoot := filepath.Dir(dir)
	buildOutputLifecycleMu.Lock()
	defer buildOutputLifecycleMu.Unlock()
	tombstone, err := ensureBuildOutputDeletionTombstone(outputRoot, id)
	if err != nil {
		return err
	}
	if err := DB.Delete(u).Error; err != nil {
		return err
	}
	if err := removeBuildOutputDir(dir); err != nil {
		return &CustomBuildCleanupPending{Directory: dir, Cause: err}
	}
	if _, err := os.Lstat(dir); err == nil {
		return &CustomBuildCleanupPending{Directory: dir, Cause: errors.New("directory remains after cleanup")}
	} else if !os.IsNotExist(err) {
		return &CustomBuildCleanupPending{Directory: dir, Cause: fmt.Errorf("verify cleanup: %w", err)}
	}
	if err := removeBuildOutputDeletionTombstoneIfPresent(tombstone); err != nil {
		return &CustomBuildCleanupPending{Directory: dir, Cause: fmt.Errorf("remove cleanup tombstone: %w", err)}
	}
	return nil
}

func (is *CustomBuildService) Create(u *model.CustomBuild) error {
	_, err := is.CreateNormalized(u)
	return err
}

// CreateNormalized persists the canonical form and returns the same normalized
// dispatch values used by the caller to submit the build. L1 values therefore
// remain available for dispatch without being stored in custom_json.
func (is *CustomBuildService) CreateNormalized(u *model.CustomBuild) (NormalizedBuild, error) {
	if err := ValidateDirectCustomBuilderJSON(u.CustomJson); err != nil {
		return NormalizedBuild{}, err
	}
	if u.BuildRef != "" || u.SourceTag != "" || u.AssetsRelease != "" || u.AssetsReleaseID != 0 || u.AssetsReleaseAssets != "" {
		if _, err := VersionIdentityFromRecord(u); err != nil {
			return NormalizedBuild{}, &ClientValidationError{Err: err}
		}
	}
	normalized, err := NormalizeCustomBuildJSON(u.CustomJson, BuildRecordContext{
		BuildID:  u.Id,
		Platform: u.Platform,
		AppName:  u.AppName,
		Version:  u.Version,
	})
	if err != nil {
		return NormalizedBuild{}, &ClientValidationError{Err: err}
	}
	if err := utils.RequireSecretEncryptionForCustomBuilderJSON(normalized.PersistedJSON); err != nil {
		return NormalizedBuild{}, err
	}
	u.CustomJson = normalized.PersistedJSON
	if err := DB.Create(u).Error; err != nil {
		return NormalizedBuild{}, err
	}
	return normalized, nil
}

// CreateNormalizedWithIdentity persists a build after the configured catalog
// has resolved its display version to one immutable source/assets identity.
// The identity is copied into the row before Create; the dispatch transition
// later verifies the same values and cannot replace them.
func (is *CustomBuildService) CreateNormalizedWithIdentity(u *model.CustomBuild, identity VersionIdentity) (NormalizedBuild, error) {
	if err := identity.validate(); err != nil {
		return NormalizedBuild{}, &ClientValidationError{Err: err}
	}
	if u.Version != "" && u.Version != identity.DisplayVersion {
		return NormalizedBuild{}, &ClientValidationError{Err: fmt.Errorf("build version does not match resolved catalog identity")}
	}
	u.Version = identity.DisplayVersion
	u.GithubRepo = identity.Repo
	// Persist the exact selector separately from the resolved execution SHA. A
	// mutable branch must never replace the immutable execution identity.
	u.WorkflowSelector = identity.WorkflowRef
	u.GithubRef = identity.WorkflowSHA
	u.BuildRef = identity.BuildRef
	u.SourceTag = identity.SourceTag
	u.AssetsRelease = identity.AssetsRelease.TagName
	u.AssetsReleaseID = identity.AssetsRelease.ID
	assets, err := marshalReleaseAssets(identity.AssetsRelease.Assets)
	if err != nil {
		return NormalizedBuild{}, &ClientValidationError{Err: err}
	}
	u.AssetsReleaseAssets = assets
	return is.CreateNormalized(u)
}

// UpdateValidated is the user-facing update boundary for a future custom-build
// edit path. It writes only user-editable canonical fields; provenance and
// asynchronous state are deliberately not part of this update boundary.
func (is *CustomBuildService) UpdateValidated(u *model.CustomBuild) error {
	if u.Id == 0 {
		return &ClientValidationError{Err: fmt.Errorf("build id is required")}
	}
	var stored model.CustomBuild
	if err := DB.First(&stored, u.Id).Error; err != nil {
		return err
	}
	if stored.BuildRef != "" || stored.SourceTag != "" || stored.AssetsRelease != "" || stored.AssetsReleaseID != 0 || stored.AssetsReleaseAssets != "" {
		if u.Version != stored.Version {
			return &ClientValidationError{Err: fmt.Errorf("immutable build version cannot be changed")}
		}
	}
	if err := ValidateDirectCustomBuilderJSON(u.CustomJson); err != nil {
		return err
	}
	canonicalJSON, err := CanonicalizeCustomBuildJSON(u.CustomJson, BuildRecordContext{
		BuildID:  u.Id,
		Platform: u.Platform,
		AppName:  u.AppName,
		Version:  u.Version,
	})
	if err != nil {
		return err
	}
	storedJSON, err := utils.EncryptCustomBuilderJSON(canonicalJSON)
	if err != nil {
		return err
	}
	tx := DB.Model(&model.CustomBuild{}).Where("id = ?", u.Id).Updates(map[string]any{
		"name":        u.Name,
		"platform":    u.Platform,
		"version":     u.Version,
		"app_name":    u.AppName,
		"custom_json": storedJSON,
	})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf("validated update affected no rows")
	}
	return nil
}

// BuildProgress contains the fields owned by the asynchronous build worker.
// It intentionally has no custom_json or other user-authored fields.
type BuildProgress struct {
	BuildID            uint
	ExpectedRunID      int64
	ExpectedArtifactID int64
	Status             string
	BuildLog           string
	FileSize           int64
}

const MaxBuildLogBytes = 4096

// BoundBuildLog keeps asynchronous/provider messages bounded before they cross
// the persistence boundary. The newest message is retained because it carries
// the actionable lifecycle failure when older history is discarded.
func BoundBuildLog(existing, message string) string {
	if message != "" {
		if existing != "" {
			existing += "\n"
		}
		existing += message
	}
	if len(existing) <= MaxBuildLogBytes {
		return existing
	}
	return existing[len(existing)-MaxBuildLogBytes:]
}

// BuildProgressPersistenceError means that asynchronous build state could not
// be durably recorded. Callers must not continue with provider-derived state
// (such as starting a poll) after this error.
type BuildProgressPersistenceError struct {
	BuildID uint
	Cause   error
}

func (e *BuildProgressPersistenceError) Error() string {
	return fmt.Sprintf("persist build %d progress: %v", e.BuildID, e.Cause)
}

func (e *BuildProgressPersistenceError) Unwrap() error { return e.Cause }

func (is *CustomBuildService) requireCompletionCapability(buildID uint) error {
	var build model.CustomBuild
	if err := DB.Select("platform").First(&build, buildID).Error; err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	if err := RequireProductionBuildCapability(build.Platform); err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	return nil
}

func (is *CustomBuildService) requireCompletedPublication(buildID uint, expectedRunID, expectedArtifactID int64) error {
	var build model.CustomBuild
	if err := DB.First(&build, buildID).Error; err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	if err := is.requireCompletionCapability(buildID); err != nil {
		return err
	}
	provenance, err := BuildProvenanceFromRecord(&build)
	if err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	if provenance.GithubRunID != expectedRunID || provenance.GithubArtifactID != expectedArtifactID {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("completion provenance does not match expected run and artifact")}
	}
	if build.PublicationRecordedAt <= 0 || !validPublishedDigest(build.PublishedDigest) {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("completion requires publication marker and digest")}
	}
	if _, err := ValidatePublishedOutputProof(&build); err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("completion requires current output matching publication proof: %w", err)}
	}
	return nil
}

// BuildProvenance is the immutable provider-derived identity of a dispatched
// build. It deliberately contains no token, payload key, or dispatch payload.
type BuildProvenance struct {
	Version             string         `json:"version"`
	BuildRef            string         `json:"build_ref"`
	SourceTag           string         `json:"source_tag"`
	AssetsRelease       string         `json:"assets_release"`
	AssetsReleaseID     int64          `json:"assets_release_id"`
	GithubProvider      string         `json:"github_provider"`
	GithubRepo          string         `json:"github_repo"`
	GithubWorkflow      string         `json:"github_workflow"`
	WorkflowRef         string         `json:"workflow_ref"`
	WorkflowSHA         string         `json:"workflow_sha"`
	GithubRef           string         `json:"github_ref"`
	GithubArtifactName  string         `json:"github_artifact_name"`
	GithubArtifactID    int64          `json:"github_artifact_id"`
	GithubRunID         int64          `json:"github_run_id"`
	GithubRunURL        string         `json:"github_run_url"`
	GithubHTMLURL       string         `json:"github_html_url"`
	GithubSourceSHA     string         `json:"github_source_sha,omitempty"`
	AssetsReleaseAssets []ReleaseAsset `json:"assets_release_assets"`
}

const (
	maxGithubProviderLength     = 32
	maxGithubRepoLength         = 128
	maxGithubWorkflowLength     = 128
	maxGithubRefLength          = 128
	maxGithubArtifactNameLength = 128
	maxGithubURLLength          = 512
	maxGithubSourceSHALength    = 64
)

// BuildProvenancePersistenceError means that an immutable provenance write
// failed or was rejected because the build was already dispatched.
type BuildProvenancePersistenceError struct {
	BuildID uint
	Cause   error
}

func (e *BuildProvenancePersistenceError) Error() string {
	return fmt.Sprintf("persist build %d provenance: %v", e.BuildID, e.Cause)
}

func (e *BuildProvenancePersistenceError) Unwrap() error { return e.Cause }

func (p BuildProvenance) validate() error {
	workflowRef := p.WorkflowRef
	workflowSHA := p.WorkflowSHA
	if workflowSHA == "" {
		workflowSHA = p.GithubRef
	}
	if p.WorkflowSHA != "" && p.GithubRef != "" && !strings.EqualFold(p.WorkflowSHA, p.GithubRef) {
		return fmt.Errorf("workflow_sha and github_ref must match")
	}
	if workflowRef != "" && len(workflowRef) > maxGithubRefLength {
		return fmt.Errorf("workflow_ref exceeds %d bytes", maxGithubRefLength)
	}
	identity := VersionIdentity{
		Repo:           p.GithubRepo,
		DisplayVersion: p.Version,
		BuildRef:       p.BuildRef,
		SourceTag:      p.SourceTag,
		WorkflowRef:    workflowRef,
		WorkflowSHA:    workflowSHA,
		AssetsRelease:  AssetsRelease{ID: p.AssetsReleaseID, TagName: p.AssetsRelease, Assets: p.AssetsReleaseAssets},
	}
	if err := identity.validate(); err != nil {
		return err
	}
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{"github_provider", p.GithubProvider, maxGithubProviderLength},
		{"github_repo", p.GithubRepo, maxGithubRepoLength},
		{"github_workflow", p.GithubWorkflow, maxGithubWorkflowLength},
		{"workflow_selector", workflowRef, maxGithubRefLength},
		{"workflow_sha", workflowSHA, maxGithubSourceSHALength},
		{"github_artifact_name", p.GithubArtifactName, maxGithubArtifactNameLength},
		{"github_run_url", p.GithubRunURL, maxGithubURLLength},
		{"github_html_url", p.GithubHTMLURL, maxGithubURLLength},
	}
	for _, field := range fields {
		if field.value == "" {
			return fmt.Errorf("%s is required", field.name)
		}
		if len(field.value) > field.limit {
			return fmt.Errorf("%s exceeds %d bytes", field.name, field.limit)
		}
	}
	if p.GithubRunID <= 0 {
		return fmt.Errorf("github_run_id must be positive")
	}
	if len(p.GithubSourceSHA) > maxGithubSourceSHALength {
		return fmt.Errorf("github_source_sha exceeds %d bytes", maxGithubSourceSHALength)
	}
	if p.GithubSourceSHA != "" && !validGithubSourceSHA(p.GithubSourceSHA) {
		return fmt.Errorf("github_source_sha must be 40-64 hexadecimal characters")
	}
	if p.GithubSourceSHA != "" && !strings.EqualFold(p.GithubSourceSHA, workflowSHA) {
		return fmt.Errorf("github_source_sha must equal immutable workflow execution SHA")
	}
	if p.GithubArtifactID < 0 {
		return fmt.Errorf("github_artifact_id must not be negative")
	}
	return nil
}

func validGithubSourceSHA(sourceSHA string) bool {
	if len(sourceSHA) < 40 || len(sourceSHA) > 64 {
		return false
	}
	for _, char := range sourceSHA {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

// BuildProvenanceFromRecord returns the immutable snapshot stored on a build
// record. A missing field is an error so legacy rows cannot silently fall back
// to the mutable global GitHub configuration.
func BuildProvenanceFromRecord(b *model.CustomBuild) (BuildProvenance, error) {
	identity, err := VersionIdentityFromRecord(b)
	if err != nil {
		return BuildProvenance{}, err
	}
	p := BuildProvenance{
		Version:             identity.DisplayVersion,
		BuildRef:            identity.BuildRef,
		SourceTag:           identity.SourceTag,
		AssetsRelease:       identity.AssetsRelease.TagName,
		AssetsReleaseID:     identity.AssetsRelease.ID,
		GithubProvider:      b.GithubProvider,
		GithubRepo:          b.GithubRepo,
		GithubWorkflow:      b.GithubWorkflow,
		WorkflowRef:         b.WorkflowSelector,
		WorkflowSHA:         identity.WorkflowSHA,
		GithubRef:           b.GithubRef,
		GithubArtifactName:  b.GithubArtifactName,
		GithubArtifactID:    b.GithubArtifactID,
		GithubRunID:         b.GithubRunId,
		GithubRunURL:        b.GithubRunUrl,
		GithubHTMLURL:       b.GithubHtmlUrl,
		GithubSourceSHA:     b.GithubSourceSha,
		AssetsReleaseAssets: identity.AssetsRelease.Assets,
	}
	if err := p.validate(); err != nil {
		return BuildProvenance{}, err
	}
	return p, nil
}

// CompletedBuildProvenanceFromRecord returns readiness for a public completed
// build. The done status, service-generated publication marker, and immutable
// provider/artifact identity are all required; legacy/direct done rows fail
// closed rather than becoming public through a partial provenance snapshot.
func CompletedBuildProvenanceFromRecord(b *model.CustomBuild) (BuildProvenance, error) {
	if b == nil {
		return BuildProvenance{}, fmt.Errorf("build record is required")
	}
	if b.Status != model.CustomBuildStatusDone {
		return BuildProvenance{}, fmt.Errorf("build status %q is not done", b.Status)
	}
	if b.PublicationRecordedAt <= 0 || !validPublishedDigest(b.PublishedDigest) {
		return BuildProvenance{}, fmt.Errorf("publication marker and digest are required for completed build")
	}
	p, err := BuildProvenanceFromRecord(b)
	if err != nil {
		return BuildProvenance{}, err
	}
	if p.GithubArtifactID <= 0 {
		return BuildProvenance{}, fmt.Errorf("github_artifact_id must be positive for completed build")
	}
	return p, nil
}

// SetArtifactID records the exact artifact selected from the expected stored
// run. The status, run-id, and zero-value guards make artifact identity
// write-once and prevent a later poll, retry, or callback from replacing it.
func (is *CustomBuildService) SetArtifactID(buildID uint, expectedRunID, artifactID int64) error {
	if buildID == 0 {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: fmt.Errorf("build id is required")}
	}
	if expectedRunID <= 0 {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: fmt.Errorf("github_run_id must be positive")}
	}
	if artifactID <= 0 {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: fmt.Errorf("github_artifact_id must be positive")}
	}
	tx := DB.Model(&model.CustomBuild{}).
		Where("id = ? AND github_run_id > 0 AND github_run_id = ? AND status IN ? AND (github_artifact_id = 0 OR github_artifact_id IS NULL)", buildID, expectedRunID, []string{
			model.CustomBuildStatusBuilding,
			model.CustomBuildStatusDownloading,
			model.CustomBuildStatusExtracting,
		}).
		Update("github_artifact_id", artifactID)
	if tx.Error != nil {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: tx.Error}
	}
	if tx.RowsAffected == 0 {
		return &BuildProvenancePersistenceError{
			BuildID: buildID,
			Cause:   fmt.Errorf("artifact id write affected no undispatched artifact row"),
		}
	}
	return nil
}

// GithubConfigFromProvenance creates the request configuration for an existing
// build. The token is the only mutable credential; all request identity values
// come from the stored build snapshot.
func GithubConfigFromProvenance(provenance BuildProvenance, token string) *model.GithubBuildConfig {
	return &model.GithubBuildConfig{
		Repo:   provenance.GithubRepo,
		Branch: provenance.WorkflowRef,
		Token:  token,
	}
}

// SetProvenance atomically records the dispatch identity and transitions a
// pending build to building. The run-id guard makes the write one-shot and
// prevents a second dispatch from overwriting the first identity.
func (is *CustomBuildService) SetProvenance(buildID uint, provenance BuildProvenance) error {
	if buildID == 0 {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: fmt.Errorf("build id is required")}
	}
	if err := provenance.validate(); err != nil {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: err}
	}
	assetsJSON, err := marshalReleaseAssets(provenance.AssetsReleaseAssets)
	if err != nil {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: err}
	}
	tx := DB.Model(&model.CustomBuild{}).
		Where(`id = ? AND status = ? AND (github_run_id = 0 OR github_run_id IS NULL) AND
			(((version = '' AND build_ref = '' AND source_tag = '' AND assets_release = '' AND assets_release_id = 0) AND
			  (github_repo = '' OR github_repo IS NULL)) OR
				 (version = ? AND build_ref = ? AND source_tag = ? AND assets_release = ? AND assets_release_id = ? AND
				  github_repo = ? AND assets_release_assets = ?)) AND
			(github_provider = '' OR github_provider IS NULL) AND
			(github_workflow = '' OR github_workflow IS NULL) AND
			(workflow_selector = '' OR workflow_selector IS NULL OR workflow_selector = ?) AND
			(github_ref = '' OR github_ref IS NULL OR LOWER(github_ref) = LOWER(?)) AND
			(github_artifact_name = '' OR github_artifact_name IS NULL) AND
			(github_run_url = '' OR github_run_url IS NULL) AND
			(github_html_url = '' OR github_html_url IS NULL) AND
			(github_artifact_id = 0 OR github_artifact_id IS NULL) AND
			(github_source_sha = '' OR github_source_sha IS NULL)`, buildID, model.CustomBuildStatusPending,
			provenance.Version, provenance.BuildRef, provenance.SourceTag, provenance.AssetsRelease, provenance.AssetsReleaseID,
			provenance.GithubRepo, assetsJSON, provenance.WorkflowRef, provenance.WorkflowSHA).
		Updates(map[string]any{
			"version":               provenance.Version,
			"build_ref":             provenance.BuildRef,
			"source_tag":            provenance.SourceTag,
			"assets_release":        provenance.AssetsRelease,
			"assets_release_id":     provenance.AssetsReleaseID,
			"assets_release_assets": assetsJSON,
			"status":                model.CustomBuildStatusBuilding,
			"build_log":             BoundBuildLog("", fmt.Sprintf("github run id: %d", provenance.GithubRunID)),
			"github_run_id":         provenance.GithubRunID,
			"github_provider":       provenance.GithubProvider,
			"github_repo":           provenance.GithubRepo,
			"github_workflow":       provenance.GithubWorkflow,
			"workflow_selector":     provenance.WorkflowRef,
			"github_ref":            provenance.WorkflowSHA,
			"github_artifact_name":  provenance.GithubArtifactName,
			"github_run_url":        provenance.GithubRunURL,
			"github_html_url":       provenance.GithubHTMLURL,
		})
	if tx.Error != nil {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: tx.Error}
	}
	if tx.RowsAffected == 0 {
		return &BuildProvenancePersistenceError{
			BuildID: buildID,
			Cause:   fmt.Errorf("provenance write affected no pending undispatched row"),
		}
	}
	return nil
}

// SetSourceSha records the first exact run head SHA returned by the expected
// stored run. It is guarded against the persisted workflow execution SHA and
// can never overwrite an earlier observation.
func (is *CustomBuildService) SetSourceSha(buildID uint, expectedRunID int64, sourceSHA string) error {
	if buildID == 0 {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: fmt.Errorf("build id is required")}
	}
	if expectedRunID <= 0 {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: fmt.Errorf("github_run_id must be positive")}
	}
	if !validGithubSourceSHA(sourceSHA) {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: fmt.Errorf("github_source_sha must be 40-64 hexadecimal characters")}
	}
	tx := DB.Model(&model.CustomBuild{}).
		Where("id = ? AND github_run_id > 0 AND github_run_id = ? AND status IN ? AND LOWER(github_ref) = LOWER(?) AND (github_source_sha = '' OR github_source_sha IS NULL)", buildID, expectedRunID, []string{
			model.CustomBuildStatusBuilding,
			model.CustomBuildStatusDownloading,
			model.CustomBuildStatusExtracting,
		}, sourceSHA).
		Update("github_source_sha", sourceSHA)
	if tx.Error != nil {
		return &BuildProvenancePersistenceError{BuildID: buildID, Cause: tx.Error}
	}
	if tx.RowsAffected == 0 {
		return &BuildProvenancePersistenceError{
			BuildID: buildID,
			Cause:   fmt.Errorf("source sha write affected no row with empty source sha"),
		}
	}
	return nil
}

// ValidatePublishedOutput verifies that a final output directory is present,
// contains only regular files, and includes the platform's required published
// content. It returns the required executable size for Windows and zero for
// other supported output layouts.
func ValidatePublishedOutput(outputDir string, build *model.CustomBuild) (int64, error) {
	size, _, err := publishedOutputManifest(outputDir, build)
	return size, err
}

// RecordPublishedOutput validates and records the canonical final output for
// the exact active build run and artifact. The marker and digest are write-once
// and generated here, rather than accepted from an arbitrary progress caller.
// Filesystem and DB updates cannot be one transaction: a DB failure after this
// marker write (or a filesystem change after validation) remains recoverable
// through polling.
func (is *CustomBuildService) RecordPublishedOutput(buildID uint, expectedRunID, expectedArtifactID int64, producerManifest ...ProducerManifest) error {
	if buildID == 0 {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("build id is required")}
	}
	if expectedRunID <= 0 || expectedArtifactID <= 0 {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("expected run and artifact ids must be positive")}
	}
	var build model.CustomBuild
	if err := DB.Where("id = ? AND status IN ? AND github_run_id = ? AND github_artifact_id = ?", buildID, []string{
		model.CustomBuildStatusBuilding,
		model.CustomBuildStatusDownloading,
		model.CustomBuildStatusExtracting,
	}, expectedRunID, expectedArtifactID).First(&build).Error; err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	if err := is.requireCompletionCapability(buildID); err != nil {
		return err
	}
	provenance, err := BuildProvenanceFromRecord(&build)
	if err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	if provenance.GithubRunID != expectedRunID || provenance.GithubArtifactID != expectedArtifactID || provenance.GithubArtifactID <= 0 {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("published output provenance does not match expected run and artifact")}
	}
	storedManifestJSON := build.ProducerManifestJSON
	manifestJSON := storedManifestJSON
	if len(producerManifest) > 1 {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: errors.New("at most one producer manifest may be recorded")}
	}
	if len(producerManifest) == 1 {
		manifest := producerManifest[0]
		if err := ValidateProducerManifestForBuild(manifest, &build); err != nil {
			return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
		}
		manifestJSON, err = manifest.StoredJSON()
		if err != nil {
			return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
		}
		if storedManifestJSON != "" && storedManifestJSON != manifestJSON {
			return &BuildProgressPersistenceError{BuildID: buildID, Cause: errors.New("producer manifest provenance is write-once")}
		}
	}
	if manifestJSON != "" {
		storedManifest, manifestErr := ProducerManifestFromStoredJSON(manifestJSON)
		if manifestErr != nil {
			return &BuildProgressPersistenceError{BuildID: buildID, Cause: manifestErr}
		}
		if err := ValidateProducerManifestForBuild(storedManifest, &build); err != nil {
			return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
		}
	}
	_, digest, err := publishedOutputManifest(BuildOutputDir(buildID), &build)
	if err != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: err}
	}
	if build.PublicationRecordedAt > 0 || build.PublishedDigest != "" {
		if build.PublicationRecordedAt <= 0 || !publishedDigestMatches(build.PublishedDigest, digest) {
			return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("published output marker and digest are incomplete or mismatched")}
		}
		return nil
	}

	marker := time.Now().Unix()
	tx := DB.Model(&model.CustomBuild{}).
		Where("id = ? AND status IN ? AND github_run_id = ? AND github_artifact_id = ? AND (publication_recorded_at = 0 OR publication_recorded_at IS NULL) AND (published_digest = '' OR published_digest IS NULL)", buildID, []string{
			model.CustomBuildStatusBuilding,
			model.CustomBuildStatusDownloading,
			model.CustomBuildStatusExtracting,
		}, expectedRunID, expectedArtifactID)
	updates := map[string]any{"publication_recorded_at": marker, "published_digest": digest}
	if manifestJSON != "" {
		updates["producer_manifest_json"] = manifestJSON
	}
	tx = tx.Updates(updates)
	if tx.Error != nil {
		return &BuildProgressPersistenceError{BuildID: buildID, Cause: tx.Error}
	}
	if tx.RowsAffected > 0 {
		return nil
	}

	var recorded model.CustomBuild
	err = DB.Select("publication_recorded_at", "published_digest").Where("id = ? AND status IN ? AND github_run_id = ? AND github_artifact_id = ? AND publication_recorded_at > 0 AND published_digest <> ''", buildID, []string{
		model.CustomBuildStatusBuilding,
		model.CustomBuildStatusDownloading,
		model.CustomBuildStatusExtracting,
	}, expectedRunID, expectedArtifactID).First(&recorded).Error
	if err == nil && publishedDigestMatches(recorded.PublishedDigest, digest) {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("stored publication digest does not match computed digest")
	}
	return &BuildProgressPersistenceError{BuildID: buildID, Cause: fmt.Errorf("publication marker write affected no exact active row: %w", err)}
}

// UpdateProgress updates only asynchronous status and artifact metadata for the
// exact dispatched run. Updates(map) is intentional: zero values are
// meaningful and custom_json is never read from or written through this
// compatibility path.
func (is *CustomBuildService) UpdateProgress(progress BuildProgress) error {
	if progress.BuildID == 0 {
		return fmt.Errorf("build id is required")
	}
	if progress.ExpectedRunID <= 0 {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("expected github_run_id must be positive"),
		}
	}
	if !validBuildProgressStatus(progress.Status) {
		return &BuildProgressPersistenceError{BuildID: progress.BuildID, Cause: fmt.Errorf("invalid build progress status %q", progress.Status)}
	}
	if progress.Status == model.CustomBuildStatusDone {
		if progress.ExpectedArtifactID <= 0 {
			return &BuildProgressPersistenceError{
				BuildID: progress.BuildID,
				Cause:   fmt.Errorf("expected github_artifact_id must be positive for completion"),
			}
		}
		if err := is.requireCompletedPublication(progress.BuildID, progress.ExpectedRunID, progress.ExpectedArtifactID); err != nil {
			return err
		}
	}
	where := "id = ? AND status IN ? AND github_run_id = ?"
	args := []any{progress.BuildID, []string{
		model.CustomBuildStatusBuilding,
		model.CustomBuildStatusDownloading,
		model.CustomBuildStatusExtracting,
	}, progress.ExpectedRunID}
	if progress.Status == model.CustomBuildStatusDone {
		where += " AND github_artifact_id > 0 AND github_artifact_id = ? AND publication_recorded_at > 0 AND published_digest <> ''"
		args = append(args, progress.ExpectedArtifactID)
	}
	tx := DB.Model(&model.CustomBuild{}).
		Where(where, args...).
		Updates(map[string]any{
			"status":    progress.Status,
			"build_log": BoundBuildLog(progress.BuildLog, ""),
			"file_size": progress.FileSize,
		})
	if tx.Error != nil {
		return &BuildProgressPersistenceError{BuildID: progress.BuildID, Cause: tx.Error}
	}
	if tx.RowsAffected == 0 {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("progress update affected no rows"),
		}
	}
	return nil
}

// UpdateNoRunFailure closes an active legacy/partial row that never received
// a provider run. It is intentionally separate from UpdateProgress so the
// positive run-ID guard remains intact and no fake provider run is recorded.
func (is *CustomBuildService) UpdateNoRunFailure(progress BuildProgress) error {
	if progress.BuildID == 0 {
		return fmt.Errorf("build id is required")
	}
	if progress.ExpectedRunID != 0 {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("no-run failure must use expected github_run_id zero"),
		}
	}
	if progress.Status != model.CustomBuildStatusFailed {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("no-run failure status must be failed"),
		}
	}
	tx := DB.Model(&model.CustomBuild{}).
		Where("id = ? AND status IN ? AND (github_run_id = 0 OR github_run_id IS NULL)", progress.BuildID, []string{
			model.CustomBuildStatusBuilding,
			model.CustomBuildStatusDownloading,
			model.CustomBuildStatusExtracting,
		}).
		Updates(map[string]any{
			"status":    progress.Status,
			"build_log": BoundBuildLog(progress.BuildLog, ""),
			"file_size": progress.FileSize,
		})
	if tx.Error != nil {
		return &BuildProgressPersistenceError{BuildID: progress.BuildID, Cause: tx.Error}
	}
	if tx.RowsAffected == 0 {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("no-run failure update affected no active undispatched row"),
		}
	}
	return nil
}

// UpdatePendingFailure is the explicit pre-dispatch failure path. It may only
// transition a still-pending row that has no provider run, so it cannot be
// used to bypass the run guard for an already-dispatched build.
func (is *CustomBuildService) UpdatePendingFailure(progress BuildProgress) error {
	if progress.BuildID == 0 {
		return fmt.Errorf("build id is required")
	}
	if progress.ExpectedRunID != 0 {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("pending failure must not include expected github_run_id"),
		}
	}
	if progress.Status != model.CustomBuildStatusFailed {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("pending failure status must be failed"),
		}
	}
	tx := DB.Model(&model.CustomBuild{}).
		Where("id = ? AND status = ? AND (github_run_id = 0 OR github_run_id IS NULL)", progress.BuildID, model.CustomBuildStatusPending).
		Updates(map[string]any{
			"status":    progress.Status,
			"build_log": BoundBuildLog(progress.BuildLog, ""),
			"file_size": progress.FileSize,
		})
	if tx.Error != nil {
		return &BuildProgressPersistenceError{BuildID: progress.BuildID, Cause: tx.Error}
	}
	if tx.RowsAffected == 0 {
		return &BuildProgressPersistenceError{
			BuildID: progress.BuildID,
			Cause:   fmt.Errorf("pending failure update affected no undispatched pending rows"),
		}
	}
	return nil
}

func validBuildProgressStatus(status string) bool {
	switch status {
	case model.CustomBuildStatusBuilding, model.CustomBuildStatusDownloading, model.CustomBuildStatusExtracting, model.CustomBuildStatusDone, model.CustomBuildStatusFailed:
		return true
	default:
		return false
	}
}
