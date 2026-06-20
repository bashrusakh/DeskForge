# PLAN.md - DeskForge: Single Source of Truth

> **This file is the only authoritative project plan.**
> Other agents: read it first. If you find another `*.md` build plan that conflicts with this
> file, it is outdated. Trust `PLAN.md` only.
> Related file: [CHANGELOG.md](CHANGELOG.md) - chronological change log.
>
> Last updated: 2026-06-20.
>
> Companion file: [BUGS.md](BUGS.md) — open issues in the Custom Client Builder workflow.

---

## 0. Project goal

A self-hosted, sovereign RustDesk server + web admin UI + **custom client builder**
that keeps working **even if upstream `rustdesk/rustdesk` and `rustdesk-org/*`
are closed or deleted completely**.

The end goal is to preserve the last free reproducible client version and be able to build
a branded one-file quick-support binary (`rustqs.exe`) with the server, key, and permanent
password already baked in.

Additionally, the project is published publicly on GitHub, and **anyone can fork it,
point it at their own source repo, and build the client through their own GUI** without
depending on the original RustDesk.

---

## 1. Two kinds of independence + three sovereignty levels

The key distinction that drives the strategy:

- **Independence from RustDesk upstream** (code, dependencies, engine) is the **real risk**.
  This is the reason the whole project exists. Address it now: fork + vendor + offline kit.
- **Independence from GitHub as the build platform** is a **low risk**.
  It is implemented, but it is not the priority.

**Owner decision (2026-06-11): GitHub-first.** Build `rustqs.exe` through GitHub Actions
in the rustdesk fork (fast, free Windows runners). The standalone Windows builder is fully
prepared but **frozen as a fallback** and should only be activated if GitHub becomes unacceptable.
At the same time, the GitHub path is made **sovereign from rustdesk** (see §8.8): builds run
from the fork and fetch artifacts from fork releases, not from `rustdesk.com`.

**Owner decision (2026-06-20): same model for Linux and Android.** The existing Docker
`build-linux` agent (and the abandoned MinGW `build-win` agent) become **manual fallbacks**.
Default route for `platform=linux` / `platform=android` will be a GitHub workflow mirroring
the windows-min-test pipeline (see §8.14). `windows-x86` is dropped as a supported target —
we don't ship 32-bit clients in 2026 (see §8.15). The server side (`hbbs`/`hbbr` + Go API
+ admin-ui) stays as it is.

| Level | Independence from | How it is achieved | Status |
|---|---|---|---|
| **L1. Sources** | deletion of `rustdesk/rustdesk` and `rustdesk-org/*` | fork + `cargo vendor` | yes, frozen; L1 verified offline |
| **L2. Build** | GitHub Actions / Windows runners | standalone Docker or native agent | prepared, frozen as fallback |
| **L3. Toolchain** | disappearance of `vcpkg` / Flutter / Rust | pinned offline kit | yes, frozen (5.0G, 11 artifacts) |

Dependencies strategy: **vendor + fork at the same time**. `vendor/` is the working mechanism,
forks of repositories are the archival backup.

---

## 2. Repository map: what to fork

```
YOUR GITHUB ORGANIZATION
|
|-- DeskForge                    <- main project (this repository)
|     `-- admin-ui, api, server, docker, rdgen...
|
|-- rustdesk                     <- fork of rustdesk/rustdesk, pinned to tag 1.4.7
|     |-- vendor/                <- cargo vendor, committed (L1 working path)
|     `-- releases/              <- Flutter engine zip as release asset
|
|-- hbb_common                   <- fork of rustdesk/hbb_common, REQUIRED submodule
|
`-- rustdesk-deps/               <- archival L1 backup, ~20 repos
      `-- magnum-opus, rdev, kcp-sys, rust-sciter, arboard, hwcodec,
          parity-tokio-ipc, confy, sysinfo, machine-uid, ...
```

**Facts verified against 1.4.7 sources:**
- Submodule: `rustdesk/hbb_common` (nothing builds without it).
- About 20 git dependencies under `rustdesk-org/*` in `Cargo.toml` and `libs/*/Cargo.toml`.
- The custom Flutter engine is a separate release asset, not git; the workflow swaps it in.
- `cargo vendor` pulls in the submodule and all git dependencies into `vendor/`, so the working
  build only talks to the rustdesk fork, not `rustdesk-org`.

---

## 3. Architecture

### Active path (GitHub-first) - primary

```
admin-ui (Custom Client form)
   |  POST /custom_build (server, key, password, brand)
   v
Go API (DeskForge)
   |  workflow_dispatch + ENCRYPTED inputs (password never appears in logs)
   v
GitHub Actions in rustdesk fork (windows-2022 runner)
   |  build from the fork; engine/drivers/vendor from fork releases
   v  built rustqs.exe -> POST back to your server (/api/save_custom_client)
Go API stores the binary -> admin-ui Download
```

**Security-critical point (§8.8):** the binary is NOT published as a public release.
It is sent to your server. Inputs (`server`/`key`/`password`) are encrypted in the rdgen style
and decrypted with a secret inside the run, so nothing sensitive leaks from the public fork.

### Fallback path (standalone Docker / Windows) - frozen, not active

The Docker `build-linux` and `build-win` agents are kept on disk as manual fallbacks but are
not part of the default workflow. They consume the same `/rdgen-data/jobs/{id}.json` queue,
but the api never reads their `output_dir/status` files back into the DB (see BUGS.md
**B-001**). They should not be relied on until that loop is closed.

```
PROD HOST (Linux)                               WINDOWS SERVER (separate)

  server (hbbs/hbbr + Go API + admin-ui) <-->   win-build
  linux-build (Linux/Android client,             Flutter Windows -> rustqs.exe
  server forks, vendor validation)               native toolchain

  job queue over the network (SMB)
```

Standalone `win-build`, for the case where GitHub is abandoned, uses a **separate Windows server**,
**native**, without Docker. Flutter desktop is unreliable inside Windows containers. The channel is
an SMB folder (§8.4). **Do not deploy it right now**. Scripts are ready in `win-builder/` and can
be activated later if needed.

| Component | Host | Role | Status |
|---|---|---|---|
| `server` | Linux prod | hbbs/hbbr + Go API + admin-ui | working |
| `linux-build` | Linux prod | Linux/Android client, server forks, vendor validation | builds Linux binary |
| GitHub Actions | rustdesk fork | **Flutter Windows -> rustqs.exe (ACTIVE path)** | in progress (§8.8) |
| `win-build` standalone | separate native Windows Server | Flutter Windows -> rustqs.exe (FALLBACK) | ready, not activated |

---

## 4. Data flow (job lifecycle) - FALLBACK path (standalone)

> This section describes the frozen standalone agent. The active GitHub flow is in §3 and §8.8.

```
1. admin-ui -> Custom Client form (platform=windows, server, key,
             permanent password, name=rustqs, brand)
2. Go API (custom_build.go) writes job.json:
   { platform, src_repo, src_ref, server, key, custom_txt(b64), app_name, ... }
3. job enters the queue:
   - linux job   -> local prod-host volume -> linux-build picks it up
   - windows job -> network channel -> Windows server -> win-build picks it up
4. win-build agent:
   a. git clone $src_repo @ $src_ref (or from local bundle offline)
   b. edit config.rs: server + key                 <- L1 injection
   c. apply allowCustom patch + write custom.txt   <- L2 injection (quick support)
   d. edit Cargo.toml/Runner.rc: rustqs branding   <- L3 injection
   e. cargo build --release + flutter build windows
   f. portable-packer -> one rustqs.exe
5. rustqs.exe -> output/{job} -> admin-ui shows Download
```

The only architectural change compared to the previous model is step 3: the channel between the
Linux production API and the Windows builder. Options are a shared network volume (SMB/NFS),
a tiny HTTP endpoint on the agent, or a queue. The current "file in a volume" model transfers
almost as-is.

---

## 5. Three config injection layers (how to get `rustqs.exe`)

Confirmed against 1.4.7 code and the rdgen fork workflow. **Using the filename to set the server
is only an emergency fallback, not the main path.** The main path is:

1. **Server + key -> hardcoded into the binary.** `sed` against `libs/hbb_common/src/config.rs`:
   - `RENDEZVOUS_SERVERS` (`rs-ny.rustdesk.com`) -> your server
   - `RS_PUB_KEY` (`OeVuKk5nlHiXp+...`) -> your key
2. **Quick-support behavior -> signed `custom.txt`** (permanent password,
   `verification-method`, hide connection manager). In OSS, the signature is checked against
   the rustdesk key; this is bypassed by `allowCustom.py`, already vendored in
   `rdgen/.github/patches/`.
3. **Branding -> `rustqs`.** `sed` against `Cargo.toml`, `Runner.rc`, language files, plus
   portable-packer (`libs/portable/generate.py`) wraps everything into a single self-extracting
   `rustqs.exe`.

The full recipe for all three layers already exists in
[rdgen/.github/workflows/generator-windows.yml](rdgen/.github/workflows/generator-windows.yml).
It must be brought over to the local Windows builder if the standalone path is activated.

---

## 6. Offline kit (L3 foundation) - frozen

Versions are facts from the 1.4.7 workflow tag. Frozen on 2026-06-11:
5.0G, 11 artifacts, manifest with sha256 in
`rustdesk-cache:/rustdesk-cache/offline-kit/artifacts/`.
The bundle was verified by clone-back, and L1 was verified with `cargo build --offline`.
Next use: upload it into fork releases (§8.8.2) as the source for GitHub builds, while also
keeping it as the fallback source for standalone builds.

| Artifact | Version / pin | Level |
|---|---|---|
| git bundle of rustdesk fork + hbb_common | tag 1.4.7 | L1 |
| `vendor/` (`cargo vendor`) | by `Cargo.lock` | L1 |
| Rust toolchain | **1.75** | L3 |
| LLVM / Clang | **15.0.6** | L3 |
| Flutter SDK | **3.24.5** | L3 |
| Flutter engine (custom rustdesk) | release asset | L3 |
| `vcpkg` baseline | `120deac3062162151622ca4860575a33844ba10b` | L3 |
| `vcpkg` downloads + binary cache | pinned baseline, triplet `x64-windows-static` | L3 |
| pub cache (Flutter packages) | by `pubspec.lock` | L3 |

---

## 7. Source parameterization (for downstream forkers)

The source repository is parameterized so a downstream forker can point at their own repo:

- **Agent defaults:** `RUSTDESK_REPO`, `RUSTDESK_REF` in `offline-kit/versions.env` and in
  `win-builder/agent.ps1` parameters.
- **Runtime override:** `src_repo`, `src_ref` in `job.json` and admin-ui settings.
  `agent.ps1` already reads these fields from the job.

Independence chain: you bake an image with `RUSTDESK_REPO=github.com/YOU/rustdesk`, then the
forker changes the env to their own repo, and their GUI builds from their fork.
Original `rustdesk/rustdesk` is not involved.

---

## 8. Roadmap

Priority order only. Each item should be detailed separately before implementation.
**Current active priority: §8.8 (GitHub track).**
§8.3/§8.4 (standalone) are frozen as fallback.
§8.1/§8.2/§8.3a (offline kit, fork procedure, deps) are the completed foundation.

- [~] **8.1. Offline freeze - SCRIPT READY, VOLUME REMOVED.**
  The script in [offline-kit/](offline-kit/) is idempotent. The `rustdesk-cache` volume was
  deleted by the owner on 2026-06-11 after §8.8 succeeded, because the GitHub build no longer
  depends on the local kit. What remains on host: a staging copy of 5 files in
  `offline-kit/artifacts/` (~62 MB) uploaded to GitHub release `offline-assets-1.4.7`
  in the fork (engine, usbmmidd, printer driver + adapter, sha256sums).
  Lost from the volume for standalone fallback: bundle, vendor tarball (2.7G), Flutter SDK
  for win+linux, `vcpkg` checkout, Rust MSI, TopMost bundle. Can be re-frozen at any time with
  `bash freeze.sh` on Linux.

- [x] **8.2. Fork procedure - DOCUMENTED.**
  [offline-kit/FORK-PROCEDURE.md](offline-kit/FORK-PROCEDURE.md) covers levels A (fork + vendor),
  B (binaries in releases), C (downstream forker), and the acceptance checklist.
  The actual GitHub forking remains the owner's action.

- [x] **8.3. Standalone Windows builder (FALLBACK) - DESIGNED, FROZEN.**
  Not deployed now (GitHub-first, §8.8). Activate only if GitHub is abandoned.
  Native Windows Server, no Docker. Implemented files:
  [win-builder/setup.ps1](win-builder/setup.ps1),
  [win-builder/agent.ps1](win-builder/agent.ps1),
  [win-builder/README.md](win-builder/README.md).
  Not tested due to lack of a Windows host. Risk points marked `[VERIFY]`.
  The container path (`Dockerfile.build-win-native`) was abandoned and removed.
  The old MinGW `Dockerfile.build-win` remains until §8.7 but is abandoned (§9).

- [x] **8.3a. Extra Windows build dependencies - FROZEN.**
  Added `thirdparty` stage to `freeze.sh`: `RustDeskTempTopMostWindow` (sources, pin `53b548a`),
  `usbmmidd_v2.zip`, printer drivers (driver + adapter + sha256sums).
  All are in the offline kit manifest. Building TopMostWindow via msbuild must be wired into
  the Windows path when tested on a real host.

- [x] **8.4. API <-> standalone agent channel (FALLBACK) - SOLVED: SMB.**
  Applies only to the fallback path (§8.3). Production API stays unchanged; SMB exposes
  the `rdgen-data` volume to the Windows agent. Config is documented in
  [win-builder/SERVER-SETUP.md](win-builder/SERVER-SETUP.md).

- [~] **8.8. GitHub track (ACTIVE PRIORITY) - build `rustqs.exe` through a rustdesk fork.**
  rdgen-style model: `DeskForge` triggers `workflow_dispatch` in the rustdesk fork,
  the fork builds on `windows-2022` and sends the binary back to the server.
  Guide: [github-build/README.md](github-build/README.md).
  Important: the rdgen workflow already contains encrypted inputs and `save_custom_client`,
  so §8.8.4 was almost ready out of the box.

  **State as of 2026-06-11 (handoff note):**
  - `gh` installed at `C:\Program Files\GitHub CLI\gh.exe`, account `bashrusakh`
    (`repo`/`workflow` scopes).
  - Forks ready: `bashrusakh/rustdesk`, `bashrusakh/hbb_common` (public, tags 1.4.7 / 1.4.6).
  - Release `offline-assets-1.4.7` exists in the rustdesk fork with engine/usbmmidd/drivers/sha256.
  - Blocked on two owner answers in the original session: mini-test vs full pipeline, and whether
    the server is reachable from outside for `save_custom_client`.
  - Next commands in §8.8.3 were: create build branch from tag 1.4.7, repoint `.gitmodules`
    to `bashrusakh/hbb_common`, copy `rdgen/.github/workflows/generator-windows.yml` and
    `rdgen/.github/patches/*` into the fork, repoint URLs to release `offline-assets-1.4.7`,
    set fork secrets `GENURL` / `ZIP_PASSWORD` / token, trigger and debug.

  **Subtasks:**
  - [x] **8.8.1. Fork rustdesk + hbb_common - DONE.**
  - [~] **8.8.2. Workflow sovereignty - assets uploaded.**
    `offline-assets-1.4.7` exists in the fork with engine (63M), usbmmidd, printer driver + adapter,
    and generated sha256sums. Remaining work at that stage was submodule repoint + workflow URL repoint + vendor.
  - [x] **8.8.3a. Mini-test - GREEN.**
    Branch `rustqs/min-test` from tag 1.4.7 with workflow
    [github-build/windows-min-test.yml](github-build/windows-min-test.yml), a close copy of the official
    Windows Flutter build plus `workflow_dispatch`, using the engine from the fork release.
    Run `27341830418` succeeded in about 45 minutes and proved the runner toolchain is usable.
  - [~] **8.8.3b. Build pipeline expansion (one step at a time).**
    Completed checkpoints:
    - `usbmmidd` / printer URLs moved to the fork release. Build no longer touches `rustdesk.com`
      or `rustdesk-org` for binary assets.
    - L1 `config.rs` injection validated with real server/key.
    - L3 branding validated.
    - L2 quick-support validated, including `allowCustom` patch and writing `custom_.txt`.
    - Encrypted inputs implemented and verified; open-input fallback preserved.
    - `save_custom_client` to the server works.
  - [~] **8.8.5. Go API integration - WORKING for windows, but with regressions (see BUGS.md).**
    Completed pieces:
    - `api/model/github_build_config.go` singleton model.
    - `api/service/github_build_config.go` with Get/Save, payload encryption,
      test connection, dispatch, run status, artifact download.
    - `api/http/controller/admin/github_build_config.go` with Get/Save/GenerateKey/Test/DispatchTest.
    - Service registration, AutoMigrate, `DatabaseVersion` 267 -> 268, admin router bind.
    - admin-ui page and API client for GitHub Build settings.
    - Windows job glue: `submitBuild` dispatches to GitHub for `platform=windows`, then a
      background poller downloads and unpacks the artifact into `/rdgen-data/output/{id}/`,
      updates build status, and falls back to file queue when GitHub config is absent.
    - `SetWorkflowSecret` implemented with `golang.org/x/crypto/nacl/box` and exposed as
      `/admin/github_build_config/sync_secret` plus a UI button.

    Open follow-ups (2026-06-20 audit, tracked in BUGS.md):
    - **B-003** `pollAndDownload` is orphaned on api restart — needs persistent
      `github_run_id` column + startup reconciler.
    - **B-004** `buildCustomTxtFromForm` packs ~6 of ~30 form fields into `custom_.txt`;
      everything else (permissions, branding URLs, theme, direction, ...) is silently dropped
      for the windows-via-GitHub path.
    - **B-005** UI key `enable_remote_modi` vs mapper key `allow_remote_config_modification`
      — toggle never lands in the built client.
    - **B-009** `dispatchTest` posts an empty payload and triggers a real run.
    - **B-010** clearing `Branch` in the UI persists as `""` (`Save` doesn't preserve like
      it does for `Token` / `PayloadKey`).
  - [x] **8.8.4. Security - DONE.**
    Inputs are encrypted (`enc_payload`, AES-256-CBC + PBKDF2) and decrypted with
    `WORKFLOW_PAYLOAD_KEY` from GitHub Secrets. Key resync works. Binary goes to the server,
    not to a public release.
  - [ ] **8.8.6. Switch to production workflow.**
    After tests are complete, move from `rustqs-windows-min-test.yml` (smoke-test)
    to the full `generator-windows.yml` (msi, signing, all artifacts).

- [ ] **8.5. Runtime binary verification.**
  Add a smoke test (`--version` in a container or equivalent) so a "successful" build cannot be broken.

- [~] **8.6. `.gitignore` + secret scan - PARTIAL.**
  `.gitignore` exists and excludes `data/`, private key `id_ed25519`, databases, `.env`,
  `node_modules`, build output, `offline-kit/artifacts`, `.claude/`.
  Secret scan found no hardcoded secrets in source. Remaining owner action: `git init`, first commit,
  and creation of the public repository.

- [ ] **8.7. Final ballast cleanup.**
  Remove test containers `build-win-test*`, `rdgen-data/output/test-win-*`, duplicate compose files,
  and experimental scripts. Do it last, because the build system was still being reshaped.
  Pulled in 2026-06-20: also remove the abandoned MinGW `docker/Dockerfile.build-win` +
  `entrypoint-win.sh` (§9 already marks this path as dead), and switch
  `docker/docker-compose.yml` so that `build-linux` is started behind a `--profile fallback`
  flag instead of by default.

- [ ] **8.11. Full client rebranding (remove "RustDesk" from sources) - FUTURE.**
  Current L3 only covers exe metadata and portable launcher. Real branding still leaves "RustDesk"
  in the About page, URLs, language files, icon, Windows manifest, and copyright.
  The right approach is to expand build-time `sed` logic in `rustqs-windows-min-test.yml`, not to
  commit rebranding directly into the rustdesk fork, otherwise merging upstream fixes becomes painful.
  **AGPL-3.0 requirement:** keep `LICENSE`, keep original copyright notices in files, add your own
  below if needed, and include "Modified from RustDesk" in About and README.

  Remaining rebranding targets from `rdgen/.github/workflows/generator-windows.yml` lines 161-241:

  | Category | File | What |
  |---|---|---|
  | About page | `flutter/lib/desktop/pages/desktop_setting_page.dart` | `Purslane Ltd`, `RustDesk`, copyright lines |
  | URLs | `flutter/lib/common.dart`, install/home/mobile pages | `https://rustdesk.com/*` -> new URLs (`url_link`, `download_link`) |
  | Language files | `src/lang/*.rs` | remaining `RustDesk` strings, e.g. `powered_by_me` |
  | Icon | `res/icon.ico` + Flutter assets | requires upload of PNG/ICO via `app_icon_b64` |
  | Windows manifest | `flutter/windows/runner/runner.exe.manifest` | `assemblyIdentity name=...` |
  | Copyright in `Runner.rc` | `flutter/windows/runner/Runner.rc` | copyright string |
  | MSI installer | `res/msi/Package/License.rtf`, `res/msi/preprocess.py` | only if MSI is enabled |
  | About: `Modified from RustDesk` | About page | required by AGPL |

  New workflow inputs needed later: `display_name`, `url_link`, `download_link`, `app_icon_b64`.

- [ ] **8.12. Rebrand the SERVER side to DeskForge - FUTURE.**
  This should be a one-time commit in `bashrusakh/DeskForge`, unlike §8.11 where the client is
  rebranded at build time because upstream still moves. The server upstream is a snapshot and will
  be updated rarely.

  **Safe to rebrand:**
  | Layer | Files | What |
  |---|---|---|
  | Rust server | `server/src/main.rs`, `server/Cargo.toml` | log strings, CLI help, banners, description, authors |
  | Binary names | `server/Cargo.toml` | optional `hbbs` -> `deskforge-id`, `hbbr` -> `deskforge-relay` |
  | Go API | `api/...go`, `api/conf/config.yaml` | log strings, comments, config names |
  | Env vars | `.env.example`, `docker-compose.yml`, `config.yaml` | `RUSTDESK_API_*` -> `DESKFORGE_API_*` |
  | Vue admin-ui | titles, About, logo, copyright, i18n |
  | Docker | service name `rustdesk` -> `deskforge`, image labels |
  | README/docs | rewrite fully |

  **Do NOT touch** (wire protocol compatibility with the client):
  - `*.proto` files and generated protobuf structures
  - names in `RendezvousMessage`, `HelloFromHbbs`, `RelayResponse`, etc.
  - handshake magic bytes
  - imports from `hbb_common`
  - ports 21114-21118 unless changed everywhere consistently

  **Legally required (AGPL + MIT):**
  - Root `LICENSE` must remain AGPL-3.0.
  - Keep a `NOTICE` section like:
    ```
    Includes code from:
    - rustdesk-server (AGPL-3.0) Copyright Purslane Ltd.
    - lejianwen/rustdesk-api (MIT) Copyright Lejianwen
    - lejianwen/rustdesk-api-web (MIT) Copyright Lejianwen / vue-manage-system
    ```
  - Keep per-file copyright headers in `server/src/*`.

  **Component licenses as of 2026-06-13:**
  | Component | License |
  |---|---|
  | `server/` (`rustdesk/rustdesk-server`) | AGPL-3.0 |
  | `api/` (`lejianwen/rustdesk-api`) | MIT |
  | `admin-ui/` (`lejianwen/rustdesk-api-web`) | MIT |
  | `rdgen/` (`bryangerlach/rdgen`) | GPL-3.0; only workflow patches are used |

- [~] **8.13. admin-ui UI rework - FOUNDATION IN PR #3.** Goal: turn admin-ui from a generic CRUD admin panel into an operational remote-access console (see `ui-rework.md`).
  Done in PR #3 / branch `ui-refract`:
  - design tokens for light/dark surface/text/border/status colors, radius, shadows, typography (`admin-ui/src/styles/style.scss`);
  - theme system `auto` / `light` / `dark` via `html[data-theme]`, `localStorage` (`theme-mode`), and Element Plus dark class sync;
  - `ConnectionPulse`, `ThemeSwitch`, `CopyableText`, `PageHeader`, `PageSection`, `DangerZone`, `EmptyState`, `LoadingState` as the first shared UI primitives;
  - shell refresh: sidebar/header/menu/settings on tokens, without the always-on tags bar;
  - mobile navigation through `el-drawer`, desktop collapse preserved;
  - dashboard Quick Connect: `rustdesk://id`, web client `/webclient2/#/{id}`, jump to devices;
  - Devices page: permanent Status column, `ConnectionPulse` online/offline from `last_online_time`, copyable ID, compact `Connect` + `More` actions;
  - Monitoring visual pass: login history, connection history, file transfers, and shared sessions now use shared page header/section layout;
  - Server visual pass: Server Commands, Server Config, and GitHub Build settings now use shared page header/section layout; advanced custom commands separated via `DangerZone` and require confirm before `sendCmd`; terminal output got readonly console styling, target hint, Copy/Clear controls, and empty-output placeholder;
  - Access visual pass: Address Book entries, collections, share rules, and tags now use shared page header/section layout; address book IDs use `CopyableText`; wide actions compacted into `More` dropdown;
  - Users/Security visual pass: Users, API Tokens, OAuth providers, Groups, and Device Groups now use shared page header/section layout; wide user actions compacted into `More` dropdown;
  - Client Builder/Profile visual pass: Custom Client Builder and My Profile now use shared page header/section layout; build history pagination aligned;
  - My Workspace visual pass: My Devices, My Address Book, My Address Book Collections, My Tags, My Shared Sessions, and My Login History now use shared page header/section layout;
  - 404 refresh: standalone empty-state screen with a return link to dashboard;
  - Custom Client runtime fix: preset/upload handlers return from `setup()` and are available to the template;
  - login/register/OAuth approve/OAuth bind converted to token-based auth layout;
  - `ocr review`: no high/medium findings; low nit fixed;
  - `npm run build` passes;
  - Monitoring filter pass: Login History, Connection History, File Transfer History, and Shared Sessions now use `FilterBar`;
  - DataTable pass: `admin-ui/src/components/ui/DataTable.vue` added; Users page migrated to DataTable with slot-based custom cells.

  Remaining follow-up phases:
  - [ ] i18n for the new dashboard/auth hero copy;
  - [x] table/filter/pagination/empty/loading state unification;
  - [x] Devices page: `ConnectionPulse` status, compact actions, copyable ID, web/native connect, pagination aligned via `PageSection`;
  - [x] Monitoring: shared page header/section done; Login History, Connection History, File Transfer History, Shared Sessions use `FilterBar`;
  - [x] Server commands: Simple/Advanced/Danger Zone + terminal output polishing done;
  - [x] CRUD dialogs unified with `AppDialog` (zero raw `el-dialog` in views);
  - [x] DataTable applied to all view pages (zero raw `el-table` except nested inline in `fileList`);
  - [x] My Profile added to the user dropdown menu;
  - [x] hardcoded colors in `control.vue` and `login.vue` replaced with CSS variables;
  - [~] Access/Security CRUD screens: address books/collections/share rules/tags, users, API tokens, OAuth providers, groups, and device groups page primitives are ready; custom client / my profile / my workspace page primitives are ready; remaining form/dialog standards still need unification;
  - [x] 404 page: tokenized empty-state screen done;
  - [ ] manual responsive-browser verification, not just `npm run build`.

- [x] **8.9. Custom Preset - DONE.**
  No model expansion was actually needed because all fields already live in the `custom_json`
  text blob. In practice, this task fixed four real bugs that would have broken the GUI build:
  - `server_ip` vs `server`: UI stored `server_ip`, but Go expected `server`.
    Added fallback `server_ip` -> `server`.
  - `custom_txt` not generated: when not explicitly provided, Go now builds it from fields like
    `permanent_password`, `hide_cm`, `deny_lan`, etc. via `buildCustomTxtFromForm()`.
  - `Save as preset` created duplicates on repeated save with the same name.
    `CustomPresetService.Create` now performs an upsert on `(user_id, name)`.
  - `loadPresetIntoForm` was missing some fields: `app_icon_url`, `app_logo_url`,
    `privacy_screen_url` are now restored too.

- [ ] **8.14. GitHub Actions for Linux + Android (NEW, 2026-06-20).**
  Mirror the windows-min-test approach for the other two supported platforms:
  - Port `rdgen/.github/workflows/generator-linux.yml` and
    `rdgen/.github/workflows/generator-android.yml` into `github-build/linux.yml`
    + `github-build/android.yml` in the rustdesk fork, keeping the same `enc_payload`
    contract.
  - Extend `custom_build.go::submitBuild` to dispatch them based on `b.Platform`.
  - Teach `pollAndDownload` to recognize platform-specific artifact filenames
    (`rustqs` binary, `rustqs.apk`).
  - Until shipped, the platform select in `admin-ui/src/views/custom-client/index.vue`
    should restrict to Windows (BUGS.md **B-013**); macOS stays off the menu.
  - Docker `build-linux` agent stays on disk as a manual-only fallback (§8.7).

- [ ] **8.15. Drop `windows-x86` as a build target (NEW, 2026-06-20).**
  Owner decision: 32-bit Windows is not a supported target in 2026. Remove the option from
  `admin-ui/src/views/custom-client/index.vue`, the default branches, and any router
  detection that currently falls back to the file queue for it (BUGS.md **B-002**).

- [x] **8.10. Single-binary `rustqs.exe` - DONE.**
  The workflow was reworked so that:
  - L3 no longer renames `rustdesk.exe` before packing, because the packer expects that name.
  - Native deps and TopMost artifacts are downloaded directly into `Release/`.
  - L2-B writes `custom_.txt` into `Release/` before packing, so it gets embedded inside the exe.
  - New `L4 portable-pack` runs `libs/portable/generate.py` and outputs the final packed exe.
  - Artifact upload happens from `./output/{appname}.exe`.

  Run `27462227115` succeeded in about 33 minutes. Final artifact: **one file `rustqs.exe`, 23.2 MB**.
  Metadata is `rustqs`; `custom_.txt` is packed inside.

  Note: an earlier run failed with `bad decrypt` because `WORKFLOW_PAYLOAD_KEY` in the fork had drifted
  from the local `offline-kit/artifacts/workflow-payload.key`. That was valid as a debug case and
  indicated the need for key resync.

---

## 9. Abandoned approaches (DO NOT repeat)

These paths were tested and rejected. They are documented here so future agents do not go down them again.

- **Cross-compiling the Windows Flutter client from Linux (MinGW)** is a dead end.
  Flutter Windows desktop cannot be cross-compiled from Linux; a Windows host with MSVC is required.
  The Linux/MinGW path only gives the legacy Sciter UI at best, not the current Flutter client.
- **Workarounds around broken `vcpkg` `libvpx.a` in the MinGW build** were symptom-level only.
  `vcpkg` produced Linux ELF objects instead of Windows COFF, so the archive was fundamentally
  un-linkable for PE. Linker-order hacks and compatibility stubs would only hide the real issue.
- **`x86_64-pc-windows-gnu` as the final target** is not the upstream path.
  Upstream uses `x86_64-pc-windows-msvc`; the GNU target is commented out in their CI.
- **Using the exe filename to configure the server** is only a fallback. The main path is
  hardcoding via `config.rs` (§5).

Current test containers (`build-win-test6/7/8/14`, `upbeat_carson`, `lucid_grothendieck`) are leftovers
from the abandoned MinGW path and should be removed during §8.7.

---

## 10. Completed (historical log)

Preserved as a record of what has already been built. Detailed phase-by-phase history lives in git
and `CHANGELOG.md`. Parts of the old Windows-client architecture below are outdated (see §9), but the
server side and admin UI remain current.

### Server side and admin UI
- Docker multi-stage build: Rust (`hbbs` + `hbbr`) + Go API + Node admin-ui + `s6-overlay`.
- `server` container healthy, ports 21114-21118.
- admin-ui forked from `lejianwen/rustdesk-api-web`, English by default, navigation reworked.
- admin-ui UI rework foundation: design tokens, `auto` / `light` / `dark` theme mode,
  `ConnectionPulse`, `ThemeSwitch`, refreshed shell/sidebar/header/menu/settings,
  dashboard Quick Connect, token-based login/register/OAuth screens, mobile drawer nav,
  devices/monitoring/server/access/security/client-builder/profile/my-workspace visual passes,
  refreshed 404, and shared empty/loading primitives. PR #3.
- Dashboard API + UI, Server Config UI, `GET /api/admin/config/all`.
- Custom Client UI (form + history), Presets CRUD, logo/icon upload.
- Go API: models/services/controllers for `CustomBuild` + `CustomPreset`, AutoMigrate,
  `DatabaseVersion` 265 -> 267.
- Go module renamed from `github.com/lejianwen/rustdesk-api/v2` to `rustdesk-server/api`.
- External URLs removed (update check, rendezvous, STUN, Firebase, CDN), Chinese text removed.

### Client build
- `linux-build` agent builds the Linux `rustdesk` binary (~32 MB), feature `linux-pkg-config`.
- `win-build` MinGW agent reached final linking but got blocked on broken `libvpx.a`.
  That path is abandoned and replaced by the native Windows builder (§3, §8.3).

---

## 11. Known reference facts

- The `custom.json` mechanism in `flutter/lib/` written by the current entrypoint is **not read**
  by 1.4.7 code. It is a no-op. The real mechanism is `custom.txt` + `config.rs`.
- `read_custom_client` in `src/common.rs` verifies the signature of `custom.txt` with the rustdesk key.
  Patch `allowCustom.py` in `rdgen/.github/patches/` removes that check.
- Server selection from the filename is implemented in `src/custom_server.rs`
  (parses `host=` / `key=` from the exe name). That is the fallback mechanism.
- Available patches in `rdgen/.github/patches/`: `allowCustom`, `hidecm`,
  `removeSetupServerTip`, `removeNewVersionNotif`, `cycle_monitor`, `xoffline`,
  `privacyScreen`, `flutter_3.24.4_dropdown_menu_enableFilter`.
