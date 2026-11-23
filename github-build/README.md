# github-build — reference/docs for fork workflows

All executable client-build workflows live in the configured RustDesk fork — canonical
workflow-ownership statement: [docs/FORKS-AND-PINS.md §1](../docs/FORKS-AND-PINS.md#1-topology).
This directory is reference/documentation only (no executable or deployable workflow copies);
the fork's branch/ref and workflow files are provider-side execution details it does not define
or deploy.

**Dispatch contract under test:** sends only the `ref` selector and required `inputs` (no
`return_run_details`), pinned to `X-GitHub-Api-Version: 2026-03-10` (client-wide pin stays
`2022-11-28`). Local fake-transport/static checks cover this contract; no normal GitHub dispatch,
poll, or download operation is verified. Contract rationale and evidence labels:
[PLAN.md §2](../PLAN.md#2-architecture).

The ref `rustqs/workflows` is a backup copy of the fork's `master`, kept so the workflow files
cannot be lost if something happens to `master` — it is **not** a dispatch selector: production
approval and dispatch require a guarded, provider-verified immutable `refs/tags/workflow-*` tag,
retained read-only in code for legacy compatibility. At the provider-resolved immutable commit,
the mapped workflow must declare `workflow_dispatch` and contain the exact
`# deskforge-workflow-identity-guard: v1` marker. The following ref labels describe the
configured fork's roles; they are not a claim that synchronized copies, protected tags, or a
live provider run currently exist:

| Ref                       | Purpose                                                        |
| ------------------------- | -------------------------------------------------------------- |
| `master`                  | Fork default branch; not a dispatch selector                  |
| `rustqs/workflows`        | Backup copy of the fork's `master` (workflow-file insurance if `master` is lost); not a dispatch selector; retained read-only for legacy compatibility |
| `rustqs/master-workflows` | **Deleted former ref** — fork-maintained workflow mirror/reference ref (workflow-file backup for re-applying after an upstream sync — distinct from the whole-`master` backup above); absent from the fork's branches as of 2026-11-24; historical reference only; not a selector |

| Platform | Executable workflow in fork                           | Status    |
| -------- | ----------------------------------------------------- | --------- |
| Windows  | `.github/workflows/rustqs-windows.yml`               | 🟡 API-enabled; live evidence pending |
| Linux    | `.github/workflows/rustqs-linux.yml`                  | 🟡 API capability-gated |
| Android  | `.github/workflows/rustqs-android.yml`                | 🟡 API capability-gated |

> **Current-state notes.** Windows is the only platform admitted by the API production capability gate; Linux/Android mappings exist but remain gated until PR11 records end-to-end provider, artifact, embedding, and download evidence — workflow manifests, bridge/helper source, Android app-ID/runtime-path checks, and package assertions are static evidence only, and no live provider run or clean-environment build proof is recorded here.
> RustDesk guard PR (recorded 2026-08-20): [RustDesk PR #7](https://github.com/bashrusakh/rustdesk/pull/7) merged into `rustqs/workflows` at `ced31ae07f69c20119b88212b10d2eb2df651c97` (prior source commit `6ef1cd7fe`) — adds the marker, requires outer and authenticated inner SHA checks in the bridge, and gates draft Linux/Android before secret-bearing jobs.
> `workflow-v1.2.0` does not exist (checked 2026-11-24); annotated tags `workflow-v1.2.1` (at `ced31ae`), `workflow-v1.3.7`, and `workflow-v1.3.8` exist in the fork — existence only, verified via `git ls-remote` 2026-11-24; signing and tag/ruleset protection are not verified here, and live reapproval/reverification remains pending.
> This is not provider deployment, tag protection, live workflow, or live artifact evidence. DeskForge-local remediation state for the same date (commits `da42521`/`9a1ee5e`/`d67b6e7`, PR #59): [CHANGELOG.md](../CHANGELOG.md) ("Local PR #59 review remediation — 2026-08-20").

---

## Architecture

The API maps the selected platform to an owned workflow in the configured RustDesk fork and dispatches it with an encrypted payload; the provider returns the exact run identity, which the API stores for subsequent polling. Stored run and artifact identities are required — the API selects no sole artifact and infers no path from a provider response. Flow diagram and full contract: [PLAN.md §2](../PLAN.md#2-architecture).

### Version flow

The `version` field in the admin UI is **not just metadata** — it is resolved from the configured provider repository, passed in the encrypted payload, and used by the owned workflow for its matching build assets.

- Admin UI loads available versions from `GET /api/admin/custom_build/versions`
- The API queries the configured repository for matching `offline-assets-*` releases, checks the provider-reported matching source tag and required asset metadata, and exposes only the resulting display versions; independent source/asset provenance verification remains a separate gate
- If the provider catalog is unavailable, the API returns an empty/error state; there is no hardcoded repository or obsolete version fallback
- The version list is cached for ~5 minutes; a newly published release may take a few minutes to appear in the dropdown
- The workflow receives the resolved version in `enc_payload`; the normal build flow accepts no separate raw/manual version identity
- The active Windows dispatch receives two provider-set transport inputs — the public outer `workflow_sha` and the authenticated `DFP1` `enc_payload`; the admin never authors a SHA, a no-secret outer job validates the outer SHA against `github.sha` before secret-bearing jobs, and after MAC/decryption each secret-bearing path requires the authenticated inner SHA to match both values before exports, checkout, or build use (mechanics: [PLAN.md §2](../PLAN.md#2-architecture)); direct/manual runs without a valid payload fail closed as non-build diagnostics
- The authenticated payload binds the configured workflow repository to `github.repository`, so self-hosted forks remain supported without a hardcoded owner/name; GitHub receives the required tag selector, while exact workflow-content readiness and run `head_sha` checks use the resolved SHA. `workflow_dispatch` provides no atomic SHA binding between selector and executed workflow — verified annotated tags under an active immutable no-bypass ruleset are a compensating control, not an atomic guarantee, and the two-layer SHA guard is defense in depth, not an atomic defense against a malicious workflow file; the verified tag and no-bypass ruleset remain required controls

### Provider identity and asset contract

The provider supplies the source-tree/ref/commit identity, recursive submodule commits, workflow identity, release identity, and asset metadata — this directory reports those values and does not independently verify them unless a separately trusted source proves the expected value. A URL, provider-reported digest, local presence, or first-seen hash is not by itself authenticity or release evidence; the listed asset names and digest fields are a provider/project contract under test, not evidence that normal GitHub operation has been verified.

The authenticated handoff must carry the exact required asset names and their expected
SHA-256 digests. The current required asset-name contract is:

- `windows-x64-release.zip`
- `usbmmidd_v2.zip`
- `rustdesk_printer_driver_v4-1.4.zip`
- `printer_driver_adapter.zip`

Missing names, missing digests, changed content, or unavailable source/submodule identity
must fail closed rather than being inferred from a provider listing.

### Reusable workflow details

Reusable workflow behavior, including any bridge workflow, is defined in the RustDesk fork; the
`rdgen` copy is reference material only and is not copied or deployed by the current path.

The provider artifact is downloaded through the provider API, then validated, extracted, and
published in local API storage — not sent to the API by a runner callback and not published to a
public release. Frozen local file-queue builders are manual/historical-only material, not a
production fallback for this path.

Active fork uploads use the focused workflow retention contract (`retention-days: 7` and
fail-closed missing-file handling); that provider retention is not a completed output TTL,
release-retention policy, or release publication claim. The service-owned handoff records exact
source/workflow/run/artifact identity, output names, sizes, hashes, and its publication digest;
private `custom_.txt` content is not exported, and a public download may contain only a redacted
`custom_.txt` — raw/private handoff contents, payload keys, PATs, and other internal secret-bearing
values are excluded. These records are integrity/provenance metadata, not signatures or attestations.

Credentials — encrypted payload, decrypted inside the runner via GitHub Secret.

---

## Workflow ownership

- The rustdesk fork's `.github/workflows/` files are the sole executable source (canonical statement:
  [docs/FORKS-AND-PINS.md §1](../docs/FORKS-AND-PINS.md#1-topology)); `github-build/` is not a second
  workflow source or deployment copy, the vendored `rdgen` workflows must not be copied into active fork
  workflows, and active build logic changes belong in the fork — keep this README aligned with it.
- Active dispatch is tag-only and requires a guarded, provider-verified immutable workflow tag; an
  active immutable no-bypass ruleset protects that tag, and mutable branch selectors are not an allowed
  fallback.
- Secret-bearing production dispatch remains gated until live provider evidence includes tag protection
  and ruleset administration; `workflow_dispatch` provides no atomic SHA binding, so the verified tag and
  ruleset are compensating controls rather than an atomic guarantee.
- Workflow-file presence and local focused checks do not prove provider behavior; live provider
  verification remains required.

---

## Security (REQUIRED for a public fork)

- `enc_payload` — authenticated `DFP1` AES-256-CBC + PBKDF2 + HMAC envelope, with
  `WORKFLOW_PAYLOAD_KEY` in GitHub Secrets.
- `RS_PUB_KEY` is a public key, not a secret.
- `SetWorkflowSecret` — button in admin UI (`Push to GitHub Secrets`) via `nacl/box.SealAnonymous`.

---

## Upstream updates

Follow [PLAN.md §7](../PLAN.md#7-fork-maintenance-procedure-for-a-new-upstream-rustdesk-client-release)
for the upstream release process; active workflow changes belong in the rustdesk fork, and this
directory remains reference/documentation only.

### Historical fork bump log

- 2026-06-13: `setup-msbuild` v2→v3, `upload-artifact` → SHA-pinned v7.
