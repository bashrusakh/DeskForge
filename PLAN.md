# PLAN.md — DeskForge: Single Source of Truth

> Last updated: 2026-06-28
> Related: [CHANGELOG.md](CHANGELOG.md) · [BUGS.md](BUGS.md) · [CONTRIBUTING.md](CONTRIBUTING.md)

---

## 0. Project goal

Self-hosted RustDesk server (hbbs/hbbr + API + admin panel) + **custom client builder**
that works without `rustdesk/rustdesk`, `rustdesk-org/*`, or `rustdesk.com`.

**Active client build path:** GitHub Actions in the rustdesk fork. `win-builder/`
and `linux-build` are frozen fallbacks.

GitHub-first because:
- free Windows runners
- fork is ready, min-test is green
- standalone requires a separate Windows Server, not deployed

---

## 1. Repository map

```
bashrusakh/
├── DeskForge              ← this repo (server, api, admin, docker)
├── rustdesk               ← fork of rustdesk/rustdesk, tag 1.4.7 → 1.4.8
│   ├── vendor/            ← cargo vendor (L1, ~20 rustdesk-org deps)
│   ├── .github/workflows/ ← rustqs-windows-min-test.yml, rustqs-linux.yml, rustqs-android.yml
│   └── releases/          ← offline-assets-1.4.7 (engine, usbmmidd, drivers)
├── hbb_common             ← fork of rustdesk/hbb_common (required submodule)
└── rustdesk-deps/         ← archive of ~20 rustdesk-org repos (L1 backup)
```

**Current versions:** fork at 1.4.7 (tag), workflow updated for 1.4.8 (chore/bump-client-1.4.8).

---

## 2. Architecture

### Active path (GitHub Actions)

```
admin-ui (Custom Client form)
   ↓ POST /custom_build
Go API (DeskForge)
   ↓ workflow_dispatch + enc_payload (AES-256-CBC + PBKDF2)
GitHub Actions [rustdesk fork, windows-2022]
   ↓ L1: config.rs (server + key)
   ↓ L2: custom_.txt (permanent password, allowCustom patch)
   ↓ L3: branding (rustqs, portable-packer)
   ↓ POST /api/save_custom_client (encrypted)
Go API → /rdgen-data/output/{id}/ → admin-ui Download
```

**Security:** password never published — `enc_payload`, decrypted inside runner via
GitHub Secret `WORKFLOW_PAYLOAD_KEY`. Binary goes to your server, not a public release.

### Fallback (frozen, do not deploy)

```
admin-ui → Go API → jobs/{id}.json → SMB share → standalone Windows builder
                                                   or Docker linux-build
```

Not active: `win-builder/` untested (no Windows host), `build-linux` behind `--profile fallback`.

---

## 3. Component status

| Component                      | Status         | Notes                                |
| ------------------------------ | -------------- | ------------------------------------ |
| hbbs/hbbr (Rust)               | ✅ running     | ports 21114-21118                    |
| Go API                         | ✅ running     | users, address book, OAuth, LDAP, audit |
| Admin UI (Vue 3)               | ✅ running     | 16 pages, 3 locales, DataTable, FilterBar |
| GitHub build (Windows)         | ✅ active      | min-test green, 3 layers, encryption |
| GitHub build (Linux)           | 🟡 draft       | workflow exists, not CI-validated    |
| GitHub build (Android)         | 🟡 draft       | workflow exists, not CI-validated    |
| win-builder standalone         | ❄️ frozen      | do not deploy, no Windows host       |
| linux-build (Docker)           | ❄️ frozen      | manual fallback, `--profile fallback`  |
| offline-kit                    | ❄️ frozen      | re-freeze when client version changes |

---

## 4. Three injection layers for custom client

| Layer | What we change        | Mechanism                                                    |
| ----- | --------------------- | ------------------------------------------------------------ |
| L1    | server + key          | `sed` in `libs/hbb_common/src/config.rs` — `RENDEZVOUS_SERVERS`, `RS_PUB_KEY` |
| L2    | quick-support password| `custom_.txt` (signature checked — `allowCustom.py` patch removes check) |
| L3    | branding (rustqs)     | `Cargo.toml`, `Runner.rc`, portable-packer (`libs/portable/generate.py`) |

Full recipe: `rdgen/.github/workflows/generator-windows.yml` (vendored reference).

---

## 5. ✅ Completed milestones

- [x] Forks of rustdesk + hbb_common (1.4.7)
- [x] Offline kit: L1+L3, 11 artifacts, 5 GB (frozen)
- [x] GitHub min-test Windows: green, ~33 min, single-binary rustqs.exe
- [x] Go API: workflow_dispatch, poll, download, capability-URL TTL
- [x] Admin UI redesign: design tokens, DataTable, AppDialog, FilterBar, 16 pages
- [x] Security: encrypted-at-rest (AES-GCM), OAuth delete guard, audit, TTL
- [x] Rust server: atomic blocklist, aur-fix, JWT
- [x] Database: ~272 migrations, SQLite/MySQL/PostgreSQL

---

## 6. Open roadmap

- [ ] **Linux + Android GitHub workflows** — CI validation + platform picker in UI
- [ ] **Full client rebrand** — About, URLs, icons — in workflow, not in the fork
- [ ] **Smoke test** for built binary (`--version`)
- [ ] **Ballast cleanup** — remove MinGW leftovers, test containers

---

## 7. Workflow: new upstream rustdesk-client release

When `rustdesk/rustdesk` publishes a new tag (e.g. 1.5.0), follow these steps:

### 7.1. Fork sync

```bash
# In bashrusakh/rustdesk:
git fetch upstream --tags
git checkout v1.5.0
git push origin v1.5.0

# In bashrusakh/hbb_common:
git fetch upstream --tags
git checkout v1.5.0   # or matching tag
git push origin v1.5.0
```

### 7.2. Repoint submodule

In the rustdesk fork:
```bash
# .gitmodules → url = https://github.com/bashrusakh/hbb_common.git, branch = v1.5.0
git submodule sync && git submodule update --init --recursive
git add .gitmodules libs/hbb_common
git commit -m "chore: point hbb_common to v1.5.0"
git push origin v1.5.0
```

### 7.3. Update vendor

```bash
# On a machine with Rust:
cargo vendor vendor/
git add vendor/ && git commit -m "chore: vendor deps for v1.5.0"
git push origin v1.5.0
```

Or if vendor is too heavy — upload `vendor-1.5.0.tar.gz` as a release asset.

### 7.4. Update offline-kit

```bash
cd DeskForge/offline-kit
# versions.env: RUSTDESK_REF=v1.5.0; check MSRV, Flutter, vcpkg baseline
bash freeze.sh source vendor engine
```

### 7.5. Update offline-assets release

```bash
# Upload engine/usbmmidd/driver to the fork:
gh release create offline-assets-1.5.0 --repo bashrusakh/rustdesk \
    --title "Offline build assets (1.5.0)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

> **Note:** After publishing the release, version `1.5.0` will automatically appear in the admin UI
> (the `GET /api/admin/custom_build/versions` endpoint fetches fork releases tagged
> `offline-assets-*`). No hardcoded values in UI or YAML need to be changed.

### 7.6. Adapt workflow

Compare upstream `build-for-windows-flutter` with `rustqs-windows-min-test.yml`:
- New system dependencies?
- Changed `build.py` flags?
- Changed `config.rs` / `custom_.txt` format?

Port changes to the fork workflow.

> **Important:** `bridge.yml` must stay **without `inputs.version`** — same as upstream.
> Bridge and build must work from the same code (the fork). Do not add `repository:` to checkout.

### 7.7. Deploy workflows to fork (3 branches)

Workflow files live on three fork branches. After changes, update all:

```bash
# 1) rustqs/min-test — execution (all dispatches go here)
git checkout rustqs/min-test
cp /path/to/DeskForge/github-build/rustqs-*.yml .github/workflows/
cp /path/to/DeskForge/rdgen/.github/workflows/bridge.yml .github/workflows/
git add .github/workflows/
git commit -m "feat: update rustqs-* workflows for v1.5.0"
git push origin rustqs/min-test

# 2) master — API discovery (workflow must exist on default branch)
git checkout master
cp /path/to/DeskForge/github-build/rustqs-*.yml .github/workflows/
cp /path/to/DeskForge/rdgen/.github/workflows/bridge.yml .github/workflows/
git add .github/workflows/
git commit -m "feat: update rustqs-* workflows for v1.5.0"
git push origin master

# 3) rustqs/master-workflows — mirror (backup for upstream sync)
git checkout rustqs/master-workflows
cp /path/to/DeskForge/github-build/rustqs-*.yml .github/workflows/
cp /path/to/DeskForge/rdgen/.github/workflows/bridge.yml .github/workflows/
git add .github/workflows/
git commit -m "feat: update rustqs-* workflows for v1.5.0"
git push origin rustqs/master-workflows
```

### 7.8. Verify

- [ ] GitHub Actions run ✅ (no `startup_failure`)
- [ ] `VERSION` in logs matches the version selected in admin UI
- [ ] Binary arrived at the server
- [ ] `rustqs.exe`, ~23 MB, `custom_.txt` packed inside
- [ ] Smoke test on clean Windows

### 7.9. Update DeskForge reference

- [ ] `offline-kit/versions.env` — new `RUSTDESK_REF`
- [ ] `offline-kit/FORK-PROCEDURE.md` — update versions in examples
- [ ] `PLAN.md` — update current tag in §1
- [ ] `github-build/README.md` — update patch URLs if changed

---

## 8. What are offline-kit and offline-assets

| Entity                  | What it is                                           | Where stored                          |
| ----------------------- | ---------------------------------------------------- | ------------------------------------- |
| `offline-kit/`          | Scripts (`freeze.sh`) + config (`versions.env`)      | In git, in this repo                  |
| `offline-kit/artifacts/`| Output of freeze.sh: vendor.tar.gz, engine, SDK, MSI | Locally, **not in git**               |
| `offline-assets-{tag}`  | GitHub Release with binaries for CI                  | GitHub Releases of the rustdesk fork  |

**Why:** without this insurance, if `rustdesk/rustdesk` gets deleted or `rustdesk.com`
goes down, building a custom client becomes impossible. The kit freezes everything
while upstream is still available.

---

## 9. Abandoned (do not repeat)

| Approach                     | Why dead                                           |
| ---------------------------- | -------------------------------------------------- |
| MinGW cross-compile Flutter  | Flutter Windows requires MSVC, cannot cross-compile|
| `windows-x86` target         | 32-bit not supported in 2026                      |
| standalone win-builder       | frozen — GitHub-first                              |

---

## 10. Reference facts

- `custom.json` in `flutter/lib/` — no-op, not read by the code.
- Real mechanism: `custom_.txt` + `config.rs`.
- `read_custom_client` checks signature — `allowCustom.py` patch removes the check.
- Patches in `rdgen/.github/patches/`: allowCustom, hidecm, removeSetupServerTip,
  removeNewVersionNotif, cycle_monitor, xoffline, privacyScreen.
