# Phase 5 — PR5: immutable version/build-ref

**Status:** ✅ `verified-with-notes` (no commit/push)
**Boundary:** preserve the existing typed version-selection workflow; do not turn provider identity into a normal-user raw/manual editor.

## Behavioral contract

The user naturally chooses a known available build version and starts a build. The selected version is user-authored through the existing capability-selection UI; the effective provider build-ref and run identity are system/provider-derived. The implementation must preserve that distinction: typed validation and known capabilities provide the normal input, while internal provider fields are persisted and used by the workflow without exposing raw payload editing.

## Objective

Extend the PR4 immutable snapshot so the selected `version` and provider `build-ref` are captured at the one-shot dispatch boundary and remain authoritative for the lifetime of the build. A mutable global GitHub configuration may provide the current credential, but it must not redirect an existing build to a different version, repository, workflow, or ref.

## Acceptance (exact)

All criteria are required:

1. **Atomic capture:** validated version and selected build-ref are persisted with the one-shot dispatch identity at the pending → building transition.
2. **Write-once identity:** edit, retry, poll, resume, callback, or a second dispatch cannot overwrite version or build-ref after capture.
3. **Typed normal path:** the workflow receives version/ref from the canonical typed build selection; raw `custom_txt`, raw provider payload, or manual internal identity is not accepted as the normal user-facing input.
4. **Snapshot-bound execution:** status, artifact resolution/download, and restart resume use the stored version/ref and the PR4 stored repository/workflow/run/artifact identity, even after global config mutation; only the current token may come from global config.
5. **Legacy safety:** a legacy or partial row missing immutable version/build-ref fails closed with an explicit terminal reason and never falls back to mutable global identity.
6. **Concurrency:** only one complete immutable snapshot can win for a pending build; later or concurrent writes cannot replace it.
7. **Migration safety:** existing rows remain readable through `AutoMigrate`; migration failure prevents recording the new database version.
8. **Evidence:** focused SQLite, fake-transport, race, and targeted vet checks pass; no live provider is required for this acceptance.

## Gates (exact)

- Inspect the current branch/worktree before implementation and keep the change limited to PR5; do not include unrelated files or behavior.
- Reuse the existing user-facing known-version selection pattern and typed validation. Do not add a raw/manual version or ref editor.
- Add focused SQLite tests for new fields, legacy reads, one-shot guards, and migration/version-recording failure behavior.
- Add fake-transport tests that assert the exact dispatch version/ref and assert that polling/resume remain on the stored snapshot after global configuration mutation.
- Add tests for legacy/partial fail-closed behavior and concurrent/duplicate write rejection.
- Run the narrow focused package tests, focused `-race`, and targeted `go vet`; record exact commands and outcomes.
- Keep the explicit PR4 limitations: no MySQL/Postgres or live provider evidence. Redis integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`; do not treat the broad Go checks as live Redis evidence.
- Do not claim a distributed lease or outbox for a post-dispatch database failure; those remain out of scope.
- Finish with `git diff --check`; do not commit or push.

## Exit record

PR5 is `verified-with-notes` because every acceptance item and gate above is evidenced below. PR4's `verified-with-notes` status was not treated as evidence for the version/build-ref slice.

## Exit evidence

- `CustomBuild` migration fields are covered by SQLite `AutoMigrate` tests; `DatabaseVersion` is **274**. The migration/version-recording failure test remains fail-closed.
- `version_catalog_test.go` covers configured-repo pagination, source-tag SHA resolution, cache invalidation, missing/mismatched source tags, and concurrent refresh singleflight. No live GitHub transport or credentials are used.
- `build_provenance_test.go` covers identity persistence, invalid identity/no-row behavior, write-once dispatch identity, immutable edit rejection, and stored-ref behavior after configuration mutation. Admin polling tests cover stored provenance and legacy fail-closed resume.
- `GOWORK=off go test ./cmd ./model ./service ./http/controller/admin`, the matching `-race` command, targeted `go vet`, `gofmt`, `git diff --check`, and `npm run build` passed locally. The npm build emitted existing Rollup pure-comment/chunk-size warnings only.
- MySQL/PostgreSQL, live GitHub, Redis integration/benchmark coverage, and workflow YAML behavior remain unverified/out of scope. The broad Go checks pass with `GOWORK=off`; Redis integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`. The existing workflow `version` input is intentionally retained only as the PR6 boundary and is derived from the stored resolved identity.
