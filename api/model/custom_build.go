package model

import (
	"rustdesk-server/api/utils"

	"gorm.io/gorm"
)

type CustomBuild struct {
	IdModel
	UserId      uint   `json:"user_id" gorm:"default:0;not null;"`
	Name        string `json:"name" gorm:"size:128;default:'';not null;"`
	Platform    string `json:"platform" gorm:"size:32;default:'';not null;"`
	Version     string `json:"version" gorm:"size:32;default:'';not null;"`
	Status      string `json:"status" gorm:"size:32;default:'pending';not null;"`
	AppName     string `json:"app_name" gorm:"size:128;default:'';not null;"`
	CustomJson  string `json:"-" gorm:"type:text;"`
	BuildLog    string `json:"build_log" gorm:"type:text;"`
	FileSize    int64  `json:"file_size" gorm:"default:0;not null;"`
	DownloadKey string `json:"-" gorm:"size:64;default:'';not null;"`
	// DownloadKeyExpiresAt — unix-секунды, после которых capability-ссылка
	// (download_key) протухает (BUGS.md B-006). 0 = бессрочно: legacy-строки,
	// созданные до появления TTL, остаются доступны.
	DownloadKeyExpiresAt int64 `json:"download_key_expires_at" gorm:"default:0;not null;"`
	// GithubRunId — id рана GitHub Actions, если билд диспетчился туда. Нужен для
	// возобновления `pollAndDownload` после рестарта api (BUGS.md B-003). 0 = legacy
	// file-queue/не диспетчен; это не допустимый id текущего provider-backed рана.
	GithubRunId int64 `json:"github_run_id" gorm:"default:0;not null;"`
	// Immutable GitHub dispatch provenance. These fields are provider-derived
	// build metadata; secrets and the dispatch payload are intentionally absent.
	GithubProvider string `json:"github_provider" gorm:"size:32;default:'';not null;"`
	GithubRepo     string `json:"github_repo" gorm:"size:128;default:'';not null;"`
	GithubWorkflow string `json:"github_workflow" gorm:"size:128;default:'';not null;"`
	// WorkflowSelector is the exact provider ref used to dispatch the workflow.
	// GithubRef remains the separately persisted immutable execution SHA.
	WorkflowSelector string `json:"-" gorm:"size:128;default:'';not null;"`
	// GithubRef is the immutable commit SHA of the workflow execution ref used
	// for dispatch. The source tag SHA remains in BuildRef and is never replaced
	// by this value. Mutable legacy refs are not accepted for new polling.
	GithubRef          string `json:"github_ref" gorm:"size:128;default:'';not null;"`
	GithubArtifactName string `json:"github_artifact_name" gorm:"size:128;default:'';not null;"`
	GithubArtifactID   int64  `json:"github_artifact_id" gorm:"default:0;not null;"`
	GithubRunUrl       string `json:"github_run_url" gorm:"size:512;default:'';not null;"`
	GithubHtmlUrl      string `json:"github_html_url" gorm:"size:512;default:'';not null;"`
	// GithubSourceSha retains the provider-observed run head_sha. It must match
	// GithubRef (the execution SHA); BuildRef remains the checked-out source SHA.
	GithubSourceSha string `json:"github_source_sha" gorm:"size:128;default:'';not null;"`
	// PublicationRecordedAt is a non-secret, service-generated proof that the
	// exact dispatched artifact was validated and atomically published. 0 means
	// no publication has been recorded.
	PublicationRecordedAt int64 `json:"-" gorm:"default:0;not null;"`
	// PublishedDigest is the service-generated SHA-256 manifest of the
	// canonical published output. It is write-once with PublicationRecordedAt.
	PublishedDigest string `json:"-" gorm:"size:64;default:'';not null;"`
	// ProducerManifestJSON is the validated, secret-free manifest emitted by
	// the provider workflow. It is retained so handoff exports do not depend on
	// the extracted manifest.txt or a mutable host filesystem.
	ProducerManifestJSON string `json:"-" gorm:"type:text;"`
	// Version remains the display metadata selected by the user. These fields
	// are the immutable, system-derived release identity used for dispatch.
	BuildRef        string `json:"-" gorm:"size:128;default:'';not null;"`
	SourceTag       string `json:"-" gorm:"size:128;default:'';not null;"`
	AssetsRelease   string `json:"-" gorm:"size:128;default:'';not null;"`
	AssetsReleaseID int64  `json:"-" gorm:"default:0;not null;"`
	// AssetsReleaseAssets is canonical JSON of provider-supplied required asset
	// IDs, names, and digests. It is storage-only; BuildProvenance exposes the
	// typed form when reporting immutable publication metadata.
	AssetsReleaseAssets string `json:"-" gorm:"type:text;"`
	TimeModel
}

type CustomBuildList struct {
	CustomBuilds []*CustomBuild `json:"list"`
	Pagination
}

// CustomBuildSafe is the administrative response view. It keeps the existing
// non-secret build contract while replacing custom_json with its redacted,
// canonical settings view. Storage-only identity fields remain excluded.
type CustomBuildSafe struct {
	Id                   uint   `json:"id"`
	UserId               uint   `json:"user_id"`
	Name                 string `json:"name"`
	Platform             string `json:"platform"`
	Version              string `json:"version"`
	Status               string `json:"status"`
	AppName              string `json:"app_name"`
	CustomJson           string `json:"custom_json"`
	BuildLog             string `json:"build_log"`
	FileSize             int64  `json:"file_size"`
	DownloadKeyExpiresAt int64  `json:"download_key_expires_at"`
	GithubRunId          int64  `json:"github_run_id"`
	GithubProvider       string `json:"github_provider"`
	GithubRepo           string `json:"github_repo"`
	GithubWorkflow       string `json:"github_workflow"`
	GithubRef            string `json:"github_ref"`
	GithubArtifactName   string `json:"github_artifact_name"`
	GithubArtifactID     int64  `json:"github_artifact_id"`
	GithubRunUrl         string `json:"github_run_url"`
	GithubHtmlUrl        string `json:"github_html_url"`
	GithubSourceSha      string `json:"github_source_sha"`
	TimeModel
}

// CustomBuildSafeList is the paginated administrative response view.
type CustomBuildSafeList struct {
	CustomBuilds []*CustomBuildSafe `json:"list"`
	Pagination
}

// CustomBuildPublic is the response view for the unauthenticated capability
// route. It contains only the status/version/platform/app and download fields
// needed by the public UI. Provider metadata, build logs, custom configuration,
// and DownloadKey are intentionally excluded.
type CustomBuildPublic struct {
	Id                   uint   `json:"id"`
	Name                 string `json:"name"`
	Platform             string `json:"platform"`
	Version              string `json:"version"`
	Status               string `json:"status"`
	AppName              string `json:"app_name"`
	FileSize             int64  `json:"file_size"`
	DownloadKeyExpiresAt int64  `json:"download_key_expires_at"`
}

// Safe returns a response-only view that cannot serialize the raw model's
// decrypted or encrypted custom_json representation.
func (c *CustomBuild) Safe() *CustomBuildSafe {
	if c == nil {
		return nil
	}
	return &CustomBuildSafe{
		Id:                   c.Id,
		UserId:               c.UserId,
		Name:                 c.Name,
		Platform:             c.Platform,
		Version:              c.Version,
		Status:               c.Status,
		AppName:              c.AppName,
		CustomJson:           utils.RedactCustomBuilderJSON(c.CustomJson),
		BuildLog:             c.BuildLog,
		FileSize:             c.FileSize,
		DownloadKeyExpiresAt: c.DownloadKeyExpiresAt,
		GithubRunId:          c.GithubRunId,
		GithubProvider:       c.GithubProvider,
		GithubRepo:           c.GithubRepo,
		GithubWorkflow:       c.GithubWorkflow,
		GithubRef:            c.GithubRef,
		GithubArtifactName:   c.GithubArtifactName,
		GithubArtifactID:     c.GithubArtifactID,
		GithubRunUrl:         c.GithubRunUrl,
		GithubHtmlUrl:        c.GithubHtmlUrl,
		GithubSourceSha:      c.GithubSourceSha,
		TimeModel:            c.TimeModel,
	}
}

// Public returns the redacted view used by the public capability detail route.
// Unlike Safe, this view has no capability key or other route credential.
func (c *CustomBuild) Public() *CustomBuildPublic {
	if c == nil {
		return nil
	}
	return &CustomBuildPublic{
		Id:                   c.Id,
		Name:                 c.Name,
		Platform:             c.Platform,
		Version:              c.Version,
		Status:               c.Status,
		AppName:              c.AppName,
		FileSize:             c.FileSize,
		DownloadKeyExpiresAt: c.DownloadKeyExpiresAt,
	}
}

// Safe returns a paginated response-only view for administrative consumers.
func (l *CustomBuildList) Safe() *CustomBuildSafeList {
	if l == nil {
		return nil
	}
	view := &CustomBuildSafeList{Pagination: l.Pagination}
	if l.CustomBuilds != nil {
		view.CustomBuilds = make([]*CustomBuildSafe, 0, len(l.CustomBuilds))
		for _, build := range l.CustomBuilds {
			view.CustomBuilds = append(view.CustomBuilds, build.Safe())
		}
	}
	return view
}

const (
	CustomBuildStatusPending     = "pending"
	CustomBuildStatusBuilding    = "building"
	CustomBuildStatusDownloading = "downloading"
	CustomBuildStatusExtracting  = "extracting"
	CustomBuildStatusDone        = "done"
	CustomBuildStatusFailed      = "failed"
)

// --- BUGS.md B-008: non-empty permanent_password lies inside custom_json.
// Secret-bearing JSON is encrypted at rest; non-secret typed JSON keeps its
// existing representation so core non-secret operations remain available. ----

func (c *CustomBuild) BeforeSave(tx *gorm.DB) error {
	if err := utils.ValidateCustomBuilderJSONFields(c.CustomJson); err != nil {
		return err
	}
	customJSON, err := utils.EncryptCustomBuilderJSON(c.CustomJson)
	if err != nil {
		return err
	}
	c.CustomJson = customJSON
	return err
}

func (c *CustomBuild) AfterSave(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.DecryptSecret(c.CustomJson)
	return err
}

func (c *CustomBuild) AfterFind(tx *gorm.DB) error {
	var err error
	c.CustomJson, err = utils.DecryptSecret(c.CustomJson)
	return err
}
