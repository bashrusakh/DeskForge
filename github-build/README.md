# github-build — active build path via GitHub Actions

All platforms (Windows, Linux, Android) are built through GitHub Actions in the
`bashrusakh/rustdesk` fork. DeskForge workflow files are deployed to three
branches:

| Branch                | Purpose                                                          |
| --------------------- | ---------------------------------------------------------------- |
| `master`                | Default branch — API discovery (workflow must exist here)        |
| `rustqs/min-test`       | Execution — all dispatches go here                               |
| `rustqs/master-workflows` | Mirror — backup copy of workflow files, kept in sync with `master` |

Each local workflow file in this directory has the same filename as its target
in the fork's `.github/workflows/` (identical names, no rename needed).

| Platform | File                                    | Target in fork                                    | Status          |
| -------- | --------------------------------------- | ------------------------------------------------- | --------------- |
| Windows  | `github-build/rustqs-windows-min-test.yml` | `.github/workflows/rustqs-windows-min-test.yml`   | ✅ active       |
| Linux    | `github-build/rustqs-linux.yml`            | `.github/workflows/rustqs-linux.yml`              | ✅ active       |
| Android  | `github-build/rustqs-android.yml`          | `.github/workflows/rustqs-android.yml`            | ✅ active       |

---

## Architecture

All dispatches go to the `rustqs/min-test` branch of the fork (the Go code
forces this branch in `tryGithubDispatch` regardless of per-install config).

```text
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
| 1b    | `rustqs/min-test/.github/workflows/rustqs-linux.yml`            | ✅ Linux x64 (active)                        |
| 1c    | `rustqs/min-test/.github/workflows/rustqs-android.yml`          | ✅ Android arm64 (active)                      |
| 2     | `rustqs/min-test/.github/workflows/bridge.yml`                  | reusable workflow (from upstream 1.4.7)       |
| 3     | `rustqs/min-test/.github/workflows/third-party-RustDeskTempTopMostWindow.yml` | TopMost build   |
| 4     | `DeskForge/github-build/`                                       | local copies for code review                  |
| 5     | `DeskForge/rdgen/.github/workflows/*.yml`                       | vendored upstream rdgen reference             |

**Rule:** change build logic → edit in fork (layer 1), then update local copy (layer 4).

---

## Workflow deployment — pushing to the fork

Workflow files are deployed to all three branches of `bashrusakh/rustdesk`:

- `master` — API discovery (must exist on default branch)
- `rustqs/min-test` — execution (all dispatches go here)
- `rustqs/master-workflows` — mirror of `master`

> **NOTE:** `rustqs-linux.yml` and `rustqs-android.yml` will not be found by
> the workflow_dispatch API unless they exist on the default branch (`master`).

```bash
cd /path/to/rustdesk-fork
# 1) Push to master first (API discovery)
git checkout master
cp /path/to/DeskForge/github-build/rustqs-*.yml .github/workflows/
cp /path/to/DeskForge/rdgen/.github/workflows/bridge.yml .github/workflows/
git add .github/workflows/
git commit -m "feat: update rustqs-* workflows"
git push origin master

# 2) Then push to rustqs/master-workflows (mirror)
git checkout rustqs/master-workflows
cp /path/to/DeskForge/github-build/rustqs-*.yml .github/workflows/
cp /path/to/DeskForge/rdgen/.github/workflows/bridge.yml .github/workflows/
git add .github/workflows/
git commit -m "feat: update rustqs-* workflows"
git push origin rustqs/master-workflows

# 3) Then push to rustqs/min-test (execution)
git checkout rustqs/min-test
cp /path/to/DeskForge/github-build/rustqs-*.yml .github/workflows/
cp /path/to/DeskForge/rdgen/.github/workflows/bridge.yml .github/workflows/
git add .github/workflows/
git commit -m "feat: update rustqs-* workflows"
git push origin rustqs/min-test
```

If a workflow file is missing from the fork, dispatch immediately fails with HTTP 404.
`bridge.yml` is required by all three `rustqs-*.yml` files — without it the workflow
run fails with a parse error (422).

> DeskForge workflow files live on all three branches: `master`, `rustqs/master-workflows`, and `rustqs/min-test`.

---

## Security (REQUIRED for a public fork)

- `enc_payload` — AES-256-CBC + PBKDF2, key `WORKFLOW_PAYLOAD_KEY` in GitHub Secrets.
- `GENURL` — your server URL (where to send the binary).
- `ZIP_PASSWORD` — password to encrypt config inside the workflow.
- `RS_PUB_KEY` is a public key, not a secret.
- `SetWorkflowSecret` — button in admin UI (`Push to GitHub Secrets`) via `nacl/box.SealAnonymous`.

---

## When a new upstream version ships

Workflow files live on `rustqs/min-test` (execution), `master` (API discovery),
and `rustqs/master-workflows` (mirror). `master` is synced with upstream
and stays clean — no DeskForge-specific files on it.

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
