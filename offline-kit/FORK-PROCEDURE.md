# FORK-PROCEDURE — how to make a fork sovereign

> **FROZEN** — procedure was completed for 1.4.7/1.4.8. Below is reference for a new version
> or for downstream forkers. Commands are executed by the repo owner.

---

## Level A — fork + vendor (minimum to survive upstream deletion)

### A1. Fork rustdesk + hbb_common

```bash
gh repo fork rustdesk/rustdesk   --org YOUR_ORG --fork-name rustdesk   --clone=false
gh repo fork rustdesk/hbb_common --org YOUR_ORG --fork-name hbb_common --clone=false
```

### A2. Vendor into the fork

From offline-kit:
```bash
git clone artifacts/rustdesk-1.4.8.bundle rustdesk-fork
cd rustdesk-fork && git remote set-url origin https://github.com/YOUR_ORG/rustdesk.git
git checkout -b release/1.4.8 1.4.8 && git submodule update --init --recursive
tar -xf ../artifacts/vendor-1.4.8.tar.gz
# .cargo/config.toml → source replacement to vendor/
git add vendor .cargo/config.toml
git commit -m "chore: freeze vendored deps 1.4.8"
git push origin release/1.4.8
```

`vendor/` is heavy — alternatively upload `vendor-{tag}.tar.gz` as a release asset.

### A3. Point versions.env to your fork

```env
RUSTDESK_REPO="https://github.com/YOUR_ORG/rustdesk.git"
RUSTDESK_REF="1.4.8"
```

---

## Level B — full sovereignty (binaries in releases)

### B1. What to upload

From `offline-kit/artifacts/`:

| Artifact                         | Why                                |
| -------------------------------- | ---------------------------------- |
| `windows-x64-release.zip`        | Custom Flutter engine              |
| `usbmmidd_v2.zip`                | Virtual display driver             |
| `rustdesk_printer_driver_v4-*.zip`| Printer driver                    |
| `printer_driver_adapter.zip`     | Printer adapter                    |
| `vendor-*.tar.gz`                | (optional, if not in git)          |

### B2. Commands

```bash
gh release create offline-assets-1.4.8 --repo YOUR_ORG/rustdesk \
    --title "Offline build assets (1.4.8)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

### B3. Archive dependency forks (optional, L1 backup)

```bash
for r in RustDeskTempTopMostWindow; do
  gh repo fork rustdesk-org/$r --org YOUR_ORG --clone=false
done
```

---

## Level C — downstream forker

Someone forks **your** DeskForge → changes one line:
```env
RUSTDESK_REPO="https://github.com/THEIR_ORG/rustdesk.git"
```
→ their GUI builds from their fork. Upstream is not involved.

---

## Sovereignty verification

- [ ] `YOUR_ORG/rustdesk` with vendor + `.cargo/config.toml`
- [ ] `YOUR_ORG/hbb_common` (submodule)
- [ ] Release `offline-assets-{tag}` with binaries
- [ ] `versions.env` → your fork
- [ ] `cargo build --offline` passes without `github.com/rustdesk*`
