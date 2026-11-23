# github-build — reference/docs for fork workflows

All executable client-build workflows live in the configured RustDesk fork.
This directory is reference and documentation only; it is not an executable
workflow source and does not contain deployable workflow copies.

The API dispatches the application-owned platform workflow in the configured
RustDesk fork. The fork's branch/ref and workflow files are provider-side
execution details; this directory does not define or deploy them.

The dispatch contract under test sends only the `ref` selector and required `inputs`
(no `return_run_details`) and pins the request to `X-GitHub-Api-Version: 2026-03-10`
(the client-wide pin stays `2022-11-28`); a standard 204 and latest-run inference
remain intentionally unsupported because they do not provide an accepted exact run
correlation. Full contract rationale and evidence labels: [PLAN.md §2](../PLAN.md#2-architecture).
Local fake-transport/static checks cover this contract; no normal GitHub
dispatch, poll, or download operation is verified.

The ref `rustqs/workflows` is a backup copy of the fork's `master`, kept so the
workflow files cannot be lost if something happens to `master`. It is not a
dispatch selector: production approval and dispatch require a guarded,
provider-verified immutable `refs/tags/workflow-*` workflow tag. The ref is also
retained read-only in code for legacy compatibility (the API normalizes legacy
workflow values to it). At the provider-resolved immutable commit, the mapped
workflow must declare
`workflow_dispatch` and contain the exact
`# deskforge-workflow-identity-guard: v1` marker. The configured provider
repository resolves the selected tag and workflow identity.
The following ref labels describe the configured fork's roles; they are not a claim
that synchronized copies, protected tags, or a live provider run currently exist:

| Ref                       | Purpose                                                        |
| ------------------------- | -------------------------------------------------------------- |
| `master`                  | Fork default branch; not a dispatch selector                  |
| `rustqs/workflows`        | Backup copy of the fork's `master` (workflow-file insurance if `master` is lost); not a dispatch selector; retained read-only for legacy compatibility |
| `rustqs/master-workflows` | Fork-maintained workflow mirror/reference ref (workflow-file backup for re-applying after an upstream sync — distinct from the whole-`master` backup above); not a selector |

| Platform | Executable workflow in fork                           | Status    |
| -------- | ----------------------------------------------------- | --------- |
| Windows  | `.github/workflows/rustqs-windows.yml`               | 🟡 API-enabled; live evidence pending |
| Linux    | `.github/workflows/rustqs-linux.yml`                  | 🟡 API capability-gated |
| Android  | `.github/workflows/rustqs-android.yml`                | 🟡 API capability-gated |

> **Current-state note (2026-08-10):** Windows is the only platform admitted by the
> API production capability gate. Linux and Android mappings exist but remain gated
> until PR11 records end-to-end provider, artifact, embedding, and download evidence.
> Workflow manifests, bridge/helper source, Android app-ID/runtime-path checks, and
> package assertions are static implementation evidence only; capabilities remain disabled.
> No live provider run or clean-environment build proof is recorded here.
>
> **RustDesk guard PR (recorded 2026-08-20):** [RustDesk PR
> #7](https://github.com/bashrusakh/rustdesk/pull/7) is merged into
> `rustqs/workflows` at merge commit `ced31ae07f69c20119b88212b10d2eb2df651c97`,
> which contains prior source commit `6ef1cd7fe`. It adds the marker, requires outer
> and authenticated inner SHA checks in the bridge, and gates draft Linux/Android
> before secret-bearing jobs. The merge does not deploy a guard tag:
> `workflow-v1.2.0` does not exist, and no newly signed provider-verified immutable
> protected workflow tag exists yet; live reapproval/reverification remains pending.
> This is not provider deployment, tag protection,
> live workflow, or live artifact evidence. DeskForge-local remediation state for the
> same date (commits `da42521`/`9a1ee5e`/`d67b6e7`, PR #59) is recorded chronologically
> in [CHANGELOG.md](../CHANGELOG.md) ("Local PR #59 review remediation — 2026-08-20").

---

## Architecture

The API maps the selected platform to an owned workflow in the configured
RustDesk fork and dispatches it with an encrypted payload. The provider returns
the exact run identity, which the API stores and uses for subsequent polling.
The stored run and artifact identities are required; the API does not select a sole
artifact or infer a path from a provider response.

```text
admin-ui → Go API → workflow_dispatch (encrypted payload, owned fork workflow) →
  GitHub Actions [configured RustDesk fork] →
    L1 config.rs (server+key) → L2 custom_.txt (permanent password) → L3 branding →
    provider run ← Go API polls provider status →
    provider artifact API → Go API validates/extracts/publishes locally → admin-ui Download
```

### Version flow

The `version` field in the admin UI is **not just metadata** — it is resolved
from the configured provider repository, passed in the encrypted payload, and
used by the owned workflow for its matching build assets.

- Admin UI loads available versions from `GET /api/admin/custom_build/versions`
- The API queries the configured repository for matching `offline-assets-*`
  releases, checks the provider-reported matching source tag and required asset metadata,
  and exposes only the resulting display versions. Independent source/asset provenance
  verification remains a separate gate.
- If the provider catalog is unavailable, the API returns an empty/error state;
  there is no hardcoded repository or obsolete version fallback
- The version list is cached for ~5 minutes; a newly published release may
  take a few minutes to appear in the dropdown
- The workflow receives the resolved version in `enc_payload`; the API does not
  accept a separate raw/manual version identity from the normal build flow
- The active Windows dispatch path receives two provider-set transport inputs: the
  public outer `workflow_sha` and the authenticated `DFP1` `enc_payload`. The
  admin never authors a SHA. A no-secret outer job validates the outer SHA against
  `github.sha` before bridge/build jobs can run; after MAC/decryption, each
  secret-bearing path requires the authenticated inner SHA to match both values
  before exporting payload values, checkout, or build use. Direct/manual runs
  without a valid payload fail closed; they are non-build diagnostics, not a
  public-debug or fixed-asset fallback.
- The authenticated payload binds the configured workflow repository to
  `github.repository`, so self-hosted forks remain supported without a hardcoded
  owner/name. GitHub receives the required tag selector; exact workflow contents
  readiness and run `head_sha` checks use the resolved SHA. `workflow_dispatch` does
  not provide atomic SHA binding between that selector and the executed workflow;
  verified annotated tags under an active immutable no-bypass ruleset are a
  compensating control, not an atomic guarantee. The two-layer SHA guard is
  defense in depth, not an atomic defense against a malicious workflow file;
  the verified tag and no-bypass ruleset remain required controls.

### Provider identity and asset contract

The provider supplies the source-tree/ref/commit identity, recursive submodule commits,
workflow identity, release identity, and asset metadata. This directory reports those
values; it does not independently verify them unless a separately trusted source proves
the expected value. A URL, provider-reported digest, local presence, or first-seen hash is
not by itself authenticity or release evidence. The listed asset names and digest fields
are a provider/project contract under test, not evidence that normal GitHub operation has
been verified.

The authenticated handoff must carry the exact required asset names and their expected
SHA-256 digests. The current required asset-name contract is:

- `windows-x64-release.zip`
- `usbmmidd_v2.zip`
- `rustdesk_printer_driver_v4-1.4.zip`
- `printer_driver_adapter.zip`

Missing names, missing digests, changed content, or unavailable source/submodule identity
must fail closed rather than being inferred from a provider listing.

### Reusable workflow details

Reusable workflow behavior, including any bridge workflow, is defined in the
RustDesk fork. The `rdgen` copy is reference material only and is not copied or
deployed by the current DeskForge path.

The provider artifact is downloaded through the provider API, then validated,
extracted, and published in local API storage. It is not sent to the API by a
runner callback and is not published to a public release. Frozen local file-queue
builders are manual/historical-only material, not a production fallback for this path.

Active fork uploads use the focused workflow retention contract (`retention-days: 7`
and fail-closed missing-file handling). That provider retention is not a completed
output TTL, release-retention policy, or release publication claim. The service-owned
handoff records exact source/workflow/run/artifact identity, output names, sizes,
hashes, and its publication digest; private `custom_.txt` content is not exported. A public
download may contain only a redacted `custom_.txt`; raw/private handoff contents, payload keys,
PATs, and other internal secret-bearing values are excluded.
These records are integrity/provenance metadata, not signatures or attestations.

Credentials — encrypted payload, decrypted inside the runner via GitHub Secret.

---

## Workflow ownership

- The rustdesk fork's `.github/workflows/` files are the sole executable source.
- `github-build/` contains reference/documentation material only; it is not a
  second workflow source or a deployment copy.
- The vendored `rdgen` workflows are historical/reference material and must not
  be copied into the active fork workflows.
- Change active build logic in the rustdesk fork. Keep this README aligned with
  the fork's actual workflow ownership and behavior.
- Active dispatch is tag-only and requires a guarded, provider-verified immutable
  workflow tag.
- An active immutable no-bypass ruleset protects that tag; mutable branch selectors
  are not an allowed fallback.
- Secret-bearing production dispatch remains gated until live provider evidence
  includes tag protection and ruleset administration.
- GitHub `workflow_dispatch` does not provide atomic SHA binding; the verified tag
  and ruleset are compensating controls rather than an atomic guarantee.
- Workflow-file presence and local focused checks do not prove provider behavior;
  live provider verification remains required.

---

## Security (REQUIRED for a public fork)

- `enc_payload` — authenticated `DFP1` AES-256-CBC + PBKDF2 + HMAC envelope, with
  `WORKFLOW_PAYLOAD_KEY` in GitHub Secrets.
- `RS_PUB_KEY` is a public key, not a secret.
- `SetWorkflowSecret` — button in admin UI (`Push to GitHub Secrets`) via `nacl/box.SealAnonymous`.

---

## Upstream updates

Follow
[PLAN.md §7](../PLAN.md#7-fork-maintenance-procedure-for-a-new-upstream-rustdesk-client-release)
for the upstream release process. Active workflow changes belong in the rustdesk
fork; this directory remains reference/documentation only.

### Historical fork bump log

- 2026-06-13: `setup-msbuild` v2→v3, `upload-artifact` → SHA-pinned v7.
