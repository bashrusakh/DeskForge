package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rustdesk-server/api/model"
)

func TestBuildHandoffManifestIsCanonicalAndContainsExactDigestFields(t *testing.T) {
	build := newCompleteBuildHandoffFixture(t)
	fixedGeneratedAt := time.Date(2026, time.August, 10, 12, 34, 56, 0, time.UTC)
	manifest, err := BuildHandoffManifestFromRecord(build, fixedGeneratedAt)
	if err != nil {
		t.Fatalf("BuildHandoffManifestFromRecord() error = %v", err)
	}
	if manifest.Schema != buildHandoffSchema || manifest.SchemaVersion != buildHandoffSchemaVersion {
		t.Fatalf("schema = %q/%d, want %q/%d", manifest.Schema, manifest.SchemaVersion, buildHandoffSchema, buildHandoffSchemaVersion)
	}
	if manifest.SourceSHA != build.BuildRef || manifest.SourceTag != build.SourceTag || manifest.WorkflowSelector != build.WorkflowSelector || manifest.WorkflowSHA != build.GithubRef {
		t.Fatalf("source/workflow identity = %#v, want source=%q/%q workflow=%q/%q", manifest, build.SourceTag, build.BuildRef, build.WorkflowSelector, build.GithubRef)
	}
	if manifest.ManifestSchema != ProducerManifestSchema || manifest.HandoffContract != buildHandoffContract || manifest.ExportRoute != buildHandoffExportRoute || manifest.ProducerReport.SourceTreeSHA == "" || len(manifest.ProducerReport.Submodules) != 1 || manifest.PublicationTimestamp != build.PublicationRecordedAt {
		t.Fatalf("handoff producer provenance = %#v, want explicit schema/contract/route/tree/submodule/timestamp", manifest)
	}
	if manifest.RunID != build.GithubRunId || manifest.RunHeadSHA != build.GithubSourceSha || manifest.ArtifactID != build.GithubArtifactID || manifest.PublishedDigest != build.PublishedDigest {
		t.Fatalf("exact run/artifact/digest fields = %#v, want run=%d head=%q artifact=%d digest=%q", manifest, build.GithubRunId, build.GithubSourceSha, build.GithubArtifactID, build.PublishedDigest)
	}
	if manifest.PublicationRecordedAt != build.PublicationRecordedAt || manifest.VerificationResult != HandoffVerificationStatusServiceVerified || manifest.VerificationStatus != HandoffVerificationStatusServiceVerified {
		t.Fatalf("publication verification = %#v, want publication=%d service_verified", manifest, build.PublicationRecordedAt)
	}
	if manifest.ReleaseRepository != build.GithubRepo || manifest.ReleaseID != build.AssetsReleaseID || manifest.ReleaseTag != build.AssetsRelease {
		t.Fatalf("release identity = %#v, want repo=%q id=%d tag=%q", manifest, build.GithubRepo, build.AssetsReleaseID, build.AssetsRelease)
	}
	if manifest.PublishedDigestScope != buildHandoffDigestScope {
		t.Fatalf("digest scope = %q, want %q", manifest.PublishedDigestScope, buildHandoffDigestScope)
	}
	if manifest.DigestScope != buildHandoffDigestScope || manifest.VerificationScope != buildHandoffVerificationScope || manifest.VerificationResult != HandoffVerificationStatusServiceVerified {
		t.Fatalf("handoff verification contract = %#v, want exact scope/result", manifest)
	}
	if manifest.ProducerReport.VerificationResult != ProducerManifestVerificationResult || manifest.ProducerReport.VerificationStatus != HandoffVerificationStatusReported || manifest.ProducerReport.VerificationResult == HandoffVerificationStatusServiceVerified || manifest.ProducerReport.VerificationScope != HandoffProducerVerificationScope {
		t.Fatalf("producer report = %#v, want explicitly reported producer source evidence", manifest.ProducerReport)
	}
	if len(manifest.OutputFiles) != 2 || manifest.OutputFiles[0].Name != "helper.dll" || manifest.OutputFiles[1].Name != "rustqs.exe" {
		t.Fatalf("redacted output files = %#v, want sorted non-secret entries", manifest.OutputFiles)
	}
	if manifest.OutputFiles[0].Size != int64(len("helper")) || manifest.OutputFiles[1].Size != int64(len("executable")) {
		t.Fatalf("redacted output sizes = %#v, want exact byte sizes", manifest.OutputFiles)
	}
	helperHash := sha256.Sum256([]byte("helper"))
	exeHash := sha256.Sum256([]byte("executable"))
	if manifest.OutputFiles[0].SHA256 != hex.EncodeToString(helperHash[:]) || manifest.OutputFiles[1].SHA256 != hex.EncodeToString(exeHash[:]) {
		t.Fatalf("redacted output hashes = %#v, want helper=%x exe=%x", manifest.OutputFiles, helperHash, exeHash)
	}
	if len(manifest.ReleaseAssets) != len(requiredOfflineAssetNames) {
		t.Fatalf("release assets = %d, want %d", len(manifest.ReleaseAssets), len(requiredOfflineAssetNames))
	}
	for index := 1; index < len(manifest.ReleaseAssets); index++ {
		if manifest.ReleaseAssets[index-1].Name >= manifest.ReleaseAssets[index].Name {
			t.Fatalf("release assets are not name ordered: %#v", manifest.ReleaseAssets)
		}
	}
	for _, asset := range manifest.ReleaseAssets {
		var expectedID int64
		var expectedDigest string
		switch asset.Name {
		case "windows-x64-release.zip":
			expectedID, expectedDigest = 101, "sha256:"+strings.Repeat("1", 64)
		case "usbmmidd_v2.zip":
			expectedID, expectedDigest = 102, "sha256:"+strings.Repeat("2", 64)
		case "rustdesk_printer_driver_v4-1.4.zip":
			expectedID, expectedDigest = 103, "sha256:"+strings.Repeat("3", 64)
		case "printer_driver_adapter.zip":
			expectedID, expectedDigest = 104, "sha256:"+strings.Repeat("4", 64)
		default:
			t.Fatalf("unexpected release asset %q", asset.Name)
		}
		if asset.ID != expectedID || asset.ProviderDigest != expectedDigest || asset.VerificationStatus != HandoffVerificationStatusReported || asset.VerificationScope != HandoffProviderAssetVerificationScope {
			t.Fatalf("release asset = %#v, want id=%d provider_digest=%q", asset, expectedID, expectedDigest)
		}
	}

	first, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatalf("first CanonicalJSON() error = %v", err)
	}
	second, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatalf("second CanonicalJSON() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("canonical JSON changed between encodes:\n%s\n%s", first, second)
	}
	thirdManifest, err := BuildHandoffManifestFromRecord(build, fixedGeneratedAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("repeat BuildHandoffManifestFromRecord() error = %v", err)
	}
	third, err := thirdManifest.CanonicalJSON()
	if err != nil {
		t.Fatalf("repeat CanonicalJSON() error = %v", err)
	}
	if string(first) != string(third) {
		t.Fatalf("handoff changed with request timestamp:\n%s\n%s", first, third)
	}
	var decoded map[string]any
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("canonical JSON is invalid: %v", err)
	}
	if _, ok := decoded["source_tree_sha"]; ok {
		t.Fatalf("producer source tree self-report was emitted at service level: %s", first)
	}
	if _, ok := decoded["submodules"]; ok {
		t.Fatalf("producer submodule self-report was emitted at service level: %s", first)
	}
	reported, ok := decoded["producer_report"].(map[string]any)
	if !ok {
		t.Fatalf("producer_report = %#v, want explicit reported-evidence object", decoded["producer_report"])
	}
	if reported["verification_status"] != HandoffVerificationStatusReported || reported["verification_result"] == HandoffVerificationStatusServiceVerified {
		t.Fatalf("producer report self-report status = %#v, want reported and never service_verified", reported)
	}
}

func TestBuildHandoffManifestRedactsSecretsAndPaths(t *testing.T) {
	build := newCompleteBuildHandoffFixture(t)
	build.CustomJson = `{"permanent_password":"password-secret","payload_key":"payload-secret","server_key":"server-secret","path":"/rdgen-data/output/91","file":"custom_.txt"}`
	manifest, err := BuildHandoffManifestFromRecord(build, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("BuildHandoffManifestFromRecord() error = %v", err)
	}
	encoded, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	for _, forbidden := range []string{
		"password-secret",
		"payload-secret",
		"server-secret",
		"/rdgen-data/output/91",
		"custom_json",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("manifest contains forbidden value %q: %s", forbidden, encoded)
		}
	}
	if strings.Contains(string(encoded), "secret-bearing settings must not be exported") {
		t.Fatal("manifest contains custom_.txt contents")
	}
}

func TestBuildHandoffManifestFailsWhenOutputMutatesDuringCollection(t *testing.T) {
	build := newCompleteBuildHandoffFixture(t)
	previousHook := publishedOutputEntriesHook
	publishedOutputEntriesHook = func() {
		if err := os.WriteFile(filepath.Join(BuildOutputDir(build.Id), "helper.dll"), []byte("mutated"), 0600); err != nil {
			t.Fatalf("mutate published output: %v", err)
		}
	}
	t.Cleanup(func() { publishedOutputEntriesHook = previousHook })
	if _, err := BuildHandoffManifestFromRecord(build, time.Time{}); err == nil {
		t.Fatal("BuildHandoffManifestFromRecord() error = nil, want TOCTOU rejection")
	}
}

func TestBuildHandoffManifestFailsWhenStoredProducerProvenanceIsTampered(t *testing.T) {
	build := newCompleteBuildHandoffFixture(t)
	build.ProducerManifestJSON = strings.Replace(build.ProducerManifestJSON, build.BuildRef, strings.Repeat("e", 40), 1)
	if _, err := BuildHandoffManifestFromRecord(build, time.Time{}); err == nil {
		t.Fatal("BuildHandoffManifestFromRecord() error = nil, want stored provenance mismatch rejection")
	}
}

func TestBuildHandoffManifestFailsClosedForPartialLegacyAndCredentialURLs(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*model.CustomBuild)
	}{
		{name: "legacy", mutate: func(build *model.CustomBuild) {
			*build = model.CustomBuild{IdModel: model.IdModel{Id: 92}, Status: model.CustomBuildStatusDone, Platform: "windows"}
		}},
		{name: "missing run head sha", mutate: func(build *model.CustomBuild) {
			build.GithubSourceSha = ""
		}},
		{name: "missing publication marker", mutate: func(build *model.CustomBuild) {
			build.PublicationRecordedAt = 0
		}},
		{name: "credential-bearing run URL", mutate: func(build *model.CustomBuild) {
			build.GithubRunUrl = "https://operator:pat@example.invalid/run"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build := newCompleteBuildHandoffFixture(t)
			tc.mutate(build)
			if _, err := BuildHandoffManifestFromRecord(build, time.Unix(1700000000, 0)); err == nil {
				t.Fatal("BuildHandoffManifestFromRecord() error = nil, want fail-closed error")
			}
		})
	}
}

func newCompleteBuildHandoffFixture(t *testing.T) *model.CustomBuild {
	t.Helper()
	root := t.TempDir()
	previousOutputDir := BuildOutputDir
	BuildOutputDir = func(id uint) string {
		return filepath.Join(root, "output", "build", "91")
	}
	t.Cleanup(func() { BuildOutputDir = previousOutputDir })

	provenance := testBuildProvenance(91, "owner/repo", "windows-min-test.yml", "refs/heads/rustqs/min-test", "rustdesk-min-test-windows")
	provenance.GithubArtifactID = 901
	provenance.GithubSourceSHA = provenance.WorkflowSHA
	assets := append([]ReleaseAsset(nil), provenance.AssetsReleaseAssets...)
	for left, right := 0, len(assets)-1; left < right; left, right = left+1, right-1 {
		assets[left], assets[right] = assets[right], assets[left]
	}
	assetsJSON, err := json.Marshal(assets)
	if err != nil {
		t.Fatalf("marshal fixture assets: %v", err)
	}
	build := &model.CustomBuild{
		IdModel:               model.IdModel{Id: 91},
		Status:                model.CustomBuildStatusDone,
		Platform:              "windows",
		AppName:               "rustqs",
		Version:               provenance.Version,
		BuildRef:              provenance.BuildRef,
		SourceTag:             provenance.SourceTag,
		AssetsRelease:         provenance.AssetsRelease,
		AssetsReleaseID:       provenance.AssetsReleaseID,
		AssetsReleaseAssets:   string(assetsJSON),
		GithubProvider:        provenance.GithubProvider,
		GithubRepo:            provenance.GithubRepo,
		GithubWorkflow:        provenance.GithubWorkflow,
		WorkflowSelector:      provenance.WorkflowRef,
		GithubRef:             provenance.WorkflowSHA,
		GithubSourceSha:       provenance.GithubSourceSHA,
		GithubArtifactName:    provenance.GithubArtifactName,
		GithubArtifactID:      provenance.GithubArtifactID,
		GithubRunId:           provenance.GithubRunID,
		GithubRunUrl:          provenance.GithubRunURL,
		GithubHtmlUrl:         provenance.GithubHTMLURL,
		PublicationRecordedAt: 1700000000,
	}
	outputDir := BuildOutputDir(build.Id)
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		t.Fatalf("create fixture output: %v", err)
	}
	for name, contents := range map[string]string{
		"rustqs.exe":  "executable",
		"helper.dll":  "helper",
		"custom_.txt": "secret-bearing settings must not be exported",
	} {
		if err := os.WriteFile(filepath.Join(outputDir, name), []byte(contents), 0600); err != nil {
			t.Fatalf("write fixture output %q: %v", name, err)
		}
	}
	producerManifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "executable"})
	producerManifestJSON, err := producerManifest.StoredJSON()
	if err != nil {
		t.Fatalf("ProducerManifest.StoredJSON() error: %v", err)
	}
	build.ProducerManifestJSON = producerManifestJSON
	digest, err := PublishedOutputDigest(build)
	if err != nil {
		t.Fatalf("PublishedOutputDigest() error = %v", err)
	}
	build.PublishedDigest = digest
	return build
}
