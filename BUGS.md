# BUGS.md — Custom Client Builder workflow audit

> Tracker for issues found in the build-custom-agent end-to-end flow.
> Backend: `api/http/controller/admin/custom_build.go`, `api/service/custom_build.go`,
> `api/service/github_build_config.go`. Frontend: `admin-ui/src/views/custom-client/index.vue`,
> `admin-ui/src/views/server/github-build.vue`. Active workflow source: the configured
> RustDesk fork's `.github/workflows/rustqs-windows-min-test.yml`; local `github-build/`
> workflow files are reference material only.
>
> Status legend: `[ ]` open · `[x]` fixed · `[~]` partial · `[skip]` won't fix (owner decision).
>
> Last audit: 2026-06-23. Current file contents are a bounded tracker snapshot:
> 1 fixed, 4 partial, and 1 open entry; older aggregate counts remain in dated
> changelog history.
> Current-state reconciliation: 2026-08-10. Historical findings below are labeled
> where the current provider-only path supersedes the old queue behavior; no live
> provider or clean-build evidence is implied.

---

## Historical architectural mismatch with PLAN.md §3

The original audit recorded a mismatch with the standalone / Docker build agents marked
**frozen as fallback** (§8.3, §8.4). The current source now checks provider readiness
before persistence and does not route production submissions into the file queue. The
queue scripts remain frozen historical material. Owner decision (2026-06-20):

1. Treat Docker `build-linux` and `build-win` containers as **frozen manual/historical-only** material, not the
   default route. They stay on disk but should not be started by `docker compose up`.
2. Remove `windows-x86` (32-bit) as a build target everywhere — UI option, form defaults,
   any router branches. 2026; not worth maintaining.
3. Keep the fork-owned Linux + Android workflow mappings, but do not re-expose those
   platforms until PR11 has real end-to-end evidence. Until then, non-Windows platforms
   remain gated in the UI/API to prevent phantom builds.

The bugs below are grouped by where they leak into user-visible breakage.

---

## Historical critical finding — workflow was silently broken end-to-end

### [~] B-001 · Historical file-queue jobs do not propagate `done` status back to the DB
**Current state:** provider readiness is checked before a production build row is persisted,
so the file-queue path is not a production fallback. `docker/docker-compose.yml` keeps
`build-linux` and `build-win` behind a `fallback` profile. The frozen scripts still have
the status-mirror limitation if an operator runs them manually; no live provider evidence
is implied by this closure boundary.

**Current path:** `api/http/controller/admin/custom_build.go:1246-1261` dispatches
production builds through the configured provider; the old queue references remain
only in the frozen scripts below. **Historical queue locations:**
`docker/entrypoint-linux.sh:37,49,76,...` and `docker/entrypoint-win.sh:33,45,210`
write `output_dir/status`.
**Historical symptom:** Linux/Android (and Windows when GitHub config was missing) builds
sat at `Status=pending` forever. `DownloadByKey` returned HTTP 409. The Download button
did not appear in the UI (`v-if="row.status === 'done'"`, `custom-client/index.vue:306`).
**Root cause:** no Go-side watcher reads `/rdgen-data/output/{id}/status`. The build agent's
status file is dead-letter.
**Fix path (per owner direction):**
- Current: keep all non-Windows-via-GitHub options gated in the UI/API (B-002, B-013).
- Future: enable Linux/Android only after fork workflow, artifact, embedding, and download
  evidence is recorded under B-012/PR11.

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
**Current state:** the API has fork-owned filename mappings, but its production capability
gate rejects Linux and Android until PR11 validates the complete workflow and artifact path.
No live provider/workflow run or clean build is recorded in the current canonical plan.

**Historical audit record:**

**Backend:** merged (PR #44 backend routing: `submitBuild` dispatches `platform=linux`/`android`
by workflow constant; `tryGithubDispatch` picks `rustqs-linux.yml`/`rustqs-android.yml`;
`pollAndDownload` selects artifact by platform).

**Workflow files:** deployed to `bashrusakh/rustdesk` on both `master` (API discovery)
and `rustqs/min-test` (execution) — all three are indexed (HTTP 200):
`rustqs-windows-min-test.yml`, `rustqs-linux.yml`, `rustqs-android.yml`.
Filenames in Go constants match fork filenames exactly.
These deployment observations are historical; they are not current provider-run,
artifact, package, or support evidence.

**Critical dependency:** `bridge.yml` must also exist on both branches — all three
`rustqs-*.yml` reference it as a reusable workflow. Without it, dispatch succeeds
but the run fails with a parse error (422).

Still open:
- validate `rustqs-linux.yml` and `rustqs-android.yml` on real Actions runs (build steps:
  vcpkg/flutter/build.py/packaging/artifact paths — need CI iteration like windows-min-test did)
- Android `custom_.txt` runtime-path and fail-closed packaging checks have local static
  evidence; no live APK/package/install/runtime evidence exists
- re-expose Linux/Android in the UI (B-013) behind a feature flag once runs are green

**Where (active source):** the configured RustDesk fork's
`.github/workflows/rustqs-linux.yml` and `.github/workflows/rustqs-android.yml`.
**Historical/reference templates:** former local `github-build/` workflow references and
`rdgen/.github/workflows/generator-linux.yml`, `rdgen/.github/workflows/generator-android.yml`.
The `github-build/` and `rdgen/` workflow material is not the executable source.
**Symptoms (historical):** before the push, dispatch returned HTTP 404 because workflow files
were not on `master` (default branch); submit went to the deprecated file queue (B-001).
Resolved by pushing workflow files to both `master` and `rustqs/min-test`.

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

### [x] AU-L-010 · Hardcoded version list in Custom Client UI
Resolved in the current source by the provider-derived version catalog; unavailable
provider catalog data returns an empty/error state rather than an obsolete hardcoded
version fallback. Live provider catalog evidence remains unverified.

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
