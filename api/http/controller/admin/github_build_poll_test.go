package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"net/http/httptest"
	"rustdesk-server/api/global"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

type adminGithubRoundTripFunc func(*http.Request) (*http.Response, error)

func prepareAdminBuildProvenance(build *model.CustomBuild) {
	if build == nil || build.GithubRunId <= 0 || build.GithubRef == "" {
		return
	}
	if build.WorkflowSelector == "" {
		build.WorkflowSelector = "rustqs/min-test"
	}
	if build.AssetsReleaseAssets == "" {
		build.AssetsReleaseAssets = `[{"id":101,"name":"windows-x64-release.zip","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"},{"id":102,"name":"usbmmidd_v2.zip","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"},{"id":103,"name":"rustdesk_printer_driver_v4-1.4.zip","digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333"},{"id":104,"name":"printer_driver_adapter.zip","digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444"}]`
	}
}

func (f adminGithubRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGithubPollErrorActionForFailsTerminalAndRetriesRetryable(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		result githubPollErrorAction
	}{
		{
			name:   "terminal API error fails promptly",
			err:    &service.GithubAPIError{StatusCode: http.StatusUnauthorized, Terminal: true},
			result: githubPollFail,
		},
		{
			name:   "rate limit remains retryable",
			err:    &service.GithubAPIError{StatusCode: http.StatusTooManyRequests, Retryable: true},
			result: githubPollRetry,
		},
		{
			name:   "transport timeout remains retryable",
			err:    &service.GithubTransportError{Operation: "GET run status", Cause: context.DeadlineExceeded},
			result: githubPollRetry,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := githubPollErrorActionFor(tc.err); got != tc.result {
				t.Fatalf("githubPollErrorActionFor() = %d, want %d", got, tc.result)
			}
		})
	}
}

func TestGithubArtifactErrorActionClassifiesRetryableAndTerminalFailures(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		result githubPollErrorAction
	}{
		{name: "429", err: &service.GithubAPIError{StatusCode: http.StatusTooManyRequests, Retryable: true}, result: githubPollRetry},
		{name: "500", err: &service.GithubAPIError{StatusCode: http.StatusInternalServerError, Retryable: true}, result: githubPollRetry},
		{name: "transport", err: &service.GithubTransportError{Operation: "download artifact", Cause: context.DeadlineExceeded}, result: githubPollRetry},
		{name: "401", err: &service.GithubAPIError{StatusCode: http.StatusUnauthorized, Terminal: true}, result: githubPollFail},
		{name: "403 ordinary", err: &service.GithubAPIError{StatusCode: http.StatusForbidden, Terminal: true}, result: githubPollFail},
		{name: "404", err: &service.GithubAPIError{StatusCode: http.StatusNotFound, Terminal: true}, result: githubPollFail},
		{name: "410", err: &service.GithubAPIError{StatusCode: http.StatusGone, Terminal: true}, result: githubPollFail},
		{name: "422", err: &service.GithubAPIError{StatusCode: http.StatusUnprocessableEntity, Terminal: true}, result: githubPollFail},
		{name: "missing artifact", err: &service.GithubArtifactUnavailableError{RunID: 7, ArtifactName: "requested", Available: []string{"other"}}, result: githubPollFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := githubArtifactErrorActionFor(tc.err); got != tc.result {
				t.Fatalf("githubArtifactErrorActionFor() = %d, want %d", got, tc.result)
			}
		})
	}
}

func TestGithubPollTerminalStatusErrorFailsEveryActiveLifecycleState(t *testing.T) {
	for _, status := range []string{
		model.CustomBuildStatusBuilding,
		model.CustomBuildStatusDownloading,
		model.CustomBuildStatusExtracting,
	} {
		t.Run(status, func(t *testing.T) {
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
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"expired"}`)),
				}, nil
			})
			t.Cleanup(func() {
				http.DefaultTransport = previousTransport
				global.DB = previousDB
				global.Logger = previousLogger
				service.DB = previousServiceDB
				service.AllService = previousService
				_ = sqlDB.Close()
			})

			const runID int64 = 901
			build := &model.CustomBuild{
				Status:             status,
				Platform:           "windows",
				Version:            "1.2.3",
				BuildRef:           strings.Repeat("a", 40),
				SourceTag:          "1.2.3",
				AssetsRelease:      "offline-assets-1.2.3",
				AssetsReleaseID:    12,
				GithubRunId:        runID,
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
			if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "token"}).Error; err != nil {
				t.Fatalf("create GitHub config: %v", err)
			}

			currentTime := time.Now()
			(&CustomBuild{}).pollAndDownloadWithClock(build.Id, runID, githubPollClock{
				now:  func() time.Time { return currentTime },
				wait: func(delay time.Duration) { currentTime = currentTime.Add(delay) },
			})

			var stored model.CustomBuild
			if err := db.First(&stored, build.Id).Error; err != nil {
				t.Fatalf("read build: %v", err)
			}
			if stored.Status != model.CustomBuildStatusFailed {
				t.Fatalf("terminal status error left %s row in %q, want failed", status, stored.Status)
			}
			if !strings.Contains(stored.BuildLog, "github status error") {
				t.Fatalf("terminal status error log = %q, want status error", stored.BuildLog)
			}
		})
	}
}

func TestGithubPollRejectsRunHeadSHAMismatchWithStoredWorkflowSHA(t *testing.T) {
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
		if req.URL.Path != "/repos/owner/repo/actions/runs/902" {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"message":"unexpected endpoint"}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"status":"in_progress","head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`)),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
		global.DB = previousDB
		global.Logger = previousLogger
		service.DB = previousServiceDB
		service.AllService = previousService
		_ = sqlDB.Close()
	})

	workflowSHA := strings.Repeat("a", 40)
	build := &model.CustomBuild{
		Status:             model.CustomBuildStatusBuilding,
		Platform:           "windows",
		Version:            "1.2.3",
		BuildRef:           strings.Repeat("c", 40),
		SourceTag:          "1.2.3",
		AssetsRelease:      "offline-assets-1.2.3",
		AssetsReleaseID:    12,
		GithubRunId:        902,
		GithubProvider:     "github",
		GithubRepo:         "owner/repo",
		GithubWorkflow:     "rustqs-windows-min-test.yml",
		WorkflowSelector:   "rustqs/min-test",
		GithubRef:          workflowSHA,
		GithubArtifactName: "artifact",
		GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/902",
		GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/902",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "current-token"}).Error; err != nil {
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
	if stored.Status != model.CustomBuildStatusFailed {
		t.Fatalf("mismatched run head SHA left build in %q, want failed", stored.Status)
	}
	if !strings.Contains(stored.BuildLog, "does not match stored workflow execution SHA") {
		t.Fatalf("mismatch failure log = %q, want explicit stored workflow SHA rejection", stored.BuildLog)
	}
	if stored.GithubSourceSha != "" {
		t.Fatalf("mismatched run head SHA was persisted as %q", stored.GithubSourceSha)
	}
}

func TestPersistBuildProgressDoesNotCarryImmutableRunIdentity(t *testing.T) {
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 7}, Status: model.CustomBuildStatusDone, GithubRunId: 9, GithubArtifactID: 42}
	wantCause := errors.New("database unavailable")
	var persisted service.BuildProgress

	err := persistBuildProgressWithRun(func(progress service.BuildProgress) error {
		persisted = progress
		return wantCause
	}, build, build.GithubRunId)
	var persistenceErr *service.BuildProgressPersistenceError
	if !errors.As(err, &persistenceErr) || !errors.Is(err, wantCause) {
		t.Fatalf("persistence error = %T %v, want typed wrapped failure", err, err)
	}
	if persisted.Status != build.Status {
		t.Fatalf("progress = %#v, want only mutable status fields", persisted)
	}
	if persisted.ExpectedRunID != build.GithubRunId {
		t.Fatalf("progress expected run id = %d, want %d", persisted.ExpectedRunID, build.GithubRunId)
	}
	if persisted.ExpectedArtifactID != build.GithubArtifactID {
		t.Fatalf("progress expected artifact id = %d, want %d", persisted.ExpectedArtifactID, build.GithubArtifactID)
	}
}

func TestPersistBuildMutationRetriesAndBoundsAttempts(t *testing.T) {
	t.Run("retry succeeds", func(t *testing.T) {
		calls := 0
		if err := persistBuildMutation(7, "test", func() error {
			calls++
			if calls < 3 {
				return errors.New("temporary database error")
			}
			return nil
		}); err != nil {
			t.Fatalf("persistBuildMutation() error = %v", err)
		}
		if calls != 3 {
			t.Fatalf("persistBuildMutation() calls = %d, want 3", calls)
		}
	})

	t.Run("exhaustion is explicit", func(t *testing.T) {
		calls := 0
		wantErr := errors.New("database remains unavailable")
		err := persistBuildMutation(7, "test", func() error {
			calls++
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("persistBuildMutation() error = %v, want wrapped exhaustion error", err)
		}
		if calls != buildPersistenceAttempts {
			t.Fatalf("persistBuildMutation() calls = %d, want bounded %d", calls, buildPersistenceAttempts)
		}
	})
}

func TestPollPersistenceExhaustionSchedulesOneRetryForSameBuildRun(t *testing.T) {
	resetScheduledGithubPollRetries := func() {
		scheduledGithubPollRetries.Lock()
		scheduledGithubPollRetries.keys = make(map[githubPollRetryKey]struct{})
		scheduledGithubPollRetries.Unlock()
	}
	resetScheduledGithubPollRetries()
	t.Cleanup(resetScheduledGithubPollRetries)
	previousSchedule := scheduleGithubPollRetry
	var scheduled []githubPollRetryKey
	scheduleGithubPollRetry = func(buildID uint, runID int64) {
		scheduled = append(scheduled, githubPollRetryKey{buildID: buildID, runID: runID})
	}
	t.Cleanup(func() { scheduleGithubPollRetry = previousSchedule })

	wantErr := errors.New("database unavailable")
	mutation := func() error { return wantErr }
	if err := persistPollMutation(991, 771, "test", mutation); !errors.Is(err, wantErr) {
		t.Fatalf("persistPollMutation() error = %v, want wrapped persistence error", err)
	}
	if err := persistPollMutation(991, 771, "test", mutation); !errors.Is(err, wantErr) {
		t.Fatalf("second persistPollMutation() error = %v, want wrapped persistence error", err)
	}
	if len(scheduled) != 1 || scheduled[0].buildID != 991 || scheduled[0].runID != 771 {
		t.Fatalf("scheduled retries = %#v, want one retry for exact build/run", scheduled)
	}
}

func TestPollPersistenceRetryCanBeScheduledAgainAfterAttemptFailure(t *testing.T) {
	resetScheduledGithubPollRetries := func() {
		scheduledGithubPollRetries.Lock()
		scheduledGithubPollRetries.keys = make(map[githubPollRetryKey]struct{})
		scheduledGithubPollRetries.Unlock()
	}
	resetScheduledGithubPollRetries()
	t.Cleanup(resetScheduledGithubPollRetries)
	previousSchedule := scheduleGithubPollRetry
	var scheduled []githubPollRetryKey
	scheduleGithubPollRetry = func(buildID uint, runID int64) {
		scheduled = append(scheduled, githubPollRetryKey{buildID: buildID, runID: runID})
	}
	t.Cleanup(func() { scheduleGithubPollRetry = previousSchedule })

	if err := persistPollMutation(992, 772, "test", func() error {
		return errors.New("database unavailable")
	}); err == nil {
		t.Fatal("first persistPollMutation() error = nil")
	}
	runScheduledGithubPollRetry(992, 772, func() bool { return false })
	if err := persistPollMutation(992, 772, "test", func() error {
		return errors.New("database unavailable again")
	}); err == nil {
		t.Fatal("second persistPollMutation() error = nil")
	}
	if len(scheduled) != 2 {
		t.Fatalf("scheduled retries after failed attempt = %d, want 2", len(scheduled))
	}
	if err := persistPollMutation(992, 772, "test", func() error {
		return errors.New("database unavailable third time")
	}); err == nil {
		t.Fatal("third persistPollMutation() error = nil")
	}
	if len(scheduled) != 2 {
		t.Fatalf("duplicate pending retries = %d, want 2", len(scheduled))
	}
}

func TestPersistCompletedBuildDoesNotClaimDoneAfterExhaustion(t *testing.T) {
	root := withTestOutputRoot(t)
	build := &model.CustomBuild{IdModel: model.IdModel{Id: 7}, Status: model.CustomBuildStatusDone, GithubArtifactID: 42}
	wantErr := errors.New("database unavailable")
	var persisted service.BuildProgress
	if err := persistCompletedBuildWith(func(progress service.BuildProgress) error {
		persisted = progress
		return wantErr
	}, build, 7); !errors.Is(err, wantErr) {
		t.Fatalf("persistCompletedBuildWith() error = %v, want wrapped persistence error", err)
	}
	if build.Status != model.CustomBuildStatusBuilding {
		t.Fatalf("build status after exhausted completion = %q, want recoverable building", build.Status)
	}
	if persisted.ExpectedArtifactID != build.GithubArtifactID {
		t.Fatalf("completion expected artifact id = %d, want %d", persisted.ExpectedArtifactID, build.GithubArtifactID)
	}
	output := filepath.Join(root, "output", "7", "rustqs.exe")
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		t.Fatalf("create published output: %v", err)
	}
	if err := os.WriteFile(output, []byte("valid output"), 0600); err != nil {
		t.Fatalf("write published output: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("published output was removed after CAS failure: %v", err)
	}
}

func TestResumePendingPollsUsesStoredProvenanceAndGuardsIncompleteRows(t *testing.T) {
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousService := global.DB, service.AllService
	previousServiceDB, previousLogger := service.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	service.DB = db
	service.AllService = &service.Service{
		CustomBuildService:       &service.CustomBuildService{},
		GithubBuildConfigService: &service.GithubBuildConfigService{},
	}
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		service.DB = previousServiceDB
		service.AllService = previousService
		_ = sqlDB.Close()
	})

	if err := db.Create(&model.GithubBuildConfig{
		IdModel:          model.IdModel{Id: 1},
		Repo:             "owner/repo-b",
		WorkflowFilename: "workflow-b.yml",
		Branch:           "ref-b",
		Token:            "current-token",
	}).Error; err != nil {
		t.Fatalf("create mutable global config: %v", err)
	}
	stored := &model.CustomBuild{
		Status:              model.CustomBuildStatusBuilding,
		Version:             "1.2.3",
		GithubRunId:         777,
		GithubProvider:      "github",
		GithubRepo:          "owner/repo-a",
		GithubWorkflow:      "workflow-a.yml",
		WorkflowSelector:    "rustqs/min-test",
		GithubRef:           strings.Repeat("a", 40),
		BuildRef:            strings.Repeat("a", 40),
		SourceTag:           "1.2.3",
		AssetsRelease:       "offline-assets-1.2.3",
		AssetsReleaseID:     12,
		GithubArtifactName:  "artifact-a",
		GithubRunUrl:        "https://api.github.com/repos/owner/repo-a/actions/runs/777",
		GithubHtmlUrl:       "https://github.com/owner/repo-a/actions/runs/777",
		AssetsReleaseAssets: `[{"id":101,"name":"windows-x64-release.zip","digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"},{"id":102,"name":"usbmmidd_v2.zip","digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222"},{"id":103,"name":"rustdesk_printer_driver_v4-1.4.zip","digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333"},{"id":104,"name":"printer_driver_adapter.zip","digest":"sha256:4444444444444444444444444444444444444444444444444444444444444444"}]`,
	}
	legacy := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 0}
	partial := &model.CustomBuild{
		Status:          model.CustomBuildStatusBuilding,
		Version:         "1.2.3",
		BuildRef:        strings.Repeat("b", 40),
		SourceTag:       "1.2.3",
		AssetsRelease:   "offline-assets-1.2.3",
		AssetsReleaseID: 13,
	}
	if err := db.Create(stored).Error; err != nil {
		t.Fatalf("create stored build: %v", err)
	}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("create legacy build: %v", err)
	}
	if err := db.Create(partial).Error; err != nil {
		t.Fatalf("create partial build: %v", err)
	}

	requestPath := make(chan string, 1)
	previousTransport := http.DefaultTransport
	http.DefaultTransport = adminGithubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestPath <- req.URL.Path
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"expired"}`)),
		}, nil
	})
	defer func() { http.DefaultTransport = previousTransport }()

	ResumePendingPolls()
	select {
	case path := <-requestPath:
		if path != "/repos/owner/repo-a/actions/runs/777" {
			t.Fatalf("resume request path = %q, want stored repo A/run 777", path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ResumePendingPolls() did not start the stored build poll")
	}
	if !activeGithubPolls.wait(stored.Id, 2*time.Second) {
		t.Fatal("stored build poll did not exit before test cleanup")
	}

	legacyFailed := false
	partialFailed := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var loadedLegacy, loadedPartial model.CustomBuild
		if err := db.First(&loadedLegacy, legacy.Id).Error; err == nil && loadedLegacy.Status == model.CustomBuildStatusFailed {
			if !strings.Contains(loadedLegacy.BuildLog, "no provider run id") {
				t.Fatalf("legacy failure log = %q, want explicit provenance reason", loadedLegacy.BuildLog)
			}
			legacyFailed = true
		}
		if err := db.First(&loadedPartial, partial.Id).Error; err != nil {
			t.Fatalf("read partial build: %v", err)
		}
		if loadedPartial.Status == model.CustomBuildStatusFailed {
			if !strings.Contains(loadedPartial.BuildLog, "no provider run id") {
				t.Fatalf("partial failure log = %q, want explicit no-run reason", loadedPartial.BuildLog)
			}
			partialFailed = true
		}
		if legacyFailed && partialFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("legacy or partial no-run row was not failed closed")
}

func TestPollConfigCredentialExhaustionPersistsGuardedFailure(t *testing.T) {
	db, sqlDB := newAdminProvenanceDB(t)
	previousDB, previousService := global.DB, service.AllService
	previousServiceDB, previousLogger := service.DB, global.Logger
	global.DB = db
	global.Logger = logrus.New()
	service.DB = db
	service.AllService = &service.Service{
		CustomBuildService:       &service.CustomBuildService{},
		GithubBuildConfigService: &service.GithubBuildConfigService{},
	}
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
		service.DB = previousServiceDB
		service.AllService = previousService
		_ = sqlDB.Close()
	})
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo"}).Error; err != nil {
		t.Fatalf("create empty GitHub config: %v", err)
	}
	build := &model.CustomBuild{
		Status:             model.CustomBuildStatusBuilding,
		Platform:           "windows",
		Version:            "1.2.3",
		BuildRef:           strings.Repeat("a", 40),
		SourceTag:          "1.2.3",
		AssetsRelease:      "offline-assets-1.2.3",
		AssetsReleaseID:    12,
		GithubRunId:        900,
		GithubProvider:     "github",
		GithubRepo:         "owner/repo",
		GithubWorkflow:     "rustqs-windows-min-test.yml",
		GithubRef:          strings.Repeat("a", 40),
		GithubArtifactName: "rustdesk-min-test-windows",
		BuildLog:           strings.Repeat("old log ", 1000),
		GithubRunUrl:       "https://api.github.com/repos/owner/repo/actions/runs/900",
		GithubHtmlUrl:      "https://github.com/owner/repo/actions/runs/900",
	}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create building row: %v", err)
	}

	currentTime := time.Now()
	(&CustomBuild{}).pollAndDownloadWithClock(build.Id, build.GithubRunId, githubPollClock{
		now:  func() time.Time { return currentTime },
		wait: func(delay time.Duration) { currentTime = currentTime.Add(delay) },
	})
	var stored model.CustomBuild
	if err := db.First(&stored, build.Id).Error; err != nil {
		t.Fatalf("read building row: %v", err)
	}
	if stored.Status != model.CustomBuildStatusFailed {
		t.Fatalf("status after credential deadline = %q, want failed", stored.Status)
	}
	if len(stored.BuildLog) > maxBuildLogBytes || !strings.Contains(stored.BuildLog, "restore the PAT") {
		t.Fatalf("credential deadline log length=%d contents=%q, want bounded actionable message", len(stored.BuildLog), stored.BuildLog)
	}
	if stored.GithubRunId != build.GithubRunId {
		t.Fatalf("credential deadline changed run identity: %d", stored.GithubRunId)
	}
}

func TestLoadGithubPollConfigRetriesWithoutRealBackoffAndKeepsStoredIdentity(t *testing.T) {
	start := time.Unix(100, 0)
	currentTime := start
	provenance := service.BuildProvenance{GithubRepo: "owner/stored-repo"}
	attempts := 0
	var waits []time.Duration
	config, ok := loadGithubPollConfig(
		17,
		provenance,
		start.Add(2*time.Minute),
		func() time.Time { return currentTime },
		func(delay time.Duration) {
			waits = append(waits, delay)
			currentTime = currentTime.Add(delay)
		},
		func() (*model.GithubBuildConfig, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("config temporarily unavailable")
			}
			return &model.GithubBuildConfig{Repo: "owner/mutable-repo", Token: "current-token"}, nil
		},
	)
	if !ok || config == nil {
		t.Fatal("loadGithubPollConfig() did not recover within the injected deadline")
	}
	if attempts != 3 || len(waits) != 2 || waits[0] != githubPollInterval || waits[1] != githubPollInterval {
		t.Fatalf("retry schedule: attempts=%d waits=%v", attempts, waits)
	}
	if config.Repo != provenance.GithubRepo || config.Token != "current-token" {
		t.Fatalf("poll config identity = repo %q token %q, want stored repo %q and current token", config.Repo, config.Token, provenance.GithubRepo)
	}
}

func TestGithubPollOwnershipIsKeyedAndReleased(t *testing.T) {
	ownership := newGithubPollOwnership()
	release, claimed := ownership.claim(42)
	if !claimed {
		t.Fatal("first poll claim = false, want true")
	}
	if _, claimed = ownership.claim(42); claimed {
		t.Fatal("second poll claim = true, want duplicate rejection")
	}
	release()
	if !ownership.wait(42, time.Second) {
		t.Fatal("released poll remains active")
	}
	release, claimed = ownership.claim(42)
	if !claimed {
		t.Fatal("claim after release = false, want true")
	}
	release()
}

func TestDeleteWaitsForSameBuildLifecycleGuard(t *testing.T) {
	db, sqlDB := newAdminProvenanceDB(t)
	previousGlobalDB, previousServiceDB := global.DB, service.DB
	previousServices := service.AllService
	previousOutputDir := service.BuildOutputDir
	global.DB = db
	service.DB = db
	service.AllService = &service.Service{CustomBuildService: &service.CustomBuildService{}}
	outputRoot := t.TempDir()
	service.BuildOutputDir = func(id uint) string { return filepath.Join(outputRoot, fmt.Sprint(id)) }
	t.Cleanup(func() {
		global.DB = previousGlobalDB
		service.DB = previousServiceDB
		service.AllService = previousServices
		service.BuildOutputDir = previousOutputDir
		_ = sqlDB.Close()
	})
	build := &model.CustomBuild{Status: model.CustomBuildStatusBuilding, GithubRunId: 17}
	prepareAdminBuildProvenance(build)
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	releasePoll, claimed := activeGithubPolls.claim(build.Id)
	if !claimed {
		t.Fatal("poll lifecycle claim = false, want true")
	}
	t.Cleanup(releasePoll)
	body, err := json.Marshal(map[string]any{"id": build.Id})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/delete", strings.NewReader(string(body)))
	done := make(chan struct{})
	go func() {
		(&CustomBuild{}).Delete(c)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("Delete completed while poll lifecycle guard was held")
	case <-time.After(50 * time.Millisecond):
	}
	var stillThere model.CustomBuild
	if err := db.First(&stillThere, build.Id).Error; err != nil {
		t.Fatalf("guarded delete removed row too early: %v", err)
	}
	releasePoll()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Delete did not complete after poll lifecycle guard release")
	}
	var deleted model.CustomBuild
	if err := db.First(&deleted, build.Id).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted build lookup error = %v, want record not found", err)
	}
}

func TestCustomBuildDeleteReturnsSuccessWhenArtifactCleanupIsPending(t *testing.T) {
	db, sqlDB := newAdminProvenanceDB(t)
	previousGlobalDB, previousServiceDB := global.DB, service.DB
	previousServices := service.AllService
	previousOutputDir := service.BuildOutputDir
	previousLogger, previousLocalizer := global.Logger, global.Localizer
	var logs bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&logs)
	t.Cleanup(func() {
		global.DB = previousGlobalDB
		service.DB = previousServiceDB
		service.AllService = previousServices
		service.BuildOutputDir = previousOutputDir
		global.Logger = previousLogger
		global.Localizer = previousLocalizer
		_ = sqlDB.Close()
	})
	global.DB = db
	service.DB = db
	service.AllService = &service.Service{CustomBuildService: &service.CustomBuildService{}}
	global.Logger = logger
	global.Localizer = testManifestLocalizer
	outputRoot := t.TempDir()
	service.BuildOutputDir = func(id uint) string {
		return filepath.Join(outputRoot, "output", fmt.Sprint(id), "\x00")
	}

	build := &model.CustomBuild{Status: model.CustomBuildStatusDone}
	if err := db.Create(build).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	body, err := json.Marshal(map[string]any{"id": build.Id})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/delete", strings.NewReader(string(body)))

	(&CustomBuild{}).Delete(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("Delete HTTP status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			CleanupPending bool `json:"cleanup_pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Delete response JSON: %v", err)
	}
	if envelope.Code != 0 {
		t.Fatalf("Delete response code = %d, want success: %s", envelope.Code, recorder.Body.String())
	}
	if !envelope.Data.CleanupPending {
		t.Fatalf("Delete response data = %#v, want cleanup_pending marker", envelope.Data)
	}
	var deleted model.CustomBuild
	if err := db.First(&deleted, build.Id).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted build lookup error = %v, want record not found", err)
	}
	if !strings.Contains(logs.String(), "artifact cleanup remains pending") {
		t.Fatalf("cleanup failure was not logged: %s", logs.String())
	}
}

func newAdminProvenanceDB(t *testing.T) (*gorm.DB, *sql.DB) {
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
	if err := db.AutoMigrate(&model.CustomBuild{}, &model.GithubBuildConfig{}); err != nil {
		t.Fatalf("migrate provenance models: %v", err)
	}
	return db, sqlDB
}
