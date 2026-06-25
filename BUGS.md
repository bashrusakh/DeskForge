# BUGS.md — Custom Client Builder workflow audit

> Tracker for issues found in the build-custom-agent end-to-end flow.
> Backend: `api/http/controller/admin/custom_build.go`, `api/service/custom_build.go`,
> `api/service/github_build_config.go`. Frontend: `admin-ui/src/views/custom-client/index.vue`,
> `admin-ui/src/views/server/github-build.vue`. Workflow: `github-build/rustqs-windows-min-test.yml`.
>
> Status legend: `[ ]` open · `[x]` fixed · `[~]` partial · `[skip]` won't fix (owner decision).
>
> Last audit: 2026-06-23 (fixed items removed; tracker lists only open work).

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

## STRUCTURAL — to enable B-001/B-002/B-013 fixes

### [~] B-012 · Build Linux + Android GitHub Actions workflows
**Partially done on branch `fix/build-linux-routing` (Linux):**
- Backend routing (build/vet-tested): `submitBuild` now dispatches `platform=linux` to GitHub
  (alongside `windows`); `tryGithubDispatch` picks the workflow per platform (`windows` →
  configurable `gcfg.WorkflowFilename`; `linux` → const `defaultLinuxWorkflowFilename`
  = `rustqs-linux.yml`) via a gcfg copy; `pollAndDownload` selects the artifact by platform
  (`rustdesk-min-test-linux`, with the single-artifact fallback from AU-L-011) and extracts the
  Linux bundle (all files, FileSize = largest) instead of looking for an `.exe`.
- **Draft workflow `github-build/rustqs-linux.yml`** mirrors the fork contract (enc_payload + L1
  config.rs + L2 allowCustom, both platform-independent; L3 brand adapted for Linux) and a
  Flutter-Linux x86_64 build ported from `rdgen/.github/workflows/generator-linux.yml`.
  **NOT yet validated by a real Actions run** — the build steps (vcpkg/flutter/build.py/
  packaging/artifact paths) need CI iteration like windows-min-test did.

**Android (branch `fix/build-android-routing`, stacked on the Linux branch):** backend extended
the same way (`submitBuild`/`tryGithubDispatch` const `defaultAndroidWorkflowFilename`
= `rustqs-android.yml`; `pollAndDownload` artifact `rustdesk-min-test-android`, shared
linux/android extract-all path). **Draft `github-build/rustqs-android.yml`** (single ABI arm64-v8a)
ported from `generator-android.yml` with the fork contract — also **NOT CI-validated**;
note the Android `custom_.txt` embedding is best-effort and needs verification.

Still open:
- **Push workflow files to fork** — `github-build/rustqs-linux.yml` and `github-build/rustqs-android.yml`
  need to be copied to `.github/workflows/` on the `rustqs/min-test` branch in the
  `bashrusakh/rustdesk` fork (same filenames — no rename needed, see `github-build/README.md`).
  Without this, `DispatchBuild` gets HTTP 404 and the build immediately fails.
- validate `rustqs-linux.yml` and `rustqs-android.yml` on Actions; re-expose Linux/Android in the
  UI (B-013) behind a feature flag once runs are green; optionally move the workflow names into
  `GithubBuildConfig` (consts for now).

**Where (new):** `github-build/rustqs-linux.yml`, `github-build/rustqs-android.yml`. Reference templates:
`rdgen/.github/workflows/generator-linux.yml`, `rdgen/.github/workflows/generator-android.yml`.
**Symptom:** today there's no GitHub path for Linux/Android, so submit goes to the deprecated
file queue (B-001). Owner direction (2026-06-20): mirror the windows-min-test approach for
these platforms; keep `entrypoint-linux.sh` as a manual fallback only.
**Fix:** push `github-build/rustqs-linux.yml` and `github-build/rustqs-android.yml`
to the fork's `.github/workflows/` (same filenames — no rename) on the `rustqs/min-test` branch;
then validate on Actions. Backend code (`submitBuild` + `tryGithubDispatch` + `pollAndDownload`)
is already merged — only the fork push is missing.

## ADMIN UI / API — open findings (consolidated from the removed `audit-report.md`)

The functional admin-UI audit (PR #19) had 65 findings; 58 were fixed in PR #20–#22.
The full report file was removed during doc consolidation (2026-06-21). The findings
still open are preserved here:

### [ ] AU-M-022 · Unauthenticated writes on the client-facing API
**Where:** `api/http/router/api.go` — routes registered before `frg.Use(RustAuth())` (line 76).
**Symptom:** `POST /api/sysinfo` (creates/updates `Peer` rows by caller-supplied `id`),
`/api/heartbeat`, `/api/audit/conn`, `/api/audit/file` are unauthenticated; an anonymous caller
can create/alter peers and inject audit entries. `/api/shared-peer` also does an unchecked
`(*j)["share_token"].(string)` assertion (`webClient.go:57`) → 500 on missing token.
**Fix:** needs RustDesk protocol design confirmation (the PC client hits these before auth).

### [ ] AU-L-010 · Hardcoded version list in Custom Client UI

## rdgen generator — open findings (consolidated from the removed `AUDIT.md`)

The custom-agent build workflow audit (Django `rdgen/` + Go `api/`) landed all its ✅ fixes.
The flagged-but-unfixed items are preserved here:

### [~] RD-B6 · `download` / `get_png` / `get_zip` are unauthenticated
**Hardened on branch `fix/rdgen-file-ttl`:** these endpoints are consumed by unauthenticated
callers by protocol (build runners fetch `get_png`/`get_zip`; users fetch the built exe), so a
token gate would break the flow. Instead each now refuses to serve files older than a TTL, so a
leaked UUID/filename is no longer a permanent capability: `download` 7 days (`RDGEN_EXE_TTL`),
`get_png` 6h (`RDGEN_PNG_TTL`), `get_zip` (encrypted secrets) 1h (`RDGEN_ZIP_TTL`); all env-tunable,
`<=0` disables. Path-traversal + UUID validation were already in place from an earlier audit.

Still open (needs design): true per-request auth via signed/expiring URLs generated server-side
and threaded through the workflows — a larger coordinated change across all URL-generation sites.

## Tracking & ownership

- This file is owned alongside `PLAN.md`. When a bug is fixed, flip `[ ]` → `[x]` and append
  the PR/commit hash on the same line.
- New findings: append below this section with a fresh `B-NNN` id.
- If something is decided as "won't fix" by the owner, mark `[skip]` and add a one-line reason.
