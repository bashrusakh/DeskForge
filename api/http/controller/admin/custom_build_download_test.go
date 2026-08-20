package admin

import (
	"archive/zip"
	"encoding/json"
	"fmt"
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
	"rustdesk-server/api/config"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/middleware"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

func TestCustomBuildDownloadByIDStreamsCompletedPublishedArtifact(t *testing.T) {
	build, db, cleanup := setupDownloadBuild(t, model.CustomBuildStatusDone)
	defer cleanup()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/custom_build/download/%d", build.Id), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(build.Id)}}
	(&CustomBuild{}).DownloadByID(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("DownloadByID() status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Length"); got != fmt.Sprint(recorder.Body.Len()) {
		t.Fatalf("Content-Length = %q, want %d", got, recorder.Body.Len())
	}
	if recorder.Header().Get("X-DeskForge-Archive-SHA256") == "" || recorder.Header().Get("X-DeskForge-Archive-SHA256-Scope") != downloadArchiveDigestScope {
		t.Fatalf("archive integrity headers = %#v, want digest and scope", recorder.Header())
	}
	archive, err := zip.NewReader(strings.NewReader(recorder.Body.String()), int64(recorder.Body.Len()))
	if err != nil {
		t.Fatalf("response is not a ZIP archive: %v", err)
	}
	if len(archive.File) != 1 || archive.File[0].Name != "rustqs.exe" {
		t.Fatalf("downloaded entries = %#v, want rustqs.exe", archive.File)
	}

	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("re-read downloaded build: %v", err)
	}
}

func TestCustomBuildDownloadByIDRejectsPartialAndInvalidBuilds(t *testing.T) {
	build, _, cleanup := setupDownloadBuild(t, model.CustomBuildStatusBuilding)
	defer cleanup()

	for _, tc := range []struct {
		name string
		id   string
		want int
	}{
		{name: "partial publication", id: fmt.Sprint(build.Id), want: http.StatusConflict},
		{name: "invalid id", id: "not-an-id", want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/download/"+tc.id, nil)
			c.Params = gin.Params{{Key: "id", Value: tc.id}}
			(&CustomBuild{}).DownloadByID(c)
			if recorder.Code != tc.want {
				t.Fatalf("DownloadByID() status = %d, want %d; body=%s", recorder.Code, tc.want, recorder.Body.String())
			}
			for _, forbidden := range []string{"capability-secret", `"download_key"`, "custom_json", "published_digest"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("unsafe download error exposed %q: %s", forbidden, recorder.Body.String())
				}
			}
			if recorder.Header().Get("X-DeskForge-Archive-SHA256") != "" {
				t.Fatalf("rejected download set archive digest header: %q", recorder.Header().Get("X-DeskForge-Archive-SHA256"))
			}
		})
	}
}

func TestPublicBuildRoutesRejectMissingStoredProducerManifest(t *testing.T) {
	build, db, cleanup := setupDownloadBuild(t, model.CustomBuildStatusDone)
	defer cleanup()
	if err := db.Model(&model.CustomBuild{}).Where("id = ?", build.Id).Update("producer_manifest_json", "").Error; err != nil {
		t.Fatalf("remove stored producer manifest: %v", err)
	}

	for _, tc := range []struct {
		name    string
		handler func(*gin.Context)
		path    string
	}{
		{name: "public detail", handler: (&CustomBuild{}).DetailByKey, path: "/detail/" + build.DownloadKey},
		{name: "public download", handler: (&CustomBuild{}).DownloadByKey, path: "/download/" + build.DownloadKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			c.Params = gin.Params{{Key: "key", Value: build.DownloadKey}}
			tc.handler(c)
			if recorder.Code != http.StatusConflict {
				t.Fatalf("%s status = %d, want %d; body=%s", tc.name, recorder.Code, http.StatusConflict, recorder.Body.String())
			}
			if got := recorder.Header().Get("X-DeskForge-Archive-SHA256"); got != "" {
				t.Fatalf("%s set archive digest header %q", tc.name, got)
			}
		})
	}
}

func TestCustomBuildDownloadByIDRejectsRangeBeforeArchiveHeaders(t *testing.T) {
	build, _, cleanup := setupDownloadBuild(t, model.CustomBuildStatusDone)
	defer cleanup()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/download/"+fmt.Sprint(build.Id), nil)
	c.Request.Header.Set("Range", "bytes=0-15")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(build.Id)}}
	(&CustomBuild{}).DownloadByID(c)

	if recorder.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("DownloadByID() range status = %d, want 416; body=%s", recorder.Code, recorder.Body.String())
	}
	for _, header := range []string{"Content-Length", "X-DeskForge-Archive-SHA256", "X-DeskForge-Archive-SHA256-Scope"} {
		if got := recorder.Header().Get(header); got != "" {
			t.Fatalf("range rejection set %s = %q", header, got)
		}
	}
}

func TestCustomBuildDownloadRouteRequiresBackendAuthAndAdminPrivilege(t *testing.T) {
	db, sqlDB := newAdminProvenanceDB(t)
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}); err != nil {
		t.Fatalf("migrate auth models: %v", err)
	}
	previousDB, previousServiceDB, previousServices, previousConfig := global.DB, service.DB, service.AllService, service.Config
	previousLogger, previousLocalizer := global.Logger, global.Localizer
	t.Cleanup(func() {
		global.DB = previousDB
		service.DB = previousServiceDB
		service.AllService = previousServices
		service.Config = previousConfig
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
		_ = sqlDB.Close()
	})
	global.DB = db
	service.DB = db
	service.Config = &config.Config{App: config.App{TokenExpire: time.Hour}}
	global.Logger = logrus.New()
	global.Localizer = testManifestLocalizer
	service.AllService = &service.Service{UserService: &service.UserService{}, CustomBuildService: &service.CustomBuildService{}}

	nonAdmin := &model.User{Username: "download-non-admin", IsAdmin: boolPointer(false), Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(nonAdmin).Error; err != nil {
		t.Fatalf("create non-admin: %v", err)
	}
	const nonAdminToken = "download-non-admin-token"
	if err := db.Create(&model.UserToken{UserId: nonAdmin.Id, Token: nonAdminToken, ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create non-admin token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/admin")
	group.Use(middleware.BackendUserAuth())
	downloadGroup := group.Group("/custom_build").Use(middleware.AdminPrivilege())
	downloadGroup.GET("/download/:id", (&CustomBuild{}).DownloadByID)

	for _, tc := range []struct {
		name   string
		token  string
		want   int
		status int
	}{
		{name: "missing authentication", want: 403, status: http.StatusOK},
		{name: "non-admin privilege", token: nonAdminToken, want: 403, status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/admin/custom_build/download/1", nil)
			if tc.token != "" {
				request.Header.Set("api-token", tc.token)
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("auth status = %d, want %d; body=%s", recorder.Code, tc.status, recorder.Body.String())
			}
			var envelope struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("auth response JSON: %v", err)
			}
			if envelope.Code != tc.want {
				t.Fatalf("auth response code = %d, want %d", envelope.Code, tc.want)
			}
		})
	}
}

func setupDownloadBuild(t *testing.T, status string) (*model.CustomBuild, *gorm.DB, func()) {
	t.Helper()
	withTestOutputRoot(t)
	db, sqlDB := newAdminProvenanceDB(t)
	previousGlobalDB, previousServiceDB, previousServices := global.DB, service.DB, service.AllService
	previousLogger := global.Logger
	global.DB = db
	service.DB = db
	global.Logger = logrus.New()
	service.AllService = &service.Service{CustomBuildService: &service.CustomBuildService{}}
	cleanup := func() {
		global.DB = previousGlobalDB
		service.DB = previousServiceDB
		service.AllService = previousServices
		global.Logger = previousLogger
		_ = sqlDB.Close()
	}

	build := &model.CustomBuild{
		Status:                status,
		Platform:              "windows",
		AppName:               "rustqs",
		Version:               "1.2.3",
		DownloadKey:           "capability-secret",
		BuildRef:              strings.Repeat("a", 40),
		SourceTag:             "1.2.3",
		AssetsRelease:         "offline-assets-1.2.3",
		AssetsReleaseID:       12,
		GithubRunId:           901,
		GithubProvider:        "github",
		GithubRepo:            "owner/repo",
		GithubWorkflow:        "workflow.yml",
		WorkflowSelector:      "rustqs/workflows",
		GithubRef:             strings.Repeat("a", 40),
		GithubArtifactName:    "artifact",
		GithubArtifactID:      42,
		GithubRunUrl:          "https://api.github.com/repos/owner/repo/actions/runs/901",
		GithubHtmlUrl:         "https://github.com/owner/repo/actions/runs/901",
		PublicationRecordedAt: 1,
	}
	prepareAdminBuildProvenance(build)
	setAdminWindowsProducerManifest(t, build, "binary")
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create download build: %v", err)
	}
	outDir := customBuildOutputDir(build.Id)
	if err := os.MkdirAll(outDir, 0700); err != nil {
		t.Fatalf("create download output: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "rustqs.exe"), []byte("binary"), 0600); err != nil {
		t.Fatalf("write download output: %v", err)
	}
	if status == model.CustomBuildStatusDone {
		markPublishedDigest(t, db, build)
	}
	return build, db, cleanup
}
