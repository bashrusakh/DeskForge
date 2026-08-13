package admin

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"rustdesk-server/api/utils"
)

type CustomBuild struct{}

func failCustomBuildRead(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if global.Logger != nil {
		global.Logger.Errorf("custom build read failed: %s", service.GithubErrorDetail(err))
	}
	response.FailStatus(c, http.StatusInternalServerError, 101, "custom build is unavailable")
}

type buildProgressUpdater func(service.BuildProgress) error
type githubPollClock struct {
	now  func() time.Time
	wait func(time.Duration)
}

const (
	buildPersistenceAttempts     = 3
	buildPersistenceRetryDelay   = 10 * time.Millisecond
	artifactCleanupRetryAttempts = 3
	maxBuildLogBytes             = service.MaxBuildLogBytes
	buildOutputTempTTL           = 24 * time.Hour
)

var (
	artifactMaxZipEntries         int64  = 4096
	artifactMaxFileBytes          uint64 = 512 << 20
	artifactMaxAggregateBytes     uint64 = 1 << 30
	artifactMaxCompressionRatio   uint64 = 1000
	downloadArchiveMaxOutputBytes int64  = 512 << 20
	downloadArchiveMaxFileBytes   int64  = 512 << 20
	downloadArchiveMaxSourceBytes int64  = 1 << 30
	downloadArchiveMaxFiles       int    = 4096
	downloadArchiveDigestScope           = "sha256 covers the exact redacted public ZIP archive bytes served by this response; it is distinct from the stored publication digest"
	downloadArchiveSlots                 = make(chan struct{}, 2)
	removeArtifactArchive                = os.Remove
	scheduleArtifactCleanupRetry         = scheduleArtifactCleanupRetryDefault
	artifactCleanupRetryDelay            = time.Minute
	downloadArchiveSnapshotHook          = func() {}
)

var scheduleGithubPollRetry func(buildID uint, runID int64)

// Kept as a seam for hermetic publication tests; production semantics remain
// the shared /rdgen-data/output/{buildID} location.
var customBuildOutputDir = service.BuildOutputDir

type githubPollOwnership struct {
	mu     sync.Mutex
	active map[uint]chan struct{}
}

func newGithubPollOwnership() *githubPollOwnership {
	return &githubPollOwnership{active: make(map[uint]chan struct{})}
}

func (o *githubPollOwnership) claim(buildID uint) (func(), bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, exists := o.active[buildID]; exists {
		return nil, false
	}
	done := make(chan struct{})
	o.active[buildID] = done
	return func() {
		o.mu.Lock()
		if current, exists := o.active[buildID]; exists && current == done {
			delete(o.active, buildID)
			close(done)
		}
		o.mu.Unlock()
	}, true
}

func (o *githubPollOwnership) wait(buildID uint, timeout time.Duration) bool {
	o.mu.Lock()
	done, active := o.active[buildID]
	o.mu.Unlock()
	if !active {
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func (o *githubPollOwnership) acquire(ctx context.Context, buildID uint) (func(), error) {
	for {
		if release, claimed := o.claim(buildID); claimed {
			return release, nil
		}
		o.mu.Lock()
		done := o.active[buildID]
		o.mu.Unlock()
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// This guard is intentionally process-local. A cross-process lease would
// require a schema/expiry contract and is outside this corrective diff.
var activeGithubPolls = newGithubPollOwnership()

type githubPollRetryKey struct {
	buildID uint
	runID   int64
}

// This is deliberately process-local and bounded to one scheduled retry per
// build/run. It is not a durable outbox or a distributed scheduler; a restart
// or another API process still requires the normal ResumePendingPolls hook.
var scheduledGithubPollRetries = struct {
	sync.Mutex
	keys map[githubPollRetryKey]struct{}
}{
	keys: make(map[githubPollRetryKey]struct{}),
}

func startGithubPoll(ct *CustomBuild, buildID uint, runID int64) bool {
	release, claimed := activeGithubPolls.claim(buildID)
	if !claimed {
		return false
	}
	go func() {
		defer release()
		ct.pollAndDownload(buildID, runID)
	}()
	return true
}

func schedulePollRetryAfterPersistence(buildID uint, runID int64) {
	if buildID == 0 || runID <= 0 {
		return
	}
	key := githubPollRetryKey{buildID: buildID, runID: runID}
	scheduledGithubPollRetries.Lock()
	_, alreadyScheduled := scheduledGithubPollRetries.keys[key]
	if !alreadyScheduled {
		scheduledGithubPollRetries.keys[key] = struct{}{}
	}
	scheduledGithubPollRetries.Unlock()
	if alreadyScheduled {
		return
	}
	if scheduleGithubPollRetry != nil {
		scheduleGithubPollRetry(buildID, runID)
		return
	}
	scheduleGithubPollRetryDefault(buildID, runID)
}

func clearScheduledGithubPollRetry(buildID uint, runID int64) {
	key := githubPollRetryKey{buildID: buildID, runID: runID}
	scheduledGithubPollRetries.Lock()
	delete(scheduledGithubPollRetries.keys, key)
	scheduledGithubPollRetries.Unlock()
}

func runScheduledGithubPollRetry(buildID uint, runID int64, start func() bool) {
	if !activeGithubPolls.wait(buildID, 2*time.Minute) {
		clearScheduledGithubPollRetry(buildID, runID)
		if global.Logger != nil {
			global.Logger.Warnf("poll retry for build %d/run %d was not started because the active poll did not release", buildID, runID)
		}
		return
	}
	// The scheduled attempt owns the retry lifecycle from this point. Clear the
	// pending marker before starting the poll so a persistence failure from this
	// attempt can schedule one later retry; defer also clears it on failure.
	clearScheduledGithubPollRetry(buildID, runID)
	defer clearScheduledGithubPollRetry(buildID, runID)
	if !start() && global.Logger != nil {
		global.Logger.Debugf("poll retry for build %d/run %d was skipped because another poll is active", buildID, runID)
	}
}

func scheduleGithubPollRetryDefault(buildID uint, runID int64) {
	go func() {
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		<-timer.C
		runScheduledGithubPollRetry(buildID, runID, func() bool {
			return startGithubPoll(&CustomBuild{}, buildID, runID)
		})
	}()
}

// persistBuildMutation is the single bounded persistence/recovery boundary for
// asynchronous build state. It deliberately retries the exact same mutation;
// it never queues a fallback job or invents a provider identity. A caller that
// cannot persist a terminal/done state must leave the row non-done so a later
// resume can retry it.
func persistBuildMutation(buildID uint, operation string, mutation func() error) error {
	var lastErr error
	for attempt := 1; attempt <= buildPersistenceAttempts; attempt++ {
		if err := mutation(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt < buildPersistenceAttempts {
			time.Sleep(buildPersistenceRetryDelay)
		}
	}
	if global.Logger != nil {
		global.Logger.Errorf("build %d: %s persistence exhausted after %d attempts; row remains recoverable and non-done: %s", buildID, operation, buildPersistenceAttempts, service.GithubErrorDetail(lastErr))
	}
	return fmt.Errorf("build %d %s persistence retries exhausted: %w", buildID, operation, lastErr)
}

func persistPollMutation(buildID uint, runID int64, operation string, mutation func() error) error {
	err := persistBuildMutation(buildID, operation, mutation)
	if err != nil {
		schedulePollRetryAfterPersistence(buildID, runID)
	}
	return err
}

func persistBuildProgressWith(updater buildProgressUpdater, b *model.CustomBuild) error {
	return persistBuildProgressWithRun(updater, b, b.GithubRunId)
}

func persistBuildProgressWithRun(updater buildProgressUpdater, b *model.CustomBuild, expectedRunID int64) error {
	return persistPollMutation(b.Id, expectedRunID, "progress", func() error {
		if err := updater(service.BuildProgress{
			BuildID:            b.Id,
			ExpectedRunID:      expectedRunID,
			ExpectedArtifactID: b.GithubArtifactID,
			Status:             b.Status,
			BuildLog:           b.BuildLog,
			FileSize:           b.FileSize,
		}); err != nil {
			var persistenceErr *service.BuildProgressPersistenceError
			if errors.As(err, &persistenceErr) {
				return err
			}
			return &service.BuildProgressPersistenceError{BuildID: b.Id, Cause: err}
		}
		return nil
	})
}

func persistPendingFailureWith(updater buildProgressUpdater, b *model.CustomBuild) error {
	return persistBuildMutation(b.Id, "pending failure", func() error {
		if err := updater(service.BuildProgress{
			BuildID:  b.Id,
			Status:   b.Status,
			BuildLog: b.BuildLog,
			FileSize: b.FileSize,
		}); err != nil {
			var persistenceErr *service.BuildProgressPersistenceError
			if errors.As(err, &persistenceErr) {
				return err
			}
			return &service.BuildProgressPersistenceError{BuildID: b.Id, Cause: err}
		}
		return nil
	})
}

func persistBuildProgress(b *model.CustomBuild, expectedRunID int64) error {
	return persistBuildProgressWithRun(service.AllService.CustomBuildService.UpdateProgress, b, expectedRunID)
}

func recordPublishedOutput(buildID uint, expectedRunID, expectedArtifactID int64) error {
	return recordPublishedOutputWithManifest(buildID, expectedRunID, expectedArtifactID, service.ProducerManifest{})
}

func recordPublishedOutputWithManifest(buildID uint, expectedRunID, expectedArtifactID int64, producerManifest service.ProducerManifest) error {
	return persistPollMutation(buildID, expectedRunID, "publication", func() error {
		var err error
		if producerManifest.SchemaVersion == 0 {
			err = service.AllService.CustomBuildService.RecordPublishedOutput(buildID, expectedRunID, expectedArtifactID)
		} else {
			err = service.AllService.CustomBuildService.RecordPublishedOutput(buildID, expectedRunID, expectedArtifactID, producerManifest)
		}
		if err != nil {
			var persistenceErr *service.BuildProgressPersistenceError
			if errors.As(err, &persistenceErr) {
				return err
			}
			return &service.BuildProgressPersistenceError{BuildID: buildID, Cause: err}
		}
		return nil
	})
}

func persistPendingFailure(b *model.CustomBuild) error {
	return persistPendingFailureWith(service.AllService.CustomBuildService.UpdatePendingFailure, b)
}

func persistNoRunFailure(b *model.CustomBuild) error {
	return persistBuildMutation(b.Id, "no-run legacy failure", func() error {
		if err := service.AllService.CustomBuildService.UpdateNoRunFailure(service.BuildProgress{
			BuildID:  b.Id,
			Status:   b.Status,
			BuildLog: b.BuildLog,
			FileSize: b.FileSize,
		}); err != nil {
			var persistenceErr *service.BuildProgressPersistenceError
			if errors.As(err, &persistenceErr) {
				return err
			}
			return &service.BuildProgressPersistenceError{BuildID: b.Id, Cause: err}
		}
		return nil
	})
}

func persistCompletedBuildWith(updater buildProgressUpdater, b *model.CustomBuild, expectedRunID int64) error {
	if err := persistPollMutation(b.Id, expectedRunID, "completion", func() error {
		if err := updater(service.BuildProgress{
			BuildID:            b.Id,
			ExpectedRunID:      expectedRunID,
			ExpectedArtifactID: b.GithubArtifactID,
			Status:             b.Status,
			BuildLog:           b.BuildLog,
			FileSize:           b.FileSize,
		}); err != nil {
			var persistenceErr *service.BuildProgressPersistenceError
			if errors.As(err, &persistenceErr) {
				return err
			}
			return &service.BuildProgressPersistenceError{BuildID: b.Id, Cause: err}
		}
		return nil
	}); err != nil {
		b.Status = model.CustomBuildStatusBuilding
		return err
	}
	return nil
}

func boundedBuildLog(existing, message string) string {
	return service.BoundBuildLog(existing, message)
}

func failActiveGithubBuild(buildID uint, expectedRunID int64, reason string) {
	b, err := service.AllService.CustomBuildService.Info(buildID)
	if err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("custom build %d read failed while persisting terminal poll state: %s", buildID, service.GithubErrorDetail(err))
		}
		return
	}
	if b.Id == 0 || b.GithubRunId != expectedRunID || !isActiveGithubBuildStatus(b.Status) {
		return
	}
	b.Status = model.CustomBuildStatusFailed
	b.BuildLog = boundedBuildLog(b.BuildLog, reason)
	if persistErr := persistBuildProgress(b, expectedRunID); persistErr != nil && global.Logger != nil {
		global.Logger.Errorf("custom build %d failed to persist terminal poll state: %s", buildID, service.GithubErrorDetail(persistErr))
	}
}

// recoverRecordedPublishedBuild finalizes only an already verified publication
// whose marker and digest were durably stored before completion persistence
// failed. Missing or mismatched proof is deliberately not upgraded to done.
func recoverRecordedPublishedBuild(buildID uint, expectedRunID int64) (bool, error) {
	build, err := service.AllService.CustomBuildService.Info(buildID)
	if err != nil {
		return false, fmt.Errorf("read custom build %d: %w", buildID, err)
	}
	if build.Id == 0 || build.GithubRunId != expectedRunID || !isActiveGithubBuildStatus(build.Status) {
		return false, nil
	}
	if build.GithubArtifactID <= 0 {
		return false, errors.New("stored publication proof has no exact artifact identity")
	}
	size, err := service.AllService.CustomBuildService.ValidateRecordedPublishedOutput(buildID, expectedRunID, build.GithubArtifactID)
	if err != nil {
		return false, err
	}
	build.FileSize = size
	build.Status = model.CustomBuildStatusDone
	build.BuildLog = boundedBuildLog(build.BuildLog, "reused validated final artifact")
	if err := persistCompletedBuildWith(service.AllService.CustomBuildService.UpdateProgress, build, expectedRunID); err != nil {
		return true, err
	}
	return true, nil
}

func cleanupArtifactArchive(buildID uint, archivePath string) {
	if err := removeArtifactArchive(archivePath); err == nil || errors.Is(err, os.ErrNotExist) {
		service.ReleaseGithubArtifactTemp(archivePath)
		return
	} else {
		if global.Logger != nil {
			global.Logger.Errorf("custom build %d failed to remove temporary archive: %s", buildID, service.GithubErrorDetail(err))
		}
		scheduleArtifactCleanupRetry(buildID, archivePath)
	}
}

func scheduleArtifactCleanupRetryDefault(buildID uint, archivePath string) {
	go func() {
		defer service.ReleaseGithubArtifactTemp(archivePath)
		for attempt := 1; attempt <= artifactCleanupRetryAttempts; attempt++ {
			time.Sleep(artifactCleanupRetryDelay)
			err := removeArtifactArchive(archivePath)
			if err == nil || errors.Is(err, os.ErrNotExist) {
				return
			}
			if global.Logger != nil {
				global.Logger.Errorf("custom build %d temporary archive cleanup retry %d/%d failed: %s", buildID, attempt, artifactCleanupRetryAttempts, service.GithubErrorDetail(err))
			}
		}
	}()
}

// defaultWindowsArtifactName — имя GitHub-артефакта, который продюсит
// windows-min-test workflow. Вынесено из inline-строки (BUGS.md AU-L-011).
const defaultWindowsArtifactName = "rustdesk-min-test-windows"

func artifactNameForPlatform(platform string) string {
	switch platform {
	case "linux":
		return "rustdesk-min-test-linux"
	case "android":
		return "rustdesk-min-test-android"
	default:
		return defaultWindowsArtifactName
	}
}

// List returns the paginated redacted custom-build list for administrators.
// @Tags CustomBuild
// @Summary List custom builds
// @Description Admin-only paginated list. The response uses the redacted CustomBuildSafeList view; provider credentials and storage-only identity fields are not serialized.
// @Produce json
// @Param page query int false "Page number"
// @Param page_size query int false "Number of builds per page"
// @Success 200 {object} response.Response{data=model.CustomBuildSafeList} "Redacted custom-build list envelope"
// @Failure 500 {object} response.Response "Custom-build list is unavailable"
// @Router /admin/custom_build/list [get]
// @Security token
func (ct *CustomBuild) List(c *gin.Context) {
	q := &admin.CustomBuildQuery{}
	if err := c.ShouldBindQuery(q); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res, err := service.AllService.CustomBuildService.List(uint(q.Page), uint(q.PageSize))
	if err != nil {
		failCustomBuildRead(c, err)
		return
	}
	response.Success(c, res.Safe())
}

// Versions — список версий RustDesk, доступных для custom-сборки. На ошибке
// GitHub API возвращает 200 + пустой массив, чтобы UI мог показать empty/error
// state, а не 5xx (BUGS.md AU-L-016).
type versionsResponse struct {
	Versions []versionOption `json:"versions"`
	Error    bool            `json:"error"`
}

// versionOption keeps the normal user contract capability-based. Provider
// refs/SHA values stay server-side and are resolved again at the create
// boundary; the UI only receives a display version and safe release metadata.
type versionOption struct {
	Version       string `json:"version"`
	AssetsRelease string `json:"assets_release"`
}

// @Tags CustomBuild
// @Summary List available custom-build versions
// @Description Admin-only provider capability list. Only display versions and release labels are returned; provider refs and commit SHAs remain server-side.
// @Produce json
// @Success 200 {object} response.Response{data=versionsResponse} "Available version options, with error state"
// @Router /admin/custom_build/versions [get]
// @Security token
func (ct *CustomBuild) Versions(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	versions, err := service.AllService.GithubBuildConfigService.GetAvailableVersions(ctx)
	if err != nil {
		global.Logger.Warnf("GetAvailableVersions failed: %s", service.GithubErrorDetail(err))
		response.Success(c, versionsResponse{Versions: []versionOption{}, Error: true})
		return
	}
	options := make([]versionOption, 0, len(versions))
	for _, version := range versions {
		options = append(options, versionOption{
			Version:       version.DisplayVersion,
			AssetsRelease: version.AssetsRelease.TagName,
		})
	}
	response.Success(c, versionsResponse{Versions: options, Error: false})
}

// Detail — admin endpoint: полная запись custom-билда по id.
func (ct *CustomBuild) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	u, err := service.AllService.CustomBuildService.Info(uint(iid))
	if err != nil {
		failCustomBuildRead(c, err)
		return
	}
	if u.Id > 0 {
		response.Success(c, u.Safe())
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// Manifest returns the admin-only, secret-free operator handoff for a
// completed build. The response is canonical JSON data, not a filesystem
// export or a newly generated ZIP container; published_digest covers the
// service-owned canonical output that was already proven before completion.
//
// @Tags CustomBuild
// @Summary Export a completed custom-build handoff manifest
// @Description Admin-only redacted handoff data. The BuildHandoffManifest contains no secrets, raw custom configuration, filesystem paths, or signature claim. The published digest covers stored canonical output and is distinct from the SHA-256 header over these response bytes.
// @Produce json
// @Param id path int true "Completed custom build ID"
// @Success 200 {object} response.Response{data=service.BuildHandoffManifest} "Redacted build handoff envelope"
// @Header 200 {string} X-DeskForge-Manifest-SHA256 "SHA-256 of the exact response body bytes"
// @Failure 400 {object} response.Response "Invalid build ID"
// @Failure 409 {object} response.Response "Completed handoff is unavailable"
// @Failure 500 {object} response.Response "Handoff is unavailable"
// @Router /admin/custom_build/manifest/{id} [get]
// @Security token
func (ct *CustomBuild) Manifest(c *gin.Context) {
	parsedID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || parsedID == 0 {
		failCustomValidation(c, errors.New("invalid custom build id"))
		return
	}
	manifest, err := (&service.BuildManifestService{}).ForBuild(uint(parsedID), time.Now().UTC())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			failCustomBuildRead(c, err)
			return
		}
		if global.Logger != nil {
			global.Logger.Warnf("custom build %d handoff unavailable", parsedID)
		}
		response.FailStatus(c, http.StatusConflict, 101, "custom build handoff is unavailable")
		return
	}
	body, err := json.Marshal(response.Response{Code: 0, Message: "success", Data: manifest})
	if err != nil {
		response.FailStatus(c, http.StatusInternalServerError, 101, "custom build handoff is unavailable")
		return
	}
	digest := sha256.Sum256(body)
	// This header covers the exact redacted JSON response bytes, not the
	// service-owned publication digest and not a signature.
	c.Header("X-DeskForge-Manifest-SHA256", hex.EncodeToString(digest[:]))
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// Create queues an administrator-selected custom build after resolving its
// provider-owned build identity.
// Валидирует version формат ДО персиста (защита от command injection в
// workflow shell — see utils.ValidateBuildVersion).
// @Tags CustomBuild
// @Summary Create a custom build
// @Description Admin-only build request. The display version and platform are user-selected capabilities; provider workflow refs, release identity, and dispatch details are resolved server-side.
// @Accept json
// @Produce json
// @Param body body admin.CustomBuildForm true "Custom-build request"
// @Success 200 {object} response.Response{data=model.CustomBuildSafe} "Redacted queued custom-build envelope"
// @Failure 400 {object} response.Response "Invalid custom-build request"
// @Failure 412 {object} response.Response "Workflow reference approval is required"
// @Failure 500 {object} response.Response "Custom build could not be queued"
// @Failure 503 {object} response.Response "Build provider or encryption configuration is unavailable"
// @Router /admin/custom_build/create [post]
// @Security token
func (ct *CustomBuild) Create(c *gin.Context) {
	f := &admin.CustomBuildForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.BuildRef != "" {
		failCustomValidation(c, fmt.Errorf("build_ref is system-derived and cannot be supplied"))
		return
	}
	if !validateCustomPlatform(c, f.Platform) {
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}

	user := service.AllService.UserService.CurUser(c)
	b := f.ToCustomBuild()
	b.UserId = user.Id
	b.Status = model.CustomBuildStatusPending
	b.DownloadKey = utils.RandomString(32)
	// BUGS.md B-006: capability-ссылка должна протухать. TTL из конфига,
	// дефолт 7 дней если не задан/невалиден.
	ttl := global.Config.App.DownloadKeyTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	b.DownloadKeyExpiresAt = time.Now().Add(ttl).Unix()

	// Reject unsafe version early; keeps DB clean and gives the caller a clear error.
	if !utils.ValidateBuildVersion(b.Version) {
		failCustomValidation(c, fmt.Errorf("invalid version format: %s", b.Version))
		return
	}
	if err := utils.RequireSecretEncryptionForCustomBuilderJSON(b.CustomJson); err != nil {
		if failCustomServiceError(c, err) {
			return
		}
		response.FailStatus(c, http.StatusServiceUnavailable, 101, "secret encryption is not configured")
		return
	}
	if err := service.RequireConfiguredPublicKey(); err != nil {
		var providerErr *service.GithubProviderConfigurationError
		if errors.As(err, &providerErr) {
			response.FailStatus(c, http.StatusServiceUnavailable, 101, "GitHub build provider is not configured")
			return
		}
		response.FailStatus(c, http.StatusServiceUnavailable, 101, "configured public key is unavailable")
		return
	}

	prepareCtx, prepareCancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	prepared, err := service.AllService.GithubBuildConfigService.PrepareBuild(prepareCtx, b.Platform, b.Version)
	prepareCancel()
	if err != nil {
		var capabilityErr *service.ProductionCapabilityUnavailableError
		if errors.As(err, &capabilityErr) {
			failGithubConfigError(c, err)
			return
		}
		var providerErr *service.GithubProviderConfigurationError
		if errors.As(err, &providerErr) {
			failGithubConfigError(c, err)
			return
		}
		var approvalErr *service.WorkflowRefApprovalError
		if errors.As(err, &approvalErr) {
			response.FailStatus(c, http.StatusPreconditionFailed, 101, "workflow reference approval is required")
			return
		}
		failGithubConfigError(c, err)
		return
	}

	normalized, err := service.AllService.CustomBuildService.CreateNormalizedWithIdentity(b, prepared.Identity)
	if err != nil {
		if failCustomServiceError(c, err) {
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}

	if err := ct.submitBuild(b, prepared.Config, normalized.DispatchParams); err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		response.Fail(c, 101, "GitHub workflow dispatch failed")
		return
	}

	response.Success(c, b.Safe())
}

// Delete removes an administrator-owned custom-build record and its artifacts.
// @Tags CustomBuild
// @Summary Delete a custom build
// @Description Admin-only deletion of a custom-build record and its published artifacts.
// @Accept json
// @Produce json
// @Param body body admin.CustomBuildForm true "Custom-build identifier"
// @Success 200 {object} response.Response "Custom build deleted"
// @Failure 500 {object} response.Response "Custom build could not be deleted"
// @Router /admin/custom_build/delete [post]
// @Security token
func (ct *CustomBuild) Delete(c *gin.Context) {
	f := &admin.CustomBuildForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	ex, err := service.AllService.CustomBuildService.Info(f.Id)
	if err != nil {
		failCustomBuildRead(c, err)
		return
	}
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	release, err := activeGithubPolls.acquire(c.Request.Context(), ex.Id)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	defer release()
	// The row may have been deleted or changed while Delete waited for the
	// in-flight poll to release the same process-local lifecycle guard.
	ex, err = service.AllService.CustomBuildService.Info(f.Id)
	if err != nil {
		failCustomBuildRead(c, err)
		return
	}
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err = service.AllService.CustomBuildService.Delete(ex)
	if err != nil {
		var cleanupPending *service.CustomBuildCleanupPending
		if errors.As(err, &cleanupPending) {
			if global.Logger != nil {
				global.Logger.Warnf("custom build %d deleted from database; artifact cleanup remains pending: %s", ex.Id, service.GithubErrorDetail(err))
			}
			// Preserve HTTP/UI deletion success after the irreversible DB delete,
			// but expose the filesystem warning without requiring a new API shape.
			response.Success(c, gin.H{"cleanup_pending": true})
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// findBuildByDownloadKey — единая точка валидации capability-ключа для всех
// публичных эндпоинтов (DetailByKey/DownloadByKey). Проверяет и существование,
// и срок жизни (BUGS.md B-006), чтобы протухание нельзя было забыть проверить
// в одном из обработчиков. Возвращает (build, httpStatus, ok), где httpStatus =
// 404 для ненайденного ключа, 410 для протухшего, 200 для валидного.
func findBuildByDownloadKey(key string) (*model.CustomBuild, int, bool) {
	var build model.CustomBuild
	if err := global.DB.Where("download_key = ?", key).First(&build).Error; err != nil {
		return nil, 404, false
	}
	if build.DownloadKeyExpiresAt > 0 && time.Now().Unix() > build.DownloadKeyExpiresAt {
		return nil, 410, false
	}
	return &build, 200, true
}

// DetailByKey returns a redacted build view through a public capability URL.
// No authentication is required; the opaque key is the capability and is
// subject to the configured download-key-ttl (default seven days for newly
// created builds). Expired keys return HTTP 410.
//
// @Tags CustomBuild
// @Summary Read a completed custom build by capability key
// @Description Public capability route with no authentication. The key expires according to download-key-ttl; an expired key returns 410. The response uses the public CustomBuildPublic view and omits download_key, provider refs, and secrets.
// @Produce json
// @Param key path string true "Opaque build capability key"
// @Success 200 {object} response.Response{data=model.CustomBuildPublic} "Public build detail envelope without download_key"
// @Failure 409 {object} response.Response "Build is not publicly ready"
// @Failure 410 {object} response.Response "Capability key has expired"
// @Router /admin/custom_build/public/detailByKey/{key} [get]
func (ct *CustomBuild) DetailByKey(c *gin.Context) {
	key := c.Param("key")
	build, status, ok := findBuildByDownloadKey(key)
	if !ok {
		if status == 410 {
			c.JSON(410, gin.H{"code": 410, "message": "download link has expired"})
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if _, _, err := service.ValidateCompletedPublishedOutput(build); err != nil {
		if global.Logger != nil {
			global.Logger.Warnf("DetailByKey: build %d is not publicly ready: %s", build.Id, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusConflict, gin.H{
			"code":    http.StatusConflict,
			"message": "build is not publicly ready",
		})
		return
	}
	response.Success(c, build.Public())
}

// DownloadByKey is the public capability URL. The capability key remains the
// only credential accepted by this unauthenticated route.
//
// @Tags CustomBuild
// @Summary Download a completed custom build by capability key
// @Description Public capability route with no authentication. The key expires according to download-key-ttl; an expired key returns 410. The response is the exact redacted ZIP bytes served by this response, not the stored publication output digest and not a signature.
// @Produce application/zip
// @Param key path string true "Opaque build capability key"
// @Success 200 {file} file "Redacted custom-build ZIP archive"
// @Header 200 {string} Content-Length "Exact ZIP response length in bytes"
// @Header 200 {string} Content-Disposition "Attachment filename"
// @Header 200 {string} X-DeskForge-Archive-SHA256 "SHA-256 of the exact ZIP bytes served by this response"
// @Header 200 {string} X-DeskForge-Archive-SHA256-Scope "Digest scope; distinct from the stored publication digest"
// @Failure 408 {object} response.Response "Download request was cancelled or timed out"
// @Failure 409 {object} response.Response "Build completion or output provenance is unavailable"
// @Failure 410 {object} response.Response "Capability key has expired"
// @Failure 416 {object} response.Response "Range requests are not supported"
// @Failure 500 {object} response.Response "ZIP packaging or validation failed"
// @Router /admin/custom_build/public/download/{key} [get]
func (ct *CustomBuild) DownloadByKey(c *gin.Context) {
	// Preserve the public route's fail-closed Range behavior even when the key
	// is invalid; no lookup or response headers are needed for a rejected range.
	if rejectDownloadRange(c) {
		return
	}
	key := c.Param("key")
	build, status, ok := findBuildByDownloadKey(key)
	if !ok {
		if status == 410 {
			c.JSON(410, gin.H{"code": 410, "message": "download link has expired"})
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	ct.downloadCompletedBuild(c, build, "DownloadByKey")
}

// DownloadByID streams a completed build to an authenticated administrator.
// Unlike the public capability route, it resolves the artifact by the
// administrator-selected build ID and never serializes or accepts DownloadKey.
//
// @Tags CustomBuild
// @Summary Download a completed custom build by ID
// @Description Admin-only download by build ID. BackendUserAuth and AdminPrivilege authorize the request; the build must have a complete immutable publication proof. The response is the exact redacted ZIP bytes served by this response, not the stored publication output digest and not a signature.
// @Produce application/zip
// @Param id path int true "Completed custom build ID"
// @Success 200 {file} file "Redacted custom-build ZIP archive"
// @Header 200 {string} Content-Length "Exact ZIP response length in bytes"
// @Header 200 {string} Content-Disposition "Attachment filename"
// @Header 200 {string} X-DeskForge-Archive-SHA256 "SHA-256 of the exact ZIP bytes served by this response"
// @Header 200 {string} X-DeskForge-Archive-SHA256-Scope "Digest scope; distinct from the stored publication digest"
// @Failure 400 {object} response.Response "Invalid build ID"
// @Failure 403 {object} response.Response "Administrator authentication or privilege is required"
// @Failure 408 {object} response.Response "Download request was cancelled or timed out"
// @Failure 409 {object} response.Response "Build completion or output provenance is unavailable"
// @Failure 416 {object} response.Response "Range requests are not supported"
// @Failure 500 {object} response.Response "ZIP packaging or validation failed"
// @Router /admin/custom_build/download/{id} [get]
// @Security token
func (ct *CustomBuild) DownloadByID(c *gin.Context) {
	parsedID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || parsedID == 0 {
		failCustomValidation(c, errors.New("invalid custom build id"))
		return
	}
	build, err := service.AllService.CustomBuildService.Info(uint(parsedID))
	if err != nil {
		failCustomBuildRead(c, err)
		return
	}
	if build.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	ct.downloadCompletedBuild(c, build, "DownloadByID")
}

func rejectDownloadRange(c *gin.Context) bool {
	if c.Request.Header.Get("Range") == "" {
		return false
	}
	response.FailStatus(c, http.StatusRequestedRangeNotSatisfiable, http.StatusRequestedRangeNotSatisfiable, "range requests are not supported")
	return true
}

// downloadCompletedBuild is the shared authenticated/capability archive
// pipeline. It validates publication state, snapshots and packages the exact
// redacted output, rechecks the stored digest before opening the archive, and
// only then commits download headers. Keep both routes on this path so their
// TOCTOU, range, archive digest, and secret-redaction behavior cannot diverge.
func (ct *CustomBuild) downloadCompletedBuild(c *gin.Context, build *model.CustomBuild, operation string) {
	if rejectDownloadRange(c) {
		return
	}
	if _, _, err := service.ValidateCompletedPublishedOutput(build); err != nil {
		if global.Logger != nil {
			global.Logger.Warnf("%s: build %d has invalid completion provenance: %s", operation, build.Id, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusConflict, gin.H{
			"code":    http.StatusConflict,
			"message": "build completion provenance is unavailable",
		})
		return
	}

	select {
	case downloadArchiveSlots <- struct{}{}:
		defer func() { <-downloadArchiveSlots }()
	case <-c.Request.Context().Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"code": http.StatusRequestTimeout, "message": "download request cancelled"})
		return
	}
	if _, err := service.ValidatePublishedOutputDigest(build); err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("%s: published output is not immutable for build %d: %s", operation, build.Id, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "published build artifact is no longer valid",
		})
		return
	}
	archivePath, archiveSize, err := buildDownloadArchive(c.Request.Context(), build)
	if err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("%s: package build %d: %s", operation, build.Id, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "failed to package build artifacts",
		})
		return
	}
	defer cleanupArtifactArchive(build.Id, archivePath)
	// Recompute again after packaging and immediately before opening the
	// archive. A mutation between the initial guard and packaging fails closed
	// without committing response headers.
	if _, err := service.ValidatePublishedOutputDigest(build); err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("%s: published output changed for build %d: %s", operation, build.Id, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "published build artifact is no longer valid",
		})
		return
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("%s: open completed archive: %s", operation, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "failed to open build archive",
		})
		return
	}
	archiveSHA, err := hashDownloadArchive(archive, archiveSize)
	if err != nil {
		_ = archive.Close()
		if global.Logger != nil {
			global.Logger.Errorf("%s: hash completed archive for build %d: %s", operation, build.Id, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "failed to verify build archive",
		})
		return
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		_ = archive.Close()
		if global.Logger != nil {
			global.Logger.Errorf("%s: rewind completed archive for build %d: %s", operation, build.Id, service.GithubErrorDetail(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "failed to prepare build archive",
		})
		return
	}

	appName := build.AppName
	if err := service.ValidateOutputAppName(appName); err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("%s: invalid output app name for build %d: %s", operation, build.Id, service.GithubErrorDetail(err))
		}
		_ = archive.Close()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    http.StatusInternalServerError,
			"message": "invalid build output name",
		})
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`,
		appName, time.Now().Format("20060102-150405")))
	c.Header("Content-Length", strconv.FormatInt(archiveSize, 10))
	// This digest covers the exact ZIP bytes written below, not the stored
	// publication digest and not a signature. The scope header makes that
	// distinction explicit to clients.
	c.Header("X-DeskForge-Archive-SHA256", archiveSHA)
	c.Header("X-DeskForge-Archive-SHA256-Scope", downloadArchiveDigestScope)
	http.ServeContent(c.Writer, c.Request, filepath.Base(archivePath), time.Now(), archive)
	if err := archive.Close(); err != nil && global.Logger != nil {
		global.Logger.Warnf("%s: close served archive: %s", operation, service.GithubErrorDetail(err))
	}
}

func buildDownloadArchive(ctx context.Context, build *model.CustomBuild) (archivePath string, archiveSize int64, err error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	if build == nil {
		return "", 0, errors.New("build record is required")
	}
	return archivePath, archiveSize, service.WithPublishedOutputExportLock(func() error {
		dir := service.BuildOutputDir(build.Id)
		beforeDigest, digestErr := service.PublishedOutputDigest(build)
		if digestErr != nil {
			return fmt.Errorf("validate published output before packaging: %w", digestErr)
		}
		if build.PublishedDigest != "" && !strings.EqualFold(build.PublishedDigest, beforeDigest) {
			return errors.New("published output digest does not match recorded digest")
		}
		snapshotDir, files, snapshotErr := collectDownloadSnapshot(ctx, dir, build.Id)
		if snapshotErr != nil {
			return snapshotErr
		}
		defer cleanupDownloadSnapshot(snapshotDir)
		downloadArchiveSnapshotHook()

		buildID := build.Id
		parent := filepath.Dir(dir)
		part, createErr := os.CreateTemp(parent, fmt.Sprintf(".%d-download-*.zip.part", buildID))
		if createErr != nil {
			return fmt.Errorf("create temporary download archive: %w", createErr)
		}
		partPath := part.Name()
		archivePath = strings.TrimSuffix(partPath, ".part")
		service.ProtectGithubArtifactTemp(partPath)
		service.ProtectGithubArtifactTemp(archivePath)
		keepArchive := false
		defer func() {
			if !keepArchive {
				_ = os.Remove(partPath)
				_ = os.Remove(archivePath)
				service.ReleaseGithubArtifactTemp(partPath)
				service.ReleaseGithubArtifactTemp(archivePath)
			}
		}()

		limited := &downloadArchiveLimitWriter{writer: part, limit: downloadArchiveMaxOutputBytes}
		zw := zip.NewWriter(limited)
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				_ = part.Close()
				return err
			}
			input, openErr := os.Open(file.path)
			if openErr != nil {
				_ = part.Close()
				return fmt.Errorf("open build snapshot %q: %w", file.name, openErr)
			}
			output, createErr := zw.Create(file.name)
			if createErr != nil {
				_ = input.Close()
				_ = part.Close()
				return fmt.Errorf("create ZIP entry %q: %w", file.name, createErr)
			}
			hasher := sha256.New()
			if _, copyErr := copyDownloadSourceLimit(ctx, io.MultiWriter(output, hasher), input, file.size); copyErr != nil {
				_ = input.Close()
				_ = part.Close()
				return fmt.Errorf("copy build snapshot %q: %w", file.name, copyErr)
			}
			if hex.EncodeToString(hasher.Sum(nil)) != file.sha {
				_ = input.Close()
				_ = part.Close()
				return fmt.Errorf("build snapshot %q changed during packaging", file.name)
			}
			if closeErr := input.Close(); closeErr != nil {
				_ = part.Close()
				return fmt.Errorf("close build snapshot %q: %w", file.name, closeErr)
			}
		}
		if err := zw.Close(); err != nil {
			_ = part.Close()
			return fmt.Errorf("close ZIP archive: %w", err)
		}
		if err := part.Sync(); err != nil {
			_ = part.Close()
			return fmt.Errorf("sync ZIP archive: %w", err)
		}
		if err := part.Close(); err != nil {
			return fmt.Errorf("close temporary ZIP archive: %w", err)
		}
		if limited.written > downloadArchiveMaxOutputBytes {
			return fmt.Errorf("completed ZIP exceeds output limit")
		}
		if err := validateDownloadArchive(ctx, partPath); err != nil {
			return err
		}
		if err := os.Rename(partPath, archivePath); err != nil {
			return fmt.Errorf("publish completed ZIP archive: %w", err)
		}
		service.ReleaseGithubArtifactTemp(partPath)
		archiveInfo, statErr := os.Stat(archivePath)
		if statErr != nil {
			return fmt.Errorf("stat completed ZIP archive: %w", statErr)
		}
		afterDigest, digestErr := service.PublishedOutputDigest(build)
		if digestErr != nil {
			return fmt.Errorf("recheck published output after packaging: %w", digestErr)
		}
		if beforeDigest != afterDigest || (build.PublishedDigest != "" && !strings.EqualFold(build.PublishedDigest, afterDigest)) {
			return errors.New("published output changed during archive packaging")
		}
		keepArchive = true
		archiveSize = archiveInfo.Size()
		return nil
	})
}

type downloadSnapshotFile struct {
	name string
	path string
	size int64
	sha  string
}

func cleanupDownloadSnapshot(snapshotDir string) {
	if snapshotDir == "" {
		return
	}
	_ = os.RemoveAll(snapshotDir)
	service.ReleaseBuildOutputSnapshot(snapshotDir)
}

func collectDownloadSnapshot(ctx context.Context, dir string, buildID uint) (string, []downloadSnapshotFile, error) {
	if downloadArchiveMaxFiles <= 0 || downloadArchiveMaxFileBytes <= 0 || downloadArchiveMaxSourceBytes <= 0 {
		return "", nil, errors.New("download archive source limits are invalid")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf("read build output directory: %w", err)
	}
	if len(entries) > downloadArchiveMaxFiles {
		return "", nil, fmt.Errorf("build output contains too many files: %d", len(entries))
	}
	parent := filepath.Dir(dir)
	snapshotDir, err := os.MkdirTemp(parent, fmt.Sprintf(".%d-snapshot-*", buildID))
	if err != nil {
		return "", nil, fmt.Errorf("create output snapshot: %w", err)
	}
	if err := os.Chmod(snapshotDir, 0700); err != nil {
		_ = os.RemoveAll(snapshotDir)
		return "", nil, fmt.Errorf("restrict output snapshot: %w", err)
	}
	service.ProtectBuildOutputSnapshot(snapshotDir)
	keep := false
	defer func() {
		if !keep {
			cleanupDownloadSnapshot(snapshotDir)
		}
	}()
	files := make([]downloadSnapshotFile, 0, len(entries))
	publicCustomTxtAdded := false
	var sourceBytes int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		full := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(full)
		if err != nil {
			return "", nil, fmt.Errorf("inspect build artifact %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", nil, fmt.Errorf("build artifact %q is not a regular file", entry.Name())
		}
		if info.Size() < 0 || info.Size() > downloadArchiveMaxFileBytes {
			return "", nil, fmt.Errorf("build artifact %q exceeds uncompressed limit", entry.Name())
		}
		if sourceBytes > downloadArchiveMaxSourceBytes-info.Size() {
			return "", nil, errors.New("build artifact source exceeds aggregate uncompressed limit")
		}
		isCustomTxt := strings.EqualFold(entry.Name(), "custom_.txt")
		if isCustomTxt && info.Size() > service.MaxPublicCustomTxtBytes {
			// Never fall back to copying an oversized private custom_.txt.
			// The client remains valid through its compiled L1 settings.
			continue
		}
		input, err := os.Open(full)
		if err != nil {
			return "", nil, fmt.Errorf("open build artifact %q: %w", entry.Name(), err)
		}
		snapshotName := entry.Name()
		var publicContents []byte
		if isCustomTxt {
			raw, readErr := io.ReadAll(io.LimitReader(input, service.MaxPublicCustomTxtBytes+1))
			inputCloseErr := input.Close()
			if readErr != nil {
				return "", nil, fmt.Errorf("read build artifact %q: %w", entry.Name(), readErr)
			}
			if inputCloseErr != nil {
				return "", nil, fmt.Errorf("close build artifact %q: %w", entry.Name(), inputCloseErr)
			}
			if int64(len(raw)) != info.Size() {
				return "", nil, fmt.Errorf("build artifact %q changed during snapshot", entry.Name())
			}
			publicEncoded, transformErr := service.PublicCustomTxt(string(raw))
			if transformErr != nil {
				// Malformed, unknown, or secret-like native content is omitted;
				// it is never copied as an opaque public file.
				continue
			}
			if publicCustomTxtAdded {
				continue
			}
			publicCustomTxtAdded = true
			snapshotName = "custom_.txt"
			publicContents = []byte(publicEncoded)
		}
		snapshotPath := filepath.Join(snapshotDir, snapshotName)
		output, err := os.OpenFile(snapshotPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			_ = input.Close()
			return "", nil, fmt.Errorf("create build snapshot %q: %w", entry.Name(), err)
		}
		hasher := sha256.New()
		var copied int64
		var copyErr error
		if publicContents != nil {
			copied, copyErr = copyDownloadSourceLimit(ctx, io.MultiWriter(output, hasher), strings.NewReader(string(publicContents)), int64(len(publicContents)))
		} else {
			copyLimit := info.Size()
			if copyLimit == 0 {
				copyLimit = 1
			}
			copied, copyErr = copyDownloadSourceLimit(ctx, io.MultiWriter(output, hasher), input, copyLimit)
		}
		inputCloseErr := error(nil)
		if publicContents == nil {
			inputCloseErr = input.Close()
		}
		outputCloseErr := output.Close()
		if copyErr != nil {
			return "", nil, fmt.Errorf("snapshot build artifact %q: %w", entry.Name(), copyErr)
		}
		expectedSnapshotSize := info.Size()
		if publicContents != nil {
			expectedSnapshotSize = int64(len(publicContents))
		}
		if inputCloseErr != nil || outputCloseErr != nil || copied != expectedSnapshotSize {
			return "", nil, fmt.Errorf("build artifact %q changed during snapshot", entry.Name())
		}
		sourceBytes += copied
		files = append(files, downloadSnapshotFile{
			name: snapshotName,
			path: snapshotPath,
			size: copied,
			sha:  hex.EncodeToString(hasher.Sum(nil)),
		})
	}
	keep = true
	return snapshotDir, files, nil
}

func hashDownloadArchive(archive *os.File, archiveSize int64) (string, error) {
	if archiveSize < 0 || archiveSize > downloadArchiveMaxOutputBytes {
		return "", errors.New("completed ZIP exceeds output limit")
	}
	info, err := archive.Stat()
	if err != nil {
		return "", fmt.Errorf("stat completed ZIP archive: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != archiveSize {
		return "", errors.New("completed ZIP archive changed before serving")
	}
	hasher := sha256.New()
	copied, err := io.Copy(hasher, io.LimitReader(archive, archiveSize+1))
	if err != nil {
		return "", fmt.Errorf("hash completed ZIP archive: %w", err)
	}
	if copied != archiveSize {
		return "", errors.New("completed ZIP archive changed while hashing")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type downloadArchiveLimitWriter struct {
	writer  io.Writer
	limit   int64
	written int64
}

func (w *downloadArchiveLimitWriter) Write(p []byte) (int, error) {
	if w.limit <= 0 || int64(len(p)) > w.limit-w.written {
		return 0, errors.New("download archive output exceeds limit")
	}
	n, err := w.writer.Write(p)
	w.written += int64(n)
	return n, err
}

func copyDownloadSource(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	return copyDownloadSourceLimit(ctx, dst, src, 0)
}

func copyDownloadSourceLimit(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	emptyReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if limit > 0 && written > limit-int64(n) {
				return written, fmt.Errorf("source exceeds uncompressed limit of %d bytes", limit)
			}
			emptyReads = 0
			if err := ctx.Err(); err != nil {
				return written, err
			}
			writtenNow, writeErr := dst.Write(buf[:n])
			written += int64(writtenNow)
			if writeErr != nil {
				return written, writeErr
			}
			if writtenNow != n {
				return written, io.ErrShortWrite
			}
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return written, io.ErrNoProgress
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func validateDownloadArchive(ctx context.Context, archivePath string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("validate ZIP archive: %w", err)
	}
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		input, openErr := entry.Open()
		if openErr != nil {
			_ = reader.Close()
			return fmt.Errorf("open ZIP entry %q during validation: %w", entry.Name, openErr)
		}
		_, copyErr := copyDownloadSource(ctx, io.Discard, input)
		closeErr := input.Close()
		if copyErr != nil {
			_ = reader.Close()
			return fmt.Errorf("validate ZIP entry %q: %w", entry.Name, copyErr)
		}
		if closeErr != nil {
			_ = reader.Close()
			return fmt.Errorf("close ZIP entry %q during validation: %w", entry.Name, closeErr)
		}
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close ZIP validation reader: %w", err)
	}
	return nil
}

// submitBuild sends every production custom build to the configured GitHub
// provider. Frozen manual builders are intentionally outside this API path.
func (ct *CustomBuild) submitBuild(b *model.CustomBuild, config service.GithubBuildConfigSnapshot, dispatchParams map[string]any) error {
	if err := service.RequireProductionBuildCapability(b.Platform); err != nil {
		return err
	}
	return ct.tryGithubDispatch(b, config, dispatchParams)
}

// tryGithubDispatch dispatches through the fixed platform workflow. The
// production API has no local file-queue fallback.
func (ct *CustomBuild) tryGithubDispatch(b *model.CustomBuild, config service.GithubBuildConfigSnapshot, dispatchParams map[string]any) error {
	if err := service.RequireProductionBuildCapability(b.Platform); err != nil {
		return err
	}
	if config.Token == "" || config.Repo == "" || config.PayloadKey == "" {
		providerErr := &service.GithubProviderConfigurationError{Cause: errors.New("GitHub build provider is not configured")}
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", service.GithubErrorDetail(providerErr))
		if persistErr := persistPendingFailure(b); persistErr != nil {
			return persistErr
		}
		return providerErr
	}
	identity, err := service.VersionIdentityFromRecord(b)
	if err != nil {
		global.Logger.Errorf("tryGithubDispatch: build %d has no immutable version identity: %s", b.Id, service.GithubErrorDetail(err))
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", "immutable GitHub build identity is unavailable")
		if persistErr := persistPendingFailure(b); persistErr != nil {
			return persistErr
		}
		return err
	}
	if identity.Repo != config.Repo {
		err := fmt.Errorf("stored version identity repo %q does not match configured repo", identity.Repo)
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", "immutable GitHub build identity mismatch")
		if persistErr := persistPendingFailure(b); persistErr != nil {
			return persistErr
		}
		return err
	}
	// Validate version format to prevent command injection in workflow shell commands.
	// b.Version попадает в env VERSION через безопасную запись workflow env-файла
	// и в download URL (`offline-assets-${VERSION}/...`) — shell metacharacters опасны.
	if !utils.ValidateBuildVersion(b.Version) {
		global.Logger.Warnf("tryGithubDispatch: invalid version format %q for build %d — failing build",
			b.Version, b.Id)
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", "invalid version format: "+b.Version)
		if persistErr := persistPendingFailure(b); persistErr != nil {
			return persistErr
		}
		return nil
	}
	if err := service.ValidateBuildRecordContext(service.BuildRecordContext{
		BuildID:  b.Id,
		Platform: b.Platform,
		AppName:  b.AppName,
		Version:  b.Version,
	}); err != nil {
		global.Logger.Warnf("invalid build context for build %d: %s", b.Id, service.GithubErrorDetail(err))
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", "invalid build context")
		if persistErr := persistPendingFailure(b); persistErr != nil {
			return persistErr
		}
		return nil
	}
	workflow, err := service.WorkflowFilenameForPlatform(b.Platform)
	if err != nil {
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", "workflow mapping is unavailable")
		if persistErr := persistPendingFailure(b); persistErr != nil {
			return persistErr
		}
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// The create boundary already parsed/validated once and returned the
	// dispatch values. Do not reparse the now-L2-only persisted custom_json;
	// DispatchBuild owns immutable version propagation.
	dispatch, err := service.AllService.GithubBuildConfigService.DispatchBuild(ctx, config.ProviderConfig(), identity, b.Platform, dispatchParams)
	if err != nil {
		global.Logger.Errorf("github dispatch failed for build %d: %s", b.Id, service.GithubErrorDetail(err))
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", "github dispatch error: "+service.GithubSafeErrorMessage(err))
		if persistErr := persistPendingFailure(b); persistErr != nil {
			return persistErr
		}
		return err
	}
	provenance := service.BuildProvenance{
		Version:             identity.DisplayVersion,
		BuildRef:            identity.BuildRef,
		SourceTag:           identity.SourceTag,
		AssetsRelease:       identity.AssetsRelease.TagName,
		AssetsReleaseID:     identity.AssetsRelease.ID,
		GithubProvider:      "github",
		GithubRepo:          config.Repo,
		GithubWorkflow:      workflow,
		WorkflowRef:         identity.WorkflowRef,
		WorkflowSHA:         identity.WorkflowSHA,
		GithubRef:           identity.WorkflowSHA,
		GithubArtifactName:  artifactNameForPlatform(b.Platform),
		GithubRunID:         dispatch.WorkflowRunID,
		GithubRunURL:        dispatch.RunURL,
		GithubHTMLURL:       dispatch.HTMLURL,
		AssetsReleaseAssets: identity.AssetsRelease.Assets,
	}
	if err := persistBuildMutation(b.Id, "dispatch provenance", func() error {
		return service.AllService.CustomBuildService.SetProvenance(b.Id, provenance)
	}); err != nil {
		// GitHub has already accepted this exact run, but no durable identity was
		// recorded. Polling must not start and this request must not report success.
		// A distributed outbox or transactional provider cancellation is impossible
		// here and remains an explicit later lifecycle limitation; never guess a run.
		providerErr := fmt.Errorf("github dispatch run %d was accepted but provenance persistence failed; polling was not started: %w", dispatch.WorkflowRunID, err)
		global.Logger.Errorf("tryGithubDispatch: %s", service.GithubErrorDetail(providerErr))
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog("", service.GithubErrorDetail(providerErr))
		if terminalErr := persistPendingFailure(b); terminalErr != nil {
			global.Logger.Errorf("tryGithubDispatch: terminal failure status was not durable for build %d; provider run remains an orphan requiring manual recovery: %s", b.Id, service.GithubErrorDetail(terminalErr))
		}
		return providerErr
	}
	b.Status = model.CustomBuildStatusBuilding
	b.GithubRunId = dispatch.WorkflowRunID
	b.GithubProvider = provenance.GithubProvider
	b.GithubRepo = provenance.GithubRepo
	b.GithubWorkflow = provenance.GithubWorkflow
	b.GithubRef = provenance.GithubRef
	b.GithubArtifactName = provenance.GithubArtifactName
	b.GithubRunUrl = provenance.GithubRunURL
	b.GithubHtmlUrl = provenance.GithubHTMLURL
	b.BuildLog = boundedBuildLog("", fmt.Sprintf("github run id: %d", dispatch.WorkflowRunID))

	// Поллинг в фоне. Используем независимый context (запрос уйдёт раньше, чем сборка).
	startGithubPoll(ct, b.Id, dispatch.WorkflowRunID)
	return nil
}

// ResumePendingPolls — стартап-хук. Находит все незавершённые GitHub-билды,
// включая transient downloading/extracting states, и перезапускает
// pollAndDownload только для строк с полной immutable provenance.
// Incomplete rows are closed only when the stored run guard is available;
// malformed rows without a run remain untouched rather than bypassing it.
//
// Должен вызываться один раз из cmd/apimain.go ПОСЛЕ AutoMigrate.
func ResumePendingPolls() {
	defer func() {
		if r := recover(); r != nil {
			global.Logger.Errorf("ResumePendingPolls panic: %s", service.GithubErrorDetail(fmt.Errorf("background poll panic: %v", r)))
		}
	}()
	ct := &CustomBuild{}
	var builds []*model.CustomBuild
	if err := global.DB.Where("status IN ?", []string{
		model.CustomBuildStatusBuilding,
		model.CustomBuildStatusDownloading,
		model.CustomBuildStatusExtracting,
	}).
		Find(&builds).Error; err != nil {
		global.Logger.Warnf("ResumePendingPolls: query failed: %s", service.GithubErrorDetail(err))
		return
	}
	activeBuildIDs := make(map[uint]struct{}, len(builds))
	for _, b := range builds {
		activeBuildIDs[b.Id] = struct{}{}
	}
	outputRoot := filepath.Dir(customBuildOutputDir(0))
	if err := service.SweepBuildOutputTemps(outputRoot, time.Now(), buildOutputTempTTL, activeBuildIDs); err != nil {
		global.Logger.Warnf("ResumePendingPolls: stale build output cleanup failed: %s", service.GithubErrorDetail(err))
	}
	if err := service.SweepGithubArtifactTemps(os.TempDir(), time.Now(), buildOutputTempTTL); err != nil {
		global.Logger.Warnf("ResumePendingPolls: stale GitHub artifact temp cleanup failed: %s", service.GithubErrorDetail(err))
	}
	for _, b := range builds {
		provenance, err := service.BuildProvenanceFromRecord(b)
		if err != nil {
			reason := "building GitHub build has incomplete immutable provenance; refusing to resume: " + service.GithubErrorDetail(err)
			if b.GithubRunId == 0 {
				reason = "legacy/partial GitHub build has no provider run id; refusing to resume: " + service.GithubErrorDetail(err)
			}
			global.Logger.Errorf("ResumePendingPolls: build %d failed closed: %s", b.Id, reason)
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, reason)
			persistFailure := persistBuildProgress
			if b.GithubRunId == 0 {
				persistFailure = func(build *model.CustomBuild, _ int64) error { return persistNoRunFailure(build) }
			}
			if persistErr := persistFailure(b, b.GithubRunId); persistErr != nil {
				global.Logger.Errorf("ResumePendingPolls: failed to persist closed state for build %d: %s", b.Id, service.GithubErrorDetail(persistErr))
			}
			continue
		}
		if provenance.GithubRunID != b.GithubRunId {
			global.Logger.Errorf("ResumePendingPolls: build %d run identity mismatch; refusing to resume", b.Id)
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, "immutable GitHub provenance run identity mismatch; refusing to resume")
			if persistErr := persistBuildProgress(b, b.GithubRunId); persistErr != nil {
				global.Logger.Errorf("ResumePendingPolls: failed to persist mismatch state for build %d: %s", b.Id, service.GithubErrorDetail(persistErr))
			}
			continue
		}
		global.Logger.Infof("ResumePendingPolls: resuming build %d (run %d)", b.Id, b.GithubRunId)
		startGithubPoll(ct, b.Id, b.GithubRunId)
	}
}

// pollAndDownload — асинхронно опрашивает статус рана GitHub до завершения,
// при успехе скачивает артефакт rustdesk-min-test-windows.zip, кладёт exe в
// /rdgen-data/output/{buildId}/{appname}.exe, обновляет статус CustomBuild.
type githubPollErrorAction uint8

const (
	githubPollRetry githubPollErrorAction = iota
	githubPollFail
)

func githubPollErrorActionFor(err error) githubPollErrorAction {
	if service.IsGithubRetryable(err) && !service.IsGithubTerminal(err) {
		return githubPollRetry
	}
	return githubPollFail
}

func isActiveGithubBuildStatus(status string) bool {
	switch status {
	case model.CustomBuildStatusBuilding, model.CustomBuildStatusDownloading, model.CustomBuildStatusExtracting:
		return true
	default:
		return false
	}
}

func githubArtifactErrorActionFor(err error) githubPollErrorAction {
	return githubPollErrorActionFor(err)
}

const (
	githubPollInterval = 30 * time.Second
	githubPollDeadline = 90 * time.Minute
)

func loadGithubPollConfig(buildID uint, provenance service.BuildProvenance, deadline time.Time, now func() time.Time, wait func(time.Duration), get func() (*model.GithubBuildConfig, error)) (*model.GithubBuildConfig, bool) {
	for {
		currentTime := now()
		if !currentTime.Before(deadline) {
			if global.Logger != nil {
				global.Logger.Errorf("pollAndDownload: GitHub credentials/configuration remained unavailable for build %d until the bounded polling deadline; resume polling after provider configuration is restored", buildID)
			}
			return nil, false
		}
		gcfg, err := get()
		if err == nil && gcfg != nil && gcfg.Token != "" {
			// Only the mutable credential comes from current configuration. The
			// repository and workflow identity remain bound to the stored snapshot.
			return service.GithubConfigFromProvenance(provenance, gcfg.Token), true
		}
		if global.Logger != nil {
			if err != nil {
				global.Logger.Errorf("pollAndDownload: current GitHub configuration unavailable for build %d; retrying until the bounded polling deadline: %s", buildID, service.GithubErrorDetail(err))
			} else {
				global.Logger.Errorf("pollAndDownload: current GitHub PAT is unavailable for build %d; retrying until the bounded polling deadline", buildID)
			}
		}
		remaining := deadline.Sub(currentTime)
		if remaining <= 0 {
			return nil, false
		}
		delay := githubPollInterval
		if remaining < delay {
			delay = remaining
		}
		wait(delay)
	}
}

func (ct *CustomBuild) pollAndDownload(buildId uint, runId int64) {
	ct.pollAndDownloadWithClock(buildId, runId, githubPollClock{now: time.Now, wait: time.Sleep})
}

func (ct *CustomBuild) pollAndDownloadWithClock(buildId uint, runId int64, clock githubPollClock) {
	// Паника в фоновой горутине роняет весь процесс — гасим её здесь.
	defer func() {
		if r := recover(); r != nil {
			global.Logger.Errorf("pollAndDownload panic for build %d: %s", buildId, service.GithubErrorDetail(fmt.Errorf("background poll panic: %v", r)))
		}
	}()
	b, err := service.AllService.CustomBuildService.Info(buildId)
	if err != nil {
		if global.Logger != nil {
			global.Logger.Errorf("pollAndDownload: failed to read build %d: %s", buildId, service.GithubErrorDetail(err))
		}
		return
	}
	if b.Id == 0 {
		return
	}
	if runId == 0 && b.GithubRunId == 0 {
		reason := "legacy/partial GitHub build has no provider run id; refusing to poll"
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog(b.BuildLog, reason)
		if persistErr := persistNoRunFailure(b); persistErr != nil && global.Logger != nil {
			global.Logger.Errorf("pollAndDownload: failed to persist no-run terminal state for build %d: %s", buildId, service.GithubErrorDetail(persistErr))
		}
		return
	}
	provenance, err := service.BuildProvenanceFromRecord(b)
	if err != nil {
		reason := fmt.Sprintf("immutable GitHub provenance is missing; refusing to poll run %d: %s", runId, service.GithubErrorDetail(err))
		global.Logger.Errorf("pollAndDownload: build %d failed closed: %s", buildId, reason)
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog(b.BuildLog, reason)
		if persistErr := persistBuildProgress(b, runId); persistErr != nil {
			global.Logger.Errorf("pollAndDownload: failed to persist closed state for build %d: %s", buildId, service.GithubErrorDetail(persistErr))
		}
		return
	}
	if provenance.GithubRunID != runId || b.GithubRunId != runId {
		reason := fmt.Sprintf("immutable GitHub provenance run identity mismatch (stored=%d requested=%d); refusing to poll", provenance.GithubRunID, runId)
		global.Logger.Errorf("pollAndDownload: build %d failed closed: %s", buildId, reason)
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = boundedBuildLog(b.BuildLog, reason)
		if persistErr := persistBuildProgress(b, runId); persistErr != nil {
			global.Logger.Errorf("pollAndDownload: failed to persist mismatch state for build %d: %s", buildId, service.GithubErrorDetail(persistErr))
		}
		return
	}
	if clock.now == nil {
		clock.now = time.Now
	}
	if clock.wait == nil {
		clock.wait = time.Sleep
	}
	artifactName := provenance.GithubArtifactName
	deadline := clock.now().Add(githubPollDeadline) // защита от зависших ранов
	firstPoll := true
	for clock.now().Before(deadline) {
		if !firstPoll {
			clock.wait(githubPollInterval)
		}
		firstPoll = false
		// Recover a publication that already has both immutable proof values
		// before loading provider configuration or handling a terminal provider
		// response. This prevents a valid download from being stranded by a
		// completion-write failure. Any partial/mismatched proof is terminal: an
		// output with unverifiable provenance must never be silently reused.
		current, currentErr := service.AllService.CustomBuildService.Info(buildId)
		if currentErr != nil {
			if global.Logger != nil {
				global.Logger.Errorf("pollAndDownload: failed to refresh build %d: %s", buildId, service.GithubErrorDetail(currentErr))
			}
			return
		}
		if current.Id == 0 || current.GithubRunId != runId || !isActiveGithubBuildStatus(current.Status) {
			return
		}
		if current.PublicationRecordedAt > 0 || current.PublishedDigest != "" {
			recovered, recoveryErr := recoverRecordedPublishedBuild(buildId, runId)
			if recovered {
				if recoveryErr != nil && global.Logger != nil {
					global.Logger.Errorf("pollAndDownload: failed to finalize recorded publication for build %d: %s", buildId, service.GithubErrorDetail(recoveryErr))
				}
				return
			}
			if recoveryErr != nil {
				reason := "stored publication proof does not match the current output; refusing completion: " + service.GithubErrorDetail(recoveryErr)
				failActiveGithubBuild(buildId, runId, reason)
				return
			}
		}
		// The token/config may become unavailable after a prior successful poll.
		// Re-read it within the same bounded loop, while keeping all provider
		// identity fields pinned to the stored provenance.
		pollCfg, configAvailable := loadGithubPollConfig(
			buildId,
			provenance,
			deadline,
			clock.now,
			clock.wait,
			service.AllService.GithubBuildConfigService.Get,
		)
		if !configAvailable {
			failActiveGithubBuild(buildId, runId, fmt.Sprintf("github poll stopped at deadline; restore the PAT for stored repository %q and resume this build", provenance.GithubRepo))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		details, err := service.AllService.GithubBuildConfigService.RunStatusDetails(ctx, pollCfg, runId)
		cancel()
		if err != nil {
			if githubPollErrorActionFor(err) == githubPollRetry {
				continue
			}
			b, readErr := service.AllService.CustomBuildService.Info(buildId)
			if readErr != nil {
				if global.Logger != nil {
					global.Logger.Errorf("custom build %d read failed after GitHub status error: %s", buildId, service.GithubErrorDetail(readErr))
				}
				return
			}
			if b.Id != 0 && b.GithubRunId == runId && isActiveGithubBuildStatus(b.Status) {
				b.Status = model.CustomBuildStatusFailed
				b.BuildLog = boundedBuildLog(b.BuildLog, "github status error: "+service.GithubSafeErrorMessage(err))
				if persistErr := persistBuildProgress(b, runId); persistErr != nil {
					global.Logger.Errorf("custom build %d failed to persist status error: %s", buildId, service.GithubErrorDetail(persistErr))
				}
			}
			return
		}
		if details.SourceSHA == "" {
			reason := "provider run status omitted head_sha; refusing to continue without execution identity"
			global.Logger.Errorf("pollAndDownload: build %d failed closed: %s", buildId, reason)
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, reason)
			if persistErr := persistBuildProgress(b, runId); persistErr != nil {
				global.Logger.Errorf("pollAndDownload: failed to persist missing head_sha for build %d: %s", buildId, service.GithubErrorDetail(persistErr))
			}
			return
		}
		if !strings.EqualFold(details.SourceSHA, provenance.WorkflowSHA) {
			reason := fmt.Sprintf("provider run head SHA %q does not match stored workflow execution SHA; refusing to continue", details.SourceSHA)
			global.Logger.Errorf("pollAndDownload: build %d failed closed: %s", buildId, reason)
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, reason)
			if persistErr := persistBuildProgress(b, runId); persistErr != nil {
				global.Logger.Errorf("pollAndDownload: failed to persist workflow execution SHA mismatch for build %d: %s", buildId, service.GithubErrorDetail(persistErr))
			}
			return
		}
		if b.GithubSourceSha == "" {
			shaErr := persistPollMutation(buildId, runId, "run head SHA", func() error {
				return service.AllService.CustomBuildService.SetSourceSha(buildId, runId, details.SourceSHA)
			})
			if shaErr != nil {
				global.Logger.Errorf("pollAndDownload: run head SHA persistence exhausted for build %d; leaving building row resume-eligible: %s", buildId, service.GithubErrorDetail(shaErr))
				return
			}
			b.GithubSourceSha = details.SourceSHA
		}
		if details.Status != "completed" {
			continue
		}
		// completed → финализируем
		b, err = service.AllService.CustomBuildService.Info(buildId)
		if err != nil {
			if global.Logger != nil {
				global.Logger.Errorf("pollAndDownload: failed to refresh completed build %d: %s", buildId, service.GithubErrorDetail(err))
			}
			return
		}
		// A successful Info call returns a complete row; a missing row is handled
		// as an error before this point.
		if b.Id == 0 {
			return
		}
		if details.Conclusion != "success" {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, fmt.Sprintf("run %d completed with conclusion=%s", runId, details.Conclusion))
			if persistErr := persistBuildProgress(b, runId); persistErr != nil {
				global.Logger.Errorf("custom build %d failed to persist conclusion: %s", buildId, service.GithubErrorDetail(persistErr))
			}
			return
		}
		// An output without a valid stored publication proof is never reused. If
		// one exists, the exact stored provider artifact is downloaded again and
		// atomically replaces it after validation. A proof mismatch was handled
		// above and is terminal instead of being repaired by a guessed artifact.
		_, outputExists, inspectErr := inspectPublishedArtifact(customBuildOutputDir(b.Id), b)
		if outputExists && global.Logger != nil {
			if inspectErr != nil {
				global.Logger.Warnf("custom build %d has an unproven/invalid final output; redownloading exact artifact: %s", buildId, service.GithubErrorDetail(inspectErr))
			} else {
				global.Logger.Warnf("custom build %d has an unproven final output; redownloading exact artifact", buildId)
			}
		}
		b.Status = model.CustomBuildStatusDownloading
		b.BuildLog = boundedBuildLog(b.BuildLog, "artifact download started")
		if persistErr := persistBuildProgressWithRun(service.AllService.CustomBuildService.UpdateProgress, b, runId); persistErr != nil {
			global.Logger.Errorf("custom build %d failed to persist downloading state: %s", buildId, service.GithubErrorDetail(persistErr))
			return
		}
		dlCtx, dlCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		storedArtifactID := provenance.GithubArtifactID
		download, err := service.AllService.GithubBuildConfigService.DownloadArtifact(
			dlCtx, pollCfg, runId, storedArtifactID, artifactName,
		)
		dlCancel()
		if err != nil {
			if githubArtifactErrorActionFor(err) == githubPollRetry {
				continue
			}
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, "download artifact: "+service.GithubSafeErrorMessage(err))
			if persistErr := persistBuildProgress(b, runId); persistErr != nil {
				global.Logger.Errorf("custom build %d failed to persist artifact error: %s", buildId, service.GithubErrorDetail(persistErr))
			}
			return
		}
		// DownloadArtifact owns the temporary archive only until it returns a
		// successful hand-off. Register cleanup before any later lifecycle write;
		// a database failure must not strand the ZIP or its temporary path.
		archiveCleanupDone := false
		defer func(archivePath string) {
			if !archiveCleanupDone {
				cleanupArtifactArchive(buildId, archivePath)
			}
		}(download.ArchivePath)
		b.Status = model.CustomBuildStatusExtracting
		b.BuildLog = boundedBuildLog(b.BuildLog, "artifact download complete; extracting")
		if persistErr := persistBuildProgressWithRun(service.AllService.CustomBuildService.UpdateProgress, b, runId); persistErr != nil {
			global.Logger.Errorf("custom build %d failed to persist extracting state: %s", buildId, service.GithubErrorDetail(persistErr))
			return
		}
		if storedArtifactID == 0 {
			setErr := persistPollMutation(buildId, runId, "artifact ID", func() error {
				return service.AllService.CustomBuildService.SetArtifactID(buildId, runId, download.ArtifactID)
			})
			if setErr != nil {
				// A concurrent/resumed poll may have durably written the same
				// provider-selected ID. Re-read only to recover that exact ID;
				// never select a different artifact or infer one from a list.
				latest, latestReadErr := service.AllService.CustomBuildService.Info(buildId)
				if latestReadErr != nil {
					global.Logger.Errorf("pollAndDownload: failed to reread build %d after artifact ID persistence: %s", buildId, service.GithubErrorDetail(latestReadErr))
					return
				}
				latestProvenance, latestErr := service.BuildProvenanceFromRecord(latest)
				if latestErr != nil || latestProvenance.GithubArtifactID != download.ArtifactID {
					global.Logger.Errorf("pollAndDownload: artifact ID persistence exhausted for build %d; leaving building row resume-eligible: %s", buildId, service.GithubErrorDetail(setErr))
					return
				}
				provenance = latestProvenance
			}
			provenance.GithubArtifactID = download.ArtifactID
		} else if download.ArtifactID != storedArtifactID || download.ArtifactName != artifactName {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, "provider returned an artifact identity different from stored provenance")
			if persistErr := persistBuildProgress(b, runId); persistErr != nil {
				global.Logger.Errorf("custom build %d failed to persist artifact identity mismatch: %s", buildId, service.GithubErrorDetail(persistErr))
			}
			return
		}
		b.GithubArtifactID = download.ArtifactID

		// Delete and poll share the same process-local lifecycle guard. Re-read
		// the guarded row immediately before publication so a delete or terminal
		// transition observed after the download cannot publish into a stale
		// build directory. This is intentionally not a distributed lease.
		latest, latestErr := service.AllService.CustomBuildService.Info(buildId)
		if latestErr != nil {
			if global.Logger != nil {
				global.Logger.Errorf("pollAndDownload: failed to reread build %d before publication: %s", buildId, service.GithubErrorDetail(latestErr))
			}
			return
		}
		if latest.Id == 0 || latest.GithubRunId != runId || !isActiveGithubBuildStatus(latest.Status) {
			return
		}
		b = latest
		b.GithubArtifactID = download.ArtifactID
		// This is the exact stored run/artifact download. If another unproven
		// output appeared after the inspection, replace it rather than reusing it;
		// a valid stored proof still wins inside the helper.
		fileSize, producerManifest, publishErr := publishDownloadedArtifactWithManifest(b, download.ArchivePath, true)
		if publishErr != nil {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog = boundedBuildLog(b.BuildLog, "artifact validation/publication: "+service.GithubErrorDetail(publishErr))
			if persistErr := persistBuildProgress(b, runId); persistErr != nil {
				global.Logger.Errorf("custom build %d failed to persist artifact publication error: %s", buildId, service.GithubErrorDetail(persistErr))
			}
			return
		}
		cleanupArtifactArchive(buildId, download.ArchivePath)
		archiveCleanupDone = true

		b.FileSize = fileSize
		b.Status = model.CustomBuildStatusDone
		b.BuildLog = boundedBuildLog(b.BuildLog, "artifact downloaded and extracted")
		if publishErr := recordPublishedOutputWithManifest(b.Id, runId, b.GithubArtifactID, producerManifest); publishErr != nil {
			if global.Logger != nil {
				global.Logger.Errorf("custom build %d failed to record publication: %s", buildId, service.GithubErrorDetail(publishErr))
			}
			return
		}
		if persistErr := persistCompletedBuildWith(service.AllService.CustomBuildService.UpdateProgress, b, runId); persistErr != nil {
			// UpdateProgress is the only operation that can claim completion. If all
			// bounded retries fail, the database row remains building and resume can
			// retry the exact artifact identity; never report or leave a false done.
			global.Logger.Errorf("custom build %d failed to persist completion; leaving row resume-eligible and non-done: %s", buildId, service.GithubErrorDetail(persistErr))
			// The output is already validated and published. Keep it intact: a
			// later poll can idempotently reuse it after the guarded DB write
			// recovers, and cleanup here could destroy the only valid artifact.
		}
		return
	}
	// таймаут
	failActiveGithubBuild(buildId, runId, "github polling deadline reached; verify the provider run and resume this build")
}

// publishDownloadedArtifact validates and extracts a temporary ZIP into a
// sibling staging directory, then atomically renames that directory to the
// final output path. The final directory is never created before validation.
func publishDownloadedArtifact(build *model.CustomBuild, archivePath string) (int64, error) {
	return publishDownloadedArtifactWithMode(build, archivePath, false)
}

func publishDownloadedArtifactWithMode(build *model.CustomBuild, archivePath string, replaceUnproven bool) (int64, error) {
	size, _, err := publishDownloadedArtifactWithManifest(build, archivePath, replaceUnproven)
	return size, err
}

func publishDownloadedArtifactWithManifest(build *model.CustomBuild, archivePath string, replaceUnproven bool) (int64, service.ProducerManifest, error) {
	if err := service.RequireProductionBuildCapability(build.Platform); err != nil {
		return 0, service.ProducerManifest{}, err
	}
	outDir := customBuildOutputDir(build.Id)
	parent := filepath.Dir(outDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return 0, service.ProducerManifest{}, fmt.Errorf("create artifact output parent: %w", err)
	}
	staging, err := os.MkdirTemp(parent, fmt.Sprintf(".%d-artifact-*", build.Id))
	if err != nil {
		return 0, service.ProducerManifest{}, fmt.Errorf("create artifact staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()

	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, service.ProducerManifest{}, fmt.Errorf("open artifact ZIP: %w", err)
	}
	archiveClosed := false
	defer func() {
		if !archiveClosed {
			_ = zr.Close()
		}
	}()
	fileSize, producerManifest, extractErr := extractValidatedArtifact(zr, staging, build)
	closeErr := zr.Close()
	archiveClosed = true
	if extractErr != nil {
		return 0, service.ProducerManifest{}, extractErr
	}
	if closeErr != nil {
		return 0, service.ProducerManifest{}, fmt.Errorf("close artifact ZIP: %w", closeErr)
	}
	if _, exists, err := inspectPublishedArtifact(outDir, build); exists {
		if proofSize, proofErr := service.ValidatePublishedOutputProof(build); proofErr == nil {
			return proofSize, producerManifest, nil
		}
		if !replaceUnproven {
			if err != nil {
				return 0, service.ProducerManifest{}, fmt.Errorf("refusing to reuse unproven existing artifact output: %w", err)
			}
			return 0, service.ProducerManifest{}, errors.New("refusing to reuse unproven existing artifact output")
		}
		if err := replacePublishedOutput(staging, outDir); err != nil {
			return 0, service.ProducerManifest{}, err
		}
		published = true
		return fileSize, producerManifest, nil
	} else if err != nil {
		return 0, service.ProducerManifest{}, err
	}
	if err := os.Rename(staging, outDir); err != nil {
		// Another poller may have won the absent-target race. Reuse only a
		// complete output that satisfies the same stored publication proof.
		if proofSize, proofErr := service.ValidatePublishedOutputProof(build); proofErr == nil {
			return proofSize, producerManifest, nil
		}
		if replaceUnproven {
			if replaceErr := replacePublishedOutput(staging, outDir); replaceErr == nil {
				published = true
				return fileSize, producerManifest, nil
			}
		}
		return 0, service.ProducerManifest{}, fmt.Errorf("atomically publish artifact output: %w", err)
	}
	published = true
	return fileSize, producerManifest, nil
}

func replacePublishedOutput(staging, outputDir string) error {
	outputInfo, err := os.Lstat(outputDir)
	if err != nil {
		return fmt.Errorf("inspect existing artifact output before replacement: %w", err)
	}
	if outputInfo.Mode()&os.ModeSymlink != 0 || !outputInfo.IsDir() {
		return errors.New("refusing to replace non-directory artifact output")
	}
	backup, err := os.MkdirTemp(filepath.Dir(outputDir), fmt.Sprintf(".%s-artifact-recovery-*", filepath.Base(outputDir)))
	if err != nil {
		return fmt.Errorf("create artifact recovery backup: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare artifact recovery backup: %w", err)
	}
	if err := os.Rename(outputDir, backup); err != nil {
		return fmt.Errorf("stage unproven artifact output for replacement: %w", err)
	}
	if err := os.Rename(staging, outputDir); err != nil {
		if restoreErr := os.Rename(backup, outputDir); restoreErr != nil {
			return fmt.Errorf("replace artifact output: %w (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("replace artifact output: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove replaced artifact output backup: %w", err)
	}
	return nil
}

// inspectPublishedArtifact validates an already-published output without
// mutating it. A valid output is reusable after a crash between filesystem
// publication and guarded status persistence.
func inspectPublishedArtifact(outDir string, build *model.CustomBuild) (int64, bool, error) {
	size, err := service.ValidatePublishedOutput(outDir, build)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	} else if err != nil {
		return 0, true, err
	}
	return size, true, nil
}

// extractValidatedArtifact preserves the existing platform contract while
// rejecting unsafe, duplicate, non-regular, or incomplete entries before any
// output becomes visible at the final path.
func extractValidatedArtifact(zr *zip.ReadCloser, staging string, build *model.CustomBuild) (int64, service.ProducerManifest, error) {
	if artifactMaxZipEntries <= 0 || int64(len(zr.File)) > artifactMaxZipEntries {
		return 0, service.ProducerManifest{}, fmt.Errorf("artifact ZIP contains too many entries: %d", len(zr.File))
	}
	if err := validateZipMetadata(zr.File); err != nil {
		return 0, service.ProducerManifest{}, err
	}
	producerManifest, hasProducerManifest, err := readProducerManifest(zr, build)
	if err != nil {
		return 0, service.ProducerManifest{}, err
	}
	appName := build.AppName
	if err := service.ValidateOutputAppName(appName); err != nil {
		return 0, service.ProducerManifest{}, err
	}
	seen := make(map[string]struct{}, len(zr.File))
	var largestFile int64
	var extractedBytes uint64
	var extractedFiles int
	var windowsExe bool

	for _, zf := range zr.File {
		name, err := safeZipEntryName(zf.Name)
		if err != nil {
			return 0, service.ProducerManifest{}, err
		}
		if hasProducerManifest {
			if zf.Name == service.ProducerManifestFilename {
				continue
			}
			if zf.FileInfo().Mode()&os.ModeSymlink != 0 {
				return 0, service.ProducerManifest{}, fmt.Errorf("producer artifact contains symlink %q", zf.Name)
			}
			if zf.FileInfo().IsDir() {
				if build.Platform == "bridge" {
					continue
				}
				return 0, service.ProducerManifest{}, fmt.Errorf("producer artifact contains unsafe extra entry %q", zf.Name)
			}
			if name != zf.Name {
				return 0, service.ProducerManifest{}, fmt.Errorf("producer artifact contains unsafe extra entry %q", zf.Name)
			}
			if !producerManifestHasFile(producerManifest, name) && !producerManifestHasPrivateFile(producerManifest, name) {
				return 0, service.ProducerManifest{}, fmt.Errorf("producer artifact contains unexpected output file %q", zf.Name)
			}
			n, err := extractZipFile(zf, staging, name, &extractedBytes, build.Platform == "bridge")
			if err != nil {
				return 0, service.ProducerManifest{}, fmt.Errorf("extract producer output %q: %w", zf.Name, err)
			}
			extractedFiles++
			if n > largestFile {
				largestFile = n
			}
			continue
		}
		if zf.FileInfo().IsDir() || strings.HasSuffix(strings.ReplaceAll(zf.Name, "\\", "/"), "/") {
			continue
		}
		if build.Platform == string(service.PlatformWindows) {
			if err := service.ValidateWindowsArtifactFilename(zf.Name); err != nil {
				return 0, service.ProducerManifest{}, fmt.Errorf("unsafe Windows artifact filename %q: %w", zf.Name, err)
			}
		}
		if zf.FileInfo().Mode()&os.ModeSymlink != 0 {
			return 0, service.ProducerManifest{}, fmt.Errorf("artifact ZIP contains symlink %q", zf.Name)
		}

		switch build.Platform {
		case "linux", "android":
			if _, exists := seen[name]; exists {
				return 0, service.ProducerManifest{}, fmt.Errorf("artifact ZIP contains duplicate output file %q", name)
			}
			n, err := extractZipFile(zf, staging, name, &extractedBytes, false)
			if err != nil {
				return 0, service.ProducerManifest{}, fmt.Errorf("extract %q: %w", zf.Name, err)
			}
			seen[name] = struct{}{}
			extractedFiles++
			if n > largestFile {
				largestFile = n
			}
		default:
			target := ""
			caseInsensitive := build.Platform == string(service.PlatformWindows)
			equalName := func(left, right string) bool {
				if caseInsensitive {
					return strings.EqualFold(left, right)
				}
				return left == right
			}
			switch {
			case equalName(name, appName+".exe") || equalName(name, "rustdesk.exe"):
				target = appName + ".exe"
			case equalName(name, "custom_.txt") || (caseInsensitive && strings.EqualFold(filepath.Ext(name), ".dll")) || (!caseInsensitive && filepath.Ext(name) == ".dll"):
				target = name
			}
			if target == "" {
				continue
			}
			seenKey := target
			if caseInsensitive {
				seenKey = service.WindowsArtifactNameKey(target)
			}
			if _, exists := seen[seenKey]; exists {
				return 0, service.ProducerManifest{}, fmt.Errorf("artifact ZIP contains duplicate output file %q", target)
			}
			n, err := extractZipFile(zf, staging, target, &extractedBytes, false)
			if err != nil {
				return 0, service.ProducerManifest{}, fmt.Errorf("extract %q: %w", zf.Name, err)
			}
			seen[seenKey] = struct{}{}
			extractedFiles++
			if target == appName+".exe" {
				windowsExe = true
				largestFile = n
			}
		}
	}

	if extractedFiles == 0 {
		return 0, service.ProducerManifest{}, errors.New("artifact ZIP contains no usable files")
	}
	if hasProducerManifest {
		if _, err := service.ValidateProducerManifestOutput(producerManifest, staging); err != nil {
			return 0, service.ProducerManifest{}, err
		}
		if build.Platform == string(service.PlatformWindows) {
			if info, err := os.Stat(filepath.Join(staging, appName+".exe")); err == nil {
				largestFile = info.Size()
			}
		}
		return largestFile, producerManifest, nil
	}
	if build.Platform != "linux" && build.Platform != "android" && !windowsExe {
		return 0, service.ProducerManifest{}, fmt.Errorf("artifact ZIP is missing required %s.exe", appName)
	}
	return largestFile, service.ProducerManifest{}, nil
}

func readProducerManifest(zr *zip.ReadCloser, build *model.CustomBuild) (service.ProducerManifest, bool, error) {
	var matches []*zip.File
	for _, entry := range zr.File {
		if strings.EqualFold(entry.Name, service.ProducerManifestFilename) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		if service.RequiresProducerManifest(build) {
			return service.ProducerManifest{}, false, errors.New("active provider artifact is missing manifest.txt")
		}
		return service.ProducerManifest{}, false, nil
	}
	if len(matches) != 1 || matches[0].Name != service.ProducerManifestFilename {
		return service.ProducerManifest{}, false, errors.New("artifact ZIP contains duplicate or case-colliding manifest.txt entries")
	}
	entry := matches[0]
	if entry.FileInfo().Mode()&os.ModeSymlink != 0 || entry.FileInfo().IsDir() || entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > service.MaxProducerManifestBytes {
		return service.ProducerManifest{}, false, errors.New("manifest.txt is missing or exceeds the manifest size limit")
	}
	reader, err := entry.Open()
	if err != nil {
		return service.ProducerManifest{}, false, fmt.Errorf("open manifest.txt: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, service.MaxProducerManifestBytes+1))
	closeErr := reader.Close()
	if readErr != nil {
		return service.ProducerManifest{}, false, fmt.Errorf("read manifest.txt: %w", readErr)
	}
	if closeErr != nil {
		return service.ProducerManifest{}, false, fmt.Errorf("close manifest.txt: %w", closeErr)
	}
	if int64(len(data)) != int64(entry.UncompressedSize64) || len(data) > service.MaxProducerManifestBytes {
		return service.ProducerManifest{}, false, errors.New("manifest.txt size changed during extraction")
	}
	manifest, err := service.ParseProducerManifest(data)
	if err != nil {
		return service.ProducerManifest{}, false, err
	}
	if err := service.ValidateProducerManifestForBuild(manifest, build); err != nil {
		return service.ProducerManifest{}, false, err
	}
	return manifest, true, nil
}

func producerManifestHasFile(manifest service.ProducerManifest, name string) bool {
	for _, file := range manifest.Files {
		if file.Name == name {
			return true
		}
	}
	return false
}

func producerManifestHasPrivateFile(manifest service.ProducerManifest, name string) bool {
	for _, privateName := range manifest.PrivateFilenames {
		if privateName == name {
			return true
		}
	}
	return false
}

func validateZipMetadata(files []*zip.File) error {
	if artifactMaxFileBytes == 0 || artifactMaxAggregateBytes == 0 || artifactMaxCompressionRatio == 0 {
		return errors.New("artifact ZIP safety limits are invalid")
	}
	var aggregate uint64
	for _, zf := range files {
		uncompressed := zf.UncompressedSize64
		compressed := zf.CompressedSize64
		if uncompressed > artifactMaxFileBytes {
			return fmt.Errorf("artifact ZIP entry %q exceeds per-file uncompressed limit", zf.Name)
		}
		if uncompressed > artifactMaxAggregateBytes || aggregate > artifactMaxAggregateBytes-uncompressed {
			return fmt.Errorf("artifact ZIP exceeds aggregate uncompressed limit")
		}
		aggregate += uncompressed
		if uncompressed == 0 {
			continue
		}
		if compressed == 0 {
			return fmt.Errorf("artifact ZIP entry %q has invalid compressed size", zf.Name)
		}
		quotient, remainder := uncompressed/compressed, uncompressed%compressed
		if quotient > artifactMaxCompressionRatio || (quotient == artifactMaxCompressionRatio && remainder != 0) {
			return fmt.Errorf("artifact ZIP entry %q has suspicious compression ratio", zf.Name)
		}
	}
	return nil
}

func safeZipEntryName(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("artifact ZIP contains an invalid path")
	}
	normalized := strings.ReplaceAll(name, "\\", "/")
	if path.IsAbs(normalized) || strings.HasPrefix(normalized, "//") || (len(normalized) >= 2 && normalized[1] == ':') {
		return "", fmt.Errorf("zip slip: absolute artifact path %q", name)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("zip slip: parent artifact path %q", name)
		}
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == "/" || clean == ".." {
		return "", fmt.Errorf("artifact ZIP contains an invalid path %q", name)
	}
	return clean, nil
}

// extractZipFile writes one already-validated output name without
// replacing an earlier entry. A failed copy removes its partial file.

func extractZipFile(zf *zip.File, outDir, name string, aggregate *uint64, allowNested bool) (int64, error) {
	if name == "" || strings.ContainsAny(name, `\\`) || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") {
		return 0, fmt.Errorf("invalid extraction name %q", name)
	}
	if !allowNested && strings.ContainsRune(name, '/') {
		return 0, fmt.Errorf("invalid flat extraction name %q", name)
	}
	dst := filepath.Join(outDir, name)
	if allowNested {
		parent := filepath.Dir(dst)
		if err := os.MkdirAll(parent, 0700); err != nil {
			return 0, err
		}
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return 0, err
	}
	rc, err := zf.Open()
	if err != nil {
		_ = f.Close()
		_ = os.Remove(dst)
		return 0, err
	}
	n, copyErr := io.Copy(f, io.LimitReader(rc, int64(artifactMaxFileBytes)+1))
	readerCloseErr := rc.Close()
	fileCloseErr := f.Close()
	if copyErr != nil || readerCloseErr != nil || fileCloseErr != nil || n > int64(artifactMaxFileBytes) || uint64(n) != zf.UncompressedSize64 {
		_ = os.Remove(dst)
		if copyErr != nil {
			return 0, copyErr
		}
		if readerCloseErr != nil {
			return 0, readerCloseErr
		}
		if fileCloseErr != nil {
			return 0, fileCloseErr
		}
		if n > int64(artifactMaxFileBytes) {
			return 0, errors.New("uncompressed file exceeds limit")
		}
		return 0, fmt.Errorf("extracted size %d does not match ZIP metadata %d", n, zf.UncompressedSize64)
	}
	if *aggregate > artifactMaxAggregateBytes-uint64(n) {
		_ = os.Remove(dst)
		return 0, errors.New("aggregate extracted bytes exceed limit")
	}
	*aggregate += uint64(n)
	return n, nil
}
