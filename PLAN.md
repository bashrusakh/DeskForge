# PLAN.md — DeskForge: Single Source of Truth

> Last updated: 2026-11-23
> Related: [CHANGELOG.md](CHANGELOG.md) · [CONTRIBUTING.md](CONTRIBUTING.md) · [Open findings & tracker IDs](#61-open-findings--tracker-ids)

> **Evidence labels (2026-11-23):** no live provider run or clean-environment build
> proof is claimed; Windows is the only production capability admitted by the API gate,
> Linux/Android are gated pending PR11 evidence. Current schema state and full
> dispatch-contract, workflow-approval, schema/migration, and atomicity evidence: §2.
> Ports: Go API 21114; Rust services 21115–21119 (relay WebSocket 21119);
> production Compose exposure 21114–21118.

---

## 0. Project goal

Self-hosted RustDesk server: hbbs/hbbr + API + admin panel + a **custom client builder**.
Goal: reduce dependence on `rustdesk/rustdesk`, `rustdesk-org/*`, and `rustdesk.com`.
Current source manifests still reference active third-party upstream repositories, so full upstream independence is not verified.

**Active client build path:** the API dispatches an owned workflow in the configured RustDesk fork and polls the provider run.
It downloads the exact artifact through the provider API (§2), then validates, extracts, and publishes it locally.

Outside the API path:

- Reference/documentation only: `github-build/`, `rdgen/`.
- Manual/historical-only: `win-builder/` and the Docker build agents (`docker/Dockerfile.build-linux` / `docker/Dockerfile.build-win` + `entrypoint-linux.sh` / `entrypoint-win.sh`, compose `profiles: ["fallback"]`).

**GitHub-first:** free Windows runners, a configured fork workflow (the min-test result below is historical), and no deployed standalone Windows Server.

---

## 1. Repository map

```
bashrusakh/
├── DeskForge              ← this repo (server, api, admin, docker)
│   └── libs/hbb_common/   ← tracked copy of our hbb_common fork @ pin + patches; not a submodule here
├── rustdesk               ← owned RustDesk fork and active workflow source
│   └── .github/workflows/ ← rustqs-windows.yml, rustqs-linux.yml, rustqs-android.yml
└── hbb_common             ← mirror fork of rustdesk/hbb_common (sync mandate: docs/FORKS-AND-PINS.md §3)
```

**Client version:** owned by the fork `bashrusakh/rustdesk` — currently **1.5.3** (`Cargo.toml` on `master`); DeskForge contains no built-in client.

`offline-kit/versions.env` is a frozen 1.4.8-era snapshot (`RUSTDESK_REF="1.4.8"`, commit `3c574a4`), not a current version source. Current schema/migration state: §2.

**Pins:**

- The configured RustDesk fork's `libs/hbb_common` is a separate git submodule. Its gitlink points at our mirror fork `bashrusakh/hbb_common` by SHA (`master` → `36ab6ed`, 2026-11-23).
- DeskForge's own `libs/hbb_common/` copy is pinned at `fae47e024dd8` — the `rustdesk/rustdesk` 1.5.1/1.5.3 pin, re-pinned 2026-11-23 — plus the recorded local patch set.
- Pin record: `libs/hbb_common/UPSTREAM.md` · topology and rules: `docs/FORKS-AND-PINS.md`.

**Evidence boundaries:**

- Active RustDesk and DeskForge manifests also reference third-party upstream repositories/modules; provider-side live execution and clean-build proof are not recorded here.
- The broad `GOWORK=off go vet ./...` / `go test ./...` pass and its opt-in Redis boundary are recorded in CHANGELOG.md ("Current-state reconciliation — 2026-08-10"). Other release/build gates remain unverified (§2).
- DeskForge tracks no repository `vendor/` tree, client-release directory, or `rustdesk-deps` archive.

**License:** identified as AGPL-3.0 for the covered work.
Canonical: [LICENSE](LICENSE) and [NOTICE](NOTICE).
Inventory incomplete; no signatures/attestations; no full-sovereignty claim — see [README](README.md) ("License").

---

## 2. Architecture

### Current path (GitHub Actions)

```
admin-ui (Custom Client form)
   ↓ custom-build request
Go API (DeskForge)
   ↓ workflow_dispatch + provider-derived public workflow_sha + authenticated DFP1 enc_payload (AES-256-CBC + PBKDF2 + HMAC)
GitHub Actions [configured RustDesk fork, owned platform workflow]
   ↓ L1 config.rs (server + key) · L2 custom_.txt (permanent password, allowCustom patch) · L3 branding (rustqs, portable-packer)
provider run ← Go API polls provider status
provider artifact API → Go API downloads the exact artifact
Go API validates/extracts/publishes locally → admin-ui Download
```

**Security:**

- The password is never published — `enc_payload`, decrypted inside the runner via GitHub Secret `WORKFLOW_PAYLOAD_KEY`.
- Before approval, preparation, or secret-bearing dispatch, the API requires the exact provider-owned `# deskforge-workflow-identity-guard: v1` marker in workflow content. The content is resolved at the immutable workflow SHA; legacy unguarded tags fail closed.
- The provider-derived outer `workflow_sha` is checked against `github.sha` before secret-bearing jobs. The same authenticated inner payload field is checked again before exports, checkout, or build use.
- Defense in depth, not an atomic defense against a malicious workflow file or GitHub's selector/SHA binding. The verified tag and active no-bypass ruleset remain required controls. The runner does not callback to the API. The API retrieves the artifact through the provider API and publishes it locally, not to a public release.

### Workflow approval and schema evidence

Before a provider-backed build can run, the API accepts only a provider-derived verified annotated tag:

- an accepted verification reason;
- the aggregate of all applicable active protected tag rulesets for that label, with effective update and deletion protections;
- no bypass actors.

A tag/branch collision with the same label is rejected.
The identity-guard marker check above (before approval, preparation, or secret-bearing dispatch) is additionally required at the resolved immutable workflow SHA.

Legacy unguarded tags fail closed.
Raw refs, SHAs, credentials, and workflow internals are not normal UI inputs.

**Schema history.** The published DeskForge schema is `DatabaseVersion` **272**; the current local corrective candidate targets schema **285** — not published and not live-provider evidence. The historical workflow-approval additive fields are **280**–**282**, followed by migration steps **283**–**285**:

| Schema | Change |
| ------ | ------ |
| **280** | `workflow_ref_approved` — approval status |
| **281** | `workflow_ref_provider_verified` — provider-policy status |
| **282** | `workflow_ref_approval_sha` — provider-resolved approval commit |
| **283** | `idx_custom_presets_user_id_name` composite index on `(user_id, name)` |
| **284** | create-time `preset_id` / `preset_name` provenance snapshots on `custom_builds` |
| **285** | custom-client branding-field preflight before `AutoMigrate`, failing closed when an existing `custom_builds` or `custom_presets` row already carries a branding field value the shared branding seam rejects |

- Before `AutoMigrate`, migration preflight fails if it finds an existing duplicate group; it does not
  automatically select or delete a row.
- Legacy plaintext secret rows stay readable and metadata-only saves preserve them without
  `SECRET_ENCRYPTION_KEY`; semantics: [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

**Migration test evidence.** SQLite-only migration evidence: tests cover:

- the schema-283 duplicate-data preflight/unique index;
- the schema-284 provenance columns, including migration and readback of a populated pre-provenance `custom_builds` row;
- the schema-285 branding preflight/version gate:
  - an install already at 284 fails closed on a rejected stored branding value without recording 285;
  - an accepted one upgrades and records 285;
  - a fresh/283/284 install runs the preflight while a 285 install skips it.

**MySQL/PostgreSQL migration and read/write coverage remain unverified.**
Local workflow manifests, bridge/helper source, Android app-ID/runtime-path checks, and package assertions are static evidence only; no APK/package/install/runtime evidence exists.

**Dispatch and atomicity.**

Focused fake-transport and SQLite checks do not prove live GitHub/provider approval, dispatch, polling, or artifact delivery.

- Dispatch relies on the exact HTTP 200 run-details contract. It pins `X-GitHub-Api-Version: 2026-03-10` (the client-wide pin stays `2022-11-28`) and sends only the ref/inputs body.
- GitHub's 2026-03-10 breaking change removed `return_run_details` and made the endpoint always return HTTP 200 with the run details. The dispatch reads them directly and no longer sends the retired parameter.
- That is a deliberate contract modernization, not a #80 requirement: the pinned `2022-11-28` version with `return_run_details=true` also returns the run details.
- A standard 204 or latest-run inference is unsupported.
- The local identity write is atomic. No durable outbox or distributed lease makes provider dispatch plus the database write one atomic operation. A post-dispatch database failure remains a documented limitation.

### Historical/frozen builders (not used by the current API)

`admin-ui → Go API → jobs/{id}.json → SMB share → standalone Windows builder or Docker
build-linux`: these local-queue builders are not a production fallback for the current
API — historical/frozen material only, never an alternative workflow source.

---

## 3. Component status

| Component                      | Status         | Notes                                |
| ------------------------------ | -------------- | ------------------------------------ |
| hbbs/hbbr (Rust)               | ✅ running     | ports 21115-21119; relay WebSocket is 21119 |
| Go API                         | ✅ running     | users, address book, OAuth, LDAP, audit |
| Admin UI (Vue 3)               | ✅ running     | 16 pages, 3 locales, DataTable, FilterBar |
| GitHub build (Windows)         | 🟡 implemented | API path is provider-backed; no current live provider/clean-build proof |
| GitHub build (Linux)           | 🟡 gated       | workflow mapping exists; production capability remains disabled    |
| GitHub build (Android)         | 🟡 gated       | workflow mapping exists; production capability remains disabled    |
| `github-build/`                | 📘 reference   | documentation only; no executable workflow copies                |
| `rdgen/`                       | 📘 reference   | vendored historical/reference material, not the active source      |
| win-builder standalone         | ❄️ frozen      | manual/historical only; not API path |
| Docker build agents (`Dockerfile.build-linux`/`build-win`) | ❄️ frozen      | manual/historical only; not API path |
| offline-kit                    | ❄️ frozen      | re-freeze when client version changes |

---

## 4. Three injection layers for custom client

| Layer | What we change        | Mechanism                                                    |
| ----- | --------------------- | ------------------------------------------------------------ |
| L1    | server + key          | `sed` in `libs/hbb_common/src/config.rs` — `RENDEZVOUS_SERVERS`, `RS_PUB_KEY` |
| L2    | quick-support password| `custom_.txt` (signature checked — `allowCustom.py` patch removes check) |
| L3    | branding (rustqs)     | `Cargo.toml`, `Runner.rc`, portable-packer (`libs/portable/generate.py`) |

Historical reference: `rdgen/.github/workflows/generator-windows.yml` — not the executable workflow source; never copy it into the fork.

---

## 5. ✅ Completed milestones

- [x] Client builds from the owned fork `bashrusakh/rustdesk` @ 1.5.3 — version lives in the fork; schema/migration state and evidence: §2
- [x] Offline-kit freeze/verify tooling and local artifact set retained; real-kit verification remains blocked under PR10
- [x] GitHub min-test Windows: historical green run, ~33 min, single-binary rustqs.exe
- [x] Go API: provider workflow dispatch/polling, artifact download, local validation, extraction, publication, capability-URL TTL
- [x] Admin UI redesign: design tokens, DataTable, AppDialog, FilterBar, 16 pages
- [x] Security: encrypted-at-rest (AES-GCM), OAuth delete guard, audit, TTL
- [x] Rust server: atomic blocklist, aur-fix, JWT

---

## 6. Open roadmap

- [ ] **Linux + Android capability validation** — live workflow/artifact evidence + guarded platform picker
- [ ] **Full client rebrand** — About, URLs, icons — in workflow, not in the fork
- [ ] **Smoke test** for built binary (`--version`)
- [ ] **Ballast cleanup** — remove MinGW leftovers, test containers

### 6.1. Open findings & tracker IDs

Folded from `BUGS.md` (deleted 2026-11-24). Live findings first, then every retired ID
still cited from code, so each citation resolves here.
Legend: `[ ]` open · `[~]` partial · `[x]` fixed.

| ID | Status | Essence | Where |
| --- | --- | --- | --- |
| `AU-M-022` | `[ ]` open · security | Unauthenticated client-API writes: `POST /api/sysinfo`, `/api/heartbeat`, `/api/audit/conn`, `/api/audit/file` register before `RustAuth()`, so an anonymous caller can create/alter `Peer` rows and inject audit entries; `/api/shared-peer` asserts `share_token` unchecked (500 when missing). Fix needs RustDesk protocol-design confirmation — the PC client hits these endpoints before auth. | `api/http/router/api.go` (before `Use(RustAuth())` at `:76`); `api/http/controller/api/webClient.go:57` |
| `RD-B6` | `[~]` partial | rdgen `download`/`get_png`/`get_zip` stay unauthenticated by protocol; hardened with file TTLs instead of a token gate — exe 7d (`RDGEN_EXE_TTL`), png 6h (`RDGEN_PNG_TTL`), zip 1h (`RDGEN_ZIP_TTL`), `<=0` disables; path-traversal/UUID validation already in place. Still open (needs design): server-signed expiring URLs threaded through all URL-generation sites. | `rdgen/rdgenerator/views.py:506,527,716`, `rdgen/docker-compose.yml:18-20` (frozen) |
| `B-012` | `[~]` partial | Linux/Android build workflows: the API maps fork-owned workflow filenames, but the production capability gate rejects both platforms until PR11 validates real Actions runs and the full artifact path; `bridge.yml` must exist on both branches (else dispatch 422s). No live provider/workflow evidence. UI re-expose tracked as `B-013` behind a feature flag once runs are green. | fork `.github/workflows/rustqs-linux.yml`, `rustqs-android.yml` (reference templates only: `github-build/`, `rdgen/.github/workflows/`) |
| `B-014` | `[~]` partial | Unused conversion helpers/API surface partially deleted 2026-06-20 (`FromCustomBuild`, `FromCustomPreset`, `detail()` export, `GET /custom_build/detail/:id` un-routed). Still open: `GET /api/admin/custom_build/public/detailByKey/:key` has no UI caller — kept as a documented capability URL; delete in a janitorial PR after the higher-priority work. | `api/http/router/admin.go:63` (open route), `:388` (un-routed `detail/:id`) |
| `AU-C-001` | `[x]` fixed · retired | Server settings are volatile — runtime changes lost on restart; state now persists in `server_cmd_states` and is replayed on startup (`ReplayServerCmds`). | `api/cmd/apimain.go:66,271`; `api/http/controller/admin/rustdesk.go:238`; `api/service/serverCmd.go:47`; `api/model/server_cmd_state.go:3` |
| `B-003` | `[x]` fixed · retired | `pollAndDownload` was orphaned on API container restart; `custom_builds.github_run_id` + `ResumePendingPolls()` now resume in-flight GitHub polling after restart. | `api/cmd/apimain.go:268`; `api/model/custom_build.go:34` |
| `B-006` | `[x]` fixed · retired | `download_key` was permanent and rate-limit-free; capability URLs now expire via `download_key_expires_at` + config `download-key-ttl` (default 7d; `<=0` → 7d). | `api/conf/config.yaml:13`; `api/config/config.go:28`; `api/model/custom_build.go:30`; `api/http/controller/admin/custom_build.go:697,824` |
| `B-008` | `[x]` fixed · retired | GitHub PAT and `permanent_password` were stored in plaintext; secrets at rest are now AES-256-GCM-encrypted via `SECRET_ENCRYPTION_KEY` (semantics: [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)), including `permanent_password` inside `custom_json`. | `api/utils/secretcrypt.go:18`; `api/model/github_build_config.go:154`; `api/model/custom_build.go:212`; `api/model/custom_preset.go:82` |
| `B-009` | `[x]` fixed · retired | `dispatchTest` sent an empty payload `map[string]any{}` to the workflow; the real payload is sent now. | `api/http/controller/admin/github_build_config.go:295` |
| `AU-M-021` | `[x]` fixed · retired | My Profile — account info not editable; self-editable `UpdateCurrent`/`UpdateCurrentForm` added (GORM `Updates(struct)` skips zero values — noted at the call site). | `api/http/controller/admin/user.go:283`; `api/http/request/admin/user.go:65`; `api/service/user.go:269` |
| `AU-S-001` | `[x]` fixed · retired | No audit logging for server commands; `server_cmd_audits` records command, payload, actor, timestamp — after auth. | `api/model/server_cmd_audit.go:3`; `api/http/middleware/audit.go:17`; `api/http/controller/admin/rustdesk.go:50` |
| `AU-L-007` | `[x]` fixed · retired | OAuth provider delete had no check for in-flight sessions; a usage guard runs before deletion. | `api/service/oauth.go:398` |
| `AU-L-011` | `[x]` fixed · retired | Hardcoded artifact name in the build downloader; lifted out of an inline string into `defaultWindowsArtifactName`. | `api/http/controller/admin/custom_build.go:461` |
| `AU-L-015` | `[x]` fixed · retired | Auto-registered users always got `GroupId=1`; creation-order-dependent assignment replaced by a fallback. | `api/service/group.go:20` |
| `AU-L-016` | `[x]` fixed · retired | Versions-list GitHub failures surfaced as 5xx; now 200 + empty/error state so the UI can show empty/error instead (cited from code; absent from the final `BUGS.md` snapshot). | `api/http/controller/admin/custom_build.go:502` |
| `RD-B5` | `[x]` fixed · retired | rdgen `ALLOWED_HOSTS = ['*']`; hosts now come from env and startup refuses a wildcard host. | `rdgen/rdgen/settings.py:61,73` (frozen — that comment still names `BUGS.md`) |

> **Retired:** the remaining `BUGS.md` entries — `B-001` `[~]` (file-queue `done`
> propagation, tombstoned), `B-002`, `B-013`, `AU-L-010` `[x]`, and older fixed IDs —
> were retired 2026-11-24; the 2026-06-20 frozen-builder / `windows-x86` /
> platform-gating owner decisions are already reflected in PLAN §2, §3, §9.
> Full record: CHANGELOG + git history.

> **Tracker rule** (carried from `BUGS.md`): this block is owned alongside PLAN.md —
> new findings take a fresh `B-NNN` (or `AU-*`/`RD-*`) ID here; on fix, flip the status
> and record the PR/commit on the same line; owner won't-fix → `[skip]` + one-line reason.

---

## 7. Fork maintenance procedure for a new upstream rustdesk-client release

Maintenance procedure for the owned forks and DeskForge's `libs/hbb_common/` copy — not the DeskForge build execution path. Workflow ownership (canonical statement): [docs/FORKS-AND-PINS.md §1](docs/FORKS-AND-PINS.md#1-topology) — no workflow copies from `github-build/` or `rdgen/`. Fork topology, sync rules, pin bookkeeping, and re-pin mechanics: [docs/FORKS-AND-PINS.md](docs/FORKS-AND-PINS.md); pin and patch record: [`libs/hbb_common/UPSTREAM.md`](libs/hbb_common/UPSTREAM.md).

**Pins are SHAs.** Upstream `rustdesk/hbb_common` has no version tags at all, so every
`hbb_common` pin is a commit SHA. Client release tags (e.g. `1.5.0`, unprefixed) exist
only on `rustdesk/rustdesk` — and tag names are not identities: the fork's `1.5.0`
(`9253e1fc`) is a fork commit, not upstream's `1.5.0` (`fada664d`). Record the SHA.

When `rustdesk/rustdesk` publishes a new tag, follow these steps:

### 7.1. Sync the `hbb_common` fork

Run the standing auto-sync rule so [`bashrusakh/hbb_common`](https://github.com/bashrusakh/hbb_common)
contains the target upstream commit — `sync-upstream.yml`, merge-based (`upstream/main` into
`main`, never history-rewriting). Status, manual fallback, and conflict rules:
[docs/FORKS-AND-PINS.md §3](docs/FORKS-AND-PINS.md#3-fork-maintenance).

### 7.2. Re-point the RustDesk fork's gitlink by SHA

In `bashrusakh/rustdesk`, bring the branch up to the new client tag (fetch
`upstream --tags`, then merge or push the mirrored tag), and record the chosen
`hbb_common` pin SHA in the `libs/hbb_common` gitlink (URL:
`https://github.com/bashrusakh/hbb_common.git`). Exact commands:
[docs/FORKS-AND-PINS.md §5](docs/FORKS-AND-PINS.md#5-updating-the-deskforge-copy-to-a-new-pin).

**Tag-collision resolution:** never overwrite an existing published fork tag — nothing is history-rewritten. If the fork already owns a different commit under the upstream tag name (as with `1.5.0`), record the collision and the upstream tag's SHA; if a mirror ref is needed, use a disambiguating ref name (e.g. `upstream-<tag>`).

### 7.3. Re-pin DeskForge's `libs/hbb_common/` copy

Replace the tracked copy with the fork's content at the pin SHA, record the new fork point
in [`libs/hbb_common/UPSTREAM.md`](libs/hbb_common/UPSTREAM.md), then re-apply exactly the
record's **kept rows** under its carry-over rule (the kept/dropped table is the
authoritative patch-set list). Exact steps, save/restore, and verification:
[docs/FORKS-AND-PINS.md §5](docs/FORKS-AND-PINS.md#5-updating-the-deskforge-copy-to-a-new-pin).

### 7.4. Update vendor (historical procedure; not current repository state)

```bash
# On a machine with Rust:
cargo vendor vendor/
git add vendor/ && git commit -m "chore: vendor deps for <client-tag>"
git push origin <client-tag>
```

Or if vendor is too heavy — historically, upload `vendor-<client-tag>.tar.gz` as a release asset. DeskForge does not currently contain that vendor tree or release asset.

### 7.5. Re-freeze offline-kit (when the client line changes)

```bash
cd DeskForge/offline-kit
# versions.env: RUSTDESK_REF=<client-tag>, RUSTDESK_COMMIT=<full SHA>; check MSRV, Flutter, vcpkg baseline
bash freeze.sh source vendor engine
```

### 7.6. Update offline-assets release (historical operator procedure)

```bash
# Upload engine/usbmmidd/driver to the fork:
gh release create offline-assets-<client-tag> --repo bashrusakh/rustdesk \
    --title "Offline build assets (<client-tag>)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

> **Note:** After publishing the release, the version catalog can expose it in the admin UI
> when the configured repository contains the matching `offline-assets-*` release and source
> tag. The catalog is repository-derived — there is no hardcoded repository or obsolete version
> fallback list (contract: `github-build/README.md` §Version flow).

### 7.7. Adapt workflow

Compare upstream `build-for-windows-flutter` with `rustqs-windows.yml` (new system
dependencies? changed `build.py` flags? changed `config.rs` / `custom_.txt` format?) and
port the changes to the fork workflow.

> **Important:** `bridge.yml` must stay **without `inputs.version`** — same as upstream.
> Bridge and build must work from the same code (the fork). Do not add `repository:` to checkout.

### 7.8. Workflow ownership

Canonical statement: [docs/FORKS-AND-PINS.md §1](docs/FORKS-AND-PINS.md#1-topology) —
maintain active workflow logic in the fork; create no deployment copies in this repository.

### 7.9. Verify and update DeskForge reference

- [ ] GitHub Actions run ✅ (no `startup_failure`)
- [ ] `VERSION` in logs matches the version selected in admin UI
- [ ] Binary arrived at the server; `rustqs.exe`, ~23 MB, `custom_.txt` packed inside
- [ ] Smoke test on clean Windows
- [ ] `libs/hbb_common/UPSTREAM.md` — new fork point + patch record
- [ ] `offline-kit/versions.env` — new `RUSTDESK_REF`; `offline-kit/FORK-PROCEDURE.md` — versions in examples
- [ ] `PLAN.md` — update current tag in §1; `github-build/README.md` — update patch URLs if changed

---

## 8. What are offline-kit and offline-assets

| Entity                  | What it is                                           | Where stored                          |
| ----------------------- | ---------------------------------------------------- | ------------------------------------- |
| `offline-kit/`          | Scripts (`freeze.sh`) + config (`versions.env`)      | In git, in this repo                  |
| `offline-kit/artifacts/`| Local freeze output (ignored/untracked)              | Local operator storage only           |
| `offline-assets-{tag}`  | Provider-side asset naming convention                | Availability and release proof unverified |

**Why:** without this insurance, loss of upstream source or services can block a custom client build.
The kit is frozen local operator material; active third-party upstream dependencies remain in the
source manifests, and its current incomplete verification does not prove upstream independence,
sovereignty, a network-denied full build, a signature, or release readiness.

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
- Patches in `rdgen/.github/patches/` (7 `.diff`): allowCustom, cycle_monitor, flutter_3.24.4_dropdown_menu_enableFilter,
  hidecm, removeNewVersionNotif, removeSetupServerTip, xoffline — plus the `allowCustom.py` / `privacyScreen.py` helper scripts.
