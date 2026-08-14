package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"rustdesk-server/api/model"
	"rustdesk-server/api/utils"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const validRustDeskPublicKey = "5Qbwsde3unUcJBtrx9ZkvUmwFNoExHzpryHuPUdqlWM="

const testRulesetMetadata = `"name":"workflow-protection","source_type":"Repository","source":"owner/repo","created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z"`

func testProtectedRulesetResponse(pattern string) string {
	pattern = strings.TrimSuffix(strings.TrimPrefix(pattern, "refs/tags/"), "*")
	return `{"id":1,` + testRulesetMetadata + `,"target":"tag","enforcement":"active","bypass_actors":[],"current_user_can_bypass":"never","conditions":{"ref_name":{"include":["refs/tags/` + pattern + `*"],"exclude":[]}},"rules":[{"type":"tag_name_pattern","parameters":{"name":"tag_name","negate":false,"operator":"starts_with","pattern":"` + pattern + `"}},{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}]}`
}

func testLegacyProtectedTagResponse(pattern string) string {
	return `[{"id":1,"created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z","enabled":true,"pattern":"` + pattern + `"}]`
}

func testPatternOnlyRulesetResponse(pattern string) string {
	pattern = strings.TrimSuffix(strings.TrimPrefix(pattern, "refs/tags/"), "*")
	return `{"id":1,` + testRulesetMetadata + `,"target":"tag","enforcement":"active","bypass_actors":[],"current_user_can_bypass":"never","conditions":{"ref_name":{"include":["refs/tags/` + pattern + `*"],"exclude":[]}},"rules":[{"type":"tag_name_pattern","parameters":{"name":"tag_name","negate":false,"operator":"starts_with","pattern":"` + pattern + `"}}]}`
}

func testObservedGithubDocsRulesetResponse(includeBypassActors bool) string {
	bypassActors := ""
	if includeBypassActors {
		bypassActors = `,"bypass_actors":[]`
	}
	return `{"id":18281681,"name":"Enterprise Tags","target":"tag","source_type":"Organization","source":"github","enforcement":"active","conditions":{"ref_name":{"exclude":[],"include":["refs/tags/enterprise-[0-9].*-freeze","refs/tags/enterprise-[0-9].[0-9].[0-9]","refs/tags/enterprise-[0-9].[0-9].[0-9][0-9]","refs/tags/enterprise-[0-9].[0-9][0-9].[0-9]","refs/tags/enterprise-[0-9].[0-9][0-9].[0-9][0-9]","refs/tags/enterprise-[0-9].*.pre[0-9]","refs/tags/enterprise-[0-9].*.pre[0-9][0-9]","refs/tags/enterprise-[0-9].*.gm[0-9]","refs/tags/enterprise-[0-9].*.gm[0-9][0-9]","refs/tags/enterprise-[0-9].*.rc[0-9]","refs/tags/enterprise-[0-9].*.rc[0-9][0-9]"]}},"rules":[{"type":"deletion"},{"type":"non_fast_forward"},{"type":"creation"},{"type":"update"}],"created_at":"2026-06-30T07:19:50.340+11:00","updated_at":"2026-06-30T07:20:37.936+11:00"` + bypassActors + `,"current_user_can_bypass":"never"}`
}

func testObservedGithubDocsRulesetSummaryResponse() string {
	return `[{"id":18281681,"target":"tag","source_type":"Organization","source":"github","enforcement":"active"}]`
}

func testRulesetResponseWithRules(id int, include string, exclude string, rules string) string {
	excludeJSON := "[]"
	if exclude != "" {
		excludeJSON = `["` + exclude + `"]`
	}
	return fmt.Sprintf(`{"id":%d,%s,"target":"tag","enforcement":"active","bypass_actors":[],"current_user_can_bypass":"never","conditions":{"ref_name":{"include":["%s"],"exclude":%s}},"rules":[%s]}`, id, testRulesetMetadata, include, excludeJSON, rules)
}

func testRulesetSummaryResponse() string {
	return "[" + testProtectedRulesetResponse("workflow-*") + "]"
}

func testRulesetSummaryResponseForID(id int) string {
	return "[" + strings.Replace(testProtectedRulesetResponse("workflow-*"), `"id":1`, fmt.Sprintf(`"id":%d`, id), 1) + "]"
}

func testRulesetResponse(req *http.Request, pattern string) *http.Response {
	if strings.Contains(req.URL.Path, "/rulesets/") {
		id := strings.TrimPrefix(req.URL.Path, "/repos/owner/repo-a/rulesets/")
		id = strings.TrimPrefix(id, "/repos/owner/repo/rulesets/")
		response := testProtectedRulesetResponse(pattern)
		if id != "" && id != req.URL.Path {
			response = strings.Replace(response, `"id":1`, `"id":`+id, 1)
		}
		return githubResponse(http.StatusOK, response, nil)
	}
	return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil)
}

func TestWorkflowFilenameForPlatformUsesFixedApplicationMapping(t *testing.T) {
	for _, test := range []struct {
		platform string
		workflow string
	}{
		{platform: string(PlatformWindows), workflow: "rustqs-windows-min-test.yml"},
		{platform: string(PlatformLinux), workflow: "rustqs-linux.yml"},
		{platform: string(PlatformAndroid), workflow: "rustqs-android.yml"},
	} {
		t.Run(test.platform, func(t *testing.T) {
			got, err := WorkflowFilenameForPlatform(test.platform)
			if err != nil || got != test.workflow {
				t.Fatalf("WorkflowFilenameForPlatform(%q) = %q, %v; want %q", test.platform, got, err, test.workflow)
			}
		})
	}
	if _, err := WorkflowFilenameForPlatform("macos"); err == nil {
		t.Fatal("unsupported platform returned a workflow")
	}
}

func TestGithubTagPatternMatchingUsesRefPathSemantics(t *testing.T) {
	for _, test := range []struct {
		pattern string
		value   string
		want    bool
	}{
		{"workflow-*", "workflow-v1", true},
		{"workflow-*", "workflow/team-v1", false},
		{"workflow/**", "workflow/team/v1", true},
		{"workflow/?", "workflow/x", true},
		{"workflow/?", "workflow/team", false},
		{"release/**/*", "release/v1", true},
		{"release/**/*", "release/foo/bar", true},
		{"release/**/*", "release", false},
		{"**/release", "release", true},
		{"**/release", "team/release", true},
		{"**/release", "team/subteam/release", true},
		{"**/release", "team/release/extra", false},
		{"release/*", "release/v1", true},
		{"release/*", "release/foo/bar", false},
		{"release/?", "release/x", true},
		{"release/?", "release/foo", false},
		{"refs/tags/release/v1", "release/v1", true},
		{"refs/tags/release/v1", "release/v2", false},
		{"workflow-[abc]", "workflow-b", true},
		{"workflow-[abc]", "workflow-d", false},
		{"workflow-v[0-9]", "workflow-v7", true},
		{"workflow-v[0-9]", "workflow-vx", false},
		{"[-a]", "-", true},
		{"[-a]", "a", true},
		{"[-a]", "0", false},
		{"workflow-[-a]", "workflow--", true},
		{"workflow-[-a]", "workflow-a", true},
		{"workflow-[-a]", "workflow-0", false},
		{"refs/tags/release/[a-z]/**", "release/a/nested/v1", true},
		{"refs/tags/release/[a-z]/**", "release/a/b/c", true},
		{"refs/tags/release/[a-z]/**", "release/ab/c", false},
		{"~ALL", "anything/nested", true},
		{"~DEFAULT_BRANCH", "workflow-v1", false},
	} {
		t.Run(test.pattern+"/"+test.value, func(t *testing.T) {
			got := githubTagPatternMatches(test.pattern, test.value)
			if got != test.want {
				t.Fatalf("githubTagPatternMatches(%q, %q) = %v, want %v", test.pattern, test.value, got, test.want)
			}
		})
	}
}

func TestRulesetRefNameMatchesGitHubIncludeExcludeGlobstarSemantics(t *testing.T) {
	condition := &githubRulesetRefNameCondition{
		Include: []string{"refs/tags/release/**/*"},
		Exclude: []string{"refs/tags/release/private/**"},
	}
	for _, test := range []struct {
		name string
		tag  string
		want bool
	}{
		{name: "single component after directory", tag: "release/v1", want: true},
		{name: "deeper nested path", tag: "release/foo/bar", want: true},
		{name: "excluded nested path", tag: "release/private/v1", want: false},
		{name: "directory itself remains outside exclusion suffix", tag: "release/private", want: true},
		{name: "outside include", tag: "hotfix/v1", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := rulesetRefNameMatches(condition, test.tag); got != test.want {
				t.Fatalf("rulesetRefNameMatches(%q) = %v, want %v", test.tag, got, test.want)
			}
		})
	}
}

func TestProviderTagPatternsPreserveWhitespace(t *testing.T) {
	for _, test := range []struct {
		name    string
		pattern string
		tag     string
		want    bool
	}{
		{name: "leading whitespace is literal", pattern: " workflow-*", tag: " workflow-v1", want: true},
		{name: "leading whitespace does not match trimmed tag", pattern: " workflow-*", tag: "workflow-v1"},
		{name: "trailing whitespace is literal", pattern: "workflow-* ", tag: "workflow-v1 ", want: true},
		{name: "trailing whitespace does not match trimmed tag", pattern: "workflow-* ", tag: "workflow-v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := githubTagPatternMatches(test.pattern, test.tag); got != test.want {
				t.Fatalf("githubTagPatternMatches(%q, %q) = %v, want %v", test.pattern, test.tag, got, test.want)
			}
		})
	}
	if githubTagNamePatternMatches("starts_with", " workflow-", "workflow-v1") {
		t.Fatal("tag_name_pattern leading whitespace was normalized")
	}
	if !githubTagNamePatternMatches("starts_with", " workflow-", " workflow-v1") {
		t.Fatal("tag_name_pattern leading whitespace was not treated literally")
	}
}

func TestGithubRulesetConditionsAcceptRepositoryScopedRefName(t *testing.T) {
	var conditions githubRulesetConditions
	if err := json.Unmarshal([]byte(`{"ref_name":{"include":["refs/tags/*"],"exclude":[]}}`), &conditions); err != nil {
		t.Fatalf("repository-scoped ref_name condition rejected: %v", err)
	}
	if err := validateGithubTagRulesetConditions(&conditions); err != nil {
		t.Fatalf("repository-scoped ref_name condition failed validation: %v", err)
	}
}

func TestGithubTagRulesetConditionsRequireRefName(t *testing.T) {
	for _, conditions := range []*githubRulesetConditions{
		nil,
		{},
		{RefName: nil},
	} {
		if err := validateGithubTagRulesetConditions(conditions); err == nil {
			t.Fatalf("validateGithubTagRulesetConditions(%#v) = nil, want missing ref_name rejection", conditions)
		}
	}
}

func TestGithubRulesetConditionsRejectUnknownFieldsAndMalformedTypes(t *testing.T) {
	for _, fixture := range []string{
		`{"unknown":{"include":["refs/tags/*"]}}`,
		`{"ref_name":{"include":["refs/tags/*"],"unknown":[]}}`,
		`{"ref_name":{"include":["refs/tags/*"],"protected":true}}`,
		`{"repository_name":{"include":["owner/*"]}}`,
		`{"repository_id":{"repository_ids":[123]}}`,
		`{"repository_property":{"include":[]}}`,
		`{"ref_name":{"include":"refs/tags/*"}}`,
		`{"ref_name":{"include":null}}`,
		`{"ref_name":null}`,
	} {
		var conditions githubRulesetConditions
		if err := json.Unmarshal([]byte(fixture), &conditions); err == nil {
			t.Fatalf("ruleset conditions %s were accepted, want fail-closed rejection", fixture)
		}
	}
}

func TestGithubTagGlobRejectsMalformedUnsupportedAndUnsafeClasses(t *testing.T) {
	for _, pattern := range []string{
		"workflow-[abc",
		"workflow-[]",
		"workflow-[z-a]",
		"workflow-[^a]",
		"workflow-[!a]",
		"workflow-[a/]",
		"workflow-[!-0]",
		"workflow-]",
		"workflow-\\",
		`workflow-\*`,
		`workflow-[a\-z]`,
	} {
		if githubTagGlobMatches(pattern, "workflow-a") {
			t.Fatalf("githubTagGlobMatches(%q) = true, want malformed/unsupported pattern rejected", pattern)
		}
	}
	for _, pattern := range []string{`workflow-\*`, `workflow-\[a\]`, `workflow-[a\-z]`} {
		if githubTagGlobMatches(pattern, "workflow-*") || githubTagGlobMatches(pattern, "workflow-[a]") {
			t.Fatalf("githubTagGlobMatches(%q) matched a backslash-quoted value", pattern)
		}
	}
}

func TestRulesetTagNamePatternOperatorsDoNotControlRefProtection(t *testing.T) {
	for _, operator := range []string{"starts_with", "ends_with", "contains", "regex"} {
		t.Run(operator, func(t *testing.T) {
			if !githubTagNamePatternMatches(operator, map[string]string{
				"starts_with": "workflow-",
				"ends_with":   "-v1",
				"contains":    "flow",
				"regex":       `^workflow-v[0-9]+$`,
			}[operator], "workflow-v1") {
				t.Fatalf("operator %q did not match documented pattern", operator)
			}
		})
	}
	if githubTagNamePatternMatches("fnmatch", "workflow-*", "workflow-v1") {
		t.Fatal("unsupported fnmatch operator was accepted")
	}
}

func TestResolveWorkflowExecutionResolvesOwnedDefaultRef(t *testing.T) {
	workflowSHA := strings.Repeat("a", 40)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/repos/owner/repo/git/ref/heads/rustqs/min-test" {
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
		return githubResponse(http.StatusOK, `{"ref":"refs/heads/rustqs/min-test","object":{"sha":"`+workflowSHA+`","type":"commit"}}`, nil), nil
	}))

	identity, err := (&GithubBuildConfigService{}).resolveWorkflowExecution(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if err != nil {
		t.Fatalf("resolveWorkflowExecution() error = %v", err)
	}
	if identity.Ref != defaultWorkflowExecutionRef || identity.SHA != workflowSHA {
		t.Fatalf("workflow execution identity = %#v, want ref %q and SHA %q", identity, defaultWorkflowExecutionRef, workflowSHA)
	}
}

func TestResolveWorkflowExecutionRejectsConfiguredSHAAsDispatchSelector(t *testing.T) {
	workflowSHA := strings.Repeat("b", 64)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("configured immutable SHA caused provider lookup: %s %s", req.Method, req.URL.Path)
		return nil, nil
	}))

	identity, err := (&GithubBuildConfigService{}).resolveWorkflowExecution(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo", Branch: workflowSHA})
	if err == nil {
		t.Fatalf("resolveWorkflowExecution() = %#v, want raw SHA selector rejection", identity)
	}
}

func TestWorkflowRefPolicyRejectsMutableDefaultBeforeProvider(t *testing.T) {
	var requests int
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		t.Fatalf("unexpected provider request for mutable default: %s %s", req.Method, req.URL.Path)
		return nil, nil
	}))
	_, err := (&GithubBuildConfigService{}).validateWorkflowRefPolicy(context.Background(), &model.GithubBuildConfig{
		Repo: "fork.example/repo", Branch: defaultWorkflowExecutionRef,
	})
	var approvalErr *WorkflowRefApprovalError
	if !errors.As(err, &approvalErr) || requests != 0 {
		t.Fatalf("validateWorkflowRefPolicy() = %T %v, requests=%d; want fail-closed mutable default", err, err, requests)
	}
}

func TestDispatchBuildRejectsLegacyDefaultEvenWhenApprovalBitsAreForged(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected provider request for forged mutable approval: %s %s", req.Method, req.URL.Path)
		return nil, nil
	}))
	config := &model.GithubBuildConfig{
		Repo:                        "owner/repo",
		Token:                       "github_pat_test",
		PayloadKey:                  "payload-key",
		Branch:                      defaultWorkflowExecutionRef,
		WorkflowRefApproved:         true,
		WorkflowRefProviderVerified: true,
		WorkflowRefApprovalSHA:      strings.Repeat("a", 40),
	}
	_, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, githubVersionIdentity(), string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey})
	var approvalErr *WorkflowRefApprovalError
	if !errors.As(err, &approvalErr) {
		t.Fatalf("DispatchBuild() error = %T %v, want mutable default rejection", err, err)
	}
}

func TestResolveWorkflowExecutionRequiresVerifiedAnnotatedTag(t *testing.T) {
	const selector = "refs/tags/workflow-v1"
	tagObjectSHA := strings.Repeat("b", 40)
	commitSHA := strings.Repeat("c", 40)
	for _, test := range []struct {
		name      string
		refType   string
		verified  bool
		objectSHA string
		wantError bool
	}{
		{name: "verified annotated", refType: "tag", verified: true, objectSHA: commitSHA},
		{name: "lightweight", refType: "commit", verified: true, objectSHA: commitSHA, wantError: true},
		{name: "unverified annotated", refType: "tag", verified: false, objectSHA: commitSHA, wantError: true},
		{name: "provider tag moved to a new valid commit", refType: "tag", verified: true, objectSHA: strings.Repeat("d", 40)},
	} {
		t.Run(test.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/git/ref/heads/workflow-v1") {
					t.Fatal("normal catalog workflow resolution queried the short-selector branch ref")
				}
				if strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
					return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+tagObjectSHA+`","type":"`+test.refType+`"}}`, nil), nil
				}
				if strings.HasSuffix(req.URL.Path, "/git/tags/"+tagObjectSHA) {
					return githubResponse(http.StatusOK, `{"sha":"`+tagObjectSHA+`","object":{"sha":"`+test.objectSHA+`","type":"commit"},"verification":{"verified":`+fmt.Sprintf("%t", test.verified)+`,"reason":"valid"}}`, nil), nil
				}
				return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
			}))
			identity, err := (&GithubBuildConfigService{}).resolveWorkflowExecution(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo", Branch: selector})
			if test.wantError {
				if err == nil {
					t.Fatalf("resolveWorkflowExecution() = %#v, nil; want tag rejection", identity)
				}
				return
			}
			wantSHA := commitSHA
			if test.objectSHA != commitSHA {
				wantSHA = test.objectSHA
			}
			if err != nil || identity.SHA != wantSHA {
				t.Fatalf("resolveWorkflowExecution() = %#v, %v; want verified commit %q", identity, err, wantSHA)
			}
			if identity.VerificationReason != githubVerificationReasonValid || identity.TrustStatus != githubTrustStatusProvider {
				t.Fatalf("verification trust metadata = %#v, want provider-reported valid status", identity)
			}
		})
	}
}

func TestVerifyProtectedWorkflowTagRequiresMatchingProviderProtection(t *testing.T) {
	for _, test := range []struct {
		name         string
		legacyStatus int
		legacyBody   string
		modernBody   string
		wantError    bool
		wantAPIErr   bool
		wantPolicy   bool
	}{
		{name: "both positive", legacyStatus: http.StatusOK, legacyBody: testLegacyProtectedTagResponse("workflow-*"), modernBody: testProtectedRulesetResponse("workflow-*")},
		{name: "modern-only fallback", legacyStatus: http.StatusNotFound, legacyBody: `{"message":"unsupported"}`, modernBody: testProtectedRulesetResponse("workflow-*")},
		{name: "legacy mismatch modern positive", legacyStatus: http.StatusOK, legacyBody: testLegacyProtectedTagResponse("release-*"), modernBody: testProtectedRulesetResponse("workflow-*")},
		{name: "legacy malformed modern positive", legacyStatus: http.StatusOK, legacyBody: `{`, modernBody: testProtectedRulesetResponse("workflow-*")},
		{name: "legacy positive modern mismatch", legacyStatus: http.StatusOK, legacyBody: testLegacyProtectedTagResponse("workflow-*"), modernBody: testProtectedRulesetResponse("release-*"), wantError: true, wantPolicy: true},
		{name: "legacy positive modern non-immutable", legacyStatus: http.StatusOK, legacyBody: testLegacyProtectedTagResponse("workflow-*"), modernBody: testPatternOnlyRulesetResponse("workflow-*"), wantError: true, wantPolicy: true},
		{name: "initial modern 404 plus legacy positive", legacyStatus: http.StatusOK, legacyBody: testLegacyProtectedTagResponse("workflow-*"), modernBody: `{"message":"unsupported"}`},
		{name: "legacy permission modern fallback", legacyStatus: http.StatusForbidden, legacyBody: `{"message":"permission denied"}`, modernBody: testProtectedRulesetResponse("workflow-*")},
		{name: "both unsupported", legacyStatus: http.StatusNotFound, legacyBody: `{"message":"unsupported"}`, modernBody: `{"message":"unsupported"}`, wantError: true, wantAPIErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/repos/owner/repo/rulesets/1" {
					return githubResponse(http.StatusOK, test.modernBody, nil), nil
				}
				if req.URL.Path == "/repos/owner/repo/rulesets" {
					status := http.StatusOK
					if strings.HasPrefix(test.modernBody, `{"message"`) {
						status = http.StatusNotFound
					}
					body := testRulesetSummaryResponse()
					if status != http.StatusOK {
						body = test.modernBody
					}
					return githubResponse(status, body, nil), nil
				}
				if req.URL.Path != "/repos/owner/repo/tags/protection" {
					t.Fatalf("unexpected protected-tag request: %s %s", req.Method, req.URL.Path)
				}
				return githubResponse(test.legacyStatus, test.legacyBody, nil), nil
			}))
			err := (&GithubBuildConfigService{}).verifyProtectedWorkflowTag(
				context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1",
			)
			if (err != nil) != test.wantError {
				t.Fatalf("verifyProtectedWorkflowTag() error = %v, wantError=%v", err, test.wantError)
			}
			if test.wantAPIErr {
				var apiErr *GithubAPIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want GithubAPIError", err, err)
				}
			}
			if test.wantPolicy {
				var policyErr *WorkflowRefApprovalError
				if !errors.As(err, &policyErr) {
					t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want policy rejection", err, err)
				}
			}
		})
	}
}

func TestVerifyModernProtectedWorkflowTagAcceptsObservedInheritedOrganizationRuleset(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/rulesets":
			return githubResponse(http.StatusOK, testObservedGithubDocsRulesetSummaryResponse(), nil), nil
		case "/repos/owner/repo/rulesets/18281681":
			return githubResponse(http.StatusOK, testObservedGithubDocsRulesetResponse(true), nil), nil
		default:
			t.Fatalf("unexpected observed inherited-ruleset request: %s %s", req.Method, req.URL.RequestURI())
			return nil, nil
		}
	}))
	if err := (&GithubBuildConfigService{}).verifyModernProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "enterprise-1.2.3"); err != nil {
		t.Fatalf("verifyModernProtectedWorkflowTag() error = %v, want observed inherited organization protection", err)
	}
}

func TestVerifyModernProtectedWorkflowTagRejectsObservedOmittedBypassActors(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/rulesets":
			return githubResponse(http.StatusOK, testObservedGithubDocsRulesetSummaryResponse(), nil), nil
		case "/repos/owner/repo/rulesets/18281681":
			return githubResponse(http.StatusOK, testObservedGithubDocsRulesetResponse(false), nil), nil
		default:
			t.Fatalf("unexpected omitted-bypass ruleset request: %s %s", req.Method, req.URL.RequestURI())
			return nil, nil
		}
	}))
	err := (&GithubBuildConfigService{}).verifyModernProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "enterprise-1.2.3")
	var contractErr *GithubContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("verifyModernProtectedWorkflowTag() error = %T %v, want omitted bypass metadata to fail closed", err, err)
	}
}

func TestVerifyProtectedWorkflowTagDoesNotFallbackAfterModernSurfaceIsSupported(t *testing.T) {
	tests := []struct {
		name          string
		modernPageTwo bool
		detailStatus  int
		detailBody    string
		wantAPI       bool
	}{
		{
			name:         "detail 404",
			detailStatus: http.StatusNotFound,
			detailBody:   `{"message":"detail disappeared"}`,
		},
		{
			name:         "detail 403",
			detailStatus: http.StatusForbidden,
			detailBody:   `{"message":"permission denied"}`,
		},
		{
			name:         "detail 500",
			detailStatus: http.StatusInternalServerError,
			detailBody:   `{"message":"server failed"}`,
		},
		{
			name:       "detail malformed",
			detailBody: `{`,
		},
		{
			name:          "later list page 404",
			modernPageTwo: true,
			detailBody:    testProtectedRulesetResponse("workflow-*"),
			wantAPI:       true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/repos/owner/repo/tags/protection":
					return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
				case "/repos/owner/repo/rulesets":
					if test.modernPageTwo {
						if req.URL.Query().Get("page") == "2" {
							return githubResponse(http.StatusNotFound, test.detailBody, nil), nil
						}
						return githubResponse(http.StatusOK, testRulesetSummaryResponse(), http.Header{"Link": []string{`<https://api.github.com/repos/owner/repo/rulesets?per_page=100&page=2>; rel="next"`}}), nil
					}
					return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil), nil
				case "/repos/owner/repo/rulesets/1":
					if test.detailStatus != 0 {
						return githubResponse(test.detailStatus, test.detailBody, nil), nil
					}
					return githubResponse(http.StatusOK, test.detailBody, nil), nil
				default:
					t.Fatalf("unexpected protected-tag request: %s %s", req.Method, req.URL.RequestURI())
					return nil, nil
				}
			}))
			err := (&GithubBuildConfigService{}).verifyProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1")
			if err == nil {
				t.Fatal("verifyProtectedWorkflowTag() returned nil after modern failure despite legacy positive")
			}
			if test.wantAPI {
				var apiErr *GithubAPIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want later-page GithubAPIError", err, err)
				}
				return
			}
			var contractErr *GithubContractError
			if !errors.As(err, &contractErr) {
				t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want detail contract error", err, err)
			}
		})
	}
}

func TestClassifyProtectionSurfaceRequiresInitialModernUnsupportedState(t *testing.T) {
	initial404 := &githubModernRulesetsUnsupportedError{Cause: &GithubAPIError{StatusCode: http.StatusNotFound}}
	if result := classifyProtectionSurface(initial404); result.state != protectionSurfaceUnsupported {
		t.Fatalf("classifyProtectionSurface(initial 404) = %v, want unsupported", result.state)
	}
	for _, err := range []error{
		&GithubAPIError{StatusCode: http.StatusNotFound},
		fmt.Errorf("detail: %w", &GithubAPIError{StatusCode: http.StatusNotFound}),
		&GithubContractError{Operation: "detail", Cause: &GithubAPIError{StatusCode: http.StatusNotFound}},
	} {
		if result := classifyProtectionSurface(err); result.state == protectionSurfaceUnsupported {
			t.Fatalf("classifyProtectionSurface(%T) = unsupported, want invalid", err)
		}
	}
}

func TestVerifyProtectedWorkflowTagRejectsLegacyPositiveWhenModernRulesetAllowsBypass(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/tags/protection":
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		case "/repos/owner/repo/rulesets":
			return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil), nil
		case "/repos/owner/repo/rulesets/1":
			return githubResponse(http.StatusOK, strings.Replace(
				testProtectedRulesetResponse("workflow-*"),
				`"bypass_actors":[]`,
				`"bypass_actors":[{"actor_id":1,"actor_type":"RepositoryRole","bypass_mode":"always"}]`,
				1,
			), nil), nil
		default:
			t.Fatalf("unexpected protected-tag request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	}))

	err := (&GithubBuildConfigService{}).verifyProtectedWorkflowTag(
		context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1",
	)
	var policyErr *WorkflowRefApprovalError
	if !errors.As(err, &policyErr) {
		t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want modern bypass policy rejection", err, err)
	}
}

func TestVerifyProtectedWorkflowTagRequiresModernActiveRuleset(t *testing.T) {
	for _, test := range []struct {
		name         string
		ruleset      string
		status       int
		wantAPI      bool
		wantPolicy   bool
		wantContract bool
	}{
		{name: "protected", ruleset: testProtectedRulesetResponse("workflow-*")},
		{name: "mismatch", ruleset: testProtectedRulesetResponse("release-*"), wantPolicy: true},
		{name: "tag metadata does not select protection", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"operator":"starts_with","pattern":"workflow-"`, `"operator":"ends_with","pattern":"not-the-ref"`, 1)},
		{name: "tag metadata is optional", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `{"type":"tag_name_pattern","parameters":{"name":"tag_name","negate":false,"operator":"starts_with","pattern":"workflow-"}},`, "", 1)},
		{name: "pattern only", ruleset: testPatternOnlyRulesetResponse("workflow-*"), wantPolicy: true},
		{name: "missing update rule", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},`, "", 1), wantPolicy: true},
		{name: "missing deletion rule", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `,{"type":"deletion"}`, "", 1), wantPolicy: true},
		{name: "unsupported rule", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `{"type":"deletion"}`, `{"type":"required_signatures"}`, 1), wantPolicy: true},
		{name: "unknown rule is rejected", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `,{"type":"deletion"}]`, `,{"type":"deletion"},{"type":"unknown_rule"}]`, 1), wantContract: true},
		{name: "update allows fetch and merge", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"update_allows_fetch_and_merge":false`, `"update_allows_fetch_and_merge":true`, 1)},
		{name: "missing update parameter", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}}`, `{"type":"update","parameters":{}}`, 1), wantContract: true},
		{name: "unknown update parameter", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}}`, `{"type":"update","parameters":{"operator":"fnmatch"}}`, 1), wantContract: true},
		{name: "inactive", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"enforcement":"active"`, `"enforcement":"disabled"`, 1), wantPolicy: true},
		{name: "bypass actor", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"bypass_actors":[]`, `"bypass_actors":[{"actor_id":1,"actor_type":"RepositoryRole","bypass_mode":"always"}]`, 1), wantPolicy: true},
		{name: "missing bypass declaration", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"bypass_actors":[],`, "", 1), wantContract: true},
		{name: "current user can always bypass", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"current_user_can_bypass":"never"`, `"current_user_can_bypass":"always"`, 1), wantPolicy: true},
		{name: "current user can bypass pull requests", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"current_user_can_bypass":"never"`, `"current_user_can_bypass":"pull_requests_only"`, 1), wantPolicy: true},
		{name: "current user is exempt", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"current_user_can_bypass":"never"`, `"current_user_can_bypass":"exempt"`, 1), wantPolicy: true},
		{name: "missing current-user bypass declaration", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"current_user_can_bypass":"never",`, "", 1), wantContract: true},
		{name: "malformed bypass actor", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"bypass_actors":[]`, `"bypass_actors":[{"actor_id":0,"actor_type":"User","bypass_mode":"always"}]`, 1), wantContract: true},
		{name: "pull-request bypass mode is not valid for tags", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"bypass_actors":[]`, `"bypass_actors":[{"actor_id":1,"actor_type":"User","bypass_mode":"pull_request"}]`, 1), wantContract: true},
		{name: "missing ruleset target", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"target":"tag"`, `"target":""`, 1), wantContract: true},
		{name: "invalid ruleset enforcement", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"enforcement":"active"`, `"enforcement":"unknown"`, 1), wantContract: true},
		{name: "missing ruleset name", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"name":"workflow-protection",`, "", 1), wantContract: true},
		{name: "missing ruleset source", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"source":"owner/repo",`, "", 1), wantContract: true},
		{name: "missing ruleset source type", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"source_type":"Repository",`, "", 1), wantContract: true},
		{name: "invalid ruleset source type", ruleset: strings.Replace(testProtectedRulesetResponse("workflow-*"), `"source_type":"Repository"`, `"source_type":"Unknown"`, 1), wantContract: true},
		{name: "optional ruleset timestamps", ruleset: strings.Replace(strings.Replace(testProtectedRulesetResponse("workflow-*"), `"created_at":"2026-08-01T00:00:00Z",`, "", 1), `,"updated_at":"2026-08-01T00:00:00Z"`, "", 1)},
		{name: "malformed response", ruleset: `{`, wantContract: true},
		{name: "unsupported", status: http.StatusNotFound, wantPolicy: true},
		{name: "permission ambiguity", status: http.StatusForbidden, wantAPI: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/repos/owner/repo/tags/protection":
					return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("release-*"), nil), nil
				case "/repos/owner/repo/rulesets/1":
					if test.status != 0 {
						return githubResponse(test.status, `{"message":"rulesets unavailable"}`, nil), nil
					}
					return githubResponse(http.StatusOK, test.ruleset, nil), nil
				case "/repos/owner/repo/rulesets":
					if test.status != 0 {
						return githubResponse(test.status, `{"message":"rulesets unavailable"}`, nil), nil
					}
					return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil), nil
				default:
					t.Fatalf("unexpected ruleset policy request: %s %s", req.Method, req.URL.Path)
					return nil, nil
				}
			}))
			err := (&GithubBuildConfigService{}).verifyProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1")
			if test.wantContract {
				var contractErr *GithubContractError
				if !errors.As(err, &contractErr) {
					t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want malformed-response contract error", err, err)
				}
				return
			}
			if test.wantAPI {
				var apiErr *GithubAPIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want provider API error", err, err)
				}
				return
			}
			if test.wantPolicy {
				var policyErr *WorkflowRefApprovalError
				if !errors.As(err, &policyErr) {
					t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want policy rejection", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("verifyProtectedWorkflowTag() error = %v, want protected tag", err)
			}
		})
	}
}

func TestVerifyProtectedWorkflowTagFindsLegacyPatternBeyondFirstPage(t *testing.T) {
	var legacyPages int
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/tags/protection":
			legacyPages++
			if req.URL.Query().Get("page") == "2" {
				return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
			}
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("release-*"), http.Header{"Link": []string{`<https://api.github.com/repos/owner/repo/tags/protection?per_page=100&page=2>; rel="next"`}}), nil
		case "/repos/owner/repo/rulesets":
			return githubResponse(http.StatusNotFound, `{"message":"unsupported"}`, nil), nil
		case "/repos/owner/repo/rulesets/1":
			return githubResponse(http.StatusOK, testProtectedRulesetResponse("workflow-*"), nil), nil
		default:
			t.Fatalf("unexpected paginated protection request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	}))
	if err := (&GithubBuildConfigService{}).verifyProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1"); err != nil {
		t.Fatalf("verifyProtectedWorkflowTag() error = %v, want page-two protection", err)
	}
	if legacyPages != 2 {
		t.Fatalf("legacy protection pages = %d, want 2", legacyPages)
	}
}

func TestVerifyLegacyProtectedWorkflowTagRejectsUnsupportedPolicyFields(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/tags/protection":
			return githubResponse(http.StatusOK, strings.Replace(testLegacyProtectedTagResponse("workflow-*"), `"pattern":"workflow-*"`, `"pattern":"workflow-*","allow_update":true`, 1), nil), nil
		case "/repos/owner/repo/rulesets":
			return githubResponse(http.StatusNotFound, `{"message":"unsupported"}`, nil), nil
		default:
			t.Fatalf("unexpected legacy policy request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	}))
	err := (&GithubBuildConfigService{}).verifyProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1")
	var contractErr *GithubContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("verifyProtectedWorkflowTag() error = %T %v, want unsupported legacy policy contract error", err, err)
	}
}

func TestLegacyTagProtectionDecodesDocumentedListResponseShape(t *testing.T) {
	var records []githubTagProtectionRecord
	if err := json.Unmarshal([]byte(testLegacyProtectedTagResponse("workflow-*")), &records); err != nil {
		t.Fatalf("documented legacy tag protection response rejected: %v", err)
	}
	if len(records) != 1 || !legacyTagProtectionProvesImmutable(records[0], "workflow-v1") {
		t.Fatalf("legacy tag protection records = %#v, want enabled matching record", records)
	}

	var disabled []githubTagProtectionRecord
	if err := json.Unmarshal([]byte(strings.Replace(testLegacyProtectedTagResponse("workflow-*"), `"enabled":true`, `"enabled":false`, 1)), &disabled); err != nil {
		t.Fatalf("documented disabled legacy tag protection response rejected: %v", err)
	}
	if legacyTagProtectionProvesImmutable(disabled[0], "workflow-v1") {
		t.Fatal("disabled legacy tag protection record proved immutability")
	}
	var patternOnly []githubTagProtectionRecord
	if err := json.Unmarshal([]byte(`[{"pattern":"workflow-*"}]`), &patternOnly); err != nil {
		t.Fatalf("legacy response with only required pattern rejected: %v", err)
	}
	if legacyTagProtectionProvesImmutable(patternOnly[0], "workflow-v1") {
		t.Fatal("legacy response without enabled=true proved immutability")
	}
	var malformed []githubTagProtectionRecord
	if err := json.Unmarshal([]byte(strings.Replace(testLegacyProtectedTagResponse("workflow-*"), `"pattern":"workflow-*"`, `"pattern":"workflow-*","unexpected":true`, 1)), &malformed); err == nil {
		t.Fatal("legacy tag protection response with unknown field was accepted")
	}
}

func TestVerifyModernProtectedWorkflowTagFindsRulesetBeyondFirstPage(t *testing.T) {
	var rulesetPages int
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/rulesets":
			rulesetPages++
			if req.URL.Query().Get("targets") != "tag" || req.URL.Query().Get("includes_parents") != "true" {
				t.Fatalf("ruleset page %q omitted targets=tag or includes_parents=true", req.URL.RawQuery)
			}
			if req.URL.Query().Get("page") == "2" {
				return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil), nil
			}
			return githubResponse(http.StatusOK, testRulesetSummaryResponseForID(2), http.Header{"Link": []string{`<https://api.github.com/repos/owner/repo/rulesets?per_page=100&page=2>; rel="next"`}}), nil
		case "/repos/owner/repo/rulesets/1":
			return githubResponse(http.StatusOK, testProtectedRulesetResponse("workflow-*"), nil), nil
		case "/repos/owner/repo/rulesets/2":
			return githubResponse(http.StatusOK, strings.Replace(testProtectedRulesetResponse("release-*"), `"id":1`, `"id":2`, 1), nil), nil
		default:
			t.Fatalf("unexpected ruleset pagination request: %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	}))
	if err := (&GithubBuildConfigService{}).verifyModernProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1"); err != nil {
		t.Fatalf("verifyModernProtectedWorkflowTag() error = %v, want page-two ruleset protection", err)
	}
	if rulesetPages != 2 {
		t.Fatalf("ruleset pages = %d, want 2", rulesetPages)
	}
}

func TestVerifyModernProtectedWorkflowTagIncludesInheritedParentRulesets(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/repos/owner/repo/rulesets/9" {
			return githubResponse(http.StatusOK, strings.Replace(testProtectedRulesetResponse("workflow-*"), `"id":1`, `"id":9`, 1), nil), nil
		}
		if req.URL.Path != "/repos/owner/repo/rulesets" {
			t.Fatalf("unexpected inherited-ruleset request: %s %s", req.Method, req.URL.RequestURI())
		}
		if req.URL.Query().Get("targets") != "tag" || req.URL.Query().Get("includes_parents") != "true" || req.URL.Query().Get("per_page") != "100" || req.URL.Query().Get("page") != "1" {
			t.Fatalf("ruleset query = %q, want bounded inherited-ruleset query", req.URL.RawQuery)
		}
		return githubResponse(http.StatusOK, `[{"id":9}]`, nil), nil
	}))
	if err := (&GithubBuildConfigService{}).verifyModernProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1"); err != nil {
		t.Fatalf("verifyModernProtectedWorkflowTag() error = %v, want inherited parent policy to count", err)
	}
}

func TestVerifyModernProtectedWorkflowTagAggregatesApplicableRulesets(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		details []struct {
			id   int
			body string
		}
		wantPolicy bool
	}{
		{
			name: "split update and deletion rules pass",
			details: []struct {
				id   int
				body string
			}{
				{1, testRulesetResponseWithRules(1, "refs/tags/workflow-*", "", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}}`)},
				{2, testRulesetResponseWithRules(2, "refs/tags/workflow-*", "", `{"type":"deletion"}`)},
			},
		},
		{
			name: "stronger known rule does not weaken complete protection",
			details: []struct {
				id   int
				body string
			}{
				{1, testRulesetResponseWithRules(1, "refs/tags/workflow-*", "", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"},{"type":"required_signatures"}`)},
			},
		},
		{
			name: "matching weak-only set fails closed",
			details: []struct {
				id   int
				body string
			}{
				{1, testRulesetResponseWithRules(1, "refs/tags/workflow-*", "", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}}`)},
			},
			wantPolicy: true,
		},
		{
			name: "protected tag ignores non-matching weak ruleset",
			details: []struct {
				id   int
				body string
			}{
				{1, testRulesetResponseWithRules(1, "refs/tags/workflow-*", "", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}`)},
				{2, testRulesetResponseWithRules(2, "refs/tags/release-*", "", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}}`)},
			},
		},
		{
			name: "protected tag ignores inactive weak ruleset",
			details: []struct {
				id   int
				body string
			}{
				{1, testRulesetResponseWithRules(1, "refs/tags/workflow-*", "", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}`)},
				{2, strings.Replace(testRulesetResponseWithRules(2, "refs/tags/workflow-*", "", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}}`), `"enforcement":"active"`, `"enforcement":"evaluate"`, 1)},
			},
		},
		{
			name: "excluded tag is not protected",
			tag:  "workflow-v1",
			details: []struct {
				id   int
				body string
			}{
				{1, testRulesetResponseWithRules(1, "refs/tags/workflow-*", "refs/tags/workflow-v1", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}`)},
			},
			wantPolicy: true,
		},
		{
			name: "non-excluded tag remains protected",
			tag:  "workflow-v2",
			details: []struct {
				id   int
				body string
			}{
				{1, testRulesetResponseWithRules(1, "refs/tags/workflow-*", "refs/tags/workflow-v1", `{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}`)},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tag := test.tag
			if tag == "" {
				tag = "workflow-v1"
			}
			summary := "["
			for index, detail := range test.details {
				if index > 0 {
					summary += ","
				}
				summary += strings.TrimSuffix(strings.TrimPrefix(testRulesetSummaryResponseForID(detail.id), "["), "]")
			}
			summary += "]"
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/repos/owner/repo/rulesets" {
					return githubResponse(http.StatusOK, summary, nil), nil
				}
				for _, detail := range test.details {
					if req.URL.Path == fmt.Sprintf("/repos/owner/repo/rulesets/%d", detail.id) {
						return githubResponse(http.StatusOK, detail.body, nil), nil
					}
				}
				t.Fatalf("unexpected ruleset request: %s %s", req.Method, req.URL.RequestURI())
				return nil, nil
			}))

			err := (&GithubBuildConfigService{}).verifyModernProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, tag)
			if test.wantPolicy {
				var policyErr *WorkflowRefApprovalError
				if !errors.As(err, &policyErr) {
					t.Fatalf("verifyModernProtectedWorkflowTag() error = %T %v, want policy rejection", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("verifyModernProtectedWorkflowTag() error = %v, want protected tag", err)
			}
		})
	}
}

func TestGithubRulesetParserAcceptsDocumentedKnownRuleParameters(t *testing.T) {
	fixture := `{"id":1,` + testRulesetMetadata + `,"target":"tag","enforcement":"active","bypass_actors":[],"current_user_can_bypass":"never","conditions":{"ref_name":{"include":["refs/tags/workflow-*"]}},"rules":[{"type":"workflows","parameters":{"do_not_enforce_on_create":true,"workflows":[{"path":".github/workflows/build.yml","repository_id":123,"ref":"refs/heads/main","sha":"` + strings.Repeat("a", 40) + `"}]}},{"type":"pull_request","parameters":{"dismiss_stale_reviews_on_push":true,"require_code_owner_review":true,"required_approving_review_count":1,"required_review_thread_resolution":true,"require_last_push_approval":true,"allowed_merge_methods":["merge","squash"],"dismissal_restriction":{"enabled":true,"allowed_actors":[{"id":123,"type":"User"}]},"required_reviewers":[{"file_patterns":["src/**"],"minimum_approvals":1,"reviewer":{"id":123,"type":"Team"}}]}},{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}]}`
	var ruleset githubRepositoryRulesetRecord
	if err := json.Unmarshal([]byte(fixture), &ruleset); err != nil {
		t.Fatalf("documented ruleset fixture rejected: %v", err)
	}
	protection, err := evaluateRulesetTagProtection(ruleset, "workflow-0")
	if err != nil || !protection.hasUpdateRule || !protection.hasDeletionRule {
		t.Fatalf("documented known rule parameters prevented valid tag ruleset protection: protection=%#v err=%v", protection, err)
	}
}

func TestGithubRulesetParserRejectsMalformedKnownRuleParameters(t *testing.T) {
	for _, parameters := range []string{`null`, `[]`, `{`} {
		fixture := `{"type":"workflows","parameters":` + parameters + `}`
		var rule githubRulesetRule
		if err := json.Unmarshal([]byte(fixture), &rule); err == nil {
			t.Fatalf("ruleset parameters %s were accepted, want fail-closed rejection", parameters)
		}
	}
	for _, ruleType := range []string{"required_deployments", "pull_request", "required_status_checks", "workflows", "merge_queue", "copilot_code_review", "code_scanning", "file_path_restriction", "max_file_path_length", "file_extension_restriction", "max_file_size"} {
		fixture := `{"type":"` + ruleType + `","parameters":{}}`
		var rule githubRulesetRule
		if err := json.Unmarshal([]byte(fixture), &rule); err == nil {
			t.Fatalf("known neutral rule %q with empty parameters was accepted", ruleType)
		}
	}
	for _, ruleType := range []string{"creation", "deletion", "required_linear_history", "required_signatures", "non_fast_forward", "license_compliance_scanning"} {
		fixture := `{"type":"` + ruleType + `","parameters":{}}`
		var rule githubRulesetRule
		if err := json.Unmarshal([]byte(fixture), &rule); err == nil {
			t.Fatalf("no-parameter rule %q with parameters was accepted", ruleType)
		}
	}
	var unknown githubRulesetRule
	if err := json.Unmarshal([]byte(`{"type":"unknown_rule","parameters":{}}`), &unknown); err == nil {
		t.Fatal("unknown ruleset type was accepted")
	}
}

func TestGithubRulesetUpdateValidationIsTargetAware(t *testing.T) {
	for _, test := range []struct {
		name       string
		target     string
		parameters string
		wantDecode bool
		wantValid  bool
	}{
		{name: "tag without parameters", target: "tag", wantDecode: true, wantValid: true},
		{name: "tag with valid parameters", target: "tag", parameters: `{"update_allows_fetch_and_merge":false}`, wantDecode: true, wantValid: true},
		{name: "branch without parameters", target: "branch", wantDecode: true},
		{name: "branch with valid parameters", target: "branch", parameters: `{"update_allows_fetch_and_merge":true}`, wantDecode: true, wantValid: true},
		{name: "tag null parameters", target: "tag", parameters: `null`},
		{name: "tag empty parameters", target: "tag", parameters: `{}`},
		{name: "tag array parameters", target: "tag", parameters: `[]`},
		{name: "tag wrong boolean type", target: "tag", parameters: `{"update_allows_fetch_and_merge":"false"}`},
		{name: "tag unknown parameter", target: "tag", parameters: `{"update_allows_fetch_and_merge":false,"unknown":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := `{"type":"update"`
			if test.parameters != "" {
				fixture += `,"parameters":` + test.parameters
			}
			fixture += `}`
			var rule githubRulesetRule
			err := json.Unmarshal([]byte(fixture), &rule)
			if test.wantDecode {
				if err != nil {
					t.Fatalf("decode update rule = %v, want decode success", err)
				}
				if err := validateGithubRulesetRules(test.target, []githubRulesetRule{rule}); (err == nil) != test.wantValid {
					t.Fatalf("validateGithubRulesetRules(%q) = %v, wantValid=%v", test.target, err, test.wantValid)
				}
				return
			}
			if err == nil {
				t.Fatalf("decode update rule %s succeeded, want fail-closed rejection", fixture)
			}
		})
	}
}

func TestGithubRulesetParserAcceptsDocumentedNullableBypassActorIDs(t *testing.T) {
	fixture := `[{"actor_id":null,"actor_type":"DeployKey","bypass_mode":"always"},{"actor_id":null,"actor_type":"OrganizationAdmin","bypass_mode":"exempt"}]`
	var actors []githubRulesetBypassActor
	if err := json.Unmarshal([]byte(fixture), &actors); err != nil {
		t.Fatalf("documented nullable bypass actors rejected: %v", err)
	}
	if len(actors) != 2 || actors[0].ActorID != nil || actors[1].ActorID != nil {
		t.Fatalf("nullable bypass actors = %#v, want nil actor IDs", actors)
	}
	if err := validateGithubRulesetBypassActors(actors); err != nil {
		t.Fatalf("documented nullable bypass actors failed validation: %v", err)
	}
}

func TestResolveWorkflowTagRequiresExplicitAcceptedVerificationReason(t *testing.T) {
	for _, reason := range []string{"", "unsigned", "unknown_signature_type"} {
		t.Run("reason-"+reason, func(t *testing.T) {
			tagObjectSHA := strings.Repeat("a", 40)
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
					return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+tagObjectSHA+`","type":"tag"}}`, nil), nil
				}
				if strings.HasSuffix(req.URL.Path, "/git/tags/"+tagObjectSHA) {
					return githubResponse(http.StatusOK, `{"sha":"`+tagObjectSHA+`","object":{"sha":"`+strings.Repeat("b", 40)+`","type":"commit"},"verification":{"verified":true,"reason":"`+reason+`"}}`, nil), nil
				}
				return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
			}))
			if _, err := (&GithubBuildConfigService{}).ResolveWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1"); err == nil {
				t.Fatalf("ResolveWorkflowTag() accepted verification reason %q", reason)
			}
		})
	}
}

func TestResolveWorkflowTagRejectsShortSelectorBranchCollision(t *testing.T) {
	tagObjectSHA := strings.Repeat("a", 40)
	workflowSHA := strings.Repeat("b", 40)
	for _, test := range []struct {
		name          string
		branchCode    int
		branchBody    string
		transportErr  bool
		wantError     bool
		wantAPI       bool
		wantTransport bool
	}{
		{name: "no branch collision", branchCode: http.StatusNotFound, branchBody: `{"message":"branch not found"}`},
		{name: "same label branch", branchCode: http.StatusOK, branchBody: `{"ref":"refs/heads/workflow-v1","object":{"sha":"` + workflowSHA + `","type":"commit"}}`, wantError: true},
		{name: "provider outage", branchCode: http.StatusBadGateway, branchBody: `{"message":"provider unavailable"}`, wantError: true, wantAPI: true},
		{name: "provider transport error", transportErr: true, wantError: true, wantTransport: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/repos/owner/repo/git/ref/tags/workflow-v1":
					return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+tagObjectSHA+`","type":"tag"}}`, nil), nil
				case "/repos/owner/repo/git/tags/" + tagObjectSHA:
					return githubResponse(http.StatusOK, `{"sha":"`+tagObjectSHA+`","object":{"sha":"`+workflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
				case "/repos/owner/repo/git/ref/heads/workflow-v1":
					if test.transportErr {
						return nil, context.DeadlineExceeded
					}
					return githubResponse(test.branchCode, test.branchBody, nil), nil
				default:
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
					return nil, nil
				}
			}))

			identity, err := (&GithubBuildConfigService{}).ResolveWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1")
			if test.wantError {
				if err == nil {
					t.Fatalf("ResolveWorkflowTag() = %#v, nil; want collision/provider rejection", identity)
				}
				if test.wantAPI {
					var apiErr *GithubAPIError
					if !errors.As(err, &apiErr) || apiErr.Retryable != true || apiErr.Terminal {
						t.Fatalf("ResolveWorkflowTag() error = %T %v, want retryable provider error", err, err)
					}
				} else if test.wantTransport {
					var transportErr *GithubTransportError
					if !errors.As(err, &transportErr) || !IsGithubRetryable(err) || IsGithubTerminal(err) {
						t.Fatalf("ResolveWorkflowTag() error = %T %v, want retryable transport error", err, err)
					}
				} else {
					var approvalErr *WorkflowRefApprovalError
					if !errors.As(err, &approvalErr) {
						t.Fatalf("ResolveWorkflowTag() error = %T %v, want collision approval rejection", err, err)
					}
				}
				return
			}
			if err != nil || identity.Ref != "refs/tags/workflow-v1" || identity.SHA != workflowSHA {
				t.Fatalf("ResolveWorkflowTag() = %#v, %v; want verified tag identity", identity, err)
			}
		})
	}
}

func TestDispatchBuildRejectsBranchCollisionBeforeProviderPayloadPost(t *testing.T) {
	identity := githubVersionIdentity()
	config := githubConfig()
	var payloadPosts int
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/tags/protection":
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		case "/repos/owner/repo/rulesets":
			return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil), nil
		case "/repos/owner/repo/rulesets/1":
			return githubResponse(http.StatusOK, testProtectedRulesetResponse("workflow-*"), nil), nil
		case "/repos/owner/repo/git/ref/tags/workflow-v1":
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("e", 40)+`","type":"tag"}}`, nil), nil
		case "/repos/owner/repo/git/tags/" + strings.Repeat("e", 40):
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("e", 40)+`","object":{"sha":"`+identity.WorkflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		case "/repos/owner/repo/git/ref/heads/workflow-v1":
			return githubResponse(http.StatusOK, `{"ref":"refs/heads/workflow-v1","object":{"sha":"`+strings.Repeat("d", 40)+`","type":"commit"}}`, nil), nil
		case "/repos/owner/repo/contents/.github/workflows/rustqs-windows-min-test.yml":
			return githubResponse(http.StatusOK, testWorkflowFileResponse(windowsWorkflowFilename), nil), nil
		case "/repos/owner/repo/actions/workflows/rustqs-windows-min-test.yml":
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		default:
			if req.Method == http.MethodPost {
				payloadPosts++
			}
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	if _, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, identity, string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey}); err == nil {
		t.Fatal("DispatchBuild() error = nil, want branch collision rejection")
	}
	if payloadPosts != 0 {
		t.Fatalf("DispatchBuild() made %d provider payload POST(s), want none on branch collision", payloadPosts)
	}
}

func TestListWorkflowTagOptionsReturnsOnlyVerifiedReadyProviderTags(t *testing.T) {
	tagObjectSHA := strings.Repeat("a", 40)
	workflowSHA := strings.Repeat("b", 40)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/tags/protection"):
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		case strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/"):
			return testRulesetResponse(req, "workflow-*"), nil
		case strings.HasSuffix(req.URL.Path, "/rulesets"):
			return testRulesetResponse(req, "workflow-*"), nil
		case strings.HasSuffix(req.URL.Path, "/git/refs/tags"):
			return githubResponse(http.StatusOK, `[{"ref":"refs/tags/lightweight","object":{"sha":"`+strings.Repeat("c", 40)+`","type":"commit"}},{"ref":"refs/tags/unverified","object":{"sha":"`+strings.Repeat("d", 40)+`","type":"tag"}},{"ref":"refs/tags/workflow-v1","object":{"sha":"`+tagObjectSHA+`","type":"tag"}}]`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/ref/tags/unverified"):
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/unverified","object":{"sha":"`+strings.Repeat("d", 40)+`","type":"tag"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1"):
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+tagObjectSHA+`","type":"tag"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("d", 40)):
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("d", 40)+`","object":{"sha":"`+workflowSHA+`","type":"commit"},"verification":{"verified":false,"reason":"unsigned"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/tags/"+tagObjectSHA):
			return githubResponse(http.StatusOK, `{"sha":"`+tagObjectSHA+`","object":{"sha":"`+workflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		case strings.Contains(req.URL.Path, "/contents/.github/workflows/"):
			return githubResponse(http.StatusOK, testWorkflowFileResponse(windowsWorkflowFilename), nil), nil
		case strings.Contains(req.URL.Path, "/actions/workflows/"):
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))
	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if err != nil {
		t.Fatalf("ListWorkflowTagOptions() error = %v", err)
	}
	if len(options) != 1 || options[0].Tag != "workflow-v1" || options[0].Label != "workflow-v1" {
		t.Fatalf("workflow tag options = %#v, want only safe verified label", options)
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal workflow tag options: %v", err)
	}
	if strings.Contains(string(encoded), workflowSHA) || strings.Contains(string(encoded), "verification") || strings.Contains(string(encoded), "refs/tags") {
		t.Fatalf("workflow tag options exposed internal provider details: %s", encoded)
	}
}

func TestListWorkflowTagOptionsBoundsPagination(t *testing.T) {
	var pages int
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/git/refs/tags") {
			t.Fatalf("unexpected request after bounded pagination: %s %s", req.Method, req.URL.Path)
		}
		pages++
		return githubResponse(http.StatusOK, `[]`, http.Header{"Link": []string{`<https://api.github.com/repos/owner/repo/git/refs/tags?page=` + fmt.Sprint(pages+1) + `>; rel="next"`}}), nil
	}))
	_, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	var contractErr *GithubContractError
	if !errors.As(err, &contractErr) || pages != maxWorkflowTagPages {
		t.Fatalf("ListWorkflowTagOptions() = %T %v, pages=%d; want bounded pagination contract error at %d pages", err, err, pages, maxWorkflowTagPages)
	}
}

func TestDispatchBuildRejectsMovedWorkflowBeforeDFP1Payload(t *testing.T) {
	identity := githubVersionIdentity()
	config := githubConfig()
	var payloadRequests int
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/tags/protection"):
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		case strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/"):
			return testRulesetResponse(req, "workflow-*"), nil
		case strings.HasSuffix(req.URL.Path, "/rulesets"):
			return testRulesetResponse(req, "workflow-*"), nil
		case strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1"):
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("e", 40)+`","type":"tag"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("e", 40)):
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("e", 40)+`","object":{"sha":"`+strings.Repeat("e", 40)+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/contents/.github/workflows/rustqs-windows-min-test.yml"):
			return githubResponse(http.StatusOK, testWorkflowFileResponse(windowsWorkflowFilename), nil), nil
		case strings.HasSuffix(req.URL.Path, "/actions/workflows/rustqs-windows-min-test.yml"):
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		case req.Method == http.MethodPost:
			payloadRequests++
			return githubResponse(http.StatusOK, `{"workflow_run_id":1,"run_url":"https://api.github.com/runs/1","html_url":"https://github.com/runs/1"}`, nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	_, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, identity, string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey})
	if err == nil || payloadRequests != 0 {
		t.Fatalf("DispatchBuild() = %v, payload requests=%d; want moved-ref rejection before DFP1 dispatch", err, payloadRequests)
	}
}

func TestVerifyWorkflowAvailableRequiresWorkflowDispatch(t *testing.T) {
	workflowSHA := strings.Repeat("a", 40)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/contents/.github/workflows/") {
			return githubResponse(http.StatusOK, testWorkflowFileResponseWithContent("rustqs-windows-min-test.yml", "name: missing trigger\non:\n  push:\n"), nil), nil
		}
		t.Fatalf("unexpected readiness request after missing trigger: %s %s", req.Method, req.URL.Path)
		return nil, nil
	}))

	err := (&GithubBuildConfigService{}).verifyWorkflowAvailable(
		context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, windowsWorkflowFilename, workflowSHA,
	)
	var contractErr *GithubContractError
	if !errors.As(err, &contractErr) || !strings.Contains(err.Error(), "workflow_dispatch") {
		t.Fatalf("verifyWorkflowAvailable() error = %T %v, want missing-trigger contract error", err, err)
	}
}

func TestVerifyWorkflowAvailableAcceptsWorkflowDispatchForms(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
	}{
		{name: "block mapping", content: "on:\n  workflow_dispatch:\n"},
		{name: "inline sequence", content: "on: [push, workflow_dispatch]\n"},
		{name: "inline mapping", content: "on: {workflow_dispatch: {}}\n"},
		{name: "quoted key", content: "\"on\":\n  workflow_dispatch: {}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workflowSHA := strings.Repeat("b", 40)
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/contents/.github/workflows/") {
					if req.URL.Query().Get("ref") != workflowSHA {
						t.Fatalf("contents ref = %q, want exact workflow SHA", req.URL.Query().Get("ref"))
					}
					return githubResponse(http.StatusOK, testWorkflowFileResponseWithContent("rustqs-windows-min-test.yml", test.content), nil), nil
				}
				if strings.Contains(req.URL.Path, "/actions/workflows/") {
					return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
				}
				t.Fatalf("unexpected readiness request: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}))

			if err := (&GithubBuildConfigService{}).verifyWorkflowAvailable(
				context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, windowsWorkflowFilename, workflowSHA,
			); err != nil {
				t.Fatalf("verifyWorkflowAvailable() error = %v", err)
			}
		})
	}
}

func TestWorkflowExecutionRefNormalizesKnownLegacyValuesAndRejectsMutableValues(t *testing.T) {
	for _, test := range []struct {
		name string
		ref  string
		want string
		err  bool
	}{
		{name: "empty", ref: "", want: defaultWorkflowExecutionRef},
		{name: "legacy master", ref: "master", want: defaultWorkflowExecutionRef},
		{name: "legacy full master ref", ref: "refs/heads/master", want: defaultWorkflowExecutionRef},
		{name: "canonical full ref", ref: "refs/heads/" + defaultWorkflowExecutionRef, want: defaultWorkflowExecutionRef},
		{name: "explicit 40 character sha", ref: strings.Repeat("c", 40), want: strings.Repeat("c", 40)},
		{name: "explicit 64 character sha", ref: strings.Repeat("d", 64), want: strings.Repeat("d", 64)},
		{name: "explicit tag selector", ref: "refs/tags/workflow-v1", want: "refs/tags/workflow-v1"},
		{name: "unknown mutable ref", ref: "main", err: true},
		{name: "unknown legacy ref", ref: "legacy-ref", err: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := workflowExecutionRef(&model.GithubBuildConfig{Branch: test.ref})
			if test.err || strings.Contains(test.name, "sha") {
				if err == nil {
					t.Fatalf("workflowExecutionRef(%q) error = nil, want fail-closed selector rejection", test.ref)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("workflowExecutionRef(%q) = %q, %v; want %q", test.ref, got, err, test.want)
			}
		})
	}
}

func TestApproveWorkflowRefUsesClosedDomainAndOptionalEffectiveSelector(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	if err := db.Create(&model.GithubBuildConfig{IdModel: model.IdModel{Id: 1}, Repo: "owner/repo"}).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}
	workflowSHA := strings.Repeat("a", 40)
	annotatedTagSHA := strings.Repeat("b", 40)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/tags/protection"):
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		case strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/"):
			return testRulesetResponse(req, "workflow-*"), nil
		case strings.HasSuffix(req.URL.Path, "/rulesets"):
			return testRulesetResponse(req, "workflow-*"), nil
		case strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1"):
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+annotatedTagSHA+`","type":"tag"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/tags/"+annotatedTagSHA):
			return githubResponse(http.StatusOK, `{"sha":"`+annotatedTagSHA+`","object":{"sha":"`+workflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		case strings.Contains(req.URL.Path, "/contents/.github/workflows/"):
			return githubResponse(http.StatusOK, testWorkflowFileResponse(windowsWorkflowFilename), nil), nil
		case strings.Contains(req.URL.Path, "/actions/workflows/"):
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))
	svc := &GithubBuildConfigService{}
	if _, err := svc.ApproveWorkflowRef(""); err == nil {
		t.Fatal("ApproveWorkflowRef(\"\") accepted mutable/default selector")
	}
	approved, err := svc.ApproveWorkflowTag("workflow-v1")
	if err != nil {
		t.Fatalf("ApproveWorkflowTag() error = %v", err)
	}
	if approved.Branch != "refs/tags/workflow-v1" || !approved.WorkflowRefApproved || approved.WorkflowRefApprovalSHA != workflowSHA {
		t.Fatalf("tag approval = %#v, want approved immutable selector", approved)
	}

	if err := db.Model(&model.GithubBuildConfig{}).Where("id = 1").Updates(map[string]any{"workflow_ref_approved": false}).Error; err != nil {
		t.Fatalf("reset approval: %v", err)
	}
	approved, err = svc.ApproveWorkflowRef("refs/tags/workflow-v1")
	if err != nil {
		t.Fatalf("ApproveWorkflowRef(tag) error = %v", err)
	}
	if approved.Branch != "refs/tags/workflow-v1" || !approved.WorkflowRefApproved {
		t.Fatalf("tag approval = %#v, want approved tag selector", approved)
	}

	for _, selector := range []string{"main", strings.Repeat("a", 40), "refs/heads/main", "refs/tags/" + strings.Repeat("b", 40)} {
		t.Run(selector, func(t *testing.T) {
			if _, err := svc.ApproveWorkflowRef(selector); err == nil {
				t.Fatalf("ApproveWorkflowRef(%q) error = nil, want closed-domain rejection", selector)
			} else {
				var approvalErr *WorkflowRefApprovalError
				if !errors.As(err, &approvalErr) {
					t.Fatalf("ApproveWorkflowRef(%q) error = %T %v, want WorkflowRefApprovalError", selector, err, err)
				}
			}
		})
	}
}

func TestApproveWorkflowRefPreservesLegacyPlaintextSecretsWithoutEncryptionKey(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})

	const token = "legacy-github-pat"
	const payloadKey = "legacy-payload-key"
	if err := db.Exec(`INSERT INTO github_build_configs (id, repo, branch, workflow_ref_approved, token, payload_key) VALUES (?, ?, ?, ?, ?, ?)`,
		1, "owner/repo", defaultWorkflowExecutionRef, false, token, payloadKey).Error; err != nil {
		t.Fatalf("seed legacy config: %v", err)
	}
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/tags/protection") {
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		}
		if strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/") {
			return testRulesetResponse(req, "workflow-*"), nil
		}
		if strings.HasSuffix(req.URL.Path, "/rulesets") {
			return testRulesetResponse(req, "workflow-*"), nil
		}
		if strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("b", 40)+`","type":"tag"}}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("b", 40)) {
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("b", 40)+`","object":{"sha":"`+strings.Repeat("a", 40)+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		}
		if strings.Contains(req.URL.Path, "/contents/.github/workflows/") {
			return githubResponse(http.StatusOK, testWorkflowFileResponse(windowsWorkflowFilename), nil), nil
		}
		if strings.Contains(req.URL.Path, "/actions/workflows/") {
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
	}))

	approved, err := (&GithubBuildConfigService{}).ApproveWorkflowTag("workflow-v1")
	if err != nil {
		t.Fatalf("ApproveWorkflowRef() legacy error = %v", err)
	}
	if approved.Branch != "refs/tags/workflow-v1" || !approved.WorkflowRefApproved {
		t.Fatalf("legacy approval = %#v, want approved tag selector", approved)
	}
	if rawToken, rawPayloadKey := rawGithubConfigSecrets(t, db); rawToken != token || rawPayloadKey != payloadKey {
		t.Fatalf("legacy secrets changed during approval: token preserved=%v payload key preserved=%v", rawToken == token, rawPayloadKey == payloadKey)
	}
}

func TestApproveWorkflowRefPreservesNewEncryptedSecrets(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})

	const token = "new-github-pat"
	const payloadKey = "new-payload-key"
	if err := db.Create(&model.GithubBuildConfig{
		IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Branch: defaultWorkflowExecutionRef,
		Token: token, PayloadKey: payloadKey,
	}).Error; err != nil {
		t.Fatalf("seed encrypted config: %v", err)
	}
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/tags/protection") {
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		}
		if strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/") {
			return testRulesetResponse(req, "workflow-*"), nil
		}
		if strings.HasSuffix(req.URL.Path, "/rulesets") {
			return testRulesetResponse(req, "workflow-*"), nil
		}
		if strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("b", 40)+`","type":"tag"}}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("b", 40)) {
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("b", 40)+`","object":{"sha":"`+strings.Repeat("a", 40)+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		}
		if strings.Contains(req.URL.Path, "/contents/.github/workflows/") {
			return githubResponse(http.StatusOK, testWorkflowFileResponse(windowsWorkflowFilename), nil), nil
		}
		if strings.Contains(req.URL.Path, "/actions/workflows/") {
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
	}))
	rawToken, rawPayloadKey := rawGithubConfigSecrets(t, db)
	if !utils.IsEncryptedSecret(rawToken) || !utils.IsEncryptedSecret(rawPayloadKey) {
		t.Fatal("seed config did not use encrypted secret storage")
	}

	if _, err := (&GithubBuildConfigService{}).ApproveWorkflowTag("workflow-v1"); err != nil {
		t.Fatalf("ApproveWorkflowRef() encrypted error = %v", err)
	}
	if gotToken, gotPayloadKey := rawGithubConfigSecrets(t, db); gotToken != rawToken || gotPayloadKey != rawPayloadKey {
		t.Fatalf("encrypted secrets were reserialized during approval: token preserved=%v payload key preserved=%v", gotToken == rawToken, gotPayloadKey == rawPayloadKey)
	}
}

func TestGithubConfigRepoChangePreservesLegacySecretsAndClearsApproval(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Setenv(utils.SecretEncryptionKeyEnv, "")
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})

	const token = "legacy-repo-change-pat"
	const payloadKey = "legacy-repo-change-key"
	if err := db.Exec(`INSERT INTO github_build_configs (id, repo, branch, workflow_ref_approved, token, payload_key) VALUES (?, ?, ?, ?, ?, ?)`,
		1, "owner/old", defaultWorkflowExecutionRef, true, token, payloadKey).Error; err != nil {
		t.Fatalf("seed legacy repo config: %v", err)
	}

	if err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{Repo: "owner/new"}); err != nil {
		t.Fatalf("Save() legacy repo change error = %v", err)
	}
	loaded, err := (&GithubBuildConfigService{}).Get()
	if err != nil {
		t.Fatalf("reload repo config: %v", err)
	}
	if loaded.Repo != "owner/new" || loaded.WorkflowRefApproved {
		t.Fatalf("repo change result = %#v, want new repo and cleared approval", loaded)
	}
	if rawToken, rawPayloadKey := rawGithubConfigSecrets(t, db); rawToken != token || rawPayloadKey != payloadKey {
		t.Fatalf("legacy secrets changed during repo reset: token preserved=%v payload key preserved=%v", rawToken == token, rawPayloadKey == payloadKey)
	}
}

func TestGithubBuildConfigSaveClearsWorkflowApprovalWhenRepoChanges(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	if err := db.Create(&model.GithubBuildConfig{
		IdModel: model.IdModel{Id: 1}, Repo: "owner/old", Branch: defaultWorkflowExecutionRef, WorkflowRefApproved: true,
	}).Error; err != nil {
		t.Fatalf("seed approved config: %v", err)
	}
	if err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{Repo: "owner/new"}); err != nil {
		t.Fatalf("Save(repo change) error = %v", err)
	}
	loaded, err := (&GithubBuildConfigService{}).Get()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if loaded.Repo != "owner/new" || loaded.WorkflowRefApproved {
		t.Fatalf("repo reset result = %#v, want new repo and cleared approval", loaded)
	}
}

func TestPrepareBuildRejectsUnapprovedWorkflowBeforeProviderOrBuildRow(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	if err := db.Create(&model.GithubBuildConfig{
		IdModel: model.IdModel{Id: 1}, Repo: "owner/repo", Token: "github_pat_test", PayloadKey: "payload-key",
	}).Error; err != nil {
		t.Fatalf("seed unapproved config: %v", err)
	}
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected provider request for unapproved workflow: %s %s", req.Method, req.URL.Path)
		return nil, nil
	}))

	_, err := (&GithubBuildConfigService{}).PrepareBuild(context.Background(), string(PlatformWindows), "1.2.3")
	var approvalErr *WorkflowRefApprovalError
	if !errors.As(err, &approvalErr) {
		t.Fatalf("PrepareBuild() error = %T %v, want WorkflowRefApprovalError", err, err)
	}
	var count int64
	if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
		t.Fatalf("count builds: %v", err)
	}
	if count != 0 {
		t.Fatalf("unapproved workflow left %d build row(s)", count)
	}
}

func TestDispatchBuildRejectsUnapprovedWorkflowBeforeProvider(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected provider request for unapproved workflow: %s %s", req.Method, req.URL.Path)
		return nil, nil
	}))
	config := githubConfig()
	config.WorkflowRefApproved = false
	_, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), config, githubVersionIdentity(), string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey})
	var approvalErr *WorkflowRefApprovalError
	if !errors.As(err, &approvalErr) {
		t.Fatalf("DispatchBuild() error = %T %v, want WorkflowRefApprovalError", err, err)
	}
}

func TestGithubBuildConfigGetNormalizesKnownLegacyBranchAtConfigBoundary(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	if err := db.Create(&model.GithubBuildConfig{
		IdModel: model.IdModel{Id: 1},
		Repo:    "owner/repo",
		Branch:  "master",
	}).Error; err != nil {
		t.Fatalf("seed legacy GitHub config: %v", err)
	}

	config, err := (&GithubBuildConfigService{}).Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if config.Branch != defaultWorkflowExecutionRef {
		t.Fatalf("normalized config branch = %q, want %q", config.Branch, defaultWorkflowExecutionRef)
	}

	if err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{Repo: "owner/repo"}); err != nil {
		t.Fatalf("Save() after legacy normalization error = %v", err)
	}
	var stored model.GithubBuildConfig
	if err := db.First(&stored, 1).Error; err != nil {
		t.Fatalf("reload normalized config: %v", err)
	}
	if stored.Branch != defaultWorkflowExecutionRef {
		t.Fatalf("persisted normalized branch = %q, want %q", stored.Branch, defaultWorkflowExecutionRef)
	}
}

func TestRequireProductionBuildCapabilityKeepsUnvalidatedPlatformsFailClosed(t *testing.T) {
	for _, platform := range []string{string(PlatformLinux), string(PlatformAndroid)} {
		t.Run(platform, func(t *testing.T) {
			err := RequireProductionBuildCapability(platform)
			var unavailable *ProductionCapabilityUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("RequireProductionBuildCapability(%q) = %T %v, want explicit capability-unavailable error", platform, err, err)
			}
			if unavailable.Platform != platform || unavailable.Capability == "" {
				t.Fatalf("capability error = %#v, want platform and capability", unavailable)
			}
		})
	}
	if err := RequireProductionBuildCapability(string(PlatformWindows)); err != nil {
		t.Fatalf("Windows production capability = %v, want available", err)
	}
	if err := RequireProductionBuildCapability(string(PlatformLinux)); err == nil {
		t.Fatal("Linux production capability error = nil")
	}
}

func TestRequireDispatchPublicKeyRejectsMalformedMaterial(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   any
		wantErr string
	}{
		{name: "missing", value: nil, wantErr: "missing"},
		{name: "whitespace-only", value: " \t", wantErr: "whitespace-only"},
		{name: "control", value: validRustDeskPublicKey[:8] + "\x00" + validRustDeskPublicKey[8:], wantErr: "control"},
		{name: "malformed", value: "public-key", wantErr: "base64"},
		{name: "wrong length", value: "cHVibGljLWtleQ==", wantErr: "32 bytes"},
		{name: "valid trailing line endings", value: validRustDeskPublicKey + "\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := map[string]any{}
			if test.value != nil {
				params["key"] = test.value
			}
			err := RequireDispatchPublicKey(params)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("RequireDispatchPublicKey() error = %v, want valid key", err)
				}
				return
			}
			var providerErr *GithubProviderConfigurationError
			if !errors.As(err, &providerErr) || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("RequireDispatchPublicKey() error = %T %v, want typed provider error containing %q", err, err, test.wantErr)
			}
		})
	}
}

func TestPrepareBuildRejectsUnvalidatedPlatformBeforeProviderOrBuildRow(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected provider request for unavailable platform: %s %s", req.Method, req.URL.Path)
		return nil, nil
	}))

	_, err := (&GithubBuildConfigService{}).PrepareBuild(context.Background(), string(PlatformLinux), "1.2.3")
	var unavailable *ProductionCapabilityUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("PrepareBuild() error = %T %v, want explicit capability-unavailable error", err, err)
	}
	var count int64
	if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
		t.Fatalf("count builds: %v", err)
	}
	if count != 0 {
		t.Fatalf("unavailable platform left %d build row(s)", count)
	}
}

func TestGithubBuildConfigSafeAndSaveExposeOnlyProviderContract(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})

	stored := &model.GithubBuildConfig{
		IdModel:          model.IdModel{Id: 1},
		Repo:             "owner/old",
		WorkflowFilename: "legacy.yml",
		Branch:           "legacy-ref",
		Token:            "old-token",
		PayloadKey:       "old-key",
	}
	if err := db.Create(stored).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{
		Repo:             "owner/new",
		WorkflowFilename: "attacker.yml",
		Branch:           "attacker-ref",
		Token:            "new-token",
		PayloadKey:       "new-key",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var loaded model.GithubBuildConfig
	if err := db.First(&loaded, 1).Error; err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Repo != "owner/new" || loaded.Token != "new-token" || loaded.PayloadKey != "new-key" {
		t.Fatalf("saved provider contract = %#v", loaded)
	}
	if loaded.WorkflowFilename != "legacy.yml" || loaded.Branch != "legacy-ref" {
		t.Fatalf("legacy workflow/ref columns were rewritten: %#v", loaded)
	}

	encoded, err := json.Marshal(loaded.Safe())
	if err != nil {
		t.Fatalf("marshal safe config: %v", err)
	}
	for _, forbidden := range []string{"workflow_filename", "branch", "legacy.yml", "legacy-ref"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe config exposes forbidden value %q: %s", forbidden, encoded)
		}
	}
}

func TestPrepareBuildRejectsMissingProviderBeforeBuildPersistence(t *testing.T) {
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
	})

	_, err := (&GithubBuildConfigService{}).PrepareBuild(context.Background(), string(PlatformWindows), "1.2.3")
	var providerErr *GithubProviderConfigurationError
	if !errors.As(err, &providerErr) {
		t.Fatalf("PrepareBuild() error = %T %v, want provider configuration error", err, err)
	}
	var count int64
	if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
		t.Fatalf("count builds: %v", err)
	}
	if count != 0 {
		t.Fatalf("provider readiness failure left %d build row(s)", count)
	}
}

func TestPrepareBuildRejectsUnavailableWorkflowBeforeBuildPersistence(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		retryable  bool
		terminal   bool
		response   string
		expectType string
	}{
		{name: "401 unauthorized", status: http.StatusUnauthorized, terminal: true, response: `{"message":"bad credentials"}`, expectType: "api"},
		{name: "403 forbidden", status: http.StatusForbidden, terminal: true, response: `{"message":"workflow access denied"}`, expectType: "api"},
		{name: "404 missing tag", status: http.StatusNotFound, terminal: true, response: `{"message":"workflow tag not found"}`, expectType: "api"},
		{name: "500 provider outage", status: http.StatusInternalServerError, retryable: true, response: `{"message":"provider unavailable"}`, expectType: "api"},
		{name: "workflow file contract", status: http.StatusOK, terminal: true, response: `{"state":"disabled_manually"}`, expectType: "contract"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, sqlDB := newGithubConfigTestDB(t)
			previousDB := DB
			DB = db
			t.Cleanup(func() {
				DB = previousDB
				_ = sqlDB.Close()
			})
			if err := db.Create(&model.GithubBuildConfig{
				IdModel:                     model.IdModel{Id: 1},
				Repo:                        "owner/repo",
				Token:                       "github_pat_test",
				PayloadKey:                  "payload-key",
				Branch:                      "refs/tags/workflow-v1",
				WorkflowRefApproved:         true,
				WorkflowRefProviderVerified: true,
				WorkflowRefApprovalSHA:      strings.Repeat("b", 40),
			}).Error; err != nil {
				t.Fatalf("seed GitHub config: %v", err)
			}

			var paths []string
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				paths = append(paths, req.Method+" "+req.URL.Path)
				if strings.HasSuffix(req.URL.Path, "/tags/protection") {
					return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
				}
				if strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/") {
					return testRulesetResponse(req, "workflow-*"), nil
				}
				if strings.HasSuffix(req.URL.Path, "/rulesets") {
					return testRulesetResponse(req, "workflow-*"), nil
				}
				if strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1") {
					if tc.expectType == "contract" {
						return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("a", 40)+`","type":"tag"}}`, nil), nil
					}
					return githubResponse(tc.status, tc.response, nil), nil
				}
				if strings.HasSuffix(req.URL.Path, "/git/tags/"+strings.Repeat("a", 40)) {
					return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("a", 40)+`","object":{"sha":"`+strings.Repeat("b", 40)+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
				}
				if strings.HasSuffix(req.URL.Path, "/git/ref/heads/workflow-v1") {
					return githubResponse(http.StatusNotFound, `{"message":"branch not found"}`, nil), nil
				}
				if strings.Contains(req.URL.Path, "/contents/.github/workflows/") {
					return githubResponse(http.StatusOK, tc.response, nil), nil
				}
				return githubResponse(tc.status, tc.response, nil), nil
			}))

			_, err := (&GithubBuildConfigService{}).PrepareBuild(context.Background(), string(PlatformWindows), "1.2.3")
			var providerErr *GithubProviderConfigurationError
			if !errors.As(err, &providerErr) {
				t.Fatalf("PrepareBuild() error = %T %v, want provider configuration error", err, err)
			}
			expectedPaths := []string{"GET /repos/owner/repo/rulesets", "GET /repos/owner/repo/rulesets/1", "GET /repos/owner/repo/git/ref/tags/workflow-v1"}
			if tc.expectType == "contract" {
				expectedPaths = append(expectedPaths, "GET /repos/owner/repo/git/tags/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "GET /repos/owner/repo/git/ref/heads/workflow-v1", "GET /repos/owner/repo/contents/.github/workflows/rustqs-windows-min-test.yml")
			}
			if len(paths) != len(expectedPaths) {
				t.Fatalf("readiness requests = %v, want %v", paths, expectedPaths)
			}
			for i := range expectedPaths {
				if paths[i] != expectedPaths[i] {
					t.Fatalf("readiness requests = %v, want %v", paths, expectedPaths)
				}
			}
			switch tc.expectType {
			case "api":
				var apiErr *GithubAPIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("PrepareBuild() error = %T %v, want wrapped GithubAPIError", err, err)
				}
				if apiErr.Retryable != tc.retryable || apiErr.Terminal != tc.terminal {
					t.Fatalf("provider classification = retryable:%v terminal:%v, want retryable:%v terminal:%v", apiErr.Retryable, apiErr.Terminal, tc.retryable, tc.terminal)
				}
			case "contract":
				var contractErr *GithubContractError
				if !errors.As(err, &contractErr) || !IsGithubTerminal(err) || IsGithubRetryable(err) {
					t.Fatalf("PrepareBuild() error = %T %v, want terminal GithubContractError", err, err)
				}
			}

			var count int64
			if err := db.Model(&model.CustomBuild{}).Count(&count).Error; err != nil {
				t.Fatalf("count builds: %v", err)
			}
			if count != 0 {
				t.Fatalf("workflow readiness failure left %d build row(s)", count)
			}
		})
	}
}

func TestPrepareBuildAcceptsActiveMappedWorkflowAndPreservesCatalogIdentity(t *testing.T) {
	resetVersionCatalogCache()
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
		resetVersionCatalogCache()
	})
	if err := db.Create(&model.GithubBuildConfig{
		IdModel:                     model.IdModel{Id: 1},
		Repo:                        "owner/repo",
		Token:                       "github_pat_test",
		PayloadKey:                  "payload-key",
		Branch:                      "refs/tags/workflow-v1",
		WorkflowRefApproved:         true,
		WorkflowRefProviderVerified: true,
		WorkflowRefApprovalSHA:      strings.Repeat("b", 40),
	}).Error; err != nil {
		t.Fatalf("seed GitHub config: %v", err)
	}
	buildRef := strings.Repeat("a", 40)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/tags/protection":
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		case "/repos/owner/repo/rulesets":
			return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil), nil
		case "/repos/owner/repo/rulesets/1":
			return githubResponse(http.StatusOK, testProtectedRulesetResponse("workflow-*"), nil), nil
		case "/repos/owner/repo/git/ref/tags/workflow-v1":
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("a", 40)+`","type":"tag"}}`, nil), nil
		case "/repos/owner/repo/git/tags/" + strings.Repeat("a", 40):
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("a", 40)+`","object":{"sha":"`+strings.Repeat("b", 40)+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		case "/repos/owner/repo/contents/.github/workflows/rustqs-windows-min-test.yml":
			if req.URL.Query().Get("ref") != strings.Repeat("b", 40) {
				t.Fatalf("workflow readiness ref = %q, want immutable workflow SHA", req.URL.Query().Get("ref"))
			}
			return githubResponse(http.StatusOK, testWorkflowFileResponse("rustqs-windows-min-test.yml"), nil), nil
		case "/repos/owner/repo/actions/workflows/rustqs-windows-min-test.yml":
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		case "/repos/owner/repo/releases":
			return githubResponse(http.StatusOK, `[{"id":12,"tag_name":"offline-assets-1.2.3"}]`, nil), nil
		case "/repos/owner/repo/releases/12":
			return githubResponse(http.StatusOK, testReleaseDetails(12, "offline-assets-1.2.3"), nil), nil
		case "/repos/owner/repo/git/ref/tags/1.2.3":
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/1.2.3","object":{"sha":"`+buildRef+`","type":"commit"}}`, nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	prepared, err := (&GithubBuildConfigService{}).PrepareBuild(context.Background(), string(PlatformWindows), "1.2.3")
	if err != nil {
		t.Fatalf("PrepareBuild() error = %v", err)
	}
	if prepared.Identity.Repo != "owner/repo" || prepared.Identity.BuildRef != buildRef || prepared.Identity.SourceTag != "1.2.3" || prepared.Identity.WorkflowRef != "refs/tags/workflow-v1" || prepared.Identity.WorkflowSHA != strings.Repeat("b", 40) {
		t.Fatalf("prepared identity = %#v, want catalog-resolved identity", prepared.Identity)
	}
}

func TestPreparedConfigSnapshotSurvivesGlobalConfigMutationBeforeDispatch(t *testing.T) {
	resetVersionCatalogCache()
	db, sqlDB := newGithubConfigTestDB(t)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
		resetVersionCatalogCache()
	})
	if err := db.Create(&model.GithubBuildConfig{
		IdModel:                     model.IdModel{Id: 1},
		Repo:                        "owner/repo-a",
		Token:                       "token-a",
		PayloadKey:                  "payload-a",
		Branch:                      "refs/tags/workflow-v1",
		WorkflowRefApproved:         true,
		WorkflowRefProviderVerified: true,
		WorkflowRefApprovalSHA:      strings.Repeat("b", 40),
	}).Error; err != nil {
		t.Fatalf("seed GitHub config: %v", err)
	}
	buildRef := strings.Repeat("a", 40)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo-a/tags/protection":
			return githubResponse(http.StatusOK, testLegacyProtectedTagResponse("workflow-*"), nil), nil
		case "/repos/owner/repo-a/rulesets":
			return githubResponse(http.StatusOK, testRulesetSummaryResponse(), nil), nil
		case "/repos/owner/repo-a/rulesets/1":
			return githubResponse(http.StatusOK, testProtectedRulesetResponse("workflow-*"), nil), nil
		case "/repos/owner/repo-a/git/ref/tags/workflow-v1":
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+strings.Repeat("a", 40)+`","type":"tag"}}`, nil), nil
		case "/repos/owner/repo-a/git/tags/" + strings.Repeat("a", 40):
			return githubResponse(http.StatusOK, `{"sha":"`+strings.Repeat("a", 40)+`","object":{"sha":"`+strings.Repeat("b", 40)+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		case "/repos/owner/repo-a/contents/.github/workflows/rustqs-windows-min-test.yml":
			if req.URL.Query().Get("ref") != strings.Repeat("b", 40) {
				t.Fatalf("workflow readiness ref = %q, want immutable workflow SHA", req.URL.Query().Get("ref"))
			}
			return githubResponse(http.StatusOK, testWorkflowFileResponse("rustqs-windows-min-test.yml"), nil), nil
		case "/repos/owner/repo-a/actions/workflows/rustqs-windows-min-test.yml":
			return githubResponse(http.StatusOK, testWorkflowStateResponse(), nil), nil
		case "/repos/owner/repo-a/releases":
			return githubResponse(http.StatusOK, `[{"id":12,"tag_name":"offline-assets-1.2.3"}]`, nil), nil
		case "/repos/owner/repo-a/releases/12":
			return githubResponse(http.StatusOK, testReleaseDetails(12, "offline-assets-1.2.3"), nil), nil
		case "/repos/owner/repo-a/git/ref/tags/1.2.3":
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/1.2.3","object":{"sha":"`+buildRef+`","type":"commit"}}`, nil), nil
		case "/repos/owner/repo-a/actions/workflows/rustqs-windows-min-test.yml/dispatches":
			if req.Header.Get("Authorization") != "Bearer token-a" {
				return githubResponse(http.StatusBadRequest, `{"message":"dispatch used mutated token"}`, nil), nil
			}
			return githubResponse(http.StatusOK, `{"workflow_run_id":12345,"run_url":"https://api.github.com/repos/owner/repo-a/actions/runs/12345","html_url":"https://github.com/owner/repo-a/actions/runs/12345"}`, nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	prepared, err := (&GithubBuildConfigService{}).PrepareBuild(context.Background(), string(PlatformWindows), "1.2.3")
	if err != nil {
		t.Fatalf("PrepareBuild() error = %v", err)
	}
	if err := (&GithubBuildConfigService{}).Save(&model.GithubBuildConfig{
		Repo:       "owner/repo-b",
		Token:      "token-b",
		PayloadKey: "payload-b",
	}); err != nil {
		t.Fatalf("mutate global GitHub config: %v", err)
	}
	if prepared.Config.Repo != "owner/repo-a" || prepared.Config.Token != "token-a" || prepared.Config.PayloadKey != "payload-a" {
		t.Fatalf("prepared config snapshot changed after global mutation: %#v", prepared.Config)
	}

	result, err := (&GithubBuildConfigService{}).DispatchBuild(context.Background(), prepared.Config.ProviderConfig(), prepared.Identity, string(PlatformWindows), map[string]any{"key": validRustDeskPublicKey})
	if err != nil {
		t.Fatalf("DispatchBuild() error = %v", err)
	}
	if result.WorkflowRunID != 12345 {
		t.Fatalf("DispatchBuild() run id = %d, want exact snapshot dispatch", result.WorkflowRunID)
	}
}

func newGithubConfigTestDB(t *testing.T) (*gorm.DB, *sql.DB) {
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
	if err := db.AutoMigrate(&model.GithubBuildConfig{}, &model.CustomBuild{}); err != nil {
		t.Fatalf("migrate config test models: %v", err)
	}
	return db, sqlDB
}

func rawGithubConfigSecrets(t *testing.T, db *gorm.DB) (string, string) {
	t.Helper()
	var raw struct {
		Token      string
		PayloadKey string
	}
	if err := db.Table("github_build_configs").Select("token, payload_key").Where("id = ?", 1).Scan(&raw).Error; err != nil {
		t.Fatalf("read raw config secrets: %v", err)
	}
	return raw.Token, raw.PayloadKey
}
