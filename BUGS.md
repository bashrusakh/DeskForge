# BUGS.md — Custom Client Builder workflow audit

> Tracker for issues found in the build-custom-agent end-to-end flow.
>
> **Surfaces:** backend `api/http/controller/admin/custom_build.go`,
> `api/service/custom_build.go`, `api/service/github_build_config.go`; frontend
> `admin-ui/src/views/custom-client/index.vue`, `admin-ui/src/views/server/github-build.vue`.
>
> **Workflow path:** Windows workflow file `.github/workflows/rustqs-windows.yml`, artifact
> `rustqs-windows`. Workflow ownership and the `rustqs/workflows` backup-ref definition (not a
> dispatch selector; legacy read-only compatibility): `github-build/README.md` (canonical) and
> `docs/FORKS-AND-PINS.md` §1. Status legend: `[ ]` open · `[x]` fixed · `[~]` partial ·
> `[skip]` won't fix (owner decision). **Last audit:** 2026-06-23; current file contents are a
> bounded tracker snapshot: 1 fixed, 4 partial, and 1 open entry (older aggregate counts remain in
> dated changelog history).
>
> **Current-state reconciliation:** 2026-08-10. Historical findings below are labeled where the
> current provider-only path supersedes the old queue behavior; no live provider or clean-build
> evidence is implied, and older branch/workflow names are historical records that do not describe
> the current executable path.

---

## Historical architectural mismatch with PLAN.md's architecture section

> Note 2026-11-24: the `PLAN.md` references in this section (§3, §8.3, §8.4) use the 2026-06-era numbering and do not resolve in the current `PLAN.md`. Name-based current homes: `PLAN.md` "Historical/frozen builders (not used by the current API)" and the frozen builder rows of "Component status".

Tombstone: the mismatch (frozen fallback builders vs the provider path) is resolved in the current source, which checks provider readiness before persistence and does not route production submissions into the file queue. Owner decision (2026-06-20), kept complete: (1) Docker `build-linux`/`build-win` remain **frozen manual/historical-only** material — on disk, but not started by `docker compose up`; (2) `windows-x86` (32-bit) removed as a build target everywhere (UI option, form defaults, router branches) — 2026, not worth maintaining; (3) keep the fork-owned Linux + Android workflow mappings, but do not re-expose those platforms until PR11 has real end-to-end evidence — until then non-Windows platforms stay gated in the UI/API to prevent phantom builds. Current homes: `PLAN.md` §3 (frozen builder rows), §9 (`windows-x86` dead), §2 (Linux/Android gated).

The bugs below are grouped by where they leak into user-visible breakage.

---

## Historical critical finding — workflow was silently broken end-to-end

### [~] B-001 · Historical file-queue jobs do not propagate `done` status back to the DB

**Tombstone (historical):** no Go-side watcher read `/rdgen-data/output/{id}/status` — the build agent's status file was dead-letter, so Linux/Android builds (and Windows when GitHub config was missing) sat at `pending` forever (`DownloadByKey` HTTP 409, no Download button; UI gated on `row.status === 'done'`, `custom-client/index.vue:672`; status writes: `docker/entrypoint-linux.sh:37,49,76`, `docker/entrypoint-win.sh:33,45,210`). Superseded as a production path: provider readiness is now checked before a build row is persisted (`api/http/controller/admin/custom_build.go:1461,1470` → `tryGithubDispatch`), and `docker/docker-compose.yml` keeps `build-linux`/`build-win` behind the `fallback` profile; the frozen scripts (`docker/entrypoint-linux.sh`, `docker/entrypoint-win.sh`) still have the status-mirror limitation if run manually. Chronology: CHANGELOG.md ("Current-state reconciliation — 2026-08-10"). Enable Linux/Android only after fork workflow, artifact, embedding, and download evidence is recorded under B-012/PR11.

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

**Where:** only that public route remains —
`GET /api/admin/custom_build/public/detailByKey/:key` (the other unused surfaces
listed above were deleted in the partial fix; the original `audit-report.md`
reference was removed with that file on 2026-06-21).

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

**Workflow files:** deployed to `bashrusakh/rustdesk` on both `master` (API discovery) and
`rustqs/min-test` (execution — historical; that branch no longer exists, absent from the fork's
branches as of 2026-11-24) — all three indexed (HTTP 200, as of the 2026-06 audit):
`rustqs-windows-min-test.yml`, `rustqs-linux.yml`, `rustqs-android.yml`; filenames in Go constants
match fork filenames exactly. These deployment observations are historical, not current
provider-run, artifact, package, or support evidence.

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

**Symptoms (historical):** before the push, dispatch returned HTTP 404 because workflow files were not on `master` (default branch) and submit went to the deprecated file queue (B-001); resolved by pushing workflow files to `master` and `rustqs/min-test` (historical — `rustqs/min-test` no longer exists as of 2026-11-24).

## ADMIN UI / API — open findings (consolidated from the removed `audit-report.md`)

The functional admin-UI audit (PR #19) had 65 findings; 58 were fixed in PR #20–#22, and the full report file was removed during doc consolidation (2026-06-21). The findings still open are preserved here:

### [ ] AU-M-022 · Unauthenticated writes on the client-facing API

**Where:** `api/http/router/api.go` — routes registered before `frg.Use(RustAuth())`
(line 76).

**Symptom:** `POST /api/sysinfo` (creates/updates `Peer` rows by caller-supplied `id`),
`/api/heartbeat`, `/api/audit/conn`, `/api/audit/file` are unauthenticated; an
anonymous caller can create/alter peers and inject audit entries. `/api/shared-peer`
also does an unchecked `(*j)["share_token"].(string)` assertion (`webClient.go:57`)
→ 500 on missing token.

**Fix:** needs RustDesk protocol design confirmation (the PC client hits these before
auth).

### [x] AU-L-010 · Hardcoded version list in Custom Client UI
Resolved in the current source by the provider-derived version catalog; unavailable provider catalog data returns an empty/error state rather than an obsolete hardcoded version fallback. Live provider catalog evidence remains unverified.

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
