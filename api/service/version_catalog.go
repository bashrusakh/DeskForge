package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"
)

const (
	assetsReleasePrefix  = "offline-assets-"
	maxReleasePages      = 10
	versionCatalogTTL    = 5 * time.Minute
	githubSourceRefLimit = 128
)

var githubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// AssetsRelease is the provider release metadata paired with one source
// version. The release tag is user-visible metadata; the ID prevents silently
// switching to a different GitHub release with the same tag.
type AssetsRelease struct {
	ID      int64          `json:"id"`
	TagName string         `json:"tag_name"`
	Name    string         `json:"name,omitempty"`
	Assets  []ReleaseAsset `json:"assets"`
}

// ReleaseAsset is provider-derived identity for one required offline asset.
// Digest is copied verbatim from GitHub; the service never computes or invents
// a provider checksum at catalog time.
type ReleaseAsset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

var requiredOfflineAssetNames = []string{
	"windows-x64-release.zip",
	"usbmmidd_v2.zip",
	"rustdesk_printer_driver_v4-1.4.zip",
	"printer_driver_adapter.zip",
}

// VersionIdentity is the complete immutable release identity selected by the
// normal typed version flow. BuildRef is the source object SHA resolved from
// the configured repository's exact source tag; it is never user supplied.
type VersionIdentity struct {
	Repo           string `json:"-"`
	DisplayVersion string `json:"version"`
	BuildRef       string `json:"-"`
	SourceTag      string `json:"-"`
	// WorkflowRef is the provider selector used to locate the owned workflow
	// definition at dispatch time. WorkflowSHA is the immutable commit that
	// GitHub resolves that selector to and is persisted in CustomBuild.GithubRef.
	WorkflowRef   string        `json:"-"`
	WorkflowSHA   string        `json:"-"`
	AssetsRelease AssetsRelease `json:"assets_release"`
}

// AvailableVersion is a catalog entry. Its provider identity is retained in
// the service domain for persistence, while controllers expose only the
// display version and safe release metadata to normal users.
type AvailableVersion struct {
	VersionIdentity
}

type versionCatalogCache struct {
	entries    map[string][]AvailableVersion
	cachedAt   map[string]time.Time
	activeRepo string
	mu         sync.Mutex
}

var (
	versionCache  versionCatalogCache
	versionFlight singleflight.Group
)

func resetVersionCatalogCache() {
	versionCache.mu.Lock()
	versionCache.entries = nil
	versionCache.cachedAt = nil
	versionCache.activeRepo = ""
	versionCache.mu.Unlock()
}

func validateGithubRepo(repo string) error {
	if !githubRepoPattern.MatchString(repo) {
		return fmt.Errorf("configured GitHub repo must be owner/name")
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "." || owner == ".." || name == "." || name == ".." {
		return fmt.Errorf("configured GitHub repo must not contain dot-segment paths")
	}
	return nil
}

func githubRepoPath(repo, suffix string) (string, error) {
	if err := validateGithubRepo(repo); err != nil {
		return "", err
	}
	return "/repos/" + repo + suffix, nil
}

func (identity VersionIdentity) validate() error {
	if err := validateGithubRepo(identity.Repo); err != nil {
		return err
	}
	if !utils.ValidateBuildVersion(identity.DisplayVersion) {
		return fmt.Errorf("invalid display version %q", identity.DisplayVersion)
	}
	if identity.SourceTag != identity.DisplayVersion {
		return fmt.Errorf("source tag %q does not match version %q", identity.SourceTag, identity.DisplayVersion)
	}
	if !validGithubSourceSHA(identity.BuildRef) {
		return fmt.Errorf("build ref must be a 40-64 character hexadecimal SHA")
	}
	if identity.WorkflowRef != "" && !validGithubWorkflowRef(identity.WorkflowRef) {
		return fmt.Errorf("workflow execution ref is invalid")
	}
	if identity.WorkflowRef == "" {
		return fmt.Errorf("workflow execution selector is missing")
	}
	if !validGithubSourceSHA(identity.WorkflowSHA) {
		return fmt.Errorf("workflow execution SHA is missing or invalid")
	}
	wantRelease := assetsReleasePrefix + identity.DisplayVersion
	if identity.AssetsRelease.TagName != wantRelease || identity.AssetsRelease.ID <= 0 {
		return fmt.Errorf("assets release does not match version %q", identity.DisplayVersion)
	}
	if err := validateReleaseAssets(identity.AssetsRelease.Assets); err != nil {
		return fmt.Errorf("release asset identity is missing or invalid: %w", err)
	}
	return nil
}

func validGithubWorkflowRef(ref string) bool {
	if validGithubSourceSHA(ref) {
		return false
	}
	if len(ref) == 0 || len(ref) > githubSourceRefLimit || strings.Contains(ref, "..") || strings.Contains(ref, "//") {
		return false
	}
	if strings.HasPrefix(ref, "refs/") && !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/tags/") {
		return false
	}
	for _, char := range ref {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && !strings.ContainsRune("._/-", char) {
			return false
		}
	}
	if ref[0] == '/' || ref[len(ref)-1] == '/' {
		return false
	}
	if strings.HasSuffix(ref, "refs/heads/") || strings.HasSuffix(ref, "refs/tags/") {
		return false
	}
	return true
}

func validateReleaseAssets(assets []ReleaseAsset) error {
	if len(assets) != len(requiredOfflineAssetNames) {
		return fmt.Errorf("release asset metadata must contain exactly %d required assets", len(requiredOfflineAssetNames))
	}
	byName := make(map[string]ReleaseAsset, len(assets))
	for _, asset := range assets {
		if asset.ID <= 0 || asset.Name == "" || asset.Name != strings.TrimSpace(asset.Name) {
			return fmt.Errorf("asset has invalid id or name")
		}
		if _, duplicate := byName[asset.Name]; duplicate {
			return fmt.Errorf("asset %q is ambiguous", asset.Name)
		}
		if !strings.HasPrefix(asset.Digest, "sha256:") || !validGithubSourceSHA(strings.TrimPrefix(asset.Digest, "sha256:")) || len(strings.TrimPrefix(asset.Digest, "sha256:")) != 64 {
			return fmt.Errorf("asset %q has no trusted SHA-256 digest", asset.Name)
		}
		byName[asset.Name] = asset
	}
	for name := range byName {
		required := false
		for _, requiredName := range requiredOfflineAssetNames {
			if name == requiredName {
				required = true
				break
			}
		}
		if !required {
			return fmt.Errorf("unexpected release asset %q", name)
		}
	}
	for _, name := range requiredOfflineAssetNames {
		if _, ok := byName[name]; !ok {
			return fmt.Errorf("required asset %q is missing", name)
		}
	}
	return nil
}

func marshalReleaseAssets(assets []ReleaseAsset) (string, error) {
	if err := validateReleaseAssets(assets); err != nil {
		return "", err
	}
	data, err := json.Marshal(assets)
	if err != nil {
		return "", fmt.Errorf("marshal release assets: %w", err)
	}
	return string(data), nil
}

func unmarshalReleaseAssets(raw string) ([]ReleaseAsset, error) {
	if raw == "" {
		// Legacy provenance rows predate release-asset identity. They remain
		// readable for restart compatibility, while every new persistence path
		// still requires marshalReleaseAssets to succeed.
		return nil, nil
	}
	var assets []ReleaseAsset
	if err := json.Unmarshal([]byte(raw), &assets); err != nil {
		return nil, fmt.Errorf("decode release asset metadata: %w", err)
	}
	if err := validateReleaseAssets(assets); err != nil {
		return nil, err
	}
	return assets, nil
}

// VersionIdentityFromRecord returns the stored identity and rejects legacy or
// partial rows instead of reconstructing a ref from mutable global config.
func VersionIdentityFromRecord(build *model.CustomBuild) (VersionIdentity, error) {
	if build == nil {
		return VersionIdentity{}, errors.New("build record is nil")
	}
	identity := VersionIdentity{
		Repo:           build.GithubRepo,
		DisplayVersion: build.Version,
		BuildRef:       build.BuildRef,
		SourceTag:      build.SourceTag,
		// WorkflowSelector is the exact dispatch selector. GithubRef is the
		// resolved execution commit; a mutable legacy branch/ref therefore fails
		// the SHA validation above instead of being reused as immutable identity.
		WorkflowRef: build.WorkflowSelector,
		WorkflowSHA: build.GithubRef,
		AssetsRelease: AssetsRelease{
			ID:      build.AssetsReleaseID,
			TagName: build.AssetsRelease,
		},
	}
	assets, err := unmarshalReleaseAssets(build.AssetsReleaseAssets)
	if err != nil {
		return VersionIdentity{}, fmt.Errorf("immutable release assets missing or invalid: %w", err)
	}
	identity.AssetsRelease.Assets = assets
	if err := identity.validate(); err != nil {
		return VersionIdentity{}, fmt.Errorf("immutable version identity missing or invalid: %w", err)
	}
	return identity, nil
}

func copyAvailableVersions(entries []AvailableVersion) []AvailableVersion {
	result := make([]AvailableVersion, len(entries))
	copy(result, entries)
	return result
}

func (s *GithubBuildConfigService) cachedVersions(repo string) ([]AvailableVersion, bool) {
	versionCache.mu.Lock()
	defer versionCache.mu.Unlock()
	if versionCache.activeRepo != repo {
		// The configured repository is part of the cache identity. Clear the
		// previous catalog when the setting changes so it cannot be reused.
		versionCache.entries = make(map[string][]AvailableVersion)
		versionCache.cachedAt = make(map[string]time.Time)
		versionCache.activeRepo = repo
	}
	entries, ok := versionCache.entries[repo]
	if !ok || time.Since(versionCache.cachedAt[repo]) >= versionCatalogTTL {
		return nil, false
	}
	return copyAvailableVersions(entries), true
}

func (s *GithubBuildConfigService) storeVersions(repo string, entries []AvailableVersion) {
	versionCache.mu.Lock()
	defer versionCache.mu.Unlock()
	if versionCache.entries == nil || versionCache.activeRepo != repo {
		versionCache.entries = make(map[string][]AvailableVersion)
		versionCache.cachedAt = make(map[string]time.Time)
		versionCache.activeRepo = repo
	}
	versionCache.entries[repo] = copyAvailableVersions(entries)
	versionCache.cachedAt[repo] = time.Now()
}

// GetAvailableVersions returns only versions whose matching assets release and
// immutable source tag/ref were both verified in the configured repository.
func (s *GithubBuildConfigService) GetAvailableVersions(ctx context.Context) ([]AvailableVersion, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	gcfg, err := s.Get()
	if err != nil {
		return nil, fmt.Errorf("get GitHub build config: %w", err)
	}
	return s.getAvailableVersionsWithConfig(ctx, gcfg)
}

func (s *GithubBuildConfigService) getAvailableVersionsWithConfig(ctx context.Context, gcfg *model.GithubBuildConfig) ([]AvailableVersion, error) {
	if gcfg == nil {
		return nil, errors.New("GitHub build config is missing")
	}
	if err := validateGithubRepo(gcfg.Repo); err != nil {
		return nil, err
	}
	workflowIdentity, err := s.resolveWorkflowExecution(ctx, gcfg)
	if err != nil {
		return nil, err
	}
	return s.getAvailableVersionsWithWorkflowIdentity(ctx, gcfg, workflowIdentity)
}

func (s *GithubBuildConfigService) getAvailableVersionsWithWorkflowIdentity(ctx context.Context, gcfg *model.GithubBuildConfig, workflowIdentity WorkflowExecutionIdentity) ([]AvailableVersion, error) {
	if gcfg == nil {
		return nil, errors.New("GitHub build config is missing")
	}
	if err := validateGithubRepo(gcfg.Repo); err != nil {
		return nil, err
	}
	if entries, ok := s.cachedVersions(gcfg.Repo); ok {
		return applyWorkflowIdentity(entries, workflowIdentity), nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	key := "catalog:" + gcfg.Repo
	result := versionFlight.DoChan(key, func() (any, error) {
		if entries, ok := s.cachedVersions(gcfg.Repo); ok {
			return entries, nil
		}
		fetchCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		entries, fetchErr := s.fetchReleasesWithConfig(fetchCtx, gcfg, workflowIdentity)
		if fetchErr != nil {
			return nil, fetchErr
		}
		if len(entries) == 0 {
			return nil, errors.New("configured repository has no valid offline-assets releases")
		}
		s.storeVersions(gcfg.Repo, entries)
		return entries, nil
	})
	select {
	case shared := <-result:
		if shared.Err != nil {
			return nil, shared.Err
		}
		return applyWorkflowIdentity(copyAvailableVersions(shared.Val.([]AvailableVersion)), workflowIdentity), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func applyWorkflowIdentity(entries []AvailableVersion, identity WorkflowExecutionIdentity) []AvailableVersion {
	for index := range entries {
		entries[index].WorkflowRef = identity.Ref
		entries[index].WorkflowSHA = identity.SHA
	}
	return entries
}

// ResolveVersion resolves only a display version from the current configured
// repository. The caller persists the returned identity before dispatch.
func (s *GithubBuildConfigService) ResolveVersion(ctx context.Context, displayVersion string) (VersionIdentity, error) {
	if !utils.ValidateBuildVersion(displayVersion) {
		return VersionIdentity{}, &ClientValidationError{Err: errors.New("invalid display version")}
	}
	gcfg, err := s.Get()
	if err != nil {
		return VersionIdentity{}, fmt.Errorf("get GitHub build config: %w", err)
	}
	workflowIdentity, err := s.resolveWorkflowExecution(ctx, gcfg)
	if err != nil {
		return VersionIdentity{}, err
	}
	return s.resolveVersionWithConfig(ctx, gcfg, displayVersion, workflowIdentity)
}

func (s *GithubBuildConfigService) resolveVersionWithConfig(ctx context.Context, gcfg *model.GithubBuildConfig, displayVersion string, workflowIdentity WorkflowExecutionIdentity) (VersionIdentity, error) {
	if !utils.ValidateBuildVersion(displayVersion) {
		return VersionIdentity{}, &ClientValidationError{Err: errors.New("invalid display version")}
	}
	entries, err := s.getAvailableVersionsWithWorkflowIdentity(ctx, gcfg, workflowIdentity)
	if err != nil {
		return VersionIdentity{}, err
	}
	for _, entry := range entries {
		if entry.DisplayVersion == displayVersion {
			identity := entry.VersionIdentity
			if err := identity.validate(); err != nil {
				return VersionIdentity{}, err
			}
			return identity, nil
		}
	}
	return VersionIdentity{}, &ClientValidationError{Err: errors.New("selected version is not available in configured repository")}
}

type githubReleaseRecord struct {
	ID         int64  `json:"id"`
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

type githubReleaseAssetRecord struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type githubRefRecord struct {
	Ref    string `json:"ref"`
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"object"`
}

func nextGithubLink(link string) (string, bool, error) {
	for _, part := range strings.Split(link, ",") {
		segments := strings.Split(strings.TrimSpace(part), ";")
		if len(segments) < 2 || !strings.Contains(segments[1], `rel="next"`) {
			continue
		}
		value := strings.TrimSpace(segments[0])
		if len(value) < 2 || value[0] != '<' || value[len(value)-1] != '>' {
			return "", false, errors.New("invalid GitHub Link header")
		}
		u, err := url.Parse(value[1 : len(value)-1])
		if err != nil || (u.Host != "" && u.Host != "api.github.com") || u.Path == "" {
			return "", false, errors.New("unsafe GitHub pagination link")
		}
		return u.EscapedPath() + func() string {
			if u.RawQuery == "" {
				return ""
			}
			return "?" + u.RawQuery
		}(), true, nil
	}
	return "", false, nil
}

func (s *GithubBuildConfigService) fetchReleasesWithConfig(ctx context.Context, gcfg *model.GithubBuildConfig, workflowIdentity WorkflowExecutionIdentity) ([]AvailableVersion, error) {
	if gcfg == nil {
		return nil, errors.New("GitHub build config is missing")
	}
	if err := validateGithubRepo(gcfg.Repo); err != nil {
		return nil, err
	}
	path, err := githubRepoPath(gcfg.Repo, "/releases?per_page=100")
	if err != nil {
		return nil, err
	}
	var releases []githubReleaseRecord
	for page := 0; page < maxReleasePages; page++ {
		resp, err := s.ghReq(ctx, gcfg, http.MethodGet, path, nil, http.StatusOK)
		if err != nil {
			return nil, err
		}
		var pageReleases []githubReleaseRecord
		if err := decodeGithubJSON(resp, "list releases", &pageReleases); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		releases = append(releases, pageReleases...)
		next, hasNext, err := nextGithubLink(resp.Header.Get("Link"))
		if err != nil {
			return nil, &GithubContractError{Operation: "list releases", Cause: err}
		}
		if !hasNext {
			break
		}
		if page == maxReleasePages-1 {
			return nil, &GithubContractError{Operation: "list releases", Cause: fmt.Errorf("pagination exceeds %d pages", maxReleasePages)}
		}
		path = next
	}

	entries := make([]AvailableVersion, 0, len(releases))
	seen := make(map[string]struct{})
	for _, release := range releases {
		if release.Draft || release.TagName == "" || !strings.HasPrefix(release.TagName, assetsReleasePrefix) {
			continue
		}
		version := strings.TrimPrefix(release.TagName, assetsReleasePrefix)
		if !utils.ValidateBuildVersion(version) {
			continue
		}
		if release.ID <= 0 {
			return nil, &GithubContractError{Operation: "list releases", Cause: fmt.Errorf("release %q has no immutable id", release.TagName)}
		}
		if _, duplicate := seen[version]; duplicate {
			return nil, &GithubContractError{Operation: "list releases", Cause: fmt.Errorf("duplicate release for version %q", version)}
		}
		seen[version] = struct{}{}

		releasePath, err := githubRepoPath(gcfg.Repo, fmt.Sprintf("/releases/%d", release.ID))
		if err != nil {
			return nil, err
		}
		releaseResp, err := s.ghReq(ctx, gcfg, http.MethodGet, releasePath, nil, http.StatusOK)
		if err != nil {
			return nil, fmt.Errorf("resolve assets release %q: %w", version, err)
		}
		var releaseDetails struct {
			ID      int64                      `json:"id"`
			TagName string                     `json:"tag_name"`
			Assets  []githubReleaseAssetRecord `json:"assets"`
		}
		if err := decodeGithubJSON(releaseResp, "resolve assets release", &releaseDetails); err != nil {
			releaseResp.Body.Close()
			return nil, err
		}
		releaseResp.Body.Close()
		if releaseDetails.ID != release.ID || releaseDetails.TagName != release.TagName {
			return nil, &GithubContractError{Operation: "resolve assets release", Cause: fmt.Errorf("release identity changed for %q", version)}
		}
		assets := make([]ReleaseAsset, 0, len(requiredOfflineAssetNames))
		for _, requiredName := range requiredOfflineAssetNames {
			var match *githubReleaseAssetRecord
			for index := range releaseDetails.Assets {
				asset := &releaseDetails.Assets[index]
				if asset.Name != requiredName {
					continue
				}
				if match != nil {
					return nil, &GithubContractError{Operation: "resolve assets release", Cause: fmt.Errorf("asset %q is ambiguous", requiredName)}
				}
				match = asset
			}
			if match == nil {
				return nil, &GithubContractError{Operation: "resolve assets release", Cause: fmt.Errorf("required asset %q is missing", requiredName)}
			}
			assets = append(assets, ReleaseAsset{ID: match.ID, Name: match.Name, Digest: match.Digest})
		}
		if err := validateReleaseAssets(assets); err != nil {
			return nil, &GithubContractError{Operation: "resolve assets release", Cause: err}
		}

		sourcePath, err := githubRepoPath(gcfg.Repo, "/git/ref/tags/"+url.PathEscape(version))
		if err != nil {
			return nil, err
		}
		resp, err := s.ghReq(ctx, gcfg, http.MethodGet, sourcePath, nil, http.StatusOK)
		if err != nil {
			return nil, fmt.Errorf("resolve source tag %q: %w", version, err)
		}
		var ref githubRefRecord
		if err := decodeGithubJSON(resp, "resolve source tag", &ref); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		expectedRef := "refs/tags/" + version
		if ref.Ref != expectedRef || ref.Object.Type != "commit" || !validGithubSourceSHA(ref.Object.SHA) || len(ref.Object.SHA) > githubSourceRefLimit {
			return nil, &GithubContractError{Operation: "resolve source tag", Cause: fmt.Errorf("tag %q did not resolve to an immutable source SHA", version)}
		}
		entry := AvailableVersion{VersionIdentity: VersionIdentity{
			Repo:           gcfg.Repo,
			DisplayVersion: version,
			BuildRef:       ref.Object.SHA,
			SourceTag:      version,
			WorkflowRef:    workflowIdentity.Ref,
			WorkflowSHA:    workflowIdentity.SHA,
			AssetsRelease:  AssetsRelease{ID: release.ID, TagName: release.TagName, Name: release.Name, Assets: assets},
		}}
		if err := entry.VersionIdentity.validate(); err != nil {
			return nil, &GithubContractError{Operation: "resolve version identity", Cause: err}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return compareSemver(entries[i].DisplayVersion, entries[j].DisplayVersion) > 0
	})
	return entries, nil
}
