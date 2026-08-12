# Phase 9 — PR9: Go/Rust Reproducibility + SHA-Pinned Actions

**Status:** ✅ `verified-with-notes`
**Scope:** reproducible Go/Rust build inputs and provenance, plus immutable SHA pinning for active workflow actions. This phase records build-integrity evidence; it does not claim live workflow execution or release publication.

## Behavioral contract

An operator or release process builds from declared Go/Rust toolchains and dependency inputs and can compare the resulting source/build identity with the recorded provenance. CI actions resolve to reviewed immutable SHAs rather than mutable tags or branches. Normal users select a known capability; they do not edit toolchain, action, or provider internals.

## Required contract

- Go modules, Rust crates/toolchains, vendored inputs, build flags, and generated inputs have an auditable source of truth.
- Repeated builds under the supported environment produce explainable, reproducible outputs or explicitly recorded nondeterminism.
- Every active third-party workflow action is pinned to a full commit SHA and reviewed for repository ownership/version intent.
- Build metadata connects source, toolchain, workflow, action pins, and artifact identity without leaking secrets.

## Gates

- Inventory Go/Rust versions, lockfiles, vendored dependencies, generators, environment inputs, and active workflow action references.
- Run repeat-build and clean-environment comparisons where supported; record exact limitations instead of inferring reproducibility.
- Verify SHA pins, transitive action usage, permissions, and update ownership across active workflows; do not modify frozen/vendored workflows without evidence of ownership.
- Add focused checks for provenance completeness and mutable action-reference detection.
- Preserve PR4/PR5 immutable identity and PR6 workflow ownership boundaries; no live provider or release claim is implied.
- Treat protected/tagged workflow refs as a deployment gate: GitHub dispatch must retain a
  branch/tag selector, exact workflow contents must be ready at the resolved `WorkflowSHA`, and
  the run `head_sha` must match. A mutable branch is not claimed to be immutable. Production
  publication requires an operator-approved protected branch or immutable workflow-ref policy
  in the configured self-hosted fork.

## Recorded evidence

- Go 1.25.0, `GOTOOLCHAIN=local`, tracked module metadata, existing swag version, the
  committed Rust lockfile, and a pinned Rust Docker toolchain are recorded in the active
  build paths.
- Active DeskForge workflow actions are pinned to full commit SHAs with reviewed ownership
  and scoped permissions. The configured RustDesk fork remains the sole executable source
  for client workflows; `github-build/` and vendored `rdgen` copies remain reference/frozen.
- Focused Go, UI, API, Docker/Compose, Cargo metadata, DFP1 payload, workflow-contract,
  Swagger regeneration/parity, and `git diff --check` checks passed locally. The API build and
  UI build passed. On 2026-08-12, `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and
  `GOWORK=off go test -race ./...` also passed after test-only cache diagnostics, opt-in Redis
  test changes, and test-only lock-test race fixes; production lock code is unchanged.
- Active fork bridge/windows/linux/android workflows now accept only the authenticated `DFP1`
  payload, bind its `workflow_repo` to the executing fork, and fail closed before checkout/build
  for direct/manual runs without a valid payload. The API still dispatches the required encrypted
  payload and preserves the GitHub branch/tag selector contract.
- `SweepBuildOutputTemps` removes bounded, stale interrupted `.part`/download/archive files and
  service-owned `.artifact-recovery-*` files/directories inside inactive per-build output
  directories while preserving active build directories, published output, and unrelated files.
  DB-first custom-build deletion remains fail-safe; without a durable tombstone, complete orphan
  directory removal is intentionally left for operator review rather than risking a published
  artifact.
- Custom build/preset model saves and service boundaries enforce one exact canonical typed field
  allowlist. Compacted aliases, record/internal fields, unknown keys, nested values, nulls, and
  stringified booleans are rejected on new writes; valid BuildSpec fields and legacy internal
  reads remain supported, and safe DTOs stay redacted.
- RustDesk `hbb_common` timestamp work remains dirty and unpublished submodule state; it
  is not clean published reproducibility evidence. `SOURCE_DATE_EPOCH` remains deterministic,
  while wall-clock metadata remains an explicit `RUSTDESK_NON_REPRODUCIBLE_DEBUG=1` opt-in.
  Focused Cargo metadata and targeted checks passed locally. The 2026-08-12 broad Go
  verification records `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and
  `GOWORK=off go test -race ./...` passing after test-only cache diagnostics, opt-in Redis test
  changes, and test-only lock-test race fixes; production lock code is unchanged. Redis
  integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`, with no live Redis
  run recorded.

## Remaining evidence before a reproducibility claim

- Establish repeat-build and clean-environment comparisons where supported, including artifact
  identity and provenance checks; do not infer reproducibility from local checks alone.
- Keep live provider/runner execution, cross-DB evidence, Windows/Linux/Android execution,
  default OpenSSL/FindBin compatibility, native `actionlint`/`shellcheck`/`pwsh`, and release
  publication outside the PR9 claim until separately verified.
- The protected/immutable workflow-ref deployment gate and real fork/provider execution remain
  unverified locally; no mutable branch is treated as an immutable release source.

## Dependencies / remaining evidence

Depends on PR4 provenance, PR5 immutable version/build-ref, PR6 workflow ownership/config, the actual streaming artifact contract from PR7, and the recorded PR8 secret-persistence boundary. Live provider/runner execution, clean-environment and repeat-build comparisons, cross-DB evidence, default OpenSSL/FindBin compatibility, native `actionlint`/`shellcheck`/`pwsh`, Windows/Linux/Android execution, and release proof remain unverified; PR10 and PR11 remain `in-progress`, while PR12 is `verified-with-notes` for documentation only.
