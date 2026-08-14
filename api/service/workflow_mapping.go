package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"rustdesk-server/api/config"
	"rustdesk-server/api/global"
	"rustdesk-server/api/model"
)

const (
	windowsWorkflowFilename = "rustqs-windows-min-test.yml"
	linuxWorkflowFilename   = "rustqs-linux.yml"
	androidWorkflowFilename = "rustqs-android.yml"
	// This is retained only for read-only catalog/version health compatibility.
	// Production approval and dispatch never accept this mutable branch.
	defaultWorkflowExecutionRef = "rustqs/min-test"
)

// normalizeKnownWorkflowExecutionRef translates only representations that were
// used by DeskForge before the workflow selector became application-owned. It
// deliberately leaves unknown values untouched so workflowExecutionRef can
// reject them rather than silently expanding repository/workflow ownership.
func normalizeKnownWorkflowExecutionRef(ref string) string {
	ref = strings.TrimSpace(ref)
	switch ref {
	case "", defaultWorkflowExecutionRef, "refs/heads/" + defaultWorkflowExecutionRef,
		"master", "refs/heads/master":
		return defaultWorkflowExecutionRef
	default:
		return ref
	}
}

func normalizePersistedWorkflowExecutionRef(config *model.GithubBuildConfig) {
	if config == nil {
		return
	}
	config.Branch = normalizeKnownWorkflowExecutionRef(config.Branch)
}

func workflowExecutionRef(config *model.GithubBuildConfig) (string, error) {
	if config == nil {
		return "", errors.New("GitHub build config is missing")
	}
	ref := normalizeKnownWorkflowExecutionRef(config.Branch)
	if ref == "" {
		ref = defaultWorkflowExecutionRef
	}
	if !validGithubWorkflowRef(ref) || (ref != defaultWorkflowExecutionRef && !strings.HasPrefix(ref, "refs/tags/")) {
		return "", fmt.Errorf("configured workflow execution ref %q is invalid", ref)
	}
	return ref, nil
}

// WorkflowRefApprovalError means the operator attestation required before a
// workflow-backed build is persisted or dispatched is missing or invalid.
// Reason is internal and must not be returned directly by HTTP controllers.
type WorkflowRefApprovalError struct {
	Reason string
}

func (e *WorkflowRefApprovalError) Error() string {
	if e == nil || e.Reason == "" {
		return "workflow reference approval is required"
	}
	return "workflow reference approval is required: " + e.Reason
}

// approvedWorkflowRef validates the deliberately closed operator approval
// domain. Only a provider-derived fully-qualified tag selector is acceptable;
// the legacy default branch is intentionally excluded from production policy.
func approvedWorkflowRef(selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if !strings.HasPrefix(selector, "refs/tags/") || validGithubSourceSHA(selector) || !validGithubWorkflowRef(selector) {
		return "", &WorkflowRefApprovalError{Reason: "selector is outside the approved workflow reference domain"}
	}
	if validGithubSourceSHA(strings.TrimPrefix(selector, "refs/tags/")) {
		return "", &WorkflowRefApprovalError{Reason: "raw SHA selectors are not approved"}
	}
	return selector, nil
}

// RequireWorkflowRefApproval is the shared deployment gate for provider-backed
// builds. It validates the current selector as well as the persisted approval
// bit, so malformed or changed compatibility values fail closed.
func (s *GithubBuildConfigService) RequireWorkflowRefApproval(config *model.GithubBuildConfig) error {
	ref, err := workflowExecutionRef(config)
	if err != nil {
		return &WorkflowRefApprovalError{Reason: "configured selector is invalid"}
	}
	if config == nil || !config.WorkflowRefApproved || !config.WorkflowRefProviderVerified || !validGithubSourceSHA(config.WorkflowRefApprovalSHA) {
		return &WorkflowRefApprovalError{Reason: fmt.Sprintf("selector %q has not been approved", ref)}
	}
	if _, err := approvedWorkflowRef(ref); err != nil {
		return err
	}
	return nil
}

// WorkflowTagOption is the safe admin-facing representation of a provider
// candidate. The tag label is the only value a client may select; refs, SHAs,
// verification objects, and credentials remain inside the provider boundary.
type WorkflowTagOption struct {
	Tag   string `json:"tag"`
	Label string `json:"label"`
}

func workflowTagLabel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "refs/tags/") {
		return "", &WorkflowRefApprovalError{Reason: "workflow tag must be selected by its provider-derived label"}
	}
	selector := "refs/tags/" + value
	if value == "" || !validGithubWorkflowRef(selector) || validGithubSourceSHA(value) {
		return "", &WorkflowRefApprovalError{Reason: "workflow tag is outside the provider-derived domain"}
	}
	return value, nil
}

// ApproveWorkflowTag resolves and verifies a provider-derived annotated tag,
// confirms the current owned workflow is ready at that exact commit, and then
// writes selector, SHA, and approval metadata in one repo-scoped update.
func (s *GithubBuildConfigService) ApproveWorkflowTag(tag string) (*model.GithubBuildConfig, error) {
	config, err := s.Get()
	if err != nil {
		return nil, err
	}
	if err := validateGithubRepo(config.Repo); err != nil {
		return nil, &WorkflowRefApprovalError{Reason: "configured repository is invalid"}
	}
	tag, err = workflowTagLabel(tag)
	if err != nil {
		return nil, err
	}
	ref := "refs/tags/" + tag
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	policyConfig := *config
	policyConfig.Branch = ref
	execution, err := s.validateWorkflowRefPolicy(ctx, &policyConfig)
	if err != nil {
		return nil, fmt.Errorf("validate workflow reference policy: %w", err)
	}
	workflow, err := WorkflowFilenameForPlatform(string(PlatformWindows))
	if err != nil {
		return nil, err
	}
	if err := s.verifyWorkflowAvailable(ctx, &policyConfig, workflow, execution.SHA); err != nil {
		return nil, fmt.Errorf("validate workflow readiness: %w", err)
	}
	if err := updateGithubBuildConfigMetadata(config.Id, config.Repo, map[string]any{
		"branch":                         ref,
		"workflow_ref_approved":          true,
		"workflow_ref_provider_verified": true,
		"workflow_ref_approval_sha":      execution.SHA,
	}); err != nil {
		return nil, err
	}
	config.Branch = ref
	config.WorkflowRefApproved = true
	config.WorkflowRefProviderVerified = true
	config.WorkflowRefApprovalSHA = execution.SHA
	return config, nil
}

// ApproveWorkflowRef is retained as an internal compatibility wrapper for
// callers that already hold the full provider selector. HTTP clients use the
// label-only ApproveWorkflowTag contract instead.
func (s *GithubBuildConfigService) ApproveWorkflowRef(selector string) (*model.GithubBuildConfig, error) {
	if !strings.HasPrefix(strings.TrimSpace(selector), "refs/tags/") {
		return nil, &WorkflowRefApprovalError{Reason: "only provider-derived tag selectors may be approved"}
	}
	return s.ApproveWorkflowTag(strings.TrimPrefix(strings.TrimSpace(selector), "refs/tags/"))
}

func workflowDispatchRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	return strings.TrimPrefix(ref, "refs/tags/")
}

// WorkflowExecutionIdentity is the two-part identity needed by GitHub: Ref
// selects the workflow definition for dispatch, while SHA pins the commit that
// the resulting run must execute.
type WorkflowExecutionIdentity struct {
	Ref                string
	SHA                string
	VerificationReason string
	TrustStatus        string
}

type githubTagObjectRecord struct {
	SHA    string `json:"sha"`
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"object"`
	Verification struct {
		Verified bool   `json:"verified"`
		Reason   string `json:"reason"`
	} `json:"verification"`
}

type githubTagProtectionRecord struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Enabled   bool      `json:"enabled"`
	Pattern   string    `json:"pattern"`
}

func (r *githubTagProtectionRecord) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var record struct {
		ID        *int64     `json:"id"`
		CreatedAt *time.Time `json:"created_at"`
		UpdatedAt *time.Time `json:"updated_at"`
		Enabled   *bool      `json:"enabled"`
		Pattern   *string    `json:"pattern"`
	}
	if err := decoder.Decode(&record); err != nil {
		return err
	}
	if record.Pattern == nil {
		return errors.New("legacy tag protection record is missing required pattern")
	}
	if record.ID != nil {
		r.ID = *record.ID
	}
	if record.CreatedAt != nil {
		r.CreatedAt = *record.CreatedAt
	}
	if record.UpdatedAt != nil {
		r.UpdatedAt = *record.UpdatedAt
	}
	if record.Enabled != nil {
		r.Enabled = *record.Enabled
	}
	r.Pattern = *record.Pattern
	return nil
}

type githubRulesetBypassActor struct {
	ActorID    *int64 `json:"actor_id"`
	ActorType  string `json:"actor_type"`
	BypassMode string `json:"bypass_mode"`
}

func (a *githubRulesetBypassActor) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var raw struct {
		ActorID    json.RawMessage `json:"actor_id"`
		ActorType  string          `json:"actor_type"`
		BypassMode string          `json:"bypass_mode"`
	}
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.ActorID == nil {
		return errors.New("ruleset bypass actor is missing actor_id")
	}
	if string(raw.ActorID) != "null" {
		var actorID int64
		if err := json.Unmarshal(raw.ActorID, &actorID); err != nil {
			return fmt.Errorf("decode actor_id: %w", err)
		}
		a.ActorID = &actorID
	} else {
		a.ActorID = nil
	}
	if raw.ActorType == "" {
		return errors.New("ruleset bypass actor is missing actor_type")
	}
	switch raw.ActorType {
	case "Integration", "OrganizationAdmin", "RepositoryRole", "Team", "DeployKey", "User":
	default:
		return fmt.Errorf("unsupported ruleset bypass actor type %q", raw.ActorType)
	}
	switch raw.BypassMode {
	case "always", "pull_request", "exempt":
	default:
		return fmt.Errorf("unsupported ruleset bypass mode %q", raw.BypassMode)
	}
	a.ActorType = raw.ActorType
	a.BypassMode = raw.BypassMode
	return nil
}

type githubRulesetRefNameCondition struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type githubRulesetRepositoryNameCondition struct {
	Include   []string `json:"include"`
	Exclude   []string `json:"exclude"`
	Protected *bool    `json:"protected"`
}

type githubRulesetRepositoryIDCondition struct {
	RepositoryIDs []int64 `json:"repository_ids"`
}

func (c *githubRulesetRepositoryIDCondition) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var raw struct {
		RepositoryIDs json.RawMessage `json:"repository_ids"`
	}
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if len(raw.RepositoryIDs) == 0 || string(raw.RepositoryIDs) == "null" {
		return errors.New("repository_id.repository_ids is required")
	}
	var repositoryIDs []int64
	if err := json.Unmarshal(raw.RepositoryIDs, &repositoryIDs); err != nil {
		return fmt.Errorf("decode repository_id.repository_ids: %w", err)
	}
	if len(repositoryIDs) == 0 {
		return errors.New("repository_id.repository_ids must not be empty")
	}
	for _, repositoryID := range repositoryIDs {
		if repositoryID <= 0 {
			return errors.New("repository_id.repository_ids must contain positive IDs")
		}
	}
	c.RepositoryIDs = repositoryIDs
	return nil
}

type githubRulesetRepositoryProperty struct {
	Name           string   `json:"name"`
	PropertyValues []string `json:"property_values"`
	Source         string   `json:"source"`
}

func (p *githubRulesetRepositoryProperty) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var raw struct {
		Name           *string         `json:"name"`
		PropertyValues json.RawMessage `json:"property_values"`
		Source         *string         `json:"source"`
	}
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if raw.Name == nil || strings.TrimSpace(*raw.Name) == "" {
		return errors.New("repository_property entry name is required")
	}
	if len(raw.PropertyValues) == 0 || string(raw.PropertyValues) == "null" {
		return errors.New("repository_property entry property_values is required")
	}
	var propertyValues []string
	if err := json.Unmarshal(raw.PropertyValues, &propertyValues); err != nil {
		return fmt.Errorf("decode repository_property entry property_values: %w", err)
	}
	if err := validateNonEmptyStrings(propertyValues, "repository_property entry property_values"); err != nil {
		return err
	}
	source := "custom"
	if raw.Source != nil {
		if strings.TrimSpace(*raw.Source) == "" {
			return errors.New("repository_property entry source must not be empty")
		}
		source = *raw.Source
	}
	switch source {
	case "custom", "system":
	default:
		return fmt.Errorf("unsupported repository_property entry source %q", source)
	}
	p.Name = *raw.Name
	p.PropertyValues = propertyValues
	p.Source = source
	return nil
}

type githubRulesetRepositoryPropertyCondition struct {
	Include []githubRulesetRepositoryProperty `json:"include"`
	Exclude []githubRulesetRepositoryProperty `json:"exclude"`
}

func (c *githubRulesetRepositoryPropertyCondition) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var raw struct {
		Include json.RawMessage `json:"include"`
		Exclude json.RawMessage `json:"exclude"`
	}
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	decodeProperties := func(data json.RawMessage, field string) ([]githubRulesetRepositoryProperty, error) {
		if len(data) == 0 || string(data) == "null" {
			if string(data) == "null" {
				return nil, fmt.Errorf("repository_property.%s must be an array", field)
			}
			return nil, nil
		}
		var properties []githubRulesetRepositoryProperty
		if err := json.Unmarshal(data, &properties); err != nil {
			return nil, fmt.Errorf("decode repository_property.%s: %w", field, err)
		}
		return properties, nil
	}
	include, err := decodeProperties(raw.Include, "include")
	if err != nil {
		return err
	}
	exclude, err := decodeProperties(raw.Exclude, "exclude")
	if err != nil {
		return err
	}
	if len(include) == 0 && len(exclude) == 0 {
		return errors.New("repository_property must contain an include or exclude entry")
	}
	c.Include = include
	c.Exclude = exclude
	return nil
}

func decodeStrictGithubRulesetCondition(data []byte, conditionName string, allowProtected bool) ([]string, []string, *bool, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var raw struct {
		Include   json.RawMessage `json:"include"`
		Exclude   json.RawMessage `json:"exclude"`
		Protected json.RawMessage `json:"protected"`
	}
	if err := decoder.Decode(&raw); err != nil {
		return nil, nil, nil, err
	}
	if len(raw.Protected) != 0 && !allowProtected {
		return nil, nil, nil, errors.New("protected is not valid for this ruleset condition")
	}
	decodePatterns := func(data json.RawMessage, field string) ([]string, error) {
		if len(data) == 0 {
			return nil, nil
		}
		if string(data) == "null" {
			return nil, fmt.Errorf("%s.%s must be an array", conditionName, field)
		}
		var patterns []string
		if err := json.Unmarshal(data, &patterns); err != nil {
			return nil, fmt.Errorf("decode %s.%s: %w", conditionName, field, err)
		}
		return patterns, nil
	}
	include, err := decodePatterns(raw.Include, "include")
	if err != nil {
		return nil, nil, nil, err
	}
	exclude, err := decodePatterns(raw.Exclude, "exclude")
	if err != nil {
		return nil, nil, nil, err
	}
	var protected *bool
	if len(raw.Protected) != 0 {
		if string(raw.Protected) == "null" {
			return nil, nil, nil, fmt.Errorf("%s.protected must be a boolean", conditionName)
		}
		var value bool
		if err := json.Unmarshal(raw.Protected, &value); err != nil {
			return nil, nil, nil, fmt.Errorf("decode %s.protected: %w", conditionName, err)
		}
		protected = &value
	}
	return include, exclude, protected, nil
}

func (c *githubRulesetRefNameCondition) UnmarshalJSON(data []byte) error {
	include, exclude, _, err := decodeStrictGithubRulesetCondition(data, "ref_name", false)
	if err != nil {
		return err
	}
	c.Include = include
	c.Exclude = exclude
	return nil
}

func (c *githubRulesetRepositoryNameCondition) UnmarshalJSON(data []byte) error {
	include, exclude, protected, err := decodeStrictGithubRulesetCondition(data, "repository_name", true)
	if err != nil {
		return err
	}
	hasNonEmptyPattern := func(patterns []string) bool {
		for _, pattern := range patterns {
			if strings.TrimSpace(pattern) != "" {
				return true
			}
		}
		return false
	}
	if !hasNonEmptyPattern(include) && !hasNonEmptyPattern(exclude) {
		return errors.New("repository_name must contain at least one non-empty include or exclude pattern")
	}
	c.Include = include
	c.Exclude = exclude
	c.Protected = protected
	return nil
}

type githubRulesetRuleParameters struct {
	Name     string `json:"name"`
	Negate   bool   `json:"negate"`
	Operator string `json:"operator"`
	Pattern  string `json:"pattern"`
}

type githubRulesetConditions struct {
	RefName            *githubRulesetRefNameCondition            `json:"ref_name"`
	RepositoryName     *githubRulesetRepositoryNameCondition     `json:"repository_name"`
	RepositoryID       *githubRulesetRepositoryIDCondition       `json:"repository_id"`
	RepositoryProperty *githubRulesetRepositoryPropertyCondition `json:"repository_property"`
}

func (c *githubRulesetConditions) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var raw struct {
		RefName            json.RawMessage `json:"ref_name"`
		RepositoryName     json.RawMessage `json:"repository_name"`
		RepositoryID       json.RawMessage `json:"repository_id"`
		RepositoryProperty json.RawMessage `json:"repository_property"`
	}
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	c.RefName = nil
	c.RepositoryName = nil
	c.RepositoryID = nil
	c.RepositoryProperty = nil
	if len(raw.RefName) != 0 {
		if string(raw.RefName) == "null" {
			return errors.New("conditions.ref_name must be an object")
		}
		var refName githubRulesetRefNameCondition
		if err := json.Unmarshal(raw.RefName, &refName); err != nil {
			return fmt.Errorf("decode conditions.ref_name: %w", err)
		}
		c.RefName = &refName
	}
	if len(raw.RepositoryName) != 0 {
		if string(raw.RepositoryName) == "null" {
			return errors.New("conditions.repository_name must be an object")
		}
		var repositoryName githubRulesetRepositoryNameCondition
		if err := json.Unmarshal(raw.RepositoryName, &repositoryName); err != nil {
			return fmt.Errorf("decode conditions.repository_name: %w", err)
		}
		c.RepositoryName = &repositoryName
	}
	if len(raw.RepositoryID) != 0 {
		if string(raw.RepositoryID) == "null" {
			return errors.New("conditions.repository_id must be an object")
		}
		var repositoryID githubRulesetRepositoryIDCondition
		if err := json.Unmarshal(raw.RepositoryID, &repositoryID); err != nil {
			return fmt.Errorf("decode conditions.repository_id: %w", err)
		}
		c.RepositoryID = &repositoryID
	}
	if len(raw.RepositoryProperty) != 0 {
		if string(raw.RepositoryProperty) == "null" {
			return errors.New("conditions.repository_property must be an object")
		}
		var repositoryProperty githubRulesetRepositoryPropertyCondition
		if err := json.Unmarshal(raw.RepositoryProperty, &repositoryProperty); err != nil {
			return fmt.Errorf("decode conditions.repository_property: %w", err)
		}
		c.RepositoryProperty = &repositoryProperty
	}
	return nil
}

type githubRulesetRule struct {
	Type                      string                       `json:"type"`
	Parameters                *githubRulesetRuleParameters `json:"parameters"`
	ParametersPresent         bool                         `json:"-"`
	UpdateAllowsFetchAndMerge *bool                        `json:"-"`
}

type githubRulesetUpdateParameters struct {
	UpdateAllowsFetchAndMerge *bool `json:"update_allows_fetch_and_merge"`
}

type githubRulesetRequiredDeploymentsParameters struct {
	RequiredDeploymentEnvironments *[]string `json:"required_deployment_environments"`
}

type githubRulesetWorkflowParameter struct {
	Path         *string `json:"path"`
	Ref          *string `json:"ref"`
	RepositoryID *int64  `json:"repository_id"`
	SHA          *string `json:"sha"`
}

type githubRulesetWorkflowsParameters struct {
	DoNotEnforceOnCreate *bool                             `json:"do_not_enforce_on_create"`
	Workflows            *[]githubRulesetWorkflowParameter `json:"workflows"`
}

type githubRulesetStatusCheck struct {
	Context       *string `json:"context"`
	IntegrationID *int64  `json:"integration_id"`
}

type githubRulesetRequiredStatusChecksParameters struct {
	DoNotEnforceOnCreate             *bool                       `json:"do_not_enforce_on_create"`
	RequiredStatusChecks             *[]githubRulesetStatusCheck `json:"required_status_checks"`
	StrictRequiredStatusChecksPolicy *bool                       `json:"strict_required_status_checks_policy"`
}

type githubRulesetDismissalActor struct {
	ID   *int64  `json:"id"`
	Type *string `json:"type"`
}

type githubRulesetDismissalRestriction struct {
	AllowedActors *[]githubRulesetDismissalActor `json:"allowed_actors"`
	Enabled       *bool                          `json:"enabled"`
}

type githubRulesetRequiredReviewer struct {
	FilePatterns     *[]string `json:"file_patterns"`
	MinimumApprovals *int      `json:"minimum_approvals"`
	Reviewer         *struct {
		ID   *int64  `json:"id"`
		Type *string `json:"type"`
	} `json:"reviewer"`
}

type githubRulesetPullRequestParameters struct {
	AllowedMergeMethods            *[]string                          `json:"allowed_merge_methods"`
	DismissStaleReviewsOnPush      *bool                              `json:"dismiss_stale_reviews_on_push"`
	DismissalRestriction           *githubRulesetDismissalRestriction `json:"dismissal_restriction"`
	RequireCodeOwnerReview         *bool                              `json:"require_code_owner_review"`
	RequireLastPushApproval        *bool                              `json:"require_last_push_approval"`
	RequiredApprovingReviewCount   *int                               `json:"required_approving_review_count"`
	RequiredReviewThreadResolution *bool                              `json:"required_review_thread_resolution"`
	RequiredReviewers              *[]githubRulesetRequiredReviewer   `json:"required_reviewers"`
}

type githubRulesetMergeQueueParameters struct {
	CheckResponseTimeoutMinutes  *int    `json:"check_response_timeout_minutes"`
	GroupingStrategy             *string `json:"grouping_strategy"`
	MaxEntriesToBuild            *int    `json:"max_entries_to_build"`
	MaxEntriesToMerge            *int    `json:"max_entries_to_merge"`
	MergeMethod                  *string `json:"merge_method"`
	MinEntriesToMerge            *int    `json:"min_entries_to_merge"`
	MinEntriesToMergeWaitMinutes *int    `json:"min_entries_to_merge_wait_minutes"`
}

type githubRulesetCopilotCodeReviewParameters struct {
	ReviewDraftPullRequests *bool `json:"review_draft_pull_requests"`
	ReviewOnPush            *bool `json:"review_on_push"`
}

type githubRulesetCodeScanningTool struct {
	AlertsThreshold         *string `json:"alerts_threshold"`
	SecurityAlertsThreshold *string `json:"security_alerts_threshold"`
	Tool                    *string `json:"tool"`
}

type githubRulesetCodeScanningParameters struct {
	CodeScanningTools *[]githubRulesetCodeScanningTool `json:"code_scanning_tools"`
}

type githubRulesetFilePathRestrictionParameters struct {
	RestrictedFilePaths *[]string `json:"restricted_file_paths"`
}

type githubRulesetMaxFilePathLengthParameters struct {
	MaxFilePathLength *int `json:"max_file_path_length"`
}

type githubRulesetFileExtensionRestrictionParameters struct {
	RestrictedFileExtensions *[]string `json:"restricted_file_extensions"`
}

type githubRulesetMaxFileSizeParameters struct {
	MaxFileSize *int `json:"max_file_size"`
}

func decodeStrictGithubRulesetParameters(data json.RawMessage, ruleType string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s parameters: %w", ruleType, err)
	}
	return nil
}

func validateNonEmptyStrings(values []string, field string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", field)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty value", field)
		}
	}
	return nil
}

func validateGithubRulesetParameters(ruleType string, data json.RawMessage) error {
	switch ruleType {
	case "required_deployments":
		var parameters githubRulesetRequiredDeploymentsParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.RequiredDeploymentEnvironments == nil {
			return errors.New("required_deployment_environments is required")
		}
		return validateNonEmptyStrings(*parameters.RequiredDeploymentEnvironments, "required_deployment_environments")
	case "pull_request":
		var parameters githubRulesetPullRequestParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.DismissStaleReviewsOnPush == nil || parameters.RequireCodeOwnerReview == nil || parameters.RequireLastPushApproval == nil || parameters.RequiredApprovingReviewCount == nil || parameters.RequiredReviewThreadResolution == nil {
			return errors.New("pull_request parameters are missing a required field")
		}
		if parameters.AllowedMergeMethods != nil {
			if err := validateNonEmptyStrings(*parameters.AllowedMergeMethods, "allowed_merge_methods"); err != nil {
				return err
			}
			for _, method := range *parameters.AllowedMergeMethods {
				switch method {
				case "merge", "squash", "rebase":
				default:
					return fmt.Errorf("unsupported allowed merge method %q", method)
				}
			}
		}
		if restriction := parameters.DismissalRestriction; restriction != nil {
			if restriction.Enabled == nil || restriction.AllowedActors == nil {
				return errors.New("dismissal_restriction is missing a required field")
			}
			for _, actor := range *restriction.AllowedActors {
				if actor.ID == nil || *actor.ID <= 0 || actor.Type == nil {
					return errors.New("dismissal_restriction contains an invalid allowed actor")
				}
				switch *actor.Type {
				case "User", "Team", "IntegrationInstallation", "RepositoryRole":
				default:
					return fmt.Errorf("unsupported dismissal actor type %q", *actor.Type)
				}
			}
		}
		if reviewers := parameters.RequiredReviewers; reviewers != nil {
			for _, reviewer := range *reviewers {
				if reviewer.FilePatterns == nil || reviewer.MinimumApprovals == nil || reviewer.Reviewer == nil || reviewer.Reviewer.ID == nil || reviewer.Reviewer.Type == nil {
					return errors.New("required_reviewers contains a missing required field")
				}
				if err := validateNonEmptyStrings(*reviewer.FilePatterns, "required reviewer file_patterns"); err != nil {
					return err
				}
				if *reviewer.Reviewer.ID <= 0 || *reviewer.Reviewer.Type != "Team" {
					return errors.New("required_reviewers contains an invalid reviewer")
				}
			}
		}
	case "required_status_checks":
		var parameters githubRulesetRequiredStatusChecksParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.RequiredStatusChecks == nil || parameters.StrictRequiredStatusChecksPolicy == nil {
			return errors.New("required_status_checks parameters are missing a required field")
		}
		if len(*parameters.RequiredStatusChecks) == 0 {
			return errors.New("required_status_checks must not be empty")
		}
		for _, check := range *parameters.RequiredStatusChecks {
			if check.Context == nil || strings.TrimSpace(*check.Context) == "" {
				return errors.New("required_status_checks contains an empty context")
			}
		}
	case "workflows":
		var parameters githubRulesetWorkflowsParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.Workflows == nil || len(*parameters.Workflows) == 0 {
			return errors.New("workflows must not be empty")
		}
		for _, workflow := range *parameters.Workflows {
			if workflow.Path == nil || strings.TrimSpace(*workflow.Path) == "" || workflow.RepositoryID == nil || *workflow.RepositoryID <= 0 {
				return errors.New("workflows contains a missing required field")
			}
		}
	case "merge_queue":
		var parameters githubRulesetMergeQueueParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.CheckResponseTimeoutMinutes == nil || parameters.GroupingStrategy == nil || parameters.MaxEntriesToBuild == nil || parameters.MaxEntriesToMerge == nil || parameters.MergeMethod == nil || parameters.MinEntriesToMerge == nil || parameters.MinEntriesToMergeWaitMinutes == nil {
			return errors.New("merge_queue parameters are missing a required field")
		}
		switch *parameters.GroupingStrategy {
		case "ALLGREEN", "HEADGREEN":
		default:
			return fmt.Errorf("unsupported merge queue grouping strategy %q", *parameters.GroupingStrategy)
		}
		switch *parameters.MergeMethod {
		case "MERGE", "SQUASH", "REBASE":
		default:
			return fmt.Errorf("unsupported merge queue merge method %q", *parameters.MergeMethod)
		}
	case "copilot_code_review":
		var parameters githubRulesetCopilotCodeReviewParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.ReviewDraftPullRequests == nil && parameters.ReviewOnPush == nil {
			return errors.New("copilot_code_review parameters must not be empty")
		}
	case "code_scanning":
		var parameters githubRulesetCodeScanningParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.CodeScanningTools == nil || len(*parameters.CodeScanningTools) == 0 {
			return errors.New("code_scanning_tools must not be empty")
		}
		for _, tool := range *parameters.CodeScanningTools {
			if tool.AlertsThreshold == nil || tool.SecurityAlertsThreshold == nil || tool.Tool == nil || strings.TrimSpace(*tool.Tool) == "" {
				return errors.New("code_scanning_tools contains a missing required field")
			}
			switch *tool.AlertsThreshold {
			case "none", "errors", "errors_and_warnings", "all":
			default:
				return fmt.Errorf("unsupported code scanning alerts threshold %q", *tool.AlertsThreshold)
			}
			switch *tool.SecurityAlertsThreshold {
			case "none", "critical", "high_or_higher", "medium_or_higher", "all":
			default:
				return fmt.Errorf("unsupported code scanning security alerts threshold %q", *tool.SecurityAlertsThreshold)
			}
		}
	case "file_path_restriction":
		var parameters githubRulesetFilePathRestrictionParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.RestrictedFilePaths == nil {
			return errors.New("restricted_file_paths is required")
		}
		return validateNonEmptyStrings(*parameters.RestrictedFilePaths, "restricted_file_paths")
	case "max_file_path_length":
		var parameters githubRulesetMaxFilePathLengthParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.MaxFilePathLength == nil {
			return errors.New("max_file_path_length is required")
		}
	case "file_extension_restriction":
		var parameters githubRulesetFileExtensionRestrictionParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.RestrictedFileExtensions == nil {
			return errors.New("restricted_file_extensions is required")
		}
		return validateNonEmptyStrings(*parameters.RestrictedFileExtensions, "restricted_file_extensions")
	case "max_file_size":
		var parameters githubRulesetMaxFileSizeParameters
		if err := decodeStrictGithubRulesetParameters(data, ruleType, &parameters); err != nil {
			return err
		}
		if parameters.MaxFileSize == nil {
			return errors.New("max_file_size is required")
		}
	default:
		return fmt.Errorf("unsupported parameterized ruleset rule type %q", ruleType)
	}
	return nil
}

func (r *githubRulesetRule) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type       string          `json:"type"`
		Parameters json.RawMessage `json:"parameters"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	r.Type = raw.Type
	if !githubRulesetRuleTypeAllowed(r.Type) {
		return fmt.Errorf("unsupported or missing ruleset rule type %q", r.Type)
	}
	r.Parameters = nil
	r.UpdateAllowsFetchAndMerge = nil
	r.ParametersPresent = len(raw.Parameters) != 0
	if r.ParametersPresent && string(raw.Parameters) == "null" {
		return errors.New("ruleset rule parameters cannot be null")
	}
	if !r.ParametersPresent {
		if r.Type == "tag_name_pattern" || rulesetRuleRequiresParameters(r.Type) {
			return fmt.Errorf("ruleset rule %q requires parameters", r.Type)
		}
		return nil
	}
	var parameters githubRulesetRuleParameters
	parametersDecoder := json.NewDecoder(strings.NewReader(string(raw.Parameters)))
	parametersDecoder.DisallowUnknownFields()
	switch raw.Type {
	case "tag_name_pattern", "branch_name_pattern", "commit_author_email_pattern", "commit_message_pattern", "committer_email_pattern":
		if err := parametersDecoder.Decode(&parameters); err != nil {
			return fmt.Errorf("decode %s parameters: %w", raw.Type, err)
		}
		if !validGithubTagNamePattern(parameters.Operator, parameters.Pattern) {
			return fmt.Errorf("%s parameters contain an unsupported operator or pattern", raw.Type)
		}
	case "update":
		var updateParameters githubRulesetUpdateParameters
		if err := parametersDecoder.Decode(&updateParameters); err != nil {
			return fmt.Errorf("decode update parameters: %w", err)
		}
		if updateParameters.UpdateAllowsFetchAndMerge == nil {
			return errors.New("decode update parameters: update_allows_fetch_and_merge is required")
		}
		r.UpdateAllowsFetchAndMerge = updateParameters.UpdateAllowsFetchAndMerge
	case "creation", "deletion", "required_linear_history", "required_signatures", "non_fast_forward", "license_compliance_scanning":
		return fmt.Errorf("ruleset rule %q does not accept parameters", raw.Type)
	default:
		if err := validateGithubRulesetParameters(raw.Type, raw.Parameters); err != nil {
			return fmt.Errorf("decode %s parameters: %w", raw.Type, err)
		}
	}
	r.Parameters = &parameters
	return nil
}

// githubRulesetRuleTypeAllowed is intentionally explicit. Ruleset evaluation
// must reject a provider rule added outside this reviewed domain rather than
// silently treating it as irrelevant to tag protection.
func githubRulesetRuleTypeAllowed(ruleType string) bool {
	switch ruleType {
	case "creation", "update", "deletion", "required_linear_history", "merge_queue",
		"required_deployments", "required_signatures", "pull_request", "required_status_checks",
		"non_fast_forward", "workflows", "copilot_code_review", "code_scanning",
		"commit_author_email_pattern", "commit_message_pattern", "committer_email_pattern",
		"branch_name_pattern", "tag_name_pattern", "license_compliance_scanning",
		"file_path_restriction", "max_file_path_length", "file_extension_restriction", "max_file_size":
		return true
	default:
		return false
	}
}

func rulesetRuleRequiresParameters(ruleType string) bool {
	switch ruleType {
	case "update", "required_deployments", "pull_request", "required_status_checks", "workflows",
		"merge_queue", "copilot_code_review", "code_scanning", "file_path_restriction",
		"max_file_path_length", "file_extension_restriction", "max_file_size",
		"commit_author_email_pattern", "commit_message_pattern", "committer_email_pattern",
		"branch_name_pattern":
		return true
	default:
		return false
	}
}

type githubRepositoryRulesetRecord struct {
	ID                   int64                       `json:"id"`
	Target               string                      `json:"target"`
	Enforcement          string                      `json:"enforcement"`
	SourceType           string                      `json:"source_type"`
	Source               string                      `json:"source"`
	BypassActors         *[]githubRulesetBypassActor `json:"bypass_actors"`
	CurrentUserCanBypass *string                     `json:"current_user_can_bypass"`
	Conditions           *githubRulesetConditions    `json:"conditions"`
	Rules                []githubRulesetRule         `json:"rules"`
}

type githubRulesetSummary struct {
	ID int64 `json:"id"`
}

type githubRepositoryRulesetDetail struct {
	ID                   *int64          `json:"id"`
	Name                 *string         `json:"name"`
	Target               *string         `json:"target"`
	Enforcement          *string         `json:"enforcement"`
	SourceType           *string         `json:"source_type"`
	Source               *string         `json:"source"`
	CreatedAt            json.RawMessage `json:"created_at"`
	UpdatedAt            json.RawMessage `json:"updated_at"`
	BypassActors         json.RawMessage `json:"bypass_actors"`
	CurrentUserCanBypass json.RawMessage `json:"current_user_can_bypass"`
	Conditions           json.RawMessage `json:"conditions"`
	Rules                json.RawMessage `json:"rules"`
}

func validateOptionalGithubRulesetTimestamp(data json.RawMessage, field string) error {
	if len(data) == 0 {
		return nil
	}
	if string(data) == "null" {
		return fmt.Errorf("ruleset detail %s must be a date-time string", field)
	}
	var timestamp time.Time
	if err := json.Unmarshal(data, &timestamp); err != nil {
		return fmt.Errorf("decode ruleset detail %s: %w", field, err)
	}
	return nil
}

const (
	maxProtectedTagPatterns       = 256
	maxRulesetRecords             = 256
	maxRulesetRules               = 64
	maxProtectedTagPages          = 3
	githubTrustStatusProvider     = "provider-reported"
	githubVerificationReasonValid = "valid"
)

// githubTagPatternMatches implements the small glob domain GitHub exposes for
// tag-protection patterns. A pattern may be returned as a tag label or as a
// fully-qualified refs/tags selector; neither form is treated as a raw manual
// selector outside the provider boundary.
func githubTagPatternMatches(pattern, tag string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "~ALL" {
		return true
	}
	if pattern == "~DEFAULT_BRANCH" {
		return false
	}
	candidate := tag
	if strings.HasPrefix(pattern, "refs/tags/") {
		candidate = "refs/tags/" + tag
	}
	return githubTagGlobMatches(pattern, candidate)
}

// legacyTagProtectionProvesImmutable uses the legacy GitHub tag-protection
// endpoint as a typed policy surface, rather than treating an arbitrary
// pattern field as equivalent protection. GitHub's legacy response schema has
// no per-rule update/deletion switches: a matching record returned by this
// endpoint is the provider's explicit contract that the tag pattern is
// protected from tag mutation and deletion.
func legacyTagProtectionProvesImmutable(record githubTagProtectionRecord, tag string) bool {
	return record.Enabled && record.Pattern != "" && githubTagPatternMatches(record.Pattern, tag)
}

func githubRulesetTagPatternMatches(pattern, tag string) bool {
	return githubTagPatternMatches(pattern, tag)
}

type githubTagGlobRange struct {
	first rune
	last  rune
}

type githubTagGlobToken struct {
	kind          byte
	literal       rune
	ranges        []githubTagGlobRange
	globstar      bool
	globstarSlash bool
}

const (
	githubTagGlobLiteral byte = iota
	githubTagGlobStar
	githubTagGlobQuestion
	githubTagGlobClass
)

func readGithubTagGlobClassRune(pattern []rune, index int) (rune, int, bool) {
	if index >= len(pattern) {
		return 0, index, false
	}
	if pattern[index] == '\\' {
		return 0, index, false
	}
	return pattern[index], index + 1, true
}

// parseGithubTagGlobClass parses one positive fnmatch character class. GitHub
// protection patterns do not need complement classes here; rejecting them and
// any class that could match '/' keeps pathname matching fail-closed.
func parseGithubTagGlobClass(pattern []rune, start int) (githubTagGlobToken, int, bool) {
	if start >= len(pattern) || pattern[start] != '[' {
		return githubTagGlobToken{}, start, false
	}
	index := start + 1
	if index >= len(pattern) || pattern[index] == '^' || pattern[index] == '!' {
		return githubTagGlobToken{}, start, false
	}
	ranges := make([]githubTagGlobRange, 0, 1)
	for {
		if index >= len(pattern) {
			return githubTagGlobToken{}, start, false
		}
		if pattern[index] == ']' {
			if len(ranges) == 0 {
				return githubTagGlobToken{}, start, false
			}
			return githubTagGlobToken{kind: githubTagGlobClass, ranges: ranges}, index + 1, true
		}
		if pattern[index] == '[' {
			return githubTagGlobToken{}, start, false
		}
		first, next, ok := readGithubTagGlobClassRune(pattern, index)
		if !ok || first == '/' {
			return githubTagGlobToken{}, start, false
		}
		index = next
		// A leading hyphen is literal in a positive fnmatch class. Do not
		// parse a range when the first member is '-', so [-a] denotes '-'
		// or 'a' rather than a range beginning at '-'.
		if first != '-' && index < len(pattern) && pattern[index] == '-' && index+1 < len(pattern) && pattern[index+1] != ']' {
			last, rangeEnd, rangeOK := readGithubTagGlobClassRune(pattern, index+1)
			if !rangeOK || last == '/' || first > last || (first <= '/' && '/' <= last) {
				return githubTagGlobToken{}, start, false
			}
			ranges = append(ranges, githubTagGlobRange{first: first, last: last})
			index = rangeEnd
			continue
		}
		ranges = append(ranges, githubTagGlobRange{first: first, last: first})
	}
}

func parseGithubTagGlob(pattern string) ([]githubTagGlobToken, bool) {
	// GitHub ruleset fnmatch does not support backslash quoting. Reject it
	// before parsing classes as well, so no parser path can reinterpret it as
	// an escape and produce a false-positive protection match.
	if strings.ContainsRune(pattern, '\\') {
		return nil, false
	}
	patternRunes := []rune(pattern)
	tokens := make([]githubTagGlobToken, 0, len(patternRunes))
	for index := 0; index < len(patternRunes); {
		switch patternRunes[index] {
		case '[':
			token, next, ok := parseGithubTagGlobClass(patternRunes, index)
			if !ok {
				return nil, false
			}
			tokens = append(tokens, token)
			index = next
		case ']':
			return nil, false
		case '{', '}':
			// Brace expansion is not part of the supported GitHub pattern subset.
			return nil, false
		case '*':
			end := index
			for end < len(patternRunes) && patternRunes[end] == '*' {
				end++
			}
			isGlobstar := end-index >= 2
			tokens = append(tokens, githubTagGlobToken{
				kind:          githubTagGlobStar,
				globstar:      isGlobstar,
				globstarSlash: isGlobstar && end < len(patternRunes) && patternRunes[end] == '/',
			})
			index = end
		case '?':
			tokens = append(tokens, githubTagGlobToken{kind: githubTagGlobQuestion})
			index++
		default:
			tokens = append(tokens, githubTagGlobToken{kind: githubTagGlobLiteral, literal: patternRunes[index]})
			index++
		}
	}
	return tokens, true
}

func githubTagGlobClassMatches(ranges []githubTagGlobRange, value rune) bool {
	if value == '/' {
		return false
	}
	for _, classRange := range ranges {
		if classRange.first <= value && value <= classRange.last {
			return true
		}
	}
	return false
}

func githubTagGlobMatches(pattern, value string) bool {
	tokens, valid := parseGithubTagGlob(pattern)
	if !valid {
		return false
	}
	valueRunes := []rune(value)
	cache := make(map[[2]int]bool)
	seen := make(map[[2]int]bool)
	var match func(int, int) bool
	match = func(patternIndex, valueIndex int) bool {
		key := [2]int{patternIndex, valueIndex}
		if seen[key] {
			return cache[key]
		}
		seen[key] = true
		matched := false
		switch {
		case patternIndex == len(tokens):
			matched = valueIndex == len(valueRunes)
		case tokens[patternIndex].kind == githubTagGlobStar:
			starEnd := patternIndex + 1
			if tokens[patternIndex].globstarSlash {
				// GitHub's FNM_PATHNAME-compatible `**/` matches zero or more
				// complete directory components. The slash belongs to this
				// construct, so `release/**/*` also matches `release/v1`.
				if starEnd < len(tokens) && tokens[starEnd].kind == githubTagGlobLiteral && tokens[starEnd].literal == '/' {
					matched = match(starEnd+1, valueIndex)
					for index := valueIndex; !matched && index < len(valueRunes); index++ {
						if valueRunes[index] == '/' {
							matched = match(starEnd+1, index+1)
						}
					}
				}
			} else if tokens[patternIndex].globstar {
				matched = match(starEnd, valueIndex)
				if !matched && valueIndex < len(valueRunes) {
					matched = match(patternIndex, valueIndex+1)
				}
			} else {
				matched = match(starEnd, valueIndex)
				if !matched && valueIndex < len(valueRunes) && valueRunes[valueIndex] != '/' {
					matched = match(patternIndex, valueIndex+1)
				}
			}
		case tokens[patternIndex].kind == githubTagGlobQuestion:
			matched = valueIndex < len(valueRunes) && valueRunes[valueIndex] != '/' && match(patternIndex+1, valueIndex+1)
		case tokens[patternIndex].kind == githubTagGlobClass:
			matched = valueIndex < len(valueRunes) && githubTagGlobClassMatches(tokens[patternIndex].ranges, valueRunes[valueIndex]) && match(patternIndex+1, valueIndex+1)
		default:
			matched = valueIndex < len(valueRunes) && tokens[patternIndex].literal == valueRunes[valueIndex] && match(patternIndex+1, valueIndex+1)
		}
		cache[key] = matched
		return matched
	}
	return match(0, 0)
}

func githubTagNamePatternMatches(operator, pattern, tag string) bool {
	if !validGithubTagNamePattern(operator, pattern) {
		return false
	}
	switch operator {
	case "starts_with":
		return strings.HasPrefix(tag, pattern)
	case "ends_with":
		return strings.HasSuffix(tag, pattern)
	case "contains":
		return strings.Contains(tag, pattern)
	case "regex":
		re, _ := regexp.Compile(pattern)
		return re.MatchString(tag)
	default:
		return false
	}
}

func validGithubTagNamePattern(operator, pattern string) bool {
	if pattern == "" {
		return false
	}
	switch operator {
	case "starts_with":
	case "ends_with":
	case "contains":
	case "regex":
		re, err := regexp.Compile(pattern)
		return err == nil && re != nil
	default:
		return false
	}
	return true
}

func validGithubTagVerification(record githubTagObjectRecord) bool {
	return record.Verification.Verified && strings.EqualFold(strings.TrimSpace(record.Verification.Reason), githubVerificationReasonValid)
}

// verifyWorkflowTagSelectorUnambiguous ensures GitHub's short workflow_dispatch
// selector cannot resolve to a branch with the same label as the verified tag.
// A missing heads ref is the expected no-collision result; all other provider
// failures and malformed successful responses fail closed at the policy boundary.
func (s *GithubBuildConfigService) verifyWorkflowTagSelectorUnambiguous(ctx context.Context, config *model.GithubBuildConfig, tag string) error {
	path, err := githubRepoPath(config.Repo, "/git/ref/heads/"+url.PathEscape(tag))
	if err != nil {
		return err
	}
	resp, err := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
	if err != nil {
		var apiErr *GithubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("verify workflow tag selector collision: %w", err)
	}
	defer resp.Body.Close()

	var record githubRefRecord
	if err := decodeGithubJSON(resp, "verify workflow tag selector collision", &record); err != nil {
		return err
	}
	expectedRef := "refs/heads/" + tag
	if record.Ref != expectedRef || record.Object.Type != "commit" || !validGithubSourceSHA(record.Object.SHA) {
		return &GithubContractError{
			Operation: "verify workflow tag selector collision",
			Cause:     errors.New("provider returned an invalid branch ref response"),
		}
	}
	return &WorkflowRefApprovalError{Reason: "workflow tag selector collides with a branch of the same label"}
}

func (s *GithubBuildConfigService) verifyLegacyProtectedWorkflowTag(ctx context.Context, config *model.GithubBuildConfig, tag string) error {
	path, err := githubRepoPath(config.Repo, "/tags/protection?per_page=100&page=1")
	if err != nil {
		return err
	}
	patternCount := 0
	for page := 0; page < maxProtectedTagPages; page++ {
		resp, requestErr := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
		if requestErr != nil {
			return fmt.Errorf("verify protected workflow tag: %w", requestErr)
		}
		var records []githubTagProtectionRecord
		decodeErr := decodeGithubJSON(resp, "verify protected workflow tag", &records)
		next, hasNext, linkErr := nextGithubLink(resp.Header.Get("Link"))
		resp.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if linkErr != nil {
			return &GithubContractError{Operation: "verify protected workflow tag", Cause: linkErr}
		}
		patternCount += len(records)
		if patternCount > maxProtectedTagPatterns {
			return &GithubContractError{
				Operation: "verify protected workflow tag",
				Cause:     errors.New("provider returned too many active tag-protection patterns"),
			}
		}
		for _, record := range records {
			if legacyTagProtectionProvesImmutable(record, tag) {
				return nil
			}
		}
		if !hasNext {
			break
		}
		if page == maxProtectedTagPages-1 {
			return &GithubContractError{Operation: "verify protected workflow tag", Cause: fmt.Errorf("pagination exceeds %d pages", maxProtectedTagPages)}
		}
		path = next
	}
	if patternCount == 0 {
		return &GithubContractError{
			Operation: "verify protected workflow tag",
			Cause:     errors.New("provider returned no bounded active tag-protection patterns"),
		}
	}
	return &WorkflowRefApprovalError{Reason: "workflow tag is not covered by an active provider protection pattern"}
}

func rulesetRefNameMatches(condition *githubRulesetRefNameCondition, tag string) bool {
	if condition == nil || len(condition.Include) == 0 {
		return false
	}
	for _, pattern := range condition.Exclude {
		if githubRulesetTagPatternMatches(pattern, tag) {
			return false
		}
	}
	for _, pattern := range condition.Include {
		if githubRulesetTagPatternMatches(pattern, tag) {
			return true
		}
	}
	return false
}

type githubRulesetProtection struct {
	hasUpdateRule   bool
	hasDeletionRule bool
}

func validateGithubRulesetBypassActors(actors []githubRulesetBypassActor) error {
	for _, actor := range actors {
		switch actor.ActorType {
		case "Integration", "OrganizationAdmin", "RepositoryRole", "Team", "DeployKey", "User":
		default:
			return fmt.Errorf("unsupported ruleset bypass actor type %q", actor.ActorType)
		}
		switch actor.ActorType {
		case "Integration", "RepositoryRole", "Team", "User":
			if actor.ActorID == nil || *actor.ActorID <= 0 {
				return errors.New("ruleset bypass actor has no positive actor_id")
			}
		case "DeployKey", "OrganizationAdmin":
			if actor.ActorID != nil && *actor.ActorID <= 0 {
				return errors.New("ruleset bypass actor has an invalid actor_id")
			}
		}
		switch actor.BypassMode {
		case "always", "exempt":
		case "pull_request":
			return errors.New("pull_request bypass mode is not valid for tag rulesets")
		default:
			return fmt.Errorf("unsupported ruleset bypass mode %q", actor.BypassMode)
		}
	}
	return nil
}

func validateGithubRulesetBypassPolicy(ruleset githubRepositoryRulesetRecord) error {
	if ruleset.BypassActors == nil || ruleset.CurrentUserCanBypass == nil {
		return errors.New("ruleset bypass metadata is missing or not visible")
	}
	if err := validateGithubRulesetBypassActors(*ruleset.BypassActors); err != nil {
		return err
	}
	switch *ruleset.CurrentUserCanBypass {
	case "never":
	case "always", "pull_requests_only", "exempt":
		return &WorkflowRefApprovalError{Reason: "workflow tag matches an active repository ruleset with bypass permission"}
	default:
		return fmt.Errorf("unsupported current_user_can_bypass value %q", *ruleset.CurrentUserCanBypass)
	}
	if len(*ruleset.BypassActors) != 0 {
		return &WorkflowRefApprovalError{Reason: "workflow tag matches an active repository ruleset with bypass actors"}
	}
	return nil
}

func evaluateRulesetTagProtection(ruleset githubRepositoryRulesetRecord, tag string) (githubRulesetProtection, error) {
	if ruleset.Conditions == nil || !rulesetRefNameMatches(ruleset.Conditions.RefName, tag) {
		return githubRulesetProtection{}, nil
	}
	if err := validateGithubRulesetBypassPolicy(ruleset); err != nil {
		return githubRulesetProtection{}, err
	}
	if len(ruleset.Rules) > maxRulesetRules {
		return githubRulesetProtection{}, errors.New("provider returned too many ruleset rules")
	}
	protection := githubRulesetProtection{}
	for _, rule := range ruleset.Rules {
		switch rule.Type {
		case "tag_name_pattern":
			// Parse documented metadata, but do not use its semantics as a
			// protection selector. Ref protection comes from conditions.ref_name
			// plus update/deletion.
		case "creation", "required_linear_history", "merge_queue", "required_deployments",
			"required_signatures", "pull_request", "required_status_checks", "non_fast_forward",
			"workflows", "copilot_code_review", "code_scanning", "license_compliance_scanning",
			"file_path_restriction", "max_file_path_length", "file_extension_restriction", "max_file_size",
			"commit_author_email_pattern", "commit_message_pattern", "committer_email_pattern", "branch_name_pattern":
			// These known rules do not weaken tag immutability. Their parameters
			// are validated by the ruleset decoder but are not part of this
			// protection decision.
		case "update":
			if protection.hasUpdateRule || !validGithubUpdateRuleParameters(rule) {
				return githubRulesetProtection{}, errors.New("ruleset update rule is duplicated or malformed")
			}
			protection.hasUpdateRule = true
		case "deletion":
			if protection.hasDeletionRule || rule.ParametersPresent {
				return githubRulesetProtection{}, errors.New("ruleset deletion rule is duplicated or malformed")
			}
			protection.hasDeletionRule = true
		default:
			// The JSON decoder rejects unknown rule types. This branch is kept
			// fail-closed if the in-memory representation is constructed directly.
			return githubRulesetProtection{}, fmt.Errorf("unsupported or missing ruleset rule type %q", rule.Type)
		}
	}
	return protection, nil
}

func validGithubUpdateRuleParameters(rule githubRulesetRule) bool {
	return rule.ParametersPresent && rule.Parameters != nil && rule.UpdateAllowsFetchAndMerge != nil
}

func activeRulesetTargetsWorkflowTag(ruleset githubRepositoryRulesetRecord, tag string) bool {
	if ruleset.ID <= 0 || ruleset.Target != "tag" || ruleset.Enforcement != "active" || ruleset.Conditions == nil || !rulesetRefNameMatches(ruleset.Conditions.RefName, tag) {
		return false
	}
	return true
}

func fetchRulesetDetail(ctx context.Context, s *GithubBuildConfigService, config *model.GithubBuildConfig, summary githubRulesetSummary, tag string) (githubRepositoryRulesetRecord, error) {
	if summary.ID <= 0 {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset summary has no positive id")}
	}
	path, err := githubRepoPath(config.Repo, fmt.Sprintf("/rulesets/%d?includes_parents=true", summary.ID))
	if err != nil {
		return githubRepositoryRulesetRecord{}, err
	}
	resp, err := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
	if err != nil {
		return githubRepositoryRulesetRecord{}, fmt.Errorf("verify repository ruleset detail: %w", err)
	}
	defer resp.Body.Close()
	var rawDetail githubRepositoryRulesetDetail
	if err := decodeGithubJSON(resp, "verify repository ruleset detail", &rawDetail); err != nil {
		return githubRepositoryRulesetRecord{}, err
	}
	if rawDetail.ID == nil || *rawDetail.ID != summary.ID {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset detail id does not match summary")}
	}
	if *rawDetail.ID <= 0 || rawDetail.Name == nil || strings.TrimSpace(*rawDetail.Name) == "" || rawDetail.Source == nil || strings.TrimSpace(*rawDetail.Source) == "" || rawDetail.SourceType == nil {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset detail is missing required metadata")}
	}
	switch *rawDetail.SourceType {
	case "Repository", "Organization", "Enterprise":
	default:
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: fmt.Errorf("ruleset detail has an invalid source_type %q", *rawDetail.SourceType)}
	}
	if err := validateOptionalGithubRulesetTimestamp(rawDetail.CreatedAt, "created_at"); err != nil {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: err}
	}
	if err := validateOptionalGithubRulesetTimestamp(rawDetail.UpdatedAt, "updated_at"); err != nil {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: err}
	}
	if rawDetail.Target == nil || *rawDetail.Target != "tag" {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset detail has an invalid target")}
	}
	if rawDetail.Enforcement == nil {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset detail has an invalid enforcement")}
	}
	switch *rawDetail.Enforcement {
	case "active", "evaluate", "disabled":
	default:
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset detail has an invalid enforcement")}
	}
	detail := githubRepositoryRulesetRecord{
		ID:          *rawDetail.ID,
		Target:      *rawDetail.Target,
		Enforcement: *rawDetail.Enforcement,
		SourceType:  *rawDetail.SourceType,
		Source:      *rawDetail.Source,
	}
	if string(rawDetail.Conditions) == "null" {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset conditions must be an object")}
	}
	if len(rawDetail.Conditions) != 0 {
		var conditions githubRulesetConditions
		if err := json.Unmarshal(rawDetail.Conditions, &conditions); err != nil {
			return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: fmt.Errorf("decode conditions: %w", err)}
		}
		detail.Conditions = &conditions
	}
	if detail.Target == "tag" {
		if err := validateGithubTagRulesetConditions(detail.SourceType, detail.Conditions); err != nil {
			return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: err}
		}
	}
	if detail.Target != "tag" || detail.Enforcement != "active" {
		return detail, nil
	}
	if !activeRulesetTargetsWorkflowTag(detail, tag) {
		return detail, nil
	}
	if len(rawDetail.BypassActors) != 0 && string(rawDetail.BypassActors) != "null" {
		var actors []githubRulesetBypassActor
		if err := json.Unmarshal(rawDetail.BypassActors, &actors); err != nil {
			return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: fmt.Errorf("decode bypass_actors: %w", err)}
		}
		detail.BypassActors = &actors
	}
	if len(rawDetail.CurrentUserCanBypass) != 0 && string(rawDetail.CurrentUserCanBypass) != "null" {
		var currentUserCanBypass string
		if err := json.Unmarshal(rawDetail.CurrentUserCanBypass, &currentUserCanBypass); err != nil {
			return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: fmt.Errorf("decode current_user_can_bypass: %w", err)}
		}
		detail.CurrentUserCanBypass = &currentUserCanBypass
	}
	if len(rawDetail.Rules) == 0 {
		return detail, nil
	}
	if string(rawDetail.Rules) == "null" {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: errors.New("ruleset rules cannot be null")}
	}
	if err := json.Unmarshal(rawDetail.Rules, &detail.Rules); err != nil {
		return githubRepositoryRulesetRecord{}, &GithubContractError{Operation: "verify repository ruleset detail", Cause: fmt.Errorf("decode rules: %w", err)}
	}
	return detail, nil
}

func validateGithubTagRulesetConditions(sourceType string, conditions *githubRulesetConditions) error {
	if conditions == nil || conditions.RefName == nil {
		return errors.New("tag ruleset conditions must include ref_name")
	}
	selectorCount := 0
	if conditions.RepositoryName != nil {
		selectorCount++
	}
	if conditions.RepositoryID != nil {
		selectorCount++
	}
	if conditions.RepositoryProperty != nil {
		selectorCount++
	}
	switch sourceType {
	case "Repository":
		if selectorCount != 0 {
			return errors.New("repository tag ruleset conditions must contain only ref_name")
		}
	case "Organization", "Enterprise":
		if selectorCount != 1 {
			return errors.New("organization or enterprise tag ruleset conditions must include exactly one repository selector")
		}
	default:
		return fmt.Errorf("tag ruleset has an invalid source_type %q", sourceType)
	}
	return nil
}

func inheritedRulesetPagePath(path string) (string, error) {
	u, err := url.Parse(path)
	if err != nil || u.Path == "" {
		return "", errors.New("invalid GitHub ruleset pagination path")
	}
	query := u.Query()
	query.Set("targets", "tag")
	query.Set("includes_parents", "true")
	u.RawQuery = query.Encode()
	return u.EscapedPath() + "?" + u.RawQuery, nil
}

// verifyModernProtectedWorkflowTag inspects repository rulesets, including
// inherited parent rulesets. Missing, unsupported, forbidden, or ambiguous
// ruleset metadata is never treated as proof of protection. An explicitly empty
// bypass list is required because any bypass actor could bypass protection for
// the configured workflow selector; exact workflow path/SHA readiness remains
// enforced separately by verifyWorkflowAvailable.
func (s *GithubBuildConfigService) verifyModernProtectedWorkflowTag(ctx context.Context, config *model.GithubBuildConfig, tag string) error {
	path, err := githubRepoPath(config.Repo, "/rulesets?targets=tag&includes_parents=true&per_page=100&page=1")
	if err != nil {
		return err
	}
	rulesetCount := 0
	foundApplicableRuleset := false
	var effectiveProtection githubRulesetProtection
	var evaluationErr error
	for page := 0; page < maxProtectedTagPages; page++ {
		resp, requestErr := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
		if requestErr != nil {
			if page == 0 {
				var apiErr *GithubAPIError
				if errors.As(requestErr, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
					return &githubModernRulesetsUnsupportedError{Cause: requestErr}
				}
			}
			return fmt.Errorf("verify repository rulesets: %w", requestErr)
		}
		var summaries []githubRulesetSummary
		decodeErr := decodeGithubJSON(resp, "verify repository rulesets", &summaries)
		next, hasNext, linkErr := nextGithubLink(resp.Header.Get("Link"))
		resp.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if linkErr != nil {
			return &GithubContractError{Operation: "verify repository rulesets", Cause: linkErr}
		}
		rulesetCount += len(summaries)
		if rulesetCount > maxRulesetRecords {
			return &GithubContractError{Operation: "verify repository rulesets", Cause: errors.New("provider returned too many repository rulesets")}
		}
		for _, summary := range summaries {
			ruleset, detailErr := fetchRulesetDetail(ctx, s, config, summary, tag)
			if detailErr != nil {
				if evaluationErr == nil {
					evaluationErr = detailErr
				}
				continue
			}
			if !activeRulesetTargetsWorkflowTag(ruleset, tag) {
				continue
			}
			foundApplicableRuleset = true
			protection, err := evaluateRulesetTagProtection(ruleset, tag)
			if err != nil {
				if evaluationErr == nil {
					evaluationErr = err
				}
				continue
			}
			effectiveProtection.hasUpdateRule = effectiveProtection.hasUpdateRule || protection.hasUpdateRule
			effectiveProtection.hasDeletionRule = effectiveProtection.hasDeletionRule || protection.hasDeletionRule
		}
		if !hasNext {
			break
		}
		if page == maxProtectedTagPages-1 {
			return &GithubContractError{Operation: "verify repository rulesets", Cause: fmt.Errorf("pagination exceeds %d pages", maxProtectedTagPages)}
		}
		path, err = inheritedRulesetPagePath(next)
		if err != nil {
			return &GithubContractError{Operation: "verify repository rulesets", Cause: err}
		}
	}
	if evaluationErr != nil {
		var approvalErr *WorkflowRefApprovalError
		if errors.As(evaluationErr, &approvalErr) {
			return evaluationErr
		}
		return &GithubContractError{Operation: "verify repository ruleset detail", Cause: evaluationErr}
	}
	if foundApplicableRuleset && effectiveProtection.hasUpdateRule && effectiveProtection.hasDeletionRule {
		return nil
	}
	if rulesetCount == 0 {
		return &GithubContractError{Operation: "verify repository rulesets", Cause: errors.New("provider returned no repository rulesets")}
	}
	return &WorkflowRefApprovalError{Reason: "workflow tag is not covered by an active immutable repository ruleset without bypass actors"}
}

type protectionSurfaceState uint8

const (
	protectionSurfaceUnsupported protectionSurfaceState = iota
	protectionSurfacePositive
	protectionSurfaceNegative
	protectionSurfaceInvalid
)

type protectionSurfaceResult struct {
	state protectionSurfaceState
	err   error
}

type githubModernRulesetsUnsupportedError struct {
	Cause error
}

func (e *githubModernRulesetsUnsupportedError) Error() string {
	if e == nil || e.Cause == nil {
		return "GitHub modern rulesets surface is unsupported"
	}
	return fmt.Sprintf("GitHub modern rulesets surface is unsupported: %v", e.Cause)
}

func (e *githubModernRulesetsUnsupportedError) Unwrap() error { return e.Cause }

func classifyProtectionSurface(err error) protectionSurfaceResult {
	if err == nil {
		return protectionSurfaceResult{state: protectionSurfacePositive}
	}
	var unsupportedErr *githubModernRulesetsUnsupportedError
	if errors.As(err, &unsupportedErr) {
		return protectionSurfaceResult{state: protectionSurfaceUnsupported, err: err}
	}
	var approvalErr *WorkflowRefApprovalError
	if errors.As(err, &approvalErr) {
		return protectionSurfaceResult{state: protectionSurfaceNegative, err: err}
	}
	return protectionSurfaceResult{state: protectionSurfaceInvalid, err: err}
}

// verifyProtectedWorkflowTag treats the modern /rulesets surface as
// authoritative whenever GitHub supports it. The legacy /tags/protection
// endpoint is used only when the modern surface is not found (404). Modern
// rulesets (requested with includes_parents=true) aggregate effective update
// and deletion rules across every applicable active tag ruleset, while every
// applicable ruleset must expose an empty bypass list and current-user value
// of never. Permission/provider failures and malformed successful responses
// fail closed; no bypass actor is accepted.
func (s *GithubBuildConfigService) verifyProtectedWorkflowTag(ctx context.Context, config *model.GithubBuildConfig, tag string) error {
	modern := classifyProtectionSurface(s.verifyModernProtectedWorkflowTag(ctx, config, tag))
	if modern.state == protectionSurfacePositive {
		return nil
	}
	if modern.state != protectionSurfaceUnsupported {
		return modern.err
	}
	legacy := classifyProtectionSurface(s.verifyLegacyProtectedWorkflowTag(ctx, config, tag))
	if legacy.state == protectionSurfacePositive {
		return nil
	}
	return legacy.err
}

const maxWorkflowTagPages = 3

// resolveWorkflowTag resolves one provider-derived tag label and requires the
// GitHub ref to point at an annotated tag object whose signature is verified
// and whose nested object is exactly a commit. Lightweight tags, unverified
// tags, malformed refs, and missing provider objects all fail closed.
func (s *GithubBuildConfigService) resolveWorkflowTag(ctx context.Context, config *model.GithubBuildConfig, tag string, enforceSelectorPolicy bool) (WorkflowExecutionIdentity, error) {
	tag, err := workflowTagLabel(tag)
	if err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	if config == nil {
		return WorkflowExecutionIdentity{}, errors.New("GitHub build config is missing")
	}
	ref := "refs/tags/" + tag
	path, err := githubRepoPath(config.Repo, "/git/ref/tags/"+url.PathEscape(tag))
	if err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	resp, err := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
	if err != nil {
		return WorkflowExecutionIdentity{}, fmt.Errorf("resolve workflow tag: %w", err)
	}
	defer resp.Body.Close()
	var record githubRefRecord
	if err := decodeGithubJSON(resp, "resolve workflow tag", &record); err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	if record.Ref != ref || record.Object.Type != "tag" || !validGithubSourceSHA(record.Object.SHA) {
		return WorkflowExecutionIdentity{}, &GithubContractError{
			Operation: "resolve workflow tag",
			Cause:     errors.New("workflow tag is missing or lightweight"),
		}
	}

	tagPath, err := githubRepoPath(config.Repo, "/git/tags/"+url.PathEscape(record.Object.SHA))
	if err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	tagResp, err := s.ghReq(ctx, config, http.MethodGet, tagPath, nil, http.StatusOK)
	if err != nil {
		return WorkflowExecutionIdentity{}, fmt.Errorf("resolve workflow tag object: %w", err)
	}
	defer tagResp.Body.Close()
	var tagRecord githubTagObjectRecord
	if err := decodeGithubJSON(tagResp, "resolve workflow tag object", &tagRecord); err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	if tagRecord.SHA != record.Object.SHA || !validGithubTagVerification(tagRecord) || tagRecord.Object.Type != "commit" || !validGithubSourceSHA(tagRecord.Object.SHA) {
		return WorkflowExecutionIdentity{}, &GithubContractError{
			Operation: "resolve workflow tag object",
			Cause:     errors.New("workflow tag is not a verified annotated commit tag with an accepted verification reason"),
		}
	}
	if enforceSelectorPolicy {
		if err := s.verifyWorkflowTagSelectorUnambiguous(ctx, config, tag); err != nil {
			return WorkflowExecutionIdentity{}, err
		}
	}
	return WorkflowExecutionIdentity{Ref: ref, SHA: tagRecord.Object.SHA, VerificationReason: githubVerificationReasonValid, TrustStatus: githubTrustStatusProvider}, nil
}

// ResolveWorkflowTag resolves a provider-derived tag for the tag-policy
// boundary. Unlike normal catalog reads, it rejects a branch sharing the
// workflow_dispatch short selector.
func (s *GithubBuildConfigService) ResolveWorkflowTag(ctx context.Context, config *model.GithubBuildConfig, tag string) (WorkflowExecutionIdentity, error) {
	return s.resolveWorkflowTag(ctx, config, tag, true)
}

// resolveWorkflowExecution resolves a workflow ref for read-only catalog
// identity. Tag-policy collision checks are intentionally performed by the
// approval/build boundary instead of normal catalog reads.
func (s *GithubBuildConfigService) resolveWorkflowExecution(ctx context.Context, config *model.GithubBuildConfig) (WorkflowExecutionIdentity, error) {
	ref, err := workflowExecutionRef(config)
	if err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	if strings.HasPrefix(ref, "refs/tags/") {
		return s.resolveWorkflowTag(ctx, config, strings.TrimPrefix(ref, "refs/tags/"), false)
	}
	lookupRef := ref
	refPrefix := "heads"
	expectedRef := "refs/heads/" + ref
	if strings.HasPrefix(ref, "refs/heads/") {
		lookupRef = strings.TrimPrefix(ref, "refs/heads/")
		expectedRef = ref
	}
	path, err := githubRepoPath(config.Repo, "/git/ref/"+refPrefix+"/"+lookupRef)
	if err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	resp, err := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
	if err != nil {
		return WorkflowExecutionIdentity{}, fmt.Errorf("resolve workflow execution ref %q: %w", ref, err)
	}
	defer resp.Body.Close()
	var record githubRefRecord
	if err := decodeGithubJSON(resp, "resolve workflow execution ref", &record); err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	if record.Ref != expectedRef || !validGithubSourceSHA(record.Object.SHA) {
		return WorkflowExecutionIdentity{}, &GithubContractError{
			Operation: "resolve workflow execution ref",
			Cause:     fmt.Errorf("owned ref %q did not resolve to a valid provider object", ref),
		}
	}
	if refPrefix == "heads" {
		if record.Object.Type != "commit" {
			return WorkflowExecutionIdentity{}, &GithubContractError{
				Operation: "resolve workflow execution ref",
				Cause:     fmt.Errorf("branch %q did not resolve directly to a commit SHA", ref),
			}
		}
		return WorkflowExecutionIdentity{Ref: ref, SHA: record.Object.SHA}, nil
	}
	return WorkflowExecutionIdentity{}, &GithubContractError{Operation: "resolve workflow execution", Cause: errors.New("unsupported workflow ref type")}
}

// ListWorkflowTagOptions returns a bounded, provider-derived list of tag
// labels that are signed annotated tags and contain the current Windows
// workflow with an active workflow_dispatch trigger. It never returns refs,
// SHAs, verification payloads, or credentials to the admin client.
func (s *GithubBuildConfigService) ListWorkflowTagOptions(ctx context.Context, config *model.GithubBuildConfig) ([]WorkflowTagOption, error) {
	if config == nil {
		return nil, errors.New("GitHub build config is missing")
	}
	if err := validateGithubRepo(config.Repo); err != nil {
		return nil, err
	}
	workflow, err := WorkflowFilenameForPlatform(string(PlatformWindows))
	if err != nil {
		return nil, err
	}
	path, err := githubRepoPath(config.Repo, "/git/refs/tags?per_page=100&page=1")
	if err != nil {
		return nil, err
	}
	options := make([]WorkflowTagOption, 0)
	seen := make(map[string]struct{})
	for page := 0; page < maxWorkflowTagPages; page++ {
		resp, err := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
		if err != nil {
			return nil, err
		}
		var refs []githubRefRecord
		if err := decodeGithubJSON(resp, "list workflow tags", &refs); err != nil {
			resp.Body.Close()
			return nil, err
		}
		next, hasNext, err := nextGithubLink(resp.Header.Get("Link"))
		resp.Body.Close()
		if err != nil {
			return nil, &GithubContractError{Operation: "list workflow tags", Cause: err}
		}
		for _, ref := range refs {
			if !strings.HasPrefix(ref.Ref, "refs/tags/") || ref.Object.Type != "tag" {
				// Lightweight tags are not candidates; they are not surfaced as
				// selectable options and cannot be approved by label.
				continue
			}
			tag := strings.TrimPrefix(ref.Ref, "refs/tags/")
			if _, duplicate := seen[tag]; duplicate {
				return nil, &GithubContractError{Operation: "list workflow tags", Cause: errors.New("provider returned duplicate workflow tag")}
			}
			if protectionErr := s.verifyProtectedWorkflowTag(ctx, config, tag); protectionErr != nil {
				var approvalErr *WorkflowRefApprovalError
				if errors.As(protectionErr, &approvalErr) {
					continue
				}
				return nil, protectionErr
			}
			identity, resolveErr := s.ResolveWorkflowTag(ctx, config, tag)
			if resolveErr != nil {
				// A tag can be removed or have invalid signature metadata between
				// the list and resolve calls. Such a tag is simply not selectable;
				// provider transport/API failures remain visible and fail closed.
				var apiErr *GithubAPIError
				var transportErr *GithubTransportError
				if errors.As(resolveErr, &apiErr) || errors.As(resolveErr, &transportErr) {
					return nil, resolveErr
				}
				continue
			}
			if err := s.verifyWorkflowAvailable(ctx, config, workflow, identity.SHA); err != nil {
				var apiErr *GithubAPIError
				var transportErr *GithubTransportError
				if errors.As(err, &apiErr) || errors.As(err, &transportErr) {
					return nil, err
				}
				continue
			}
			seen[tag] = struct{}{}
			options = append(options, WorkflowTagOption{Tag: tag, Label: tag})
		}
		if !hasNext {
			break
		}
		if page == maxWorkflowTagPages-1 {
			return nil, &GithubContractError{Operation: "list workflow tags", Cause: fmt.Errorf("pagination exceeds %d pages", maxWorkflowTagPages)}
		}
		path = next
	}
	return options, nil
}

func (s *GithubBuildConfigService) validateWorkflowRefPolicy(ctx context.Context, config *model.GithubBuildConfig) (WorkflowExecutionIdentity, error) {
	ref, err := workflowExecutionRef(config)
	if err != nil {
		return WorkflowExecutionIdentity{}, &WorkflowRefApprovalError{Reason: "configured selector is invalid"}
	}
	if _, err := approvedWorkflowRef(ref); err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	if ref == defaultWorkflowExecutionRef || strings.HasPrefix(ref, "refs/heads/") {
		return WorkflowExecutionIdentity{}, &WorkflowRefApprovalError{Reason: "mutable workflow branch selectors are not approvable"}
	}
	if err := s.verifyProtectedWorkflowTag(ctx, config, strings.TrimPrefix(ref, "refs/tags/")); err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	var execution WorkflowExecutionIdentity
	if strings.HasPrefix(ref, "refs/tags/") {
		execution, err = s.ResolveWorkflowTag(ctx, config, strings.TrimPrefix(ref, "refs/tags/"))
	} else {
		execution, err = s.resolveWorkflowExecution(ctx, config)
	}
	if err != nil {
		return WorkflowExecutionIdentity{}, err
	}
	return execution, nil
}

// WorkflowFilenameForPlatform is the single application-owned workflow map.
// The configured repository supplies the executable workflow; users never
// select a filename.
func WorkflowFilenameForPlatform(platform string) (string, error) {
	switch platform {
	case string(PlatformWindows):
		return windowsWorkflowFilename, nil
	case string(PlatformLinux):
		return linuxWorkflowFilename, nil
	case string(PlatformAndroid):
		return androidWorkflowFilename, nil
	default:
		return "", fmt.Errorf("no GitHub workflow is mapped for platform %q", platform)
	}
}

// GithubProviderConfigurationError means a build cannot be submitted through
// the production provider path. It is intentionally raised before a build row
// is persisted, so the API never creates a row that can only enter a dead
// local-file queue.
type GithubProviderConfigurationError struct {
	Cause error
}

func (e *GithubProviderConfigurationError) Error() string {
	return fmt.Sprintf("GitHub build provider is not ready: %v", e.Cause)
}

func (e *GithubProviderConfigurationError) Unwrap() error { return e.Cause }

// ProductionCapabilityUnavailableError means the platform is valid in the
// typed custom-build domain but its production completion path is not yet
// validated. PR11 owns re-enabling additional platforms after evidence.
type ProductionCapabilityUnavailableError struct {
	Platform   string
	Capability string
}

func (e *ProductionCapabilityUnavailableError) Error() string {
	return fmt.Sprintf("production capability %q is unavailable for platform %q", e.Capability, e.Platform)
}

// RequireProductionBuildCapability is the single backend capability gate for
// production build execution and completion. Linux and Android remain valid
// enum values for typed settings/preset data, but cannot enter this provider
// path until PR11 validates and explicitly re-enables them.
func RequireProductionBuildCapability(platform string) error {
	if err := ValidateCustomPlatform(platform); err != nil {
		return err
	}
	if platform != string(PlatformWindows) {
		return &ProductionCapabilityUnavailableError{
			Platform:   platform,
			Capability: "production build execution and completion",
		}
	}
	return nil
}

// RequireConfiguredPublicKey enforces the runtime key-file/config contract at
// the custom-build boundary. Startup remains compatible with installations
// that intentionally have no key yet; only a build dispatch requires one.
func RequireConfiguredPublicKey() error {
	if err := global.Config.Rustdesk.RequirePublicKey(); err != nil {
		return &GithubProviderConfigurationError{Cause: err}
	}
	return nil
}

// RequireDispatchPublicKey rejects a missing typed workflow key after normal
// BuildSpec normalization. This keeps direct provider callers fail-closed even
// when they are not running through the admin DispatchTest controller.
func RequireDispatchPublicKey(params map[string]any) error {
	value, ok := params["key"].(string)
	if !ok {
		return &GithubProviderConfigurationError{Cause: &config.PublicKeyConfigurationError{Reason: "public key is missing"}}
	}
	if err := config.ValidatePublicKeyMaterial(value); err != nil {
		return &GithubProviderConfigurationError{Cause: err}
	}
	return nil
}

type githubWorkflowRecord struct {
	State string `json:"state"`
}

type githubWorkflowFileRecord struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// workflowDeclaresDispatch recognizes the GitHub Actions YAML trigger forms
// used by the owned workflows without turning the provider boundary into a
// general-purpose YAML interpreter. It accepts a block mapping, an inline
// sequence, and an inline mapping under the top-level "on" key.
func workflowDeclaresDispatch(content string) bool {
	lines := strings.Split(strings.TrimPrefix(content, "\ufeff"), "\n")
	for index, line := range lines {
		line = stripWorkflowYAMLComment(line)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || leadingWorkflowYAMLSpaces(line) != 0 {
			continue
		}
		key, value, ok := splitWorkflowYAMLKey(trimmed)
		if !ok || key != "on" {
			continue
		}
		if workflowDispatchInline(value) {
			return true
		}
		if strings.TrimSpace(value) != "" {
			continue
		}
		childIndent := -1
		for _, child := range lines[index+1:] {
			child = stripWorkflowYAMLComment(child)
			if strings.TrimSpace(child) == "" {
				continue
			}
			indent := leadingWorkflowYAMLSpaces(child)
			if indent <= 0 {
				break
			}
			if childIndent < 0 {
				childIndent = indent
			}
			if indent == childIndent {
				childKey, _, childOK := splitWorkflowYAMLKey(strings.TrimSpace(child))
				if childOK && childKey == "workflow_dispatch" {
					return true
				}
			}
		}
	}
	return false
}

func leadingWorkflowYAMLSpaces(line string) int {
	for index, char := range line {
		if char != ' ' && char != '\t' {
			return index
		}
	}
	return len(line)
}

func splitWorkflowYAMLKey(line string) (key, value string, ok bool) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", "", false
	}
	key = strings.Trim(strings.TrimSpace(line[:colon]), "\"'")
	return key, strings.TrimSpace(line[colon+1:]), key != ""
}

func workflowDispatchInline(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		value = strings.Trim(value, "[]{} ")
		for _, item := range strings.Split(value, ",") {
			item = strings.Trim(strings.TrimSpace(item), "\"'")
			itemKey, _, hasKey := splitWorkflowYAMLKey(item)
			if item == "workflow_dispatch" || (hasKey && itemKey == "workflow_dispatch") {
				return true
			}
		}
		return false
	}
	return strings.Trim(value, "\"'") == "workflow_dispatch"
}

func stripWorkflowYAMLComment(line string) string {
	var quote rune
	escaped := false
	for index, char := range line {
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 && char == '\\' && quote == '"' {
			escaped = true
			continue
		}
		if char == '\'' || char == '"' {
			if quote == 0 {
				quote = char
			} else if quote == char {
				quote = 0
			}
			continue
		}
		if char == '#' && quote == 0 && (index == 0 || line[index-1] == ' ' || line[index-1] == '\t') {
			return line[:index]
		}
	}
	return line
}

// verifyWorkflowAvailable confirms that the configured repository owns the
// fixed workflow for this build platform at the exact immutable workflow SHA.
// GitHub's repository-contents API officially supports a commit SHA in ref;
// the actions/workflows endpoint does not provide the same immutable-file
// contract. The REST error is deliberately preserved so callers can distinguish
// terminal configuration failures from retryable provider outages.
func (s *GithubBuildConfigService) verifyWorkflowAvailable(ctx context.Context, config *model.GithubBuildConfig, workflow, workflowSHA string) error {
	workflowSHA = strings.TrimSpace(workflowSHA)
	if !validGithubSourceSHA(workflowSHA) {
		return &GithubContractError{
			Operation: "workflow readiness",
			Cause:     errors.New("workflow readiness requires the resolved immutable workflow SHA"),
		}
	}
	path, err := githubRepoPath(config.Repo, "/contents/.github/workflows/"+url.PathEscape(workflow)+"?ref="+url.QueryEscape(workflowSHA))
	if err != nil {
		return err
	}
	resp, err := s.ghReq(ctx, config, http.MethodGet, path, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("workflow %q readiness request: %w", workflow, err)
	}
	defer resp.Body.Close()

	var fileRecord githubWorkflowFileRecord
	if err := decodeGithubJSON(resp, "workflow readiness file", &fileRecord); err != nil {
		return fmt.Errorf("workflow %q readiness response: %w", workflow, err)
	}
	expectedPath := ".github/workflows/" + workflow
	if fileRecord.Type != "file" || fileRecord.Path != expectedPath || !validGithubSourceSHA(fileRecord.SHA) {
		return &GithubContractError{
			Operation: "workflow readiness",
			Cause:     fmt.Errorf("workflow %q is not a file at the resolved workflow SHA", workflow),
		}
	}
	if fileRecord.Encoding != "base64" || fileRecord.Content == "" {
		return &GithubContractError{
			Operation: "workflow readiness",
			Cause:     fmt.Errorf("workflow %q contents are missing or not base64 encoded", workflow),
		}
	}
	encodedContent := strings.Join(strings.Fields(fileRecord.Content), "")
	content, err := base64.StdEncoding.DecodeString(encodedContent)
	if err != nil {
		return &GithubContractError{Operation: "workflow readiness", Cause: fmt.Errorf("workflow %q contents are invalid base64: %w", workflow, err)}
	}
	if !workflowDeclaresDispatch(string(content)) {
		return &GithubContractError{
			Operation: "workflow readiness",
			Cause:     fmt.Errorf("workflow %q does not declare a workflow_dispatch trigger", workflow),
		}
	}
	statePath, err := githubRepoPath(config.Repo, "/actions/workflows/"+workflow)
	if err != nil {
		return err
	}
	stateResp, err := s.ghReq(ctx, config, http.MethodGet, statePath, nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("workflow %q state request: %w", workflow, err)
	}
	defer stateResp.Body.Close()
	var stateRecord githubWorkflowRecord
	if err := decodeGithubJSON(stateResp, "workflow readiness state", &stateRecord); err != nil {
		return fmt.Errorf("workflow %q state response: %w", workflow, err)
	}
	if stateRecord.State != "active" {
		return &GithubContractError{
			Operation: "workflow readiness",
			Cause:     fmt.Errorf("workflow %q is not active (state=%q)", workflow, stateRecord.State),
		}
	}
	return nil
}

// GithubBuildConfigSnapshot is the validated provider configuration captured at
// the create boundary. It is intentionally an in-memory value: credentials
// are needed by the provider request but are never copied into a build row.
type GithubBuildConfigSnapshot struct {
	Repo                        string
	Token                       string
	PayloadKey                  string
	WorkflowRef                 string
	WorkflowRefApproved         bool
	WorkflowRefProviderVerified bool
	WorkflowRefApprovalSHA      string
}

// ProviderConfig materializes the snapshot for one provider request without
// exposing it to persistence or API response serialization.
func (c GithubBuildConfigSnapshot) ProviderConfig() *model.GithubBuildConfig {
	return &model.GithubBuildConfig{
		Repo:                        c.Repo,
		Token:                       c.Token,
		PayloadKey:                  c.PayloadKey,
		Branch:                      c.WorkflowRef,
		WorkflowRefApproved:         c.WorkflowRefApproved,
		WorkflowRefProviderVerified: c.WorkflowRefProviderVerified,
		WorkflowRefApprovalSHA:      c.WorkflowRefApprovalSHA,
	}
}

// PreparedGithubBuild contains the provider configuration snapshot plus
// immutable version identity selected from the configured repository. It is
// created at the API create boundary and consumed by dispatch.
type PreparedGithubBuild struct {
	Config   GithubBuildConfigSnapshot
	Identity VersionIdentity
}

// PrepareBuild validates all provider prerequisites before persistence: the
// strict repository, current credentials, fixed workflow mapping, and the
// selected version's immutable source/assets identity.
func (s *GithubBuildConfigService) PrepareBuild(ctx context.Context, platform, displayVersion string) (PreparedGithubBuild, error) {
	if err := RequireProductionBuildCapability(platform); err != nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: err}
	}
	workflow, err := WorkflowFilenameForPlatform(platform)
	if err != nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: err}
	}
	gcfg, err := s.Get()
	if err != nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: fmt.Errorf("load configuration: %w", err)}
	}
	if gcfg == nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: errors.New("configuration is missing")}
	}
	if err := validateGithubRepo(gcfg.Repo); err != nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: err}
	}
	if err := s.RequireWorkflowRefApproval(gcfg); err != nil {
		return PreparedGithubBuild{}, err
	}
	if gcfg.Token == "" {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: errors.New("PAT is not configured")}
	}
	if gcfg.PayloadKey == "" {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: errors.New("payload key is not configured")}
	}
	execution, err := s.validateWorkflowRefPolicy(ctx, gcfg)
	if err != nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: fmt.Errorf("resolve workflow execution identity: %w", err)}
	}
	if !strings.EqualFold(execution.SHA, gcfg.WorkflowRefApprovalSHA) {
		return PreparedGithubBuild{}, &WorkflowRefApprovalError{Reason: "provider tag resolved to a different commit than the approved tag"}
	}
	if err := s.verifyWorkflowAvailable(ctx, gcfg, workflow, execution.SHA); err != nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{
			Cause: fmt.Errorf("mapped workflow %q is not ready: %w", workflow, err),
		}
	}
	identity, err := s.resolveVersionWithConfig(ctx, gcfg, displayVersion, execution)
	if err != nil {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: fmt.Errorf("resolve version: %w", err)}
	}
	if identity.Repo != gcfg.Repo {
		return PreparedGithubBuild{}, &GithubProviderConfigurationError{Cause: fmt.Errorf("resolved version repository %q does not match configured repo", identity.Repo)}
	}
	return PreparedGithubBuild{
		Config: GithubBuildConfigSnapshot{
			Repo:                        gcfg.Repo,
			Token:                       gcfg.Token,
			PayloadKey:                  gcfg.PayloadKey,
			WorkflowRef:                 identity.WorkflowRef,
			WorkflowRefApproved:         gcfg.WorkflowRefApproved,
			WorkflowRefProviderVerified: gcfg.WorkflowRefProviderVerified,
			WorkflowRefApprovalSHA:      gcfg.WorkflowRefApprovalSHA,
		},
		Identity: identity,
	}, nil
}
