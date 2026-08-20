# DeskForge Corrective Refactor — Todo

**Plan:** `plans/deskforge-corrective-refactor/plan.md`
**Boundary:** cumulative PR1–PR12 DeskForge candidate plus a local-only Phase 13 PR59
review-remediation record and a separate RustDesk candidate.
The DeskForge candidate includes API/UI/Docker/server/offline-kit/Swagger/docs/tests;
the RustDesk candidate separately includes active workflow/Android/portable/helper/tests.
`rdgen` deletion and dirty submodule content are excluded. Follow-up commit `68e7f30`
(`workflow: harden ruleset failure handling`) was pushed to
`refactor/deskforge-corrective-pr`, and PR #59 remains open. GitHub run `31785951197` passed
Go, Rust, Admin UI, and CodeQL/analyzer checks; CodeRabbit was skipped because the PR has 191
files. This does not mark the PR ready, close PR10/PR11, or close their preserved evidence gates.
No live-provider, full-offline-build, signature/attestation, cross-DB, protected-ref,
release-readiness, or sovereignty claim is made.

## Status by original PR order

- [x] **PR1 — verified-with-notes**
  - [x] Typed `BuildSpec` owns settings and canonical persisted `custom_json`.
  - [x] Platform gate covers `windows`, `linux`, and `android`; unsupported values fail closed.
  - [x] Preset create/update uses canonicalization and preserves empty/zero compatibility.
  - [x] Exact files and local commands are recorded in [`phase-1.md`](phases/phase-1.md).

- [x] **PR2 — verified-with-notes**
  - [x] Endpoint literals, including explicit ports, are preserved; no `stripPort` service transformation.
  - [x] UI/Docker/public endpoint and `21119` checks are recorded.
  - [x] Manual browser, docs, and dev-compose gaps remain explicit.
  - [x] Evidence is recorded in [`phase-2.md`](phases/phase-2.md).

- [x] **PR3 — verified-with-notes**
  - [x] Dispatch contract under test requires `return_run_details=true` and an exact HTTP `200` response with `workflow_run_id`, `run_url`, and `html_url`; standard `204` is intentionally unsupported because it cannot provide accepted exact run correlation.
  - [x] No recent-run/list guessing; typed errors, redaction, bounded bodies, artifact identity, and retry policy are covered by local fake-transport checks only. Normal GitHub operation is not verified.
  - [x] Live provider and poll/download integration remain unverified.
  - [x] Evidence is recorded in [`phase-3.md`](phases/phase-3.md).

- [x] **PR4 — verified-with-notes**
  - [x] Record `DatabaseVersion 273`, provenance fields/guards, `AutoMigrate`, SQLite and race evidence.
  - [x] Record exact dispatch identity, artifact/source write-once guards, snapshot-bound polling/resume, and legacy fail-closed behavior.
  - [x] Preserve limits: no MySQL/Postgres/live provider/full suite; process-local guard only; no distributed lease/outbox.
  - [x] Evidence is preserved in [`phase-4.md`](phases/phase-4.md).

- [x] **PR5 — verified-with-notes**
  - [x] Record `DatabaseVersion 274`, catalog pagination/source-tag SHA checks, and immutable version/build-ref persistence.
  - [x] Record catalog/repository identity, pagination, source tag/commit/ref, focused checks, and legacy fail-closed behavior.
  - [x] Preserve the temporary workflow `version` boundary and current hardcoded UI/config fallback findings for PR6.
  - [x] Preserve exact local commands/evidence and all unverified coverage limits.
  - [x] Evidence is preserved in [`phase-5.md`](phases/phase-5.md).

- [x] **PR6 — verified-with-notes: workflow ownership/config simplification**
  - [x] Fixed application workflow mapping owns filenames; catalog identity owns repository/version/build ref; version propagation is overwritten from immutable identity.
  - [x] User config is Repo/PAT/PayloadKey only; legacy workflow/ref columns remain unread for migration safety.
  - [x] Provider readiness precedes build-row persistence and production API has no file-queue fallback or PAT-sync machinery.
  - [x] DeskForge duplicate platform workflows and draft sync workflow deleted; rdgen bridge retained because vendored generator workflows still reference it.
  - [ ] Live GitHub dispatch/poll and real workflow execution remain deferred to later capability evidence.

- [x] **PR7 — verified-with-notes: streaming artifact**
  - [x] GitHub-body bounded temp-file streaming is recorded; no runner callback is part of the contract or implementation.
  - [x] Exact run/artifact binding, compressed/decompressed/public archive limits, and fail-closed mismatch/partial/duplicate/expired handling are recorded.
  - [x] Temp/staging cleanup, retry/sweep protection, digest/publication proof, atomic/idempotent output, and lifecycle statuses/guards are recorded.
  - [x] Windows-only fail-closed capability and UI polling/lifecycle checks are recorded; Linux/Android remains PR11.
  - [x] Focused Go tests, race, targeted vet, and UI build passed locally.
  - [x] `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` pass after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; production lock code is unchanged. Redis integration/benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is configured, with no live Redis run. Live provider/runner, cross-DB, crash/distributed lease/outbox, and browser/manual evidence remain unverified.
  - [x] Reviewer callback request is explicitly out of scope because the user contract specifies GitHub HTTP body flow.

- [x] **PR8 — verified-with-notes: secret persistence/security hardening**
  - [x] `SECRET_ENCRYPTION_KEY` is canonical for API secret-at-rest encryption; new non-empty secret writes fail closed when the current key is unavailable.
  - [x] Legacy plaintext rows remain readable and are encrypted on re-save; malformed ciphertext prefixes are rejected.
  - [x] Safe DTOs with explicit raw `json` tags preserve redaction boundaries; generated payload keys use `base64.RawURLEncoding`, and one-time generated-key display is intentional.
  - [x] Docker pass-through, focused secret-persistence tests, race checks, targeted vet, and `npm run build` were verified.
  - [x] Limitations are recorded: key rotation is unsupported/current key is required; evidence is SQLite-only with no MySQL/PostgreSQL coverage; Redis integration/benchmarks are opt-in through `DESKFORGE_TEST_REDIS_ADDR`, with no live Redis run recorded.

- [x] **PR9 — verified-with-notes: Go/Rust reproducibility + SHA-pinned actions**
   - [x] Inventory recorded Go/Rust inputs, toolchains, lockfiles, vendored dependencies, generators, and artifact identity checks.
   - [x] Active workflow actions are pinned to immutable SHAs with ownership/permission review; fork-only workflow ownership is preserved without claiming live execution.
   - [x] Full Go, API, UI, Swagger, Compose, and diff verification passes locally; the exact full Go vet/test/race commands and the test-only cache/Redis/lock-test fixes are recorded below.
   - [x] Full `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` pass after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; production lock code is unchanged. Redis integration/benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is configured; no live Redis run is recorded. Live provider/runner execution, clean-worktree/repeat-build evidence, cross-DB coverage, default OpenSSL/FindBin compatibility, native `actionlint`/`shellcheck`/`pwsh`, and Windows/Linux/Android execution remain unverified.
   - [x] Active client workflows fail closed without authenticated DFP1 payloads and bind payload source identity to the configured fork.
   - [x] Nested interrupted build-output download cleanup and direct-model canonical custom-field persistence guards are covered locally.
    - [x] Local workflow-ref hardening requires provider-reported verified annotated tags,
      an accepted verification reason, a matching protected-tag pattern, explicit update and
      deletion protections, no bypass actors, and tag/branch collision rejection before approval
      and again before DFP1 dispatch; live fork/provider execution remains unverified.
    - [x] Approval persists the provider-resolved tag SHA and policy state through historical
      migration evidence at `DatabaseVersion 282`; legacy compatibility remains preserved.
    - [x] Build preparation and the dispatch primitive revalidate the selector, protected policy,
      workflow contents, and resolved SHA immediately before secret-bearing DFP1 payload creation;
      typed provider/approval/validation/contract errors expose only bounded safe messages and
      diagnostics, not credentials, selectors, response bodies, or encrypted payloads.
    - [ ] GitHub `workflow_dispatch` still accepts only a branch/tag selector and offers no atomic
      SHA binding. A theoretical selector move between final validation and `POST` can expose
      DFP1; production secret-bearing dispatch/live readiness stays gated until provider-side
      immutable/no-bypass deployment proof or atomic API support is evidenced.
   - [ ] RustDesk `hbb_common` timestamp work remains dirty and unpublished submodule state.

  - [ ] **PR10 — in-progress: offline-kit/release evidence**
   - [x] **Package A implementation:** strict offline fixtures and freeze/verify-only stage checks pass locally.
   - [ ] **Package A real-kit verification:** verify/freeze fails closed on secret-bearing artifact presence, the legacy manifest, missing engine, empty printer digest, and incomplete license evidence; trusted manifest, source/submodule, and repeat no-mutation evidence remain open.
   - [x] **Package B implementation/tests:** v2 producer/API manifests, including the private `custom_.txt` declaration, deterministic redacted handoff/archive hashes, exact identity/checksum/digest assertions, and public `custom_.txt` redaction pass focused checks.
   - [x] **Package C implementation/tests:** TOCTOU-safe snapshot/archive cleanup, exact output/manifest and sidecar verification, retention-days and `if-no-files-found: error` contracts, and Windows-only capability checks pass focused tests.
   - [ ] Completed-output retention/deletion remains an explicit manual operator policy; no completed-output TTL or release-retention policy is claimed.
   - [x] Active/frozen workflow evidence boundaries are recorded: active fork workflows are executable evidence, while frozen offline-kit and `win-builder/` material remains manual/historical-only evidence.
   - [ ] Protected workflow ref, clean published RustDesk `hbb_common` submodule, live provider/runner, signatures/attestations, MySQL/PostgreSQL cross-DB, and complete network-denied full-build evidence remain blocked/unverified.
   - [x] Concrete files, checks, stop points, risks, and all PR10 gates are recorded in [`phase-10.md`](phases/phase-10.md).
    - [x] Frozen/manual historical semantics are explicit; no tag, release, asset upload, or sovereignty claim is implied by this phase.


 - [ ] **PR11 — in-progress: Linux/Android capability (not verified)**
  - [x] Local bridge staging/hash-restore implementation evidence is recorded.
  - [x] DFP1-only workflow/helper preservation is recorded; the helper remains untracked at the remote HEAD.
  - [x] Manifest v2 implementation evidence is recorded.
  - [x] Android app-id validation and the `applicationId` contract are recorded.
  - [x] Android `custom_.txt` runtime-path handling and the fail-closed packaging check are recorded.
  - [x] Linux deb/RPM `custom_.txt` ordering is recorded.
  - [x] Portable deterministic traversal/timestamp tests and the capability gate/UI disabled state are recorded as local evidence.
  - [x] Workflow manifests, bridge/helper source, Android app-ID/`applicationId`, custom_.txt runtime-path checks, and package assertions are labeled static only; deleted local copies and frozen `rdgen` workflows are not proof.
  - [ ] Follow-up commit `68e7f30` was pushed and PR #59 remains open; the helper is untracked at the remote HEAD. This does not close PR11's evidence gates.
  - [ ] No live Linux, Android, Windows, or provider runs have been performed.
  - [ ] No APK/package/install/runtime evidence exists.
  - [ ] No protected workflow-ref proof exists.
  - [ ] No clean or repeat-build evidence exists.
  - [ ] Required toolchains and native validators are unavailable.
  - [ ] RustDesk `hbb_common` remains dirty and unpublished.
  - [ ] Android signing/debug evidence is unavailable.
  - [ ] Cross-DB and cryptographic signature evidence are unavailable.
  - [ ] Re-expose platform choices only after green end-to-end capability evidence and the PR7 streaming boundary.

 - [x] **PR12 — verified-with-notes: docs/tracker reconciliation (not a release step)**
   - [x] Audit the external docs, tracker, and PR10/PR11 implementation notes listed in [`phase-12.md`](phases/phase-12.md).
   - [x] Reconcile the canonical record to current PR10/PR11 evidence: focused local implementation checks are recorded, while live/provider/platform/release evidence is not.
    - [x] Update the separately scoped external docs/tracker sources from current PR10/PR11 evidence; preserve the audited non-claims and do not imply release, sovereignty, Linux/Android support, live provider runs, signatures, cross-DB coverage, or a full offline build.
    - [x] Every audited source uses the same PR10/PR11 status and remaining-gate wording; PR12 closure preserves the documentation/tracker status and does not close those evidence gates. The cumulative candidate also retains the PR1–PR11 implementation scope.
  - [x] Source Swagger annotations and regenerated `api/docs/api` plus `api/docs/admin` use module-pinned `swag v1.16.3`; JSON/YAML/`docs.go` outputs are deterministic, and route/auth/schema/redaction parity checks pass.
  - [x] Swagger metadata remains sparse for legacy operations; this is recorded as a low-usability issue, not a correctness, support, or publication claim.

  - [ ] **Phase 13 — in-progress: DeskForge-local PR59 review remediation**
    - [x] Record current local `DatabaseVersion 283`: composite index
      `idx_custom_presets_user_id_name` on `(user_id, name)` with duplicate-group
      preflight before `AutoMigrate`, without auto-selecting a row or deleting data.
    - [x] Preserve `DatabaseVersion 282` as the earlier workflow-approval migration,
      not the current local schema target.
    - [x] Record the exact provider-owned
      `# deskforge-workflow-identity-guard: v1` requirement at the resolved workflow
      SHA before approval, preparation, or secret-bearing dispatch; legacy unguarded
      tags fail closed without claiming atomic selector/SHA binding or live readiness.
    - [x] Record local `d67b6e7` enforcement of a stored v2 producer-manifest and exact
      canonical-output proof across publication, recovery, detail, download, reuse, and
      handoff boundaries, without claiming live provider artifact proof.
    - [x] Record [RustDesk PR #7](https://github.com/bashrusakh/rustdesk/pull/7) as
      merged into `rustqs/workflows` at
      `ced31ae07f69c20119b88212b10d2eb2df651c97`, containing prior source commit
      `6ef1cd7fe`.
    - [x] Record PR #59 as open/dirty remotely, with local-only `2c38c87` merge
      resolution and unpushed `da42521`/`9a1ee5e`/`d67b6e7` remediation commits.
    - [ ] `workflow-v1.2.0` does not exist. Before production dispatch, obtain a newly
      signed provider-verified immutable protected tag and perform live
      reapproval/reverification. MySQL/PostgreSQL and live-provider evidence remain
      separate unverified gates. See
      [`phase-13.md`](phases/phase-13.md).

### PR12 audited-doc and remaining-gate checklist

- [x] Audited `PLAN.md`, `README.md`, `CHANGELOG.md`, `BUGS.md`,
      `CONTRIBUTING.md`, `AGENTS.md`, `github-build/README.md`,
      `offline-kit/README.md`, `offline-kit/FORK-PROCEDURE.md`, `api/README.md`,
      and canonical `phase-10.md`/`phase-11.md`.
- [x] Recorded current PR10/PR11 local evidence without promoting it to release,
      sovereignty, Linux/Android support, live provider execution, signatures,
      cross-DB coverage, or a full offline build.
- [ ] Close real offline-kit verification and trusted manifest/source/submodule/
      license/repeatability evidence.
- [ ] Close protected immutable workflow-ref, clean published `hbb_common`, live
      provider/runner, exact artifact/download, and required toolchain/native-
      validator evidence.
- [ ] Close Linux/Android package/install/runtime and Android signing/debug proof,
      clean/repeat builds, MySQL/PostgreSQL coverage, signatures/attestations, and
      the complete network-denied full-build gate.
- [ ] Keep completed-output retention policy explicit and keep release/tag/asset
      publication outside this plan restoration.

## Current local verification boundary

- PR1–PR9 and the security-hardening work are `verified-with-notes`; PR10 and PR11 remain
  `in-progress`, PR12 is `verified-with-notes` for documentation/tracker reconciliation,
  and Phase 13 records DeskForge-local PR59 remediation.
- Packages A–C implementation and focused checks pass: strict offline fixtures/stage checks; v2 producer/API manifests including private `custom_.txt`; deterministic handoff/archive hashes; TOCTOU/snapshot cleanup; retention-days/`if-no-files-found: error`; and public `custom_.txt` redaction. PR12 source Swagger annotations and regenerated `api/docs/api` plus `api/docs/admin` use module-pinned `swag v1.16.3`; JSON/YAML/`docs.go` outputs are deterministic, and route/auth/schema/redaction parity checks pass.
- Real offline-kit verify/freeze fails closed on secret-bearing artifact presence, legacy manifest, missing engine, empty printer digest, and incomplete license evidence. Full `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` pass after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; production lock code is unchanged. Redis integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`, with no live Redis run recorded.
- Full `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` pass after the test-only cache diagnostics, opt-in Redis changes, and lock-test race fixes; production lock code is unchanged. API build, UI build, deterministic Swagger regeneration/parity, Compose checks, and `git diff --check` pass. Redis integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`, with no live Redis run recorded. Default OpenSSL/FindBin compatibility, native `actionlint`/`shellcheck`/`pwsh`, live provider/runner execution, clean/repeat builds, cross-DB coverage, and Windows/Linux/Android execution remain unverified. Swagger metadata remains sparse for legacy operations as a low-usability issue.
- Workflow dispatch requires a branch/tag selector, exact contents-at-`WorkflowSHA` readiness, and a matching run `head_sha`; protected/immutable workflow-ref deployment remains a release gate.
- Follow-up commit `68e7f30` (`workflow: harden ruleset failure handling`) was pushed to
  `refactor/deskforge-corrective-pr`, and PR #59 remains open. GitHub run `31785951197` passed
  Go, Rust, Admin UI, and CodeQL/analyzer checks; CodeRabbit was skipped because the PR has 191
  files. The PR11 helper is untracked at the remote HEAD, and PR10/PR11 remain `in-progress`
  with their evidence limitations preserved; the PR is not marked ready and no live evidence is
  claimed.
- Latest ruleset review/fix confirmation: modern repository rulesets are the sole protection
  surface; initial and later list-page/detail/policy/contract failures fail closed. The former
  tag-protection LIST compatibility path is **CONFIRMED OBSOLETE** because DeskForge is hard-wired
  to `https://api.github.com`, has no configurable GHES base URL/provider, and current GitHub
  documentation marks that legacy surface as closing down/removed. A GHES `enabled`/`pattern`
  schema is separate historical context, not a DeskForge capability.
- Endpoint distinction is explicit: GitHub's organization authoring endpoint
  `https://docs.github.com/en/rest/orgs/rules#create-an-organization-repository-ruleset`
  (`/orgs/{org}/rulesets`) uses repository selectors, while DeskForge consumes the repository
  list `/repos/{owner}/{repo}/rulesets?targets=tag&includes_parents=true` and detail
  `/repos/{owner}/{repo}/rulesets/{id}?includes_parents=true` from the [repository ruleset
  docs](https://docs.github.com/en/rest/repos/rules#get-a-repository-ruleset). The provider-resolved repository detail
  establishes applicability; only `conditions.ref_name` is required for tag matching, and
  unknown/malformed condition fields fail closed.
- Exact authenticated observation on 2026-08-14 for `github/docs`: list returned ruleset
  `18281681`, `target=tag`, `source_type=Organization`, `source=github`, and
  `enforcement=active`, with no conditions/rules/bypass metadata. Detail returned
  `conditions.ref_name` only, rules `deletion`, `non_fast_forward`, `creation`, and parameterless
  `update`, plus `current_user_can_bypass=never`; `bypass_actors` was omitted because the token
  lacked write access. The positive contract uses `bypass_actors: []` and omission fails closed;
  the exact ref patterns are recorded in `api/README.md`.
- Root cause/fix: repository-scoped inherited Organization/Enterprise details were incorrectly
  required to contain exactly one repository selector. Selector parsing/evaluation was removed.
  Tag `update` may omit parameters; present parameters remain strict and branch `update` still
  requires boolean `update_allows_fetch_and_merge`. The workflow-dispatch selector TOCTOU,
  live-provider, PR10/PR11, cross-DB, and offline limitations remain unchanged.
- Current Phase 13 state: schema 283 is local-only and adds the CustomPreset owner/name
  unique index with a fail-closed duplicate preflight; schema 282 remains historical.
  The exact workflow identity marker and `d67b6e7` stored v2 producer-manifest and exact
  canonical-output proof are locally required. RustDesk PR #7 is merged into
  `rustqs/workflows` at `ced31ae07f69c20119b88212b10d2eb2df651c97`, containing
  `6ef1cd7fe`; `workflow-v1.2.0` does not exist. PR #59 remains open/dirty remotely
  because `2c38c87`, `da42521`, `9a1ee5e`, and `d67b6e7` are not pushed. These facts do
  not provide live artifact proof, cross-DB evidence, or atomic selector/SHA binding.

## Immediate next action

PR9 is `verified-with-notes`; PR10 and PR11 remain `in-progress`, PR12 is
`verified-with-notes` for documentation/tracker reconciliation, and DeskForge Phase 13 remains
local-only. Do not mark PR10 or PR11 complete without their phase-specific evidence.
Do not mark PR #59 ready or claim live evidence while its local remediation commits are
unpushed. No live provider execution, clean build, or release publication is implied by
the PR9 notes. The exact scope, gates, and dependencies are maintained in the
corresponding phase documents.

## Final restoration checks

- [x] `git diff --check`
- [x] Verify exactly 15 canonical files: `plan.md`, `todo.md`, and `phases/phase-1.md` through `phases/phase-13.md`.
- [x] Historical restoration check (2026-08-09): that restoration changed no product/UI/
      Docker/workflow/RustDesk/external-doc files; it is not the scope of the current
      cumulative publication candidate.
