# BUGS.md — Custom Client Builder workflow audit

> Tracker for issues found in the build-custom-agent end-to-end flow.
> Backend: `api/http/controller/admin/custom_build.go`, `api/service/custom_build.go`,
> `api/service/github_build_config.go`. Frontend: `admin-ui/src/views/custom-client/index.vue`,
> `admin-ui/src/views/server/github-build.vue`. Workflow: `github-build/windows-min-test.yml`.
>
> Status legend: `[ ]` open · `[x]` fixed · `[~]` partial · `[skip]` won't fix (owner decision).
>
> Last audit: 2026-06-20.

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

### [ ] B-006 · `download_key` is permanent and rate-limit-free
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

### [ ] B-008 · GitHub PAT and `permanent_password` stored in plaintext
**Where:** `api/model/github_build_config.go:19`, `api/model/custom_build.go:11` (in
`custom_json`).
**Symptom:** anyone with DB access reads both. Already flagged in `audit-report.md:893-894`,
not yet fixed.
**Fix:** symmetric encryption at rest using a key from environment (deployer-supplied,
mirrored by an env-var on rotate). Don't roll the same key twice — `WORKFLOW_PAYLOAD_KEY`
is already cluster-shared and not a good fit.

### [ ] B-009 · `dispatchTest` sends an empty payload to the workflow
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

### [ ] B-016 · `onMounted` clobbers prefilled fields with `/config/all` values
**Where:** `admin-ui/src/views/custom-client/index.vue:603-617`.
**Symptom:** if a preset is selected before `fetchConfig` resolves, server-side defaults
overwrite preset values. Race-y but harmless on fast networks.
**Fix:** only fill empty fields from `fetchConfig`, or move `fetchConfig` before
`loadPresets`.

### [ ] B-017 · Build history doesn't auto-refresh
**Where:** `admin-ui/src/views/custom-client/index.vue:602`.
**Symptom:** rows transition `pending → building → done` server-side, but the table keeps
showing stale status until the user reloads.
**Fix:** poll `/custom_build/list` every 10–15 s while any row is in `pending`/`building`;
stop polling when everything is terminal.

---

## STRUCTURAL — to enable B-001/B-002/B-013 fixes

### [ ] B-012 · Build Linux + Android GitHub Actions workflows
**Where (new):** `github-build/linux.yml`, `github-build/android.yml`. Reference templates:
`rdgen/.github/workflows/generator-linux.yml`, `rdgen/.github/workflows/generator-android.yml`.
**Symptom:** today there's no GitHub path for Linux/Android, so submit goes to the deprecated
file queue (B-001). Owner direction (2026-06-20): mirror the windows-min-test approach for
these platforms; keep `entrypoint-linux.sh` as a manual fallback only.
**Fix:** port the rdgen workflows into the fork with the same `enc_payload` contract;
extend `submitBuild` to dispatch them based on `b.Platform`; extend `pollAndDownload` to
recognize the artifact filenames (`rustqs`, `rustqs.apk`).

### [x] B-013 · UI must hide platforms that have no working backend
**Fixed on branch `fix/build-custom-agent` (2026-06-20):** platform select is now locked
to `Windows 64Bit` (see B-002 fix). Re-open this issue once B-012 lands so we can re-expose
Linux/Android behind a feature flag.


**Where:** `admin-ui/src/views/custom-client/index.vue:29-34`.
**Symptom:** Linux/Android/macOS are offered to users; only Windows works end-to-end today
(B-001). macOS has no agent and no workflow at all.
**Fix:** until B-012 lands, restrict the platform select to `windows`. Re-add Linux/Android
behind a feature flag once their GitHub workflows are live. Drop the macOS option entirely
unless it goes on a roadmap item.

---

## Tracking & ownership

- This file is owned alongside `PLAN.md`. When a bug is fixed, flip `[ ]` → `[x]` and append
  the PR/commit hash on the same line.
- New findings: append below this section with a fresh `B-NNN` id.
- If something is decided as "won't fix" by the owner, mark `[skip]` and add a one-line reason.
