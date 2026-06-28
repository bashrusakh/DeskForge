# Plan: Fix CI Workflows and Version Management

**Date:** 2026-06-28
**Status:** 🟡 Implemented — pending live workflow validation
**Priority:** High (workflows were failing)

---

## 1. Context and Problem

### 1.1. What broke

Commit `02541269` (message `"feat: update bridge.yml"`) in the `bashrusakh/rustdesk` fork on branch `rustqs/min-test` added to `bridge.yml`:

```yaml
on:
  workflow_call:
    inputs:
      version:
        required: true
        default: '1.3.1'
        type: string
```

**Why it failed:**
- `required: true` requires **explicit input** from the caller
- Parent workflows (`rustqs-*.yml`) call bridge **without `with:`**
- DeskForge admin does not pass `version` in the dispatch
- GitHub Actions validates reusable workflows at startup → `startup_failure` in 1-2 sec, 0 jobs

### 1.2. What was wrong with versions

- UI (`custom-client/index.vue`) had hardcoded `VERSIONS = ['1.4.8','1.4.7','1.4.6',...'1.3.3']` — 13 versions
- Fork only has assets (`offline-assets-*`) for `1.4.7` and `1.4.8`
- `VERSION: "1.4.8"` was hardcoded in every `rustqs-*.yml`
- bridge.yml checked out **upstream** `rustdesk/rustdesk` by tag instead of the fork — desync with build code

### 1.3. How bridge works in upstream

In `rustdesk/rustdesk`:
- `bridge.yml` — **no** `inputs.version`, checkout **its own repo** (without `repository:`)
- `flutter-build.yml` calls bridge **without `with:`**
- bridge and build work from **the same code**

Our fork BEFORE commit `02541269` did the same. The commit broke this model.

---

## 2. Fork Branch Architecture

```
bashrusakh/rustdesk
│
├── master                          # 1:1 mirror of upstream rustdesk/rustdesk:master
│   ├── .github/workflows/bridge.yml
│   ├── .github/workflows/rustqs-*.yml
│   └── ... (all upstream code)
│
├── rustqs/master-workflows         # Workflow copy for applying to master after upstream sync
│   └── .github/workflows/
│       ├── bridge.yml
│       ├── rustqs-*.yml
│       └── third-party-*.yml
│
└── rustqs/min-test                 # Execution branch — all dispatches go here
    ├── .github/workflows/           #   our L1/L2/L3 patches live here
    │   ├── bridge.yml
    │   ├── rustqs-*.yml
    │   └── third-party-*.yml
    └── ... (fork code with patches)
```

**Important:**
- `master` — needed for API discovery (GitHub requires workflow on default branch)
- `rustqs/min-test` — **only** branch where admin sends dispatches
- `rustqs/master-workflows` — backup copy so workflow files aren't lost after upstream merge

---

## 3. What Was Done

### 3.1. Fixed `bridge.yml` — restored upstream pattern

**Changed in (3 fork branches):**
- `bashrusakh/rustdesk:master` — commit `ed15f35`
- `bashrusakh/rustdesk:rustqs/master-workflows` — commit `2e41c56`
- `bashrusakh/rustdesk:rustqs/min-test` — commit `3bdd91b`

**What changed:**
- Removed `inputs.version` entirely
- Removed `if: ${{ inputs.version != 'master' }}` / `if: ${{ inputs.version == 'master' }}`
- Restored checkout **without `repository:`** — checkout current repo (fork)
- Restored matrix like upstream (2 jobs: default + flutter 3.44 for arm64)

**Why:** bridge and build must work from the same code. Like upstream.

### 3.2. Synced `rdgen/.github/workflows/bridge.yml`

Brought in line with the fork version (without `inputs.version`).

### 3.3. `version` is now passed from admin to workflow

**File:** `api/http/controller/admin/custom_build.go`

Added `"version": b.Version` to the params map in `tryGithubDispatch()`.

### 3.4. `VERSION` in workflows — dynamic, from dispatch

**Changed in:**
- `github-build/rustqs-windows-min-test.yml`
- `github-build/rustqs-linux.yml`
- `github-build/rustqs-android.yml`
- Fork `rustqs/min-test` (3 files) — commit `3fc9415`
- Fork `master` (3 files) — commit `be3ebc0`

**What changed:**
- Added `version` to `workflow_dispatch` inputs (optional, `required: false`)
- Replaced `env.VERSION` from `"1.4.8"` to `${{ inputs.version || '1.4.8' }}`
- Added `RQS_VERSION` to the decrypt step (from `enc_payload`)
- Added **Override VERSION from dispatch payload** step — if `RQS_VERSION` is not empty, it overwrites `VERSION` in `$GITHUB_ENV`

**Flow:**
```
enc_payload (base64) → decrypt → RQS_VERSION=1.4.8
  → Override VERSION from dispatch payload
    → VERSION=1.4.8 (overrides workflow-level default)
```

### 3.5. Version list — dynamic, from GitHub API

**New endpoint:** `GET /api/admin/custom_build/versions`

**Logic:**
```
GET /repos/bashrusakh/rustdesk/releases?per_page=100
→ filter: tag_name starts with "offline-assets-"
→ parse: tag_name.replace("offline-assets-", "")
→ sort by semver (desc)
→ return ["1.4.8", "1.4.7"]
```

**Changed in:**
- `api/service/github_build_config.go` — `GetAvailableVersions()`, `fetchReleases()`, `compareSemver()`, 5 min cache
- `api/http/controller/admin/custom_build.go` — `Versions()` handler
- `api/http/router/admin.go` — route `GET /custom_build/versions`
- `admin-ui/src/api/custom_client.js` — `getVersions()`
- `admin-ui/src/views/custom-client/index.vue` — load from API on mount, removed hardcoded `VERSIONS`

**Cache:** 5 min TTL on the API side. UI re-fetches on page load, so a freshly published release may take up to 5 minutes to appear until the cache expires.

**Empty / API down:** if GitHub API is unavailable or no `offline-assets-*` releases exist, the API returns an empty list (no fake fallback). The UI shows `VersionListError` / `VersionListEmpty` and `StartBuild` stays disabled — the operator must wait for the API to recover or publish a release.

### 3.6. `rdgen/.github/workflows/third-party-RustDeskTempTopMostWindow.yml`

**Not changed.** The fork version (for min-test) is intentionally simplified (no encrypted-secrets, privacy screen). `rdgen/` is a vendored reference of the upstream version. The divergence is intentional.

---

## 4. Validation

| Check | Result |
|---|---|
| `bridge.yml` — workflow_dispatch without `startup_failure` | ✅ Windows and Linux — `in_progress` (previously `startup_failure` in 1-2 sec) |
| `VERSION` in logs matches dispatch | ✅ `VERSION overridden from dispatch: 1.4.8` (from `RQS_VERSION`) |
| Version list in UI | 🟡 Pending live deploy — should show `1.4.8` and `1.4.7` |
| Empty/error states (no GitHub releases / API down) | 🟡 Pending live deploy — UI shows `VersionListEmpty` / `VersionListError`, `StartBuild` disabled |
| `sync-workflows.yml` manual `workflow_dispatch` (DRAFT) | 🟡 Pending first run (push trigger intentionally off until validated) |
| `sync-workflows.yml` automated push after validation | ⬜ Out of scope for this PR — gated on successful manual run |
| `go build -o /tmp/apimain cmd/apimain.go` | ✅ Clean |
| `go test ./service/...` | ✅ Pass (incl. `TestCompareSemver`) |
| `npm install && npm run build` | ✅ Pass |
| `go vet ./...` | ✅ Only pre-existing `Fatalf` warnings in cache tests |

---

## 5. Files Changed

### In fork `bashrusakh/rustdesk`

| Branch | File | Commit |
|---|---|---|
| `master` | `.github/workflows/bridge.yml` | `ed15f35` |
| `rustqs/master-workflows` | `.github/workflows/bridge.yml` | `2e41c56` |
| `rustqs/min-test` | `.github/workflows/bridge.yml` | `3bdd91b` |
| `master` | `.github/workflows/rustqs-*.yml` (3 files) | `be3ebc0` |
| `rustqs/min-test` | `.github/workflows/rustqs-*.yml` (3 files) | `3fc9415` |

### In DeskForge

| File | Change |
|---|---|
| `rdgen/.github/workflows/bridge.yml` | Removed `inputs.version`, restored upstream pattern |
| `github-build/rustqs-windows-min-test.yml` | `version` input, `VERSION` from inputs, decrypt/override |
| `github-build/rustqs-linux.yml` | Same |
| `github-build/rustqs-android.yml` | Same |
| `api/service/github_build_config.go` | `GetAvailableVersions()` (singleflight + detached ctx), `fetchReleases()` (response body in error), `compareSemver()` (pre-release segments), 5 min cache |
| `api/service/github_build_config_test.go` | `TestCompareSemver` unit test |
| `api/http/controller/admin/custom_build.go` | `Versions()` handler (returns `[]string{}` on error), `version` in params, `ValidateBuildVersion()`, early validation in `Create` |
| `api/http/router/admin.go` | Route `GET /custom_build/versions`, route `POST /sync_pat` |
| `admin-ui/src/api/custom_client.js` | `getVersions()` |
| `admin-ui/src/api/github_build_config.js` | `syncPat()` |
| `admin-ui/src/views/custom-client/index.vue` | Removed hardcoded VERSIONS, load from API, `StartBuild` gated until load completes, preset version gated during load, i18n keys `VersionListLoading`/`Empty`/`Error` |
| `admin-ui/src/views/server/github-build.vue` | "Sync PAT to CI" button |
| `admin-ui/src/utils/i18n/{en,ru,zh_CN}.json` | `VersionListLoading`, `VersionListEmpty`, `VersionListError` keys |
| `rdgen/.github/workflows/sync-workflows.yml` | DRAFT auto-sync: manual `workflow_dispatch` only, per-branch working-tree wipe, idempotency check, retry on non-fast-forward, per-command `set -e` recovery, prune deleted workflow files, empty `git commit-tree` guard, PAT via `http.extraheader` |
| `github-build/README.md` | Updated Architecture + Version flow + bridge.yml section |
| `go.mod`, `go.work`, `api/go.mod` | Added root `go.mod` / `go.work` for monorepo tooling, aligned `go 1.25.0`, added `golang.org/x/sync` direct dep |

### In PR #57 (DeskForge: `ocr-fixes`)

| Commit | Notes |
|---|---|
| `996cc00` | OCR fixes — singleflight cache, version validation at create, sync-workflow reliability, i18n keys, `compareSemver` unit test, `go.work` sync |
| `e9414e7` | Drop fallback versions (API returns `[]string{}` on failure), detach singleflight context from caller, prune stale workflow files on retry, empty `commit-tree` guard |

---

## 6. Adding a New Version (remains manual)

When a new upstream version ships (e.g., 1.5.0):

1. **Fork:** `git fetch upstream --tags && git push origin v1.5.0`
2. **Branch:** `git checkout rustqs/min-test && git rebase v1.5.0`
3. **Assets:** `offline-kit/freeze.sh` → `gh release create offline-assets-1.5.0 ...`
4. **Versions:** After publishing the release, version `1.5.0` will automatically appear in the UI (the `/versions` endpoint picks it up from the GitHub API)

No hardcoded values in UI or YAML need to be changed.

---

## 7. Documentation Updated

| Document | Status | What changed |
|---|---|---|
| `github-build/README.md` | ✅ Updated | Architecture + Version flow + bridge.yml |
| `PLAN.md §7` | ✅ Updated | Added note about auto-appearing versions, 3-branch deploy section, bridge.yml warning |
| `offline-kit/FORK-PROCEDURE.md` | ✅ Updated | Added C1 (versions in UI) and C2 (workflow deployment) |
