# Phase 4 — PR4: provenance

**Status:** ✅ `verified-with-notes`
**Date recorded:** 2026-08-09
**Scope:** provider identity persistence and restart-safe polling; no claim of distributed delivery.

## Delivered contract

A GitHub build stores one immutable provider-derived snapshot:

- `github_provider`
- `github_repo`
- `github_workflow`
- `github_ref`
- `github_run_id`
- `github_run_url`
- `github_html_url`
- `github_artifact_name`
- `github_artifact_id`
- `github_source_sha`

Secrets and dispatch payloads are not part of the snapshot. `github_run_id` comes from GitHub's exact dispatch response; the code does not infer a run by listing recent runs. The artifact name is selected by platform, the artifact ID is resolved from the exact stored run and exact name, and the source SHA is taken from the run-status response when available.

## Schema and write guards

- **DatabaseVersion:** `273`.
- **Migration:** `AutoMigrate` adds the provenance fields and keeps legacy rows readable.
- **Dispatch guard:** one atomic pending → building write stores the complete run/repository/workflow/ref/URL identity and rejects a second dispatch or partial pre-existing identity.
- **Artifact guard:** artifact ID is write-once, positive, tied to the stored run ID and current building state.
- **SHA guard:** source SHA is validated and write-once for the stored run; a later poll cannot replace it.
- **Progress guard:** asynchronous progress updates do not carry or overwrite immutable run identity.

## Polling and restart evidence

Polling builds a request configuration from the stored provenance snapshot. After global repository/workflow/ref configuration is mutated, requests still target the original stored repository, workflow, ref, run, and artifact. Only the current token is read from the mutable global configuration. Startup resume runs after migration and applies the same snapshot validation before starting a poll.

Rows with a run ID but missing or inconsistent immutable provenance fail closed to `failed` with an explicit log reason. A process-local ownership guard prevents duplicate pollers for one build and releases its claim when polling exits. It is intentionally not a distributed lease.

## Verification recorded

- Focused SQLite migration/backward-compatibility checks: **pass**.
- Fake-transport exact dispatch/status/artifact checks, including global-config mutation: **pass**.
- Focused race checks: **pass**.
- Targeted `go vet`: **pass**.
- `go.work.sum`: cleaned.

## Notes and limitations

- No MySQL or PostgreSQL pass is recorded.
- No live GitHub/provider pass is recorded.
- Historical note: this earlier phase record did not include the broad Go checks. The 2026-08-11 verification records `GOWORK=off go vet ./...` and `GOWORK=off go test ./...` passing after test-only cache diagnostics and opt-in Redis test changes; Redis integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`, with no live Redis run recorded.
- PR4 does not provide a distributed lease or outbox for the failure window where provider dispatch succeeds but the local database write fails.
- The process-local poll guard does not coordinate multiple API processes.

PR5 is the next phase and must address immutable version/build-ref under the exact acceptance and gates in [`phase-5.md`](phase-5.md).
