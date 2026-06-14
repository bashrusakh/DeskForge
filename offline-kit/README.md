# offline-kit - sovereign build kit

Implements **L1 (sources) + L3 (toolchain)** from [PLAN.md](../PLAN.md). It freezes
everything needed to build the Windows Flutter client offline **while upstream is still
available**, in case `rustdesk/rustdesk` and `rustdesk-org/*` disappear.

> ⚠️ **This is the most urgent item in the plan (§8.1)** and the only one with an external deadline.
> Upstream artifacts may disappear at any time. Run the freeze as early as possible.

## Files

| File | Purpose |
|---|---|
| `versions.env` | Single source of pins (Rust 1.75, Flutter 3.24.5, `vcpkg` baseline, URLs). If you change the client version, edit this file. |
| `freeze.sh` | Idempotent freeze script. Safe to rerun after interruption. |
| `artifacts/` | Output directory (not committed to git; see Storage below). |

## How to run

You need an environment with `git` + `cargo`. Easiest option: inside the already-running
`build-linux` container:

```bash
# copy offline-kit into the container and run it
docker cp offline-kit docker-build-linux-1:/offline-kit
docker exec -it docker-build-linux-1 bash -c "cd /offline-kit && bash freeze.sh"
```

Or in WSL/Linux with Rust installed:

```bash
cd offline-kit && bash freeze.sh
```

Individual stages can be run separately to save time/limits:

```bash
bash freeze.sh source        # git clone + bundle
bash freeze.sh vendor        # cargo vendor (freezes hbb_common + rustdesk-org/*)
bash freeze.sh engine        # custom Flutter engine
bash freeze.sh flutter_sdk   # Flutter SDK (win + linux)
bash freeze.sh vcpkg         # vcpkg checkout at the pinned baseline
bash freeze.sh rust          # Rust toolchain offline installer
```

For a **downstream fork**, override the source repository:

```bash
RUSTDESK_REPO=https://github.com/YOUR_ORG/rustdesk.git RUSTDESK_REF=1.4.7 bash freeze.sh
```

## What gets frozen (stages)

1. **source** - `git clone --recurse-submodules` at the tag + `git bundle`
   (portable archive with the full history of submodule `hbb_common`).
2. **vendor** - `cargo vendor`: pulls the submodule and ~20 git dependencies from
   `rustdesk-org/*` into `vendor/`. After this, the build no longer touches `rustdesk-org`.
3. **engine** - the custom RustDesk Flutter engine (`windows-x64-release.zip`) used by
   the workflow to replace the standard engine.
4. **flutter_sdk** - Flutter SDK 3.24.5 archives for Windows and Linux.
5. **vcpkg** - checkout of `vcpkg` at baseline `120deac...`. The **binary cache
   (`ffmpeg`/`hwcodec`, triplet `x64-windows-static`) is NOT built here**. That heavy MSVC step
   happens on the Windows host during `win-build` (PLAN.md §8.3).
6. **rust** - Rust 1.75 offline installer for the Windows host.

Each stage appends a line to `artifacts/MANIFEST.txt` with size and sha256.

## Storage of the completed kit

`artifacts/` is large (tens of GB) and **must not be committed to git**
(see root `.gitignore`, PLAN.md §8.6). Long-term storage options:

- **vendor/** - commit directly into the `rustdesk` fork (moves dependencies into the repo).
- **Large binaries** (engine, Flutter SDK, `vcpkg` cache) - upload as **release assets**
  of the `rustdesk` fork (`gh release upload`) instead of git history.
- **bundle** - backup of the sources; store outside GitHub (backup disk/S3).

## Offline build (how to use the kit later)

On the Windows builder (PLAN.md §8.3), without network:

```bash
# use sources from the bundle instead of clone:
git clone artifacts/rustdesk-1.4.7.bundle rustdesk
# vendor in place -> cargo reads from it:
cargo build --release --offline --locked
# vcpkg in asset-cache mode (X_VCPKG_ASSET_SOURCES) + binary cache
```

Exact Windows build commands are part of the next phase (§8.3, `Dockerfile.build-win`).
