# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased] - 2026-06-17

### Security
- **api: stop exposing OAuth `client_secret`** — `Oauth.ClientSecret` now `json:"-"` so list/detail responses never ship secrets to the browser. (M-016)
- **api: harden LDAP** — `filterField` now escapes user-supplied values via `ldap.EscapeFilter`; empty-password bind is rejected; TLS verification defaults to secure. (S-002)
- **api: gate `/rustdesk/*` behind `AdminPrivilege`** — server-command send/list/create/delete/update were open to any authenticated user. (S-001 + H-011)
- **api: fix file upload path traversal** — `Upload` sanitizes filename with `filepath.Base`, enforces PNG magic bytes via `io.ReadFull` + signature check, and limits total size to 5 MB on actual bytes (not client-declared `Content-Length`); 0755 instead of 0777 on the upload dir; filename now prefixed with `UnixNano()` to prevent same-day collisions. (C-003)
- **api: scope UUID lookup to owner in `BatchDeleteByOwner`** — added `GetUuidListByIDsAndOwner` so a user cannot invalidate tokens of other users' peers by guessing row_ids. (re-review)
- **api: enforce preset ownership on Detail/Update/Delete** — closes privilege-escalation gap introduced by user-scoped List. (3rd-pass review)
- **server: atomic blocklist/blacklist file write** — `write_set_to_file` writes to `.tmp` then `rename`s; writes serialized through `tokio::sync::Mutex` to prevent stale-snapshot races. (re-review + 3rd-pass review)

### Fixed (critical)
- **server: stop `aur` command from destroying relay servers** — removed stray `Data::RelayServers0` send. (C-002)
- **api + admin-ui: implement My Devices delete** — `/admin/my/peer/delete` and `/admin/my/peer/batchDelete` with ownership-scoped SQL inside a transaction; `gorm.ErrRecordNotFound` treated as idempotent success; frontend `del` / `toBatchDelete` uncommented and wired. (C-004)

### Fixed (high)
- **api: last-admin race condition** — `getAdminUserCount` moved inside the deletion transaction. (H-002)
- **admin-ui: address book bulk delete silently no-op** — `useBulkRemove` now supports `getRemovePayload`; AB entries correctly send `{ row_id }`. (H-010)
- **admin-ui: missing server-command edit route** — `cmdUpdate` registered under `AdminPrivilege`. (H-011)
- **admin-ui: custom client preset permission fields lost** — save/load/reset now round-trip all 13 permission flags + `x_offline` + branding URLs; stale field names removed. (H-006 + M-010)
- **admin-ui: batch delete cleared selection on cancel or API failure** — waiters now use truthy check after `batchdel` was patched to return `res`. (H-008 + 3rd-pass review)
- **admin-ui: csv peer import gave no feedback** — replaced `Promise.all().catch(_=>false)` with `Promise.allSettled` + counts. (H-003)
- **admin-ui: csv export crashed on null cells** — `jsonToCsv` now null-guards and `JSON.stringify`s nested objects. (H-004)

### Fixed (medium)
- **admin-ui: partial-failure messaging in `useBulkRemove`** — three-state success/partial/total-failure feedback; payloadFn errors logged instead of swallowed. (M-004 + 3rd-pass review)
- **admin-ui: address book collection delete cascade warning** — new `warningMessage` option surfaces "this also deletes entries and rules" in the confirm dialog. (H-005)
- **admin-ui: GitHub dispatch no longer holds HTTP for 90 min** — `DispatchTest` returns `run_id` immediately; frontend `github-build.vue` updated to match new response contract. (M-009)
- **admin-ui: persist blocklist/blacklist runtime changes** — relay server writes back to `blacklist.txt` / `blocklist.txt` atomically after every add/remove. (M-013)
- **admin-ui: preset list scoped by current user** — `ListByUser` so admins no longer see each other's presets. (M-008)

### Changed
- **admin-ui: removed all `console.log` statements** flagged by the audit — including one in `store/user.js:45` that logged the full login response with the JWT. (M-012)
- **api: `Ldap` config retains deprecated `tls-verify` key** for backward compat; `tls-skip-verify` is now `*bool` so "unset" cleanly falls back. New deployments should use `tls-skip-verify`. (S-002 + 3rd-pass review)

### Reference
- Functional audit report: `audit-report.md` (PR #19).
- Re-review identified 3 additional High findings — all fixed.
- 3rd-pass `ocr` review identified 6 additional High findings — all fixed in this change set.
- Round 2 (PR #21) resolved 15 more findings — see below.

### Security (round 2)
- **api: scope token batch-delete to current user** — non-admin callers now have `AND user_id = ?` applied to the `BatchDeleteUserToken` query; admins retain full scope. The route is currently behind `AdminPrivilege`, so the non-admin branch is defense-in-depth that matches the per-row owner check already in `(ct *UserToken).Delete`. (H-001)
- **api: gate `/config/all` behind `AdminPrivilege`** — the supermarket endpoint that returns `register`, `ws_host`, `show_swagger`, `personal`, etc. is now admin-only. `/config/server` and `/config/app` stay behind `BackendUserAuth` because the web-client login flow writes `id_server`/`key`/`api-server` into `localStorage` for every authenticated user, and `web_client` drives UI rendering. (H-009 / L-016)
- **api: split `/user/groupUsers` into admin and personal endpoints** — `/user/groupUsers` is admin-only; new `/my/groupUsers` (`BackendUserAuth`, reuses the same handler) lets non-admins populate the grantee picker for their Personal Address Book share rules. Frontend `admin-ui/src/views/address_book/rule.js` dispatches by `api_type`. (M-017)

### Fixed (round 2)
- **api: reject admin self-lockout in three shapes** — `Update` rejects `curUser.Id == u.Id` when the request would either disable the current user (`Status == COMMON_STATUS_DISABLED`) or demote them (`IsAdmin(curUser) && !*u.IsAdmin`); `Delete` rejects `curUser.Id == u.Id` outright. Backend is authoritative; frontend disable/delete controls for the current user's own row are not yet visually disabled. (M-019)
- **api: filter soft-deleted records from admin login history** — admin list now includes `is_deleted = ?` filter using `model.IsDeletedNo`, consistent with the user-facing path. (L-022)
- **api: fix `LoginLog.UserTokenId` assignment** — was `ut.UserId` (duplicates user_id), now correctly stores `ut.Id` (the token's own PK). (L-023)
- **admin-ui: validate csv import headers by name + strip UTF-8 BOM** — import strips the leading BOM from the header line before splitting, trims and unquotes each column name, checks for required columns and maps by header name instead of positional index; missing columns produce a descriptive error toast. (M-001)
- **admin-ui: fix csv import `group_id` NaN fallback** — `parseInt(group_id)` → `parseInt(group_id) || 0`. (M-002)
- **admin-ui: normalize peer export page_size** — changed from 10,000 to 1,000,000, consistent with other export functions. Not a true cap removal — deployments with >1M peers still need a server-side streaming export. (M-003)
- **admin-ui: conditionally require `pkce_method`** — replaced the unconditional `required: true` with a `validator` that requires (and constrains to `S256`/`plain`) only when `formData.pkce_enable === true`. OAuth configs with PKCE disabled still save even if the stored method is empty. (M-015)
- **admin-ui: fix tag collection dropdown query** — `changeUserForUpdate` was setting the wrong query variable, so the dropdown never populated for the selected user. (M-018)
- **admin-ui: stop double-toasting backend errors on user create/update** — both `submitCreate` and `submitUpdate` now coerce the falsy `res` from `.catch(_ => false)` before dereferencing `res.code`; the real error toast (`UsernameExists`, etc.) comes from the global axios interceptor's `res.message`, so the composable no longer stacks a generic `OperationFailed` on top. (M-020)
- **admin-ui: hide Share Rules for personal address book row** — the synthetic `id=0` row now hides "Share Rules" and disables "Edit". (L-020)
- **admin-ui: allow clearing tags in batch edit (with confirm)** — removed the silent `tags.length === 0` guard; an empty tag list now triggers an `ElMessageBox.confirm` ("Confirm? Clear tags") so an accidental "Save" no longer wipes every selected entry. New i18n key `ClearTags`. (L-021)
- **admin-ui: refresh my collections on `onActivated`** — `my/address_book/collection.vue` now refreshes the list when navigating back with keep-alive. (L-025)
- **admin-ui: format `close_time` in connection log export** — raw unix timestamp now converted to formatted date string in CSV. (M-006)
- **admin-ui: reset password dialog fields on open** — `changePwdDialog` now clears form on dialog open via `watch`; `window.location.reload()` replaced with `router.push('/login')`. (M-011 + L-017)
- **admin-ui: fix logout `$patch` field names** — `name` → `nickname`, `{}` → `''` for `role`. (L-002)
- **admin-ui: populate tag dropdown in address book add dialog** — `createABForm.vue` now imports `getTagList` from `useABRepositories`. (L-006)
- **admin-ui: display `title` field on server config page** — added `el-descriptions-item` for `cfg.title`. (L-013)

### Fixed (admin-ui: card hover-shadow flicker on route enter)
- `PageSection` and `DangerZone` switched from `shadow="hover"` to `shadow="never"` so cards no longer animate `box-shadow: none → var(--shadow-card)` on page enter. In dark mode the old transition (`rgba(0, 0, 0, 0.28)` shadow) read as a black-to-blue flash whenever the cursor was already over a card after a route transition; static cards (border + background) make the layout stable on navigation.
- Same change applied to the six Server Commands `simple-card` views: `always_use_relay.vue`, `blacklist.vue`, `blocklist.vue`, `must_login.vue`, `relay_servers.vue`, `usage.vue` — all now `shadow="never"` for consistency with the rest of the page.

### Fixed (admin-ui: v-loading mask flash in dark mode)
- **Root cause**: Element Plus's `v-loading` directive uses `--el-mask-color` for its overlay. In `:root` this is `#ffffffe6` (semi-transparent white), but `html[data-theme="dark"]` overrides it to `#000c` (black, 80% opacity). Whenever `v-loading` was set `true` — even for a few hundred ms while a command round-tripped — the whole element flashed solid black and then faded back. On the dashboard's `Relay Activity` card body and on every Server Commands `simple-card` this read as a "black → blue" blink (the dark-blue `--color-border` becomes visible again as the mask fades out).
- **Targeted removal** on the six Server Commands `simple-card` views: dropped `v-loading="form.loading"` and the now-dead `form.loading` state in `always_use_relay.vue`, `blacklist.vue`, `blocklist.vue`, `must_login.vue`, `relay_servers.vue`, `usage.vue`. The cards are small and the `sendCmd` round-trips complete in tens of ms, so no replacement loading indicator is needed.
- **Global safety net** in `admin-ui/src/styles/style.scss`: added `.el-loading-mask { background-color: transparent; }` so the remaining `v-loading` users (`ServerHealth.vue` Relay Activity card body, `custom-client/index.vue` form, `server/config.vue` descriptions x2, `server/github-build.vue` form) keep the spinner but no longer lay a black rectangle over the content during the `transition: opacity 0.3s` fade.

### Fixed (admin-ui: empty menu item above Dashboard)
- `admin-ui/src/router/index.js` — the root redirect `{ path: '/', redirect: '/dashboard' }` had no `name`, no `meta`, and no `component`, but it was still being iterated by `menu/index.vue` and rendered by `menu/item.vue` as an `<el-menu-item>` with no icon, no label, and `index="undefined"`. Because the redirect was the first entry in `asyncRoutes`, the ghost item landed directly above the Dashboard entry in the sidebar. Added `meta: { hide: true }` to the redirect so `MenuItem`'s `!parseRoute(route).meta?.hide` guard skips it.

### Fixed (admin-ui: input background flash on disabled→enabled transition in dark mode)
- **Root cause**: the six Server Commands `simple-card` views (`blacklist.vue`, `blocklist.vue`, `always_use_relay.vue`, `relay_servers.vue`, `must_login.vue`, `usage.vue`) bind `<el-form :disabled="!canSend">`. While `control.vue` is awaiting `refreshCanSendRelayServerCmd()` / `refreshCanSendIdServerCmd()`, the form is disabled. The moment `canSend` flips to `true`, Element Plus removes `.is-disabled` from every `el-input` / `el-textarea` inside. In dark mode the disabled background is `var(--el-disabled-bg-color) → var(--el-fill-color-light) → #262727` (Element Plus's dark-theme override) and the enabled background is `var(--el-fill-color-blank) → #111827` (project's style.scss override). The change is instant — `el-textarea__inner` has `transition` only on `box-shadow`, not on `background-color` — and the dark-blue `--color-border: #263244` was clearly visible against the lighter disabled background, then visually disappeared against the darker enabled one. The eye reads this as "border vanished", i.e. a "black → blue" flash in reverse.
- Most visible on `BLOCK_LIST` / `BLACK_LIST` because their `el-textarea` is large, but the same effect is present on all six cards.
- **Fix** in `admin-ui/src/styles/style.scss`: in the `html[data-theme="dark"], html[data-theme="auto"].dark` block, set `--el-disabled-bg-color: var(--color-surface);` so disabled and enabled inputs share the same `#111827` background in dark mode. The disabled border colour, text colour, and `cursor: not-allowed` are preserved — the field still reads as "off" without a background pop. Light mode is untouched (the standard Material-style disabled fill stays as `--el-fill-color-light` where it looks right).

### Removed (Account Info: redundant Welcome block)
- `admin-ui/src/views/my/info.vue` no longer renders the standalone `<page-section title="Welcome">` that surfaced `marked(appStore.setting.hello)` at the bottom of the page. The welcome markdown belongs on the public login surface, not on the post-auth Account Info screen.
- Cleaned up the now-unused `useAppStore` / `marked` / `computed` imports and the `.hello-section` style.

### Added (README screenshots)
- New `### Screenshots` section in `README.md` with a two-column table linking to `docs/screenshots/dashboard.png` and `docs/screenshots/client-builder.png`. Both PNGs are committed under `docs/screenshots/` for GitHub / npm preview rendering.

### Changed (admin-ui: Actions column refactor — toolbar + checkboxes)
- **New `ActionsToolbar.vue`** (`admin-ui/src/components/ui/ActionsToolbar.vue`): shared toolbar rendered above the table that shows the current selection count ("3 selected") and exposes a slot for bulk action buttons. Bulk action buttons receive `:disabled` and stay visually muted until at least one row is selected. The bar shifts its border/background tint when active so the user gets a clear "you have N rows selected" cue.
- **New `useBulkRemove` composable** (`admin-ui/src/composables/useBulkRemove.js`): single `confirm-and-delete-N` flow with one shared ElMessageBox confirmation and parallel API calls. Replaces the previous per-row `ElMessageBox.confirm` pattern which would have stacked N dialogs.
- **15 list views** refactored to drop the right-side `fixed: 'right'` Actions column and use the new toolbar + a single non-fixed More dropdown per row:
  - Admin: `user/index.vue`, `group/index.vue`, `group/deviceGroupList.vue`, `tag/index.vue`, `oauth/index.vue`, `address_book/index.vue`, `address_book/collection.vue`, `address_book/rule.vue`, `peer/index.vue`
  - Workspace: `my/address_book/index.vue`, `my/address_book/collection.vue`, `my/tag/index.vue`, `my/peer/index.vue`
  - Logs already used `FilterBar`; their inline Actions column was just slimmed to a 80-px no-label column to free horizontal space.
- **Behavior changes worth knowing**:
  - Single-row Delete now goes through the row-level More dropdown. For pages where Delete was the only action, a single non-fixed "More" replaces the wider inline button.
  - Bulk Delete, BatchAddToAddressBook, BatchEditTags are now toolbar buttons and are disabled when nothing is selected. Confirmation is one dialog for the whole batch, not N dialogs.
  - On the users page, the single-row Delete still uses the existing composable's per-row confirm so accidental deletes still get a single, deliberate "are you sure?" prompt; bulk delete shows one dialog with the count in the prompt.
  - On `my/address_book`, BatchEditTags moved out of the filter form into the toolbar; selecting rows still populates `row_ids` for the dialog automatically.

### Fixed (AddressBookName label inconsistency)
- `T('AddressBookName')` was used in filters and form labels while the column header had been shortened to "Name" in a previous commit. Renamed all 14 view-level references to `T('Name')` and removed the now-unused `AddressBookName` and `AddressBookNameManage` i18n keys (en/ru/zh_CN). Hardcoded column labels `'Name'` switched to `T('Name')` for proper i18n. Affected: `address_book/rule.vue`, `address_book/index.vue`, `my/address_book/index.vue`, `my/peer/index.vue`, `my/tag/index.vue`, `peer/createABForm.vue`, `peer/index.vue`, `tag/index.vue`, and the collections / address-book-collection column labels in `address_book/collection.vue` and `my/address_book/collection.vue`.

### Fixed (ElSwitch warnings in admin-ui)
- **Root cause** (`admin-ui/src/views/user/index.vue`): `<el-table>` renders all column slots in a hidden `.hidden-columns` measurement container. The `#status` slot created `<el-switch v-model="row.status">` there with `row.status === undefined`, triggering Element Plus's `model-value must be active-value or inactive-value` warning at setup. Added `v-if="row && (row.status === ENABLE_STATUS || row.status === DISABLE_STATUS)"` guard so the switch only renders for real rows with valid status values.
- **Defensive normalizations**:
  - `admin-ui/src/views/user/composables/index.js` — list loader now coerces `status` to `ENABLE_STATUS` (1) or `DISABLE_STATUS` (2) before passing rows to the table.
  - `admin-ui/src/views/user/composables/edit.js` — edit form normalizes `is_admin` to boolean and `status` to 1/2 on load, so the form switches always receive a valid `modelValue`.
  - `admin-ui/src/views/custom-client/index.vue` — added explicit `:active-value="true" :inactive-value="false"` to every `<el-switch>` in the Client Builder form so boolean fields never fall back to Element Plus's default `activeValue: true` / `inactiveValue: false` mismatch with `null`/`undefined` form data.

### Added (dashboard server health)
- **Backend `GET /api/admin/dashboard/health`** (`api/http/controller/admin/dashboard.go`, `router/admin.go`): new endpoint that checks ID server and relay server availability via local socket commands (`h`), retrieves relay usage data (`u`), and reads bandwidth limits (`total-bandwidth`, `single-bandwidth`, `limit-speed`). Returns structured JSON with status, top-5 connections, and bandwidth values.
- **Frontend `ServerHealth.vue`** (`admin-ui/src/components/dashboard/ServerHealth.vue`): new dashboard component showing ID/Relay server status with ConnectionPulse, active sessions count, bandwidth progress bars, top-5 connections table, and 15-second auto-refresh with countdown timer.
- **Dashboard integration** (`admin-ui/src/views/index/index.vue`): added `<server-health/>` block between Quick Connect and stat cards.
- **Removed duplicate status block** (`admin-ui/src/views/rustdesk/control.vue`): deleted the "Server command availability" section (ID Status / RELAY Status cards) since the same information now lives on the dashboard. The underlying `canSendIdServerCmd`/`canSendRelayServerCmd` check logic is preserved for child component `:can-send` props.

### Added (admin-ui UI rework foundation - PR #3, 2026-06-14)
- **Design tokens + theme system** (`admin-ui/src/styles/style.scss`, `src/store/app.js`): added light/dark tokens for surface/text/border/status colors, radius, shadows, and typography; theme mode `auto` / `light` / `dark` is stored in `localStorage` as `theme-mode` and applied through `html[data-theme]` + `html.dark` for Element Plus.
- **New UI primitives**:
  - `admin-ui/src/components/ui/ConnectionPulse.vue` for product-style status indication in shell/dashboard/auth/OAuth screens;
  - `admin-ui/src/components/ui/ThemeSwitch.vue` as a shared theme switcher;
  - `admin-ui/src/components/ui/CopyableText.vue` for copyable monospace ID/token text;
  - `admin-ui/src/components/ui/PageHeader.vue` and `PageSection.vue` for common page structure;
  - `admin-ui/src/components/ui/EmptyState.vue` and `LoadingState.vue` for shared empty/loading states;
  - `admin-ui/src/components/ui/DataTable.vue` as a shared `el-table` wrapper with loading, empty state, selection/index hooks, sort/row events, compact density, nested props, and horizontal scroll.
- **Shell/layout refresh**:
  - moved sidebar/header/menu/settings from hardcoded `#2d3a4b` / `#3f454b` to tokens;
  - removed the always-visible tags bar from the main layout;
  - sidebar brand now uses shared `--sidebar-brand-height`;
  - mobile navigation now opens through `el-drawer`, while desktop toggle still collapses the sidebar.
- **Dashboard refresh** (`admin-ui/src/views/index/index.vue`): added a Quick Connect panel with native `rustdesk://id`, web client `/webclient2/#/{id}`, and a link to devices.
- **Devices refresh** (`admin-ui/src/views/peer/index.vue`): added a permanent Status column with `ConnectionPulse`; online/offline is determined by `last_online_time < 60s`; ID uses `CopyableText`; action column reduced to `Connect` + `More`; pagination aligned via `PageSection`.
- **Monitoring visual pass**: login history, connection history, file transfers, and shared sessions now use shared `PageHeader` / `PageSection` layout without changing API/composable logic.
- **Server visual pass**: Server Commands, Server Config, and GitHub Build settings now use shared `PageHeader` / `PageSection`; advanced custom commands moved into `DangerZone` and require confirmation before `sendCmd`; terminal output now has readonly console styling, target hint, Copy/Clear controls, and empty-output placeholder.
- **Access visual pass**: Address Book entries, collections, share rules, and tags now use shared `PageHeader` / `PageSection`; address book IDs use `CopyableText`; wide action columns were reduced into `More` dropdowns without changing composables/API.
- **Users/Security visual pass**: Users, API Tokens, OAuth providers, Groups, and Device Groups now use shared `PageHeader` / `PageSection`; wide action sets were compacted into `More` dropdowns without changing CRUD/API behavior.
- **Client Builder/Profile visual pass**: Custom Client Builder and My Profile now use shared `PageHeader` / `PageSection`; build history pagination aligned with the rest of the UI.
- **My Workspace visual pass**: My Devices, My Address Book, My Address Book Collections, My Tags, My Shared Sessions, and My Login History now use shared page header/section layout; personal device and address book IDs use `CopyableText`.
- **404 refresh**: replaced the minimal `404` page with a tokenized empty-state screen linking back to dashboard.
- **Auth/OAuth visual refresh**: login, register, OAuth approve, and OAuth bind screens now use the token-based visual language and support theme switch + `ConnectionPulse`.
- **Custom Client runtime fix**: preset/upload handlers now return from `setup()` and are available to template buttons without changing backend/API contracts.
- **Monitoring filter pass**: Login History, Connection History, File Transfer History, and Shared Sessions now use the `FilterBar` primitive with collapsible panel, reset/clear, active filter count, and integrated action buttons.
- **DataTable pass**: `DataTable` wrapper applied to Users and Address Book pages with slot-based custom cells, loading state, empty state, and horizontal scroll.
- **AppDialog/AppDrawer/FormSection pass**: added `admin-ui/src/components/ui/AppDialog.vue`, `AppDrawer.vue`, and `FormSection.vue` shared primitives for unified dialog/drawer/form patterns.
- **CRUD dialog unification pass**: all remaining `el-dialog` usages migrated to `AppDialog`. Zero raw `el-dialog` remain in views.
- **DataTable migration COMPLETE**: all `el-table` usages migrated to `DataTable` across all view pages. The only remaining `el-table` is one nested inline table in `audit/fileList.vue` (directory file listing inside a cell) and the unused `my/address_book/indexv2.vue`.
- **Element Plus icons cleanup**: replaced deprecated `@element-plus/icons` imports and dependency with current `@element-plus/icons-vue`; build passes without the deprecation warning.
- **Legacy view cleanup**: removed unused `my/address_book/indexv2.vue` and replaced the nested file-info `el-table` in `audit/fileList.vue` with tokenized list markup, leaving no raw `el-table` usages in active views.
- **Review/verification**:
  - `ocr review` on the working tree found no high/medium issues; the only low nit about a magic number was fixed via `--sidebar-brand-height`;
  - `npm run build` passes; remaining output is limited to existing Vite/Rollup warnings about large chunks and `@vueuse` pure annotations.

### Known Follow-ups (admin-ui UI rework)
- Full i18n coverage for the new dashboard/auth hero copy.
- Tables, forms, dialogs/drawers, and CRUD screens still need further unification through the new primitives.
- Monitoring filters/toolbars: Connection, File Transfer, and Shared Sessions still need full FilterBar unification.

### Done (§8.9 Custom Preset - in practice 4 real UI/backend glue bugs, 2026-06-13)
No model expansion was needed: all fields were already stored in the `custom_json` text blob.
The work surfaced four real bugs that would have broken the actual GUI build flow through GitHub:

1. **`server_ip` vs `server`** (`api/http/controller/admin/custom_build.go`):
   the UI stored `server_ip`, while `tryGithubDispatch` only extracted `server`, so the value from
   the form never reached the workflow. Added fallback `server_ip` -> `server`.
2. **`custom_txt` was not being generated** (`buildCustomTxtFromForm`):
   the UI stores `permanent_password`, `hide_cm`, `deny_lan`, etc. separately rather than as an rdgen
   blob. When `custom_txt` is not explicitly provided, Go now assembles it automatically by mapping
   `permanent_password` -> `password`, `hide_cm` ->
   `verification-method: use-permanent-password` + `hide-connection-management: Y`,
   `deny_lan` -> `deny-lan-discovery: Y`, etc., then JSON-encodes and base64-encodes it.
3. **`Save as preset` created duplicates** (`api/service/custom_preset.go`):
   `CustomPresetService.Create` now performs an upsert on `(user_id, name)`, replacing by name as
   promised in §8.9.
4. **`loadPresetIntoForm` had an incomplete field list** (`admin-ui/src/views/custom-client/index.vue`):
   `app_icon_url`, `app_logo_url`, and `privacy_screen_url` were saved into the preset but not restored.
   Added them to the restore list.

### Done (§8.10 single-binary `rustqs.exe` - CLOSED, 2026-06-13)
- Reworked `rustqs-windows-min-test.yml` for full packaging:
  - reverted the L3 `BINARY_NAME` rename, because `libs/portable/generate.py` is hardcoded to look
    for `rustdesk.exe` inside `Release/`;
  - removed `mv Release ./rustdesk`; native deps (`usbmmidd`, printer driver + adapter) and the
    TopMost artifact are now downloaded directly into `flutter/build/windows/x64/runner/Release/`;
  - the L2-B step places `custom_.txt` in `Release/` **before** the packer runs, so it is packed in;
  - added new `L4 portable-pack` step:
    `cd libs/portable && pip3 install -r requirements.txt && python3 ./generate.py -f ../../{Release} -o . -e ../../{Release}/rustdesk.exe`;
  - final artifact comes from `./output/{appname}.exe`;
  - `actions/upload-artifact` now uploads `output` instead of the old `rustdesk` directory.
- Run [27462227115](https://github.com/bashrusakh/rustdesk/actions/runs/27462227115)
  succeeded in about 33 minutes. Final artifact: **one file `rustqs.exe`, 23.2 MB**
  instead of a small launcher plus a DLL folder. Exe metadata is `rustqs` and `custom_.txt`
  is packed inside the self-extracting exe.
- First attempt [27462157839](https://github.com/bashrusakh/rustdesk/actions/runs/27462157839)
  failed almost immediately with `bad decrypt` in `Resolve build config` because
  `WORKFLOW_PAYLOAD_KEY` in the fork had diverged from local
  `offline-kit/artifacts/workflow-payload.key`. Open inputs were used as a temporary bypass to
  validate §8.10. Remaining TODO: resync the key, either from the UI (`Push to GitHub Secrets`) or
  by replacing the local file.

### Fixed (Docker build)
- Added root **`.dockerignore`** to exclude `node_modules/`, `.git/`, `data/`, `rdgen-data/`,
  `rustdesk-cache/`, `**/target/`, `*.exe|*.dll|*.apk|*.msi`, and `offline-kit/artifacts/`
  from the build context. Without it, the context pulled in 155 MB of `node_modules` and host-only files.
- **Docker build fix**: production `docker/Dockerfile` now builds `admin-ui` from source inside a `node:20-bookworm` stage and no longer requires a pre-existing `admin-ui/dist/`; `.dockerignore` excludes host `node_modules/` and stale `admin-ui/dist/`.
- Reset the **admin password** to `admin123` via `apimain reset-admin-pwd` because the generated initial
  password from first-start logs was lost after restart.

### Added (GitHub Build integration - PLAN.md §8.8.5)
- Added **defaults in the GitHub Build form** (`admin-ui/src/views/server/github-build.vue`):
  when DB values are empty, it pre-fills `bashrusakh/rustdesk`, `rustqs-windows-min-test.yml`,
  and `rustqs/min-test`. The user can still change them before Save.
- Added **`DownloadByKey` endpoint** (`api/http/controller/admin/custom_build.go`):
  public (no api-token) endpoint `/api/admin/custom_build/public/download/:key` that returns a zip from
  `/rdgen-data/output/{id}/`. Capability URL is keyed by `download_key` (32 random chars), same as
  existing `DetailByKey`. Output filename format: `{app_name}-{YYYYMMDD-HHMMSS}.zip`.
- Moved **public routes out of `adg`** (`api/http/router/admin.go`): previously
  `aRPublic := rg.Group("/custom_build/public")` inherited `BackendUserAuth` from the parent group,
  so `detailByKey` and `download` returned 403 without a token. Now they are bound directly on root `g`.
- Added **NoCache middleware** (`api/http/middleware/nocache.go`) setting
  `Cache-Control: no-cache, no-store, must-revalidate`, `Pragma: no-cache`, `Expires: 0` on all
  `/api/admin/*` responses. Applied **before** `BackendUserAuth` so headers also reach 403 responses.
- Added **Axios cache-busting** (`admin-ui/src/utils/request.js`) adding `Cache-Control: no-cache`
  on all GET requests as extra protection against aggressive proxies/CDNs.
- `DispatchTest` now **polls until completion** (`api/http/controller/admin/github_build_config.go`):
  instead of returning an immediate `run_id`, it polls GitHub every 30 seconds until `status=completed`
  (max 90 min) and returns `{run_id, status, conclusion, ok, message}`. In Vue this uses a separate
  Axios request with a 95-minute timeout and shows `Build running...` -> success/failure.

### Fixed (GitHub fork cleanup)
- Removed **10 upstream workflows** from `bashrusakh/rustdesk@master`:
  `bridge.yml`, `ci.yml`, `clear-cache.yml`, `fdroid.yml`, `flutter-build.yml`, `flutter-ci.yml`,
  `flutter-nightly.yml`, `flutter-tag.yml`, `playground.yml`, `wf-cliprdr-ci.yml`.
  Kept only the needed ones: `rustqs-windows-min-test.yml` and `third-party-RustDeskTempTopMostWindow.yml`.
- Restored **`bridge.yml`** after cleanup because `rustqs-windows-min-test.yml` uses it as a reusable workflow.
  Without it, dispatch failed with HTTP 422 / workflow parse error. Restored from upstream `rustdesk/rustdesk@1.4.7`.

### Added (PLAN.md tasks)
- Added **§8.9 Custom Preset** task: extend the effective preset data with `server`, `key`, `custom_txt`,
  `logo`, and `icon`, while making UI `Save as preset` overwrite on name match.
- Added **§8.10 Single-binary `rustqs.exe`** task: replace the old `--skip-portable-pack` multi-file
  artifact with an upstream-style single binary and verify `custom_.txt` ends up inside the packed exe.

## [0.4.0] - 2026-06-11

### Changed (Architecture - Sovereign Build Strategy)
- Fully rewrote **`PLAN.md`** as the single source of truth. It now documents the sovereign build model
  (3 levels of independence: sources / build / toolchain), fork map (`rustdesk` + `hbb_common` +
  ~20 `rustdesk-org/*` repos through `vendor`), the 3-container architecture, and the offline kit with
  pinned versions (Rust 1.75, Flutter 3.24.5, LLVM 15.0.6, `vcpkg` baseline `120deac...`).
- Moved **Windows build** from Linux MinGW cross-compilation to a **native Windows builder** on a separate
  Windows server. Reason: current Flutter client cannot be cross-compiled from Linux; the MinGW path only
  reached the legacy Sciter UI and got blocked by an un-linkable `vcpkg` `libvpx.a` (ELF objects instead of COFF).
- Documented the 3-layer config injection model for `rustqs.exe`: server + key in `config.rs`, quick-support
  behavior in signed `custom.txt` (`allowCustom` patch), and branding through `sed` + portable-packer.

### Removed
- Removed `FEATURE_CUSTOM_CLIENT.md`, the outdated MinGW/Wine/NSIS build plan. The approach was rejected;
  valid parts were folded into `PLAN.md`.

### Added (Offline kit - PLAN.md §8.1)
- Added `offline-kit/versions.env` as the single source of pins: Rust 1.75, Flutter 3.24.5, LLVM 15.0.6,
  `vcpkg` baseline `120deac...`, custom Flutter engine URL, source repo for forkers.
- Added `offline-kit/freeze.sh`, an idempotent L1+L3 freeze script split by stages
  (`source`/`vendor`/`engine`/`flutter_sdk`/`vcpkg`/`rust`) and parameterized by env for downstream forks.
- Added `offline-kit/README.md` documenting how to run the freeze, how to store outputs
  (`vendor` in the fork, binaries in release assets), and how to build offline.
- Captured the full list of Windows `vcpkg` dependencies from `vcpkg.json`: `aom`, `libjpeg-turbo`,
  `opus`, `libvpx`, `libyuv`, `mfx-dispatch`, **`ffmpeg`** (`amf`/`nvcodec`/`qsv` for `hwcodec`).

### Done (Offline kit frozen)
- Ran `freeze.sh` on 2026-06-11 in `docker-build-linux-1`. Frozen into `rustdesk-cache` volume:
  1.4.7 bundle, `vendor` (2.7G, all `rustdesk-org/*` + `hbb_common`), Flutter engine,
  Flutter SDK for win+linux, `vcpkg` at baseline, Rust 1.75.0 MSI. Manifest with sha256 stored in `artifacts/MANIFEST.txt`.
- Corrected `RUST_VERSION` pin from `1.75` to `1.75.0` because the standalone MSI URL 404'd on the short version.

### Added (Windows-native builder - PLAN.md §8.3, designed, NOT tested)
- Added `docker/Dockerfile.build-win-native` with `servercore ltsc2022` + VS BuildTools (VCTools) +
  Flutter 3.24.5 + Rust 1.75 (msvc) + LLVM 15.0.6 + `vcpkg` baseline + `flutter_rust_bridge` 1.80.
- Added `docker/entrypoint-win-native.ps1` with job loop + 3 config injection layers +
  `build.py --portable --hwcodec --flutter --vram`.
- Added `docker/docker-compose.win.yml` for a Windows host with process isolation.
- Marked risky spots as `[VERIFY]` for real-host testing.
- Identified extra dependency `RustDeskTempTopMostWindow` and recorded it in PLAN §8.3a.

### Added (Autonomous session - §8.2 / §8.3a / §8.6)
- Extended `offline-kit/freeze.sh` with `thirdparty` stage: `RustDeskTempTopMostWindow` (sources,
  pin `53b548a`), `usbmmidd_v2.zip`, printer drivers. `offline-kit` became complete: 11 artifacts,
  5.0G. Fixed `record()` / manifest idempotence so partial runs do not overwrite the manifest.
- Added `offline-kit/FORK-PROCEDURE.md` documenting the sovereign fork procedure (levels A/B/C + acceptance).
- Added `.gitignore` to exclude secrets (private key `id_ed25519`, DB, `data/`, `.env`), build output,
  `node_modules`, `offline-kit/artifacts`, `.claude/`. Secret scan found no leaks in source.

### Changed (Windows builder: container -> native, owner decision)
- Final decision: build the Windows client **natively** on a separate Windows Server, no Docker.
- API <-> agent channel is an **SMB job queue folder**. Linux hosts Samba, Windows mounts it.
- Added `win-builder/setup.ps1` (toolchain, `-KitPath` support), `win-builder/agent.ps1`
  (SMB poller + 3 injection layers + `build.py`), `win-builder/README.md` (deployment + SMB).
- Removed the container-native files `docker/Dockerfile.build-win-native`, `entrypoint-win-native.ps1`,
  and `docker-compose.win.yml` as the native path became the single source of truth.

### Cleanup (owner, 2026-06-11 after §8.8.3a)
- Deleted all local Docker volumes and images. Test containers from the abandoned MinGW path
  (`build-win-test*`, `upbeat_carson`, `lucid_grothendieck`) disappeared, partially closing §8.7.
- Deleted `offline-kit` volume `rustdesk-cache`. A staging copy of 5 release assets remained in
  `offline-kit/artifacts/` (~62 MB). For standalone fallback, the kit can be re-frozen at any time.
  The GitHub track does not depend on that volume.

### Done (§8.8.3b(5) encrypted inputs + §8.8.5 scaffold, 2026-06-12)
**(5) Encrypted inputs - CLOSED**
- Generated 43-char `WORKFLOW_PAYLOAD_KEY` and stored it in fork GitHub Secrets.
  Important pitfall: `gh secret set ... --body -` via pipe adds a trailing newline under PowerShell,
  causing `bad decrypt` on the runner. Fix: use `--body $secret` without a pipe.
- Refactored the workflow to take `enc_payload` and resolve it through `Resolve build config`
  (OpenSSL AES-256-CBC + PBKDF2 + `jq` -> env vars), while still supporting open inputs as fallback.
- Migrated L1/L2/L3 steps from `inputs.X` to `RQS_*` env vars and masked sensitive values with `::add-mask::`.
- Verified with successful runs for both open-inputs and encrypted payload.

**§8.8.5 Go API - SCAFFOLD**
- Added `model/github_build_config.go`: singleton with `Token`, `PayloadKey`, and safe view.
- Added `service/github_build_config.go`: Get/Save, key generation, OpenSSL-compatible payload encryption,
  test connection, dispatch build, run status, artifact download.
- Added `controller/admin/github_build_config.go`: Get, Save, GenerateKey, Test, DispatchTest.
- Added AutoMigrate, router bind, and `DatabaseVersion` 267 -> 268.
- Added admin-ui page/API for GitHub Build settings.
- Design choice: PAT lives in admin UI / DB, not `.env`.

**SetWorkflowSecret one-click**
- Implemented with `golang.org/x/crypto/nacl/box.SealAnonymous` using the repo public key from
  `GET /actions/secrets/public-key`, then storing to `WORKFLOW_PAYLOAD_KEY`.
- Exposed via `/admin/github_build_config/sync_secret` plus `Push to GitHub Secrets` button in UI.

**Windows-job glue**
- `controller/admin/custom_build.go::submitBuild` now calls `tryGithubDispatch` for
  `platform=windows` when GitHub Build is configured, dispatches `enc_payload`, and starts a background
  `pollAndDownload` loop (30-second poll, 90-minute timeout).
- On success it downloads `rustdesk-min-test-windows.zip`, unpacks it, stores `{appname}.exe` + DLLs +
  `custom_.txt` into `/rdgen-data/output/{id}/`, and updates `CustomBuild.Status`.
- Falls back to file-queue mode for Linux/Android.

**Self review §8.8.5**
- Prevented panic in background goroutine `pollAndDownload` via `defer recover()`.
- Fixed unchecked `zf.Open()` error by moving extraction into helper `extractZipFile`.
- Removed fixed 30s `http.Client` timeout that broke 32 MB downloads.
- Replaced `context.WithTimeout(c, 60*1e9)` with `context.Background(), 60*time.Second`.

### Done (§8.8.3b full injection pipeline green, 2026-06-11/12)
- Sovereign binary assets: run [27352640159](https://github.com/bashrusakh/rustdesk/actions/runs/27352640159)
  switched `usbmmidd`/printer URLs to fork release `offline-assets-1.4.7`.
- L1 config.rs injection validated with noop run and real run.
- L3 branding validated in run [27359858171](https://github.com/bashrusakh/rustdesk/actions/runs/27359858171).
- L2 quick-support validated in run [27362132331](https://github.com/bashrusakh/rustdesk/actions/runs/27362132331),
  including `allowCustom` patch and `custom_.txt` payload.
- Exe rename polish fixed after initial path mistake: final run [27395862737](https://github.com/bashrusakh/rustdesk/actions/runs/27395862737)
  produced `rustqs.exe` with correct metadata and `custom_.txt` nearby.
- Uploaded additional optional rdgen patches to the fork for future use:
  `hidecm`, `removeSetupServerTip`, `removeNewVersionNotif`, `cycle_monitor`, `xoffline`,
  `privacyScreen`, `allowCustom.diff`.

### Done (§8.8.3a GitHub mini-test green, 2026-06-11)
- Created branch `rustqs/min-test` from tag 1.4.7 in `bashrusakh/rustdesk`.
- Added `rustqs-windows-min-test.yml`, a close copy of official `build-for-windows-flutter`
  plus `workflow_dispatch`, with the Flutter engine pulled from the fork release instead of `rustdesk-org`.
- First attempt failed at startup because of an extra input in the TopMost sub-workflow; fixed.
- Run [27341830418](https://github.com/bashrusakh/rustdesk/actions/runs/27341830418) completed:
  bridge about 6 min, topmost about 2 min, build about 37 min. Artifact `rustdesk-min-test-windows`
  confirmed the runner toolchain and fork release source path work.

### Done (§8.8 GitHub track - implementation start, 2026-06-11)
- Owner installed `gh` CLI (`bashrusakh`, `repo`/`workflow` scopes); GitHub API access confirmed.
- Forked `bashrusakh/rustdesk` and `hbb_common` publicly.
- Created fork release `offline-assets-1.4.7` with engine 63M, `usbmmidd`, printer driver + adapter,
  and generated `sha256sums`. Flutter SDK / Rust / `vcpkg` were not uploaded because GitHub runners install them directly.

### Changed (STRATEGY: GitHub-first, owner decision 2026-06-11)
- Split the two independence goals: from RustDesk upstream (real risk, solve now) and from GitHub
  platform (low risk, not priority). `rustqs.exe` is built through GitHub Actions in the rustdesk fork.
- Rewrote PLAN §§1/3/4/6 to reflect this and added §8.8 as the active track.
- Marked §§8.3/8.4 (standalone + SMB) as FALLBACK/frozen.
- Added `github-build/README.md` documenting URL repointing in forked `generator-windows.yml`,
  GitHub Secrets, and Go `workflow_dispatch` integration. Also documented that rdgen already contains
  `fetch-encrypted-secrets`, `ZIP_PASSWORD`, and `save_custom_client`.

### Added (win-builder)
- Added `win-builder/SERVER-SETUP.md`, a detailed Windows build server guide:
  OS choice (Server 2022 vs Windows 11 Pro), hardware sizing, provisioning options
  (Hyper-V VM / physical / cloud), long paths, antivirus exclusions, SMB, service agent,
  first end-to-end test, and security.

### Verified / Fixed (offline-kit)
- Proved **L1 sovereignty**: `cargo metadata --offline` on the vendored tree resolves all 1049 crates
  from `vendor` with no network access.
- Rebuilt and verified the **bundle** on full history (70M). Clone-back on tag 1.4.7 succeeded.
  Fixed the earlier shallow-clone defect (`remote did not send all necessary objects`).

### Notes
- Abandoned approaches are now documented in `PLAN.md` §9 so future agents do not repeat them.
- Ballast cleanup is delayed until the final phase (§8.7).
- `.gitignore` + secret scan are mandatory before the first public push (§8.6).
- Repository was still not under git at that point; `git init` was part of §8.6.

## [0.3.0] - 2026-06-09

### Added
- `PLAN.md` as a unified project roadmap for other agents
- `admin-ui/` forked from `lejianwen/rustdesk-api-web` (Vue 3 + Element Plus)
- New nav structure: Dashboard, Devices, Users, Groups, Address Book, Security, Monitoring,
  Custom Client, Server, My Profile
- New page stubs: Custom Client Builder, Server Config
- Dashboard route at `/dashboard/`
- i18n keys for all new nav sections (`en.json`)

### Changed
- Admin UI default locale: `zhCn` -> `en`
- Admin UI store default language: `'zh-CN'` -> `'en'`
- `en.json` `ChangeLang`: `Switch Language`
- Title: `Rustdesk API Admin` -> `RustDesk Server Admin`
- Router fully restructured into logical nav groups
- All admin routes moved under `/admin/*`
- My Profile routes cleaned up under `/my/*`
- Login redirect: `/` -> `/dashboard`
- `PLAN.md` updated with all completed phases

### Added (Phase 4 - Custom Client Builder Backend)
- `docker/Dockerfile.build-linux` for Linux `.rpm` + Android `.apk`
- `docker/Dockerfile.build-win` for Windows `.exe`/`.msi` via MinGW + NSIS cross-compiler
- `docker/entrypoint-linux.sh` job poller
- `docker/entrypoint-win.sh` job poller
- `api/model/custom_build.go` model
- `api/service/custom_build.go` CRUD service
- `api/http/request/admin/custom_build.go` form/query structs
- `api/http/controller/admin/custom_build.go` CRUD controller + build job submission by file tickets
- Routes under `/admin/custom_build/*` and public `detailByKey`
- `docker-compose.yml` services `build-linux` + `build-win` with shared `rdgen-data`
- `CustomBuild` added to AutoMigrate; `DatabaseVersion` 265 -> 266
- `.env.example` build agent section

### Added (Phase 5 - Dashboard API + UI)
- `GET /api/admin/dashboard/stats`
- `admin-ui/src/api/dashboard.js`
- Rewritten dashboard view with stat cards, quick actions, recent activity
- New dashboard i18n keys
- Route bind for dashboard stats

### Added (Phase 6 - Custom Client UI Page)
- Full build form in `admin-ui/src/views/custom-client/index.vue`
- `admin-ui/src/api/custom_client.js`
- 50+ i18n keys for build form fields and status labels

### Added (Phase 7 - Server Config UI Page)
- Rewritten `admin-ui/src/views/server/config.vue`
- New `AllConfig` endpoint in `api/http/controller/admin/config.go`
- `GET /config/all` route under auth
- `admin-ui/src/api/config.js` `all()`

### Changed (Phase 8 - Polish)
- Chinese language names in app store normalized
- `peer/index.vue` warning translated to `Not implemented`
- `rustdesk/blocklist.vue` and `blacklist.vue` tips translated to English
- Removed commented Chinese labels in address book views

### Changed (Phase 9 - Dockerfile finalize)
- Dockerfile stage 3 uses local `admin-ui/` instead of `git clone`

### Added/Fixed (Phase 10 - Integration verification)
- Added missing i18n keys: `Download`, `Reset`
- Code review of Go patterns, frontend/backend route alignment, permissions, and i18n found no issues

## [0.2.0] - 2026-06-08

### Fixed
- Docker build: `Cargo.lock` version 4 requires Rust 1.88+
- Docker build: missing `go.sum`, generated with `go mod tidy`
- Docker build: `sqlx` requires `DATABASE_URL` for compile-time query checks

### Changed
- Dockerfile Rust base image updated to `rust:bookworm`
- Dockerfile deletes `Cargo.lock` before build to regenerate compatible versions
- Dockerfile creates a dummy SQLite DB for `sqlx` macro compilation

## [0.1.0] - 2026-06-08

### Added
- Unified Docker image with Rust server + Go API + Web Admin
- Multi-stage Dockerfile (Rust, Go, Node.js, `s6-overlay`)
- Single `docker-compose.yml` for easy deployment
- Web Admin panel at `/admin/`
- Web Client for browser-based remote desktop
- JWT authentication
- Mandatory login support (`MUST_LOGIN`)
- OAuth2/OIDC support
- LDAP authentication
- User/Group/Device management
- Address Book
- Audit logs
- Captcha and ban system
- Swagger API documentation
- `CHANGELOG.md`

### Changed
- Admin panel route changed from `/_admin/` to `/admin/`
- All Chinese text removed from source code (`api/`)
- Documentation made English-only

### Removed
- Chinese README files (`api/README.md`)
- Chinese comments from source code
