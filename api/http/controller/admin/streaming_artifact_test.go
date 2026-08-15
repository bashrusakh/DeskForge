package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"rustdesk-server/api/global"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

func makeArtifactZip(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, contents := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create ZIP entry %q: %v", name, err)
		}
		if _, err := w.Write([]byte(contents)); err != nil {
			t.Fatalf("write ZIP entry %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close ZIP: %v", err)
	}
	archive, err := os.CreateTemp(t.TempDir(), "artifact-*.zip")
	if err != nil {
		t.Fatalf("create test archive: %v", err)
	}
	if _, err := archive.Write(buf.Bytes()); err != nil {
		t.Fatalf("write test archive: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close test archive: %v", err)
	}
	return archive.Name()
}

func withTestOutputRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	previous := customBuildOutputDir
	previousService := service.BuildOutputDir
	customBuildOutputDir = func(id uint) string {
		return filepath.Join(root, "output", fmt.Sprintf("%d", id))
	}
	service.BuildOutputDir = customBuildOutputDir
	t.Cleanup(func() {
		customBuildOutputDir = previous
		service.BuildOutputDir = previousService
	})
	return root
}

func TestPublishDownloadedArtifactAtomicallyPublishesValidatedWindowsOutput(t *testing.T) {
	root := withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 71}, Platform: "windows", AppName: "rustqs"}
	archive := makeArtifactZip(t, map[string]string{
		"rustqs.exe":  "exe",
		"helper.dll":  "dll",
		"custom_.txt": "settings",
	})

	size, err := publishDownloadedArtifact(build, archive)
	if err != nil {
		t.Fatalf("publishDownloadedArtifact() error = %v", err)
	}
	if size != int64(len("exe")) {
		t.Fatalf("published file size = %d, want exe size", size)
	}
	outDir := filepath.Join(root, "output", "71")
	for _, name := range []string{"rustqs.exe", "helper.dll", "custom_.txt"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("published %s: %v", name, err)
		}
	}
	if entries, err := os.ReadDir(filepath.Dir(outDir)); err != nil || len(entries) != 1 || entries[0].Name() != "71" {
		t.Fatalf("output parent entries = %v, err=%v; staging directory leaked", entries, err)
	}
}

func TestPublishDownloadedArtifactFailsClosedAndCleansStaging(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
	}{
		{name: "invalid zip", files: nil},
		{name: "zip slip", files: map[string]string{"../escape.exe": "bad"}},
		{name: "backslash traversal", files: map[string]string{`..\escape.exe`: "bad"}},
		{name: "absolute path", files: map[string]string{"/escape.exe": "bad"}},
		{name: "drive absolute path", files: map[string]string{`C:\escape.exe`: "bad"}},
		{name: "reserved device", files: map[string]string{"CON.exe": "bad"}},
		{name: "unsafe extension", files: map[string]string{"helper.bat": "bad"}},
		{name: "nested Windows path", files: map[string]string{"nested/helper.dll": "bad"}},
		{name: "missing required exe", files: map[string]string{"custom_.txt": "settings"}},
		{name: "duplicate output", files: map[string]string{"rustqs.exe": "one", "nested/rustqs.exe": "two"}},
		{name: "case-insensitive duplicate executable", files: map[string]string{"rustqs.exe": "one", "RUSTQS.EXE": "two"}},
		{name: "case-insensitive duplicate DLL", files: map[string]string{"rustqs.exe": "one", "helper.dll": "one", "HELPER.DLL": "two"}},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := withTestOutputRoot(t)
			build := &model.CustomBuild{IdModel: model.IdModel{Id: uint(80 + index)}, Platform: "windows", AppName: "rustqs"}
			archive := ""
			if tc.files == nil {
				file, err := os.CreateTemp(t.TempDir(), "invalid-*.zip")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.WriteString("not a ZIP"); err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				archive = file.Name()
			} else {
				archive = makeArtifactZip(t, tc.files)
			}
			if _, err := publishDownloadedArtifact(build, archive); err == nil {
				t.Fatal("publishDownloadedArtifact() error = nil, want fail-closed validation error")
			}
			outDir := filepath.Join(root, "output", fmt.Sprintf("%d", build.Id))
			if _, err := os.Stat(outDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("final output stat = %v, want no final output", err)
			}
			if entries, err := os.ReadDir(filepath.Dir(outDir)); err != nil {
				t.Fatalf("read output parent: %v", err)
			} else if len(entries) != 0 {
				t.Fatalf("output parent entries = %v, want staging cleanup", entries)
			}
			if _, err := os.Stat(filepath.Join(root, "output", "escape.exe")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("zip-slip escape stat = %v, want absent", err)
			}
		})
	}
}

func TestPublishDownloadedArtifactRejectsChecksumMismatch(t *testing.T) {
	root := withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 90}, Platform: "windows", AppName: "rustqs"}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: "rustqs.exe", Method: zip.Store}
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatalf("create stored ZIP entry: %v", err)
	}
	if _, err := w.Write([]byte("valid-exe")); err != nil {
		t.Fatalf("write stored ZIP entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close stored ZIP: %v", err)
	}
	corrupted := buf.Bytes()
	payload := bytes.Index(corrupted, []byte("valid-exe"))
	if payload < 0 {
		t.Fatal("stored ZIP payload not found")
	}
	corrupted[payload] = 'X'
	archive := filepath.Join(t.TempDir(), "checksum-invalid.zip")
	if err := os.WriteFile(archive, corrupted, 0600); err != nil {
		t.Fatalf("write corrupted ZIP: %v", err)
	}
	if _, err := publishDownloadedArtifact(build, archive); err == nil {
		t.Fatal("publishDownloadedArtifact() error = nil, want checksum failure")
	}
	if _, err := os.Stat(filepath.Join(root, "output", "90")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checksum-invalid final output stat = %v, want absent", err)
	}
}

func TestPublishDownloadedArtifactRequiresExactProducerManifestForActiveBuild(t *testing.T) {
	cases := []struct {
		name    string
		archive func(*testing.T) string
	}{
		{name: "missing manifest", archive: func(t *testing.T) string {
			return makeArtifactZip(t, map[string]string{"rustqs.exe": "exe"})
		}},
		{name: "extra output", archive: func(t *testing.T) string {
			manifest := producerManifestBytes(t, "exe")
			return makeArtifactZip(t, map[string]string{
				"rustqs.exe":   "exe",
				"helper.dll":   "extra",
				"manifest.txt": manifest,
			})
		}},
		{name: "case collision", archive: func(t *testing.T) string {
			manifest := producerManifestBytes(t, "exe")
			return makeArtifactZip(t, map[string]string{
				"RUSTQS.EXE":   "exe",
				"manifest.txt": manifest,
			})
		}},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTestOutputRoot(t)
			build := activeWindowsBuild(uint(140 + index))
			if _, err := publishDownloadedArtifact(build, tc.archive(t)); err == nil {
				t.Fatal("publishDownloadedArtifact() error = nil, want producer manifest rejection")
			}
			if _, err := os.Stat(customBuildOutputDir(build.Id)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("published output stat = %v, want absent", err)
			}
		})
	}
}

func TestExtractValidatedArtifactAcceptsNestedBridgeManifestFiles(t *testing.T) {
	build := &model.CustomBuild{
		IdModel:          model.IdModel{Id: 150},
		Platform:         "bridge",
		AppName:          "rustdesk-bridge",
		Version:          "1.2.3",
		BuildRef:         strings.Repeat("a", 40),
		GithubRef:        strings.Repeat("b", 40),
		WorkflowSelector: "bridge",
	}
	names, err := service.ExpectedProducerOutputFilenames(build.Platform, build.AppName, build.Version)
	if err != nil {
		t.Fatal(err)
	}
	contents := make(map[string]string, len(names))
	manifestFiles := make([]service.ProducerManifestFile, 0, len(names))
	for _, name := range names {
		contents[name] = name
		digest := sha256.Sum256([]byte(name))
		manifestFiles = append(manifestFiles, service.ProducerManifestFile{
			Name: name, Size: int64(len(name)), SHA256: hex.EncodeToString(digest[:]),
		})
	}
	manifestBytes, err := json.Marshal(service.ProducerManifest{
		Schema:               service.ProducerManifestSchema,
		ManifestSchema:       service.ProducerManifestSchema,
		SchemaVersion:        service.ProducerManifestVersion,
		Platform:             build.Platform,
		AppName:              build.AppName,
		OutputFilenames:      names,
		SourceSHA:            build.BuildRef,
		WorkflowSHA:          build.GithubRef,
		WorkflowRef:          build.WorkflowSelector,
		Version:              build.Version,
		SourceTreeSHA:        strings.Repeat("c", 40),
		Submodules:           []service.ProducerManifestSubmodule{},
		DigestScope:          service.ProducerManifestDigestScope,
		VerificationScope:    service.ProducerManifestVerificationScope,
		VerificationResult:   service.ProducerManifestVerificationResult,
		PublicationTimestamp: 1700000000,
		HandoffContract:      service.ProducerManifestHandoffContract,
		Files:                manifestFiles,
		PrivateFilenames:     []string{"custom_.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	archive := makeArtifactZip(t, map[string]string{
		"manifest.txt": string(manifestBytes),
		names[0]:       contents[names[0]],
		names[1]:       contents[names[1]],
		names[2]:       contents[names[2]],
		names[3]:       contents[names[3]],
		names[4]:       contents[names[4]],
		names[5]:       contents[names[5]],
		"custom_.txt":  "private settings",
	})
	archiveReader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer archiveReader.Close()
	staging := t.TempDir()
	if _, parsed, err := extractValidatedArtifact(archiveReader, staging, build); err != nil {
		t.Fatalf("extractValidatedArtifact() bridge error = %v", err)
	} else if parsed.Platform != "bridge" {
		t.Fatalf("parsed bridge manifest platform = %q", parsed.Platform)
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(name))); err != nil {
			t.Fatalf("nested bridge output %q was not extracted: %v", name, err)
		}
	}
	if contents, err := os.ReadFile(filepath.Join(staging, "custom_.txt")); err != nil || string(contents) != "private settings" {
		t.Fatalf("private custom_.txt = %q, err=%v; want extracted private file", contents, err)
	}
}

func TestExtractValidatedArtifactRejectsTraversalOutsideStaging(t *testing.T) {
	archive := makeArtifactZip(t, map[string]string{
		"../escape.txt": "must not be written",
	})
	archiveReader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer archiveReader.Close()

	staging := t.TempDir()
	build := &model.CustomBuild{
		Platform: "bridge",
		AppName:  "rustdesk-bridge",
	}
	if _, _, err := extractValidatedArtifact(archiveReader, staging, build); err == nil {
		t.Fatal("extractValidatedArtifact() error = nil, want traversal rejection")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(staging), "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("traversal destination stat = %v, want absent", err)
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		t.Fatalf("read staging directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging entries = %v, want no extracted files", entries)
	}
}

func TestPublishDownloadedArtifactEnforcesDecompressionSafetyLimits(t *testing.T) {
	previousEntries := artifactMaxZipEntries
	previousFileBytes := artifactMaxFileBytes
	previousAggregate := artifactMaxAggregateBytes
	previousRatio := artifactMaxCompressionRatio
	t.Cleanup(func() {
		artifactMaxZipEntries = previousEntries
		artifactMaxFileBytes = previousFileBytes
		artifactMaxAggregateBytes = previousAggregate
		artifactMaxCompressionRatio = previousRatio
	})

	cases := []struct {
		name      string
		files     map[string]string
		entries   int64
		fileBytes uint64
		total     uint64
		ratio     uint64
	}{
		{name: "entry limit", files: map[string]string{"rustqs.exe": "one", "custom_.txt": "two"}, entries: 1, fileBytes: 64, total: 128, ratio: 1000},
		{name: "per-file limit", files: map[string]string{"rustqs.exe": "12345"}, entries: 8, fileBytes: 4, total: 128, ratio: 1000},
		{name: "aggregate limit", files: map[string]string{"rustqs.exe": "1234", "custom_.txt": "5678"}, entries: 8, fileBytes: 64, total: 7, ratio: 1000},
		{name: "compression ratio", files: map[string]string{"rustqs.exe": strings.Repeat("x", 4096)}, entries: 8, fileBytes: 8192, total: 8192, ratio: 1},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTestOutputRoot(t)
			artifactMaxZipEntries = tc.entries
			artifactMaxFileBytes = tc.fileBytes
			artifactMaxAggregateBytes = tc.total
			artifactMaxCompressionRatio = tc.ratio
			build := &model.CustomBuild{IdModel: model.IdModel{Id: uint(120 + index)}, Platform: "windows", AppName: "rustqs"}
			archive := makeArtifactZip(t, tc.files)
			if _, err := publishDownloadedArtifact(build, archive); err == nil {
				t.Fatal("publishDownloadedArtifact() error = nil, want decompression safety rejection")
			}
			if _, err := os.Stat(customBuildOutputDir(build.Id)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("final output stat = %v, want no output", err)
			}
		})
	}
}

func TestPublishDownloadedArtifactRejectsUnprovenExistingOutputAndPreservesSiblingStaging(t *testing.T) {
	root := withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 130}, Platform: "windows", AppName: "rustqs"}
	parent := filepath.Dir(customBuildOutputDir(build.Id))
	stale := filepath.Join(parent, fmt.Sprintf(".%d-artifact-stale", build.Id))
	if err := os.MkdirAll(stale, 0755); err != nil {
		t.Fatalf("create stale staging: %v", err)
	}
	first := makeArtifactZip(t, map[string]string{"rustqs.exe": "old"})
	if _, err := publishDownloadedArtifact(build, first); err != nil {
		t.Fatalf("initial publish: %v", err)
	}
	second := makeArtifactZip(t, map[string]string{"rustqs.exe": "new"})
	if _, err := publishDownloadedArtifact(build, second); err == nil {
		t.Fatal("duplicate publish succeeded without stored publication proof")
	}
	contents, err := os.ReadFile(filepath.Join(customBuildOutputDir(build.Id), "rustqs.exe"))
	if err != nil || string(contents) != "old" {
		t.Fatalf("reused output = %q, err=%v; existing output must not be replaced", contents, err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("sibling staging stat = %v, want preserved", err)
	}
	_ = root
}

func TestPublishDownloadedArtifactRejectsUnvalidatedLinuxAndroidCapability(t *testing.T) {
	for _, platform := range []string{"linux", "android"} {
		t.Run(platform, func(t *testing.T) {
			root := withTestOutputRoot(t)
			build := &model.CustomBuild{IdModel: model.IdModel{Id: 100}, Platform: platform, AppName: "rustqs"}
			archive := makeArtifactZip(t, map[string]string{"nested/binary": "binary", "custom_.txt": "settings"})
			if _, err := publishDownloadedArtifact(build, archive); err == nil || !strings.Contains(err.Error(), "production capability") {
				t.Fatalf("publishDownloadedArtifact() error = %v, want explicit capability-unavailable error", err)
			}
			outDir := filepath.Join(root, "output", "100")
			if _, err := os.Stat(outDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unvalidated platform output stat = %v, want absent", err)
			}
		})
	}
}

func TestDownloadByKeyBuildsArchiveBeforeSendingSuccessHeaders(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousLogger := global.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:                model.CustomBuildStatusDone,
		Platform:              "windows",
		AppName:               "rustqs",
		DownloadKey:           "download-key",
		Version:               "1.2.3",
		BuildRef:              strings.Repeat("a", 40),
		SourceTag:             "1.2.3",
		AssetsRelease:         "offline-assets-1.2.3",
		AssetsReleaseID:       12,
		GithubRunId:           901,
		GithubProvider:        "github",
		GithubRepo:            "owner/repo",
		GithubWorkflow:        "workflow.yml",
		GithubRef:             strings.Repeat("a", 40),
		GithubArtifactName:    "artifact",
		GithubArtifactID:      42,
		PublicationRecordedAt: 1,
		GithubRunUrl:          "https://api.github.com/repos/owner/repo/actions/runs/901",
		GithubHtmlUrl:         "https://github.com/owner/repo/actions/runs/901",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("binary"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	markPublishedDigest(t, db, build)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/download/download-key", nil)
	c.Params = gin.Params{{Key: "key", Value: "download-key"}}
	(&CustomBuild{}).DownloadByKey(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("DownloadByKey() status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Length"); got != fmt.Sprint(recorder.Body.Len()) {
		t.Fatalf("DownloadByKey() Content-Length = %q, want full response length %d", got, recorder.Body.Len())
	}
	archiveHash := sha256.Sum256(recorder.Body.Bytes())
	if got := recorder.Header().Get("X-DeskForge-Archive-SHA256"); got != hex.EncodeToString(archiveHash[:]) {
		t.Fatalf("archive SHA header = %q, want hash of served redacted bytes %x", got, archiveHash)
	}
	if got := recorder.Header().Get("X-DeskForge-Archive-SHA256-Scope"); got != downloadArchiveDigestScope {
		t.Fatalf("archive SHA scope = %q, want %q", got, downloadArchiveDigestScope)
	}
	reader, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	if err != nil {
		t.Fatalf("downloaded response is not a ZIP: %v", err)
	}
	if len(reader.File) != 1 || reader.File[0].Name != "rustqs.exe" {
		t.Fatalf("downloaded ZIP entries = %#v, want rustqs.exe", reader.File)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("mutated"), 0600); err != nil {
		t.Fatalf("mutate published output: %v", err)
	}
	mutatedRecorder := httptest.NewRecorder()
	mutatedContext, _ := gin.CreateTestContext(mutatedRecorder)
	mutatedContext.Request = httptest.NewRequest(http.MethodGet, "/download/download-key", nil)
	mutatedContext.Params = gin.Params{{Key: "key", Value: "download-key"}}
	(&CustomBuild{}).DownloadByKey(mutatedContext)
	if mutatedRecorder.Code != http.StatusConflict {
		t.Fatalf("mutated DownloadByKey() status = %d, want 409; body=%s", mutatedRecorder.Code, mutatedRecorder.Body.String())
	}
	_ = root
}

func TestDownloadByKeyRejectsRangeBeforeServingDigestHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/download/any-key", nil)
	c.Request.Header.Set("Range", "bytes=0-15")
	c.Params = gin.Params{{Key: "key", Value: "any-key"}}

	(&CustomBuild{}).DownloadByKey(c)
	if recorder.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("DownloadByKey() range status = %d, want 416; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/zip") {
		t.Fatalf("range rejection committed ZIP content type: %q", recorder.Header().Get("Content-Type"))
	}
	for _, header := range []string{"Content-Length", "X-DeskForge-Archive-SHA256", "X-DeskForge-Archive-SHA256-Scope"} {
		if got := recorder.Header().Get(header); got != "" {
			t.Fatalf("range rejection set %s = %q", header, got)
		}
	}
}

func TestDetailByKeyRedactsCapabilityKey(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousLogger := global.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:                model.CustomBuildStatusDone,
		Platform:              "windows",
		AppName:               "rustqs",
		Name:                  "public build",
		DownloadKey:           "capability-secret",
		DownloadKeyExpiresAt:  time.Now().Add(time.Hour).Unix(),
		Version:               "1.2.3",
		CustomJson:            `{"enable_audio":true,"permanent_password":"private-build-secret"}`,
		BuildRef:              strings.Repeat("a", 40),
		SourceTag:             "1.2.3",
		AssetsRelease:         "offline-assets-1.2.3",
		AssetsReleaseID:       12,
		GithubRunId:           901,
		GithubProvider:        "github",
		GithubRepo:            "owner/repo",
		GithubWorkflow:        "workflow.yml",
		GithubRef:             strings.Repeat("a", 40),
		GithubArtifactName:    "artifact",
		GithubArtifactID:      42,
		PublicationRecordedAt: 1,
		GithubRunUrl:          "https://api.github.com/repos/owner/repo/actions/runs/901",
		GithubHtmlUrl:         "https://github.com/owner/repo/actions/runs/901",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("binary"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	markPublishedDigest(t, db, build)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/detail/capability-secret", nil)
	c.Params = gin.Params{{Key: "key", Value: "capability-secret"}}
	(&CustomBuild{}).DetailByKey(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("DetailByKey() status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{
		"capability-secret", "private-build-secret", "private build log", `"download_key"`, `"user_id"`,
		`"github_run_id"`, `"github_provider"`, `"github_repo"`, `"github_workflow"`,
		`"github_ref"`, `"github_artifact_name"`, `"github_artifact_id"`, `"github_run_url"`,
		`"github_html_url"`, `"github_source_sha"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("DetailByKey() exposed %q: %s", forbidden, body)
		}
	}
	for _, expected := range []string{`"name":"public build"`, `"version":"1.2.3"`, `"download_key_expires_at"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("DetailByKey() omitted %q: %s", expected, body)
		}
	}
	_ = root
}

func TestBuildDownloadArchiveRedactsOrOmitsCustomTxt(t *testing.T) {
	cases := []struct {
		name          string
		customPayload string
		wantCustom    bool
	}{
		{
			name: "safe native settings",
			customPayload: base64.StdEncoding.EncodeToString([]byte(
				`{"conn-type":"outgoing","password":"password-secret","default-settings":{"allow-hide-cm":"Y","verification-method":"use-permanent-password","enable-audio":"Y"}}`,
			)),
			wantCustom: true,
		},
		{
			name:          "malformed payload omitted",
			customPayload: base64.StdEncoding.EncodeToString([]byte("not-json")),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			withTestOutputRoot(t)
			build := &model.CustomBuild{IdModel: model.IdModel{Id: 200}, Platform: "windows", AppName: "rustqs"}
			dir := customBuildOutputDir(build.Id)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatalf("create output: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "rustqs.exe"), []byte("binary"), 0600); err != nil {
				t.Fatalf("write executable: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "custom_.txt"), []byte(test.customPayload), 0600); err != nil {
				t.Fatalf("write custom_.txt: %v", err)
			}
			archivePath, _, err := buildDownloadArchive(context.Background(), build)
			if err != nil {
				t.Fatalf("buildDownloadArchive() error = %v", err)
			}
			defer cleanupArtifactArchive(build.Id, archivePath)
			archive, err := zip.OpenReader(archivePath)
			if err != nil {
				t.Fatalf("open public archive: %v", err)
			}
			defer archive.Close()
			entries := make(map[string]string, len(archive.File))
			for _, entry := range archive.File {
				reader, readErr := entry.Open()
				if readErr != nil {
					t.Fatalf("open archive entry %q: %v", entry.Name, readErr)
				}
				contents, readErr := io.ReadAll(reader)
				closeErr := reader.Close()
				if readErr != nil || closeErr != nil {
					t.Fatalf("read archive entry %q: read=%v close=%v", entry.Name, readErr, closeErr)
				}
				entries[entry.Name] = string(contents)
			}
			if entries["rustqs.exe"] != "binary" {
				t.Fatalf("public executable = %q, want binary", entries["rustqs.exe"])
			}
			publicCustom, exists := entries["custom_.txt"]
			if exists != test.wantCustom {
				t.Fatalf("public custom_.txt presence = %v, want %v; entries=%v", exists, test.wantCustom, entries)
			}
			if exists {
				decoded, decodeErr := base64.StdEncoding.DecodeString(publicCustom)
				if decodeErr != nil {
					t.Fatalf("public custom_.txt is not base64: %v", decodeErr)
				}
				var public map[string]any
				if err := json.Unmarshal(decoded, &public); err != nil {
					t.Fatalf("public custom_.txt is not JSON: %v", err)
				}
				if public["conn-type"] != "outgoing" || strings.Contains(string(decoded), "password-secret") || strings.Contains(string(decoded), "allow-hide-cm") {
					t.Fatalf("public custom_.txt leaked private settings: %s", decoded)
				}
			}
		})
	}
}

func TestBuildDownloadArchiveIsDeterministicWithPublicCustomTxt(t *testing.T) {
	withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 201}, Platform: "windows", AppName: "rustqs"}
	dir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("create output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rustqs.exe"), []byte("binary"), 0600); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	raw := base64.StdEncoding.EncodeToString([]byte(`{"conn-type":"incoming","default-settings":{"theme":"dark","enable-audio":"Y"}}`))
	if err := os.WriteFile(filepath.Join(dir, "custom_.txt"), []byte(raw), 0600); err != nil {
		t.Fatalf("write custom_.txt: %v", err)
	}
	firstPath, _, err := buildDownloadArchive(context.Background(), build)
	if err != nil {
		t.Fatalf("first buildDownloadArchive() error = %v", err)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first archive: %v", err)
	}
	cleanupArtifactArchive(build.Id, firstPath)
	secondPath, _, err := buildDownloadArchive(context.Background(), build)
	if err != nil {
		t.Fatalf("second buildDownloadArchive() error = %v", err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second archive: %v", err)
	}
	cleanupArtifactArchive(build.Id, secondPath)
	if !bytes.Equal(first, second) {
		t.Fatalf("repeated public archives differ: first sha=%x second sha=%x", sha256.Sum256(first), sha256.Sum256(second))
	}
}

func TestDownloadByKeyRejectsDoneLegacyBuildBeforePackaging(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousLogger := global.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:      model.CustomBuildStatusDone,
		AppName:     "rustqs",
		DownloadKey: "legacy-download-key",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create legacy build: %v", err)
	}
	outDir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("legacy output"), 0600); err != nil {
		t.Fatalf("write legacy output: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/download/legacy-download-key", nil)
	c.Params = gin.Params{{Key: "key", Value: "legacy-download-key"}}
	(&CustomBuild{}).DownloadByKey(c)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("DownloadByKey() status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); strings.HasPrefix(got, "application/zip") {
		t.Fatalf("legacy DownloadByKey() committed ZIP headers: %q", got)
	}
	_ = root
}

func TestDetailByKeyRejectsUnmarkedDoneBuild(t *testing.T) {
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousLogger := global.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:             model.CustomBuildStatusDone,
		DownloadKey:        "detail-unmarked-key",
		Version:            "1.2.3",
		BuildRef:           strings.Repeat("a", 40),
		SourceTag:          "1.2.3",
		AssetsRelease:      "offline-assets-1.2.3",
		AssetsReleaseID:    12,
		GithubRunId:        901,
		GithubProvider:     "github",
		GithubRepo:         "owner/repo",
		GithubWorkflow:     "workflow.yml",
		GithubRef:          strings.Repeat("a", 40),
		GithubArtifactName: "artifact",
		GithubArtifactID:   42,
		GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/901",
		GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/901",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/detail/detail-unmarked-key", nil)
	c.Params = gin.Params{{Key: "key", Value: "detail-unmarked-key"}}
	(&CustomBuild{}).DetailByKey(c)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("DetailByKey() status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDetailByKeyRejectsDoneBuildAfterPublishedOutputMutation(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousLogger := global.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:                model.CustomBuildStatusDone,
		AppName:               "rustqs",
		DownloadKey:           "detail-mutated-key",
		Version:               "1.2.3",
		BuildRef:              strings.Repeat("a", 40),
		SourceTag:             "1.2.3",
		AssetsRelease:         "offline-assets-1.2.3",
		AssetsReleaseID:       12,
		GithubRunId:           902,
		GithubProvider:        "github",
		GithubRepo:            "owner/repo",
		GithubWorkflow:        "workflow.yml",
		GithubRef:             strings.Repeat("a", 40),
		GithubArtifactName:    "artifact",
		GithubArtifactID:      42,
		PublicationRecordedAt: 1,
		GithubRunUrl:          "https://api.github.com/repos/owner/repo/actions/runs/902",
		GithubHtmlUrl:         "https://github.com/owner/repo/actions/runs/902",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	outputPath := filepath.Join(outDir, "rustqs.exe")
	if err := os.WriteFile(outputPath, []byte("published"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	markPublishedDigest(t, db, build)
	if err := os.WriteFile(outputPath, []byte("mutated"), 0600); err != nil {
		t.Fatalf("mutate output: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/detail/detail-mutated-key", nil)
	c.Params = gin.Params{{Key: "key", Value: "detail-mutated-key"}}
	(&CustomBuild{}).DetailByKey(c)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("DetailByKey() status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
	}
	_ = root
}

func TestDownloadByKeyReturnsServerErrorBeforeHeadersWhenPackagingFails(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousLogger := global.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:                model.CustomBuildStatusDone,
		Platform:              "windows",
		AppName:               "rustqs",
		DownloadKey:           "broken-key",
		Version:               "1.2.3",
		BuildRef:              strings.Repeat("a", 40),
		SourceTag:             "1.2.3",
		AssetsRelease:         "offline-assets-1.2.3",
		AssetsReleaseID:       12,
		GithubRunId:           901,
		GithubProvider:        "github",
		GithubRepo:            "owner/repo",
		GithubWorkflow:        "workflow.yml",
		GithubRef:             strings.Repeat("a", 40),
		GithubArtifactName:    "artifact",
		GithubArtifactID:      42,
		PublicationRecordedAt: 1,
		GithubRunUrl:          "https://api.github.com/repos/owner/repo/actions/runs/901",
		GithubHtmlUrl:         "https://github.com/owner/repo/actions/runs/901",
	}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	outDir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("binary"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	markPublishedDigest(t, db, build)
	if err := os.Symlink(filepath.Join(outDir, "missing"), filepath.Join(outDir, "broken.bin")); err != nil {
		t.Fatalf("create broken source symlink: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/download/broken-key", nil)
	c.Params = gin.Params{{Key: "key", Value: "broken-key"}}
	(&CustomBuild{}).DownloadByKey(c)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("DownloadByKey() status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); strings.HasPrefix(got, "application/zip") {
		t.Fatalf("packaging failure committed ZIP headers: %q", got)
	}
	_ = root
}

func TestArtifactCleanupRetryExhaustionReleasesTempProtection(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "deskforge-artifact-abcdef.zip")
	if err := os.WriteFile(archivePath, []byte("stale"), 0600); err != nil {
		t.Fatalf("create archive: %v", err)
	}
	service.ProtectGithubArtifactTemp(archivePath)
	previousRemove := removeArtifactArchive
	previousDelay := artifactCleanupRetryDelay
	removeArtifactArchive = func(string) error { return errors.New("cleanup unavailable") }
	artifactCleanupRetryDelay = 0
	t.Cleanup(func() {
		removeArtifactArchive = previousRemove
		artifactCleanupRetryDelay = previousDelay
		service.ReleaseGithubArtifactTemp(archivePath)
	})

	cleanupArtifactArchive(7, archivePath)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := service.SweepGithubArtifactTemps(root, time.Now().Add(2*time.Hour), time.Hour); err == nil {
			if _, statErr := os.Stat(archivePath); errors.Is(statErr, os.ErrNotExist) {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("cleanup retry exhaustion left provider temp protected from TTL sweep")
}

func TestBuildDownloadArchiveEnforcesOutputFileAndContextBounds(t *testing.T) {
	previousMaxBytes, previousMaxFiles := downloadArchiveMaxOutputBytes, downloadArchiveMaxFiles
	previousMaxFileBytes, previousMaxSourceBytes := downloadArchiveMaxFileBytes, downloadArchiveMaxSourceBytes
	t.Cleanup(func() {
		downloadArchiveMaxOutputBytes = previousMaxBytes
		downloadArchiveMaxFiles = previousMaxFiles
		downloadArchiveMaxFileBytes = previousMaxFileBytes
		downloadArchiveMaxSourceBytes = previousMaxSourceBytes
	})
	withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 7}, Platform: "linux", AppName: "rustqs"}
	dir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), []byte("aa"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), []byte("bb"), 0600); err != nil {
		t.Fatal(err)
	}

	downloadArchiveMaxFiles = 1
	if _, _, err := buildDownloadArchive(context.Background(), build); err == nil || !strings.Contains(err.Error(), "too many files") {
		t.Fatalf("file-count bound error = %v, want too-many-files rejection", err)
	}
	downloadArchiveMaxFiles = 4096
	downloadArchiveMaxOutputBytes = 16
	if _, _, err := buildDownloadArchive(context.Background(), build); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("archive-size bound error = %v, want output-limit rejection", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := buildDownloadArchive(ctx, build); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled archive error = %v, want context cancellation", err)
	}

	downloadArchiveMaxOutputBytes = 1 << 20
	downloadArchiveMaxFileBytes = 1
	if _, _, err := buildDownloadArchive(context.Background(), build); err == nil || !strings.Contains(err.Error(), "uncompressed limit") {
		t.Fatalf("per-file source bound error = %v, want uncompressed-limit rejection", err)
	}
	downloadArchiveMaxFileBytes = 1 << 20
	downloadArchiveMaxSourceBytes = 1
	if _, _, err := buildDownloadArchive(context.Background(), build); err == nil || !strings.Contains(err.Error(), "uncompressed limit") {
		t.Fatalf("aggregate source bound error = %v, want uncompressed-limit rejection", err)
	}
}

func TestBuildDownloadArchiveRevalidatesMissingOrMutatedOutput(t *testing.T) {
	withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 8}, Platform: "linux"}
	dir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary"), []byte("safe"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "binary")); err != nil {
		t.Fatalf("remove output: %v", err)
	}
	if _, _, err := buildDownloadArchive(context.Background(), build); err == nil {
		t.Fatal("buildDownloadArchive() error = nil for missing output")
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "binary")); err != nil {
		t.Fatalf("create mutated output symlink: %v", err)
	}
	if _, _, err := buildDownloadArchive(context.Background(), build); err == nil {
		t.Fatal("buildDownloadArchive() error = nil for unsafe mutated output")
	}
}

func TestBuildDownloadArchiveUsesOneBoundedSnapshotAndRejectsMutation(t *testing.T) {
	withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 9}, Platform: "linux", AppName: "rustqs"}
	dir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("create output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.bin"), []byte("before"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	previousHook := downloadArchiveSnapshotHook
	downloadArchiveSnapshotHook = func() {
		if err := os.WriteFile(filepath.Join(dir, "artifact.bin"), []byte("after"), 0600); err != nil {
			t.Fatalf("mutate output after snapshot: %v", err)
		}
	}
	t.Cleanup(func() { downloadArchiveSnapshotHook = previousHook })
	if _, _, err := buildDownloadArchive(context.Background(), build); err == nil || !strings.Contains(err.Error(), "changed during archive packaging") {
		t.Fatalf("buildDownloadArchive() error = %v, want canonical digest mutation rejection", err)
	}
}

func TestCollectDownloadSnapshotUsesRestrictedServiceOwnedLifecycle(t *testing.T) {
	root := withTestOutputRoot(t)
	buildID := uint(11)
	dir := filepath.Join(root, "output", fmt.Sprintf("%d", buildID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("create output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.bin"), []byte("snapshot"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	snapshotDir, files, err := collectDownloadSnapshot(context.Background(), dir, buildID)
	if err != nil {
		t.Fatalf("collectDownloadSnapshot() error = %v", err)
	}
	defer cleanupDownloadSnapshot(snapshotDir)
	if filepath.Dir(snapshotDir) != filepath.Join(root, "output") {
		t.Fatalf("snapshot directory = %q, want service-owned output parent", snapshotDir)
	}
	info, err := os.Stat(snapshotDir)
	if err != nil {
		t.Fatalf("stat snapshot directory: %v", err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("snapshot directory permissions = %o, want 700", info.Mode().Perm())
	}
	if len(files) != 1 {
		t.Fatalf("snapshot files = %d, want 1", len(files))
	}
	fileInfo, err := os.Stat(files[0].path)
	if err != nil {
		t.Fatalf("stat snapshot file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0600 {
		t.Fatalf("snapshot file permissions = %o, want 600", fileInfo.Mode().Perm())
	}
}

func TestBuildDownloadArchiveRemovesInterruptedSnapshot(t *testing.T) {
	root := withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 12}, Platform: "linux", AppName: "rustqs"}
	dir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("create output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "artifact.bin"), []byte("before"), 0600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	previousHook := downloadArchiveSnapshotHook
	downloadArchiveSnapshotHook = cancel
	t.Cleanup(func() { downloadArchiveSnapshotHook = previousHook })
	if _, _, err := buildDownloadArchive(ctx, build); !errors.Is(err, context.Canceled) {
		t.Fatalf("buildDownloadArchive() error = %v, want interrupted export cancellation", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "output", ".12-snapshot-*"))
	if err != nil {
		t.Fatalf("glob interrupted snapshots: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("interrupted snapshots remain after cleanup: %v", matches)
	}
}

func TestBuildDownloadArchiveIsDeterministicForUnchangedOutput(t *testing.T) {
	withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 10}, Platform: "linux", AppName: "rustqs"}
	dir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("create output: %v", err)
	}
	for name, contents := range map[string]string{"z.bin": "last", "a.bin": "first"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	firstPath, _, err := buildDownloadArchive(context.Background(), build)
	if err != nil {
		t.Fatalf("first buildDownloadArchive() error = %v", err)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first archive: %v", err)
	}
	if err := os.Remove(firstPath); err != nil {
		t.Fatalf("remove first archive: %v", err)
	}
	secondPath, _, err := buildDownloadArchive(context.Background(), build)
	if err != nil {
		t.Fatalf("second buildDownloadArchive() error = %v", err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second archive: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("unchanged output produced different ZIP bytes")
	}
	if err := os.Remove(secondPath); err != nil {
		t.Fatalf("remove second archive: %v", err)
	}
}

func TestCopyDownloadSourceRejectsNoProgress(t *testing.T) {
	noProgress := readerReturningNoProgress{}
	if _, err := copyDownloadSource(context.Background(), io.Discard, noProgress); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("copyDownloadSource() error = %v, want io.ErrNoProgress", err)
	}
}

type readerReturningNoProgress struct{}

func (readerReturningNoProgress) Read([]byte) (int, error) { return 0, nil }

func TestPollPublishesOnlyValidatedArtifactBeforeDone(t *testing.T) {
	cases := []struct {
		name       string
		zipPayload []byte
		wantStatus string
		wantOutput bool
	}{
		{
			name:       "successful output",
			zipPayload: mustWindowsProducerArtifactZip(t, "exe"),
			wantStatus: model.CustomBuildStatusDone,
			wantOutput: true,
		},
		{
			name:       "invalid ZIP stays non-done",
			zipPayload: []byte("not a ZIP"),
			wantStatus: model.CustomBuildStatusFailed,
			wantOutput: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := withTestOutputRoot(t)
			db, sqlDB := newAdminProvenanceDB(t)
			previousDB, previousService := global.DB, service.AllService
			previousServiceDB, previousLogger := service.DB, global.Logger
			previousTransport := http.DefaultTransport
			global.DB = db
			global.Logger = logrus.New()
			service.DB = db
			service.AllService = &service.Service{
				CustomBuildService:       &service.CustomBuildService{},
				GithubBuildConfigService: &service.GithubBuildConfigService{},
			}
			http.DefaultTransport = adminGithubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.URL.Path == "/repos/owner/repo/actions/runs/777":
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))}, nil
				case req.URL.Path == "/repos/owner/repo/actions/runs/777/artifacts":
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"artifacts":[{"id":42,"name":"artifact"}]}`))}, nil
				case req.URL.Path == "/repos/owner/repo/actions/artifacts/42/zip":
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(tc.zipPayload))}, nil
				default:
					return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"unexpected endpoint"}`))}, nil
				}
			})
			t.Cleanup(func() {
				http.DefaultTransport = previousTransport
				global.DB = previousDB
				global.Logger = previousLogger
				service.DB = previousServiceDB
				service.AllService = previousService
				_ = sqlDB.Close()
			})

			build := &model.CustomBuild{
				Status:             model.CustomBuildStatusBuilding,
				Platform:           "windows",
				AppName:            "rustqs",
				Version:            "1.2.3",
				BuildRef:           strings.Repeat("a", 40),
				SourceTag:          "1.2.3",
				AssetsRelease:      "offline-assets-1.2.3",
				AssetsReleaseID:    12,
				GithubRunId:        777,
				GithubProvider:     "github",
				GithubRepo:         "owner/repo",
				GithubWorkflow:     "workflow.yml",
				GithubRef:          strings.Repeat("a", 40),
				GithubArtifactName: "artifact",
				GithubArtifactID:   42,
				GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/777",
				GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/777",
			}
			prepareAdminBuildProvenance(build)
			if err := db.Create(build).Error; err != nil {
				t.Fatalf("create build: %v", err)
			}
			if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "token"}).Error; err != nil {
				t.Fatalf("create GitHub config: %v", err)
			}

			currentTime := time.Now()
			(&CustomBuild{}).pollAndDownloadWithClock(build.Id, build.GithubRunId, githubPollClock{
				now:  func() time.Time { return currentTime },
				wait: func(delay time.Duration) { currentTime = currentTime.Add(delay) },
			})

			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read build: %v", err)
			}
			if stored.Status != tc.wantStatus {
				t.Fatalf("stored status = %q, want %q", stored.Status, tc.wantStatus)
			}
			outDir := filepath.Join(root, "output", fmt.Sprintf("%d", build.Id))
			_, outputErr := os.Stat(outDir)
			if (outputErr == nil) != tc.wantOutput {
				t.Fatalf("final output presence = %v, err=%v, want %v", outputErr == nil, outputErr, tc.wantOutput)
			}
			if entries, err := os.ReadDir(filepath.Dir(outDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("read output parent: %v", err)
			} else if !tc.wantOutput && len(entries) != 0 {
				t.Fatalf("failed publication left output entries = %v", entries)
			}
		})
	}
}

func TestPollCleanupFailureDoesNotFailPublishedBuild(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousService := global.DB, service.AllService
	previousServiceDB, previousLogger := service.DB, global.Logger
	previousTransport := http.DefaultTransport
	previousRemove := removeArtifactArchive
	previousSchedule := scheduleArtifactCleanupRetry
	global.DB = db
	global.Logger = logrus.New()
	service.DB = db
	service.AllService = &service.Service{
		CustomBuildService:       &service.CustomBuildService{},
		GithubBuildConfigService: &service.GithubBuildConfigService{},
	}
	var cleanupPath string
	removeArtifactArchive = func(path string) error {
		cleanupPath = path
		return errors.New("simulated temporary cleanup failure")
	}
	scheduleArtifactCleanupRetry = func(_ uint, path string) {
		cleanupPath = path
	}
	http.DefaultTransport = adminGithubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/actions/runs/778":
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))}, nil
		case "/repos/owner/repo/actions/runs/778/artifacts":
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"artifacts":[{"id":42,"name":"artifact"}]}`))}, nil
		case "/repos/owner/repo/actions/artifacts/42/zip":
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(mustWindowsProducerArtifactZip(t, "exe")))}, nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"unexpected endpoint"}`))}, nil
		}
	})
	t.Cleanup(func() {
		if cleanupPath != "" {
			_ = os.Remove(cleanupPath)
		}
		removeArtifactArchive = previousRemove
		scheduleArtifactCleanupRetry = previousSchedule
		http.DefaultTransport = previousTransport
		global.DB = previousDB
		global.Logger = previousLogger
		service.DB = previousServiceDB
		service.AllService = previousService
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:             model.CustomBuildStatusBuilding,
		Platform:           "windows",
		AppName:            "rustqs",
		Version:            "1.2.3",
		BuildRef:           strings.Repeat("a", 40),
		SourceTag:          "1.2.3",
		AssetsRelease:      "offline-assets-1.2.3",
		AssetsReleaseID:    12,
		GithubRunId:        778,
		GithubProvider:     "github",
		GithubRepo:         "owner/repo",
		GithubWorkflow:     "workflow.yml",
		GithubRef:          strings.Repeat("a", 40),
		GithubArtifactName: "artifact",
		GithubArtifactID:   42,
		GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/778",
		GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/778",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "token"}).Error; err != nil {
		t.Fatalf("create GitHub config: %v", err)
	}

	currentTime := time.Now()
	(&CustomBuild{}).pollAndDownloadWithClock(build.Id, build.GithubRunId, githubPollClock{
		now:  func() time.Time { return currentTime },
		wait: func(delay time.Duration) { currentTime = currentTime.Add(delay) },
	})

	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusDone {
		t.Fatalf("cleanup failure changed status to %q, want done", stored.Status)
	}
	if cleanupPath == "" {
		t.Fatal("cleanup failure did not schedule a retry")
	}
	if _, err := os.Stat(filepath.Join(root, "output", fmt.Sprintf("%d", build.Id), "rustqs.exe")); err != nil {
		t.Fatalf("published output unavailable after cleanup failure: %v", err)
	}
}

func TestPollRedownloadsExactArtifactForUnprovenExistingOutput(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousService := global.DB, service.AllService
	previousServiceDB, previousLogger := service.DB, global.Logger
	previousTransport := http.DefaultTransport
	global.DB = db
	global.Logger = logrus.New()
	service.DB = db
	service.AllService = &service.Service{
		CustomBuildService:       &service.CustomBuildService{},
		GithubBuildConfigService: &service.GithubBuildConfigService{},
	}
	artifactPayload := mustWindowsProducerArtifactZip(t, "provider-output")
	artifactRequests := 0
	http.DefaultTransport = adminGithubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/repos/owner/repo/actions/runs/903":
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))}, nil
		case req.URL.Path == "/repos/owner/repo/actions/runs/903/artifacts":
			artifactRequests++
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"artifacts":[{"id":42,"name":"artifact"}]}`))}, nil
		case req.URL.Path == "/repos/owner/repo/actions/artifacts/42/zip":
			artifactRequests++
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(artifactPayload))}, nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"unexpected endpoint"}`))}, nil
		}
	})
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
		global.DB = previousDB
		global.Logger = previousLogger
		service.DB = previousServiceDB
		service.AllService = previousService
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:             model.CustomBuildStatusBuilding,
		Platform:           "windows",
		AppName:            "rustqs",
		Version:            "1.2.3",
		BuildRef:           strings.Repeat("a", 40),
		SourceTag:          "1.2.3",
		AssetsRelease:      "offline-assets-1.2.3",
		AssetsReleaseID:    12,
		GithubRunId:        903,
		GithubProvider:     "github",
		GithubRepo:         "owner/repo",
		GithubWorkflow:     "workflow.yml",
		GithubRef:          strings.Repeat("a", 40),
		GithubArtifactName: "artifact",
		GithubArtifactID:   42,
		GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/903",
		GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/903",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "token"}).Error; err != nil {
		t.Fatalf("create GitHub config: %v", err)
	}
	outDir := filepath.Join(root, "output", fmt.Sprintf("%d", build.Id))
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create unproven output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("stale-output"), 0600); err != nil {
		t.Fatalf("write unproven output: %v", err)
	}

	currentTime := time.Now()
	(&CustomBuild{}).pollAndDownloadWithClock(build.Id, build.GithubRunId, githubPollClock{
		now:  func() time.Time { return currentTime },
		wait: func(delay time.Duration) { currentTime = currentTime.Add(delay) },
	})

	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read recovered build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusDone || stored.PublicationRecordedAt <= 0 || stored.PublishedDigest == "" {
		t.Fatalf("unproven recovery state = %#v, want done with publication proof", stored)
	}
	contents, err := os.ReadFile(filepath.Join(outDir, "rustqs.exe"))
	if err != nil || string(contents) != "provider-output" {
		t.Fatalf("recovered output = %q, err=%v; exact provider artifact was not republished", contents, err)
	}
	if artifactRequests != 2 {
		t.Fatalf("provider artifact requests = %d, want exact list plus ZIP download", artifactRequests)
	}
}

func TestPollRecoversValidFinalOutputBeforeProviderRedownload(t *testing.T) {
	root := withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousService := global.DB, service.AllService
	previousServiceDB, previousLogger := service.DB, global.Logger
	previousTransport := http.DefaultTransport
	global.DB = db
	global.Logger = logrus.New()
	service.DB = db
	service.AllService = &service.Service{
		CustomBuildService:       &service.CustomBuildService{},
		GithubBuildConfigService: &service.GithubBuildConfigService{},
	}
	artifactRequests := 0
	http.DefaultTransport = adminGithubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/repos/owner/repo/actions/runs/779" {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))}, nil
		}
		artifactRequests++
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"artifact redownload must not happen"}`))}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
		global.DB = previousDB
		global.Logger = previousLogger
		service.DB = previousServiceDB
		service.AllService = previousService
		_ = sqlDB.Close()
	})

	build := &model.CustomBuild{
		Status:             model.CustomBuildStatusBuilding,
		Platform:           "windows",
		AppName:            "rustqs",
		Version:            "1.2.3",
		BuildRef:           strings.Repeat("a", 40),
		SourceTag:          "1.2.3",
		AssetsRelease:      "offline-assets-1.2.3",
		AssetsReleaseID:    12,
		GithubRunId:        779,
		GithubProvider:     "github",
		GithubRepo:         "owner/repo",
		GithubWorkflow:     "workflow.yml",
		GithubRef:          strings.Repeat("a", 40),
		GithubArtifactName: "artifact",
		GithubArtifactID:   42,
		GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/779",
		GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/779",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "token"}).Error; err != nil {
		t.Fatalf("create GitHub config: %v", err)
	}
	outDir := filepath.Join(root, "output", fmt.Sprintf("%d", build.Id))
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("create recovered output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("recovered"), 0600); err != nil {
		t.Fatalf("write recovered output: %v", err)
	}
	if err := db.Model(&model.CustomBuild{}).Where("id = ?", build.Id).Updates(map[string]any{
		"publication_recorded_at": 1,
	}).Error; err != nil {
		t.Fatalf("store recovered publication marker: %v", err)
	}
	markPublishedDigest(t, db, build)

	currentTime := time.Now()
	(&CustomBuild{}).pollAndDownloadWithClock(build.Id, build.GithubRunId, githubPollClock{
		now:  func() time.Time { return currentTime },
		wait: func(delay time.Duration) { currentTime = currentTime.Add(delay) },
	})

	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read recovered build: %v", err)
	}
	if stored.Status != model.CustomBuildStatusDone || stored.FileSize != int64(len("recovered")) {
		t.Fatalf("recovered build = %#v, want guarded done with existing size", stored)
	}
	if artifactRequests != 0 {
		t.Fatalf("provider artifact requests = %d, want no redownload", artifactRequests)
	}
	contents, err := os.ReadFile(filepath.Join(outDir, "rustqs.exe"))
	if err != nil || string(contents) != "recovered" {
		t.Fatalf("recovered output changed: %q, err=%v", contents, err)
	}
}

func mustArtifactZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, contents := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create ZIP entry %q: %v", name, err)
		}
		if _, err := w.Write([]byte(contents)); err != nil {
			t.Fatalf("write ZIP entry %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close ZIP: %v", err)
	}
	return buf.Bytes()
}

func mustWindowsProducerArtifactZip(t *testing.T, contents string) []byte {
	t.Helper()
	return mustArtifactZip(t, map[string]string{
		"rustqs.exe":   contents,
		"manifest.txt": producerManifestBytes(t, contents),
	})
}

func producerManifestBytes(t *testing.T, contents string) string {
	t.Helper()
	digest := sha256.Sum256([]byte(contents))
	manifest := service.ProducerManifest{
		Schema:               service.ProducerManifestSchema,
		ManifestSchema:       service.ProducerManifestSchema,
		SchemaVersion:        service.ProducerManifestVersion,
		Platform:             "windows",
		AppName:              "rustqs",
		OutputFilenames:      []string{"rustqs.exe"},
		SourceSHA:            strings.Repeat("a", 40),
		WorkflowSHA:          strings.Repeat("a", 40),
		WorkflowRef:          "rustqs/workflows",
		Version:              "1.2.3",
		SourceTreeSHA:        strings.Repeat("c", 40),
		Submodules:           []service.ProducerManifestSubmodule{},
		DigestScope:          service.ProducerManifestDigestScope,
		VerificationScope:    service.ProducerManifestVerificationScope,
		VerificationResult:   service.ProducerManifestVerificationResult,
		PublicationTimestamp: 1700000000,
		HandoffContract:      service.ProducerManifestHandoffContract,
		Files:                []service.ProducerManifestFile{{Name: "rustqs.exe", Size: int64(len(contents)), SHA256: hex.EncodeToString(digest[:])}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal producer manifest: %v", err)
	}
	return string(manifestBytes)
}

func activeWindowsBuild(id uint) *model.CustomBuild {
	return &model.CustomBuild{
		IdModel:          model.IdModel{Id: id},
		Platform:         "windows",
		AppName:          "rustqs",
		Version:          "1.2.3",
		BuildRef:         strings.Repeat("a", 40),
		WorkflowSelector: "rustqs/workflows",
		GithubRef:        strings.Repeat("a", 40),
		GithubSourceSha:  strings.Repeat("a", 40),
		GithubRunId:      1,
	}
}

func markPublishedDigest(t *testing.T, db *gorm.DB, build *model.CustomBuild) {
	t.Helper()
	digest, err := service.PublishedOutputDigest(build)
	if err != nil {
		t.Fatalf("compute published digest: %v", err)
	}
	build.PublishedDigest = digest
	if err := db.Model(&model.CustomBuild{}).Where("id = ?", build.Id).Update("published_digest", digest).Error; err != nil {
		t.Fatalf("store published digest: %v", err)
	}
}
