# Audit — custom-agent build workflow

Scope: end-to-end review of the Django generator (`rdgen/`) and the Go API
(`api/`) used by the custom-client build pipeline. Looks at the workflow
as a whole — how the form submission, the GitHub Actions runner, and the
download/cleanup endpoints fit together — not just the lines touched by
the earlier CodeQL audit.

Legend: ✅ fixed in this PR · ⚠️ flagged, not fixed (out of scope or
architectural) · ❌ confirmed bug, can't fix without breaking the protocol.

**Status:** all ✅ items below are landed on this branch as of the most
recent commit. The PR is ready for review; the ⚠️ items are listed for
the operator to consider during deployment but are not blockers.

---

## A. Critical — workflow-breaking bugs

### A1 — `appname.upper != "rustdesk".upper` is a method comparison ✅
`rdgen/rdgenerator/views.py:170`
```python
if appname.upper != "rustdesk".upper and appname != "":
    decodedCustom['app-name'] = appname
```
`upper` without `()` compares **bound method objects** — they're never
equal, so the branch is always entered (when `appname != ""`). The
"don't override app-name when it's still the default" logic was dead.

Fix: call the methods (`.upper()`) and lower-case both sides for
case-insensitive compare.

### A2 — GitHub dispatch `204 No Content` parsed as JSON ✅
`rdgen/rdgenerator/views.py:349-352`
```python
if response.status_code == 204 or response.status_code == 200:
    github_data = response.json()      # 204 has no body → JSONDecodeError
    new_github_run.github_run_id = github_data.get('workflow_run_id')
```
GitHub's `actions/workflows/.../dispatches` returns `204 No Content`.
On 204, `.json()` raises `JSONDecodeError` and the surrounding
`except Exception` returns "Connection error" 500 — **the run row is
never written and the polling page sits forever**.

Fix: when status is 204, skip the JSON parse and leave
`github_run_id = None`; only attempt JSON when there's a body.

### A3 — `gh_run.github_run_id` may be `None` ✅
`rdgen/rdgenerator/views.py:350` (consequence of A2 or any failed dispatch)
```python
api_url = f"https://api.github.com/repos/{GHUSER}/{REPONAME}/actions/runs/{gh_run.github_run_id}"
```
If `github_run_id` is `None`, the URL becomes `/runs/None`. GitHub
returns 404, and the user sees a broken "View GitHub Action Logs" link
that also contains the literal string `None`.

Fix: guard `check_for_file` against `None`/empty `github_run_id`; treat
as "still starting" instead of polling GitHub.

### A4 — Hard-coded `X-GitHub-Api-Version: '2026-03-10'` ⚠️
`rdgen/rdgenerator/views.py:340, 498`

The placeholder header doesn't match any real GitHub API version.
GitHub falls back to the default version, so it works, but the header
is misleading. Not breaking — leave as a follow-up, the upstream value
should be `2022-11-28`.

---

## B. Critical — security / auth

### B1 — Four POST endpoints have no authentication ❌ partially mitigated
`update_github_run`, `save_custom_client`, `cleanup_secrets`, `startgh`
are all reachable by any anonymous client. The GitHub workflows send
`Authorization: Bearer ${{ env.token }}`, but Django doesn't validate
that header, so the token is decorative. Worse, the current generator
**does not put `token` into `inputs_raw`**, so `${{ env.token }}` is
empty in the runners today.

What this enables:
- DoS on `startgh` — anyone can dispatch the GitHub workflow
  repeatedly, burning the maintainer's GHBEARER quota.
- Anonymous file upload on `save_custom_client` — anyone with a UUID
  (or who guesses one) can overwrite the cached binaries.
- Anonymous status spoofing on `update_github_run` — mark any UUID
  "failed"/"success" without permission.
- Anonymous deletion on `cleanup_secrets` — wipe any UUID's secrets zip.

Fix in this PR:
- Add `token` to `inputs_raw` so the runners receive the same shared
  secret currently stored as `SH_SECRET` (the only secret already
  shipped via the encrypted zip).
- Decorate the four views with `_require_workflow_token`, which checks
  `Authorization: Bearer <SH_SECRET>` and returns 401 otherwise.
- When `SH_SECRET` is the literal placeholder `"secret"` (the default),
  log a warning and skip the check so existing dev deployments don't
  immediately 401. Production deployments **must** set `SH_SECRET`.

This keeps the existing protocol — workflows already send the header —
and only tightens enforcement on the server.

### B2 — `SECRET_KEY` default is `django-insecure-…` ✅
`rdgen/rdgen/settings.py:23` falls back to the insecure literal when
`SECRET_KEY` env var is unset. CodeQL doesn't flag this but it's the
single biggest "production booby trap" in the codebase.

Fix: when `DEBUG=False` and `SECRET_KEY` env var is missing, raise at
startup (existing default kept only for dev).

### B3 — `ZIP_PASSWORD` default is `'insecure'` ✅
`rdgen/rdgen/settings.py:28` — the AES password protecting the
secrets zip falls back to the literal string `"insecure"` when env is
unset. Same shape as B2.

Fix: same guard — production startup must fail without the env var.

### B4 — `DATA_UPLOAD_MAX_MEMORY_SIZE = None` ✅
`rdgen/rdgen/settings.py:139` — unlimited POST body size. Combined
with B1, an unauthenticated attacker can fill disk via
`save_custom_client`.

Fix: set 200 MiB (`200 * 1024 * 1024`) — enough for the real artifacts
seen in workflows (signed APK / AppImage), small enough to bound
abuse.

### B5 — `ALLOWED_HOSTS = ['*']` ⚠️
`rdgen/rdgen/settings.py:41` — wildcard host header trust enables
host header injection in `password reset emails`, generated absolute
URLs, etc. Not changed here because the existing flow uses
`request.get_host()` for building callback URLs (line 129); narrowing
host validation needs the operator to provide the real hostnames in
env. Documented for the deployment guide.

### B6 — `download`/`get_png`/`get_zip` are unauthenticated ⚠️
GET endpoints serve any file under `exe/<uuid>/`, `png/<uuid>/`,
`temp_zips/` to anyone who knows the UUID. UUIDs leak into HTML
templates (waiting/generated pages) and into GitHub Actions logs. Not
fixed in this PR — the system relies on UUID secrecy. Adding session
auth would change the public contract significantly.

---

## C. Medium — error handling / robustness

### C1 — `json.loads(request.body)` without `try` ✅
`update_github_run` and `cleanup_secrets` blow up with a 500 + stack
trace if the body isn't valid JSON.

Fix: wrap and return 400.

### C2 — `os.listdir(temp_dir)` raises if `temp_zips/` is missing ✅
`cleanup_secrets` (called from every workflow on completion) returns
500 if the directory hasn't been created yet (e.g. first run after
deploy).

Fix: `os.makedirs(temp_dir, exist_ok=True)` before `listdir`.

### C3 — `FileNotFoundError` in download endpoints ✅
`download`, `get_png`, `get_zip` raise 500 when the file doesn't exist
(common race: user clicks Download before the worker uploads).

Fix: return 404 on missing file.

### C4 — Bare `except:` clauses in `generator_view` ⚠️
`views.py:136, 146, 156` — `except:` (no exception type) hides
`KeyboardInterrupt`, `SystemExit`, etc. Annoying but not unsafe in a
WSGI worker.

Not fixed — would change behaviour subtly (real errors visible
instead of silently using "false" placeholders). Documented.

### C5 — `defaultManual/overrideManual` parser crashes on empty / no-`=` lines ✅
`views.py:229-235`
```python
for line in defaultManual.splitlines():
    k, value = line.split('=')
```
A blank line or a line without `=` raises `ValueError: not enough
values to unpack` and the whole submission 500s.

Fix: skip empty/whitespace-only lines and lines without `=`.

### C6 — `_safe_open_path` raises `PermissionError`, callers only caught `(ValueError, KeyError)` ✅
Already fixed in the previous commit on this branch.

### C7 — `GenerateToken` could return `""` on RNG failure ✅
Already fixed (panics now).

### C8 — `getPublicKey` SSRF allowlist rejected the documented CDN host ✅
Already fixed (`gosspublic.alicdn.com` + `*.aliyuncs.com`, https-only).

---

## D. Out of scope / not bugs

- `tools.go` MD5 helper — kept for the bcrypt-migration path in
  `VerifyPassword`. CodeQL suppression added per-file.
- Rust `hard-coded-cryptographic-value` (37 alerts) — all in
  `#[cfg(test)]` blocks. Dismissed as "used in tests".
- CodeQL workflow — per the user, GitHub's own feature, not project
  code. Left alone after the v3→v4 + Go path fix.

---

## E. Summary

| Severity | Count | Fixed | Flagged |
|----------|-------|-------|---------|
| Critical (workflow) | 4 | 3 | 1 |
| Critical (security) | 6 | 4 | 2 |
| Medium | 7 (incl. carry-over) | 6 | 1 |

After this PR the custom-agent build pipeline regains: the working
"don't override app-name with the default" branch (A1), correct
handling of GitHub's 204 dispatch response (A2), graceful behaviour
when the dispatch fails (A3), Bearer-token auth on the four
runner-callable endpoints (B1), and graceful 400/404s in place of
stack-trace 500s (C1/C2/C3/C5).
