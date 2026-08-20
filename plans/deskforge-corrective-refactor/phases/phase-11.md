# Phase 11 — PR11: Linux/Android Capability

**Status:** 🟡 `in-progress` (not verified)
**Scope:** end-to-end Linux and Android build capability validation and guarded platform exposure. This phase consumes PR7 streaming artifacts and does not equate workflow-file presence with support. Local static implementation evidence is recorded below; live capability evidence remains blocked and the capabilities remain disabled.

## Behavioral contract

A user may choose Linux or Android only when the corresponding capability is known-good: the owned workflow, bridge dependencies, custom settings embedding, exact artifact, streamed delivery, and download path all pass. Unsupported or unproven platforms fail closed and are not shown as selectable normal capabilities.

## Local implementation evidence — 2026-08-10

These are local source/static implementation and focused-test observations only; they do
not verify real platform builds, provider execution, APK/package creation, installation,
custom runtime delivery, or Android app-ID behavior in a built application:

- Bridge staging and hash restore are implemented and locally evidenced.
- The DFP1-only workflow/helper boundary is preserved, with no alternate payload path.
- Manifest v2 is implemented and locally evidenced.
- Android app-id validation preserves the `applicationId` contract.
- Android `custom_.txt` uses the required runtime path and has a fail-closed packaging
  check rather than best-effort embedding.
- Linux deb/RPM packaging preserves `custom_.txt` ordering.
- Portable deterministic traversal/timestamp tests pass locally.
- Capability gating keeps the normal UI platform choice disabled until evidence is
  green; unsupported or unproven platforms remain unavailable.

Workflow manifests, bridge/helper source, app-ID/`applicationId` checks, custom_.txt
runtime-path checks, and local package assertions are not live evidence. Deleted local
reference workflow copies and frozen `rdgen` workflows are not proof of provider
execution, packaging, installation, or support.

## Shared repository verification — 2026-08-12

The full repository checks pass independently of live platform capability: `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and `GOWORK=off go test -race ./...` passed after test-only cache diagnostics, opt-in Redis test changes, and test-only lock-test race fixes; production lock code is unchanged. The API build, UI build, deterministic Swagger regeneration/parity, Compose checks, and `git diff --check` also passed. These checks do not provide live Linux/Android/Windows/provider, package/install/runtime, signing, protected-ref, clean/repeat-build, cross-DB, signature, or offline-build evidence.

## Shared workflow approval and dispatch evidence — 2026-08-11

- The local API contract now records provider-derived verified annotated commit tags, accepted
  verification reasons, active matching protection with explicit update/deletion rules, no bypass
  actors, and same-label tag/branch collision rejection. Approval/migration checks cover the
  historical additive fields at `DatabaseVersion 282`; the later current local schema-283
  candidate is recorded in Phase 13. Safe typed errors keep provider bodies, selectors,
  credentials, and encrypted payloads out of normal responses and diagnostics.
- Preparation and dispatch revalidate the approved selector, protected policy, exact workflow
  contents, and resolved SHA immediately before DFP1 encryption. This is local focused evidence,
  not proof that a provider POST is immutably bound to that SHA.
- GitHub `workflow_dispatch` requires a branch/tag selector and does not provide atomic SHA
  binding. A theoretical selector move between final validation and POST can expose DFP1 to a
  different workflow revision, so secret-bearing production dispatch/live readiness remains gated
  until provider-side immutable/no-bypass deployment proof or atomic API support is evidenced.

## Required contract

- Real `rustqs-linux.yml` and `rustqs-android.yml` runs execute on the approved fork/branch with the PR6 ownership mapping.
- vcpkg, Flutter, `build.py`, packaging, signing/input, and bridge dependencies are verified for the supported environment.
- Android `custom_.txt` embedding must be validated in a real build; local generation and
  fail-closed packaging checks are not live embedding evidence.
- Exact artifact names/paths, PR4 provenance, PR5 version/build-ref, and PR7 streaming/download completion agree end to end.
- UI/API platform choices are enabled only behind the validated capability gate.

## Gates

- Use approved non-production provider credentials and redact all secrets; record exact workflow run, artifact, and environment evidence.
- Verify failure behavior for missing dependencies, unsupported platform, incomplete embedding, wrong artifact, stream interruption, and provider terminal errors.
- Recheck public endpoint and runtime reachability without claiming Docker/dev-compose parity unless it is actually tested.
- Preserve the PR1 closed platform domain and all PR2/PR3/PR4/PR5/PR6 limitations; no platform is marked supported from YAML presence alone.

## Explicit blockers / non-claims

PR11 remains `in-progress`, not verified. The following blockers are explicit:

- All changes are unpublished, and the helper is untracked at the remote HEAD.
- No live Linux, Android, Windows, or provider runs have been performed.
- No APK/package/install/runtime evidence exists.
- No protected workflow-ref proof exists.
- No clean-worktree or repeat-build evidence exists.
- Required toolchains and native validators are unavailable.
- RustDesk `hbb_common` remains dirty and unpublished.
- Android signing/debug evidence is unavailable.
- Cross-DB and cryptographic signature evidence are unavailable.

## Dependencies / remaining evidence

Depends on PR6 workflow ownership/config, PR7 streaming artifact, PR9 reproducibility/action pins, and PR10 offline-kit/release inputs. Local implementation evidence is recorded above, but live Linux/Android workflow execution, exact APK/package output, installation/runtime delivery, and provider-backed download completion remain unverified. PR10 and PR11 remain `in-progress`; PR12 is `verified-with-notes` for the completed documentation reconciliation.
