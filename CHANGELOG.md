# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased] - 2026-06-14

### Added (admin-ui UI rework foundation - PR #3, 2026-06-14)
- **Design tokens + theme system** (`admin-ui/src/styles/style.scss`, `src/store/app.js`): added light/dark tokens for surface/text/border/status colors, radius, shadows, and typography; theme mode `auto` / `light` / `dark` is stored in `localStorage` as `theme-mode` and applied through `html[data-theme]` + `html.dark` for Element Plus.
- **New UI primitives**:
  - `admin-ui/src/components/ui/ConnectionPulse.vue` for product-style status indication in shell/dashboard/auth/OAuth screens;
  - `admin-ui/src/components/ui/ThemeSwitch.vue` as a shared theme switcher;
  - `admin-ui/src/components/ui/CopyableText.vue` for copyable monospace ID/token text;
  - `admin-ui/src/components/ui/PageHeader.vue` and `PageSection.vue` for common page structure;
  - `admin-ui/src/components/ui/EmptyState.vue` and `LoadingState.vue` for shared empty/loading states;
  - `admin-ui/src/components/ui/DataTable.vue` as a shared `el-table` wrapper with loading, empty state, selection/index hooks, sort/row events, compact density, nested props, and horizontal scroll.
- **Shell/layout refresh**:
  - moved sidebar/header/menu/settings from hardcoded `#2d3a4b` / `#3f454b` to tokens;
  - removed the always-visible tags bar from the main layout;
  - sidebar brand now uses shared `--sidebar-brand-height`;
  - mobile navigation now opens through `el-drawer`, while desktop toggle still collapses the sidebar.
- **Dashboard refresh** (`admin-ui/src/views/index/index.vue`): added a Quick Connect panel with native `rustdesk://id`, web client `/webclient2/#/{id}`, and a link to devices.
- **Devices refresh** (`admin-ui/src/views/peer/index.vue`): added a permanent Status column with `ConnectionPulse`; online/offline is determined by `last_online_time < 60s`; ID uses `CopyableText`; action column reduced to `Connect` + `More`; pagination aligned via `PageSection`.
- **Monitoring visual pass**: login history, connection history, file transfers, and shared sessions now use shared `PageHeader` / `PageSection` layout without changing API/composable logic.
- **Server visual pass**: Server Commands, Server Config, and GitHub Build settings now use shared `PageHeader` / `PageSection`; advanced custom commands moved into `DangerZone` and require confirmation before `sendCmd`; terminal output now has readonly console styling, target hint, Copy/Clear controls, and empty-output placeholder.
- **Access visual pass**: Address Book entries, collections, share rules, and tags now use shared `PageHeader` / `PageSection`; address book IDs use `CopyableText`; wide action columns were reduced into `More` dropdowns without changing composables/API.
- **Users/Security visual pass**: Users, API Tokens, OAuth providers, Groups, and Device Groups now use shared `PageHeader` / `PageSection`; wide action sets were compacted into `More` dropdowns without changing CRUD/API behavior.
- **Client Builder/Profile visual pass**: Custom Client Builder and My Profile now use shared `PageHeader` / `PageSection`; build history pagination aligned with the rest of the UI.
- **My Workspace visual pass**: My Devices, My Address Book, My Address Book Collections, My Tags, My Shared Sessions, and My Login History now use shared page header/section layout; personal device and address book IDs use `CopyableText`.
- **404 refresh**: replaced the minimal `404` page with a tokenized empty-state screen linking back to dashboard.
- **Auth/OAuth visual refresh**: login, register, OAuth approve, and OAuth bind screens now use the token-based visual language and support theme switch + `ConnectionPulse`.
- **Custom Client runtime fix**: preset/upload handlers now return from `setup()` and are available to template buttons without changing backend/API contracts.
- **Monitoring filter pass**: Login History, Connection History, File Transfer History, and Shared Sessions now use the `FilterBar` primitive with collapsible panel, reset/clear, active filter count, and integrated action buttons.
- **DataTable pass**: `DataTable` wrapper applied to Users and Address Book pages with slot-based custom cells, loading state, empty state, and horizontal scroll.
- **AppDialog/AppDrawer/FormSection pass**: added `admin-ui/src/components/ui/AppDialog.vue`, `AppDrawer.vue`, and `FormSection.vue` shared primitives for unified dialog/drawer/form patterns.
- **CRUD dialog unification pass**: all remaining `el-dialog` usages migrated to `AppDialog`. Zero raw `el-dialog` remain in views.
- **DataTable migration COMPLETE**: all `el-table` usages migrated to `DataTable` across all view pages. The only remaining `el-table` is one nested inline table in `audit/fileList.vue` (directory file listing inside a cell) and the unused `my/address_book/indexv2.vue`.
- **Element Plus icons cleanup**: replaced deprecated `@element-plus/icons` imports and dependency with current `@element-plus/icons-vue`; build passes without the deprecation warning.
- **Legacy view cleanup**: removed unused `my/address_book/indexv2.vue` and replaced the nested file-info `el-table` in `audit/fileList.vue` with tokenized list markup, leaving no raw `el-table` usages in active views.
- **Review/verification**:
  - `ocr review` on the working tree found no high/medium issues; the only low nit about a magic number was fixed via `--sidebar-brand-height`;
  - `npm run build` passes; remaining output is limited to existing Vite/Rollup warnings about large chunks and `@vueuse` pure annotations.

### Known Follow-ups (admin-ui UI rework)
- Full i18n coverage for the new dashboard/auth hero copy.
- Tables, forms, dialogs/drawers, and CRUD screens still need further unification through the new primitives.
- Monitoring filters/toolbars: Connection, File Transfer, and Shared Sessions still need full FilterBar unification.

### Done (§8.9 Custom Preset - in practice 4 real UI/backend glue bugs, 2026-06-13)
No model expansion was needed: all fields were already stored in the `custom_json` text blob.
The work surfaced four real bugs that would have broken the actual GUI build flow through GitHub:

1. **`server_ip` vs `server`** (`api/http/controller/admin/custom_build.go`):
   the UI stored `server_ip`, while `tryGithubDispatch` only extracted `server`, so the value from
   the form never reached the workflow. Added fallback `server_ip` -> `server`.
2. **`custom_txt` was not being generated** (`buildCustomTxtFromForm`):
   the UI stores `permanent_password`, `hide_cm`, `deny_lan`, etc. separately rather than as an rdgen
   blob. When `custom_txt` is not explicitly provided, Go now assembles it automatically by mapping
   `permanent_password` -> `password`, `hide_cm` ->
   `verification-method: use-permanent-password` + `hide-connection-management: Y`,
   `deny_lan` -> `deny-lan-discovery: Y`, etc., then JSON-encodes and base64-encodes it.
3. **`Save as preset` created duplicates** (`api/service/custom_preset.go`):
   `CustomPresetService.Create` now performs an upsert on `(user_id, name)`, replacing by name as
   promised in §8.9.
4. **`loadPresetIntoForm` had an incomplete field list** (`admin-ui/src/views/custom-client/index.vue`):
   `app_icon_url`, `app_logo_url`, and `privacy_screen_url` were saved into the preset but not restored.
   Added them to the restore list.

### Done (§8.10 single-binary `rustqs.exe` - CLOSED, 2026-06-13)
- Reworked `rustqs-windows-min-test.yml` for full packaging:
  - reverted the L3 `BINARY_NAME` rename, because `libs/portable/generate.py` is hardcoded to look
    for `rustdesk.exe` inside `Release/`;
  - removed `mv Release ./rustdesk`; native deps (`usbmmidd`, printer driver + adapter) and the
    TopMost artifact are now downloaded directly into `flutter/build/windows/x64/runner/Release/`;
  - the L2-B step places `custom_.txt` in `Release/` **before** the packer runs, so it is packed in;
  - added new `L4 portable-pack` step:
    `cd libs/portable && pip3 install -r requirements.txt && python3 ./generate.py -f ../../{Release} -o . -e ../../{Release}/rustdesk.exe`;
  - final artifact comes from `./output/{appname}.exe`;
  - `actions/upload-artifact` now uploads `output` instead of the old `rustdesk` directory.
- Run [27462227115](https://github.com/bashrusakh/rustdesk/actions/runs/27462227115)
  succeeded in about 33 minutes. Final artifact: **one file `rustqs.exe`, 23.2 MB**
  instead of a small launcher plus a DLL folder. Exe metadata is `rustqs` and `custom_.txt`
  is packed inside the self-extracting exe.
- First attempt [27462157839](https://github.com/bashrusakh/rustdesk/actions/runs/27462157839)
  failed almost immediately with `bad decrypt` in `Resolve build config` because
  `WORKFLOW_PAYLOAD_KEY` in the fork had diverged from local
  `offline-kit/artifacts/workflow-payload.key`. Open inputs were used as a temporary bypass to
  validate §8.10. Remaining TODO: resync the key, either from the UI (`Push to GitHub Secrets`) or
  by replacing the local file.

### Fixed (Docker build)
- Added root **`.dockerignore`** to exclude `node_modules/`, `.git/`, `data/`, `rdgen-data/`,
  `rustdesk-cache/`, `**/target/`, `*.exe|*.dll|*.apk|*.msi`, and `offline-kit/artifacts/`
  from the build context. Without it, the context pulled in 155 MB of `node_modules` and host-only files.
- **Docker build fix**: production `docker/Dockerfile` now builds `admin-ui` from source inside a `node:20-bookworm` stage and no longer requires a pre-existing `admin-ui/dist/`; `.dockerignore` excludes host `node_modules/` and stale `admin-ui/dist/`.
- Reset the **admin password** to `admin123` via `apimain reset-admin-pwd` because the generated initial
  password from first-start logs was lost after restart.

### Added (GitHub Build integration - PLAN.md §8.8.5)
- Added **defaults in the GitHub Build form** (`admin-ui/src/views/server/github-build.vue`):
  when DB values are empty, it pre-fills `bashrusakh/rustdesk`, `rustqs-windows-min-test.yml`,
  and `rustqs/min-test`. The user can still change them before Save.
- Added **`DownloadByKey` endpoint** (`api/http/controller/admin/custom_build.go`):
  public (no api-token) endpoint `/api/admin/custom_build/public/download/:key` that returns a zip from
  `/rdgen-data/output/{id}/`. Capability URL is keyed by `download_key` (32 random chars), same as
  existing `DetailByKey`. Output filename format: `{app_name}-{YYYYMMDD-HHMMSS}.zip`.
- Moved **public routes out of `adg`** (`api/http/router/admin.go`): previously
  `aRPublic := rg.Group("/custom_build/public")` inherited `BackendUserAuth` from the parent group,
  so `detailByKey` and `download` returned 403 without a token. Now they are bound directly on root `g`.
- Added **NoCache middleware** (`api/http/middleware/nocache.go`) setting
  `Cache-Control: no-cache, no-store, must-revalidate`, `Pragma: no-cache`, `Expires: 0` on all
  `/api/admin/*` responses. Applied **before** `BackendUserAuth` so headers also reach 403 responses.
- Added **Axios cache-busting** (`admin-ui/src/utils/request.js`) adding `Cache-Control: no-cache`
  on all GET requests as extra protection against aggressive proxies/CDNs.
- `DispatchTest` now **polls until completion** (`api/http/controller/admin/github_build_config.go`):
  instead of returning an immediate `run_id`, it polls GitHub every 30 seconds until `status=completed`
  (max 90 min) and returns `{run_id, status, conclusion, ok, message}`. In Vue this uses a separate
  Axios request with a 95-minute timeout and shows `Build running...` -> success/failure.

### Fixed (GitHub fork cleanup)
- Removed **10 upstream workflows** from `bashrusakh/rustdesk@master`:
  `bridge.yml`, `ci.yml`, `clear-cache.yml`, `fdroid.yml`, `flutter-build.yml`, `flutter-ci.yml`,
  `flutter-nightly.yml`, `flutter-tag.yml`, `playground.yml`, `wf-cliprdr-ci.yml`.
  Kept only the needed ones: `rustqs-windows-min-test.yml` and `third-party-RustDeskTempTopMostWindow.yml`.
- Restored **`bridge.yml`** after cleanup because `rustqs-windows-min-test.yml` uses it as a reusable workflow.
  Without it, dispatch failed with HTTP 422 / workflow parse error. Restored from upstream `rustdesk/rustdesk@1.4.7`.

### Added (PLAN.md tasks)
- Added **§8.9 Custom Preset** task: extend the effective preset data with `server`, `key`, `custom_txt`,
  `logo`, and `icon`, while making UI `Save as preset` overwrite on name match.
- Added **§8.10 Single-binary `rustqs.exe`** task: replace the old `--skip-portable-pack` multi-file
  artifact with an upstream-style single binary and verify `custom_.txt` ends up inside the packed exe.

## [0.4.0] - 2026-06-11

### Changed (Architecture - Sovereign Build Strategy)
- Fully rewrote **`PLAN.md`** as the single source of truth. It now documents the sovereign build model
  (3 levels of independence: sources / build / toolchain), fork map (`rustdesk` + `hbb_common` +
  ~20 `rustdesk-org/*` repos through `vendor`), the 3-container architecture, and the offline kit with
  pinned versions (Rust 1.75, Flutter 3.24.5, LLVM 15.0.6, `vcpkg` baseline `120deac...`).
- Moved **Windows build** from Linux MinGW cross-compilation to a **native Windows builder** on a separate
  Windows server. Reason: current Flutter client cannot be cross-compiled from Linux; the MinGW path only
  reached the legacy Sciter UI and got blocked by an un-linkable `vcpkg` `libvpx.a` (ELF objects instead of COFF).
- Documented the 3-layer config injection model for `rustqs.exe`: server + key in `config.rs`, quick-support
  behavior in signed `custom.txt` (`allowCustom` patch), and branding through `sed` + portable-packer.

### Removed
- Removed `FEATURE_CUSTOM_CLIENT.md`, the outdated MinGW/Wine/NSIS build plan. The approach was rejected;
  valid parts were folded into `PLAN.md`.

### Added (Offline kit - PLAN.md §8.1)
- Added `offline-kit/versions.env` as the single source of pins: Rust 1.75, Flutter 3.24.5, LLVM 15.0.6,
  `vcpkg` baseline `120deac...`, custom Flutter engine URL, source repo for forkers.
- Added `offline-kit/freeze.sh`, an idempotent L1+L3 freeze script split by stages
  (`source`/`vendor`/`engine`/`flutter_sdk`/`vcpkg`/`rust`) and parameterized by env for downstream forks.
- Added `offline-kit/README.md` documenting how to run the freeze, how to store outputs
  (`vendor` in the fork, binaries in release assets), and how to build offline.
- Captured the full list of Windows `vcpkg` dependencies from `vcpkg.json`: `aom`, `libjpeg-turbo`,
  `opus`, `libvpx`, `libyuv`, `mfx-dispatch`, **`ffmpeg`** (`amf`/`nvcodec`/`qsv` for `hwcodec`).

### Done (Offline kit frozen)
- Ran `freeze.sh` on 2026-06-11 in `docker-build-linux-1`. Frozen into `rustdesk-cache` volume:
  1.4.7 bundle, `vendor` (2.7G, all `rustdesk-org/*` + `hbb_common`), Flutter engine,
  Flutter SDK for win+linux, `vcpkg` at baseline, Rust 1.75.0 MSI. Manifest with sha256 stored in `artifacts/MANIFEST.txt`.
- Corrected `RUST_VERSION` pin from `1.75` to `1.75.0` because the standalone MSI URL 404'd on the short version.

### Added (Windows-native builder - PLAN.md §8.3, designed, NOT tested)
- Added `docker/Dockerfile.build-win-native` with `servercore ltsc2022` + VS BuildTools (VCTools) +
  Flutter 3.24.5 + Rust 1.75 (msvc) + LLVM 15.0.6 + `vcpkg` baseline + `flutter_rust_bridge` 1.80.
- Added `docker/entrypoint-win-native.ps1` with job loop + 3 config injection layers +
  `build.py --portable --hwcodec --flutter --vram`.
- Added `docker/docker-compose.win.yml` for a Windows host with process isolation.
- Marked risky spots as `[VERIFY]` for real-host testing.
- Identified extra dependency `RustDeskTempTopMostWindow` and recorded it in PLAN §8.3a.

### Added (Autonomous session - §8.2 / §8.3a / §8.6)
- Extended `offline-kit/freeze.sh` with `thirdparty` stage: `RustDeskTempTopMostWindow` (sources,
  pin `53b548a`), `usbmmidd_v2.zip`, printer drivers. `offline-kit` became complete: 11 artifacts,
  5.0G. Fixed `record()` / manifest idempotence so partial runs do not overwrite the manifest.
- Added `offline-kit/FORK-PROCEDURE.md` documenting the sovereign fork procedure (levels A/B/C + acceptance).
- Added `.gitignore` to exclude secrets (private key `id_ed25519`, DB, `data/`, `.env`), build output,
  `node_modules`, `offline-kit/artifacts`, `.claude/`. Secret scan found no leaks in source.

### Changed (Windows builder: container -> native, owner decision)
- Final decision: build the Windows client **natively** on a separate Windows Server, no Docker.
- API <-> agent channel is an **SMB job queue folder**. Linux hosts Samba, Windows mounts it.
- Added `win-builder/setup.ps1` (toolchain, `-KitPath` support), `win-builder/agent.ps1`
  (SMB poller + 3 injection layers + `build.py`), `win-builder/README.md` (deployment + SMB).
- Removed the container-native files `docker/Dockerfile.build-win-native`, `entrypoint-win-native.ps1`,
  and `docker-compose.win.yml` as the native path became the single source of truth.

### Cleanup (owner, 2026-06-11 after §8.8.3a)
- Deleted all local Docker volumes and images. Test containers from the abandoned MinGW path
  (`build-win-test*`, `upbeat_carson`, `lucid_grothendieck`) disappeared, partially closing §8.7.
- Deleted `offline-kit` volume `rustdesk-cache`. A staging copy of 5 release assets remained in
  `offline-kit/artifacts/` (~62 MB). For standalone fallback, the kit can be re-frozen at any time.
  The GitHub track does not depend on that volume.

### Done (§8.8.3b(5) encrypted inputs + §8.8.5 scaffold, 2026-06-12)
**(5) Encrypted inputs - CLOSED**
- Generated 43-char `WORKFLOW_PAYLOAD_KEY` and stored it in fork GitHub Secrets.
  Important pitfall: `gh secret set ... --body -` via pipe adds a trailing newline under PowerShell,
  causing `bad decrypt` on the runner. Fix: use `--body $secret` without a pipe.
- Refactored the workflow to take `enc_payload` and resolve it through `Resolve build config`
  (OpenSSL AES-256-CBC + PBKDF2 + `jq` -> env vars), while still supporting open inputs as fallback.
- Migrated L1/L2/L3 steps from `inputs.X` to `RQS_*` env vars and masked sensitive values with `::add-mask::`.
- Verified with successful runs for both open-inputs and encrypted payload.

**§8.8.5 Go API - SCAFFOLD**
- Added `model/github_build_config.go`: singleton with `Token`, `PayloadKey`, and safe view.
- Added `service/github_build_config.go`: Get/Save, key generation, OpenSSL-compatible payload encryption,
  test connection, dispatch build, run status, artifact download.
- Added `controller/admin/github_build_config.go`: Get, Save, GenerateKey, Test, DispatchTest.
- Added AutoMigrate, router bind, and `DatabaseVersion` 267 -> 268.
- Added admin-ui page/API for GitHub Build settings.
- Design choice: PAT lives in admin UI / DB, not `.env`.

**SetWorkflowSecret one-click**
- Implemented with `golang.org/x/crypto/nacl/box.SealAnonymous` using the repo public key from
  `GET /actions/secrets/public-key`, then storing to `WORKFLOW_PAYLOAD_KEY`.
- Exposed via `/admin/github_build_config/sync_secret` plus `Push to GitHub Secrets` button in UI.

**Windows-job glue**
- `controller/admin/custom_build.go::submitBuild` now calls `tryGithubDispatch` for
  `platform=windows` when GitHub Build is configured, dispatches `enc_payload`, and starts a background
  `pollAndDownload` loop (30-second poll, 90-minute timeout).
- On success it downloads `rustdesk-min-test-windows.zip`, unpacks it, stores `{appname}.exe` + DLLs +
  `custom_.txt` into `/rdgen-data/output/{id}/`, and updates `CustomBuild.Status`.
- Falls back to file-queue mode for Linux/Android.

**Self review §8.8.5**
- Prevented panic in background goroutine `pollAndDownload` via `defer recover()`.
- Fixed unchecked `zf.Open()` error by moving extraction into helper `extractZipFile`.
- Removed fixed 30s `http.Client` timeout that broke 32 MB downloads.
- Replaced `context.WithTimeout(c, 60*1e9)` with `context.Background(), 60*time.Second`.

### Done (§8.8.3b full injection pipeline green, 2026-06-11/12)
- Sovereign binary assets: run [27352640159](https://github.com/bashrusakh/rustdesk/actions/runs/27352640159)
  switched `usbmmidd`/printer URLs to fork release `offline-assets-1.4.7`.
- L1 config.rs injection validated with noop run and real run.
- L3 branding validated in run [27359858171](https://github.com/bashrusakh/rustdesk/actions/runs/27359858171).
- L2 quick-support validated in run [27362132331](https://github.com/bashrusakh/rustdesk/actions/runs/27362132331),
  including `allowCustom` patch and `custom_.txt` payload.
- Exe rename polish fixed after initial path mistake: final run [27395862737](https://github.com/bashrusakh/rustdesk/actions/runs/27395862737)
  produced `rustqs.exe` with correct metadata and `custom_.txt` nearby.
- Uploaded additional optional rdgen patches to the fork for future use:
  `hidecm`, `removeSetupServerTip`, `removeNewVersionNotif`, `cycle_monitor`, `xoffline`,
  `privacyScreen`, `allowCustom.diff`.

### Done (§8.8.3a GitHub mini-test green, 2026-06-11)
- Created branch `rustqs/min-test` from tag 1.4.7 in `bashrusakh/rustdesk`.
- Added `rustqs-windows-min-test.yml`, a close copy of official `build-for-windows-flutter`
  plus `workflow_dispatch`, with the Flutter engine pulled from the fork release instead of `rustdesk-org`.
- First attempt failed at startup because of an extra input in the TopMost sub-workflow; fixed.
- Run [27341830418](https://github.com/bashrusakh/rustdesk/actions/runs/27341830418) completed:
  bridge about 6 min, topmost about 2 min, build about 37 min. Artifact `rustdesk-min-test-windows`
  confirmed the runner toolchain and fork release source path work.

### Done (§8.8 GitHub track - implementation start, 2026-06-11)
- Owner installed `gh` CLI (`bashrusakh`, `repo`/`workflow` scopes); GitHub API access confirmed.
- Forked `bashrusakh/rustdesk` and `hbb_common` publicly.
- Created fork release `offline-assets-1.4.7` with engine 63M, `usbmmidd`, printer driver + adapter,
  and generated `sha256sums`. Flutter SDK / Rust / `vcpkg` were not uploaded because GitHub runners install them directly.

### Changed (STRATEGY: GitHub-first, owner decision 2026-06-11)
- Split the two independence goals: from RustDesk upstream (real risk, solve now) and from GitHub
  platform (low risk, not priority). `rustqs.exe` is built through GitHub Actions in the rustdesk fork.
- Rewrote PLAN §§1/3/4/6 to reflect this and added §8.8 as the active track.
- Marked §§8.3/8.4 (standalone + SMB) as FALLBACK/frozen.
- Added `github-build/README.md` documenting URL repointing in forked `generator-windows.yml`,
  GitHub Secrets, and Go `workflow_dispatch` integration. Also documented that rdgen already contains
  `fetch-encrypted-secrets`, `ZIP_PASSWORD`, and `save_custom_client`.

### Added (win-builder)
- Added `win-builder/SERVER-SETUP.md`, a detailed Windows build server guide:
  OS choice (Server 2022 vs Windows 11 Pro), hardware sizing, provisioning options
  (Hyper-V VM / physical / cloud), long paths, antivirus exclusions, SMB, service agent,
  first end-to-end test, and security.

### Verified / Fixed (offline-kit)
- Proved **L1 sovereignty**: `cargo metadata --offline` on the vendored tree resolves all 1049 crates
  from `vendor` with no network access.
- Rebuilt and verified the **bundle** on full history (70M). Clone-back on tag 1.4.7 succeeded.
  Fixed the earlier shallow-clone defect (`remote did not send all necessary objects`).

### Notes
- Abandoned approaches are now documented in `PLAN.md` §9 so future agents do not repeat them.
- Ballast cleanup is delayed until the final phase (§8.7).
- `.gitignore` + secret scan are mandatory before the first public push (§8.6).
- Repository was still not under git at that point; `git init` was part of §8.6.

## [0.3.0] - 2026-06-09

### Added
- `PLAN.md` as a unified project roadmap for other agents
- `admin-ui/` forked from `lejianwen/rustdesk-api-web` (Vue 3 + Element Plus)
- New nav structure: Dashboard, Devices, Users, Groups, Address Book, Security, Monitoring,
  Custom Client, Server, My Profile
- New page stubs: Custom Client Builder, Server Config
- Dashboard route at `/dashboard/`
- i18n keys for all new nav sections (`en.json`)

### Changed
- Admin UI default locale: `zhCn` -> `en`
- Admin UI store default language: `'zh-CN'` -> `'en'`
- `en.json` `ChangeLang`: `Switch Language`
- Title: `Rustdesk API Admin` -> `RustDesk Server Admin`
- Router fully restructured into logical nav groups
- All admin routes moved under `/admin/*`
- My Profile routes cleaned up under `/my/*`
- Login redirect: `/` -> `/dashboard`
- `PLAN.md` updated with all completed phases

### Added (Phase 4 - Custom Client Builder Backend)
- `docker/Dockerfile.build-linux` for Linux `.rpm` + Android `.apk`
- `docker/Dockerfile.build-win` for Windows `.exe`/`.msi` via MinGW + NSIS cross-compiler
- `docker/entrypoint-linux.sh` job poller
- `docker/entrypoint-win.sh` job poller
- `api/model/custom_build.go` model
- `api/service/custom_build.go` CRUD service
- `api/http/request/admin/custom_build.go` form/query structs
- `api/http/controller/admin/custom_build.go` CRUD controller + build job submission by file tickets
- Routes under `/admin/custom_build/*` and public `detailByKey`
- `docker-compose.yml` services `build-linux` + `build-win` with shared `rdgen-data`
- `CustomBuild` added to AutoMigrate; `DatabaseVersion` 265 -> 266
- `.env.example` build agent section

### Added (Phase 5 - Dashboard API + UI)
- `GET /api/admin/dashboard/stats`
- `admin-ui/src/api/dashboard.js`
- Rewritten dashboard view with stat cards, quick actions, recent activity
- New dashboard i18n keys
- Route bind for dashboard stats

### Added (Phase 6 - Custom Client UI Page)
- Full build form in `admin-ui/src/views/custom-client/index.vue`
- `admin-ui/src/api/custom_client.js`
- 50+ i18n keys for build form fields and status labels

### Added (Phase 7 - Server Config UI Page)
- Rewritten `admin-ui/src/views/server/config.vue`
- New `AllConfig` endpoint in `api/http/controller/admin/config.go`
- `GET /config/all` route under auth
- `admin-ui/src/api/config.js` `all()`

### Changed (Phase 8 - Polish)
- Chinese language names in app store normalized
- `peer/index.vue` warning translated to `Not implemented`
- `rustdesk/blocklist.vue` and `blacklist.vue` tips translated to English
- Removed commented Chinese labels in address book views

### Changed (Phase 9 - Dockerfile finalize)
- Dockerfile stage 3 uses local `admin-ui/` instead of `git clone`

### Added/Fixed (Phase 10 - Integration verification)
- Added missing i18n keys: `Download`, `Reset`
- Code review of Go patterns, frontend/backend route alignment, permissions, and i18n found no issues

## [0.2.0] - 2026-06-08

### Fixed
- Docker build: `Cargo.lock` version 4 requires Rust 1.88+
- Docker build: missing `go.sum`, generated with `go mod tidy`
- Docker build: `sqlx` requires `DATABASE_URL` for compile-time query checks

### Changed
- Dockerfile Rust base image updated to `rust:bookworm`
- Dockerfile deletes `Cargo.lock` before build to regenerate compatible versions
- Dockerfile creates a dummy SQLite DB for `sqlx` macro compilation

## [0.1.0] - 2026-06-08

### Added
- Unified Docker image with Rust server + Go API + Web Admin
- Multi-stage Dockerfile (Rust, Go, Node.js, `s6-overlay`)
- Single `docker-compose.yml` for easy deployment
- Web Admin panel at `/admin/`
- Web Client for browser-based remote desktop
- JWT authentication
- Mandatory login support (`MUST_LOGIN`)
- OAuth2/OIDC support
- LDAP authentication
- User/Group/Device management
- Address Book
- Audit logs
- Captcha and ban system
- Swagger API documentation
- `CHANGELOG.md`

### Changed
- Admin panel route changed from `/_admin/` to `/admin/`
- All Chinese text removed from source code (`api/`)
- Documentation made English-only

### Removed
- Chinese README files (`api/README.md`)
- Chinese comments from source code
