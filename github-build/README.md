# github-build — active `rustqs.exe` build path via GitHub Actions

Primary way to build the Windows client — GitHub Actions in the `bashrusakh/rustdesk` fork.
`win-builder/` is the frozen fallback.

---

## Architecture

```
admin-ui → Go API → workflow_dispatch (encrypted payload) →
  GitHub Actions [rustdesk fork, windows-2022] →
    L1 config.rs (server+key) → L2 custom_.txt (permanent password) → L3 branding →
    rustqs.exe → POST /api/save_custom_client → your server → admin-ui Download
```

Binary is NOT published to public releases — only to your server.
Credentials — encrypted payload, decrypted inside the runner via GitHub Secret.

---

## Workflow layers

| Layer | Path (in rustdesk fork)                                        | Role                                          |
| ----- | --------------------------------------------------------------- | --------------------------------------------- |
| 1     | `rustqs/min-test/.github/workflows/rustqs-windows-min-test.yml` | ✅ active smoke-test                          |
| 2     | `rustqs/min-test/.github/workflows/bridge.yml`                  | reusable workflow (from upstream 1.4.7)       |
| 3     | `rustqs/min-test/.github/workflows/third-party-RustDeskTempTopMostWindow.yml` | TopMost build   |
| 4     | `DeskForge/github-build/windows-min-test.yml`                   | local copy for code review                    |
| 5     | `DeskForge/rdgen/.github/workflows/*.yml`                       | vendored upstream rdgen reference             |

**Rule:** change build logic → edit in fork (layer 1), then update local copy (layer 4).

---

## Security (REQUIRED for a public fork)

- `enc_payload` — AES-256-CBC + PBKDF2, key `WORKFLOW_PAYLOAD_KEY` in GitHub Secrets.
- `GENURL` — your server URL (where to send the binary).
- `ZIP_PASSWORD` — password to encrypt config inside the workflow.
- `RS_PUB_KEY` is a public key, not a secret.
- `SetWorkflowSecret` — button in admin UI (`Push to GitHub Secrets`) via `nacl/box.SealAnonymous`.

---

## When a new upstream version ships

1. **Fork sync** → `git fetch upstream --tags && git push origin v1.5.0`
2. **Repoint submodule** → `.gitmodules` → `bashrusakh/hbb_common`
3. **Vendor** → `cargo vendor && git add vendor`
4. **Update workflow:** diff upstream `build-for-windows-flutter` with `rustqs-windows-min-test.yml`
5. **Test** → `gh workflow run rustqs-windows-min-test.yml --ref v1.5.0`

Detailed: [PLAN.md §7](../PLAN.md#7-workflow-new-upstream-rustdesk-client-release).

---

## Sync rules

1. **Build logic changes go to the fork first** (layer 1), then copy to `github-build/` (layer 4).
2. `rdgen/.github/workflows/*` (layer 5) — vendored reference, **do not edit by hand**.
3. Action version bumps (`@v4 → @v7`) — independently per layer. In fork — SHA-pinned.
4. When min-test is stable → switch to full `generator-windows.yml` (msi, signing).

### Fork bump log

- 2026-06-13: `setup-msbuild` v2→v3, `upload-artifact` → SHA-pinned v7.
