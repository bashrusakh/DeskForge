package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"rustdesk-server/api/model"
)

// workflowTagCatalogHarness is a deterministic, controllable GitHub transport
// for workflow-tag catalog tests. It records every request path so tests can
// assert call counts, and it lets a test control per-candidate completion order
// and inject fatal failures without relying on wall-clock timing.
type workflowTagCatalogHarness struct {
	mu sync.Mutex

	// tagRefPages maps a 1-based tag-ref page to its body and next link.
	tagRefPages map[int]workflowTagCatalogPage
	// rulesetPages maps a 1-based ruleset page to its body and next link.
	rulesetPages map[int]workflowTagCatalogPage
	// details maps a ruleset id to its raw detail body.
	details map[int64]string
	// detailErrors maps a ruleset id to a transport failure.
	detailErrors map[int64]error
	// tagObjects maps an annotated object SHA to its raw /git/tags/<sha> body.
	tagObjects map[string]string
	// tagObjectErrors maps an annotated object SHA to a transport failure.
	tagObjectErrors map[string]error
	// contents maps a workflow SHA to the /contents response body.
	contents map[string]string
	// workflowState is the /actions/workflows response body.
	workflowState string
	// branchRefs maps a short tag label to a /git/ref/heads/<tag> response.
	branchRefs map[string]workflowTagCatalogBranch

	// candidateGate, keyed by annotated object SHA, parks that candidate's
	// tag-object request until the channel is closed. Parked candidates report
	// their SHA on parked so a test can wait deterministically.
	candidateGate map[string]<-chan struct{}
	parked        chan string

	activeCandidates int
	maxActive        int
	refTagGets       int
	paths            []string
}

type workflowTagCatalogPage struct {
	body string
	next string
}

type workflowTagCatalogBranch struct {
	status int
	body   string
}

func newWorkflowTagCatalogHarness() *workflowTagCatalogHarness {
	return &workflowTagCatalogHarness{
		tagRefPages:     map[int]workflowTagCatalogPage{},
		rulesetPages:    map[int]workflowTagCatalogPage{},
		details:         map[int64]string{},
		detailErrors:    map[int64]error{},
		tagObjects:      map[string]string{},
		tagObjectErrors: map[string]error{},
		contents:        map[string]string{},
		workflowState:   testWorkflowStateResponse(),
		branchRefs:      map[string]workflowTagCatalogBranch{},
		candidateGate:   map[string]<-chan struct{}{},
		parked:          make(chan string, 32),
	}
}

func (h *workflowTagCatalogHarness) roundTrip(req *http.Request) (*http.Response, error) {
	path := req.URL.Path
	h.mu.Lock()
	h.paths = append(h.paths, req.Method+" "+path)
	h.mu.Unlock()

	switch {
	case strings.HasSuffix(path, "/git/refs/tags"):
		pageData, ok := h.tagRefPages[queryPage(req)]
		if !ok {
			return githubResponse(http.StatusOK, `[]`, nil), nil
		}
		return githubResponse(http.StatusOK, pageData.body, headerWithLink(pageData.next)), nil
	case strings.HasPrefix(path, "/repos/owner/repo/rulesets/"):
		idText := strings.TrimPrefix(path, "/repos/owner/repo/rulesets/")
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil {
			return githubResponse(http.StatusNotFound, `{"message":"bad ruleset id"}`, nil), nil
		}
		if failure, ok := h.detailErrors[id]; ok {
			return nil, failure
		}
		if body, ok := h.details[id]; ok {
			return githubResponse(http.StatusOK, body, nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"no such ruleset"}`, nil), nil
	case strings.HasSuffix(path, "/rulesets"):
		pageData, ok := h.rulesetPages[queryPage(req)]
		if !ok {
			return githubResponse(http.StatusOK, `[]`, nil), nil
		}
		return githubResponse(http.StatusOK, pageData.body, headerWithLink(pageData.next)), nil
	case strings.HasPrefix(path, "/repos/owner/repo/git/tags/"):
		sha := strings.TrimPrefix(path, "/repos/owner/repo/git/tags/")
		h.enterCandidate(sha)
		defer h.leaveCandidate()
		if failure, ok := h.tagObjectErrors[sha]; ok {
			return nil, failure
		}
		if body, ok := h.tagObjects[sha]; ok {
			return githubResponse(http.StatusOK, body, nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"no such tag object"}`, nil), nil
	case strings.HasPrefix(path, "/repos/owner/repo/git/ref/heads/"):
		tag := strings.TrimPrefix(path, "/repos/owner/repo/git/ref/heads/")
		if branch, ok := h.branchRefs[tag]; ok {
			return githubResponse(branch.status, branch.body, nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"branch not found"}`, nil), nil
	case strings.HasPrefix(path, "/repos/owner/repo/git/ref/tags/"):
		h.mu.Lock()
		h.refTagGets++
		h.mu.Unlock()
		return githubResponse(http.StatusNotFound, `{"message":"unexpected tag ref GET"}`, nil), nil
	case strings.Contains(path, "/contents/.github/workflows/"):
		sha := req.URL.Query().Get("ref")
		if body, ok := h.contents[sha]; ok {
			return githubResponse(http.StatusOK, body, nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"no workflow at sha"}`, nil), nil
	case strings.Contains(path, "/actions/workflows/"):
		return githubResponse(http.StatusOK, h.workflowState, nil), nil
	default:
		return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
	}
}

func (h *workflowTagCatalogHarness) enterCandidate(sha string) {
	h.mu.Lock()
	h.activeCandidates++
	if h.activeCandidates > h.maxActive {
		h.maxActive = h.activeCandidates
	}
	gate := h.candidateGate[sha]
	h.mu.Unlock()
	if gate != nil {
		h.parked <- sha
		<-gate
	}
}

func (h *workflowTagCatalogHarness) leaveCandidate() {
	h.mu.Lock()
	h.activeCandidates--
	h.mu.Unlock()
}

func (h *workflowTagCatalogHarness) use(t *testing.T) {
	t.Helper()
	withGithubTransport(t, githubRoundTripFunc(h.roundTrip))
}

func (h *workflowTagCatalogHarness) snapshotPaths() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.paths...)
}

func (h *workflowTagCatalogHarness) maxConcurrent() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.maxActive
}

func (h *workflowTagCatalogHarness) tagRefGetCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.refTagGets
}

func queryPage(req *http.Request) int {
	page, _ := strconv.Atoi(req.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}
	return page
}

func headerWithLink(next string) http.Header {
	headers := http.Header{}
	if next != "" {
		headers.Set("Link", next)
	}
	return headers
}

func workflowTagRef(name, objectSHA, objectType string) string {
	return fmt.Sprintf(`{"ref":"refs/tags/%s","object":{"sha":"%s","type":"%s"}}`, name, objectSHA, objectType)
}

func workflowTagRefList(refs ...string) string {
	return "[" + strings.Join(refs, ",") + "]"
}

// hexSHA builds a 40-character hexadecimal SHA from one hex digit. Valid hex
// characters are required: the production validators reject non-hex input.
func hexSHA(digit rune) string {
	return strings.Repeat(string(digit), 40)
}

func workflowTagObjectBody(objectSHA, commitSHA string, verified bool, reason string) string {
	return fmt.Sprintf(`{"sha":"%s","object":{"sha":"%s","type":"commit"},"verification":{"verified":%t,"reason":%q}}`,
		objectSHA, commitSHA, verified, reason)
}

// protectedRuleset builds one active tag ruleset whose ref_name include is the
// exact prefix glob "<prefix>*" with update+deletion protection and no bypass
// actors.
func protectedRuleset(id int64, prefix string) string {
	pattern := "refs/tags/" + prefix + "*"
	return fmt.Sprintf(`{"id":%d,%s,"target":"tag","enforcement":"active","bypass_actors":[],"current_user_can_bypass":"never","conditions":{"ref_name":{"include":[%q],"exclude":[]}},"rules":[{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}]}`,
		id, testRulesetMetadata, pattern)
}

func workflowTagRulesetSummary(id int64) string {
	return fmt.Sprintf(`[{"id":%d,"target":"tag","source_type":"Repository","source":"owner/repo","enforcement":"active"}]`, id)
}

// workflowTagRulesetSummaries builds a single list body with multiple ruleset
// summary entries.
func workflowTagRulesetSummaries(ids ...int64) string {
	entries := make([]string, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, fmt.Sprintf(`{"id":%d,"target":"tag","source_type":"Repository","source":"owner/repo","enforcement":"active"}`, id))
	}
	return "[" + strings.Join(entries, ",") + "]"
}

func countExact(paths []string, want string) int {
	count := 0
	for _, path := range paths {
		if path == want {
			count++
		}
	}
	return count
}

func countPrefix(paths []string, prefix string) int {
	count := 0
	for _, path := range paths {
		if strings.HasPrefix(path, prefix) {
			count++
		}
	}
	return count
}

// TestListWorkflowTagOptionsDoesNotRepeatRulesetOrTagRefWork is the failing
// regression: with the pre-fix per-candidate implementation, N candidates
// cause N ruleset list+detail passes and N /git/ref/tags GETs. The optimized
// catalog must issue one ruleset page and at most one detail per id.
func TestListWorkflowTagOptionsDoesNotRepeatRulesetOrTagRefWork(t *testing.T) {
	const candidateCount = 5
	objectSHAs := make([]string, 0, candidateCount)
	refs := make([]string, 0, candidateCount)
	for i := 0; i < candidateCount; i++ {
		objectSHA := hexSHA(rune('a' + i))
		objectSHAs = append(objectSHAs, objectSHA)
		refs = append(refs, workflowTagRef(fmt.Sprintf("workflow-v%d", i), objectSHA, "tag"))
	}

	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(refs...)}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	for i, objectSHA := range objectSHAs {
		h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, hexSHA(rune('1'+i)), true, "valid")
		h.contents[hexSHA(rune('1'+i))] = testWorkflowFileResponse(windowsWorkflowFilename)
	}
	h.use(t)

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if err != nil {
		t.Fatalf("ListWorkflowTagOptions() error = %v", err)
	}
	if len(options) != candidateCount {
		t.Fatalf("options = %#v, want %d verified tags", options, candidateCount)
	}
	paths := h.snapshotPaths()

	if got := countExact(paths, "GET /repos/owner/repo/rulesets"); got != 1 {
		t.Fatalf("ruleset list requests = %d, want exactly 1 (paths=%v)", got, paths)
	}
	if got := countPrefix(paths, "GET /repos/owner/repo/rulesets/"); got != 1 {
		t.Fatalf("ruleset detail requests = %d, want exactly 1 (paths=%v)", got, paths)
	}
	if got := h.tagRefGetCount(); got != 0 {
		t.Fatalf("/git/ref/tags GETs = %d, want none (listed object SHA must be used)", got)
	}
	for _, objectSHA := range objectSHAs {
		if got := countExact(paths, "GET /repos/owner/repo/git/tags/"+objectSHA); got != 1 {
			t.Fatalf("tag object GETs for %s = %d, want exactly 1", objectSHA, got)
		}
	}
}

// TestListWorkflowTagOptionsConcurrencyIsBoundedAndOrderIsStable proves the
// candidate window is capped and that completion order does not reorder
// successful labels.
func TestListWorkflowTagOptionsConcurrencyIsBoundedAndOrderIsStable(t *testing.T) {
	const candidateCount = 6
	refs := make([]string, 0, candidateCount)
	for i := 0; i < candidateCount; i++ {
		refs = append(refs, workflowTagRef(fmt.Sprintf("workflow-v%d", i), hexSHA(rune('a'+i)), "tag"))
	}

	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(refs...)}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	release := make(chan struct{})
	gated := maxWorkflowTagCandidateConcurrency
	for i := 0; i < candidateCount; i++ {
		objectSHA := hexSHA(rune('a' + i))
		h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, hexSHA(rune('1'+i)), true, "valid")
		h.contents[hexSHA(rune('1'+i))] = testWorkflowFileResponse(windowsWorkflowFilename)
		if i < gated {
			h.candidateGate[objectSHA] = release
		}
	}
	h.use(t)

	type result struct {
		options []WorkflowTagOption
		err     error
	}
	done := make(chan result, 1)
	go func() {
		options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
		done <- result{options: options, err: err}
	}()

	// Wait until the configured window is parked before releasing it; a smaller
	// window than expected would block here, and a larger one is caught by the
	// max-concurrency assertion below.
	for i := 0; i < gated; i++ {
		select {
		case <-h.parked:
		case got := <-done:
			t.Fatalf("catalogue finished before the concurrency window filled (err=%v)", got.err)
		}
	}
	if got := h.maxConcurrent(); got != gated {
		t.Fatalf("active candidate I/O at window fill = %d, want %d", got, gated)
	}
	close(release)
	got := <-done
	if got.err != nil {
		t.Fatalf("ListWorkflowTagOptions() error = %v", got.err)
	}
	if len(got.options) != candidateCount {
		t.Fatalf("options = %#v, want %d", got.options, candidateCount)
	}
	for i, option := range got.options {
		if want := fmt.Sprintf("workflow-v%d", i); option.Tag != want {
			t.Fatalf("options[%d] = %q, want stable listing order %q", i, option.Tag, want)
		}
	}
	if got := h.maxConcurrent(); got > maxWorkflowTagCandidateConcurrency {
		t.Fatalf("max concurrent candidate I/O = %d, want <= %d", got, maxWorkflowTagCandidateConcurrency)
	}
}

// TestListWorkflowTagOptionsAbortsOnFatalProviderFailureWithoutPartialResult
// proves a later candidate's transport failure aborts the entire catalogue
// rather than returning a partial success, even with an earlier policy
// rejection and an earlier eligible candidate.
func TestListWorkflowTagOptionsAbortsOnFatalProviderFailureWithoutPartialResult(t *testing.T) {
	goodObject := hexSHA('a')
	goodCommit := hexSHA('1')
	badObject := hexSHA('b')
	rejectedObject := hexSHA('c')

	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(
		workflowTagRef("workflow-v1", goodObject, "tag"),
		workflowTagRef("other-v1", rejectedObject, "tag"),
		workflowTagRef("workflow-v2", badObject, "tag"),
	)}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	h.tagObjects[goodObject] = workflowTagObjectBody(goodObject, goodCommit, true, "valid")
	h.contents[goodCommit] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.tagObjectErrors[badObject] = fmt.Errorf("provider exploded: %w", context.DeadlineExceeded)
	h.use(t)

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if options != nil {
		t.Fatalf("options = %#v, want no partial catalogue on fatal provider failure", options)
	}
	var transportErr *GithubTransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("ListWorkflowTagOptions() error = %T %v, want transport failure", err, err)
	}
}

// TestListWorkflowTagOptionsDoesNotMaskLaterProviderFailureBehindPolicy
// reproduces the review finding: an early applicable ruleset with a bypass
// policy rejection must not hide a later ruleset detail transport failure.
func TestListWorkflowTagOptionsDoesNotMaskLaterProviderFailureBehindPolicy(t *testing.T) {
	objectSHA := hexSHA('a')
	commitSHA := hexSHA('1')

	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(workflowTagRef("workflow-v1", objectSHA, "tag"))}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummaries(1, 2)}
	h.details[1] = strings.Replace(protectedRuleset(1, "workflow-"), `"bypass_actors":[]`, `"bypass_actors":[{"actor_id":1,"actor_type":"RepositoryRole","bypass_mode":"always"}]`, 1)
	h.detailErrors[2] = fmt.Errorf("ruleset detail transport: %w", context.DeadlineExceeded)
	h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, commitSHA, true, "valid")
	h.contents[commitSHA] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.use(t)

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if options != nil {
		t.Fatalf("options = %#v, want no partial catalogue when a provider failure is present", options)
	}
	var transportErr *GithubTransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("ListWorkflowTagOptions() error = %T %v, want the later transport failure, not the earlier policy rejection", err, err)
	}
}

// TestListWorkflowTagOptionsSkipsPolicyRejectedBeforeTagIO proves a locally
// policy-rejected candidate performs no tag-object or branch-collision I/O.
func TestListWorkflowTagOptionsSkipsPolicyRejectedBeforeTagIO(t *testing.T) {
	goodObject := hexSHA('a')
	goodCommit := hexSHA('1')
	rejectedObject := hexSHA('b')

	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(
		workflowTagRef("other-v1", rejectedObject, "tag"),
		workflowTagRef("workflow-v1", goodObject, "tag"),
	)}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	h.tagObjects[goodObject] = workflowTagObjectBody(goodObject, goodCommit, true, "valid")
	h.contents[goodCommit] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.use(t)

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if err != nil {
		t.Fatalf("ListWorkflowTagOptions() error = %v", err)
	}
	if len(options) != 1 || options[0].Tag != "workflow-v1" {
		t.Fatalf("options = %#v, want only workflow-v1", options)
	}
	paths := h.snapshotPaths()
	if got := countExact(paths, "GET /repos/owner/repo/git/tags/"+rejectedObject); got != 0 {
		t.Fatalf("policy-rejected candidate issued %d tag-object GET(s), want 0 (paths=%v)", got, paths)
	}
	if got := countExact(paths, "GET /repos/owner/repo/git/ref/heads/other-v1"); got != 0 {
		t.Fatalf("policy-rejected candidate issued %d branch-collision GET(s), want 0", got)
	}
	for _, path := range paths {
		if strings.Contains(path, "other-v1") {
			t.Fatalf("policy-rejected candidate performed I/O: %q (paths=%v)", path, paths)
		}
	}
}

// TestListWorkflowTagOptionsEmptyAndLightweightCatalogueSkipsRulesetIO proves
// that a catalogue with no annotated tag candidates performs no ruleset reads.
func TestListWorkflowTagOptionsEmptyAndLightweightCatalogueSkipsRulesetIO(t *testing.T) {
	for _, test := range []struct {
		name string
		refs string
	}{
		{name: "empty catalogue", refs: `[]`},
		{name: "lightweight only", refs: workflowTagRefList(workflowTagRef("lightweight", hexSHA('a'), "commit"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newWorkflowTagCatalogHarness()
			h.tagRefPages[1] = workflowTagCatalogPage{body: test.refs}
			h.use(t)

			options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
			if err != nil {
				t.Fatalf("ListWorkflowTagOptions() error = %v", err)
			}
			if len(options) != 0 {
				t.Fatalf("options = %#v, want empty", options)
			}
			paths := h.snapshotPaths()
			if got := countPrefix(paths, "GET /repos/owner/repo/rulesets"); got != 0 {
				t.Fatalf("ruleset requests = %d, want 0 for empty/lightweight catalogue (paths=%v)", got, paths)
			}
		})
	}
}

// TestListWorkflowTagOptionsRejectsDuplicateTags proves duplicate detection is
// preserved and deterministic after concurrent processing.
func TestListWorkflowTagOptionsRejectsDuplicateTags(t *testing.T) {
	objectSHA := hexSHA('a')
	commitSHA := hexSHA('1')
	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(
		workflowTagRef("workflow-v1", objectSHA, "tag"),
		workflowTagRef("workflow-v1", objectSHA, "tag"),
	)}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, commitSHA, true, "valid")
	h.contents[commitSHA] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.use(t)

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if options != nil {
		t.Fatalf("options = %#v, want nil on duplicate tag", options)
	}
	var contractErr *GithubContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("ListWorkflowTagOptions() error = %T %v, want duplicate contract error", err, err)
	}
}

// TestListWorkflowTagOptionsCancellationReturnsNoPartialResult proves parent
// cancellation is reported and produces no partial catalogue.
func TestListWorkflowTagOptionsCancellationReturnsNoPartialResult(t *testing.T) {
	objectSHA := hexSHA('a')
	commitSHA := hexSHA('1')
	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(workflowTagRef("workflow-v1", objectSHA, "tag"))}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, commitSHA, true, "valid")
	h.contents[commitSHA] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.use(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(ctx, &model.GithubBuildConfig{Repo: "owner/repo"})
	if options != nil || err == nil {
		t.Fatalf("ListWorkflowTagOptions(cancelled) = %#v, %v; want no partial result and an error", options, err)
	}
}

// TestListWorkflowTagOptionsPagedRulesetsFetchEachDetailOnce proves the
// snapshot still honors pagination and never repeats a detail across a page.
func TestListWorkflowTagOptionsPagedRulesetsFetchEachDetailOnce(t *testing.T) {
	objectSHA := hexSHA('a')
	commitSHA := hexSHA('1')
	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(workflowTagRef("workflow-v1", objectSHA, "tag"))}
	next := "https://api.github.com/repos/owner/repo/rulesets?per_page=100&page=2"
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1), next: `<` + next + `>; rel="next"`}
	h.rulesetPages[2] = workflowTagCatalogPage{body: workflowTagRulesetSummary(2)}
	h.details[1] = protectedRuleset(1, "release-")
	h.details[2] = protectedRuleset(2, "workflow-")
	h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, commitSHA, true, "valid")
	h.contents[commitSHA] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.use(t)

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if err != nil {
		t.Fatalf("ListWorkflowTagOptions() error = %v", err)
	}
	if len(options) != 1 || options[0].Tag != "workflow-v1" {
		t.Fatalf("options = %#v, want workflow-v1 protected by page-two ruleset", options)
	}
	paths := h.snapshotPaths()
	for _, id := range []string{"1", "2"} {
		if got := countExact(paths, "GET /repos/owner/repo/rulesets/"+id); got != 1 {
			t.Fatalf("ruleset %s detail requests = %d, want exactly 1 (paths=%v)", id, got, paths)
		}
	}
}

// TestListWorkflowTagOptionsRulesetPageLimitStillBounded preserves the
// existing pagination bound.
func TestListWorkflowTagOptionsRulesetPageLimitStillBounded(t *testing.T) {
	objectSHA := hexSHA('a')
	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(workflowTagRef("workflow-v1", objectSHA, "tag"))}
	next := "https://api.github.com/repos/owner/repo/rulesets?per_page=100&page=2"
	for page := 1; page <= maxRulesetPages; page++ {
		h.rulesetPages[page] = workflowTagCatalogPage{body: workflowTagRulesetSummary(int64(page)), next: `<` + next + `>; rel="next"`}
	}
	h.details[1] = protectedRuleset(1, "workflow-")
	h.use(t)

	_, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	var contractErr *GithubContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("ListWorkflowTagOptions() = %T %v, want bounded ruleset pagination error", err, err)
	}
}

// TestListWorkflowTagOptionsPreservesDisabledAndNonMatchingPayloads proves a
// disabled or non-matching ruleset with malformed payload does not newly reject
// an otherwise eligible tag. Applicable malformed bypass/visibility metadata
// still aborts the catalogue, matching the pre-fix stage semantics.
func TestListWorkflowTagOptionsPreservesDisabledAndNonMatchingPayloads(t *testing.T) {
	objectSHA := hexSHA('a')
	commitSHA := hexSHA('1')

	tests := []struct {
		name        string
		detail      string
		wantAbort   bool
		wantOptions int
	}{
		{
			name: "disabled ruleset with malformed bypass is ignored",
			detail: strings.Replace(
				strings.Replace(protectedRuleset(1, "workflow-"),
					`"enforcement":"active"`, `"enforcement":"disabled"`, 1),
				`"bypass_actors":[]`, `"bypass_actors":[{"actor_id":0,"actor_type":"User","bypass_mode":"always"}]`, 1),
			wantOptions: 0,
		},
		{
			name: "non-matching ruleset with malformed rules is ignored",
			detail: strings.Replace(
				protectedRuleset(1, "release-"),
				`{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}`,
				`{"type":"unknown_rule"}`, 1),
			wantOptions: 0,
		},
		{
			name: "applicable ruleset with malformed bypass aborts",
			detail: strings.Replace(protectedRuleset(1, "workflow-"),
				`"bypass_actors":[]`, `"bypass_actors":[{"actor_id":0,"actor_type":"User","bypass_mode":"always"}]`, 1),
			wantAbort: true,
		},
		{
			name: "applicable ruleset with missing visibility aborts",
			detail: strings.Replace(protectedRuleset(1, "workflow-"),
				`"bypass_actors":[],`, "", 1),
			wantAbort: true,
		},
		{
			name: "missing current-user bypass aborts",
			detail: strings.Replace(protectedRuleset(1, "workflow-"),
				`"current_user_can_bypass":"never",`, "", 1),
			wantAbort: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newWorkflowTagCatalogHarness()
			h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(workflowTagRef("workflow-v1", objectSHA, "tag"))}
			h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
			h.details[1] = test.detail
			h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, commitSHA, true, "valid")
			h.contents[commitSHA] = testWorkflowFileResponse(windowsWorkflowFilename)
			h.use(t)

			options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
			if test.wantAbort {
				if err == nil {
					t.Fatalf("options = %#v, want catalogue abort on applicable malformed ruleset", options)
				}
				var contractErr *GithubContractError
				if !errors.As(err, &contractErr) {
					t.Fatalf("ListWorkflowTagOptions() error = %T %v, want contract abort", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListWorkflowTagOptions() error = %v, want inapplicable payload ignored", err)
			}
			if len(options) != test.wantOptions {
				t.Fatalf("options = %#v, want %d", options, test.wantOptions)
			}
		})
	}
}

// TestListWorkflowTagOptionsInapplicableMalformedPayloadDoesNotRejectEligible
// proves the core applicability-gating invariant: a disabled or non-matching
// ruleset carrying malformed payload must not reject a tag that a separate
// applicable ruleset already protects.
func TestListWorkflowTagOptionsInapplicableMalformedPayloadDoesNotRejectEligible(t *testing.T) {
	objectSHA := hexSHA('a')
	commitSHA := hexSHA('1')

	tests := []struct {
		name         string
		inapplicable string
	}{
		{
			name: "disabled ruleset with malformed bypass",
			inapplicable: strings.Replace(
				strings.Replace(protectedRuleset(2, "release-"),
					`"enforcement":"active"`, `"enforcement":"disabled"`, 1),
				`"bypass_actors":[]`, `"bypass_actors":[{"actor_id":0,"actor_type":"User","bypass_mode":"always"}]`, 1),
		},
		{
			name: "non-matching ruleset with malformed rules",
			inapplicable: strings.Replace(
				protectedRuleset(2, "release-"),
				`{"type":"update","parameters":{"update_allows_fetch_and_merge":false}},{"type":"deletion"}`,
				`{"type":"unknown_rule"}`, 1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newWorkflowTagCatalogHarness()
			h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(workflowTagRef("workflow-v1", objectSHA, "tag"))}
			h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummaries(1, 2)}
			h.details[1] = protectedRuleset(1, "workflow-")
			h.details[2] = test.inapplicable
			h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, commitSHA, true, "valid")
			h.contents[commitSHA] = testWorkflowFileResponse(windowsWorkflowFilename)
			h.use(t)

			options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
			if err != nil {
				t.Fatalf("ListWorkflowTagOptions() error = %v, want inapplicable malformed payload ignored", err)
			}
			if len(options) != 1 || options[0].Tag != "workflow-v1" {
				t.Fatalf("options = %#v, want the eligible protected tag kept", options)
			}
		})
	}
}

// TestListWorkflowTagOptionsPreservesUnprotectedTagExclusion proves a tag not
// covered by any active ruleset is excluded while a protected sibling is kept.
func TestListWorkflowTagOptionsPreservesUnprotectedTagExclusion(t *testing.T) {
	protectedObject := hexSHA('a')
	unprotectedObject := hexSHA('b')
	commitA := hexSHA('1')
	commitB := hexSHA('2')
	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(
		workflowTagRef("workflow-v1", protectedObject, "tag"),
		workflowTagRef("release-v1", unprotectedObject, "tag"),
	)}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	h.tagObjects[protectedObject] = workflowTagObjectBody(protectedObject, commitA, true, "valid")
	h.tagObjects[unprotectedObject] = workflowTagObjectBody(unprotectedObject, commitB, true, "valid")
	h.contents[commitA] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.contents[commitB] = testWorkflowFileResponse(windowsWorkflowFilename)
	h.use(t)

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if err != nil {
		t.Fatalf("ListWorkflowTagOptions() error = %v", err)
	}
	if len(options) != 1 || options[0].Tag != "workflow-v1" {
		t.Fatalf("options = %#v, want only the protected tag", options)
	}
	// The unprotected tag is locally rejected before any tag-specific I/O.
	paths := h.snapshotPaths()
	if got := countExact(paths, "GET /repos/owner/repo/git/tags/"+unprotectedObject); got != 0 {
		t.Fatalf("unprotected candidate issued %d tag-object GET(s), want 0 (paths=%v)", got, paths)
	}
	if got := countExact(paths, "GET /repos/owner/repo/git/ref/heads/release-v1"); got != 0 {
		t.Fatalf("unprotected candidate issued %d branch-collision GET(s), want 0", got)
	}
}

// TestListWorkflowTagOptionsRefPageFailureAborts proves a later failure while
// listing tag-ref pages aborts the catalogue instead of returning partial data.
func TestListWorkflowTagOptionsRefPageFailureAborts(t *testing.T) {
	h := newWorkflowTagCatalogHarness()
	next := "https://api.github.com/repos/owner/repo/git/refs/tags?per_page=100&page=2"
	h.tagRefPages[1] = workflowTagCatalogPage{
		body: workflowTagRefList(workflowTagRef("workflow-v1", hexSHA('a'), "tag")),
		next: `<` + next + `>; rel="next"`,
	}
	h.tagRefPages[2] = workflowTagCatalogPage{body: ""}
	h.details[1] = protectedRuleset(1, "workflow-")
	h.use(t)
	// Force page two to fail with an API error via a custom wrapper.
	previous := ghClient.Transport
	ghClient.Transport = githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/git/refs/tags") && queryPage(req) == 2 {
			return githubResponse(http.StatusInternalServerError, `{"message":"page two failed"}`, nil), nil
		}
		return previous.RoundTrip(req)
	})
	t.Cleanup(func() { ghClient.Transport = previous })

	options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
	if options != nil {
		t.Fatalf("options = %#v, want nil on later tag-ref page failure", options)
	}
	var apiErr *GithubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ListWorkflowTagOptions() error = %T %v, want later-page API failure", err, err)
	}
}

// TestWorkflowTagPolicyIsReReadFreshPerBoundary proves the catalog snapshot is
// request-scoped only: each verification/boundary entry point re-reads provider
// rulesets instead of reusing a shared cache.
func TestWorkflowTagPolicyIsReReadFreshPerBoundary(t *testing.T) {
	tagObjectSHA := hexSHA('a')
	workflowSHA := hexSHA('1')
	var rulesetLists, rulesetDetails int
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/rulesets"):
			rulesetLists++
			return githubResponse(http.StatusOK, workflowTagRulesetSummary(1), nil), nil
		case strings.HasPrefix(req.URL.Path, "/repos/owner/repo/rulesets/"):
			rulesetDetails++
			return githubResponse(http.StatusOK, protectedRuleset(1, "workflow-"), nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/ref/tags/workflow-v1"):
			return githubResponse(http.StatusOK, `{"ref":"refs/tags/workflow-v1","object":{"sha":"`+tagObjectSHA+`","type":"tag"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/tags/"+tagObjectSHA):
			return githubResponse(http.StatusOK, `{"sha":"`+tagObjectSHA+`","object":{"sha":"`+workflowSHA+`","type":"commit"},"verification":{"verified":true,"reason":"valid"}}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/git/ref/heads/workflow-v1"):
			return githubResponse(http.StatusNotFound, `{"message":"no branch"}`, nil), nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected"}`, nil), nil
		}
	}))

	svc := &GithubBuildConfigService{}
	for i := 0; i < 2; i++ {
		if err := svc.verifyProtectedWorkflowTag(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"}, "workflow-v1"); err != nil {
			t.Fatalf("verifyProtectedWorkflowTag() call %d error = %v", i, err)
		}
	}
	if rulesetLists != 2 || rulesetDetails != 2 {
		t.Fatalf("ruleset reads list=%d detail=%d, want 2/2 (no snapshot reuse across calls)", rulesetLists, rulesetDetails)
	}
}

// TestListWorkflowTagOptionsPreservesBranchCollisionAndSignatureRejection
// proves the listed-object resolution keeps the branch-collision policy and
// signature/object/target failure semantics.
func TestListWorkflowTagOptionsPreservesBranchCollisionAndSignatureRejection(t *testing.T) {
	tests := []struct {
		name       string
		objectBody string
		branch     *workflowTagCatalogBranch
	}{
		{name: "verified tag with branch collision", objectBody: workflowTagObjectBody(hexSHA('a'), hexSHA('1'), true, "valid"),
			branch: &workflowTagCatalogBranch{status: http.StatusOK, body: `{"ref":"refs/heads/workflow-v1","object":{"sha":"` + hexSHA('d') + `","type":"commit"}}`}},
		{name: "unverified signature", objectBody: workflowTagObjectBody(hexSHA('a'), hexSHA('1'), false, "unsigned")},
		{name: "wrong verification reason", objectBody: workflowTagObjectBody(hexSHA('a'), hexSHA('1'), true, "unknown_signature_type")},
		{name: "nested object is not a commit", objectBody: `{"sha":"` + hexSHA('a') + `","object":{"sha":"` + hexSHA('1') + `","type":"tag"},"verification":{"verified":true,"reason":"valid"}}`},
		{name: "object SHA mismatch", objectBody: workflowTagObjectBody(hexSHA('b'), hexSHA('1'), true, "valid")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objectSHA := hexSHA('a')
			h := newWorkflowTagCatalogHarness()
			h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(workflowTagRef("workflow-v1", objectSHA, "tag"))}
			h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
			h.details[1] = protectedRuleset(1, "workflow-")
			h.tagObjects[objectSHA] = test.objectBody
			if test.branch != nil {
				h.branchRefs["workflow-v1"] = *test.branch
			}
			h.use(t)

			options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(context.Background(), &model.GithubBuildConfig{Repo: "owner/repo"})
			// A malformed tag object or signature is a non-abort candidate
			// failure: the candidate is skipped, the catalogue still succeeds.
			if err != nil {
				t.Fatalf("ListWorkflowTagOptions() error = %v, want candidate quietly excluded", err)
			}
			if len(options) != 0 {
				t.Fatalf("options = %#v, want candidate excluded", options)
			}
		})
	}
}

// TestListWorkflowTagOptionsAbortsOnBlockedAdmissionAfterCancel proves that a
// cancellation observed while admitting workers still joins all workers and
// reports no partial catalogue.
func TestListWorkflowTagOptionsAbortsOnBlockedAdmissionAfterCancel(t *testing.T) {
	const candidateCount = 8
	refs := make([]string, 0, candidateCount)
	for i := 0; i < candidateCount; i++ {
		refs = append(refs, workflowTagRef(fmt.Sprintf("workflow-v%d", i), hexSHA(rune('a'+i)), "tag"))
	}
	h := newWorkflowTagCatalogHarness()
	h.tagRefPages[1] = workflowTagCatalogPage{body: workflowTagRefList(refs...)}
	h.rulesetPages[1] = workflowTagCatalogPage{body: workflowTagRulesetSummary(1)}
	h.details[1] = protectedRuleset(1, "workflow-")
	// Gate every candidate so the pool stays full while the parent context is
	// cancelled; every worker must still be joined.
	release := make(chan struct{})
	for i := 0; i < candidateCount; i++ {
		objectSHA := hexSHA(rune('a' + i))
		h.tagObjects[objectSHA] = workflowTagObjectBody(objectSHA, hexSHA(rune('1'+i)), true, "valid")
		h.contents[hexSHA(rune('1'+i))] = testWorkflowFileResponse(windowsWorkflowFilename)
		h.candidateGate[objectSHA] = release
	}
	h.use(t)

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		options []WorkflowTagOption
		err     error
	}
	done := make(chan result, 1)
	go func() {
		options, err := (&GithubBuildConfigService{}).ListWorkflowTagOptions(ctx, &model.GithubBuildConfig{Repo: "owner/repo"})
		done <- result{options: options, err: err}
	}()
	// Let the pool fill, then cancel and release so workers can unwind.
	for i := 0; i < maxWorkflowTagCandidateConcurrency; i++ {
		<-h.parked
	}
	cancel()
	close(release)
	got := <-done
	if got.options != nil || got.err == nil {
		t.Fatalf("ListWorkflowTagOptions(cancelled) = %#v, %v; want no partial catalogue and an error", got.options, got.err)
	}
}
