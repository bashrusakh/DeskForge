# offline-kit — sovereign freeze tool

**Why:** if `rustdesk/rustdesk` gets deleted, `rustdesk-org/*` disappears, or
`crates.io`/Google becomes unreachable, building a custom client becomes impossible.
This kit freezes everything needed **while upstream is still alive**.

## Contents

| File                            | Purpose                                               |
| ------------------------------- | ----------------------------------------------------- |
| `freeze.sh`                       | Downloads sources, toolchain, dependencies            |
| `versions.env`                    | Versions of all components (Rust, Flutter, vcpkg...)  |
| `FORK-PROCEDURE.md`               | How to make a fork sovereign                          |
| `artifacts/`                      | Output of freeze.sh (5 GB, **not in git**)            |

## How it works

```
freeze.sh → offline-kit/artifacts/*  (locally, all 5 GB)
                ↓ upload (binaries only: engine, drivers)
         offline-assets-{tag}        (GitHub Release in rustdesk fork, ~100 MB)
                ↓ download
         GitHub Actions runner → build rustqs.exe
```

- **`offline-kit/`** (this directory) — **tool**: scripts and configs for freezing. Lightweight, in git.
- **`offline-assets-{tag}`** — **GitHub Release** in the `bashrusakh/rustdesk` fork.
  Heavy binaries (Flutter engine, usbmmidd, drivers) are uploaded there so CI downloads
  from our release, not from `rustdesk.com`.
- The rest (vendor 2.7 GB, Flutter SDK, Rust MSI, vcpkg) stays local — for standalone fallback.

## Freezing a new version

```bash
cd offline-kit
# Edit versions.env: RUSTDESK_REF, update toolchain versions for the new tag
bash freeze.sh source        # git clone + bundle
bash freeze.sh vendor        # cargo vendor
bash freeze.sh engine        # Flutter engine
# Other stages as needed
```

For downstream forks:
```bash
RUSTDESK_REPO=https://github.com/YOUR_ORG/rustdesk.git RUSTDESK_REF=1.5.0 bash freeze.sh
```

## Storage

`artifacts/` is in `.gitignore` — heavy files are not committed.
- `vendor/` — commit directly to the rustdesk fork or release asset.
- Binaries (engine, usbmmidd, driver) — upload to fork GitHub Release (`offline-assets-{tag}`).
- `bundles` — backup outside GitHub.
