# Todo: Fix CI Workflows

**Plan:** `plans/fix-ci-workflow/plan.md`

---

## Status

- [x] **Step 1:** Fix `bridge.yml` in fork (3 branches)
  - [x] `rustqs/min-test` — commit `3bdd91b`
  - [x] `master` — commit `ed15f35`
  - [x] `rustqs/master-workflows` — commit `2e41c56`
- [x] **Step 2:** Update `rdgen/.github/workflows/bridge.yml`
- [x] **Step 3:** Add `version` to dispatch from admin
  - [x] `api/http/controller/admin/custom_build.go` — params
- [x] **Step 4:** Make `VERSION` dynamic in workflows
  - [x] `github-build/rustqs-windows-min-test.yml`
  - [x] `github-build/rustqs-linux.yml`
  - [x] `github-build/rustqs-android.yml`
  - [x] Fork `rustqs/min-test` (3 files) — commit `3fc9415`
  - [x] Fork `master` (3 files) — commit `be3ebc0`
  - [x] Fork `rustqs/master-workflows` — **skipped** (not used for dispatch)
- [x] **Step 5:** Dynamic version list
  - [x] `api/service/github_build_config.go` — `GetAvailableVersions()`
  - [x] `api/http/controller/admin/custom_build.go` — handler
  - [x] `api/http/router/admin.go` — route
  - [x] `admin-ui/src/api/custom_client.js` — `getVersions()`
  - [x] `admin-ui/src/views/custom-client/index.vue` — load from API
- [x] **Step 6:** Update `rdgen/.github/workflows/third-party-RustDeskTempTopMostWindow.yml` — **skipped** (fork version intentionally simplified)
- [x] **Validation:**
  - [x] Run workflow_dispatch — bridge job passes (in_progress vs startup_failure)
  - [x] Windows and Linux started
  - [ ] VERSION in logs matches admin selection (waiting for build to finish)
  - [ ] Version list in UI = only 1.4.8 and 1.4.7 (waiting for deploy)
  - [ ] Fallback works when GitHub API unavailable (waiting for deploy)
- [x] **Step 7:** Auto-sync workflow to fork
  - [x] `rdgen/.github/workflows/sync-workflows.yml` — DRAFT workflow (TEST)
  - [x] `api/service/github_build_config.go` — `SetSyncPatSecret()`
  - [x] `api/http/controller/admin/github_build_config.go` — `SyncPat()` handler
  - [x] `api/http/router/admin.go` — route `POST /sync_pat`
  - [x] `admin-ui/src/api/github_build_config.js` — `syncPat()`
  - [x] `admin-ui/src/views/server/github-build.vue` — "Sync PAT to CI" button
- [x] **Update docs:**
  - [x] `github-build/README.md`
  - [x] `PLAN.md §7` — added: note about auto-appearing versions, 3-branch deploy section, bridge.yml warning
  - [x] `offline-kit/FORK-PROCEDURE.md` — added C1 (versions in UI) and C2 (workflow deployment)
