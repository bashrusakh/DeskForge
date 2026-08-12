# offline-kit — frozen dependency-freeze tool (verification incomplete)

**Why:** if `rustdesk/rustdesk` gets deleted, `rustdesk-org/*` disappears, or
`crates.io`/Google becomes unreachable, building a custom client becomes impossible.
This kit attempts to capture build inputs **while upstream is still alive**. It is a
frozen operator tool, not a verified license bundle, sovereignty package, or proof of a
network-denied full client build.

## Contents

| File                            | Purpose                                               |
| ------------------------------- | ----------------------------------------------------- |
| `freeze.sh`                       | Downloads sources, toolchain, dependencies            |
| `versions.env`                    | Versions of all components (Rust, Flutter, vcpkg...)  |
| `verify.sh`                        | Read-only, fail-closed local manifest/pin verifier     |
| `license-inventory.sh`             | Deterministic license-evidence inventory; known gaps are reported |
| `FORK-PROCEDURE.md`               | Historical fork procedure; not sovereignty proof      |
| `artifacts/`                      | Local output of freeze.sh (**not in git**)             |

## How it works

```
freeze.sh → offline-kit/artifacts/*  (local operator storage)
                ↓ optional operator handoff (candidate engine/driver assets)
         offline-assets-{tag}        (provider-side asset naming convention)
                ↓ download
         GitHub Actions runner → build rustqs.exe
```

- **`offline-kit/`** (this directory) — **tool**: scripts and configs for freezing. Lightweight, in git.
- **`offline-assets-{tag}`** — a provider-side asset naming convention for candidate
  Flutter-engine/driver inputs. Current provider release availability is unverified.
- The rest (local vendor tree/archive, Flutter SDK, Rust MSI, and vcpkg) stays in
  operator storage. It is not a tracked repository vendor bundle or a supported
  standalone build claim.

## Freezing a new version

```bash
cd offline-kit
# Edit versions.env: RUSTDESK_REF, update toolchain versions for the new tag
bash freeze.sh source        # git clone + bundle
bash freeze.sh vendor        # cargo vendor
bash freeze.sh engine        # Flutter engine
# Other stages as needed
bash freeze.sh --verify      # no network; never rewrites MANIFEST

# Deterministic evidence report; gaps are reported, not guessed
bash license-inventory.sh > /tmp/deskforge-third-party-inventory.txt
```

An existing non-empty file is skipped only after its expected SHA-256 matches
`versions.env`. If a digest is genuinely unavailable, `--allow-tofu` is an
explicit manual exception for acquisition only; `--verify` still fails until a
digest is recorded. Mutable release URLs are never proof of identity. Source
verification requires both `RUSTDESK_REF` and the full `RUSTDESK_COMMIT`, and
checks a clean, non-shallow checkout, recursive submodules, the bundle, and the
checkout against that commit. Only freeze-generated `vendor/` and
`.cargo/config.vendor.toml` paths are allowed as source working-tree additions;
both are recorded in the manifest (`vendor/` by deterministic content hash and
the config by exact SHA-256). Verification also compares the complete vendor
tarball contents with the staged tree and requires the Cargo config to set
`net.offline = true` and replace sources only with the local `vendor/`
directory. `MANIFEST.txt` schema 2 and all
required pin records are mandatory; legacy/incompatible manifests are never
rewritten—use a new artifact directory and re-freeze. The TopMostWindow pin is
also compared with the active workflow checkout line in the frozen RustDesk source.
The vcpkg and TopMostWindow source trees are independently required to be
initialized, non-shallow, clean, and exactly at their configured commits before
freeze stages can mutate or download anything.

For downstream forks:
```bash
RUSTDESK_REPO=https://github.com/YOUR_ORG/rustdesk.git RUSTDESK_REF=1.5.0 bash freeze.sh
```

The current local verification boundary is incomplete: the Flutter engine archive
is missing, the existing manifest is legacy/incompatible with the required trusted
manifest contract, the printer checksum sidecar has no accepted digest, and license
evidence has gaps. The presence of `workflow-payload.key` also blocks verification;
its contents are never inspected or recorded. A complete network-denied full build,
signature/attestation verification, and release readiness are not claimed.

## Storage

`artifacts/` is in `.gitignore` — heavy files are not committed.
- `vendor/` — keep in local operator storage or publish only through a separately
  approved fork-maintenance process; it is not tracked in DeskForge.
- Candidate engine/driver assets — provider-side handoff/release inputs only; their
  availability and authenticity remain unverified here.
- `bundles` — backup outside the repository.

## Operational boundary

- Missing local engine/source/vendor files and unavailable provider assets are
  failures, not successful partial freezes. Existing files are preserved; the
  tool does not delete artifacts.
- `verify.sh` is read-only: it does not download, rewrite `MANIFEST.txt`, or
  inspect secret payload keys. `workflow-payload.key` and similar files must
  never be added to an inventory or manifest.
- The inventory reports only license evidence paths and is incomplete. A `GAP` means
  evidence is absent or intentionally outside the ignored local payload; no license is
  inferred, and this inventory is not a verified license bundle. No signatures are claimed;
  signature verification remains a gap. Missing engine, legacy manifest, empty printer
  digest, incomplete license evidence, and secret-bearing artifact blockers remain explicit.
