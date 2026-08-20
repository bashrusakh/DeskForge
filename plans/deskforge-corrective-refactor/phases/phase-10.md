# Phase 10 — PR10: Offline-Kit/Release Evidence

**Status:** 🟡 `in-progress`
**Scope:** Packages A–C: offline-kit freeze/verify-only hardening, redacted operator handoff, and active-artifact retention/export evidence. Package C includes the active fork/API workflow retention, exact-output, and manifest contract. This plan record does not perform RustDesk-source build execution, tag, release, asset-upload, commit, or push actions; no release claim is made.

**Current evidence boundary:** Packages A–C implementation and focused checks pass locally: strict offline fixtures/stage checks; v2 producer/API manifests including private `custom_.txt` declaration; deterministic handoff/archive hashes; TOCTOU-safe snapshot cleanup; retention-days/`if-no-files-found: error`; and public `custom_.txt` redaction. PR10 must remain `in-progress`, not `verified-with-notes`, because real offline-kit verification fails closed and the release-boundary gates remain unproven.

## Behavioral contract

An operator naturally chooses **verify-only** for a frozen kit or follows a
redacted handoff/export verification for a specific build. The values are not
manual guesses: source/ref/commit, workflow identity, run/artifact identity,
release/assets identity, output files, and service digest come from frozen
files or stored immutable build provenance; expected hashes come from a trusted
reviewable manifest. Existing PR4/PR5 immutable provenance, PR7 exact
run/artifact binding, and the service-owned PR7 output digest are the patterns
that represent this action. The normal operator path must not expose PATs,
payload keys, generated secrets, raw database fields, or provider-internal
manual selectors.

## Completed-inventory blueprint

### Package A — offline-kit: freeze and verify-only

**Files/evidence:**

- `offline-kit/freeze.sh` and `offline-kit/versions.env` — staged downloads,
  source/toolchain pins, mutable URLs, vcpkg baseline, and third-party pins.
- `offline-kit/README.md` and `offline-kit/FORK-PROCEDURE.md` — storage,
  fork, release-asset, and historical/manual procedures; no current sovereignty claim.
- `offline-kit/artifacts/MANIFEST.txt`, `sha256sums`, and
  `printer_sha256sums` — observed manifest/checksum evidence.
- `offline-kit/artifacts/rustdesk-1.4.8.bundle`,
  `vendor-1.4.8.tar.gz`, and the frozen `rustdesk-src` tree, including
  `.gitmodules`, `Cargo.lock`, `.cargo/config.vendor.toml`, and licenses.
- `win-builder/README.md`, `setup.ps1`, and `agent.ps1` — frozen Windows-only
  manual/historical evidence, not the active API or release workflow.

**Required work contract:**

1. Keep an explicit online `freeze` mode, but add/define a no-network
   `verify-only` boundary that neither downloads nor mutates a checkout or
   observed manifest.
2. Require equality among `versions.env`'s `RUSTDESK_COMMIT`, source `HEAD`,
   declared ref/tag, bundle contents, recursive submodule commits, and the
   source tree used for vendor/config evidence. Reject shallow or dirty source.
3. Compare every artifact to independently trusted expected SHA-256 values;
   a generated `MANIFEST.txt`, a URL, or a first-seen hash is not an expected
   trust root.
4. Treat mutable URLs and TOFU as retrieval risk. A mutable location must have
   an independent expected hash/provenance or remain explicitly unverified.
5. Record vendor completeness, license/source notices, submodule provenance,
   manifest inputs, sizes, hashes, and repeatability.

**Focused implementation checks:** Strict offline fixtures and freeze/verify-only
stage checks pass locally, including fail-closed handling for secret-bearing
artifact presence and staged manifest/input conditions.

**Real-kit verification boundary:** A real offline-kit verify/freeze run fails
closed on secret-bearing artifact presence, the legacy manifest, missing engine,
empty printer digest, and incomplete license evidence. `git bundle verify`, source
`rev-parse`/clean-tree/recursive submodule checks, independently trusted
expected-manifest checks, and repeat no-mutation verification remain required
evidence and are not closed by fixture/stage tests.

**Stop points / risks:** stop for a network attempt, missing expected hash,
mutable URL without independent pin, source/ref/bundle/submodule mismatch,
shallow/dirty state, incomplete vendor/license evidence, partial artifact, or
manifest rewrite. Risks are ignored multi-gigabyte artifacts, resumable files
mistaken for verified files, hash-only evidence mistaken for authenticity, and
Windows vcpkg/binary-cache requirements not covered by Linux checks.

### Package B — operator handoff: provenance/checksum export

**Files/evidence:** `api/model/custom_build.go`, `api/service/custom_build.go`,
`api/service/build_provenance_test.go`,
`api/http/controller/admin/custom_build.go`,
`api/http/controller/admin/github_build_poll_test.go`, and the configured
fork's protected workflow files. The handoff is an export/manifest for an
operator, not a secret-bearing normal response and not a publication action.

The manifest must identify exactly: source repository, selector/ref, source
commit/tree SHA and submodule commits; workflow repository/path, protected
selector, workflow SHA/file identity and action pins; provider/run ID, run
URLs and `head_sha`; artifact ID/name; release tag/ID and exact asset names
and digests; final output file names/sizes/per-file hashes; service-owned output
digest; export location; and verification time/result. Generate it from stored
immutable provenance, re-read and compare it at export time, and fail closed
on inferred, missing, changed, or provider-only values. Redact PATs, payload
keys, generated secrets, credential-bearing URLs, and raw internal secret
fields. Export verification must work from the captured evidence without
publishing or uploading anything.

**Checks:** Package B implementation and focused tests pass locally: v2
producer/API manifests, including the private `custom_.txt` declaration;
deterministic redacted handoff and archive hashes; exact identity, checksum,
digest, sidecar, and public `custom_.txt` redaction assertions; and offline
read-back verification without provider access. No live provider evidence is
inferred.

**Stop points / risks:** stop for incomplete legacy provenance, changed
workflow contents, missing run/artifact/release identity, checksum/digest
mismatch, secret leakage, non-deterministic output, or publication attempt.
Risks are provider URL drift, replaced release assets, unsigned checksums being
misread as signatures, and the export becoming a secret exfiltration path.

### Package C — active artifacts: retention and exact export

**Files/evidence:** `api/service/published_output.go`,
`api/service/build_output_cleanup.go`, their focused tests,
`api/model/custom_build.go`, and the existing download/poll controller paths.
The service-owned canonical output root and numeric build directories are the
boundary; `offline-kit/artifacts/` is not an API output directory.

Define separate retention for active `.part`/`.zip` downloads,
extraction/staging/recovery paths, failed outputs, and completed canonical
outputs. Cleanup may remove only an exact stale service-owned temporary shape;
active builds remain protected; completed output remains until its explicit
retention/export policy is satisfied. Exact-file/manifest verification must
require regular non-symlink files, bounded count/size/aggregate, canonical
relative names, deterministic ordering, and the required Windows executable.
The service-owned digest binds immutable build/run/artifact and release/assets
identity to exact names, sizes, and contents. An export sidecar/metadata record
carries the digest and manifest identity and is checked against the bytes before
handoff; it is evidence, not a signature or release asset.

Preserve Windows-only production capability and fail closed for Linux/Android
until PR11 supplies real end-to-end evidence. Keep `win-builder/` frozen and
manual/historical-only. Local output, an export sidecar, or a handoff
manifest must never be described as a live release.

**Checks:** Package C implementation and focused tests pass locally:
TOCTOU-safe snapshot/archive cleanup, exact-manifest and digest/publication-proof
checks, retry/partial-output handling, active-versus-terminal retention,
retention-days and `if-no-files-found: error` workflow contracts, mutation/missing-
file and symlink/path/name/size-limit rejection, export-sidecar verification, and
Windows-only capability checks. Completed-output retention/deletion remains a
manual operator policy; the API does not claim an automatic completed-output TTL
or release-retention policy. Cross-DB and live-provider checks remain gates below.

**Stop points / risks:** stop for ambiguous ownership, unsafe cleanup scope,
manifest/digest/sidecar mismatch, post-proof mutation, missing Windows
executable, or Linux/Android/release exposure. Risks are premature deletion,
stale recovery trees, digest replay, detached metadata, and confusing artifact
retention with release retention.

## Explicit gates, checks, and non-claims

These are stop gates, not gaps to work around:

- **Protected workflow ref:** deployment must protect the immutable workflow
  selector and verify exact workflow contents at the resolved SHA; the required
  dispatch branch/tag selector remains distinct from the workflow file SHA.
- **Submodule publication:** RustDesk `hbb_common` must have clean, published
  provenance before reproducibility or sovereignty is claimed; dirty/unpublished
  submodule state remains a blocker.
- **Live provider:** live GitHub/provider, runner, and workflow execution remain
  unverified and are never simulated by local metadata.
- **Signatures:** SHA-256 manifests/digests provide integrity evidence only;
  signatures, attestations, and their trust roots remain a separate gate.
- **Cross-DB:** MySQL and PostgreSQL migration/read-write evidence remains
  required; SQLite-only checks do not close this gate.
- **Network-denied full build:** a complete offline build, including Windows
  toolchain/vcpkg capability and repeat-build evidence, remains required;
  verify-only or metadata checks do not satisfy it.

Release publication, tag creation, asset upload, secrets, destructive cleanup,
and a live release/readiness claim are outside PR10. A package may be marked
verified only with its own evidence; otherwise record `blocked` or
`verified-with-notes`.

## Local Package B/C implementation update — 2026-08-10

- The active fork workflows already emit one JSON schema in `manifest.txt`:
  `schema="deskforge.client-artifact"`, `schema_version=2`, `platform`,
  `app_name`, ordered `output_filenames`, `source_sha`, `workflow_sha`,
  `workflow_ref`, `version`, the exact digest scope
  `sha256 covers public delivered output files; manifest.txt and declared private
  files are excluded`, and ordered
  `files` entries containing `name` and lowercase SHA-256 `sha256`.
- DeskForge now parses this bounded schema once at the ZIP extraction boundary,
  rejects duplicate JSON keys/unknown fields, compares source/workflow/version/
  platform/app identity with the stored build snapshot, derives the exact
  platform output names from one shared mapping, and verifies extracted bytes.
  New provider-run/partial-identity artifacts require the manifest; rows with no
  provider run and no immutable identity retain the existing legacy extraction
  compatibility path. Windows normal-path filtering is not weakened.
- The admin handoff now contains sorted non-secret output entries (`name`,
  `size`, `sha256`), an explicit `verification_result`, the stored publication
  timestamp, and the existing source/workflow/run/artifact/release identity.
  Request-time `generated_at` is omitted. The service digest scope explicitly
  allows canonical output to include private `custom_.txt`; the handoff omits
  that entry and never exposes its contents. SHA-256 evidence is not a
  signature.
- Local focused Go tests, race tests, targeted vet/build, RustDesk workflow
  contract checks, active-workflow manifest marker checks, and `git diff --check`
  passed. The full `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and
  `GOWORK=off go test -race ./...` checks also pass after test-only cache diagnostics,
  opt-in Redis test changes, and test-only lock-test race fixes; production lock code is
  unchanged. The API build, UI build, deterministic Swagger regeneration/parity, and
  Compose checks pass. No live provider/runner, migration, publication, or external
  code-sharing check was run.

## Package B/C integrity follow-up — 2026-08-10

- The shared producer manifest writer now validates the output root and every
  declared path with `lstat`, rejects symlinks and special files before hashing
  or writing `manifest.txt`, rejects extra output files, and emits v2 producer
  source-tree/submodule evidence as `reported`. Older v2 scope/result labels
  remain readable and normalize to the current report label when stored.
- v2 now declares optional `private_filenames`; the only accepted private name
  is `custom_.txt`. The writer emits it without size/content/hash records when
  present, public `files` and digest scope exclude it, and Windows/Linux/
  Android workflows copy the already-embedded private file beside public
  output before writing the manifest. Extraction accepts only that declared
  private file and rejects every other unlisted or secret-bearing filename.
- Public ZIP export now takes the shared canonical-output export lock, copies
  bounded regular files into a private snapshot while hashing them, packages
  only that snapshot, and rechecks the canonical service digest before returning.
  A mutation aborts before response headers; `X-DeskForge-Archive-SHA256` is
  computed from the actual archive bytes. Existing published digest semantics,
  artifact names, and admin/public authorization boundaries are unchanged.
- The admin handoff now includes stable `export_route`, explicit
  `verification_status`, and a `producer_report` whose source-tree/submodule
  fields remain `reported`; provider release-asset digests are separately
  `reported` and explicitly not recomputed by DeskForge. Service publication,
  output, and identity checks use `service_verified`. Publication timestamp
  and verification status are derived from stored publication proof; request
  timestamps and host paths are excluded. The admin response body is emitted
  once and its exact bytes are hashed in `X-DeskForge-Manifest-SHA256`.
- Active Windows/Linux/Android workflows install failure cleanup traps before
  writing sensitive `custom_.txt` temp files and add an always-run cleanup step.
  The workflow contract test covers trap placement, regular/symlink/special
  writer inputs, path escape rejection, v2 report labeling, YAML, Bash, action
  pins, permissions, and redaction boundaries.
- Focused `go test`, `-race`, `go vet`, `go build`, Python/Bash, YAML, workflow
  contract, Swagger regeneration/parity, Compose, and `git diff --check` checks pass.
  The full Go vet/test/race commands pass after test-only cache diagnostics, opt-in Redis
  test changes, and test-only lock-test race fixes; production lock code is unchanged.
  `actionlint` and `shellcheck` are unavailable; no live provider/runner, migration, release,
  publication, or external code-sharing check was run.

## Active/frozen workflow evidence boundary

- The configured RustDesk fork workflows are the active executable source and
  their manifest, retention-days, `if-no-files-found: error`, and cleanup
  contracts have focused local evidence only. No provider, runner, or live
  workflow result is inferred from those files or tests.
- Frozen offline-kit artifacts/procedures and `win-builder/` remain manual/historical-only
  evidence, not an active release path. Their presence cannot close the protected
  workflow-ref, clean published submodule, signature, cross-DB, or complete
  network-denied full-build gates.

## Current state carried from PR9

- PR1–PR9 and the security-hardening work are `verified-with-notes`. Focused
  Go/UI/Docker/Cargo metadata/DFP1/workflow-contract checks pass locally.
- Full `GOWORK=off go vet ./...`, `GOWORK=off go test ./...`, and
  `GOWORK=off go test -race ./...` pass after test-only cache diagnostics, opt-in Redis test
  changes, and test-only lock-test race fixes; production lock code is unchanged. The API
  build, UI build, deterministic Swagger regeneration/parity, Compose checks, and
  `git diff --check` pass. Redis integration tests and benchmarks run only when
  `DESKFORGE_TEST_REDIS_ADDR` is configured, and no live Redis run is recorded. Default
  OpenSSL/FindBin compatibility, native `actionlint`/`shellcheck`/`pwsh`, live provider/runner
  execution, clean-worktree/repeat-build evidence, cross-DB coverage, and
  Windows/Linux/Android execution remain unverified.
- Workflow dispatch requires a branch/tag selector, exact contents-at-
  `WorkflowSHA` readiness, and a matching run `head_sha`; protected/immutable
  workflow-ref deployment remains a release gate. RustDesk `hbb_common`
  timestamp work remains dirty and unpublished submodule state.

## Workflow approval and dispatch evidence carried into PR10 — 2026-08-11

- Local provider-contract/focused-test evidence requires a provider-derived verified annotated
  commit tag, an accepted verification reason, active matching protection with explicit update
  and deletion rules, an empty bypass-actor list, and rejection of a same-label branch collision.
  Missing, conflicting, malformed, or bypassable protection is not evidence of immutability.
- Approval and migration evidence records the selected tag, provider-resolved SHA, and policy
  state through the historical additive `DatabaseVersion 282` fields while retaining required
  legacy compatibility. The later current local schema-283 candidate is recorded in Phase 13.
  Preparation and dispatch revalidate the selector, workflow contents, policy, and SHA before
  secret-bearing DFP1 payload creation; typed provider/approval/validation/contract failures use
  bounded safe errors and do not expose credentials, provider bodies/URLs, selectors, or payloads.
- The provider boundary remains open: GitHub `workflow_dispatch` requires a branch/tag selector
  and does not bind the POST atomically to a SHA. A theoretical selector move between final
  validation and POST can expose DFP1 to another workflow revision. Secret-bearing production
  dispatch/live readiness therefore remains gated until provider-side immutable/no-bypass
  deployment proof or atomic API support is actually evidenced.

## Dependencies / remaining evidence

PR10 depends on the verified-with-notes PR9 reproducibility/action-pin record,
PR7 exact artifact delivery/digest boundary, and PR6 workflow ownership. Package
A/B/C implementation and focused checks pass locally, but real offline-kit
verify/freeze fails closed on secret-bearing artifact presence, the legacy manifest,
missing engine, empty printer digest, and incomplete license evidence. Protected
workflow-ref deployment, clean published RustDesk `hbb_common` submodule provenance,
live provider/runner execution, signatures/attestations, MySQL/PostgreSQL
cross-DB evidence, a complete network-denied full build, and release publication
remain unproven. Completed-output retention remains manual/explicit; no completed-
output TTL, release-retention policy, signature/attestation, live provider/runner
result, or complete offline full-build proof exists.
