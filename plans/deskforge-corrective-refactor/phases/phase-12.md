# Phase 12 — PR12: Docs/Tracker

**Status:** ✅ `verified-with-notes`
**Scope:** audit and reconcile external docs, the tracker, and PR10/PR11 implementation notes against the current canonical evidence. PR12 also records source Swagger annotations and regenerated `api/docs/api` plus `api/docs/admin` as API documentation metadata, produced with module-pinned `swag v1.16.3`. PR12 remains the documentation/tracker phase; the cumulative DeskForge candidate also includes the PR1–PR11 API/UI/Docker/server/offline-kit/tests implementation, while a separate RustDesk candidate includes active workflow/Android/portable/helper/tests.
**Boundary:** no live-provider, full-offline-build, signature/attestation, cross-DB, protected-ref, release-readiness, sovereignty, or Linux/Android support claim is allowed from local implementation notes or focused checks. Commits and PR publication are authorized by the user but remain pending final checks; this phase performs no publication.
**Provenance:** the DeskForge and RustDesk candidates are separate publication units. `rdgen` deletion and dirty RustDesk submodule content are excluded; this phase is not an isolated publication diff.

## Behavioral contract

Operators and maintainers should find one truthful account of supported capabilities, configuration, endpoints, build status, artifact handling, offline/release constraints, and known gaps. Documentation and tracker entries must describe what the evidence actually proves; they must not promote planned, local-only, historical, or unverified work to ready.

The normal user action remains typed configuration/selection or an operator verification of stored/frozen identity. Provider-derived run/artifact identity, source/workflow identity, hashes, and service digests remain system/provider-derived; secrets and raw internal values are not normal manual inputs.

## Audited-document checklist

- [x] `PLAN.md` — confirms the provider-only path, published client 1.4.8 / DeskForge
      schema 272, local corrective API schema 282 target, Windows-only admitted capability,
      Linux/Android gating, current ports, DFP1 payload, and absence of live-provider/
      clean-build proof.
- [x] `README.md` — records Linux/Android as in development and labels the published
      client 1.4.8 / DeskForge schema 272 against the local API schema 282 target, key path,
      port 21119, fork ownership,
      and no support/readiness claim.
- [x] `CHANGELOG.md` — preserves the current provider-only/no-live-run boundary,
      with PR10/PR11 `in-progress` and PR12 `verified-with-notes` for the documentation/tracker phase.
- [x] `BUGS.md` — preserves the capability gate and pending live evidence;
      historical closure text remains historical and is not current run/support proof.
- [x] `CONTRIBUTING.md` and `AGENTS.md` — agree on workflow ownership, frozen/reference
      boundaries, secret handling, and no-publication constraints.
- [x] `github-build/README.md` — identifies the configured RustDesk fork as the
      sole executable workflow source and keeps Linux/Android capability-gated.
- [x] `offline-kit/README.md` and `offline-kit/FORK-PROCEDURE.md` — define
      fail-closed verify-only behavior and explicitly deny full-build/signature proof.
- [x] `api/README.md` — confirms redacted handoff metadata and that the service
      digest is integrity metadata, not a signature or release publication;
      documents provider-derived verified annotated tags, active protected-tag
      policy, explicit update/deletion protections, no bypass actors,
      tag/branch collision rejection, schema 280–282 additive fields, legacy
      metadata-only secret compatibility, pre-payload revalidation, safe errors,
      and the live-provider/atomic-dispatch limitation.
- [x] Canonical `phase-10.md` and `phase-11.md` — confirm focused local evidence
      and the exact PR10/PR11 blockers used by this reconciliation.
- [x] Canonical implementation evidence records protected verified annotated tags, active
      update/deletion protections, no bypass actors, tag/branch collision rejection, approval
      migration through `DatabaseVersion 282`, pre-payload revalidation, and bounded safe errors.
- [x] Source Swagger annotations and regenerated `api/docs/api` plus `api/docs/admin` use
      module-pinned `swag v1.16.3`; JSON/YAML/`docs.go` outputs are deterministic, and
      route/auth/schema/redaction parity checks pass.

## Reconciliation result

The canonical plan records PR10 and PR11 as `in-progress` and carries their local
evidence and blockers forward. PR12 includes source Swagger annotations and regenerated
`api/docs/api` plus `api/docs/admin` via module-pinned `swag v1.16.3`; JSON/YAML/`docs.go`
outputs are deterministic, and route/auth/schema/redaction parity checks pass. Full
`GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and
`GOWORK=off go test -race ./...` pass after test-only cache diagnostics, opt-in Redis test
changes, and test-only lock-test race fixes; production lock code is unchanged. The API
build, UI build, deterministic Swagger regeneration/parity checks, Compose checks, and
`git diff --check` pass. Swagger metadata remains sparse for legacy operations, a
low-usability issue that does not change the verified status or create a correctness claim.
The audited DeskForge documents and directly copied RustDesk README workflow links are
aligned with that record.

Recorded verification commands/checks:

- `GOWORK=off go vet ./...`
- `GOWORK=off go test ./...`
- `GOWORK=off go test -race ./...`
- API build: `GOWORK=off go build -o release/apimain cmd/apimain.go`
- UI build: `npm run build`
- Module-pinned Swagger regeneration with `swag v1.16.3`, followed by deterministic
  JSON/YAML/`docs.go` parity and route/auth/schema/redaction checks
- Compose configuration checks
- `git diff --check`

All listed checks pass. The Redis integration and benchmark path remains untested when
`DESKFORGE_TEST_REDIS_ADDR` is unset.
Published HEAD/version
claims, dispatch response requirements, PR11 static-only evidence, manual/historical
builder boundaries, the 2026-08-12 full Go vet/test/race result with its opt-in Redis
boundary, and license-inventory non-claims are explicit. The full commands passed after
test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes;
production lock code is unchanged. Redis integration tests and benchmarks run only when
`DESKFORGE_TEST_REDIS_ADDR` is configured; no live Redis run is recorded. The reconciliation
corrects stale next/status and ownership wording without adding unsupported capability or
readiness language. Local pre-payload revalidation and
safe-error behavior do not resolve GitHub's provider boundary: `workflow_dispatch` still
requires a branch/tag selector without atomic SHA binding, so a theoretical selector move
between final validation and POST can expose DFP1.

## Exact remaining gates

- [ ] **Real offline-kit verification:** independently trusted expected manifest and
      hashes; clean, non-shallow source and bundle; recursive submodule provenance;
      complete vendor/license evidence; and repeat no-mutation verification. Current
      real-kit verification fails closed on secret-bearing artifacts, the legacy
      manifest, missing engine, empty printer digest, and incomplete license evidence.
- [ ] **Protected workflow identity:** the local API now requires a provider-derived
      verified annotated tag, active protected-tag evidence with explicit update and
      deletion protections, no bypass actors, no tag/branch label collision, and exact
      workflow contents at resolved `WorkflowSHA`, distinct from GitHub's required
      dispatch branch/tag selector; approval/migration fields are at local schema 282 and
      the dispatch primitive revalidates policy/SHA immediately before DFP1 creation. Live
      provider proof remains unverified.
- [ ] **Atomic dispatch boundary:** the pending → building identity write and local final
      policy revalidation are atomic/ordered locally, but GitHub `workflow_dispatch` requires
      a branch/tag selector and provides no atomic SHA binding. A theoretical selector move
      between final validation and POST can expose DFP1 to another revision; no durable
      outbox/distributed lease or provider-side immutable/no-bypass proof closes this window.
      Secret-bearing production dispatch/live readiness remains gated until such proof or
      atomic API support is actually evidenced.
- [ ] **Published submodule provenance:** clean, published RustDesk `hbb_common` state.
- [ ] **Live execution:** provider/runner/workflow runs for any claimed platform, with
      exact run/artifact identity, matching `head_sha`, and provider-backed streamed
      download completion. No live provider run is recorded.
- [ ] **Linux/Android capability:** APK/package output, installation/runtime delivery,
      and Android signing/debug evidence. Linux/Android remain capability-gated, not
      supported.
- [ ] **Build environment:** required toolchains/native validators plus clean-worktree
      and repeat-build evidence.
- [ ] **Cross-database evidence:** MySQL and PostgreSQL migration/read-write checks;
      SQLite-only checks do not close this gate.
- [ ] **Authenticity evidence:** cryptographic signatures, attestations, and trusted
      verification roots; SHA-256 manifests/digests are integrity evidence only.
- [ ] **Complete offline build:** a network-denied full build, including Windows
      toolchain/vcpkg capability and repeat-build evidence. Verify-only/metadata
      checks do not satisfy this gate.
- [ ] **Retention/publication boundary:** explicit completed-output retention/deletion
      policy; no automatic TTL, release-retention, tag, asset upload, or release claim.

PR12 is `verified-with-notes` as a documentation/tracker phase and is not a release
step. Swagger source/output parity does not close the preserved PR10/PR11
external-provider/live-execution, real offline-kit, protected workflow-ref, signature/
attestation, cross-DB, or publication/release blockers. PR10 and PR11 remain
`in-progress` and must not be marked complete by documentation reconciliation alone.
