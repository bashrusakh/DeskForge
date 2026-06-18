# Review — PR #22 (`fix(audit-round3): resolve 11 more findings`)

Branch: `fix/audit-round3` → base `fix/audit-round2` (**not `main`**).
Reviewer: Claude (orchestrator). Coder: Sonnet.
Source of truth: `AGENTS.md` + `CONTRIBUTING.md` (read).

## Scope of the PR (13 commits)

Backend (Go API):
- L-001 — remove dead `google` oauth import (`api/service/oauth.go`)
- L-003 — replace `sync.Once` with mutex retry for version read (`api/service/app.go`)
- L-004 — TCP response buffer 1024 → 4096, then refined to `io.Copy + LimitReader(1 MiB) + 2s deadline` (`api/service/serverCmd.go`)
- L-008 — null-out `group_id` / `device_group_id` before delete in transaction (`api/service/group.go`)
- L-014 — OAuth callback templates: read message via `data-attribute` (`oauth_fail.html`, then mirrored in `oauth_success.html`)
- L-024 — server-side validation `omitempty,oneof=S256 plain` on `PkceMethod` (`api/http/request/admin/oauth.go`)
- L-026 — `os.RemoveAll` build output dir on delete (`api/service/custom_build.go`)

Frontend (Vue admin-ui):
- L-009 — fallback `ElMessage.info` after 3s in `connectByClient` (`admin-ui/src/utils/peer.js`)
- L-012 — tooltip on failed-build tag showing `build_log` (`admin-ui/src/views/custom-client/index.vue`)
- L-018 — descriptive label above OAuth RedirectUrl (`admin-ui/src/views/oauth/index.vue`)
- L-019 — rename "Create" → "Save Configuration" (custom-client)
- i18n: add `RustDeskClientNotFound`, `SaveConfiguration`, `CopyThisUrlToProvider` to en/ru/zh_CN; drop unreachable `|| 'fallback'` chains

Docs: `CHANGELOG.md`, `audit-report.md`.

---

## Progress checklist (resume from first unchecked item if interrupted)

- [x] PR base branch is correct (`main` vs `fix/audit-round2`) — see Finding #2
- [x] L-001 dead-imports cleanup — see Finding #11
- [x] L-003 `sync.Once` → mutex retry — see Finding #9
- [x] L-004 serverCmd TCP read — see Finding #5
- [x] L-008 group delete transaction — see Finding #1 (**BLOCKER**) + Finding #3 + Finding #4
- [x] L-014 OAuth template XSS — see Finding #7
- [x] L-024 pkce_method validator — see Finding #8
- [x] L-026 build artifact cleanup — see Finding #6
- [x] L-009 protocol fallback — see Finding #10
- [x] L-012 build_log tooltip — see Finding #12
- [x] L-018 / L-019 — i18n consistency, missing keys, dead fallback chains — see Finding #13
- [x] i18n round-3 fixup (eaf9066) — see Finding #13
- [x] PR base branch points to `fix/audit-round2` — see Finding #2

---

## Findings

Severity legend: **BLOCKER** (do not merge as-is), **HIGH** (fix before merge), **MED** (fix in this PR if cheap), **LOW** (nit / follow-up).

---

### Finding #1 — **BLOCKER** — L-008 `DeviceGroupDelete` references a column that does not exist

**File:** `api/service/group.go` (new code in PR).

**The patched code:**
```go
func (us *GroupService) DeviceGroupDelete(u *model.DeviceGroup) error {
    return DB.Transaction(func(tx *gorm.DB) error {
        if err := tx.Model(&model.Peer{}).
            Where("device_group_id = ?", u.Id).
            Update("device_group_id", 0).Error; err != nil {
            return err
        }
        return tx.Delete(u).Error
    })
}
```

**Why this is a blocker:** there is no `device_group_id` column on the `peers` table.

Evidence:
- `api/model/peer.go` — Peer has only `GroupId uint \`gorm:"...;index"\`` (column `group_id`). No `DeviceGroupId` field.
- `api/cmd/apimain.go:288` `AutoMigrate(&model.Peer{}, …)` is the only schema source — no separate ALTER for `device_group_id`.
- `grep -r device_group_id api/` returns 0 hits in code (only `audit-report.md` and the new patch).
- The codebase actually overloads **`peer.group_id`** to point at a *DeviceGroup* row. See:
  - `api/http/controller/api/group.go:97-110` — builds `dGroupNameById` from `DeviceGroupList`, then looks up `dGroupNameById[peer.GroupId]`.
  - `admin-ui/src/views/peer/index.vue:237` — `import { list as groupList } from '@/api/device_group'` populates the `formData.group_id` dropdown.

**Impact:** every `DeviceGroupDelete` call now issues `UPDATE peers SET device_group_id = 0 WHERE device_group_id = ?`, which all three GORM dialects (SQLite/MySQL/Postgres) reject with `no such column`. `DB.Transaction(...)` rolls back and `DeviceGroupDelete` returns an error — **the admin "Delete Device Group" button stops working** for every install once the PR ships. This is a regression caused by the fix; before the PR, DeviceGroup deletion worked (it just left peers with a dangling `group_id`).

**Right level of fix:**
- The audit-report itself was wrong about the column name; the PR followed it without verifying. The real schema overloads `peer.group_id` for both `Group` and `DeviceGroup`, which is a *separate, pre-existing* design problem (M-class). Fixing the orphan-reference bug at the SQL/GORM level requires first deciding whether to add a real `device_group_id` column (model + migration) or to keep the overload and only null `peer.group_id` when the deleted DeviceGroup id is the value stored there.
- Minimum viable patch for this PR: change `device_group_id` → `group_id` in both the `Where` and the `Update` so the SQL runs. Document in a comment that `peer.group_id` is overloaded.
- Better follow-up (separate PR): introduce `Peer.DeviceGroupId` + migration + backfill, then update controllers/UI to use it, then null it on delete. That is out of scope for an audit-round-3 cleanup PR.

**Verification before merge:** add `go test ./service/...` covering a DeviceGroup delete with at least one peer assigned, against SQLite. The current code passed `go build` because GORM only validates the SQL at run-time.

---

### Finding #2 — **HIGH** — PR is targeted at the wrong base branch

**Per CONTRIBUTING.md:** *"`main` is the protected default branch … every change goes through a Pull Request"* and the workflow snippet shows branches always rebased onto fresh `main` before opening the PR.

**Actual state:** PR #22 is opened against `fix/audit-round2`, not `main`. PR #20 went into `main` (round 1) and #21 was round 2; round 3 should follow the same shape.

**Impact:**
- Stacking on `fix/audit-round2` means #22 cannot land until #21 lands first, and the squash-merge target is a feature branch that the protection rules don't cover.
- CodeRabbit auto-review is disabled on non-default base branches — explicitly called out in the bot's comment on the PR. So an extra reviewer was silently skipped.
- The diff range shown on GitHub is `#21..#22` only; readers can't easily see whether #21 commits are part of this review surface.

**Recommendation:** rebase `fix/audit-round3` onto current `origin/main` and change the PR base to `main`. Squash-merge as usual.

---

### Finding #3 — **MED** — L-008 fix nulls only `user.group_id`, ignores `peer.group_id` for regular `Group` deletes

**File:** `api/service/group.go` — `GroupService.Delete`.

**Patched code:**
```go
func (us *GroupService) Delete(u *model.Group) error {
    return DB.Transaction(func(tx *gorm.DB) error {
        if err := tx.Model(&model.User{}).
            Where("group_id = ?", u.Id).
            Update("group_id", 0).Error; err != nil {
            return err
        }
        return tx.Delete(u).Error
    })
}
```

**Gap:** `model.Peer` also has `GroupId uint \`json:"group_id"\`` (the same overloaded column called out in Finding #1). Deleting a regular `Group` whose id happens to equal a `peer.group_id` value leaves peers with a dangling reference — exactly the bug the audit complained about, just for peers instead of users.

The fix-level reasoning was correct (do it in the service, in a transaction), but the audit/coder forgot that the overload spans two child tables. Either:
- mirror the User update for Peer (`tx.Model(&model.Peer{}).Where("group_id = ?", u.Id).Update("group_id", 0)`), with a comment that this is the overloaded column shared with DeviceGroup, or
- short-circuit and treat this fully under the larger schema cleanup follow-up.

Connected to Finding #1 — both are symptoms of the same overloaded column.

---

### Finding #4 — **LOW** — inconsistent transaction style between `group.go` (new) and `user.go` (existing)

`user.go:246-282` opens a transaction with `tx := DB.Begin(); … tx.Rollback() / tx.Commit()` and the rollback is repeated per `if err`. The new `group.go` code uses the cleaner callback form `DB.Transaction(func(tx *gorm.DB) error { … })`, which handles rollback/commit automatically.

Both are valid GORM patterns. The new code is the better one. Not a bug, just a style drift in a repo that has two patterns now. Suggest noting in `AGENTS.md` (under "Architecture patterns") that `DB.Transaction(func(tx))` is preferred for new code so future changes converge.

---

### Finding #5 — **MED** — `serverCmd.go` read-loop fix is good, but `SendCmd` v6→v4 fallback now waits up to 4 s on failure

**File:** `api/service/serverCmd.go` (after both `ae333da` and the refinement in `6a3f6a5`).

The refined read path (`io.Copy + io.LimitReader(conn, 1<<20)` under a 2 s `SetReadDeadline`) is correct and clearly an improvement over the original `Sleep(100ms) + Read(1024)`. The `errors.As(&nerr) && nerr.Timeout()` branch correctly suppresses the deadline error (which is the *expected* signal that the server finished writing and the connection is idle).

But: `SendCmd` (line 43) calls `SendSocketCmd("v6", ...)`; on error falls through to `SendSocketCmd("v4", ...)`. After this PR, "error" for v6 now includes the case where v6 listened but never responded — which under the new 2 s deadline becomes a 2 s wait, then another 2 s for v4. Net latency on a misconfigured server is now ~4 s instead of the previous ~100 ms.

Considerations:
- The relay/rendezvous TCP control sockets generally close after a single response, so `io.Copy` returns on EOF in <50 ms on a healthy server. The 2 s deadline only kicks in for hung/blocked servers. So real-world impact is small.
- Better fix level (follow-up): only fall through v6→v4 on `net.OpError` of type "connection refused"/"no such host"; treat read-timeout as a final error from v6 instead of retrying on v4.

Acceptable for this PR; flag for a follow-up.

Also notice the deadline is set *before* the read. If the server is slow to write the FIRST byte, the 2 s clock has been ticking since `SetReadDeadline`. Real servers respond quickly, but for very large blocklist responses this could matter. Consider `conn.SetReadDeadline(time.Time{})` after the first successful read, or use `conn.SetDeadline` only on the connection-wide timeout. Low priority.

---

### Finding #6 — **MED** — L-026 `Delete` hardcodes `/rdgen-data/output/<id>` for the third time

**File:** `api/service/custom_build.go`.

```go
_ = os.RemoveAll(filepath.Join("/rdgen-data", "output", fmt.Sprintf("%d", u.Id)))
```

Issues:
1. **Duplicated path construction.** The identical `filepath.Join("/rdgen-data", "output", fmt.Sprintf("%d", build.Id))` already exists in `api/http/controller/admin/custom_build.go:126` (download) and `:316` (build-result write). Now there's a third copy. This is exactly the case the project's working rules call out: *"Persistence-after-mutation belongs inside the mutation helper"* and "API calls/paths should use a shared abstraction". A `func buildOutputDir(id uint) string` helper in `api/service/custom_build.go` (or a `paths` package) and three call sites updated would centralize the convention and let the path become config-driven later.
2. **Silently swallowed error.** `_ = os.RemoveAll(...)` means a permission error or path-traversal failure (impossible here because `u.Id` is a `uint`, but still) leaves orphans and we never know. At minimum log it: `if err := os.RemoveAll(dir); err != nil { Logger.Warnf("failed to remove %s: %v", dir, err) }`.
3. **Order of operations.** Filesystem cleanup happens *before* the DB delete. If the DB delete fails, the artifacts are already gone but the DB row still references them. The current download path (`controller/admin/custom_build.go:126`) recovers gracefully (404 if file is missing), so the user-visible effect is "Delete button half-failed → row still listed → Download is 404". Reasonable order would be: DB delete first, then best-effort cleanup. Either order has tradeoffs; pick one and comment why.

Fix-level verdict: the fix is at the right layer (service), but the path duplication should be extracted now — three copies hits the project's own threshold for "shared at the root level".

---

### Finding #7 — **MED** — L-014 OAuth-template XSS fix: correct direction, but the **underlying** XSS risk is `/api/oidc/msg`, not the template

**Files:** `api/resources/templates/oauth_fail.html`, `oauth_success.html`, and `api/http/controller/api/ouath.go:266-300` (Message handler).

The template fix (read `msg` from a `data-message` attribute via `getAttribute`, then `encodeURIComponent` before stuffing into the script URL) is sound — `html/template` in HTML-attribute context auto-escapes, and `encodeURIComponent` makes the URL parameter safe.

What's left:
1. **`Message` handler is a JS-by-string-concat sink (`api/http/controller/api/ouath.go:283-293`):**
   ```go
   res = utils.StringConcat(";title='", title, "';")
   res = utils.StringConcat(res, "msg = '", msg, "';")
   ```
   It returns this as `application/javascript`. Today `title`/`msg` come from `i18n.Message{ID: mp.Title/mp.Msg}` lookups; if the lookup misses the entire arm is skipped, so today the path is safe. But the *primitive* — building JS source by concatenating localized strings into single-quoted literals — has zero escaping. The first translation that contains an apostrophe (`L'Oauth a réussi`) breaks the page, and any future change that lets caller-controlled text reach `mp.Title`/`mp.Msg` (or that lets the i18n bundle take user input) becomes an XSS. The right fix is to write JSON: `c.Header("Content-Type","application/javascript"); fmt.Fprintf(c.Writer, "title=%s;msg=%s;", jsonString(title), jsonString(msg))` (using `encoding/json.Marshal` for the strings, which produces valid JS literals with proper escaping). That fix lives one layer up from the templates and would have prevented the original finding.
2. **CSP / inline-script.** Both templates still rely on inline `<script>` (3 blocks each) and an externally loaded `<script src=…lf9-cdn-tos.bytecdntp.com…>` font CDN. The data-attribute pattern is good defense in depth, but a `Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'` header from the gin handlers would harden every OAuth page in one place. Worth a separate "S-class" follow-up.
3. **Mirrored success-template fix is correct.** The followup commit `6a3f6a5` patched `oauth_success.html` the same way. Good — the original round-3 fix was inconsistent (fail only). The PR description correctly admits this.

Verdict: the fix is at the wrong level — it patched the *template* (3 places) when the *primitive* is the JS-emitting handler. The template patch is still worth keeping (defense in depth), but if the orchestrator's rule is "fix at the primitive," the Message handler is the primitive.

---

### Finding #8 — **LOW** — L-024 PKCE validation has a gap when `pkce_enable=true` and `pkce_method=""`

**File:** `api/http/request/admin/oauth.go`.

```go
PkceMethod   string `json:"pkce_method" validate:"omitempty,oneof=S256 plain"`
```

`omitempty` means an empty string passes validation. The model code (`api/model/oauth.go:83`) then defaults empty → `PKCEMethodS256`, so the practical outcome is "empty becomes S256". That's reasonable, but the form-level validator does not enforce the dependency between `PkceEnable` and `PkceMethod` — there's no rule that says "if `PkceEnable` is true, `PkceMethod` is required (and one of S256/plain)".

Today this is harmless because of the model default. If anyone later removes the default or moves the default into the controller layer, the validator will silently accept garbage states. Consider a `validate:"required_if=PkceEnable true,oneof=S256 plain"`-style rule, or move the default *into* the form's `ToOauth()` so both layers agree.

Also: the existing `Issuer` field has `validate:"omitempty,url"`. For OIDC, `Issuer` is effectively required, and the audit explicitly noted this. Not in scope for L-024 itself, but the audit ticket mentions it and the PR description acknowledges it's deferred — fine.

---

### Finding #9 — **LOW** — L-003 mutex rewrite is correct, but the first commit (484a15e) was a no-op

**File:** `api/service/app.go`.

The final state after `6a3f6a5` uses `sync.Mutex` with a re-check under the lock — correct, race-free, and retries on transient read failure (the stated goal). The existing concurrent test in `app_test.go:TestMultipleGetAppVersion` still passes (no shared write under the new lock).

The intermediate commit `484a15e` — described as "replace sync.Once with package-level versionOnce var" — was effectively cosmetic and left the bug (still used `Do`, still sticky). The followup `6a3f6a5` did the real fix. For a squash-merged PR this washes out, but if the project keeps individual commits, `484a15e` is misleading and could be squashed away or replaced. Per CONTRIBUTING.md the merge is `--squash --delete-branch`, so this is moot at merge time. Note for the orchestrator: the first attempt missed the root cause; the second commit caught it. That's the "wrong-level fix → corrected" pattern this review was asked to look for.

---

### Finding #10 — **LOW** — L-009 `connectByClient` fallback uses a heuristic that misfires in both directions

**File:** `admin-ui/src/utils/peer.js`.

```js
setTimeout(() => {
  if (!document.hidden) {
    ElMessage.info(T('RustDeskClientNotFound'))
  }
}, 3000)
```

- **False positive:** RustDesk client is installed but slow to launch (cold start on Windows can exceed 3 s). The "not found" toast pops up *after* the client opens.
- **False negative:** the user switches tab in <3 s for any reason; toast is suppressed even though the client genuinely isn't installed.
- **Cross-platform inconsistency:** on macOS, `rustdesk://` triggers the OS' "Open in app?" dialog, which keeps the page visible and the toast still fires — even though the OS already told the user there is no handler.

Browsers don't expose a reliable "protocol handler was/wasn't accepted" signal, so a 3 s timeout heuristic is roughly the state of the art. Just don't oversell it — comment in the code that this is best-effort and tuneable.

**Fix level is correct:** the helper is imported from 5 different views (`peer/index`, `index/index`, `my/address_book/index`, `my/peer/index`, `address_book/index`). One change in `utils/peer.js` covers them all. ✅

---

### Finding #11 — **LOW** — L-001 dead-import cleanup is incomplete in nearby file

`api/service/oauth.go` removes two commented-out imports. Good. But the same file still has commented-out blocks elsewhere (e.g. `//fmt.Println("bind", ty, userData)` in `api/http/controller/api/ouath.go:182`). Scope of the audit ticket was just the two imports — leaving the others is consistent with "smallest correct change" — but worth flagging that the audit-report's "cleanliness" finding pattern has more matches if anyone wants a follow-up.

---

### Finding #12 — **LOW** — L-012 tooltip showing `build_log` should be guarded for safety and overflow

**File:** `admin-ui/src/views/custom-client/index.vue`.

```vue
<el-tooltip v-if="row.build_log" :content="row.build_log" placement="top" :show-after="500" max-width="400">
```

- Element Plus `el-tooltip` renders `content` as text (not HTML) by default, so the build_log can't run scripts. ✅
- `build_log` is typically a multi-line shell transcript; `max-width="400"` constrains horizontal layout, but Element Plus does not wrap newlines unless `:raw-content="true"` and CSS `white-space: pre-line` are set. The tooltip will likely render as a single very long unbroken string. Worth either (a) preformatting with `<pre>` and `raw-content`, or (b) truncating to the last N lines (the failure is almost always on the last few lines of the log).
- `:show-after="500"` is fine.

Functional, just not pretty. Leave for follow-up unless build_log readability matters to users.

---

### Finding #13 — **LOW** — i18n cleanup is correct but scoped only to round-3 strings

**Files:** `admin-ui/src/utils/peer.js`, `views/custom-client/index.vue`, `views/oauth/index.vue` (commit `eaf9066`).

The commit message explains the bug accurately: `T(key)` returns the bare key on miss, so the right-hand side of `T(key) || 'fallback'` is unreachable, and the 3 newly introduced keys were missing from `en/ru/zh_CN.json`. The fix adds the keys and drops the dead `|| 'fallback'` from those 3 sites.

`grep -n "T(['\"][A-Za-z_]+['\"])\s*\|\|" admin-ui/src` still returns ~10 hits in `views/custom-client/index.vue` (`LoadPreset`, `SaveAsPreset`, `Branding`, `AppIcon`, `Upload`, `PrivacyScreen`, `AppLogo`, `PresetName`). I verified each of those keys exists in `en.json`, so they render correctly — but the `|| 'fallback'` is just as dead as the three the PR removed. Scope of this PR was strictly "fix the regression my earlier commits introduced," which the commit message states explicitly, and that's defensible per the "smallest correct change" rule.

Recommendation: open a `chore/admin-ui: drop dead T() fallback chains` issue for the remaining ~10 occurrences. Not blocking this PR.

---

### Finding #14 — **LOW** — `audit-report.md` math: "11 findings resolved" includes L-005 which was already fixed in PR #21

Header note: *"52 findings resolved … 7 findings remain open"*. The "Resolved in PR #22 — 11 findings" list includes L-005 with the parenthetical "(also fixed in PR #21)". L-005 should not double-count. Cosmetic.

---

### Finding #15 — **LOW** — i18n diff for `ru.json` is patched against a stale base

The diff hunk for `admin-ui/src/utils/i18n/ru.json` shows `RelayOffline` as the last key before `}`, but the on-disk file has two more keys (`ImportMissingColumns`, `ExportTruncated`) appended after `RelayOffline`. Git's 3-way merge will still apply the new keys, but the post-merge ordering will interleave the new keys between `RelayOffline` and the two later additions, which doesn't match the order in `en.json` / `zh_CN.json`. Purely cosmetic — JSON object key order isn't semantically meaningful — but if you care about side-by-side diffability across the three locale files, append the three new keys at the end of `ru.json` instead of after `RelayOffline`.

The PR was almost certainly cut before `ImportMissingColumns` and `ExportTruncated` landed on the base branch; another reason to rebase onto `main` (Finding #2).

---

## Summary

| # | Severity | Title | Status (after orchestrator pass) |
|---|----------|-------|-----------------------------------|
| 1 | **BLOCKER** | `DeviceGroupDelete` references nonexistent `device_group_id` column | ✅ Applied (`api/service/group.go`) — `device_group_id` → `group_id` with inline comment explaining the overload |
| 2 | HIGH | PR targets `fix/audit-round2`, not `main` | ⚠️ Doc-only — cannot change branch base from files; user must `git rebase --onto main fix/audit-round2 fix/audit-round3` and re-target the PR |
| 3 | MED | Group `Delete` ignores peers' overloaded `group_id` | ✅ Applied (`api/service/group.go:Delete`) — now clears `user.group_id` AND `peer.group_id` in one transaction |
| 4 | LOW | Two GORM transaction styles in the repo | ➖ Not changed — would be a drive-by edit to `user.go`; flag for separate `docs(agents): document DB.Transaction as preferred` PR |
| 5 | MED | serverCmd v6→v4 fallback now waits up to 4 s | ✅ PR-fix applied (`io.Copy + LimitReader + 2s deadline`); the 4s worst-case fallback latency is documented in the review but acceptable |
| 6 | MED | `/rdgen-data/output/<id>` path duplicated in 3 files | ✅ Applied — added `service.BuildOutputDir(id)` helper, refactored both controller call sites and the service `Delete`; DB-first ordering; `os.RemoveAll` error now logged via `Logger.Warnf` |
| 7 | MED | OAuth XSS fix is at template; primitive is `Message` handler | ✅ Applied — `Message` handler rewritten to emit JS via `encoding/json.Marshal` (`api/http/controller/api/ouath.go`); template `data-attribute` patches kept as defence in depth |
| 8 | LOW | `PkceMethod` validator lacks `required_if PkceEnable=true` | ✅ Applied — tag is now `omitempty,oneof=S256 plain,required_if=PkceEnable true` |
| 9 | LOW | First app.go commit was a no-op; only the followup actually fixed L-003 | ✅ Final mutex+double-check version is what the working tree carries; will collapse on squash-merge |
| 10 | LOW | `connectByClient` 3 s heuristic misfires | ✅ Applied — best-effort caveat added as code comment |
| 11 | LOW | More commented-out code remains in `oauth.go` / `ouath.go` | ➖ Not changed — out of scope for an audit-cleanup PR per AGENTS.md "smallest correct change" |
| 12 | LOW | `build_log` tooltip doesn't preformat newlines | ✅ Applied — tooltip content is now a `<pre>` block with `white-space:pre-wrap`, `max-height:300px`, scroll |
| 13 | LOW | Dead `T(key) \|\| 'fallback'` chains remain ~10× | ✅ Applied — removed all remaining fallback chains in `views/custom-client/index.vue`, AND added missing `LoadPreset`/`SaveAsPreset`/`PresetName`/`Branding`/`AppIcon`/`AppLogo`/`PrivacyScreen`/`Upload` keys to `ru.json` and `zh_CN.json` (they were only in `en.json` before, so non-English users would otherwise have seen the bare key after the fallback drop) |
| 14 | LOW | Audit-report double-counts L-005 | ✅ Applied — L-005 noted as "already resolved in PR #21" and not double-counted; resolved-total corrected to 10 (not 11) |
| 15 | LOW | `ru.json` patched against stale base | ✅ Applied — new keys appended at the end of `ru.json` after `ExportTruncated`, matching the placement in `en.json` / `zh_CN.json` |

### Files edited by the orchestrator pass

Backend (Go):
- `api/service/group.go` — Findings #1, #3
- `api/service/app.go` — final L-003 mutex form
- `api/service/serverCmd.go` — final L-004 io.Copy form
- `api/service/custom_build.go` — Finding #6 (helper, DB-first, logged error)
- `api/service/oauth.go` — L-001 imports
- `api/http/request/admin/oauth.go` — Finding #8 (required_if)
- `api/http/controller/admin/custom_build.go` — Finding #6 (use shared helper)
- `api/http/controller/api/ouath.go` — Finding #7 (Message handler json.Marshal)
- `api/resources/templates/oauth_fail.html`, `oauth_success.html` — L-014 templates

Frontend (Vue):
- `admin-ui/src/utils/peer.js` — L-009 + Finding #10 (best-effort comment)
- `admin-ui/src/views/custom-client/index.vue` — L-012/L-019 + Finding #12 (pre tooltip) + Finding #13 (drop dead fallbacks)
- `admin-ui/src/views/oauth/index.vue` — L-018
- `admin-ui/src/utils/i18n/en.json` — 3 new keys
- `admin-ui/src/utils/i18n/ru.json` — 3 + 8 new keys (Finding #13 + #15)
- `admin-ui/src/utils/i18n/zh_CN.json` — 3 + 8 new keys (Finding #13)

Docs:
- `CHANGELOG.md` — Round-3 entries
- `audit-report.md` — per-finding "Fixed in PR #22" markers; resolved/open totals corrected (Finding #14)

### Not actionable from files (organizational)

- Finding #2 (rebase onto `main`) — requires `git rebase` + re-targeting the PR base on GitHub.
- Finding #9 (squash-on-merge collapses the no-op intermediate commit) — handled by the standard `gh pr merge --squash --delete-branch` flow in CONTRIBUTING.md.

### Extra fix caught during orchestrator sanity-check

**OAuth template script placement (NOT in the original review):** the L-014 PR-fix puts `document.getElementById('fail-data').getAttribute('data-message')` inside a `<script>` block in `<head>`. The body has not been parsed yet at that point, so the element doesn't exist and `.getAttribute(...)` throws `TypeError: Cannot read properties of null`. The original PR commit had this bug — moving the data into a `data-message` attribute is correct, but the read has to happen AFTER the element exists in the DOM. Orchestrator pass moved both scripts to the end of `<body>` (after the element) and added a null-guard for safety. This applies to both `oauth_fail.html` and `oauth_success.html`. Without this fix the OAuth callback pages would white-screen with a console exception.

---

## Resume notes

If this conversation runs out of context, the working tree on disk now reflects "PR #22 + all orchestrator fixes" — the user can copy the edits into the `fix/audit-round3` branch and resolve any merge conflict against the actual PR commits. Everything is documented in the Summary table above with the file paths touched.

Two pieces of context most likely to be lost:

1. The `device_group_id` column doesn't exist. Confirmed by: (a) no field on `model.Peer`; (b) `AutoMigrate` is the only schema source; (c) `grep -r device_group_id api/` returns zero hits in code. The codebase overloads `peer.group_id` for both `Group` and `DeviceGroup` references — see `api/http/controller/api/group.go:97-110` and `admin-ui/src/views/peer/index.vue:237`. The fix in `service/group.go` clears the overloaded column on BOTH kinds of delete and the long-term schema split is deferred to a separate PR.
2. `T('Key') || 'fallback'` is dead because `T()` returns the bare key on miss. Removing the fallback for keys that were only in `en.json` would have regressed non-English users — that's why the orchestrator pass ALSO added the 8 missing keys to `ru.json` and `zh_CN.json` (not just dropped the chains).
