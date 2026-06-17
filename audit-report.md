# Functional Audit Report — DeskForge Admin UI

**Date:** 2026-06-17  
**Methodology:** Full-stack trace from UI action → API call → service → persistence, cross-referenced with source code at every layer.  
**Last update:** Second-pass verification re-audit (parallel sub-agents). Corrected 6 over-stated/wrong findings (H-001, H-004, H-007, H-009, L-006, M-004), fixed the endpoint cross-reference tables, and added 17 new verified findings (H-010, H-011, S-002, M-016–M-022, L-020–L-026). See the "Second-Pass Additions" section and the changelog note below.

---

## Summary

| Category     | Count | (1st pass) |
| ------------ | ----- | ---------- |
| Critical     | 4     | 4          |
| High         | 8     | 9          |
| Medium       | 24    | 15         |
| Low / Info   | 27    | 19         |
| Security     | 2     | 1          |
| **Total**    | **65**| **48**     |

Pages checked: 40+ (all views, components, dialogs).  
API endpoints verified: 80+.  
Buttons/actions/forms traced end-to-end: 150+.

> **Re-audit changelog (2nd pass).** *Corrected (over-stated/incorrect):* **H-001** (token batch-delete is admin-gated, not open to any user → Medium), **H-004** (export bug is raw-JSON + null `.toString()` crash, not `[object Object]`), **H-007** (branding URLs *do* round-trip; the `custom_*` keys are dead code → Low), **H-009** (the exposed key is the **public** key, not a secret → Medium), **L-006** (tags aren't "always empty" — the dropdown just never loads), **M-004** (total-failure case is silent, not a false "success"). *Table fixes:* `address_book/batchCreate` **is** used (removed from "unused"); `rule/batchCreate` and `user/myPeer` are frontend-wrappers with **no backend route** (moved to "promised but missing"). *Added:* 17 new findings (see "Second-Pass Additions"). The headline new bug is **H-010** — Address Book bulk-delete silently no-ops.

---

## Critical Issues

### C-001 · Server Settings Volatile — All Runtime Changes Lost on Restart

**Page:** Server → Server Commands (Simple tab)  
**Elements:** RELAY_SERVERS Save/Refresh, ALWAYS_USE_RELAY Save/Toggle, MUST_LOGIN Save/Toggle, Blocklist Add/Delete, Blacklist Add/Delete

**Expected:** Settings saved through the UI persist across container restarts.

**Actual:** All settings are applied only to the Rust server's in-memory state. The Go API is a pure TCP proxy (`api/service/serverCmd.go:43-87`) — it stores nothing. On restart:
- RELAY_SERVERS reverts to `RELAY_SERVERS` env var
- ALWAYS_USE_RELAY reverts to `ALWAYS_USE_RELAY` env var
- MUST_LOGIN reverts to `must-login` CLI arg or `MUST_LOGIN` env var
- Blocklist/Blacklist runtime additions are lost; runtime removals are restored from file

The UI shows "Operation Success" with zero indication settings are volatile.

**Evidence:**
- `server/src/rendezvous_server.rs:193-233` — all three settings read from env/CLI at startup, modified in RAM only
- `server/src/relay_server.rs:51-80` — blacklist/blocklist read from files at startup, NEVER written back
- `api/service/serverCmd.go:43-87` — Go API opens TCP socket, sends text, reads response; no persistence
- `docker/Dockerfile:117-119` — env vars at build time, never updated by API

**Root cause:** No persistence layer between Go API and Rust server. The TCP command interface is fire-and-forget with no write-back to files, DB, or config.

**Fix:**
1. Add a persistence layer — write settings to config file, DB, or env file
2. Have Docker entrypoint capture/re-apply runtime settings on startup
3. **Minimum:** Add clear UI warning: "These settings are runtime-only and will reset on server restart."

**Status:** Critical

---

### C-002 · `always_use_relay` Toggle Destroys Relay Servers List

**Page:** Server → Server Commands → ALWAYS_USE_RELAY  
**Element:** Toggle switch → Save

**Expected:** Toggle only changes the always-use-relay flag.

**Actual:** The Rust handler sends `Data::RelayServers0(rs)` where `rs` is `"Y"` or `"N"` (the toggle value, not relay server addresses). This triggers `parse_relay_servers("Y")`, which calls `get_servers()` that tries DNS resolution on `"Y"` — it fails, producing an **empty relay servers list**.

A frontend workaround exists (`control.vue:187-189` re-saves relay servers after each aur save), but a race window exists where relay servers are empty. If the re-save fails (network error, etc.), relay servers remain broken until manual intervention.

**Evidence:**
- `server/src/rendezvous_server.rs:1244-1251` — `self.tx.send(Data::RelayServers0(rs.to_owned())).ok()` where `rs` is the toggle value
- `server/src/common.rs:39-47` — `get_servers()` splits on comma, tries `to_socket_addrs()` on each part
- `admin-ui/src/views/rustdesk/control.vue:186-189` — workaround comment confirms the bug exists

**Root cause:** `aur` handler incorrectly reuses `Data::RelayServers0` channel for a non-relay-server value.

**Fix:** Remove `self.tx.send(Data::RelayServers0(rs.to_owned())).ok()` from the `aur` handler.

**Status:** Critical

---

### C-003 · File Upload — Path Traversal + Bypassable Content-Type Validation

**Page:** Custom Client Builder → Upload icon/logo/privacy-screen  
**Element:** Upload buttons

**Expected:** Only PNG files uploaded to safe, sandboxed paths with proper validation.

**Actual:**
1. **Path traversal:** `file.Filename` from HTTP multipart upload is used UNSANITIZED in the destination path (`api/http/controller/admin/file.go:82`). A filename like `../../etc/crontab` writes outside the intended directory.
2. **Bypassable type check:** Content-Type validation at `file.go:71-77` checks `if ct != "" && ct != "image/png"` — an empty Content-Type completely bypasses the check. Content-Type is client-supplied and trivially spoofable.
3. **No magic-byte inspection:** Actual file content is never verified.
4. **No file size limit:** DoS via arbitrarily large uploads.

**Evidence:**
- `api/http/controller/admin/file.go:82` — `dst := path + file.Filename`
- `api/http/controller/admin/file.go:71-77` — empty-CT bypass condition

**Fix:**
1. Use `filepath.Base(file.Filename)` for sanitization
2. Add magic-byte PNG header verification
3. Add file size limit
4. Fix the empty-Content-Type bypass condition

**Status:** Critical

---

### C-004 · My Devices — Delete Functionality Entirely Missing

**Page:** `/my/devices` (My Profile → My Devices)  
**Elements:** Row actions, toolbar actions

**Expected:** Users can delete their own device records (single and bulk).

**Actual:** Single-delete and batch-delete code blocks are **commented out** in the frontend. No delete buttons appear in the UI. No delete API endpoints exist for the `/my/peer` scope. Only `GET /my/peer/list` is registered.

**Evidence:**
- `admin-ui/src/views/my/peer/index.vue:250-265` — single delete handler `del` commented out
- `admin-ui/src/views/my/peer/index.vue:350-369` — bulk delete handler `toBatchDelete` commented out
- `admin-ui/src/api/my/peer.js` — only exports `list`; no `remove` or `batchRemove`
- `api/http/controller/admin/my/peer.go:31-58` — only `List` handler, no `Delete`
- `api/http/router/admin.go:325-328` — only `GET /my/peer/list`

**Root cause:** The "my" profile peer management was designed as read-only. The UI layout was copied from the admin peer page, and the delete code was never completed.

**Fix:**
1. Add `POST /admin/my/peer/delete` and `POST /admin/my/peer/batchDelete` endpoints with ownership validation (`WHERE user_id = ?`)
2. Add service methods with authorization checks
3. Add API client functions in `admin-ui/src/api/my/peer.js`
4. Uncomment and connect the frontend handlers

**Status:** Critical

---

## High Issues

### H-001 · `user_token/batchDelete` Lacks Per-Record Authorization

**Page:** Security → API Tokens → Batch Delete  
**Element:** Batch Logout button

**Expected:** Only admin or token owner can revoke tokens.

**Actual:** Single-delete (`POST /user_token/delete`) checks `IsAdmin || l.UserId == u.Id`. Batch delete (`POST /user_token/batchDelete`) has **no per-record authorization check** — it deletes any IDs passed.

> **Revised (2nd pass):** The original wording ("any authenticated user can batch-revoke any tokens") was **incorrect**. The entire `user_token` group is gated by `AdminPrivilege()` (`api/http/router/admin.go:250-256`), so only admins reach either endpoint. The real defect is a least-privilege asymmetry (an admin can batch-revoke *any* token, while single-delete is owner/admin-scoped) — not a non-admin privilege escalation. Severity downgraded **High → Medium**.

**Evidence:**
- `api/http/controller/admin/userToken.go:96` — `BatchDeleteUserToken(ids)` called without userId extraction
- `api/http/controller/admin/userToken.go:66-80` — single delete has auth check; batch does not
- `api/http/router/admin.go:250-256` — `rg.Group("/user_token").Use(middleware.AdminPrivilege())` — both routes admin-only

**Fix:** Add `userId` scope filter to `BatchDeleteUserToken` for consistency with single-delete.

**Status:** Medium (revised from High)

---

### H-002 · User Delete — Last-Admin Race Condition

**Page:** Users → Delete Selected  
**Element:** Delete Selected button

**Expected:** Cannot delete the last admin user under any circumstances.

**Actual:** `getAdminUserCount()` check runs BEFORE the transaction. Two simultaneous admin deletes both pass the count check and both proceed, potentially deleting all admins.

**Evidence:**
- `api/service/user.go:210-213` — admin count check, outside transaction
- `api/service/user.go:215-230` — transaction starts AFTER count check

**Fix:** Move admin count check inside the transaction.

**Status:** High

---

### H-003 · CSV Peer Import — Total Silence on Partial Failure

**Page:** Devices → Import  
**Element:** CSV Import

**Expected:** Feedback on how many peers imported successfully and which failed, with reasons.

**Actual:** `Promise.all` with `.catch(_ => false)` swallows everything. If 7 of 10 peers import and 3 fail (duplicate IDs or invalid data), the user sees **nothing** — no success toast, no error, no count. Must manually refresh to discover results.

**Evidence:** `admin-ui/src/views/peer/index.vue:433-457` — catch returns false silently

**Fix:** Use `Promise.allSettled()`, report success/fail counts and per-row error details.

**Status:** High

---

### H-004 · CSV Export — Unparsed `info` JSON + `.toString()` Crash on Null Cells

**Page:** Monitoring → File Transfer / Connection History → Export  
**Element:** Export button

**Expected:** Readable, complete CSV export.

**Actual (revised 2nd pass):** The original "`[object Object]`" diagnosis was **wrong**. `toExport()` performs a *separate* fetch (`fileList(q)`) and feeds that raw response straight into `jsonToCsv` — it does **not** reuse the parsed list, so `info` exports as the raw JSON **string**, not `[object Object]`. The more serious, real defect is that `jsonToCsv` calls `row[key].toString()` with no null guard — any `null`/`undefined` cell throws `TypeError` and aborts the entire export. So the export is fragile (raw JSON for `info`) and can hard-fail on any null field.

**Evidence:**
- `admin-ui/src/views/audit/reponsitories.js` — `getList` mutates only the on-screen list; `toExport` re-fetches unparsed data
- `admin-ui/src/utils/file.js:41` — `jsonToCsv` does `row[key].toString()` with no null/undefined guard

**Fix:** Null-guard each cell in `jsonToCsv` (`row[key] == null ? '' : String(row[key])`); `JSON.stringify()` the `info` field (or pretty-extract its keys) before export.

**Status:** High (mechanism corrected)

---

### H-005 · Address Book Collection Delete — Cascading Data Loss Without Warning

**Page:** Address Book → Collections → Delete Selected  
**Element:** Delete Selected button

**Expected:** Confirmation dialog warns that all entries and sharing rules within the collection will be cascade-deleted.

**Actual:** Confirmation says only "Delete (N) Collections" with no mention of cascade. `DeleteCollection` (`api/service/addressBook.go:281-288`) runs a transaction deleting all rules, address book entries, then the collection itself.

**Fix:** Add explicit warning: "Deleting this collection will also permanently remove ALL address book entries and sharing rules within it."

**Status:** High

---

### H-006 · Custom Client Preset — All 13 Permission Settings Silent Data Loss

**Page:** Custom Client Builder  
**Element:** Save as Preset + Load Preset

**Expected:** All build settings including permissions round-trip through preset save/load.

**Actual:** `submitBuild()` includes 13 permission fields in `custom_json`. `saveCurrentAsPreset()` saves **none** of them. `loadPresetIntoForm()` restores **none** of them. All permission toggles silently reset to defaults on preset load.

**Evidence:**
- `admin-ui/src/views/custom-client/index.vue:439-468` — save function missing all 13 permission fields
- `admin-ui/src/views/custom-client/index.vue:523-559` — submit function includes all permissions
- `admin-ui/src/views/custom-client/index.vue:374-434` — load function missing permissions

**Fix:** Synchronize the field lists between save, load, and submit functions.

**Status:** High

---

### H-007 · Custom Client Preset — Branding Images Field Name Mismatch

**Page:** Custom Client Builder  
**Element:** Save as Preset + Load Preset

**Expected:** Icon, logo, and privacy screen URLs preserved and restored in presets.

**Actual (revised 2nd pass):** **Overstated.** Save writes the bogus `custom_app_icon_url`/`custom_app_logo_url`/`custom_privacy_screen_url` keys (always `undefined`, dropped by `JSON.stringify`) **but also writes the correct `app_icon_url`/`app_logo_url`/`privacy_screen_url`** (`index.vue:466-468`), and Load reads the correct keys (`index.vue:413`). So branding images **do** round-trip correctly; the `custom_*` keys are inert dead code, not data loss. This is a code-smell, not the High-severity data-loss bug originally claimed.

**Evidence:**
- `custom-client/index.vue:458-460` — saves dead `custom_app_icon_url` keys (always undefined)
- `custom-client/index.vue:466-468` — *also* saves the correct `app_icon_url`/`app_logo_url`/`privacy_screen_url`
- `custom-client/index.vue:413` — load uses the correct keys

**Fix:** Remove the three dead `custom_*` keys from `saveCurrentAsPreset`.

**Status:** Low (revised from High — no functional data loss; see H-006 for the real preset data-loss bug)

---

### H-008 · Batch Delete — Selection State Not Cleared After Operation (6 Views)

**Page:** Devices, Login History, Connection History, File Transfer History, Shared Sessions, API Tokens  
**Element:** Delete Selected / Batch Delete

**Expected:** After batch delete, selection count resets to 0 and button updates.

**Actual:** `multipleSelection` ref is never reset in `peer/index.vue`, `login/log.vue`, `audit/connList.vue`, `audit/fileList.vue`, `share_record/index.vue`, `user/token.vue`. The "Delete Selected (N)" button shows stale count after the records are gone.

**Fix:** Add `multipleSelection.value = []` in each batch delete success handler.

**Status:** High

---

### H-009 · Server Config Page Is Read-Only (Misleading Name)

**Page:** `/admin/server/config`  
**Element:** Entire page

**Expected:** "Server Config" page should allow editing server settings.

**Actual:** Page displays read-only values from `config.yaml` via `<el-descriptions>`. No form inputs, no save buttons, no edit endpoints exist. Also, any authenticated user (not just admins) can read `GET /config/all` and `GET /config/server` (no `AdminPrivilege`).

> **Revised (2nd pass):** The "truncated public key" exposure was **overstated**. The exposed `key` is the RustDesk **public** key, which is *meant* to be distributed to every client — not a secret. The 20-char truncation is cosmetic frontend only (`config.vue:17`); the API returns the **full** key over the wire. So the genuine issues are (a) the page is read-only despite its name (UX), and (b) the missing `AdminPrivilege` on `/config/*` (same root as L-016). No real secret is leaked. Severity downgraded **High → Medium**.

**Evidence:**
- `admin-ui/src/views/server/config.vue` — only `GET /config/all` on mount; `<el-descriptions>` display only; `config.vue:17` truncates display only
- `api/http/controller/admin/config.go:53-68` — `AllConfig` returns full values, no PUT/POST endpoint
- `api/http/router/admin.go:261-266` — `/config/server`,`/config/app`,`/config/all` behind `BackendUserAuth` only, not `AdminPrivilege`

**Fix:** Rename page to "Server Info" (or add edit capability with `AdminPrivilege`); add `AdminPrivilege` to all three: `/config/all`, `/config/server`, and `/config/app` (see L-016).

**Status:** Medium (revised from High)

---

## Medium Issues

### M-001 · CSV Import — No Header Validation, Position-Based Mapping

**Evidence:** `admin-ui/src/views/peer/index.vue:414-432` — parses by column position ignoring header names. Wrong column order silently corrupts imported data.
**Fix:** Validate header row and map by column name instead of position.

### M-002 · CSV Import — Sends `NaN` for Non-Numeric `group_id`

**Evidence:** `admin-ui/src/views/peer/index.vue:446` — `parseInt(item.group_id)` with no fallback. Empty or non-numeric group_id becomes `NaN` sent to backend.
**Fix:** Add `|| 0` fallback and validate before sending.

### M-003 · Peer Export — Silently Truncated at 10,000 Records

**Evidence:** Peer export uses `page_size=10000` vs `1000000` in other views. No truncation warning for deployments with >10k peers.
**Fix:** Use consistent page_size or show a warning about the cap.

### M-004 · `useBulkRemove` — Reports "Success" Even When Some Records Fail

**Evidence:** `admin-ui/src/composables/useBulkRemove.js:21-28` — `ok` count computed (`results.filter(Boolean).length`) but never displayed. On *partial* success it shows a flat "Operation Success" with no count; on *total* failure (`ok === 0`) it shows **no message at all** and skips `getList()`, so the user gets zero feedback and a stale table (this is exactly what makes H-010 invisible).
**Fix:** Show `"Deleted X of N selected items"`, and surface an error toast when `ok < count`.

### M-005 · Peer `batchRemove` Backend — Swallows UUID Lookup Error

**Evidence:** `api/service/peer.go:140-148` — `GetUuidListByIDs` error is captured, then `err` is reassigned by the delete operation. UUID lookup failure is silently ignored and delete proceeds anyway.
**Fix:** Return early if UUID lookup fails.

### M-006 · Connection Log Export — Raw Unix Timestamp Instead of Formatted Date

**Evidence:** Export re-fetches raw data; `close_time` remains a raw unix timestamp in CSV, unlike the formatted date shown in the table.
**Fix:** Apply `formatTime()` during export, or export timestamps consistently.

### M-007 · Preset Load — 8 Ghost/Stale Field Names That Silently No-Op

**Evidence:** `custom-client/index.vue:413-434` — `hide_connection_management` (form uses `hide_cm`), `allow_offline_input`, `allow_remote_config_modification`, `x11_extra_cmds`, `disable_update` reference form fields that were removed or renamed. Load silently ignores these.
**Fix:** Remove stale field names from the load function.

### M-008 · Preset List — Returns ALL Users' Presets (No User Scope Filter)

**Evidence:** `api/service/custom_preset.go:9-14` — `List` has no `WHERE user_id = ?` filter. Any admin sees all other admins' preset names and configurations.
**Fix:** Add user scope filter to preset List.

### M-009 · GitHub Dispatch Test — 90-Minute HTTP Hold Fails Under Standard Proxies

**Evidence:** `api/http/controller/admin/github_build_config.go:108-166` — `context.WithTimeout(90*time.Minute)` while holding Gin response writer open. nginx default `proxy_read_timeout` is 60 seconds — the connection will be killed long before completion.
**Fix:** Use fire-and-forget goroutine + client polling pattern, as already done in `custom_build.go`.

### M-010 · `resetForm()` Does Not Reset Branding Images or `x_offline`

**Evidence:** `custom-client/index.vue:588-624` — missing `app_icon_url`, `app_logo_url`, `privacy_screen_url`, `x_offline`. After creating a build, old branding values leak into new form.
**Fix:** Add all fields to the reset list.

### M-011 · Password Change Dialog — Form Values Persist Between Opens

**Evidence:** `components/changePwdDialog.vue:44-49` — `showChangePwd()` function is defined but never called. Form values from the previous attempt persist when the dialog reopens, confusing users.
**Fix:** Reset form fields when dialog opens.

### M-012 · Multiple `console.log` Statements in Production Code

**Evidence:** `changePwdDialog.vue:96`, `login/log.vue:114`, `peer/index.vue:420,424,443`, `custom-client/index.vue` log call, `tag/index.vue:32-149` (multiple). Also `user.js:45` logs the **full login response** including JWT token to browser console.
**Fix:** Remove all `console.log` calls; replace with structured logging if needed.

### M-013 · Blocklist/Blacklist — Runtime Changes Not Persisted to Disk

**Evidence:** `server/src/relay_server.rs:51-80` — reads `blacklist.txt` and `blocklist.txt` at startup, but **never writes back**. Runtime additions via the admin UI are lost on restart; runtime removals are restored from the file on restart.
**Fix:** Write back to files on each add/remove command, or use a database.

### M-014 · Usage Component — Fragile Raw Text Parsing

**Evidence:** `admin-ui/src/views/rustdesk/usage.vue:53` — usage table built by splitting TCP response on spaces and mapping by array index. Breaks if server output format changes.
**Fix:** Use structured parsing or add format versioning.

### M-015 · OAuth Form — PKCE Method Required Validation Missing

**Evidence:** `admin-ui/src/views/oauth/index.vue:209` — PKCE method field validator has `required: false`. User can save with PKCE enabled but no method selected, sending invalid data.
**Fix:** Make `required: true` when PKCE is enabled.

---

## Low Issues

### L-001 · Google OAuth — Dead `google` Package Import (Not Broken)

**Evidence:** `api/service/oauth.go:14` — `// "golang.org/x/oauth2/google"` import commented out. Google OAuth WORKS via OIDC fallback path (`oauth.go:202` — Google falls through to same case as OIDC, `FetchOidcProvider` handles discovery). `FormatOauthInfo` in `api/model/oauth.go:76-78` correctly defaults issuer to `https://accounts.google.com` for Google type. Only dead code remains — the old hardcoded-endpoint path.
**Fix:** Remove the commented-out import and dead code block for cleanliness.
**Status:** Low

### L-002 · User Store Logout Patches Wrong Field Names

**Evidence:** `admin-ui/src/store/user.js:24-27` — `$patch({ name: '', role: {} })` patches nonexistent `name` (field is `nickname`) and wrong type for `role` (should be string, not object). Zero practical impact since redirect follows immediately, but technically incorrect.
**Fix:** Use correct field names.

### L-003 · `sync.Once` Prevents Retry of Version File Read

**Evidence:** `api/service/app.go:20-27` — if first read of `resources/version` fails (file not ready during startup race), `version` stays empty permanently.
**Fix:** Add retry logic or lazy initialization.

### L-004 · TCP Response Buffer Hardcoded at 1024 Bytes

**Evidence:** `api/service/serverCmd.go:80` — `buf := make([]byte, 1024)`. May silently truncate responses with many active relay connections.
**Fix:** Use dynamic read loop or larger buffer.

### L-005 · Inconsistent `page_size` Across Exports

**Evidence:** Peer export uses 10,000; user/audit/login-log exports use 1,000,000.
**Fix:** Standardize to 1,000,000 or make configurable.

### L-006 · Batch Add to Address Book — Tag Dropdown Always Empty (revised)

**Evidence (corrected 2nd pass):** The original "always sends empty tags" framing was imprecise — the dialog *does* show a tag `<el-select>` for single-user adds, and tags are intentionally zeroed for *multi*-user batch (`createABForm.vue:112-113`, matching the backend `admin/addressBook.go:117-118`). The real bug is that the tag dropdown is **always empty** because `getTagList()` is never called in `createABForm.vue` (only `getAllUsers()` and `fromPeer()` run on mount). So even a single-user add can't pick a tag.
**Fix:** Call `getTagList()` on mount in `createABForm.vue`.

### L-007 · OAuth Provider Delete — No Check for In-Flight Sessions

**Evidence:** Deleting a provider while users are mid-authentication through it breaks their OAuth flow.
**Fix:** Warn about active use, or add a cooldown/grace period.

### L-008 · Group / Device-Group Delete — No Orphaned-Reference Check

**Evidence:** `api/service/group.go:37-39` `Delete` — deleting a group leaves users with `group_id` pointing at a nonexistent group (user list then renders a blank group cell). `api/service/group.go:71-73` `DeviceGroupDelete` has the **same** flaw for peers' `device_group_id`. Neither nulls out nor reassigns children, and there is no guard/transaction.
**Fix:** In a transaction, null-out or reassign affected users/peers (or reject deletion while children exist).

### L-009 · Dashboard Connect Button — No Feedback If RustDesk Client Missing

**Evidence:** `connectByClient()` creates a hidden `<a href="rustdesk://...">` element and clicks it. If the RustDesk client is not installed, nothing visible happens.
**Fix:** Add a timeout fallback showing a download prompt.

### L-010 · Hardcoded Version List in Custom Client

**Evidence:** `custom-client/index.vue:327` — static `VERSIONS` array (`1.3.3`–`1.4.7`) requires manual update for each new RustDesk release.
**Fix:** Fetch versions from GitHub releases API or configuration.

### L-011 · Hardcoded Artifact Name in Build Downloader

**Evidence:** `api/service/github_build_config.go:303` — artifact name `"rustdesk-min-test-windows"` hardcoded. If the workflow changes its artifact name, download fails.
**Fix:** Make configurable or derive from workflow metadata.

### L-012 · Build History Table — `build_log` Not Shown

**Evidence:** `custom-client/index.vue:282-309` — build status tag displayed but `build_log` (error details) never shown. Users cannot diagnose failed builds without inspecting the API.
**Fix:** Add expandable row or tooltip showing build log.

### L-013 · `AllConfig` Returns `title` But Config Page Doesn't Display It

**Evidence:** `api/http/controller/admin/config.go:66` — `"title": global.Config.Admin.Title` is returned in response but `config.vue` omits this field from the display.
**Fix:** Add the configured title to the config page display.

### L-014 · OAuth Callback Templates — Fragile JS String Interpolation

**Evidence:** `oauth_fail.html:63` — `var msg = '{{.message}}'` — all current messages are server constants so not exploitable, but the pattern is fragile. If any developer passes user-controlled data as `.message`, it becomes an XSS vector.
**Fix:** Use data attributes or proper JS escaping.

### L-015 · Auto-Registered Users Always Get `GroupId=1`

**Evidence:** `api/service/user.go:362-364` — hardcoded to group ID 1 regardless of OAuth provider.
**Fix:** Make default group configurable per provider.

### L-016 · Server Config Accessible to Any Authenticated User

**Evidence:** No `AdminPrivilege` middleware on `/config/all` — any user with a valid token can view server endpoints, ports, and truncated public key.
**Fix:** Add admin privilege check or split sensitive fields into separate endpoint.

### L-017 · `changePwdDialog` Uses `window.location.reload()`

**Evidence:** `components/changePwdDialog.vue:117` — full page reload instead of `router.push('/login')` after logout.
**Fix:** Use router navigation after logout for smoother UX.

### L-018 · OAuth Redirect URL Displayed But Not Configurable

**Evidence:** `oauth/index.vue:90-96` — redirect URL shown as a `<div>` with copy button, not an input. Backend model field `redirect_url` is commented out (`api/model/oauth.go:44`).
**Fix:** Either make it configurable or clearly label as "Your callback URL (copy this to the provider)".
**Status:** Low

### L-019 · Custom Client "Create" Button Misleading

**Evidence:** "Create" button only saves build configuration to DB. Does not trigger a build. Actual builds happen via separate GitHub Build Integration page or separate build agents.
**Fix:** Rename button to "Save Configuration" with a separate "Build Now" action.

---

## Security

### S-001 · Server Command Execution Unaudited and Unrestricted

**Page:** Server → Commands → Advanced → Send  
**Element:** Send/SendToId/SendToRelay buttons

**Expected:** Server commands are audited and restricted to admins.

**Actual:** `SendCmd` forwards arbitrary text commands to hbbs/hbbr TCP control ports. No audit log entry is created (who ran what command, when, result). Any authenticated user (not just admin) can send commands — the endpoint does not use `AdminPrivilege` middleware. Commands include `ip-blocker`, `blacklist-add/remove`, `limit-speed`, `total-bandwidth`, and any custom user-defined commands.

> **Extended (2nd pass):** The *entire* `/rustdesk/*` group is ungated — `RustdeskCmdBind` (`api/http/router/admin.go:69-76`) registers `sendCmd`, `cmdList`, `cmdCreate`, and `cmdDelete` directly on `adg` (auth-only, no `AdminPrivilege`). So any authenticated non-admin user can not only send server commands but also **create and delete persistent server-command records**. This is a genuine privilege-escalation surface, not just missing audit. (Note: `cmdUpdate` is unreachable — see H-011.)

**Evidence:**
- `api/http/controller/admin/rustdesk.go:104-136` — no audit logging, no admin privilege check
- `api/http/router/admin.go:69-76` — whole `/rustdesk` group lacks `AdminPrivilege()`

**Fix:** Restrict the whole `/rustdesk/*` group to `AdminPrivilege`, and add audit logging (admin userId, command, target, timestamp, result).

**Status:** Security Risk (High)

---

## Second-Pass Additions (New Findings)

*Added after a verification re-audit using parallel sub-agents across Address Book, My Profile, Users/Groups, and a full auth/OAuth/LDAP/client-API sweep. Every item below was traced to source.*

### H-010 · Address Book Bulk Delete Silently Does Nothing (entries, admin + my)

**Page:** Address Book → Contacts → Delete Selected; My Address Book → Delete Selected  
**Element:** Delete Selected (N) button

**Expected:** Selected address-book entries are deleted.

**Actual:** Bulk delete is wired to the generic `useBulkRemove`, which calls `removeApi({ id: r.id })`. But address-book entries are keyed by `row_id`, and the backend `Delete` validates `f.RowId` with `required,gt=0` (`id` is the *device-id string*, not the PK). So `RowId` stays `0`, validation fails for **every** row, and nothing is deleted. Because `useBulkRemove` shows no message when `ok === 0` (see M-004), the user gets **zero feedback** and the rows remain. Single-row delete works (it correctly sends `{ row_id }`).

**Evidence:**
- `admin-ui/src/composables/useBulkRemove.js:9-21` — `removeApi({ id })`, payload key is `id`
- `admin-ui/src/views/address_book/index.vue:212-216` — bulk delete uses `useBulkRemove({ removeApi: apiRemove })`
- `admin-ui/src/views/address_book/index.js:76` — single delete (works) sends `{ row_id: row.row_id }`
- `api/http/controller/admin/addressBook.go:259-260` — `id := f.RowId; ValidVar(id, "required,gt=0")`
- `api/http/request/admin/addressBook.go:9-10` — `RowId uint json:"row_id"` (separate from `Id string json:"id"`)

**Fix:** Pass a custom id-extractor to `useBulkRemove` for AB entries (send `{ row_id }`), or have these views call `apiRemove({ row_id })` directly. (Bulk delete of *collections* is unaffected — collections are keyed by `id`.)

**Status:** High

---

### H-011 · Editing a Server Command Returns 404 (`cmdUpdate` handler exists, route missing)

**Page:** Server → Server Commands → edit an existing command  
**Element:** Save (on edit)

**Expected:** Editing a saved server command persists the change.

**Actual:** The frontend calls `POST /rustdesk/cmdUpdate` (`api/rustdesk.js` `update`), and the controller method `Rustdesk.CmdUpdate` exists — but `RustdeskCmdBind` never registers the route (it registers only `sendCmd`, `cmdList`, `cmdDelete`, `cmdCreate`). Editing a command therefore 404s.

**Evidence:**
- `api/http/router/admin.go:69-76` — `RustdeskCmdBind` has no `cmdUpdate` registration
- `api/http/controller/admin/rustdesk.go:80` — `func (r *Rustdesk) CmdUpdate(...)` exists but is unrouted
- `admin-ui/src/api/rustdesk.js:18-21` — `update()` → `url: '/rustdesk/cmdUpdate'`

**Fix:** Register `rg.POST("/cmdUpdate", cont.CmdUpdate)` (under `AdminPrivilege`, per S-001).

**Status:** High

---

### S-002 · LDAP Authentication Hardening (injection, TLS, empty-bind)

**Page:** Login (admin `POST /login` and client `POST /api/login`) when `ldap.enable = true`  
**Element:** Username/password login

**Expected:** LDAP login escapes search input, verifies TLS, and rejects empty passwords.

**Actual:** Three issues in `api/service/ldap.go`, all on the live login path (`user.go:49-50` calls `LdapService.Authenticate` for both admin and client login):
1. **Filter injection:** `filterField` builds `fmt.Sprintf("(%s=%s)", field, value)` with the **raw** username/email — no `ldap.EscapeFilter`. Inconsistent with the group-membership filters in the same file, which *do* escape (`:175-177`, `:474-476`, `:545`). A crafted username (e.g. `*` or `admin)(uid=*`) injects into the admin-bound search.
2. **TLS verification off by default:** `InsecureSkipVerify: !cfg.TlsVerify`, and `TlsVerify` defaults to `false` → MITM possible on LDAPS unless an admin explicitly opts in.
3. **No empty-password guard before bind:** many directories treat (valid DN + empty password) as an unauthenticated bind that *succeeds*, yielding auth bypass for any known username.

**Evidence:**
- `api/service/ldap.go:399-401` — `filterField` with no `EscapeFilter`; used by `usernameSearchResult`/`emailSearchResult` (`:321`, `:329`)
- `api/service/ldap.go:84` — `InsecureSkipVerify: !cfg.TlsVerify` (default-insecure)
- `api/service/ldap.go:106-127` — bind with no empty-password check
- LDAP has **no admin UI** (config-file only): `grep -ri ldap admin-ui/src` → 0 matches

**Fix:** Wrap search values in `ldap.EscapeFilter`; default TLS verification on (invert the flag); reject empty passwords before binding.

**Status:** Security Risk (High, conditional on LDAP being enabled)

---

### M-016 · OAuth `client_secret` Returned in List/Detail Responses

**Evidence:** `api/model/oauth.go:43` — `ClientSecret string json:"client_secret"` has no `json:"-"`. `OauthService.List`/`Detail` return the full struct (`service/oauth.go:414-425`), and the edit form reads `row.client_secret`. Any admin-readable list/detail call ships every provider's client secret to the browser.
**Fix:** Add `json:"-"` to `ClientSecret` (or use a response DTO); never return the secret. Note this is a *real* secret, unlike the RustDesk public key in H-009/L-016.
**Status:** Medium

### M-017 · `/user/groupUsers` Discloses Full User + Group Directory to Any Authenticated User

**Evidence:** `api/http/router/admin.go:101-105` — `/user/current`, `/user/groupUsers` (and a few others) are registered on the bare `UserBind` group *before* `AdminPrivilege`. `user.go:298-305` returns **all** users + **all** groups, unscoped. Any non-admin authenticated user can enumerate the whole directory.
**Fix:** Move `/user/groupUsers` behind `AdminPrivilege`, or scope it.
**Status:** Medium

### M-018 · Admin Tag Create/Edit — Collection Dropdown Never Populates

**Evidence:** `admin-ui/src/views/tag/index.js:182-190` `changeUserForUpdate` sets `collectionListQuery.user_id = val` (the **filter-panel** query) instead of `collectionListQueryForUpdate.user_id`, then calls `getCollectionListForUpdate()` (which reads the *Update* query). The dialog's collection dropdown always fetches with the wrong/empty user scope, so it never shows the selected user's collections.
**Fix:** Set `collectionListQueryForUpdate.user_id = val`.
**Status:** Medium

### M-019 · Admin Can Delete or Disable Their Own Account (Self-Lockout)

**Evidence:** `api/service/user.go` `Delete`/`Update` guard only the **last** admin (`getAdminUserCount() <= 1`); neither checks `f.Id == CurUser(c).Id`. With ≥2 admins, the logged-in admin can delete or disable themselves (`user/index.vue` row switch / Delete Selected), producing a ghost-logged-in state until the next API call 403s.
**Fix:** Reject self-delete / self-disable in the controller; disable the control for the current user's own row.
**Status:** Medium

### M-020 · Admin User Add/Edit — Backend Errors Silently Swallowed

**Evidence:** `admin-ui/src/views/user/composables/edit.js:58-61` — `const res = await create(form.value).catch(_ => false); return res.code === 0`. On a backend error (e.g. `UsernameExists`), `res` is `false`, `false.code` is `undefined`, so the function returns `false` with **no error message** and the dialog just sits there. Same pattern in `submitUpdate`.
**Fix:** Detect `!res` and surface the API error message before reading `res.code`.
**Status:** Medium

### M-021 · My Profile — Account Info Not Editable

**Evidence:** `admin-ui/src/views/my/info.vue` renders username/email as read-only `<div>`s with no inputs, no Save, no update call. A self-service profile-update endpoint isn't exposed (`/user/update` is admin-only). Users cannot change their own nickname/email/avatar despite the "account details" framing.
**Fix:** Add editable nickname/email fields + a scoped `updateProfile` endpoint (no role/status escalation).
**Status:** Medium

### M-022 · Unauthenticated Writes on Client-Facing API (review)

**Evidence:** In `api/http/router/api.go`, `frg.Use(RustAuth())` is at line 76, so routes registered *before* it are unauthenticated: `POST /api/sysinfo` (`peer.go:26` creates/updates `Peer` rows keyed by caller-supplied `id`), `POST /api/heartbeat`, `POST /api/audit/conn`, `POST /api/audit/file`. An unauthenticated caller can create/alter peer rows and inject audit entries. `WebClientRoutes` `/api/shared-peer` also does an unchecked `(*j)["share_token"].(string)` assertion (`webClient.go:57`) → panic/500 on missing token.
**Note:** Some of this may be intentional for the RustDesk client protocol (clients report before login) — **verify against the intended protocol** before "fixing." Flagged because it is at minimum an abuse/DoS surface.
**Status:** Medium (needs design confirmation)

---

### L-020 · My Collections — "Share Rules" on the Synthetic `id=0` Row Fails

**Evidence:** `my/address_book/collection.vue:118-127` prepends a synthetic `{ id: 0, name: 'MyAddressBook' }` row that is selectable. Opening "Share Rules" on it submits `collection_id = 0`, which fails backend validation (`CollectionId uint validate:"required"`) and `CheckCollectionOwner(uid, 0)`. The personal address book can't have sharing rules.
**Fix:** Hide/disable "Share Rules" for the `id=0` row.
**Status:** Low

### L-021 · Batch Edit Tags Cannot *Clear* Tags

**Evidence:** `admin-ui/src/views/address_book/index.js:265-270` — the batch-update-tags submit rejects an empty selection with a warning, so a user can never clear all tags from selected entries. The backend `BatchUpdateTags` supports an empty array fine.
**Fix:** Drop the `tags.length === 0` guard (keep the `row_ids.length > 0` guard).
**Status:** Low

### L-022 · Admin Login History Shows Soft-Deleted Records

**Evidence:** `api/http/controller/admin/loginLog.go:59-65` omits the `is_deleted = 0` filter that the user path applies (`my/loginLog.go:37`). Records a user "deleted" from *My Login History* (soft-delete) still appear to admins indefinitely.
**Fix:** Add `tx.Where("is_deleted = ?", model.IsDeletedNo)` to the admin list (and reconcile admin hard-delete vs user soft-delete semantics).
**Status:** Low

### L-023 · `LoginLog.UserTokenId` Populated With Wrong Value

**Evidence:** `api/service/user.go:107` — `llog.UserTokenId = ut.UserId` copies the *user* id instead of the token's own PK (`ut.Id`). The `user_token_id` column is meaningless (duplicates `user_id`). Currently unused for queries, so no functional break.
**Fix:** `llog.UserTokenId = ut.Id`.
**Status:** Low

### L-024 · OAuth Backend Validation Gaps (PKCE method, OIDC issuer)

**Evidence:** Backend `OauthForm.PkceMethod` (`request/admin/oauth.go:27`) has **no** validation tag, so an arbitrary method (e.g. `"foo"`) persists and is then silently dropped in `BeginAuth`'s switch — disabling PKCE while `pkce_enable=true` (worse than the frontend-only M-015). Also `Issuer` is `omitempty,url`, so an empty issuer is accepted for `oauth_type=oidc` and only fails later at runtime.
**Fix:** Validate `pkce_method ∈ {S256, plain}` server-side; require `issuer` when type is `oidc`.
**Status:** Low

### L-025 · `my/address_book/collection.vue` Missing `onActivated` Refresh

**Evidence:** `onActivated` is imported but never called, unlike the admin collections/entries views (`address_book/collection.vue:150`, `address_book/index.vue:221`). With keep-alive, navigating away and back shows stale data.
**Fix:** Add `onActivated(getList)`.
**Status:** Low

### L-026 · Custom Client Build Delete — Orphaned Artifact Files on Disk (+ no bulk delete)

**Note:** The Delete button in Build History itself **works** end-to-end (button → `deleteBuild(row)` → `POST /custom_build/delete` → `Info(id)` → `DB.Delete` → `loadBuilds()`); this is a cleanup/UX gap, not a broken button.

**Evidence:** `api/service/custom_build.go:23-24` — `Delete` does only `DB.Delete(u)`. Build artifacts are written to disk at `/rdgen-data/output/{build.Id}/` (served by `DownloadByKey`, `api/http/controller/admin/custom_build.go`). Deleting a build never removes that directory, so artifact files accumulate as orphans after every delete. Also, Build History offers only per-row delete (`custom-client/index.vue:301`) — there is no multi-select/bulk delete.

**Fix:** In `CustomBuildService.Delete`, best-effort `os.RemoveAll(filepath.Join("/rdgen-data","output", id))` alongside the DB delete; optionally add a bulk-delete action to the history table.
**Status:** Low

---

## Feature Gaps (Not Bugs)

| Feature                           | Status                                                             |
| --------------------------------- | ------------------------------------------------------------------ |
| Address Book import/export        | Not implemented anywhere                                           |
| Audit log date-range filtering    | Only text filters (`peer_id`, `from_peer`), no date pickers        |
| Share record expiry filter        | Only filter by `user_id`, no "expired"/"active"/"forever" grouping |
| Peer column settings persistence  | Browser-local only (`localStorage`), not per-user server-side      |
| Address Book dialog validation    | No `:rules` binding, only server-side validation with round-trip   |

---

## UI Functions Promised But Backend Missing

| UI Feature                         | Page                      | Status                           |
| ---------------------------------- | ------------------------- | -------------------------------- |
| Settings persistence across restart| Server Commands (all)     | Critical — volatile, no write-back |
| My Devices delete                  | `/my/devices`             | Critical — commented out         |
| Address Book import/export         | `/admin/address-book/books`| Not implemented                 |
| OAuth redirect URL override        | `/admin/security/oauth`   | Model field commented out        |
| Server config edit                 | `/admin/server/config`    | Read-only                        |
| Edit server command (`cmdUpdate`)  | Server → Server Commands  | High — handler exists, route missing → 404 (see H-011) |
| Rule `batchCreate`                 | Address Book → Rules      | Frontend `api` wrapper exists, **no backend route** (would 404) |
| `POST /user/myPeer`                | —                         | Frontend wrapper exists, backend route commented out (`admin.go:104`) → would 404 |

---

## Backend Endpoints Existing But Unused in UI

| Endpoint                                         | Notes                        |
| ------------------------------------------------ | ---------------------------- |
| `GET /peer/detail/:id`                             | No view imports              |
| `GET /address_book/detail/:id`                     | No view imports              |
| `GET /address_book_collection/detail/:id`          | No view imports              |
| `GET /address_book_collection_rule/detail/:id`     | No view imports              |
| `GET /tag/detail/:id`                              | No view imports              |
| `GET /group/detail/:id`                            | No view imports              |
| `GET /device_group/detail/:id`                     | No view imports              |
| `GET /oauth/detail/:id`                            | No view imports              |
| `GET /custom_build/detail/:id`                     | No view imports              |
| `GET /custom_build/public/detailByKey/:key`        | No view imports              |
| `GET /custom_preset/detail/:id`                    | No view imports              |
| `POST /custom_preset/update`                       | Confirmed unused — `index.vue` imports only list/create/remove/detail |
| ~~`POST /address_book/batchCreate`~~               | **Correction:** this **IS** used — `peer/createABForm.vue:65,115`. Removed from this list. |

---

## Dangerous or Unclear UI Sections

1. **Server Commands → Advanced** — sends arbitrary text commands to hbbs/hbbr with no validation, audit, or admin restriction
2. **RELAY_SERVERS input** — accepts any text; no format validation (expects `host:port,host:port`); changes affect all connected clients
3. **ALWAYS_USE_RELAY / MUST_LOGIN toggles** — immediately affect all connected clients with no impact warning
4. **Blocklist vs Blacklist** — two separate concepts on the relay server with no UI distinction: blocklist completely blocks connections, blacklist rate-limits them. Unclear which is which.
5. **GitHub Build Config PAT** — stored in DB in plaintext; UI shows placeholder "(already saved)" but token is sensitive
6. **Custom Client Permanent Password** — entered in plaintext, stored in `custom_json` field without encryption

---

## Recommendations (Priority Order)

### Immediate fixes (Critical + High, ordered by severity):

1. **C-002** — Fix `aur` command destroying relay servers (one-line Rust change)
2. **C-003** — Fix file upload path traversal (sanitize filename, add magic-byte check)
3. **C-001** — Add persistence for server settings (minimum: add UI warnings about volatility)
4. **C-004** — Implement My Devices delete (add backend endpoints + unblock frontend)
5. **S-001 + S-002** — Gate the whole `/rustdesk/*` group behind `AdminPrivilege` + audit log; harden LDAP (escape filters, TLS, empty-bind)
6. **M-016** — Stop returning OAuth `client_secret` in list/detail responses (security: secret in every admin list response)
7. **H-002** — Fix last-admin race condition in user delete (move count check inside transaction)
8. **H-004** — Fix CSV export `.toString()` crash on null cells; parse `info` JSON field
9. **H-005** — Add explicit cascade-delete warning for address book collection delete
10. **H-010** — Fix Address Book bulk delete (send `row_id`, not `id`) — currently a silent no-op
11. **H-011** — Register the missing `cmdUpdate` route (editing server commands 404s)
12. **H-006** — Fix preset permission-field synchronization (real data loss; H-007 is now just dead-code cleanup)
13. **H-008** — Fix batch selection clearing (stale count across 6 views)
14. **H-003** — Fix CSV import to provide actual feedback on results

### Next batch (Medium):

1. **M-004** — Fix `useBulkRemove` partial failure messaging
2. **M-008** — Add user scope filter to preset list
3. **M-009** — Fix GitHub dispatch 90-minute HTTP hold
4. **M-013** — Persist blocklist/blacklist changes to disk
5. **M-012** — Remove `console.log` statements from production code

### Tests to add first:

| Priority | Test                                               |
| -------- | -------------------------------------------------- |
| 1        | E2E: Login → peer CRUD → verify DB persistence     |
| 2        | E2E: Server commands Save → restart container → verify reverted |
| 3        | API: `POST /user_token/batchDelete` with non-admin user → verify authorization |
| 4        | API: File upload with `../../` filename → verify sanitization |
| 5        | API: Concurrent admin user deletes → verify last-admin protection |
| 6        | Integration: Preset save → load → verify all fields round-tripped |
| 7        | Integration: CSV import with mixed success/failure → verify per-row feedback |
| 8        | Security: File upload with spoofed Content-Type → verify magic-byte check |

---

*Report generated from cross-referenced code audit covering all 40+ views, 80+ endpoints, and 150+ interactive elements. All findings verified against source code at controller, service, model, and persistence layers.*
