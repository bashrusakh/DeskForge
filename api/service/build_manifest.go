package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"rustdesk-server/api/model"
)

const (
	buildHandoffSchema                       = "deskforge.custom-build.handoff"
	buildHandoffSchemaVersion                = 2
	buildHandoffDigestScope                  = "stored service SHA-256 covers canonical published output, including private custom_.txt when present; public archive SHA-256 covers separately redacted ZIP bytes; neither is a signature or a newly generated ZIP container"
	buildHandoffVerificationScope            = "service-verified stored build/source/workflow/run/artifact/release identity, producer manifest identity, publication proof, and current output bytes; producer source-tree/submodule and provider asset-digest evidence remain reported"
	buildHandoffContract                     = "deskforge.custom-build-export-v1"
	buildHandoffExportRoute                  = "GET /api/admin/custom_build/manifest/:id"
	maxBuildHandoffJSONBytes                 = 64 << 10
	HandoffVerificationStatusServiceVerified = "service_verified"
	HandoffVerificationStatusReported        = "reported"
	HandoffProducerVerificationScope         = "producer-reported source-tree and recursive submodule evidence; not independently recomputed by DeskForge"
	HandoffProviderAssetVerificationScope    = "provider-reported release asset SHA-256; copied as evidence and not independently recomputed by DeskForge"
)

// BuildHandoffAsset is the exact provider release asset identity retained for
// an operator handoff. ProviderDigest is copied from the provider; it is not a
// signature and is not recomputed by the handoff serializer.
type BuildHandoffAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	ProviderDigest     string `json:"provider_digest"`
	VerificationScope  string `json:"verification_scope"`
	VerificationStatus string `json:"verification_status"`
}

// BuildHandoffOutputFile is public metadata for one non-secret published
// output. It never carries file contents; custom_.txt is omitted entirely.
type BuildHandoffOutputFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type BuildHandoffSubmodule struct {
	Path      string `json:"path"`
	CommitSHA string `json:"commit_sha"`
}

// BuildHandoffProducerReport keeps producer-supplied source-tree metadata
// explicit without promoting it to independently verified provenance.
type BuildHandoffProducerReport struct {
	SourceTreeSHA      string                  `json:"source_tree_sha"`
	Submodules         []BuildHandoffSubmodule `json:"submodules"`
	VerificationScope  string                  `json:"verification_scope"`
	VerificationResult string                  `json:"verification_result"`
	VerificationStatus string                  `json:"verification_status"`
}

// BuildHandoffManifest is the redacted, versioned operator handoff contract.
// It contains only immutable build/provenance data and service-owned
// publication proof. It intentionally has no secrets, raw custom JSON,
// filesystem paths, output-file contents, or signature claim.
//
// JSON is produced from this fixed-field struct rather than a map, so field
// ordering is stable for canonical handoff/checksum comparisons. The stored
// publication timestamp is canonical and must not be confused with the
// published output digest, which covers canonical output rather than a ZIP
// made for a download request.
type BuildHandoffManifest struct {
	Schema                string                     `json:"schema"`
	SchemaVersion         int                        `json:"schema_version"`
	ManifestSchema        string                     `json:"manifest_schema"`
	HandoffContract       string                     `json:"handoff_contract"`
	ExportRoute           string                     `json:"export_route"`
	BuildID               uint                       `json:"build_id"`
	Status                string                     `json:"status"`
	Platform              string                     `json:"platform"`
	Version               string                     `json:"version"`
	Provider              string                     `json:"provider"`
	Repository            string                     `json:"repository"`
	SourceTag             string                     `json:"source_tag"`
	SourceSHA             string                     `json:"source_sha"`
	WorkflowRepository    string                     `json:"workflow_repository"`
	WorkflowPath          string                     `json:"workflow_path"`
	WorkflowSelector      string                     `json:"workflow_selector"`
	WorkflowSHA           string                     `json:"workflow_sha"`
	RunID                 int64                      `json:"run_id"`
	RunURL                string                     `json:"run_url"`
	RunHTMLURL            string                     `json:"run_html_url"`
	RunHeadSHA            string                     `json:"run_head_sha"`
	ArtifactName          string                     `json:"artifact_name"`
	ArtifactID            int64                      `json:"artifact_id"`
	ReleaseRepository     string                     `json:"release_repository"`
	ReleaseID             int64                      `json:"release_id"`
	ReleaseTag            string                     `json:"release_tag"`
	ReleaseAssets         []BuildHandoffAsset        `json:"release_assets"`
	OutputFiles           []BuildHandoffOutputFile   `json:"output_files"`
	PublishedDigest       string                     `json:"published_digest"`
	PublishedDigestScope  string                     `json:"published_digest_scope"`
	DigestScope           string                     `json:"digest_scope"`
	VerificationScope     string                     `json:"verification_scope"`
	VerificationResult    string                     `json:"verification_result"`
	VerificationStatus    string                     `json:"verification_status"`
	ProducerReport        BuildHandoffProducerReport `json:"producer_report"`
	PublicationTimestamp  int64                      `json:"publication_timestamp"`
	PublicationRecordedAt int64                      `json:"publication_recorded_at"`
}

// BuildManifestService reads a stored build and creates a verified handoff.
// It performs no provider calls, publication, upload, or filesystem export.
type BuildManifestService struct{}

// ForBuild returns a redacted manifest only after revalidating the complete
// immutable provenance and current service-owned publication proof. The time
// argument is retained for route compatibility but is intentionally ignored;
// repeated exports use only stored publication state.
func (s *BuildManifestService) ForBuild(buildID uint, _ time.Time) (BuildHandoffManifest, error) {
	if buildID == 0 {
		return BuildHandoffManifest{}, errors.New("build id is required")
	}
	if DB == nil {
		return BuildHandoffManifest{}, errors.New("build database is unavailable")
	}
	var build model.CustomBuild
	if err := DB.Where("id = ?", buildID).First(&build).Error; err != nil {
		return BuildHandoffManifest{}, err
	}
	return BuildHandoffManifestFromRecord(&build, time.Time{})
}

// BuildHandoffManifestFromRecord creates a canonical manifest from an already
// loaded record. generatedAt is retained as a source-compatible, non-canonical
// argument and has no effect on the result.
func BuildHandoffManifestFromRecord(build *model.CustomBuild, _ time.Time) (BuildHandoffManifest, error) {
	if build == nil {
		return BuildHandoffManifest{}, errors.New("build record is required")
	}
	provenance, _, err := ValidateCompletedPublishedOutput(build)
	if err != nil {
		return BuildHandoffManifest{}, fmt.Errorf("completed build handoff is unavailable: %w", err)
	}
	if provenance.GithubSourceSHA == "" {
		return BuildHandoffManifest{}, errors.New("completed build handoff requires the provider run head SHA")
	}
	producerManifest, err := ProducerManifestFromStoredJSON(build.ProducerManifestJSON)
	if err != nil {
		return BuildHandoffManifest{}, errors.New("completed build handoff requires validated producer manifest provenance")
	}
	if err := ValidateProducerManifestForBuild(producerManifest, build); err != nil {
		return BuildHandoffManifest{}, fmt.Errorf("completed build producer provenance is invalid: %w", err)
	}
	if strings.EqualFold(provenance.GithubArtifactName, "custom_.txt") {
		return BuildHandoffManifest{}, errors.New("completed build handoff contains a forbidden artifact name")
	}
	if err := validateHandoffURL("run_url", provenance.GithubRunURL); err != nil {
		return BuildHandoffManifest{}, err
	}
	if err := validateHandoffURL("run_html_url", provenance.GithubHTMLURL); err != nil {
		return BuildHandoffManifest{}, err
	}
	outputFiles, err := PublishedOutputFileEntries(build)
	if err != nil {
		return BuildHandoffManifest{}, fmt.Errorf("read redacted published output entries: %w", err)
	}

	assets := make([]BuildHandoffAsset, 0, len(provenance.AssetsReleaseAssets))
	for _, asset := range provenance.AssetsReleaseAssets {
		assets = append(assets, BuildHandoffAsset{
			ID:                 asset.ID,
			Name:               asset.Name,
			ProviderDigest:     asset.Digest,
			VerificationScope:  HandoffProviderAssetVerificationScope,
			VerificationStatus: HandoffVerificationStatusReported,
		})
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].Name < assets[j].Name
	})
	submodules := make([]BuildHandoffSubmodule, 0, len(producerManifest.Submodules))
	for _, submodule := range producerManifest.Submodules {
		submodules = append(submodules, BuildHandoffSubmodule{Path: submodule.Path, CommitSHA: submodule.CommitSHA})
	}

	manifest := BuildHandoffManifest{
		Schema:               buildHandoffSchema,
		SchemaVersion:        buildHandoffSchemaVersion,
		ManifestSchema:       producerManifest.ManifestSchema,
		HandoffContract:      buildHandoffContract,
		ExportRoute:          buildHandoffExportRoute,
		BuildID:              build.Id,
		Status:               build.Status,
		Platform:             build.Platform,
		Version:              provenance.Version,
		Provider:             provenance.GithubProvider,
		Repository:           provenance.GithubRepo,
		SourceTag:            provenance.SourceTag,
		SourceSHA:            provenance.BuildRef,
		WorkflowRepository:   provenance.GithubRepo,
		WorkflowPath:         provenance.GithubWorkflow,
		WorkflowSelector:     provenance.WorkflowRef,
		WorkflowSHA:          provenance.WorkflowSHA,
		RunID:                provenance.GithubRunID,
		RunURL:               provenance.GithubRunURL,
		RunHTMLURL:           provenance.GithubHTMLURL,
		RunHeadSHA:           provenance.GithubSourceSHA,
		ArtifactName:         provenance.GithubArtifactName,
		ArtifactID:           provenance.GithubArtifactID,
		ReleaseRepository:    provenance.GithubRepo,
		ReleaseID:            provenance.AssetsReleaseID,
		ReleaseTag:           provenance.AssetsRelease,
		ReleaseAssets:        assets,
		OutputFiles:          outputFiles,
		PublishedDigest:      build.PublishedDigest,
		PublishedDigestScope: buildHandoffDigestScope,
		DigestScope:          buildHandoffDigestScope,
		VerificationScope:    buildHandoffVerificationScope,
		VerificationResult:   HandoffVerificationStatusServiceVerified,
		VerificationStatus:   HandoffVerificationStatusServiceVerified,
		ProducerReport: BuildHandoffProducerReport{
			SourceTreeSHA:      producerManifest.SourceTreeSHA,
			Submodules:         append([]BuildHandoffSubmodule(nil), submodules...),
			VerificationScope:  HandoffProducerVerificationScope,
			VerificationResult: ProducerManifestVerificationResult,
			VerificationStatus: HandoffVerificationStatusReported,
		},
		PublicationTimestamp:  build.PublicationRecordedAt,
		PublicationRecordedAt: build.PublicationRecordedAt,
	}
	encoded, err := manifest.CanonicalJSON()
	if err != nil {
		return BuildHandoffManifest{}, fmt.Errorf("encode build handoff: %w", err)
	}
	if len(encoded) > maxBuildHandoffJSONBytes {
		return BuildHandoffManifest{}, errors.New("build handoff exceeds bounded JSON size")
	}
	return manifest, nil
}

// CanonicalJSON encodes the fixed-field handoff DTO without a map or
// nondeterministic data. It does not add a signature or a manifest checksum.
func (m BuildHandoffManifest) CanonicalJSON() ([]byte, error) {
	encoded, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal build handoff: %w", err)
	}
	if len(encoded) > maxBuildHandoffJSONBytes {
		return nil, errors.New("build handoff exceeds bounded JSON size")
	}
	return encoded, nil
}

func validateHandoffURL(field, raw string) error {
	if strings.ContainsAny(raw, "\r\n") {
		return fmt.Errorf("%s contains invalid control characters", field)
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s is not a credential-free provider URL", field)
	}
	return nil
}
