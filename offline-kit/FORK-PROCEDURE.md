# FORK-PROCEDURE - sovereign fork (PLAN.md §8.2)

How to turn the frozen [offline-kit](README.md) into a **permanent self-contained fork**
that can still build the client even if upstream disappears, and how downstream forkers
repeat the same setup with their own repository.

> All `gh`/`git push` commands are executed by the **owner** (their GitHub account is
> outward-facing). This file documents the exact sequence; it is not automated.
> Assumes `gh` (GitHub CLI) is installed and authenticated, and `offline-kit` has already
> been frozen (`rustdesk-cache:/rustdesk-cache/offline-kit/artifacts/`).

---

## Level A - minimum sovereignty (fork + vendor)

Enough to survive upstream shutdown and build from your own fork.

### A1. Fork the client and submodule into your organization

```bash
gh repo fork rustdesk/rustdesk    --org YOUR_ORG --fork-name rustdesk    --clone=false
gh repo fork rustdesk/hbb_common  --org YOUR_ORG --fork-name hbb_common  --clone=false
```

### A2. Import the frozen vendor tree into the rustdesk fork

`vendor/` (2.7 GB, all ~20 `rustdesk-org/*` deps + `hbb_common`) is already frozen.
Put it into the fork so the build never talks to `rustdesk-org` again.

```bash
# extract tagged sources from the bundle + unpack vendor
git clone artifacts/rustdesk-1.4.7.bundle rustdesk-fork
cd rustdesk-fork && git remote set-url origin https://github.com/YOUR_ORG/rustdesk.git
git checkout 1.4.7 && git submodule update --init --recursive
tar -xf ../artifacts/vendor-1.4.7.tar.gz          # -> vendor/
# point cargo at vendored sources:
mkdir -p .cargo
cat > .cargo/config.toml <<'EOF'
[source.crates-io]
replace-with = "vendored-sources"
[source.vendored-sources]
directory = "vendor"
EOF
git add vendor .cargo/config.toml
git commit -m "Freeze vendored deps (sovereign offline build, tag 1.4.7)"
git push origin 1.4.7    # or a branch, e.g. sovereign/1.4.7
```

> ⚠️ `vendor/` is heavy. If you do not want to grow git history, upload
> `vendor-1.4.7.tar.gz` as a release asset instead (see B2) and unpack it at build time.

### A3. Point build agents at your fork

In `offline-kit/versions.env` and in the build-win image environment (`docker-compose.win.yml`):

```
RUSTDESK_REPO="https://github.com/YOUR_ORG/rustdesk.git"
RUSTDESK_REF="1.4.7"
```

Done: the build now uses your fork, not upstream.

---

## Level B - full sovereignty (binary artifacts in releases)

Besides sources, the Windows build also needs binary artifacts that can disappear.
Upload them into releases of your fork.

### B1. What to upload (everything is already in offline-kit)

| Artifact | File in kit | Why |
|---|---|---|
| Flutter engine (custom) | `windows-x64-release.zip` | replaces the standard engine |
| `usbmmidd_v2` | `usbmmidd_v2.zip` | virtual display |
| printer driver | `rustdesk_printer_driver_v4-1.4.zip` | printing |
| printer adapter | `printer_driver_adapter.zip` | printing |
| vendor (optional) | `vendor-1.4.7.tar.gz` | if you do not commit it to git |

### B2. Commands

```bash
gh release create offline-assets-1.4.7 --repo YOUR_ORG/rustdesk \
    --title "Offline build assets (1.4.7)" --notes "Frozen $(date +%F)" \
    artifacts/windows-x64-release.zip \
    artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip \
    artifacts/printer_driver_adapter.zip \
    artifacts/vendor-1.4.7.tar.gz
```

The build agent should fetch them from this release (fixed tag), not from `rustdesk.com`.

### B3. Archive forks of dependencies (optional, L1 backup)

For extra safety, fork the source repos too, in case you need to re-vendor for a newer version:

```bash
for r in RustDeskTempTopMostWindow; do gh repo fork rustdesk-org/$r --org YOUR_ORG --clone=false; done
# plus ~20 rustdesk-org/* repos from Cargo.toml (see PLAN.md §2) if desired
```

`RustDeskTempTopMostWindow` is already frozen as sources in
`artifacts/RustDeskTempTopMostWindow.bundle` (pinned commit `53b548a...`).

---

## Level C - downstream forker repeats after you

Someone forks **your** `full_Server` and wants to build from **their own** rustdesk fork:

1. They fork `full_Server` (this repo) and `YOUR_ORG/rustdesk` -> `THEIR_ORG/rustdesk`.
2. In their own `full_Server`, they change:
   ```
   RUSTDESK_REPO="https://github.com/THEIR_ORG/rustdesk.git"
   ```
   (one line in `versions.env` + the env in `docker-compose.win.yml`).
3. They rebuild the build-win image -> their GUI builds from their fork.

The original `rustdesk/rustdesk` is no longer part of this chain. That is the entire point of §0/§7.

---

## Sovereignty verification (acceptance)

The fork is "permanent" if all of the following are true:

- [ ] `YOUR_ORG/rustdesk` at 1.4.7 with `vendor/` (or vendor in a release) + `.cargo/config.toml`.
- [ ] `YOUR_ORG/hbb_common` forked (submodule).
- [ ] Binary artifacts are present in fork releases (engine, `usbmmidd`, printer).
- [ ] `versions.env` points to your fork.
- [ ] A test build with `--offline` succeeds without touching `github.com/rustdesk*`.
