# github-build — active build path via GitHub Actions

All platforms (Windows, Linux, Android) are built through GitHub Actions in the
`bashrusakh/rustdesk` fork. All DeskForge-specific workflow files live on the
`rustqs/min-test` branch — `master` is kept clean for upstream tracking.

Each local workflow file in this directory has the same filename as its target
in the fork's `.github/workflows/` (identical names, no rename needed).

| Platform | File                                    | Target in fork                                    | Status          |
| -------- | --------------------------------------- | ------------------------------------------------- | --------------- |
| Windows  | `github-build/rustqs-windows-min-test.yml` | `.github/workflows/rustqs-windows-min-test.yml`   | ✅ active       |
| Linux    | `github-build/rustqs-linux.yml`            | `.github/workflows/rustqs-linux.yml`              | 🟡 draft (B-012) |
| Android  | `github-build/rustqs-android.yml`          | `.github/workflows/rustqs-android.yml`            | 🟡 draft (B-012) |

---

## Architecture

All dispatches go to the `rustqs/min-test` branch of the fork (the Go code
forces this branch in `tryGithubDispatch` regardless of per-install config).

```
admin-ui → Go API → workflow_dispatch (encrypted payload, ref=rustqs/min-test) →
  GitHub Actions [rustdesk fork, rustqs/min-test branch] →
    L1 config.rs (server+key) → L2 custom_.txt (permanent password) → L3 branding →
    artifact → POST /api/save_custom_client → your server → admin-ui Download
```

Binary is NOT published to public releases — only to your server.
Credentials — encrypted payload, decrypted inside the runner via GitHub Secret.

---

## Workflow layers

| Layer | Path (in rustdesk fork)                                        | Role                                          |
| ----- | --------------------------------------------------------------- | --------------------------------------------- |
| 1a    | `rustqs/min-test/.github/workflows/rustqs-windows-min-test.yml` | ✅ Windows x64 (active)                       |
| 1b    | `rustqs/min-test/.github/workflows/rustqs-linux.yml`            | 🟡 Linux x64 (draft, B-012)                   |
| 1c    | `rustqs/min-test/.github/workflows/rustqs-android.yml`          | 🟡 Android arm64 (draft, B-012)                |
| 2     | `rustqs/min-test/.github/workflows/bridge.yml`                  | reusable workflow (from upstream 1.4.7)       |
| 3     | `rustqs/min-test/.github/workflows/third-party-RustDeskTempTopMostWindow.yml` | TopMost build   |
| 4     | `DeskForge/github-build/`                                       | local copies for code review                  |
| 5     | `DeskForge/rdgen/.github/workflows/*.yml`                       | vendored upstream rdgen reference             |

**Rule:** change build logic → edit in fork (layer 1), then update local copy (layer 4).

---

## Current version state

`rustqs/min-test` is currently based on **v1.4.7** (20 custom commits on top).
The admin UI version selector (up to v1.4.8) is **metadata only** — the selected
version is stored on the build record but is NOT passed to the `workflow_dispatch`
payload (`tryGithubDispatch` sends only `{server, key, app_name, custom_txt}`).

The build always checks out the `rustqs/min-test` branch via `actions/checkout@v4`
(no `ref:` override), so the actual client version produced depends entirely on
what source code is on that branch — currently v1.4.7.

To build v1.4.8 (or newer), the branch needs to be rebased on the new tag:

```bash
cd /path/to/rustdesk-fork
git checkout rustqs/min-test
git rebase v1.4.8
git push --force-with-lease origin rustqs/min-test
```

See [PLAN.md §7](../../PLAN.md#7-workflow-new-upstream-rustdesk-client-release) for the full upstream version bump procedure.

---

## Initial deployment — pushing workflow files to the fork

Before Linux or Android builds can be dispatched, their workflow files must exist in the
`bashrusakh/rustdesk` fork on the `rustqs/min-test` branch at `.github/workflows/`.
Filenames are identical — no rename needed.

```bash
cd /path/to/rustdesk-fork
git checkout rustqs/min-test

cp /path/to/DeskForge/github-build/rustqs-linux.yml   .github/workflows/
cp /path/to/DeskForge/github-build/rustqs-android.yml .github/workflows/

git add .github/workflows/rustqs-linux.yml .github/workflows/rustqs-android.yml
git commit -m "feat: add Linux + Android build workflows (B-012)"
git push origin rustqs/min-test
```

After push, the Go API can dispatch to these workflows. If the files are missing,
the build immediately fails with HTTP 404.

> `master` in the fork is kept clean for upstream `rustdesk/rustdesk` tracking.
> All DeskForge-specific workflows go to `rustqs/min-test` only.

---

## Security (REQUIRED for a public fork)

- `enc_payload` — AES-256-CBC + PBKDF2, key `WORKFLOW_PAYLOAD_KEY` in GitHub Secrets.
- `GENURL` — your server URL (where to send the binary).
- `ZIP_PASSWORD` — password to encrypt config inside the workflow.
- `RS_PUB_KEY` is a public key, not a secret.
- `SetWorkflowSecret` — button in admin UI (`Push to GitHub Secrets`) via `nacl/box.SealAnonymous`.

---

## When a new upstream version ships

Workflow files live on the `rustqs/min-test` branch. `master` is synced with
upstream and stays clean — no DeskForge-specific files on it.

1. **Fork sync** → `git fetch upstream --tags && git push origin v1.5.0`
2. **Repoint submodule** → `.gitmodules` → `bashrusakh/hbb_common`
3. **Vendor** → `cargo vendor && git add vendor`
4. **Update `rustqs/min-test` branch** → `git checkout rustqs/min-test && git rebase v1.5.0`
5. **Update workflows:** diff upstream with `rustqs-*.yml`
6. **Test** → `gh workflow run rustqs-windows-min-test.yml --ref rustqs/min-test`

Detailed: [PLAN.md §7](../PLAN.md#7-workflow-new-upstream-rustdesk-client-release).

---

## Sync rules

1. **Build logic changes go to the fork first** (layer 1), then copy to `github-build/` (layer 4).
2. `rdgen/.github/workflows/*` (layer 5) — vendored reference, **do not edit by hand**.
3. Action version bumps (`@v4 → @v7`) — independently per layer. In fork — SHA-pinned.
4. When min-test is stable → switch to full `generator-windows.yml` (msi, signing).

### Fork bump log

- 2026-06-13: `setup-msbuild` v2→v3, `upload-artifact` → SHA-pinned v7.
