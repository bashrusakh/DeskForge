# DeskForge Corrective Refactor

**Date:** 2026-08-10
**Scope:** cumulative PR1–PR12 DeskForge candidate publication package plus a separately
scoped RustDesk candidate. The DeskForge candidate includes API, UI, Docker, server,
offline-kit, Swagger, documentation, and tests. The separate RustDesk candidate includes
active workflows, Android, portable, helper, and tests. `rdgen` deletion and dirty
RustDesk submodule content are excluded from both candidates.
**Publication boundary:** follow-up commit `642f36a` (`workflow: harden GitHub protection
contract`) was pushed to `refactor/deskforge-corrective-pr`, and PR #59 remains open.
GitHub run `31777799368` passed Admin UI, Go, Rust, and CodeQL/analyzer checks; CodeRabbit
was skipped due to the repository's 191-file limit. This publication status does not close
PR10/PR11 or their preserved evidence gates.
**Current next action:** keep
PR10 and PR11 `in-progress`, not verified, and PR12 `verified-with-notes`. This plan
does not make live-provider, full-offline-build, signature/attestation, cross-DB,
protected-ref, release-readiness, or sovereignty claims.
**Candidate provenance:** the DeskForge and RustDesk candidates are intentionally
separate publication units; this plan records their boundaries without treating staged
files, local implementation checks, or dirty submodule state as publication evidence.

## Phase index

| Phase | User PR scope | Status | Evidence / next dependency |
|---|---|---|---|
| PR1 | BuildSpec/settings | ✅ `verified-with-notes` | [`phase-1.md`](phases/phase-1.md); PR2 endpoint contract |
| PR2 | network/public endpoints | ✅ `verified-with-notes` | [`phase-2.md`](phases/phase-2.md); manual/docs/dev-compose gaps remain |
| PR3 | exact run_id/REST errors | ✅ `verified-with-notes` | [`phase-3.md`](phases/phase-3.md); live provider/poll integration remains |
| PR4 | provenance | ✅ `verified-with-notes` | [`phase-4.md`](phases/phase-4.md); no distributed lease/outbox, no cross-DB/live evidence |
| PR5 | immutable version/build-ref | ✅ `verified-with-notes` | [`phase-5.md`](phases/phase-5.md); workflow YAML/config is PR6 boundary |
| PR6 | workflow ownership/config | ✅ `verified-with-notes` | [`phase-6.md`](phases/phase-6.md); live provider/YAML execution remains deferred |
| PR7 | streaming artifact | ✅ `verified-with-notes` | [`phase-7.md`](phases/phase-7.md); focused evidence recorded, bounded gaps remain |
| PR8 | secret persistence/security hardening | ✅ `verified-with-notes` | [`phase-8.md`](phases/phase-8.md); current-key and storage limitations remain explicit |
| PR9 | Go/Rust reproducibility + SHA-pinned actions | ✅ `verified-with-notes` | [`phase-9.md`](phases/phase-9.md); live provider/clean-build proof remains absent |
| PR10 | offline-kit/release evidence | 🟡 `in-progress` | [`phase-10.md`](phases/phase-10.md); Packages A–C implementation/focused checks pass, offline-kit and release gates remain blocked |
| PR11 | Linux/Android capability | 🟡 `in-progress` | [`phase-11.md`](phases/phase-11.md); local implementation evidence recorded, live capability evidence blocked |
| PR12 | docs/tracker | ✅ `verified-with-notes` | [`phase-12.md`](phases/phase-12.md); reconciliation included in the cumulative candidate, with PR10/PR11 evidence gates retained |

## Preserved completed facts

### PR4 — `verified-with-notes`

- `DatabaseVersion 273`; `AutoMigrate` adds provenance columns while retaining legacy readability.
- Immutable snapshot: provider, repository, workflow, ref, run ID, run URL, HTML URL, expected artifact name, artifact ID, and provider source SHA when available.
- Dispatch requires GitHub's exact run-details response; no recent-run/list inference.
- One-shot pending → building guard; artifact ID and source SHA are guarded first-write operations tied to the stored run.
- Poll/resume reconstructs identity from the snapshot after global GitHub configuration changes; only the current credential is mutable.
- Legacy/incomplete provenance fails closed; the poll ownership guard is process-local, not a distributed lease.
- Focused SQLite, fake-transport, race, and targeted vet checks passed; `go.work.sum` was cleaned.
- No MySQL/PostgreSQL/live provider/Redis integration or benchmark evidence; no distributed lease or outbox for post-dispatch DB failure.

### PR5 — `verified-with-notes`

- `DatabaseVersion 274` fields for immutable version/build identity are covered by SQLite migration/backward-compatibility checks.
- Catalog is keyed by `GithubBuildConfig.Repo`, follows bounded release pagination, filters `offline-assets-<semver>`, and verifies the matching `refs/tags/<version>` object SHA in that repository.
- `CustomBuild` stores display `Version` plus write-once `BuildRef`, `SourceTag`, `AssetsRelease`, and `AssetsReleaseID`; legacy/partial rows fail closed.
- Dispatch overwrites the temporary workflow `version` input from the resolved immutable identity; no user-supplied ref or second workflow-ref selector is accepted on the normal path.
- GitHub's `workflow_dispatch` REST contract requires the request-body `ref` to be a branch or tag selector, not a raw commit SHA. `WorkflowRef` is persisted and dispatched as that selector; `WorkflowSHA` is retained separately for exact workflow-file readiness and run `head_sha` verification.
- Focused SQLite/fake-transport/race and targeted vet checks, `gofmt`, `git diff --check`, and `npm run build` passed locally; UI build warnings were existing Rollup warnings.
- MySQL/PostgreSQL, live GitHub, Redis integration/benchmark coverage, and workflow YAML behavior remain unverified/out of scope.

### PR6 — `verified-with-notes`

- User-editable GitHub configuration is limited to repository, PAT, and payload key; legacy workflow/ref columns remain unread for migration safety.
- Fixed platform workflow mapping and immutable catalog identity own new dispatches; existing builds poll stored provenance with only the current token.
- Provider readiness is checked before custom-build persistence; production API has no implicit file-queue fallback or PAT-sync endpoint.
- DeskForge duplicate platform workflows and the draft sync workflow were deleted. The rdgen bridge copy remains because vendored rdgen generator workflows reference it; the rustdesk fork bridge remains the executable source of truth.
- Focused Go tests, race, targeted vet, UI build, formatting, diff-check, and deletion/source-marker checks passed; live GitHub credentials/runs and real workflow execution remain deferred to PR7/PR11.

### PR7 — `verified-with-notes`

- GitHub's artifact HTTP response body is streamed with a bounded reader into a server-owned temporary `.part` file, validated, and atomically renamed; the user contract specifies this GitHub HTTP body flow, so a runner callback endpoint is explicitly not implemented and any reviewer callback request is out of scope.
- The stream and final download remain bound to the exact immutable run, artifact ID, artifact name, and stored provenance; no sole-artifact, path, or inferred-run fallback is accepted.
- Limits are separate and fail closed: compressed provider body `256 MiB`; decompressed ZIP entries `4096`, per-file `512 MiB`, aggregate `1 GiB`, and compression ratio `1000:1`; public output archive `4096` files, `512 MiB` per source file, `1 GiB` source aggregate, and `512 MiB` generated archive.
- Temporary archives, `.part` files, staging directories, and failed publication outputs have bounded cleanup/retry and stale-file sweeps; active downloads/builds are protected from sweeps.
- Publication computes a service-owned SHA-256 digest bound to immutable build/run/artifact identity and canonical output contents. Publication proof is write-once, atomic at the file boundary, and idempotent on retry; `done` requires the marker, digest, and current proof.
- Lifecycle statuses and guards cover `pending`, `building`, `downloading`, `extracting`, `done`, and `failed`; mismatched, duplicate, expired, interrupted, partial, stale, or persistence-failed paths remain non-success and recoverable where specified.
- Production capability is intentionally Windows-only and fails closed for Linux/Android until PR11. The UI polls only active lifecycle states, stops polling at terminal states, and uses mounted/request-generation guards; the completed action is exposed only after lifecycle checks.
- Focused Go tests, `-race`, targeted vet, and `npm run build` passed locally. On 2026-08-12, the full `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` checks also passed after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; the production lock implementation is unchanged. The API build, UI build, deterministic Swagger regeneration/parity checks, Compose checks, and `git diff --check` passed. Redis integration tests and benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is configured, and no live Redis run is recorded. Live provider/runner execution, MySQL/PostgreSQL, crash recovery and distributed lease/outbox behavior, and browser/manual gaps remain unverified.

### PR8 — `verified-with-notes`

- `SECRET_ENCRYPTION_KEY` is the canonical API at-rest key. New non-empty secret writes fail closed when the current key is unavailable; no plaintext fallback is accepted.
- Legacy plaintext rows remain readable for migration safety, and saving them again encrypts them with the current key. Malformed ciphertext prefixes are rejected rather than treated as legacy plaintext.
- Safe DTOs and explicit raw `json` tags keep secret fields out of normal responses and raw JSON paths. Generated payload keys use `base64.RawURLEncoding`; the one-time generated-key display is intentional and is not a persistent secret readback.
- Docker passes `SECRET_ENCRYPTION_KEY` through to the API runtime without logging or transforming the value. Focused secret-persistence tests, race checks, targeted vet, and `npm run build` passed locally.
- Limitations remain explicit: key rotation is unsupported and the current key is required; evidence is SQLite-only with no MySQL/PostgreSQL coverage; Redis integration and benchmark coverage are opt-in through `DESKFORGE_TEST_REDIS_ADDR`, and no live Redis run is recorded.

### PR9 — `verified-with-notes`

- Go 1.25.0 and `GOTOOLCHAIN=local` are pinned across the recorded build paths; module
  download/build inputs use the tracked Go module metadata and existing swag version.
- Rust dependency resolution uses the committed lockfile and a pinned Rust Docker toolchain;
  active DeskForge workflow actions are pinned to full commit SHAs with scoped permissions.
  Fork-owned client workflows remain the executable source; frozen/reference workflow copies
  are excluded from the active ownership boundary.
- Full Go, API, UI, Swagger, Compose, and diff verification passes locally. On 2026-08-12, `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` passed after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; production lock code is unchanged. The API build and UI build passed; Swagger was regenerated deterministically with module-pinned `swag v1.16.3`, and JSON/YAML/`docs.go` plus route/auth/schema/redaction parity checks passed. Compose checks and `git diff --check` passed.
- Full Go vet/tests and the full race suite pass with `GOWORK=off` after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; production lock code is unchanged. Specifically, `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` pass. The API build, UI build, deterministic Swagger regeneration/parity, Compose, and `git diff --check` checks pass. Redis integration tests and benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is configured; no live Redis run is recorded. Default
  OpenSSL/FindBin compatibility, native `actionlint`/`shellcheck`/`pwsh`, live provider/runner
  execution, clean-worktree/repeat-build evidence, cross-DB evidence, Windows/Linux/Android
  execution, and release publication remain unverified.

## Current workflow approval and dispatch evidence — 2026-08-11

- The local approval boundary accepts only a provider-derived annotated tag whose verification
  reports `verified=true` with the accepted reason, whose nested object is a commit, and whose
  label does not collide with a branch. Active provider protection evidence requires the matching
  tag policy, explicit update and deletion protections, and an explicitly empty bypass-actor list;
  conflicting or ambiguous protection fails closed. This is focused fake-transport/provider-
  contract evidence, not live GitHub evidence.
- Approval persists the selected tag, provider-resolved commit SHA, provider-verification state,
  and approval state. Additive migration/model checks cover the approval and provider-policy
  fields through `DatabaseVersion 282`; legacy compatibility remains readable where required.
- Build preparation revalidates the approved selector and resolved SHA before persistence. The
  dispatch primitive revalidates mapped workflow contents and the tag policy immediately before
  DFP1 encryption, rejects a changed execution SHA, and only then creates the secret-bearing
  payload. Typed approval, validation, capability, provider, transport, contract, and artifact
  errors map to bounded safe HTTP/user messages and diagnostics without exposing selectors,
  provider bodies/URLs, credentials, or encrypted payloads.
- The provider boundary remains unresolved: GitHub `workflow_dispatch` requires a branch/tag
  selector and does not provide atomic SHA binding. A theoretical selector move between final
  validation and the dispatch `POST` can therefore expose DFP1 to a different workflow revision.
  Secret-bearing production dispatch and live readiness remain gated until provider-side
  immutable/no-bypass deployment proof or atomic API support is actually evidenced. Local
  revalidation does not close that provider-side window.

## PR10 blueprint — `in-progress`

This section records the PR10 implementation/evidence package within the cumulative
DeskForge candidate. Packages A–C implementation and focused checks are present, while
real-kit, release, provider, and other explicitly listed evidence gates remain open.
Final publication must stay within the candidate boundary and leave secrets and
unproven capability claims out of the normal operator path.

### Package A — offline-kit freeze and verify-only hardening

**Evidence files:** `offline-kit/freeze.sh`, `offline-kit/versions.env`,
`offline-kit/README.md`, `offline-kit/FORK-PROCEDURE.md`,
`offline-kit/artifacts/MANIFEST.txt`, `offline-kit/artifacts/sha256sums`,
`offline-kit/artifacts/printer_sha256sums`,
`offline-kit/artifacts/rustdesk-1.4.8.bundle`,
`offline-kit/artifacts/vendor-1.4.8.tar.gz`, and the frozen source tree's
`.gitmodules`, `Cargo.lock`, `.cargo/config.vendor.toml`, and license files.
`win-builder/README.md`, `setup.ps1`, and `agent.ps1` remain fallback evidence,
not an active release path.

**Blueprint:**

- Separate the online `freeze` operation from an idempotent, no-network
  `verify-only` operation. Verify-only must not download, mutate a checkout,
  refresh a branch, or silently regenerate the observed manifest.
- Require commit/source equality: `RUSTDESK_COMMIT`, the checked-out source
  `HEAD`, the declared ref/tag, the bundle's contained revision, recursive
  submodule commits, and the source tree used for vendor/config evidence must
  agree. A dirty or shallow source checkout fails closed.
- Verify every expected SHA-256 from a trusted, reviewable expected manifest;
  `MANIFEST.txt` is observed evidence until its expected values have an
  independent source. A present file, URL, or first-seen hash is not proof.
- Treat `versions.env` URLs, especially mutable release channels such as
  `engine/.../releases/download/main`, as retrieval locations only. Mutable
  URLs require an independently pinned expected hash and provenance; TOFU is
  not an acceptable release or sovereignty boundary.
- Record vendor completeness, source and dependency licenses, submodule
  provenance, manifest generation time, sizes, and hash results. Missing
  licenses, unexplained vendored content, or a manifest that cannot be
  reproduced is a stop condition.

**Focused implementation checks:** Strict offline fixtures and freeze/verify-only
stage checks pass locally, including fail-closed handling for secret-bearing
artifact presence and staged manifest/input conditions.

**Real-kit verification boundary:** A real offline-kit verify/freeze run fails
closed on secret-bearing artifact presence, the legacy manifest, missing engine,
empty printer digest, and incomplete license evidence. `git bundle verify`, source
`rev-parse`/clean-tree/recursive submodule checks, independently trusted
expected-manifest checks, and repeat no-mutation verification remain required
evidence and are not closed by fixture/stage tests.

**Stop points and risks:** stop on any network attempt, missing expected hash,
mutable URL without independent pin, source/ref/submodule mismatch, shallow or
dirty bundle source, missing vendor/license evidence, partial download, or
manifest rewrite. Risks include the ignored multi-gigabyte artifact store,
resumable-download state being mistaken for verified state, hash-only
integrity being mistaken for authenticity, and Windows vcpkg/binary-cache
requirements remaining outside Linux verification.

### Package B — operator handoff and export verification

**Evidence files:** the immutable build identity in `api/model/custom_build.go`
and `api/service/custom_build.go`, provenance/identity checks in the existing
`api/service/build_provenance_test.go`, artifact flow in
`api/http/controller/admin/custom_build.go` and
`api/http/controller/admin/github_build_poll_test.go`, plus the configured fork's
protected workflow files (not a local workflow copy). Handoff output is an
operator-facing export/manifest, not a secret-bearing API response or release
publication.

**Blueprint:** the handoff must carry exact source repository, selector/ref,
source commit/tree SHA and submodule commits; workflow repository/path,
protected selector, workflow SHA/file identity and action pins; provider/run
ID, run URLs and run `head_sha`; artifact ID/name; release tag/ID and exact
asset names/digests; and the final output file list, sizes, per-file hashes,
service-owned output digest, export path, and verification timestamp/result.
It must be generated from stored immutable provenance and verified again at
export time. PATs, payload keys, generated secrets, URLs containing credentials,
and raw internal secret fields must never be included. Export verification must
re-read the exact source/workflow/run/artifact/release/assets/output identity,
compare the checksum manifest and output digest, and fail closed on missing,
changed, inferred, or provider-only values. It must not upload, publish, or
claim a release.

**Checks:** Package B implementation and focused tests pass locally: v2
producer/API manifests, including the private `custom_.txt` declaration;
deterministic redacted handoff and archive hashes; exact identity, checksum,
digest, sidecar, and public `custom_.txt` redaction assertions; and offline
read-back verification without provider access. No live provider evidence is
inferred.

**Stop points and risks:** stop on incomplete legacy provenance, changed
workflow contents, missing run/artifact/release identity, digest mismatch,
secret leakage, non-deterministic export, or any attempted publication. Risks
include stale provider URLs, metadata drift after an asset is replaced, an
operator treating an unsigned checksum as a signature, and exports becoming a
new secret exfiltration path.

### Package C — active artifact retention and exact export boundary

**Evidence files:** `api/service/published_output.go`,
`api/service/build_output_cleanup.go`, their existing focused tests,
`api/model/custom_build.go`, and the existing download/poll controller paths.
The canonical output root and numeric build directories remain service-owned;
`offline-kit/artifacts/` is not an API output directory.

**Blueprint:** define retention separately for active downloads (`.part`/`.zip`),
extraction/staging/recovery paths, failed outputs, and completed canonical
outputs. Cleanup may remove only an exact, stale, service-owned temporary shape;
active builds are protected; completed output is retained until its explicit
retention/export policy is satisfied. Exact-file/manifest verification must
require regular non-symlink files, bounded count/size/aggregate, canonical
relative names, deterministic name ordering, and the required Windows
executable. The service-owned digest must bind immutable build/run/artifact and
release/assets identity to exact file names, sizes, and contents. An export
sidecar/metadata record must carry the digest and manifest identity and be
verified against the output before handoff; it is evidence, not a signature or
release asset.

Preserve the current Windows-only production capability and fail closed for
Linux/Android until PR11 provides real end-to-end evidence. Do not turn the
frozen `win-builder/` fallback into an active claim, and do not describe any
local output or handoff as a live release.

**Checks:** Package C implementation and focused tests pass locally:
TOCTOU-safe snapshot/archive cleanup, exact-manifest and digest/publication-proof
checks, retry/partial-output handling, active-versus-terminal retention,
retention-days and `if-no-files-found: error` workflow contracts, mutation/missing-
file and symlink/path/name/size-limit rejection, export-sidecar verification, and
Windows-only capability checks. Cross-DB and live-provider checks remain gates
below.

**Stop points and risks:** stop on ambiguous ownership, unsafe cleanup scope,
manifest/digest mismatch, sidecar mismatch, output mutation after proof, missing
Windows executable, or an attempt to expose Linux/Android or release status.
Risks include premature deletion, stale recovery trees, digest replay across
builds, metadata detached from exported bytes, and confusing local artifact
retention with release retention.

### Explicit PR10 gates and non-claims

The following remain explicit gates and must be recorded as blocked/unverified
rather than worked around:

- protected/immutable workflow-ref deployment and exact workflow contents at the
  resolved SHA;
- RustDesk `hbb_common` submodule publication/clean provenance;
- live provider, runner, and workflow execution;
- cryptographic signatures/attestations (SHA-256 evidence is not a signature);
- MySQL and PostgreSQL cross-DB migration/read-write evidence; and
- a complete network-denied full build, including Windows toolchain/vcpkg
  capability and repeat-build evidence.

PR10 may verify local files and prepare a redacted operator handoff, but it may
not create or publish a tag/release, upload assets, use secrets, claim a live
provider result, or mark sovereignty/release readiness from repository presence
alone.

## PR10 implementation and verification boundary — 2026-08-10

- Packages A–C implementation and focused checks pass locally: strict offline
  fixtures/stage checks; v2 producer/API manifests including private `custom_.txt`
  declaration; deterministic handoff/archive hashes; TOCTOU-safe snapshot cleanup;
  retention-days/`if-no-files-found: error`; and public `custom_.txt` redaction.
- PR10 remains `in-progress`, not `verified-with-notes`. A real offline-kit
  verify/freeze run fails closed on secret-bearing artifact presence, the legacy
  manifest, missing engine, empty printer digest, and incomplete license evidence.
- Completed-output retention/deletion remains an explicit manual operator policy;
  no completed-output TTL or release-retention policy is claimed. Active fork
  workflows are the executable evidence boundary; frozen offline-kit and
  `win-builder/` material is manual/historical-only evidence, not a production fallback.
- Live provider/runner execution, signatures/attestations, MySQL/PostgreSQL
  cross-DB evidence, protected immutable workflow-ref deployment, clean published
  RustDesk `hbb_common` submodule provenance, and a complete network-denied full
  build remain unverified gates.

## PR11 implementation and verification boundary — 2026-08-10

PR11 is `in-progress`, not verified. The following local static implementation evidence
is recorded without converting fixtures, source inspection, workflow manifests, or
focused tests into live Linux/Android/Windows/provider or release claims:

- Bridge staging and hash-restore behavior is implemented and locally evidenced.
- The DFP1-only workflow/helper boundary is preserved; the helper remains untracked at
  the remote HEAD.
- Manifest v2 behavior is implemented and locally evidenced.
- Android app-id validation and the `applicationId` contract are recorded and covered
  locally.
- Android `custom_.txt` runtime-path handling and the fail-closed packaging check are
  implemented and locally evidenced.
- Linux deb/RPM `custom_.txt` ordering is implemented and locally evidenced.
- Portable deterministic traversal/timestamp tests are present and pass locally.
- Capability gating and the UI disabled state preserve fail-closed exposure for
  unsupported or unproven platforms.

Bridge/helper source, Android app-id/`applicationId`, custom_.txt runtime-path checks,
local package assertions, and workflow manifests are static evidence only. No APK/package
creation, installation/runtime delivery, Android signing/debug, or normal provider
operation is evidenced; deleted local workflow copies and frozen `rdgen` workflows are
not proof.

Explicit blockers and non-claims:

- Follow-up commit `642f36a` (`workflow: harden GitHub protection contract`) was pushed to
  `refactor/deskforge-corrective-pr`, and PR #59 remains open; the helper is untracked at
  the remote HEAD.
- No live Linux, Android, Windows, or provider runs have been performed.
- No APK/package/install/runtime evidence exists.
- No protected workflow-ref proof exists.
- No clean or repeat-build evidence exists.
- Required toolchains and native validators are unavailable.
- RustDesk `hbb_common` remains dirty and unpublished.
- Android signing/debug evidence is unavailable.
- Cross-DB evidence and cryptographic signature evidence are unavailable.

## Current local verification boundary

- PR1–PR9 and the security-hardening work are recorded as `verified-with-notes`; PR10 and
  PR11 remain `in-progress`, while PR12 is `verified-with-notes` for its documentation/
  tracker reconciliation. This does not convert deferred evidence into a release,
  sovereignty, or platform-support claim.
- Full Go, API, UI, Swagger, Compose, and diff verification passes locally. On 2026-08-12,
  `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and
  `GOWORK=off go test -race ./...` passed after test-only cache diagnostics, opt-in Redis
  test changes, and test-only lock-test race fixes; production lock code is unchanged. The
  API build and UI build passed; Swagger was regenerated deterministically with module-pinned
  `swag v1.16.3`, and JSON/YAML/`docs.go` plus route/auth/schema/redaction parity checks
  passed. Compose checks and `git diff --check` passed. Swagger metadata remains sparse for
  legacy operations as a low-usability issue.
- Full Go vet/tests and the full race suite pass with `GOWORK=off` after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; production lock code is unchanged. Specifically, `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` pass. The API build, UI build, deterministic Swagger regeneration/parity, Compose, and `git diff --check` checks pass. Redis integration tests and benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is configured; no live Redis run is recorded.
- Default OpenSSL/FindBin compatibility, native `actionlint`/`shellcheck`/`pwsh`, live provider/runner execution, clean and repeat builds, cross-DB coverage, and Windows/Linux/Android execution remain unverified.
- Workflow dispatch retains the required branch/tag selector; exact workflow contents are checked at the resolved `WorkflowSHA`, and run `head_sha` must match. GitHub provides no atomic SHA binding, so a theoretical selector move between final validation and `POST` can expose DFP1; protected immutable/no-bypass deployment proof or atomic provider API support remains a gate for secret-bearing production dispatch/live readiness.
- RustDesk `hbb_common` timestamp work remains dirty and unpublished submodule state; no clean published reproducibility evidence is inferred from it.
- PR11 local evidence covers bridge staging/hash restore, DFP1-only workflow/helper preservation, manifest v2, Android app-id/applicationId and `custom_.txt` fail-closed packaging contracts, Linux deb/RPM ordering, deterministic portable traversal/timestamp tests, and the capability gate/UI disabled state.
- PR11 remains blocked by unpublished changes/helper state, absent live Linux/Android/Windows/provider runs, absent APK/package/install/runtime proof, absent protected-ref proof, absent clean/repeat builds, unavailable toolchains/native validators, Android signing/debug, dirty unpublished `hbb_common`, and absent cross-DB/signature evidence.

## PR12 docs/tracker reconciliation — `verified-with-notes`

PR12 records and applies the completed documentation reconciliation boundary. The listed
DeskForge documents, source Swagger annotations, regenerated API documentation, and
RustDesk README workflow links were reconciled in this documentation-metadata change
from the current PR10/PR11 evidence. File presence, local focused checks, or
implementation notes are not release, sovereignty, or platform-support evidence.

### Audited-document checklist

- [x] PR12 includes source Swagger annotations and regenerated `api/docs/api` plus
      `api/docs/admin` using module-pinned `swag v1.16.3`; JSON, YAML, and `docs.go`
      outputs are deterministic, and route/auth/schema/redaction parity checks pass.
      Swagger metadata remains sparse for legacy operations, recorded as a low-usability
      issue rather than a correctness or release-readiness blocker.
- [x] `PLAN.md` — provider-only path, published client 1.4.8 / DeskForge schema 272,
      local corrective API schema 282 target, ports, Windows-only admitted capability,
      active/frozen boundaries, DFP1 payload, and no live-provider/clean-build proof
      are recorded.
- [x] `README.md` — published/local version labels, key path, ports, combined-work
      license, fork ownership, and Linux/Android gating are aligned without readiness claims.
- [x] `CHANGELOG.md` — the current-state note uses the PR10/PR11/PR12 statuses and
      preserves the provider-only/no-live-run boundary.
- [x] `BUGS.md` — the tracker preserves the Linux/Android capability gate and
      pending live evidence; historical closure text must remain labeled and must
      not be treated as a current run or support result.
- [x] `CONTRIBUTING.md` and `AGENTS.md` — active workflow ownership, frozen/reference
      boundaries, development Compose command, and no-publication constraints agree.
- [x] `github-build/README.md` — the configured fork is the sole executable source;
      DFP1, retention/handoff boundaries, and Linux/Android gating are explicit.
- [x] `offline-kit/README.md` and `offline-kit/FORK-PROCEDURE.md` — frozen/local,
      fail-closed verify-only behavior and missing-engine/manifest/license/signature/
      network-denied gaps are explicit; no version fallback is claimed.
- [x] `api/README.md` — handoff metadata is redacted and the service digest is
      metadata/integrity evidence, not a signature or release publication.
- [x] `plans/deskforge-corrective-refactor/phases/phase-10.md` and `phase-11.md` —
      authoritative implementation notes confirm focused local evidence and the
      remaining PR10/PR11 blockers.

### Exact remaining gates

- [ ] PR10 real offline-kit verification: trusted expected manifest/hashes,
      clean non-shallow source and bundle, recursive submodule provenance,
      complete vendor/license evidence, and repeat no-mutation verification;
      the current real-kit check fails closed on secret-bearing artifacts, the
      legacy manifest, missing engine, empty printer digest, and incomplete
      license evidence.
- [ ] Protected immutable workflow-ref deployment and exact workflow contents at
      the resolved `WorkflowSHA`, distinct from GitHub's dispatch branch/tag
      selector.
- [ ] Clean, published RustDesk `hbb_common` submodule provenance.
- [ ] Live provider/runner/workflow runs for the claimed platforms, with exact
      run/artifact identity, matching `head_sha`, and provider-backed streamed
      download completion; no live provider run is recorded.
- [ ] Linux/Android APK/package, installation/runtime, and Android signing/debug
      evidence; until then Linux/Android remain capability-gated, not supported.
- [ ] Required toolchains/native validators plus clean and repeat-build evidence.
- [ ] MySQL and PostgreSQL migration/read-write evidence; SQLite-only checks do
      not close the cross-DB gate.
- [ ] Cryptographic signatures, attestations, and trusted verification roots;
      SHA-256 manifests/digests are integrity evidence only.
- [ ] A complete network-denied full build, including Windows toolchain/vcpkg
      capability and repeat-build evidence; verify-only and metadata checks do
      not satisfy this gate.
- [ ] Explicit completed-output retention/deletion policy; no automatic TTL,
      release-retention, release, tag, or asset-publication claim is made.

PR12 cannot mark release readiness, sovereignty, Linux/Android support, live
provider execution, signatures, cross-DB coverage, or a complete offline build.
The Swagger source/output verification also does not close the preserved PR10/PR11
external-provider/live-execution, real offline-kit, protected workflow-ref, signature/
attestation, cross-DB, or publication/release blockers. PR10 and PR11 remain
`in-progress` until their phase-specific evidence closes.

## Restoration boundary

This directory is the durable plan memory for the intended two-repository publication
package. PR1–PR9 evidence and limitations are preserved above; PR10 and PR11 remain
`in-progress`, while PR12 is `verified-with-notes`. The DeskForge candidate contains
API/UI/Docker/server/offline-kit/Swagger/docs/tests; the separate RustDesk candidate
contains active workflow/Android/portable/helper/tests. `rdgen` deletion and dirty
submodule content are excluded. Follow-up commit `642f36a` (`workflow: harden GitHub
protection contract`) was pushed to `refactor/deskforge-corrective-pr`, and PR #59 remains
open. GitHub run `31777799368` passed Admin UI, Go, Rust, and CodeQL/analyzer checks;
CodeRabbit was skipped due to the repository's 191-file limit. PR10 and PR11 remain
`in-progress`; their live-provider, TOCTOU, cross-DB, offline, and other preserved
limitations are unchanged.
