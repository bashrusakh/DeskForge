# BUGS.md — Custom Client Builder workflow audit

> Tracker for issues found in the build-custom-agent end-to-end flow.
> Backend: `api/http/controller/admin/custom_build.go`, `api/service/custom_build.go`,
> `api/service/github_build_config.go`. Frontend: `admin-ui/src/views/custom-client/index.vue`,
> `admin-ui/src/views/server/github-build.vue`. Workflow: `github-build/windows-min-test.yml`.
>
> Status legend: `[ ]` open · `[x]` fixed · `[~]` partial · `[skip]` won't fix (owner decision).
>
> Last audit: 2026-06-22 (statuses consolidated across all fix branches).

---

## Architectural mismatch with PLAN.md §3

PLAN.md declares the standalone / Docker build agents **frozen as fallback** (§8.3, §8.4).
Reality: `custom_build.go::submitBuild` still routes every non-Windows platform — and even
`windows-x86` and Windows when GitHub config is absent — into the file queue
(`/rdgen-data/jobs/{id}.json`). Owner decision (2026-06-20):

1. Treat Docker `build-linux` and `build-win` containers as **frozen manual fallback**, not the
   default route. They stay on disk but should not be started by `docker compose up`.
2. Remove `windows-x86` (32-bit) as a build target everywhere — UI option, form defaults,
   any router branches. 2026; not worth maintaining.
3. Build a **GitHub Actions workflow for Linux + Android** mirroring the windows-min-test
   pipeline, and re-route `submitBuild` accordingly. Until those workflows ship, non-Windows
   platforms should be hidden in the UI to stop users from creating phantom builds.

The bugs below are grouped by where they leak into user-visible breakage.

---

## CRITICAL — workflow is silently broken end-to-end

### [~] B-001 · File-queue jobs never propagate `done` status back to the DB
**Deferred on branch `fix/build-custom-agent` (2026-06-20):** UI now restricts platforms to
Windows-via-GitHub (B-013), so the file-queue path is unreachable from the default flow
even though `submitBuild` still has the branch. `docker/docker-compose.yml` moved
`build-linux` and `build-win` services behind a `fallback` profile so they don't start by
default. Full fix (status mirror) only matters once Linux/Android workflows land (B-012),
at which point we'd rather route them through GitHub too.


**Where:** `api/http/controller/admin/custom_build.go:172-201` (writes job),
`docker/entrypoint-linux.sh:37,49,76,...` and `docker/entrypoint-win.sh:33,45,210` (write `output_dir/status`).
**Symptom:** Linux/Android (and Windows when GitHub config is missing) builds sit at
`Status=pending` forever. `DownloadByKey` returns HTTP 409. The Download button never appears
in the UI (`v-if="row.status === 'done'"`, `custom-client/index.vue:306`).
**Root cause:** no Go-side watcher reads `/rdgen-data/output/{id}/status`. The build agent's
status file is dead-letter.
**Fix path (per owner direction):**
- Short term: hide all non-Windows-via-GitHub options in the UI (B-002, B-013).
- Long term: replace the file queue for the supported platforms with GitHub Actions dispatch
  (linux/android workflows, B-012).

### [x] B-002 · `windows-x86` falls into the broken file queue
**Fixed on branch `fix/build-custom-agent` (2026-06-20):** the platform `<el-select>` in
`admin-ui/src/views/custom-client/index.vue:26-37` now offers only Windows 64-bit and is
locked (`disabled`). 32-bit, Linux/Android, macOS options removed pending B-012/B-013.


**Where:** `api/http/controller/admin/custom_build.go:173` (`b.Platform == "windows"` excludes
`windows-x86`); `admin-ui/src/views/custom-client/index.vue:30` (option still offered).
**Symptom:** UI lets users pick Windows 32-bit; the resulting build hangs at `pending`
forever (see B-001).
**Fix:** remove the `windows-x86` option from `index.vue:30` and the default branches.
Optionally reject it on the backend with a clear error.

### [x] B-003 · `pollAndDownload` is orphaned on API container restart
**Fixed on branch `fix/build-custom-agent` (2026-06-20):**
- `api/model/custom_build.go` — new `GithubRunId int64` column.
- `api/cmd/apimain.go` — `DatabaseVersion` 268 → 269, calls `admin.ResumePendingPolls()`
  after `DatabaseAutoUpdate()`.
- `api/http/controller/admin/custom_build.go` — `tryGithubDispatch` now persists `runId`
  on the row; new `ResumePendingPolls()` scans `Status=building AND github_run_id>0` rows
  on startup and re-launches `pollAndDownload` for each.


**Where:** `api/http/controller/admin/custom_build.go:260` (`go ct.pollAndDownload(b.Id, runId)`),
no startup reconciler.
**Symptom:** `runId` is only stored in `BuildLog` as a free-form string. If the api container
restarts during a GitHub run, the goroutine is gone, no one finalizes the build, the row
stays at `Status=building` forever.
**Fix:** add `GithubRunId int64` column to `CustomBuild`, persist it from
`tryGithubDispatch`, and add an init hook that finds `Status=building` rows on startup and
resumes `pollAndDownload`.

### [x] B-004 · `buildCustomTxtFromForm` drops ~80% of form fields
**Fixed on branch `fix/build-custom-agent` (2026-06-20):**
`api/http/controller/admin/custom_build.go::buildCustomTxtFromForm` rewritten. Now maps the
full `PRESET_FIELDS` list from the Vue form: 14 string fields (password, theme, direction,
approve-mode, permissions-mode, company name, download URL, api/relay server, three
branding URLs, android package id) and 18 boolean fields including all 13 permission flags
(`enable-keyboard` … `enable-terminal`). Dead/never-sent keys
(`allow_remote_config_modification`, `disable_update`) removed.


**Where:** `api/http/controller/admin/custom_build.go:364-409`.
**Symptom:** for `windows`-via-GitHub (the active path), the UI looks rich, but only the
following round-trip into the built `rustqs.exe`: `permanent_password`, `deny_lan`,
`enable_direct_ip`, `hide_cm`, `remove_wallpaper`, plus `server`/`key`/`app_name`.
**Silently lost:** `direction`, `pass_approve_mode`, `auto_close`, `theme`, `company_name`,
`download_url`, `api_server`, `relay_server`, `remove_new_version_notif`, `cycle_monitor`,
`x_offline`, `android_app_id`, `permissions_type`, **all 13 permission flags**
(`enable_keyboard` … `enable_terminal`), **all 3 branding URLs** (`app_icon_url`,
`app_logo_url`, `privacy_screen_url`).
**Dead mapper keys** (frontend never sends them): `allow_remote_config_modification`,
`disable_update`.
**Fix:** rebuild the mapping table against the rdgen `allowCustom`-patched client's
`custom_.txt` schema. Treat it as the single source of truth; derive both the mapper and the
preset/save fields from one shared list.

### [x] B-005 · `enable_remote_modi` (UI) vs `allow_remote_config_modification` (mapper)
**Fixed on branch `fix/build-custom-agent` (2026-06-20):** the new boolean-mapping table in
`buildCustomTxtFromForm` reads `enable_remote_modi` (the field the UI actually sends) and
emits the canonical `allow-remote-config-modification` key into `custom_.txt`. Now wired
end-to-end.


**Where:** `admin-ui/src/views/custom-client/index.vue:187`,
`api/http/controller/admin/custom_build.go:383`.
**Symptom:** the "Remote Modification" toggle is wired to a key the backend never reads.
Always off in the produced client.
**Fix:** unify the field name with B-004.

---

## HIGH — known but unfixed gaps

### [x] B-006 · `download_key` is permanent and rate-limit-free
**Fixed on branch `fix/download-key-expiry`:** added expiry to the capability URL.
- New `download_key_expires_at` (unix-seconds) column on `custom_builds`
  (`api/model/custom_build.go`); `DatabaseVersion` 269 → 270 so AutoMigrate adds it.
- New `download-key-ttl` config (`api/config/config.go`, `api/conf/config.yaml`,
  default `168h`); `CustomBuild.Create` stamps `now + TTL` when minting the key
  (falls back to 7 days if unset/≤0). Legacy rows keep `0` = no expiry.
- `DownloadByKey` and `DetailByKey` now both go through a single
  `findBuildByDownloadKey` helper that checks existence **and** expiry and returns
  `410 Gone` on an expired link — so expiry can't be enforced in one handler and
  forgotten in the other.

Still optional follow-ups (not in this PR): explicit revoke action and per-IP rate
limit on `/public/download/:key`.


**Where:** `api/http/controller/admin/custom_build.go:97-167`.
**Symptom:** leaked key = forever-public artifact. 32 random characters so brute force is fine,
but there's no revoke/expiry/single-use.
**Fix:** add `expires_at` column + a configurable TTL (e.g. 7 days); optionally a revoke action
and per-IP rate limit on `/public/download/:key`.

### [x] B-007 · `DownloadByKey` builds the zip fully in RAM
**Fixed on branch `fix/build-custom-agent` (2026-06-20):**
`api/http/controller/admin/custom_build.go::DownloadByKey` now streams `zip.NewWriter`
directly into `c.Writer`. `Content-Length` removed (length unknown before `Close`),
response will be chunked. `bytes`/`strconv` imports still used by other helpers — kept.


**Where:** `api/http/controller/admin/custom_build.go:137-156`.
**Symptom:** OOM risk on small VPS once builds carry full Flutter/Inno bundles.
**Fix:** stream `zip.NewWriter(c.Writer)` directly to the response, set headers before the
first `Write`.

### [x] B-008 · GitHub PAT and `permanent_password` stored in plaintext
**Fixed on branch `fix/encrypt-secrets-at-rest`:** symmetric AES-256-GCM encryption at rest.
- New helper `api/utils/secretcrypt.go` (`EncryptSecret`/`DecryptSecret`): AES-256-GCM
  under a key derived (SHA-256) from a **new** `SECRET_ENCRYPTION_KEY` env var — deliberately
  not `WORKFLOW_PAYLOAD_KEY` (that one is cluster-shared with GitHub). Ciphertext is tagged
  `enc:v1:`; `EncryptSecret` is idempotent and values without the tag pass through, so legacy
  plaintext rows keep working and get encrypted on next write. If the key is unset, encryption
  is disabled (plaintext) with a one-time warning, so existing deployments don't break.
- Transparent GORM hooks (`BeforeSave`/`AfterSave`/`AfterFind`) on `GithubBuildConfig`
  (Token + PayloadKey) and on `CustomBuild`/`CustomPreset` (`custom_json`, which carries
  `permanent_password`). Callers keep seeing plaintext; only the DB holds ciphertext.
- `.env.example` documents `SECRET_ENCRYPTION_KEY`. Unit tests cover round-trip, idempotency,
  legacy passthrough, and key-unset behaviour.

Note: rotating `SECRET_ENCRYPTION_KEY` makes existing ciphertext unreadable; a re-encrypt
migration would be a separate task if rotation is ever needed.


**Where:** `api/model/github_build_config.go:19`, `api/model/custom_build.go:11` (in
`custom_json`).
**Symptom:** anyone with DB access reads both. Not yet fixed.
**Fix:** symmetric encryption at rest using a key from environment (deployer-supplied,
mirrored by an env-var on rotate). Don't roll the same key twice — `WORKFLOW_PAYLOAD_KEY`
is already cluster-shared and not a good fit.

### [x] B-009 · `dispatchTest` sends an empty payload to the workflow
**Fixed on branch `fix/dispatch-test-payload`:** combined two of the suggested fixes.
- Confirmation gate: `DispatchTest` now requires `{"confirm": true}` in the body and rejects
  unconfirmed calls with a clear message (a read-only check already exists at `/test`). The UI
  (`github-build.vue`) asks `window.confirm(...)` and `dispatchTest()` sends `confirm: true`.
- Real payload: instead of `map[string]any{}`, the smoke test now sends the server's own
  configured `server` (Rustdesk id-server, falling back to api-server), `key`, and a clear
  `app_name` "deskforge-smoketest", so the produced artifact is valid rather than an empty,
  late-failing build.


**Where:** `api/http/controller/admin/github_build_config.go:118` (`map[string]any{}`).
**Symptom:** smoke-test dispatches a real build with empty `server/key/app_name`. The
workflow will either fail late or produce an unusable artifact. Wastes minutes and clutters
build history.
**Fix:** either gate the button behind a confirmation that the user understands a real run is
triggered, or call a dedicated `noop` workflow, or pass safe placeholder inputs that the
workflow recognizes and short-circuits.

---

## MEDIUM — robustness

### [x] B-010 · `Save` wipes Branch to empty string on incomplete forms
**Fixed on branch `fix/build-custom-agent` (2026-06-20):**
`api/service/github_build_config.go::Save` now treats empty `Branch` the same way it
already treats empty `Token`/`PayloadKey` — preserve current value.


**Where:** `api/service/github_build_config.go:49-51`.
**Symptom:** clearing the Branch field and pressing Save stores `""`. `DispatchBuild` falls
back to `"master"` (controller fallback), but the UI's `load()` defaults to `rustqs/min-test`
on empty — confusing drift.
**Fix:** treat empty `Branch` the same way `Token`/`PayloadKey` are treated — preserve the
existing value.

### [x] B-011 · `Update` uses `gorm.Updates(struct)` and ignores zero-values
**Fixed on branch `fix/build-custom-agent` (2026-06-20):**
`api/service/custom_build.go::Update` now uses `DB.Save(u)` — full-row update so explicit
zero values land in the DB. Needed downstream by B-003 to clear `GithubRunId` when a build
completes (if ever desired) and to write `FileSize=0` cases.


**Where:** `api/service/custom_build.go:57-59`,
`api/service/github_build_config.go:Save` (Save itself is OK because it uses `DB.Save`).
**Symptom:** future code that legitimately needs to write a zero value (clear log, reset size)
will silently no-op. Currently latent.
**Fix:** prefer `DB.Save(u)` for full-row updates or `DB.Model(u).Select(...).Updates(map)`
for explicit field sets.

---

## LOW — dead code / cleanup

### [~] B-014 · Unused conversion helpers and API surface
**Partially fixed on branch `fix/build-custom-agent` (2026-06-20):**
- Deleted `CustomBuildForm.FromCustomBuild` (`api/http/request/admin/custom_build.go`).
- Deleted `CustomPresetForm.FromCustomPreset` (`api/http/request/admin/custom_preset.go`).
- Deleted `detail()` export from `admin-ui/src/api/custom_client.js`.
- Removed unused `detailPreset` import in
  `admin-ui/src/views/custom-client/index.vue`.
- Un-routed `GET /custom_build/detail/:id` in `api/http/router/admin.go` (handler kept on
  the controller for symmetry; can be deleted later if no internal caller appears).

Still open: `/custom_build/public/detailByKey/:key` exists but has no caller in the UI.
Left in place because it's a documented capability URL and may have third-party consumers.


**Where:**
- `api/http/request/admin/custom_build.go:14` — `CustomBuildForm.FromCustomBuild` (no callers).
- `api/http/request/admin/custom_preset.go:24` — `CustomPresetForm.FromCustomPreset` (no callers).
- `admin-ui/src/api/custom_client.js:10` — `detail()` exported, no importer.
- `admin-ui/src/views/custom-client/index.vue:323` — `detailPreset` imported, never used in
  `setup()`.
- `GET /custom_build/detail/:id` and `GET /custom_build/public/detailByKey/:key` — already
  noted as unused in `audit-report.md:879-881`.
**Fix:** delete in a single janitorial PR after the higher-priority work lands.

### [x] B-015 · `b == nil` checks after `Info()` are dead branches
**Fixed on branch `fix/build-custom-agent` (2026-06-20):** both `b == nil` arms in
`pollAndDownload` removed; comment added explaining the `Id == 0` semantics.


**Where:** `api/http/controller/admin/custom_build.go:292,352`.
`CustomBuildService.Info` always returns a non-nil pointer; the `b.Id == 0` check is the real
one.
**Fix:** drop the `b == nil` arm.

### [x] B-016 · `onMounted` clobbers prefilled fields with `/config/all` values
**Fixed on branch `fix/onmounted-config-race`:** `onMounted` now fills `server_ip`, `key`,
`api_server`, `relay_server` from `fetchConfig` **only when the field is still empty**, so a
preset applied before `fetchConfig` resolves is no longer overwritten by server defaults.
Order-independent (doesn't rely on `fetchConfig` winning the race).


**Where:** `admin-ui/src/views/custom-client/index.vue:603-617`.
**Symptom:** if a preset is selected before `fetchConfig` resolves, server-side defaults
overwrite preset values. Race-y but harmless on fast networks.
**Fix:** only fill empty fields from `fetchConfig`, or move `fetchConfig` before
`loadPresets`.

### [x] B-017 · Build history doesn't auto-refresh
**Fixed on branch `fix/build-history-autorefresh`:** `loadBuilds` now schedules a 12 s poll
(`ensurePolling`) whenever any row is `pending`/`building`, reloading the list silently (no
spinner flicker) and stopping itself once every row is terminal. The timer is cleared on
`onUnmounted`. `loadBuilds(silent)` gained a flag so background refreshes don't toggle the
loading state; the `[page,pageSize]` watcher calls `loadBuilds()` explicitly so it keeps the
spinner.


**Where:** `admin-ui/src/views/custom-client/index.vue:602`.
**Symptom:** rows transition `pending → building → done` server-side, but the table keeps
showing stale status until the user reloads.
**Fix:** poll `/custom_build/list` every 10–15 s while any row is in `pending`/`building`;
stop polling when everything is terminal.

---

## STRUCTURAL — to enable B-001/B-002/B-013 fixes

### [~] B-012 · Build Linux + Android GitHub Actions workflows
**Partially done on branch `fix/build-linux-routing` (Linux):**
- Backend routing (build/vet-tested): `submitBuild` now dispatches `platform=linux` to GitHub
  (alongside `windows`); `tryGithubDispatch` picks the workflow per platform (`windows` →
  configurable `gcfg.WorkflowFilename`; `linux` → const `defaultLinuxWorkflowFilename`
  = `rustqs-linux.yml`) via a gcfg copy; `pollAndDownload` selects the artifact by platform
  (`rustdesk-min-test-linux`, with the single-artifact fallback from AU-L-011) and extracts the
  Linux bundle (all files, FileSize = largest) instead of looking for an `.exe`.
- **Draft workflow `github-build/linux.yml`** mirrors the fork contract (enc_payload + L1
  config.rs + L2 allowCustom, both platform-independent; L3 brand adapted for Linux) and a
  Flutter-Linux x86_64 build ported from `rdgen/.github/workflows/generator-linux.yml`.
  **NOT yet validated by a real Actions run** — the build steps (vcpkg/flutter/build.py/
  packaging/artifact paths) need CI iteration like windows-min-test did.

**Android (branch `fix/build-android-routing`, stacked on the Linux branch):** backend extended
the same way (`submitBuild`/`tryGithubDispatch` const `defaultAndroidWorkflowFilename`
= `rustqs-android.yml`; `pollAndDownload` artifact `rustdesk-min-test-android`, shared
linux/android extract-all path). **Draft `github-build/android.yml`** (single ABI arm64-v8a)
ported from `generator-android.yml` with the fork contract — also **NOT CI-validated**;
note the Android `custom_.txt` embedding is best-effort and needs verification.

Still open: validate `linux.yml` and `android.yml` on Actions; re-expose Linux/Android in the
UI (B-013) behind a feature flag once runs are green; optionally move the workflow names into
`GithubBuildConfig` (consts for now).


**Where (new):** `github-build/linux.yml`, `github-build/android.yml`. Reference templates:
`rdgen/.github/workflows/generator-linux.yml`, `rdgen/.github/workflows/generator-android.yml`.
**Symptom:** today there's no GitHub path for Linux/Android, so submit goes to the deprecated
file queue (B-001). Owner direction (2026-06-20): mirror the windows-min-test approach for
these platforms; keep `entrypoint-linux.sh` as a manual fallback only.
**Fix:** port the rdgen workflows into the fork with the same `enc_payload` contract;
extend `submitBuild` to dispatch them based on `b.Platform`; extend `pollAndDownload` to
recognize the artifact filenames (`rustqs`, `rustqs.apk`).

### [x] B-013 · UI must hide platforms that have no working backend
**Updated on branch `fix/custom-client-platforms`:** after B-012 wired Linux/Android routing,
the platform select is unlocked and exposes `Linux x64 (experimental)` and
`Android arm64 (experimental)` alongside `Windows 64Bit` (default). The "experimental" labels
+ hint make the draft, CI-unvalidated status explicit. macOS / 32-bit stay unsupported.
**Originally fixed on branch `fix/build-custom-agent` (2026-06-20):** platform select was locked
to `Windows 64Bit` (see B-002 fix) until Linux/Android backends existed.


**Where:** `admin-ui/src/views/custom-client/index.vue:29-34`.
**Symptom:** Linux/Android/macOS are offered to users; only Windows works end-to-end today
(B-001). macOS has no agent and no workflow at all.
**Fix:** until B-012 lands, restrict the platform select to `windows`. Re-add Linux/Android
behind a feature flag once their GitHub workflows are live. Drop the macOS option entirely
unless it goes on a roadmap item.

---

## ADMIN UI / API — open findings (consolidated from the removed `audit-report.md`)

The functional admin-UI audit (PR #19) had 65 findings; 58 were fixed in PR #20–#22.
The full report file was removed during doc consolidation (2026-06-21). The findings
still open are preserved here:

### [x] AU-C-001 · Server settings are volatile — runtime changes lost on restart
**Fixed on branch `fix/server-cmd-persistence`:** server-command state is now persisted in a
new `server_cmd_states` table and replayed on startup.
- `model.ServerCmdState` (`target`,`cmd`,`option`); in AutoMigrate, `DatabaseVersion` → 272.
- `ServerCmdService.PersistCmd` stores applied **set** commands (skips read commands that have
  no `option`): replace-by-(target,cmd) for `rs`/`aur`/`ml`/custom; for additive
  `<x>-add`/`<x>-remove` it keeps one row per active add and a `-remove` deletes the matching
  `-add`, so the table always equals the live set. Called from `SendCmd` after a successful send.
- `admin.ReplayServerCmds()` (startup hook in `apimain`, after AutoMigrate) re-sends the stored
  commands to the id/relay sockets, best-effort with a short delay to let hbbs/hbbr bind.

Note: `DatabaseVersion` 272 collides with the 270/271 bumps on the B-006/AU-S-001 branches —
on merge keep the highest; AutoMigrate is idempotent.

### [x] AU-S-001 · No audit logging for server commands
**Fixed on branch `fix/server-cmd-audit`:** added an audit trail for admin server-commands.
- New `server_cmd_audits` table (`api/model/server_cmd_audit.go`); `DatabaseVersion` → 271 and
  the model is in the AutoMigrate list.
- New `middleware.ServerCmdAudit()` (`api/http/middleware/audit.go`) records userId/username,
  method, path, truncated request body, client IP and response status for each mutating call.
  Wired on `POST /rustdesk/{sendCmd,cmdCreate,cmdUpdate,cmdDelete}` (after `AdminPrivilege`,
  so `curUser` is set); `cmdList` (read) is left unaudited.
- `GET /rustdesk/cmdAuditList` (paginated, newest first) exposes the log via API. A UI view is
  a follow-up.

Note: the `DatabaseVersion` bump collides with B-006's 270 — on merge keep the highest number;
AutoMigrate is idempotent so the table/column are created as long as `Migrate()` runs.


**Where:** `api/http/controller/admin/rustdesk.go`, `api/http/router/admin.go`.
**Symptom:** PR #20 gated the whole `/rustdesk/*` group behind `AdminPrivilege`, but there is
still no audit trail of who ran which server command. Needs a new audit table + middleware.

### [x] AU-M-014 · Usage component — fragile raw-text parsing
**Fixed on branch `fix/usage-parsing`:** `usage.vue` now parses through a guarded `parseUsage`
helper — bails out (empty list) if the payload isn't a string, splits lines on `/\r?\n/` and
columns on `/\s+/`, and trims/filters blank lines. Previously `res.data.split('\n')...
split(" ")` threw on a non-string payload and mis-split on CRLF or repeated spaces.

### [x] AU-M-021 · My Profile — account info not editable
**Fixed on branch `fix/profile-edit`:** users can now edit their own `nickname` and `email`.
- Backend: `POST /admin/user/updateCurrent` (`User.UpdateCurrent`) with `UpdateCurrentForm`
  (validated, email format + uniqueness check excluding self) and
  `UserService.UpdateProfile(id, nickname, email)` using an explicit field `Select` so clearing
  nickname persists. `username` stays read-only; role/status/group remain admin-only.
- Frontend: `my/info.vue` turns nickname/email into inputs with a Save button
  (`api/user.js#updateCurrent`); on success it refreshes the user store.

### [ ] AU-M-022 · Unauthenticated writes on the client-facing API
**Where:** `api/http/router/api.go` — routes registered before `frg.Use(RustAuth())` (line 76).
**Symptom:** `POST /api/sysinfo` (creates/updates `Peer` rows by caller-supplied `id`),
`/api/heartbeat`, `/api/audit/conn`, `/api/audit/file` are unauthenticated; an anonymous caller
can create/alter peers and inject audit entries. `/api/shared-peer` also does an unchecked
`(*j)["share_token"].(string)` assertion (`webClient.go:57`) → 500 on missing token.
**Fix:** needs RustDesk protocol design confirmation (the PC client hits these before auth).

### [x] AU-L-007 · OAuth provider delete — no check for in-flight sessions
**Fixed on branch `fix/oauth-delete-guard`:** `Oauth.Delete` now calls
`OauthService.CountBoundUsers(op)` (counts `user_thirds` rows for the provider's `op`) and
refuses deletion with a clear message when any accounts are still linked, so deleting a
provider can't silently orphan users' only login method. Unlink the accounts first.
### [ ] AU-L-010 · Hardcoded version list in Custom Client UI
### [x] AU-L-011 · Hardcoded artifact name in the build downloader
**Fixed on branch `fix/artifact-name-fallback`:** the inline `"rustdesk-min-test-windows"` is
now a named const `defaultWindowsArtifactName`, and `DownloadArtifact` falls back to the run's
single artifact when the name is empty or doesn't match (with a helpful error listing the
available artifact names) — so changing the workflow's artifact name no longer breaks downloads.
### [x] AU-L-015 · Auto-registered users always get `GroupId=1`
**Fixed on branch `fix/default-group-id`:** new `GroupService.DefaultGroupId()` looks up the
group with `Type = GroupTypeDefault` (lowest id) and is used by both registration paths
(OAuth auto-register and `UserService.Register`) instead of the hard-coded `1`. Falls back to
`1` if the default group can't be found, preserving legacy behaviour.

---

## rdgen generator — open findings (consolidated from the removed `AUDIT.md`)

The custom-agent build workflow audit (Django `rdgen/` + Go `api/`) landed all its ✅ fixes.
The flagged-but-unfixed items are preserved here:

### [x] RD-A4 · Hard-coded `X-GitHub-Api-Version: '2026-03-10'`
**Fixed (PR #25, `f17c439` "fix(rdgen): use real GitHub API version header"):** both call
sites in `rdgen/rdgenerator/views.py` now send `X-GitHub-Api-Version: '2022-11-28'`. Verified
present on `main`, `chore/doc-consolidation`, and `fix/build-custom-agent`; the placeholder
`'2026-03-10'` no longer appears anywhere in the codebase.


**Where:** `rdgen/rdgenerator/views.py:380,599`. Placeholder version; GitHub silently falls back
to default so it works, but the header is misleading. Should be `2022-11-28`.

### [x] RD-B1 · Four POST endpoints have no authentication
**Fixed (PR #26 `fix/workflow-bearer-auth`, `b22cd9a` "require Bearer token on
runner-callable endpoints"):** `update_github_run`, `startgh`, `save_custom_client`, and
`cleanup_secrets` are each decorated with `@_require_workflow_token`, which validates the
`Authorization: Bearer <SH_SECRET>` header (constant-time compare) and fails closed when
`SH_SECRET` is missing or still the placeholder. Confirmed merged into `main`.


**Where:** `update_github_run`, `save_custom_client`, `cleanup_secrets`, `startgh`.
Reachable by any anonymous client; the workflows send `Authorization: Bearer ${{ env.token }}`
but Django never validates it. Enables DoS on `startgh`, anonymous artifact overwrite, status
spoofing, and secrets-zip deletion. Was split into its own PR (workflow Bearer auth) — confirm
it actually merged.

### [x] RD-B5 · `ALLOWED_HOSTS = ['*']`
**Fixed on branch `fix/rdgen-allowed-hosts`:** `ALLOWED_HOSTS` is read from the `ALLOWED_HOSTS`
env var (comma/space separated). Under `DEBUG` it falls back to `['*']` for dev convenience;
with `DEBUG=False` it must be set or Django refuses to boot (same fail-loud style as the
SECRET_KEY/ZIP_PASSWORD/SH_SECRET checks). Documented as a placeholder in
`rdgen/docker-compose.yml`.


**Where:** `rdgen/rdgen/settings.py:41`. Wildcard host trust → host-header injection. Needs the
operator to supply real hostnames via env.

### [~] RD-B6 · `download` / `get_png` / `get_zip` are unauthenticated
**Hardened on branch `fix/rdgen-file-ttl`:** these endpoints are consumed by unauthenticated
callers by protocol (build runners fetch `get_png`/`get_zip`; users fetch the built exe), so a
token gate would break the flow. Instead each now refuses to serve files older than a TTL, so a
leaked UUID/filename is no longer a permanent capability: `download` 7 days (`RDGEN_EXE_TTL`),
`get_png` 6h (`RDGEN_PNG_TTL`), `get_zip` (encrypted secrets) 1h (`RDGEN_ZIP_TTL`); all env-tunable,
`<=0` disables. Path-traversal + UUID validation were already in place from an earlier audit.

Still open (needs design): true per-request auth via signed/expiring URLs generated server-side
and threaded through the workflows — a larger coordinated change across all URL-generation sites.

### [x] RD-C4 · Bare `except:` clauses in `generator_view`
**Fixed on branch `fix/rdgen-bare-except`:** the three bare `except:` arms in
`generator_view` (icon/logo/privacy `save_png` calls) are now `except Exception as e:` and
log the actual error (`f"...: {e}"`), so `KeyboardInterrupt`/`SystemExit` propagate and real
failures are no longer hidden behind the `"false"` placeholders. The mislabelled
"failed to get logo" message on the privacy block was corrected to "failed to get privacy screen".


**Where:** `rdgen/rdgenerator/views.py:168,178,188`. Hides `KeyboardInterrupt`/`SystemExit`;
masks real errors behind "false" placeholders.

---

## Tracking & ownership

- This file is owned alongside `PLAN.md`. When a bug is fixed, flip `[ ]` → `[x]` and append
  the PR/commit hash on the same line.
- New findings: append below this section with a fresh `B-NNN` id.
- If something is decided as "won't fix" by the owner, mark `[skip]` and add a one-line reason.
