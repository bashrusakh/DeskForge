# github-build: how client builds run

Documentation only. All executable client-build workflows live in the configured RustDesk
fork (`.github/workflows/`); nothing here is deployed. Ownership rules:
[docs/FORKS-AND-PINS.md](../docs/FORKS-AND-PINS.md#1-topology).

## How a build runs

1. The API maps the selected platform to a workflow in the fork and dispatches it with an
   encrypted payload (`enc_payload`).
2. The provider returns the exact run; the API stores the run ID, polls it and downloads
   the artifact through the provider API.
3. The API validates and extracts the artifact and publishes it in local storage. The runner
   never calls back to the API, and nothing goes to a public release.

Diagram and full contract: [PLAN.md](../PLAN.md#2-architecture).

| Platform | Workflow in the fork | Status |
| --- | --- | --- |
| Windows | `.github/workflows/rustqs-windows.yml` | enabled |
| Linux | `.github/workflows/rustqs-linux.yml` | mapped, disabled in the API |
| Android | `.github/workflows/rustqs-android.yml` | mapped, disabled in the API |

## Refs and tags

Dispatch uses only an approved, provider-verified annotated tag `refs/tags/workflow-*`
protected by a no-bypass ruleset (approval rules: [api/README.md](../api/README.md#workflow-approval)).
Branches are never dispatch selectors: `master` is the fork's default branch, and
`rustqs/workflows` is only a backup copy of `master`.

## Version flow

The `version` field in the admin UI selects the client source and assets for the build.

- The UI loads versions from `GET /api/admin/custom_build/versions`. The API lists the
  configured repository's `offline-assets-*` releases that have a matching source tag and the
  required assets. There is no hardcoded repository or fallback list; if GitHub is
  unavailable the list is empty. The result is cached for about 5 minutes.
- The resolved version travels inside `enc_payload`; the workflow accepts no separate manual version.
- The payload binds the configured workflow repository to `github.repository`, so forks
  with other owners work without code changes.
- Two SHA checks guard secret-bearing jobs: the public outer `workflow_sha` must equal
  `github.sha`, and after decryption the inner SHA must match both. Direct or manual runs
  without a valid payload fail as non-build diagnostics. This is defense in depth, not an
  atomic guarantee (see the approval rules above).

## Required release assets

The handoff must carry these exact names with their SHA-256 digests; a missing name, digest
or source identity fails the build instead of being inferred:

- `windows-x64-release.zip`
- `usbmmidd_v2.zip`
- `rustdesk_printer_driver_v4-1.4.zip`
- `printer_driver_adapter.zip`

Provider-reported digests and URLs are not proof of authenticity by themselves.

## Output and retention

Fork uploads keep artifacts for 7 days (`retention-days: 7`, missing files fail the step).
The API handoff records source, workflow, run and artifact identity, file names, sizes,
hashes and the publication digest. The private `custom_.txt` is never exported; the public
download may contain only a redacted copy. These are integrity records, not signatures.

## Security

- `enc_payload` is a `DFP1` envelope (AES-256-CBC + PBKDF2 + HMAC); the key
  `WORKFLOW_PAYLOAD_KEY` is stored in GitHub Secrets and the payload is decrypted inside the runner.
- The admin UI button "Push to GitHub Secrets" writes secrets via `nacl/box.SealAnonymous`.
- `RS_PUB_KEY` is a public key, not a secret.

## Upstream updates

Follow [PLAN.md §7](../PLAN.md#7-fork-maintenance-procedure-for-a-new-upstream-rustdesk-client-release).
Change active build logic in the fork, not here.
