# github-build — reference/docs for fork workflows

All executable client-build workflows live in the configured RustDesk fork.
This directory is reference and documentation only; it is not an executable
workflow source and does not contain deployable workflow copies.

The API dispatches the application-owned platform workflow in the configured
RustDesk fork. The fork's branch/ref and workflow files are provider-side
execution details; this directory does not define or deploy them.

The dispatch contract under test sends `return_run_details=true` and accepts only
an exact HTTP 200 run-details response containing the provider run identity. Standard
204 is intentionally unsupported because it does not provide an accepted exact run
correlation. Local fake-transport/static checks cover this contract; no normal GitHub
dispatch, poll, or download operation is verified.

The legacy compatibility ref `rustqs/workflows` is retained for read-only
compatibility; it is not the production workflow tag. Production approval and
dispatch require a guarded, provider-verified immutable `refs/tags/*` workflow
tag. At the provider-resolved immutable commit, the mapped Windows workflow must
declare `workflow_dispatch` and contain the exact
`# deskforge-workflow-identity-guard: v1` marker. The configured provider
repository resolves the selected tag and workflow identity.
The following ref labels describe the configured fork's roles; they are not a claim
that synchronized copies, protected tags, or a live provider run currently exist:

| Ref                       | Purpose                                                        |
| ------------------------- | -------------------------------------------------------------- |
| `master`                  | Fork default branch; not a dispatch selector                  |
| `rustqs/workflows`        | Legacy/read-only compatibility ref; not a production selector |
| `rustqs/master-workflows` | Fork-maintained workflow mirror/reference ref; not a selector |

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
[PLAN.md §7](../PLAN.md#7-historical-fork-maintenance-notes-for-a-new-upstream-rustdesk-client-release)
for the upstream release process. Active workflow changes belong in the rustdesk
fork; this directory remains reference/documentation only.

### Historical fork bump log

- 2026-06-13: `setup-msbuild` v2→v3, `upload-artifact` → SHA-pinned v7.
