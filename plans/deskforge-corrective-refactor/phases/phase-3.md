# Phase 3 — PR3: exact run_id/REST errors

**Status:** ✅ `verified-with-notes`
**Scope:** GitHub REST transport, exact workflow dispatch response, typed error policy, redaction, artifact identity, and retry classification. The exact dispatch response is a project/provider contract under test; no normal GitHub operation or live-provider claim is made.

## Behavioral contract

The operator selects a known build capability and starts it. The service encrypts the typed payload and asks GitHub to dispatch the configured workflow. GitHub supplies the run identity; the service must not manufacture it by listing recent runs or accept it from user input. Provider errors are typed and safe to surface; secrets and encrypted payloads never appear in logs.

## Delivered contract

- **Contract under test:** dispatch sends `return_run_details: true` and requires the exact HTTP `200` run-details response containing `workflow_run_id`, `run_url`, and `html_url`.
- Standard `204` is intentionally unsupported because it provides no accepted exact run correlation. A `204`, missing/zero run ID, malformed JSON, or unsafe URL is a contract error; there is no “latest run” guessing or list-and-hope fallback. Stored `GithubRunId=0` remains the legacy/no-provider-run sentinel, not a current provider identity.
- Shared REST handling validates repository paths before transport, bounds response bodies, redacts bearer tokens/PATs/payload keys/encrypted payloads, and classifies transport/API failures as retryable or terminal.
- `401`, ordinary `403`, `404`, `409`, `410`, and `422` remain terminal; rate-limited `403`, `408`, `425`, `429`, and `5xx` are retryable under the recorded policy.
- Run status returns typed status/conclusion and optional exact `head_sha`.
- Artifact resolution is tied to the exact stored run and requested artifact name; there is no sole-artifact inference. Download uses the resolved artifact ID.
- Secret public-key and secret-write endpoints use the same expected-status and bounded/typed error handling.

## Exact implementation/evidence record

Relevant files checked:

- `api/service/github_build_config.go`
- `api/service/github_build_config_test.go`
- `api/service/custom_build.go`
- `api/http/controller/admin/custom_build.go`
- `api/http/controller/admin/github_build_config.go`
- `api/model/github_build_config.go`

Focused fake-transport coverage exercises the project/provider dispatch response shape and `200` requirement, no list polling/guessing, malformed/zero/missing identity rejection, URL safety, status classification, bounded and redacted bodies (including truncation boundaries), unsafe repository rejection, exact status/head SHA handling, exact artifact-name selection, artifact-ID download, secret endpoint status handling, and retry-vs-terminal poll policy. These tests do not prove normal GitHub operation.

Commands recorded:

```text
GOWORK=off go test ./cmd ./model ./service ./http/controller/admin
GOWORK=off go test -race ./cmd ./model ./service ./http/controller/admin
go vet ./cmd ./model ./service ./http/controller/admin
git diff --check
```

These are local fake-transport checks. No live GitHub/provider dispatch, real credentials, or live poll/download integration was performed. Historical note: this earlier phase record did not include the broad Go checks. The 2026-08-11 verification records `GOWORK=off go vet ./...` and `GOWORK=off go test ./...` passing after test-only cache diagnostics and opt-in Redis test changes; Redis integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`, with no live Redis run recorded.

## Gates and remaining dependencies

- Keep the exact provider contract; do not restore `204` success, list-based run inference, or guessed artifact selection.
- Preserve typed errors and redaction at every GitHub REST caller.
- PR4 owns durable provenance and restart-safe binding of this exact identity.
- PR5 owns immutable version/build-ref binding before dispatch.
- Live provider dispatch, real poll progression, and CI artifact delivery remain explicit follow-up evidence, not completed by PR3.
