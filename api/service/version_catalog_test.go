package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"rustdesk-server/api/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestValidateGithubRepoStrictOwnerNameAllowlist(t *testing.T) {
	cases := []struct {
		name  string
		repo  string
		valid bool
	}{
		{name: "normal", repo: "owner/repo", valid: true},
		{name: "allowed punctuation", repo: "Owner.Name/repo_name-1", valid: true},
		{name: "dot in name", repo: "a-b/c.d", valid: true},
		{name: "empty", repo: "", valid: false},
		{name: "missing owner", repo: "/repo", valid: false},
		{name: "missing name", repo: "owner/", valid: false},
		{name: "missing slash", repo: "owner", valid: false},
		{name: "multiple slash", repo: "owner/repo/extra", valid: false},
		{name: "empty segment", repo: "owner//repo", valid: false},
		{name: "query", repo: "owner/repo?ref=main", valid: false},
		{name: "fragment", repo: "owner/repo#fragment", valid: false},
		{name: "encoded slash", repo: "owner/repo%2Fother", valid: false},
		{name: "space in owner", repo: "owner name/repo", valid: false},
		{name: "space in name", repo: "owner/repo name", valid: false},
		{name: "unicode", repo: "владелец/repo", valid: false},
		{name: "dot owner segment", repo: "./repo", valid: false},
		{name: "dot name segment", repo: "owner/.", valid: false},
		{name: "parent owner segment", repo: "../repo", valid: false},
		{name: "parent name segment", repo: "owner/..", valid: false},
		{name: "dot only", repo: ".", valid: false},
		{name: "parent only", repo: "..", valid: false},
		{name: "backslash", repo: `owner\\repo`, valid: false},
		{name: "semicolon", repo: "owner/repo;param", valid: false},
		{name: "colon", repo: "owner/repo:tag", valid: false},
		{name: "control character", repo: "owner/repo\n", valid: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGithubRepo(tc.repo)
			if (err == nil) != tc.valid {
				t.Fatalf("validateGithubRepo(%q) error = %v, valid = %v", tc.repo, err, tc.valid)
			}
		})
	}
}

func TestResolveVersionRejectsInvalidDisplayVersionAsClientValidation(t *testing.T) {
	_, err := (&GithubBuildConfigService{}).ResolveVersion(context.Background(), "not-a-version")
	if err == nil {
		t.Fatal("ResolveVersion() error = nil, want invalid display version error")
	}
	if !IsClientValidationError(err) {
		t.Fatalf("ResolveVersion() error = %T %v, want ClientValidationError", err, err)
	}
	if got, want := err.Error(), "invalid display version"; got != want {
		t.Fatalf("ResolveVersion() error = %q, want %q", got, want)
	}
}

func TestVersionCatalogFollowsPaginationAndResolvesConfiguredRepo(t *testing.T) {
	newVersionCatalogDB(t, "owner/repo-a")
	sha := strings.Repeat("a", 40)
	var pathsMu sync.Mutex
	var paths []string
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		pathsMu.Lock()
		paths = append(paths, req.URL.RequestURI())
		pathsMu.Unlock()
		switch {
		case req.URL.Path == "/repos/owner/repo-a/git/ref/heads/rustqs/workflows":
			return githubResponse(http.StatusOK, `{"ref":"refs/heads/rustqs/workflows","object":{"sha":"`+sha+`","type":"commit"}}`, nil), nil
		case req.URL.Path == "/repos/owner/repo-a/releases" && req.URL.Query().Get("page") == "":
			return githubResponse(http.StatusOK, `[{"id":7,"tag_name":"offline-assets-1.4.7"},{"id":8,"tag_name":"not-an-assets-release"}]`, http.Header{
				"Link": []string{`<https://api.github.com/repos/owner/repo-a/releases?per_page=100&page=2>; rel="next"`},
			}), nil
		case req.URL.Path == "/repos/owner/repo-a/releases" && req.URL.Query().Get("page") == "2":
			return githubResponse(http.StatusOK, `[{"id":9,"tag_name":"offline-assets-1.4.8"},{"id":10,"tag_name":"offline-assets-not-semver"}]`, nil), nil
		case req.URL.Path == "/repos/owner/repo-a/releases/7":
			return githubResponse(http.StatusOK, testReleaseDetails(7, "offline-assets-1.4.7"), nil), nil
		case req.URL.Path == "/repos/owner/repo-a/releases/9":
			return githubResponse(http.StatusOK, testReleaseDetails(9, "offline-assets-1.4.8"), nil), nil
		case strings.HasPrefix(req.URL.Path, "/repos/owner/repo-a/git/ref/tags/"):
			version := strings.TrimPrefix(req.URL.Path, "/repos/owner/repo-a/git/ref/tags/")
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/`+version+`","object":{"sha":"`+sha+`","type":"commit"}}`, nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	entries, err := (&GithubBuildConfigService{}).GetAvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("GetAvailableVersions() error = %v", err)
	}
	if len(entries) != 2 || entries[0].DisplayVersion != "1.4.8" || entries[1].DisplayVersion != "1.4.7" {
		t.Fatalf("catalog entries = %#v, want semver-descending 1.4.8 and 1.4.7", entries)
	}
	identity, err := (&GithubBuildConfigService{}).ResolveVersion(context.Background(), "1.4.8")
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}
	if identity.Repo != "owner/repo-a" || identity.BuildRef != sha || identity.SourceTag != "1.4.8" || identity.WorkflowRef != defaultWorkflowExecutionRef || identity.WorkflowSHA != sha || identity.AssetsRelease.ID != 9 || len(identity.AssetsRelease.Assets) != len(requiredOfflineAssetNames) {
		t.Fatalf("resolved identity = %#v", identity)
	}
	pathsMu.Lock()
	defer pathsMu.Unlock()
	for _, path := range paths {
		if !strings.Contains(path, "/repos/owner/repo-a/") {
			t.Errorf("catalog request escaped configured repo: %q", path)
		}
	}
}

func TestVersionCatalogCacheInvalidatesWhenConfiguredRepoChanges(t *testing.T) {
	db := newVersionCatalogDB(t, "owner/repo-a")
	sha := strings.Repeat("b", 40)
	var releaseRequests int32
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/git/ref/heads/rustqs/workflows") {
			return githubResponse(http.StatusOK, `{"ref":"refs/heads/rustqs/workflows","object":{"sha":"`+sha+`","type":"commit"}}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/releases") {
			atomic.AddInt32(&releaseRequests, 1)
			return githubResponse(http.StatusOK, `[{"id":1,"tag_name":"offline-assets-1.4.8"}]`, nil), nil
		}
		if req.URL.Path == "/repos/owner/repo-a/releases/1" || req.URL.Path == "/repos/owner/repo-b/releases/1" {
			return githubResponse(http.StatusOK, testReleaseDetails(1, "offline-assets-1.4.8"), nil), nil
		}
		if strings.HasPrefix(req.URL.Path, "/repos/owner/repo-a/git/ref/tags/") || strings.HasPrefix(req.URL.Path, "/repos/owner/repo-b/git/ref/tags/") {
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/1.4.8","object":{"sha":"`+sha+`","type":"commit"}}`, nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
	}))

	if _, err := (&GithubBuildConfigService{}).GetAvailableVersions(context.Background()); err != nil {
		t.Fatalf("first catalog call: %v", err)
	}
	if err := db.Model(&model.GithubBuildConfig{}).Where("id = ?", 1).Update("repo", "owner/repo-b").Error; err != nil {
		t.Fatalf("mutate configured repo: %v", err)
	}
	entries, err := (&GithubBuildConfigService{}).GetAvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("catalog call after repo mutation: %v", err)
	}
	if len(entries) != 1 || entries[0].Repo != "owner/repo-b" {
		t.Fatalf("catalog after repo mutation = %#v", entries)
	}
	if got := atomic.LoadInt32(&releaseRequests); got != 2 {
		t.Fatalf("release requests = %d, want one request per configured repo", got)
	}
}

func TestVersionCatalogFailsClosedForMissingOrMismatchedSourceTag(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		code int
	}{
		{name: "missing source tag", code: http.StatusNotFound, body: `{"message":"missing"}`},
		{name: "mismatched source ref", code: http.StatusOK, body: `{"ref":"refs/tags/1.4.7","object":{"sha":"` + strings.Repeat("c", 40) + `","type":"commit"}}`},
		{name: "non-commit source object", code: http.StatusOK, body: `{"ref":"refs/tags/1.4.8","object":{"sha":"` + strings.Repeat("c", 40) + `","type":"tag"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			newVersionCatalogDB(t, "owner/repo")
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/releases") {
					return githubResponse(http.StatusOK, `[{"id":1,"tag_name":"offline-assets-1.4.8"}]`, nil), nil
				}
				if req.URL.Path == "/repos/owner/repo/releases/1" {
					return githubResponse(http.StatusOK, testReleaseDetails(1, "offline-assets-1.4.8"), nil), nil
				}
				return githubResponse(test.code, test.body, nil), nil
			}))
			_, err := (&GithubBuildConfigService{}).GetAvailableVersions(context.Background())
			if err == nil {
				t.Fatal("GetAvailableVersions() error = nil, want fail-closed catalog error")
			}
			var apiErr *GithubAPIError
			if test.code == http.StatusNotFound && !errors.As(err, &apiErr) {
				t.Fatalf("missing source error = %T %v, want wrapped GithubAPIError", err, err)
			}
		})
	}
}

func TestVersionCatalogRequiresExactReleaseAssetIdentityAndDigest(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutateBody func(string) string
	}{
		{name: "missing asset", mutateBody: func(body string) string {
			return strings.Replace(body, `{"id":104,"name":"printer_driver_adapter.zip","digest":"sha256:`+strings.Repeat("4", 64)+`"}`, "", 1)
		}},
		{name: "missing digest", mutateBody: func(body string) string {
			return strings.Replace(body, "sha256:"+strings.Repeat("1", 64), "", 1)
		}},
		{name: "provider supplied non-sha256 digest", mutateBody: func(body string) string {
			return strings.Replace(body, "sha256:"+strings.Repeat("1", 64), "md5:"+strings.Repeat("1", 32), 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			newVersionCatalogDB(t, "owner/repo")
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/repos/owner/repo/releases":
					return githubResponse(http.StatusOK, `[{"id":1,"tag_name":"offline-assets-1.4.8"}]`, nil), nil
				case "/repos/owner/repo/releases/1":
					return githubResponse(http.StatusOK, test.mutateBody(testReleaseDetails(1, "offline-assets-1.4.8")), nil), nil
				case "/repos/owner/repo/git/ref/tags/1.4.8":
					return githubResponse(http.StatusOK, `{"ref":"refs/tags/1.4.8","object":{"sha":"`+strings.Repeat("a", 40)+`","type":"commit"}}`, nil), nil
				default:
					return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
				}
			}))
			if _, err := (&GithubBuildConfigService{}).GetAvailableVersions(context.Background()); err == nil {
				t.Fatal("GetAvailableVersions() error = nil, want release asset rejection")
			}
		})
	}
}

func TestVersionCatalogConcurrentCallsShareOneRefresh(t *testing.T) {
	newVersionCatalogDB(t, "owner/repo")
	sha := strings.Repeat("d", 40)
	var releases, refs int32
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/git/ref/heads/rustqs/workflows"):
			return githubResponse(http.StatusOK, `{"ref":"refs/heads/rustqs/workflows","object":{"sha":"`+strings.Repeat("e", 40)+`","type":"commit"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/releases"):
			atomic.AddInt32(&releases, 1)
			time.Sleep(20 * time.Millisecond)
			return githubResponse(http.StatusOK, `[{"id":1,"tag_name":"offline-assets-1.4.8"}]`, nil), nil
		case strings.Contains(req.URL.Path, "/git/ref/tags/"):
			atomic.AddInt32(&refs, 1)
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/1.4.8","object":{"sha":"`+sha+`","type":"commit"}}`, nil), nil
		case req.URL.Path == "/repos/owner/repo/releases/1":
			return githubResponse(http.StatusOK, testReleaseDetails(1, "offline-assets-1.4.8"), nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entries, err := (&GithubBuildConfigService{}).GetAvailableVersions(context.Background())
			if err == nil && len(entries) != 1 {
				err = errors.New("unexpected catalog length")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent catalog call: %v", err)
		}
	}
	if atomic.LoadInt32(&releases) != 1 || atomic.LoadInt32(&refs) != 1 {
		t.Fatalf("refresh requests = releases:%d refs:%d, want one each", releases, refs)
	}
}

func newVersionCatalogDB(t *testing.T, repo string) *gorm.DB {
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
	if err := db.AutoMigrate(&model.GithubBuildConfig{}); err != nil {
		t.Fatalf("migrate GitHub config: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: repo}).Error; err != nil {
		t.Fatalf("create GitHub config: %v", err)
	}
	previousDB := DB
	DB = db
	resetVersionCatalogCache()
	t.Cleanup(func() {
		resetVersionCatalogCache()
		DB = previousDB
		_ = sqlDB.Close()
	})
	return db
}
