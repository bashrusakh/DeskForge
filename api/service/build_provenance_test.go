package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"rustdesk-server/api/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSetProvenanceIsOneShotAndStoresExactIdentity(t *testing.T) {
	db := newBuildProvenanceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusPending, Platform: "windows"}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	first := testBuildProvenance(101, "owner/repo-a", "workflow-a.yml", "refs/heads/ref-a", "artifact-a")
	if err := (&CustomBuildService{}).SetProvenance(build.Id, first); err != nil {
		t.Fatalf("SetProvenance() error = %v", err)
	}
	second := testBuildProvenance(202, "owner/repo-b", "workflow-b.yml", "refs/heads/ref-b", "artifact-b")
	err := (&CustomBuildService{}).SetProvenance(build.Id, second)
	var persistenceErr *BuildProvenancePersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("second SetProvenance() error = %T %v, want typed persistence error", err, err)
	}

	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusBuilding || stored.BuildLog != "github run id: 101" {
		t.Fatalf("stored dispatch state = %#v", stored)
	}
	got, err := BuildProvenanceFromRecord(&stored)
	if err != nil {
		t.Fatalf("BuildProvenanceFromRecord() error = %v", err)
	}
	if !reflect.DeepEqual(got, first) {
		t.Fatalf("stored provenance = %#v, want %#v", got, first)
	}
}

func TestCompletedBuildProvenanceRequiresDoneAndPublicationMarker(t *testing.T) {
	provenance := testBuildProvenance(101, "owner/repo", "workflow.yml", "refs/heads/ref", "artifact")
	provenance.GithubArtifactID = 42
	build := &model.CustomBuild{
		Status:             model.CustomBuildStatusDone,
		Version:            provenance.Version,
		BuildRef:           provenance.BuildRef,
		SourceTag:          provenance.SourceTag,
		AssetsRelease:      provenance.AssetsRelease,
		AssetsReleaseID:    provenance.AssetsReleaseID,
		GithubProvider:     provenance.GithubProvider,
		GithubRepo:         provenance.GithubRepo,
		GithubWorkflow:     provenance.GithubWorkflow,
		WorkflowSelector:   provenance.WorkflowRef,
		GithubRef:          provenance.GithubRef,
		GithubArtifactName: provenance.GithubArtifactName,
		GithubArtifactID:   provenance.GithubArtifactID,
		GithubRunId:        provenance.GithubRunID,
		GithubRunUrl:       provenance.GithubRunURL,
		GithubHtmlUrl:      provenance.GithubHTMLURL,
		PublishedDigest:    strings.Repeat("a", 64),
	}
	assetsJSON, err := json.Marshal(testReleaseAssets())
	if err != nil {
		t.Fatalf("marshal release assets: %v", err)
	}
	build.AssetsReleaseAssets = string(assetsJSON)
	if _, err := CompletedBuildProvenanceFromRecord(build); err == nil {
		t.Fatal("CompletedBuildProvenanceFromRecord() error = nil for unmarked done row")
	}
	build.PublicationRecordedAt = 1
	got, err := CompletedBuildProvenanceFromRecord(build)
	if err != nil {
		t.Fatalf("CompletedBuildProvenanceFromRecord() marked error = %v", err)
	}
	if !reflect.DeepEqual(got, provenance) {
		t.Fatalf("completed provenance = %#v, want %#v", got, provenance)
	}
	build.Status = model.CustomBuildStatusBuilding
	if _, err := CompletedBuildProvenanceFromRecord(build); err == nil {
		t.Fatal("CompletedBuildProvenanceFromRecord() error = nil for non-done marked row")
	}
}

func TestValidateCompletedPublishedOutputRequiresProductionCapability(t *testing.T) {
	provenance := testBuildProvenance(202, "owner/repo", "workflow.yml", "refs/heads/ref", "artifact")
	provenance.GithubArtifactID = 42
	for _, platform := range []string{string(PlatformLinux), string(PlatformAndroid)} {
		t.Run(platform, func(t *testing.T) {
			build := &model.CustomBuild{
				Status:                model.CustomBuildStatusDone,
				Platform:              platform,
				Version:               provenance.Version,
				BuildRef:              provenance.BuildRef,
				SourceTag:             provenance.SourceTag,
				AssetsRelease:         provenance.AssetsRelease,
				AssetsReleaseID:       provenance.AssetsReleaseID,
				GithubProvider:        provenance.GithubProvider,
				GithubRepo:            provenance.GithubRepo,
				GithubWorkflow:        provenance.GithubWorkflow,
				GithubRef:             provenance.GithubRef,
				GithubArtifactName:    provenance.GithubArtifactName,
				GithubArtifactID:      provenance.GithubArtifactID,
				GithubRunId:           provenance.GithubRunID,
				GithubRunUrl:          provenance.GithubRunURL,
				GithubHtmlUrl:         provenance.GithubHTMLURL,
				PublicationRecordedAt: 1,
				PublishedDigest:       strings.Repeat("a", 64),
			}
			_, _, err := ValidateCompletedPublishedOutput(build)
			var unavailable *ProductionCapabilityUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("ValidateCompletedPublishedOutput() error = %T %v, want capability-unavailable error", err, err)
			}
			if unavailable.Platform != platform {
				t.Fatalf("capability error platform = %q, want %q", unavailable.Platform, platform)
			}
		})
	}
}

func TestCreateNormalizedWithIdentityPersistsWriteOnceVersionFields(t *testing.T) {
	db := newBuildProvenanceDB(t)
	identity := VersionIdentity{
		Repo:           "owner/repo",
		DisplayVersion: "1.4.8",
		BuildRef:       strings.Repeat("a", 40),
		SourceTag:      "1.4.8",
		WorkflowRef:    defaultWorkflowExecutionRef,
		WorkflowSHA:    strings.Repeat("b", 40),
		AssetsRelease:  AssetsRelease{ID: 48, TagName: "offline-assets-1.4.8", Assets: testReleaseAssets()},
	}
	build := &model.CustomBuild{
		Status:     model.CustomBuildStatusPending,
		Platform:   "windows",
		Version:    identity.DisplayVersion,
		AppName:    "rustqs",
		CustomJson: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`,
	}
	normalized, err := (&CustomBuildService{}).CreateNormalizedWithIdentity(build, identity)
	if err != nil {
		t.Fatalf("CreateNormalizedWithIdentity() error = %v", err)
	}
	if normalized.DispatchParams["version"] != identity.DisplayVersion {
		t.Fatalf("dispatch version = %#v, want resolved display version %q", normalized.DispatchParams["version"], identity.DisplayVersion)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read identity build: %v", err)
	}
	if stored.Version != identity.DisplayVersion || stored.BuildRef != identity.BuildRef || stored.SourceTag != identity.SourceTag || stored.WorkflowSelector != identity.WorkflowRef || stored.GithubRef != identity.WorkflowSHA || stored.AssetsRelease != identity.AssetsRelease.TagName || stored.AssetsReleaseID != identity.AssetsRelease.ID || stored.AssetsReleaseAssets == "" {
		t.Fatalf("stored version identity = %#v, want %#v", stored, identity)
	}
	stored.Version = "1.4.7"
	if err := (&CustomBuildService{}).UpdateValidated(&stored); err == nil {
		t.Fatal("UpdateValidated() error = nil, want immutable version rejection")
	}
	var unchanged model.CustomBuild
	if err := db.First(&unchanged, build.Id).Error; err != nil {
		t.Fatalf("read unchanged identity build: %v", err)
	}
	if unchanged.Version != identity.DisplayVersion || unchanged.BuildRef != identity.BuildRef {
		t.Fatalf("immutable identity changed after rejected edit: %#v", unchanged)
	}
}

func TestCatalogResolvedIdentityDispatchesAndRejectsMismatchedRepo(t *testing.T) {
	db := newBuildProvenanceDB(t)
	if err := db.AutoMigrate(&model.GithubBuildConfig{}); err != nil {
		t.Fatalf("migrate GitHub config: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo"}).Error; err != nil {
		t.Fatalf("create GitHub config: %v", err)
	}
	resetVersionCatalogCache()
	t.Cleanup(resetVersionCatalogCache)
	buildRef := strings.Repeat("a", 40)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/repos/owner/repo/git/ref/heads/rustqs/workflows" {
			return githubResponse(http.StatusOK, `{"ref":"refs/heads/rustqs/workflows","object":{"sha":"`+strings.Repeat("b", 40)+`","type":"commit"}}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/releases") {
			return githubResponse(http.StatusOK, `[{"id":48,"tag_name":"offline-assets-1.4.8"}]`, nil), nil
		}
		if req.URL.Path == "/repos/owner/repo/git/releases/48" || req.URL.Path == "/repos/owner/repo/releases/48" {
			return githubResponse(http.StatusOK, testReleaseDetails(48, "offline-assets-1.4.8"), nil), nil
		}
		return githubResponse(http.StatusOK, `{"ref":"refs/tags/1.4.8","object":{"sha":"`+buildRef+`","type":"commit"}}`, nil), nil
	}))

	identity, err := (&GithubBuildConfigService{}).ResolveVersion(context.Background(), "1.4.8")
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}
	build := &model.CustomBuild{
		Status:     model.CustomBuildStatusPending,
		Platform:   "windows",
		Version:    identity.DisplayVersion,
		AppName:    "rustqs",
		CustomJson: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`,
	}
	if _, err := (&CustomBuildService{}).CreateNormalizedWithIdentity(build, identity); err != nil {
		t.Fatalf("CreateNormalizedWithIdentity() error = %v", err)
	}
	provenance := testBuildProvenance(606, identity.Repo, "workflow.yml", identity.BuildRef, "artifact-a")
	provenance.Version = identity.DisplayVersion
	provenance.BuildRef = identity.BuildRef
	provenance.SourceTag = identity.SourceTag
	provenance.AssetsRelease = identity.AssetsRelease.TagName
	provenance.AssetsReleaseID = identity.AssetsRelease.ID
	provenance.WorkflowRef = identity.WorkflowRef
	provenance.WorkflowSHA = identity.WorkflowSHA
	provenance.GithubRef = identity.WorkflowSHA
	if err := (&CustomBuildService{}).SetProvenance(build.Id, provenance); err != nil {
		t.Fatalf("SetProvenance() for catalog identity error = %v", err)
	}

	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read dispatched build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusBuilding || stored.GithubRepo != identity.Repo || stored.BuildRef != identity.BuildRef {
		t.Fatalf("catalog dispatch state = %#v", stored)
	}

	mismatchedBuild := &model.CustomBuild{
		Status:     model.CustomBuildStatusPending,
		Platform:   "windows",
		Version:    identity.DisplayVersion,
		AppName:    "rustqs",
		CustomJson: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`,
	}
	if _, err := (&CustomBuildService{}).CreateNormalizedWithIdentity(mismatchedBuild, identity); err != nil {
		t.Fatalf("CreateNormalizedWithIdentity() for mismatch case error = %v", err)
	}
	mismatched := provenance
	mismatched.GithubRepo = "owner/other-repo"
	err = (&CustomBuildService{}).SetProvenance(mismatchedBuild.Id, mismatched)
	var persistenceErr *BuildProvenancePersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("mismatched repo error = %T %v, want typed persistence error", err, err)
	}
	stored = model.CustomBuild{}
	if err := db.First(&stored, mismatchedBuild.Id).Error; err != nil {
		t.Fatalf("read mismatched build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusPending || stored.GithubRepo != identity.Repo || stored.GithubRunId != 0 {
		t.Fatalf("mismatched repo changed prebound row = %#v", stored)
	}
}

func TestCreateNormalizedWithIdentityRejectsInvalidVersionBeforeDBCreate(t *testing.T) {
	db := newBuildProvenanceDB(t)
	identity := VersionIdentity{
		Repo:           "owner/repo",
		DisplayVersion: "not-a-version",
		BuildRef:       strings.Repeat("a", 40),
		SourceTag:      "not-a-version",
		AssetsRelease:  AssetsRelease{ID: 1, TagName: "offline-assets-not-a-version"},
	}
	build := &model.CustomBuild{Status: model.CustomBuildStatusPending, Platform: "windows", Version: identity.DisplayVersion}
	if _, err := (&CustomBuildService{}).CreateNormalizedWithIdentity(build, identity); err == nil || !IsClientValidationError(err) {
		t.Fatalf("invalid identity error = %v, want client validation error", err)
	}
	var count int64
	if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
		t.Fatalf("count builds: %v", err)
	}
	if count != 0 {
		t.Fatalf("invalid identity created %d row(s)", count)
	}
}

func TestCreateNormalizedWithIdentityRejectsIncompleteProductionBuildBeforeDBCreate(t *testing.T) {
	const completeWindowsJSON = `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`
	identity := VersionIdentity{
		Repo:           "owner/repo",
		DisplayVersion: "1.4.8",
		BuildRef:       strings.Repeat("a", 40),
		SourceTag:      "1.4.8",
		WorkflowRef:    defaultWorkflowExecutionRef,
		WorkflowSHA:    strings.Repeat("b", 40),
		AssetsRelease:  AssetsRelease{ID: 48, TagName: "offline-assets-1.4.8", Assets: testReleaseAssets()},
	}
	cases := []struct {
		name       string
		platform   string
		version    string
		appName    string
		customJSON string
	}{
		{name: "missing platform", platform: "", version: identity.DisplayVersion, appName: "rustqs", customJSON: completeWindowsJSON},
		{name: "whitespace version", platform: "windows", version: " \t ", appName: "rustqs", customJSON: completeWindowsJSON},
		{name: "whitespace app name", platform: "windows", version: identity.DisplayVersion, appName: " \t ", customJSON: completeWindowsJSON},
		{name: "missing server endpoint", platform: "windows", version: identity.DisplayVersion, appName: "rustqs", customJSON: `{"key":"public-key","api_server":"https://api.example","relay_server":"relay.example:21117"}`},
		{name: "whitespace public key", platform: "windows", version: identity.DisplayVersion, appName: "rustqs", customJSON: `{"server_ip":"id.example:21116","key":" \t ","api_server":"https://api.example","relay_server":"relay.example:21117"}`},
		{name: "missing API endpoint", platform: "windows", version: identity.DisplayVersion, appName: "rustqs", customJSON: `{"server_ip":"id.example:21116","key":"public-key","relay_server":"relay.example:21117"}`},
		{name: "whitespace relay endpoint", platform: "windows", version: identity.DisplayVersion, appName: "rustqs", customJSON: `{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","relay_server":" \t "}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newBuildProvenanceDB(t)
			build := &model.CustomBuild{
				Status:     model.CustomBuildStatusPending,
				Platform:   tc.platform,
				Version:    tc.version,
				AppName:    tc.appName,
				CustomJson: tc.customJSON,
			}
			if _, err := (&CustomBuildService{}).CreateNormalizedWithIdentity(build, identity); err == nil || !IsClientValidationError(err) {
				t.Fatalf("CreateNormalizedWithIdentity() error = %v, want client validation error", err)
			}
			var count int64
			if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
				t.Fatalf("count builds: %v", err)
			}
			if count != 0 {
				t.Fatalf("incomplete production build created %d row(s)", count)
			}
		})
	}
}

func TestSetProvenanceRequiresPendingAndEmptyIdentity(t *testing.T) {
	db := newBuildProvenanceDB(t)
	first := testBuildProvenance(101, "owner/repo-a", "workflow-a.yml", "refs/heads/ref-a", "artifact-a")
	cases := []struct {
		name  string
		build model.CustomBuild
	}{
		{name: "wrong status", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding}},
		{name: "existing run", build: model.CustomBuild{Status: model.CustomBuildStatusPending, GithubRunId: 202}},
		{name: "partial provider identity", build: model.CustomBuild{Status: model.CustomBuildStatusPending, GithubProvider: "github"}},
		{name: "partial artifact identity", build: model.CustomBuild{Status: model.CustomBuildStatusPending, GithubArtifactName: "artifact-a"}},
		{name: "partial source identity", build: model.CustomBuild{Status: model.CustomBuildStatusPending, GithubSourceSha: strings.Repeat("a", 40)}},
		{name: "partial repo identity", build: model.CustomBuild{Status: model.CustomBuildStatusPending, GithubRepo: "owner/repo-a"}},
		{name: "partial version identity", build: model.CustomBuild{Status: model.CustomBuildStatusPending, Version: "1.2.3", BuildRef: strings.Repeat("a", 40)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build := tc.build
			if err := db.Create(&build).Error; err != nil {
				t.Fatalf("create build: %v", err)
			}
			if err := (&CustomBuildService{}).SetProvenance(build.Id, first); err == nil {
				t.Fatal("SetProvenance() error = nil, want guard rejection")
			}
			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read build: %v", err)
			}
			if stored.Status != build.Status || stored.GithubRunId != build.GithubRunId || stored.GithubProvider != build.GithubProvider || stored.GithubArtifactName != build.GithubArtifactName || stored.GithubSourceSha != build.GithubSourceSha {
				t.Fatalf("guarded row changed: %#v, want %#v", stored, build)
			}
		})
	}
}

func TestSetSourceShaIsGuardedAndCannotOverwrite(t *testing.T) {
	db := newBuildProvenanceDB(t)
	shaA := strings.Repeat("a", 40)
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, BuildRef: shaA, GithubRef: shaA}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	shaB := strings.Repeat("b", 40)
	if err := (&CustomBuildService{}).SetSourceSha(build.Id, build.GithubRunId, shaA); err != nil {
		t.Fatalf("first SetSourceSha() error = %v", err)
	}
	err := (&CustomBuildService{}).SetSourceSha(build.Id, build.GithubRunId, shaB)
	var persistenceErr *BuildProvenancePersistenceError
	if !errors.As(err, &persistenceErr) {
		t.Fatalf("second SetSourceSha() error = %T %v, want typed persistence error", err, err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.GithubSourceSha != shaA {
		t.Fatalf("source sha = %q, want first SHA", stored.GithubSourceSha)
	}
}

func TestSetSourceShaRequiresCurrentBuildingRun(t *testing.T) {
	db := newBuildProvenanceDB(t)
	sha := strings.Repeat("a", 40)
	cases := []struct {
		name     string
		build    model.CustomBuild
		expected int64
	}{
		{name: "pending", build: model.CustomBuild{Status: model.CustomBuildStatusPending, GithubRunId: 17}, expected: 17},
		{name: "missing run", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding}, expected: 17},
		{name: "wrong run", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17}, expected: 18},
		{name: "existing sha", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, GithubSourceSha: strings.Repeat("b", 40)}, expected: 17},
		{name: "current run", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, BuildRef: sha, GithubRef: sha}, expected: 17},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build := tc.build
			if err := db.Create(&build).Error; err != nil {
				t.Fatalf("create build: %v", err)
			}
			err := (&CustomBuildService{}).SetSourceSha(build.Id, tc.expected, sha)
			wantError := tc.name != "current run"
			if (err != nil) != wantError {
				t.Fatalf("SetSourceSha() error = %v, wantError=%v", err, wantError)
			}
			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read build: %v", err)
			}
			if wantError && stored.GithubSourceSha != build.GithubSourceSha {
				t.Fatalf("guarded SHA changed to %q, want %q", stored.GithubSourceSha, build.GithubSourceSha)
			}
			if !wantError && stored.GithubSourceSha != sha {
				t.Fatalf("source SHA = %q, want %q", stored.GithubSourceSha, sha)
			}
		})
	}
}

func TestSetSourceShaRejectsNonHexOrWrongLength(t *testing.T) {
	db := newBuildProvenanceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, BuildRef: strings.Repeat("a", 40), GithubRef: strings.Repeat("a", 40)}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	for _, sourceSHA := range []string{"", strings.Repeat("a", 39), strings.Repeat("a", 65), strings.Repeat("g", 40)} {
		if err := (&CustomBuildService{}).SetSourceSha(build.Id, build.GithubRunId, sourceSHA); err == nil {
			t.Errorf("SetSourceSha(%q) error = nil, want validation error", sourceSHA)
		}
	}
}

func TestSetSourceShaRequiresStoredBuildRefAndProvenanceRejectsMismatch(t *testing.T) {
	db := newBuildProvenanceDB(t)
	buildRef := strings.Repeat("a", 40)
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, BuildRef: buildRef, GithubRef: buildRef}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := (&CustomBuildService{}).SetSourceSha(build.Id, build.GithubRunId, strings.Repeat("b", 40)); err == nil {
		t.Fatal("SetSourceSha() error = nil, want stored BuildRef mismatch rejection")
	}
	if err := (&CustomBuildService{}).SetSourceSha(build.Id, build.GithubRunId, strings.ToUpper(buildRef)); err != nil {
		t.Fatalf("SetSourceSha() case-insensitive matching error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if _, err := BuildProvenanceFromRecord(&stored); err == nil {
		t.Fatal("BuildProvenanceFromRecord() error = nil, want incomplete provider identity error")
	}
	p := testBuildProvenance(18, "owner/repo", "workflow.yml", buildRef, "artifact")
	p.GithubSourceSHA = strings.Repeat("b", 40)
	if err := p.validate(); err == nil {
		t.Fatal("BuildProvenance.validate() error = nil, want source/build-ref mismatch")
	}
}

func TestSetArtifactIDIsWriteOnce(t *testing.T) {
	db := newBuildProvenanceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, GithubArtifactName: "artifact-a"}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := (&CustomBuildService{}).SetArtifactID(build.Id, build.GithubRunId, 101); err != nil {
		t.Fatalf("first SetArtifactID() error = %v", err)
	}
	if err := (&CustomBuildService{}).SetArtifactID(build.Id, build.GithubRunId, 202); err == nil {
		t.Fatal("second SetArtifactID() error = nil, want write-once rejection")
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.GithubArtifactID != 101 || stored.GithubArtifactName != build.GithubArtifactName {
		t.Fatalf("artifact id = %d, want 101", stored.GithubArtifactID)
	}
}

func TestSetArtifactIDRequiresCurrentBuildingRun(t *testing.T) {
	db := newBuildProvenanceDB(t)
	cases := []struct {
		name     string
		build    model.CustomBuild
		expected int64
	}{
		{name: "pending", build: model.CustomBuild{Status: model.CustomBuildStatusPending, GithubRunId: 17}, expected: 17},
		{name: "missing run", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding}, expected: 17},
		{name: "wrong run", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17}, expected: 18},
		{name: "existing artifact", build: model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, GithubArtifactID: 101}, expected: 17},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build := tc.build
			if err := db.Create(&build).Error; err != nil {
				t.Fatalf("create build: %v", err)
			}
			if err := (&CustomBuildService{}).SetArtifactID(build.Id, tc.expected, 202); err == nil {
				t.Fatal("SetArtifactID() error = nil, want guard rejection")
			}
			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read build: %v", err)
			}
			if stored.GithubArtifactID != build.GithubArtifactID || stored.GithubArtifactName != build.GithubArtifactName {
				t.Fatalf("guarded artifact identity changed: %#v, want id=%d name=%q", stored, build.GithubArtifactID, build.GithubArtifactName)
			}
		})
	}
}

func TestLegacyGithubRunWithoutProvenanceFailsClosed(t *testing.T) {
	legacy := &model.CustomBuild{GithubRunId: 77, Status: model.CustomBuildStatusBuilding}
	if _, err := BuildProvenanceFromRecord(legacy); err == nil {
		t.Fatal("BuildProvenanceFromRecord() error = nil, want fail-closed legacy error")
	}
}

func TestUpdateNoRunFailureClosesLegacyActiveRowWithoutProviderRun(t *testing.T) {
	db := newBuildProvenanceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 0, BuildLog: "legacy"}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create legacy active build: %v", err)
	}
	if err := (&CustomBuildService{}).UpdateNoRunFailure(BuildProgress{
		BuildID:  build.Id,
		Status:   model.CustomBuildStatusFailed,
		BuildLog: "no provider run exists; refusing to resume legacy build",
	}); err != nil {
		t.Fatalf("UpdateNoRunFailure() error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read failed legacy build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusFailed || stored.GithubRunId != 0 || !strings.Contains(stored.BuildLog, "no provider run exists") {
		t.Fatalf("legacy active row after no-run failure = %#v", stored)
	}

	positive := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17}
	if err := db.Create(positive).Error; err != nil {
		t.Fatalf("create positive-run build: %v", err)
	}
	if err := (&CustomBuildService{}).UpdateNoRunFailure(BuildProgress{
		BuildID:       positive.Id,
		ExpectedRunID: 0,
		Status:        model.CustomBuildStatusFailed,
	}); err == nil {
		t.Fatal("UpdateNoRunFailure() error = nil for positive-run row")
	}
}

func TestLegacyMutableWorkflowRefFailsClosed(t *testing.T) {
	// Keep a stale mutable workflow selector as a legacy fixture; migrated defaults
	// must continue to fail closed for mutable workflow selectors.
	legacy := &model.CustomBuild{
		Version:         "1.2.3",
		BuildRef:        strings.Repeat("a", 40),
		SourceTag:       "1.2.3",
		AssetsRelease:   "offline-assets-1.2.3",
		AssetsReleaseID: 12,
		GithubRepo:      "owner/repo",
		GithubRef:       "refs/heads/rustqs/workflows",
	}
	if _, err := VersionIdentityFromRecord(legacy); err == nil {
		t.Fatal("VersionIdentityFromRecord() error = nil, want mutable workflow ref rejection")
	}
}

func TestGithubConfigFromProvenanceIgnoresMutableGlobalIdentity(t *testing.T) {
	provenance := testBuildProvenance(303, "owner/repo-a", "workflow-a.yml", "ref-a", "artifact-a")
	globalConfig := &model.GithubBuildConfig{Repo: "owner/repo-b", WorkflowFilename: "workflow-b.yml", Branch: "ref-b", Token: "new-token"}
	requestConfig := GithubConfigFromProvenance(provenance, globalConfig.Token)
	if requestConfig.Repo != provenance.GithubRepo || requestConfig.WorkflowFilename != "" || requestConfig.Branch != provenance.WorkflowRef {
		t.Fatalf("request config = %#v, want stored repository and workflow selector identity", requestConfig)
	}
	if requestConfig.Token != globalConfig.Token {
		t.Fatalf("request config token = %q, want current token", requestConfig.Token)
	}
}

func TestStoredProvenanceTargetsOriginalRepoAndArtifactAfterConfigMutation(t *testing.T) {
	provenance := testBuildProvenance(404, "owner/repo-a", "workflow-a.yml", "ref-a", "artifact-a")
	mutableGlobalConfig := &model.GithubBuildConfig{Repo: "owner/repo-b", WorkflowFilename: "workflow-b.yml", Branch: "ref-b", Token: "current-token"}
	requestConfig := GithubConfigFromProvenance(provenance, mutableGlobalConfig.Token)
	artifactPayload := serviceArtifactZip(t, "artifact-a-bytes")
	var requestedArtifact string
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/repos/owner/repo-a/") {
			return githubResponse(http.StatusBadRequest, `{"message":"wrong repository"}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/actions/runs/404") {
			return githubResponse(http.StatusOK, `{"status":"in_progress","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/artifacts") {
			requestedArtifact = provenance.GithubArtifactName
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":9,"name":"artifact-a"}]}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/artifacts/9/zip") {
			return githubResponse(http.StatusOK, string(artifactPayload), nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
	}))

	details, err := (&GithubBuildConfigService{}).RunStatusDetails(context.Background(), requestConfig, provenance.GithubRunID)
	if err != nil || details.SourceSHA != strings.Repeat("a", 40) {
		t.Fatalf("RunStatusDetails() = %#v, %v", details, err)
	}
	artifact, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), requestConfig, provenance.GithubRunID, 0, provenance.GithubArtifactName)
	if err != nil {
		t.Fatalf("DownloadArtifact() error = %v, requested=%q", err, requestedArtifact)
	}
	t.Cleanup(func() { _ = os.Remove(artifact.ArchivePath) })
	contents, err := os.ReadFile(artifact.ArchivePath)
	if err != nil || !bytes.Equal(contents, artifactPayload) || artifact.ArtifactID != 9 || artifact.ArtifactName != "artifact-a" || requestedArtifact != "artifact-a" {
		t.Fatalf("DownloadArtifact() = %#v, contents=%q, err=%v, requested=%q", artifact, contents, err, requestedArtifact)
	}
}

func TestStoredArtifactIDSkipsNameFallbackAfterConfigMutation(t *testing.T) {
	provenance := testBuildProvenance(505, "owner/repo-a", "workflow-a.yml", "ref-a", "artifact-a")
	provenance.GithubArtifactID = 909
	mutableGlobalConfig := &model.GithubBuildConfig{Repo: "owner/repo-b", WorkflowFilename: "workflow-b.yml", Branch: "ref-b", Token: "current-token"}
	requestConfig := GithubConfigFromProvenance(provenance, mutableGlobalConfig.Token)
	artifactPayload := serviceArtifactZip(t, "stored-artifact")
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/repos/owner/repo-a/actions/runs/505/artifacts" {
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":909,"name":"artifact-a"}]}`, nil), nil
		}
		if req.URL.Path != "/repos/owner/repo-a/actions/artifacts/909/zip" {
			return githubResponse(http.StatusBadRequest, `{"message":"must use stored repository and artifact id"}`, nil), nil
		}
		return githubResponse(http.StatusOK, string(artifactPayload), nil), nil
	}))

	artifact, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), requestConfig, provenance.GithubRunID, provenance.GithubArtifactID, provenance.GithubArtifactName)
	if err != nil {
		t.Fatalf("DownloadArtifact() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(artifact.ArchivePath) })
	contents, err := os.ReadFile(artifact.ArchivePath)
	if err != nil || !bytes.Equal(contents, artifactPayload) || artifact.ArtifactID != provenance.GithubArtifactID || artifact.ArtifactName != provenance.GithubArtifactName {
		t.Fatalf("DownloadArtifact() = %#v, contents=%q, err=%v", artifact, contents, err)
	}
}

func TestRunStatusSourceSHACanBePersistedAfterInitialOmission(t *testing.T) {
	db := newBuildProvenanceDB(t)
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17, BuildRef: strings.Repeat("A", 64), GithubRef: strings.Repeat("A", 64)}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	sha := strings.Repeat("A", 64)
	if err := (&CustomBuildService{}).SetSourceSha(build.Id, build.GithubRunId, sha); err != nil {
		t.Fatalf("SetSourceSha() after omitted first response error = %v", err)
	}
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.GithubSourceSha != sha {
		t.Fatalf("source sha = %q, want %q", stored.GithubSourceSha, sha)
	}
}

func TestConcurrentProvenanceRecordsStayIsolated(t *testing.T) {
	db := newBuildProvenanceDB(t)
	builds := []*model.CustomBuild{{Status: model.CustomBuildStatusPending}, {Status: model.CustomBuildStatusPending}}
	for _, build := range builds {
		if err := db.Create(build).Error; err != nil {
			t.Fatalf("create build: %v", err)
		}
	}

	var wg sync.WaitGroup
	for index, build := range builds {
		index, build := index, build
		wg.Add(1)
		go func() {
			defer wg.Done()
			provenance := testBuildProvenance(int64(index+1), "owner/repo-"+string(rune('a'+index)), "workflow.yml", "ref-"+string(rune('a'+index)), "artifact-"+string(rune('a'+index)))
			if err := (&CustomBuildService{}).SetProvenance(build.Id, provenance); err != nil {
				t.Errorf("SetProvenance(build %d) error = %v", build.Id, err)
			}
		}()
	}
	wg.Wait()

	for index, build := range builds {
		var stored model.CustomBuild
		if err := db.First(&stored, build.Id).Error; err != nil {
			t.Fatalf("read build %d: %v", build.Id, err)
		}
		wantRepo := "owner/repo-" + string(rune('a'+index))
		if stored.GithubRepo != wantRepo || stored.GithubArtifactName != "artifact-"+string(rune('a'+index)) {
			t.Fatalf("build %d provenance crossed records: %#v", build.Id, stored)
		}
	}
}

func testBuildProvenance(runID int64, repo, workflow, ref, artifact string) BuildProvenance {
	run := strconv.FormatInt(runID, 10)
	buildRef := fmt.Sprintf("%040x", runID)
	return BuildProvenance{
		Version:             "1.2.3",
		BuildRef:            buildRef,
		SourceTag:           "1.2.3",
		AssetsRelease:       "offline-assets-1.2.3",
		AssetsReleaseID:     runID,
		AssetsReleaseAssets: testReleaseAssets(),
		GithubProvider:      "github",
		GithubRepo:          repo,
		GithubWorkflow:      workflow,
		WorkflowRef:         defaultWorkflowExecutionRef,
		WorkflowSHA:         fmt.Sprintf("%040x", runID+1000),
		GithubRef:           fmt.Sprintf("%040x", runID+1000),
		GithubArtifactName:  artifact,
		GithubRunID:         runID,
		GithubRunURL:        "https://api.github.com/repos/" + repo + "/actions/runs/" + run,
		GithubHTMLURL:       "https://github.com/" + repo + "/actions/runs/" + run,
	}
}

func newBuildProvenanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("SECRET_ENCRYPTION_KEY", "focused-test-at-rest-key")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.CustomBuild{}); err != nil {
		t.Fatalf("migrate custom builds: %v", err)
	}
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	return db
}
