package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/middleware"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

func TestCustomBuildManifestRequiresAdminPrivilege(t *testing.T) {
	previousServices := service.AllService
	previousLogger, previousLocalizer := global.Logger, global.Localizer
	t.Cleanup(func() {
		service.AllService = previousServices
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
	})
	global.Localizer = testManifestLocalizer
	global.Logger = logrus.New()
	service.AllService = &service.Service{UserService: &service.UserService{}}

	for _, tc := range []struct {
		name string
		user *model.User
	}{
		{name: "missing user"},
		{name: "non-admin", user: &model.User{IsAdmin: boolPointer(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newManifestRouter(tc.user)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/admin/custom_build/manifest/1", nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("unauthorized HTTP status = %d, want legacy 200 envelope", recorder.Code)
			}
			var envelope struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("unauthorized response JSON: %v", err)
			}
			if envelope.Code != 403 {
				t.Fatalf("unauthorized response code = %d, want 403", envelope.Code)
			}
		})
	}
}

func TestCustomBuildManifestEndpointReturnsVerifiedRedactedManifest(t *testing.T) {
	withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousGlobalDB, previousServiceDB := global.DB, service.DB
	previousServices := service.AllService
	previousLogger, previousLocalizer := global.Logger, global.Localizer
	t.Cleanup(func() {
		global.DB = previousGlobalDB
		service.DB = previousServiceDB
		service.AllService = previousServices
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
		_ = sqlDB.Close()
	})
	global.DB = db
	service.DB = db
	global.Logger = logrus.New()
	global.Localizer = testManifestLocalizer
	service.AllService = &service.Service{
		UserService:        &service.UserService{},
		CustomBuildService: &service.CustomBuildService{},
	}
	build := newAdminCompleteManifestBuild(t)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create complete build: %v", err)
	}

	router := newManifestRouter(&model.User{IsAdmin: boolPointer(true)})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/custom_build/manifest/101", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("manifest HTTP status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Code int                          `json:"code"`
		Data service.BuildHandoffManifest `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("manifest response JSON: %v", err)
	}
	if envelope.Code != 0 {
		t.Fatalf("manifest response code = %d, body=%s", envelope.Code, recorder.Body.String())
	}
	if envelope.Data.BuildID != build.Id || envelope.Data.PublishedDigest != build.PublishedDigest || envelope.Data.ArtifactID != build.GithubArtifactID {
		t.Fatalf("manifest identity = %#v, want build=%d digest=%q artifact=%d", envelope.Data, build.Id, build.PublishedDigest, build.GithubArtifactID)
	}
	if len(envelope.Data.ReleaseAssets) != 4 || envelope.Data.ReleaseAssets[0].ProviderDigest == "" {
		t.Fatalf("manifest release assets = %#v, want exact provider digests", envelope.Data.ReleaseAssets)
	}
	for _, forbidden := range []string{"PAT-secret", "payload-secret", "server-key-secret", "/rdgen-data"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("manifest response contains forbidden value %q: %s", forbidden, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "\"name\":\"custom_.txt\"") {
		t.Fatalf("manifest response exposed secret-bearing output entry: %s", recorder.Body.String())
	}
	bodyDigest := sha256.Sum256(recorder.Body.Bytes())
	if got := recorder.Header().Get("X-DeskForge-Manifest-SHA256"); got != hex.EncodeToString(bodyDigest[:]) {
		t.Fatalf("manifest response SHA header = %q, want hash of actual response bytes %x", got, bodyDigest)
	}
	repeat := httptest.NewRecorder()
	router.ServeHTTP(repeat, httptest.NewRequest(http.MethodGet, "/api/admin/custom_build/manifest/101", nil))
	if repeat.Code != http.StatusOK || string(repeat.Body.Bytes()) != string(recorder.Body.Bytes()) {
		t.Fatalf("repeated manifest response changed: first=%s second=%s", recorder.Body.Bytes(), repeat.Body.Bytes())
	}
	if repeat.Header().Get("X-DeskForge-Manifest-SHA256") != recorder.Header().Get("X-DeskForge-Manifest-SHA256") {
		t.Fatalf("repeated manifest response checksum changed: first=%q second=%q", recorder.Header().Get("X-DeskForge-Manifest-SHA256"), repeat.Header().Get("X-DeskForge-Manifest-SHA256"))
	}
}

func TestCustomBuildManifestEndpointFailsClosedForPartialBuild(t *testing.T) {
	db, sqlDB := newAdminProvenanceDB(t)
	previousGlobalDB, previousServiceDB := global.DB, service.DB
	previousServices := service.AllService
	previousLogger, previousLocalizer := global.Logger, global.Localizer
	t.Cleanup(func() {
		global.DB = previousGlobalDB
		service.DB = previousServiceDB
		service.AllService = previousServices
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
		_ = sqlDB.Close()
	})
	global.DB = db
	service.DB = db
	global.Logger = logrus.New()
	global.Localizer = testManifestLocalizer
	service.AllService = &service.Service{
		UserService:        &service.UserService{},
		CustomBuildService: &service.CustomBuildService{},
	}
	legacy := &model.CustomBuild{IdModel: model.IdModel{Id: 102}, Status: model.CustomBuildStatusDone, Platform: "windows"}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("create legacy build: %v", err)
	}
	router := newManifestRouter(&model.User{IsAdmin: boolPointer(true)})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/custom_build/manifest/102", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("partial manifest HTTP status = %d, body=%s; want conflict", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "custom_json") || strings.Contains(recorder.Body.String(), "filesystem") {
		t.Fatalf("partial manifest response exposed implementation details: %s", recorder.Body.String())
	}
}

func newManifestRouter(user *model.User) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/admin")
	if user != nil {
		group.Use(func(c *gin.Context) {
			c.Set("curUser", user)
			c.Next()
		})
	}
	manifestGroup := group.Group("/custom_build").Use(middleware.AdminPrivilege())
	manifestGroup.GET("/manifest/:id", (&CustomBuild{}).Manifest)
	return router
}

func testManifestLocalizer(_ string) *i18n.Localizer {
	return i18n.NewLocalizer(i18n.NewBundle(language.English), "en")
}

func boolPointer(value bool) *bool {
	return &value
}

func newAdminCompleteManifestBuild(t *testing.T) *model.CustomBuild {
	t.Helper()
	build := &model.CustomBuild{
		IdModel:         model.IdModel{Id: 101},
		Status:          model.CustomBuildStatusDone,
		Platform:        "windows",
		AppName:         "rustqs",
		Version:         "1.2.3",
		BuildRef:        strings.Repeat("b", 40),
		SourceTag:       "1.2.3",
		AssetsRelease:   "offline-assets-1.2.3",
		AssetsReleaseID: 101,
		AssetsReleaseAssets: `[{
"id":101,"name":"windows-x64-release.zip","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"},{
"id":102,"name":"usbmmidd_v2.zip","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"},{
"id":103,"name":"rustdesk_printer_driver_v4-1.4.zip","digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333"},{
"id":104,"name":"printer_driver_adapter.zip","digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444"}]`,
		GithubProvider:        "github",
		GithubRepo:            "owner/repo",
		GithubWorkflow:        "windows-min-test.yml",
		WorkflowSelector:      "rustqs/min-test",
		GithubRef:             strings.Repeat("a", 40),
		GithubSourceSha:       strings.Repeat("a", 40),
		GithubArtifactName:    "rustdesk-min-test-windows",
		GithubArtifactID:      1001,
		GithubRunId:           1001,
		GithubRunUrl:          "https://api.github.com/repos/owner/repo/actions/runs/1001",
		GithubHtmlUrl:         "https://github.com/owner/repo/actions/runs/1001",
		PublicationRecordedAt: 1700000000,
		CustomJson:            `{"permanent_password":"PAT-secret"}`,
	}
	root := service.BuildOutputDir
	outputDir := root(build.Id)
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		t.Fatalf("create output directory: %v", err)
	}
	for name, contents := range map[string]string{
		"rustqs.exe":  "exe",
		"helper.dll":  "dll",
		"custom_.txt": "PAT-secret",
	} {
		if err := os.WriteFile(filepath.Join(outputDir, name), []byte(contents), 0600); err != nil {
			t.Fatalf("write output %q: %v", name, err)
		}
	}
	exeHash := sha256.Sum256([]byte("exe"))
	producerManifest := service.ProducerManifest{
		Schema:               service.ProducerManifestSchema,
		ManifestSchema:       service.ProducerManifestSchema,
		SchemaVersion:        service.ProducerManifestVersion,
		Platform:             build.Platform,
		AppName:              build.AppName,
		OutputFilenames:      []string{"rustqs.exe"},
		SourceSHA:            build.BuildRef,
		WorkflowSHA:          build.GithubRef,
		WorkflowRef:          build.WorkflowSelector,
		Version:              build.Version,
		SourceTreeSHA:        strings.Repeat("c", 40),
		Submodules:           []service.ProducerManifestSubmodule{},
		DigestScope:          service.ProducerManifestDigestScope,
		VerificationScope:    service.ProducerManifestVerificationScope,
		VerificationResult:   service.ProducerManifestVerificationResult,
		PublicationTimestamp: build.PublicationRecordedAt,
		HandoffContract:      service.ProducerManifestHandoffContract,
		Files:                []service.ProducerManifestFile{{Name: "rustqs.exe", Size: 3, SHA256: hex.EncodeToString(exeHash[:])}},
	}
	producerManifestJSON, err := producerManifest.StoredJSON()
	if err != nil {
		t.Fatalf("producer manifest: %v", err)
	}
	build.ProducerManifestJSON = producerManifestJSON
	digest, err := service.PublishedOutputDigest(build)
	if err != nil {
		t.Fatalf("PublishedOutputDigest() error = %v", err)
	}
	build.PublishedDigest = digest
	return build
}
