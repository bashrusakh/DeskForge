# github-build - active `rustqs.exe` build path via GitHub Actions (PLAN.md §8.8)

The main selected way to build the Windows Flutter client is **GitHub Actions in a
fork of rustdesk**, following the rdgen model. `full_Server` triggers the build, the fork
builds on a free `windows-2022` runner, and sends the binary back to your server.
`win-builder/` standalone is frozen as the fallback path.

> **Good news:** the rdgen workflow `generator-windows.yml` already contains all the heavy
> logic: encrypted secrets (`fetch-encrypted-secrets.yml`), `ZIP_PASSWORD`, binary upload back
> to the server (`save_custom_client`), and the 3 config injection layers. The main work in §8.8
> is to **fork it, repoint external URLs to your own fork, and configure secrets.**
> Do not rewrite it from scratch.

---

## Architecture (repeat of §3)

```
admin-ui -> Go API (full_Server) -> workflow_dispatch (ENCRYPTED inputs) ->
  GitHub Actions [rustdesk fork, windows-2022] ->
    build (config.rs server/key + custom.txt + branding) ->
    rustqs.exe -> POST /api/save_custom_client -> your server -> admin-ui Download
```

The binary is **not published** as a public release. It is sent to your server.
That is why a public fork remains safe (see §4 below).

---

## §8.8.1 - Fork (owner action)

```bash
gh repo fork rustdesk/rustdesk   --org YOUR_ORG --fork-name rustdesk   --clone=false
gh repo fork rustdesk/hbb_common --org YOUR_ORG --fork-name hbb_common --clone=false
```

Repoint the submodule in the rustdesk fork (`.gitmodules`: `rustdesk/hbb_common` ->
`YOUR_ORG/hbb_common`) and commit it.

Copy the rdgen workflow recipe (`rdgen/.github/workflows/*` + `.github/patches/*`)
into the rustdesk fork, or keep it in a separate build repo, though the fork is simpler.

---

## §8.8.2 + §8.8.3 - Sovereign workflow repoint (URLs)

Upload artifacts from the offline kit into **fork releases** (FORK-PROCEDURE §B2), then
replace external URLs in `generator-windows.yml` with your own. Exact lines for 1.4.7:

| Line | Current (external) | Replace with (fork) |
|---|---|---|
| 261, 264, 383, 395 | `raw.githubusercontent.com/bryangerlach/rdgen/.../patches/*` | vendored `rdgen/.github/patches/*` (already in the repo) or `raw` URLs from your fork |
| 283 | `github.com/rustdesk/engine/releases/.../windows-x64-release.zip` | `github.com/YOUR_ORG/rustdesk/releases/download/offline-assets-1.4.7/windows-x64-release.zip` |
| 433 | `github.com/rustdesk-org/rdev/releases/.../usbmmidd_v2.zip` | fork release above |
| 441-443 | `github.com/rustdesk/hbb_common/releases/driver/*` | fork release above |

> Patches (`allowCustom.py` etc.) are **already vendored** in `rdgen/.github/patches/`.
> Do not fetch them from `bryangerlach` over the network; copy them from the repo on the runner
> or from `raw` URLs in your own fork.

Building from your own fork is guaranteed because the workflow `checkout`s the fork itself,
not upstream, and Cargo reads dependencies from `vendor/` (committed or shipped in a release,
see FORK-PROCEDURE §A2).

---

## §8.8.4 - Security (REQUIRED on a public fork)

What already exists in the rdgen workflow and should be reused:
- **Encrypted inputs** - `fetch-encrypted-secrets.yml` (line 46) + `ZIP_PASSWORD`
  (line 96, secret): config (`server`/`key`/**password**) is passed as an encrypted blob,
  decrypted inside the run with a GitHub Secret, so the password never appears in public logs.
- **Binary goes to the server**, not to releases: `curl ... ${apiServer}/api/save_custom_client`
  (line 626). Nobody can download the binary from GitHub.

Configure **GitHub Secrets** in the fork: `GENURL` (your server URL), `ZIP_PASSWORD`,
the auth token for `save_custom_client`, and optionally `SIGN_BASE_URL` / `SIGN_API_KEY`
for signing.

> Reminder: embedded `RS_PUB_KEY` is a public key, not a secret. The secret is the permanent
> quick-support password. It is inside the binary, so anyone who has the binary will have it too,
> which is expected for support targets. GitHub logs are unrelated as long as inputs are encrypted.

---

## §8.8.5 - Go API integration

In `api/service/custom_build.go`, for `platform=windows`, instead of writing a queue file,
use a "GitHub backend" branch like rdgen `views.py`:

1. Build the config (`server`/`key`/`custom.txt`/branding) -> encrypt it (ZIP under `ZIP_PASSWORD`).
2. `POST https://api.github.com/repos/YOUR_ORG/rustdesk/actions/workflows/generator-windows.yml/dispatches`
   with `ref` + `inputs` (encrypted blob, `app_name`, `uuid`) and
   `Authorization: Bearer <PAT>`.
3. Poll run status (`GET .../actions/runs`) -> update job status in the DB.
4. Accept the binary at `/api/save_custom_client` (the endpoint already exists in the rdgen API model) ->
   place it in `output/{uuid}` -> admin-ui shows Download.

**PAT token** must only come from env/secret (`.env`, not code, not git). Minimum scope:
`actions:write` for the specific fork repository.

---

## What does NOT change

- The 3 injection layers (`config.rs` / `custom.txt` / branding) stay the same as in standalone (§5 of PLAN).
- The Custom Client form in admin-ui stays the same; only the backend job path changes (GitHub vs file queue).
- `offline-kit` becomes the source of fork releases and remains the fallback for standalone.

## Subtask status

- [ ] 8.8.1 fork `rustdesk` + `hbb_common` (owner)
- [ ] 8.8.2 upload the kit into fork releases (owner, commands in FORK-PROCEDURE §B2)
- [ ] 8.8.3 repoint URLs in `generator-windows.yml` (table above)
- [ ] 8.8.4 configure GitHub Secrets (mechanism already exists in the workflow)
- [ ] 8.8.5 Go API: `workflow_dispatch` backend (code after the fork is available for testing)

---

## Where workflow files live (map for agents)

Workflow files exist in **three places**. This is not duplication; each has a different role.
Before editing any `.yml`, determine which layer it belongs to.

| Layer | Path | What it is | Where it runs |
|---|---|---|---|
| 1. Upstream reference | `rdgen/.github/workflows/*.yml` | Vendored copy from `bryangerlach/rdgen`. Source of truth for workflow logic (3 injection layers, encrypted secrets, `save_custom_client`). **Does not run** in this repo. | - read-only |
| 2. Local staging copy | `github-build/windows-min-test.yml` | Snapshot of the active workflow for code review and history. **Does not run** on GitHub. | - diff/reference only |
| 3. Active workflow (test) | `bashrusakh/rustdesk:rustqs/min-test/.github/workflows/rustqs-windows-min-test.yml` | Smoke test for the current §8.8 phase: minimal workflow that validates the 3 injection layers + encryption. Triggered via `workflow_dispatch`. Also includes `bridge.yml` and `third-party-RustDeskTempTopMostWindow.yml`. **Temporary** - will be replaced by layer 4. | GitHub Actions, rustdesk fork |
| 4. Target workflow (prod) | `bashrusakh/rustdesk:rustqs/<prod-branch>/.github/workflows/generator-windows.yml` (planned) | Full rdgen workflow: msi, signing, `save_custom_client`, all artifacts. We switch from min-test to this file once injection and Go API are stable. | GitHub Actions, rustdesk fork |

### Synchronization rules

1. **Changed build logic?** Change it in the fork first (layer 3, branch `rustqs/min-test`).
   Update local `github-build/windows-min-test.yml` (layer 2) afterwards. It is a snapshot,
   not the source. Commit locally only after the fork is green.
2. **Upstream rdgen shipped an update?** Layer 1 (`rdgen/.github/workflows/*`) is updated as a
   clean vendor pull (`git subtree` or manual cherry-pick). After that, make a separate decision
   whether to bring those changes into layer 3 (the fork). Do not do this automatically:
   the fork workflow is heavily simplified (Windows only, 3 injection layers via env vars,
   no external rdgen endpoints).
3. **Bumping action versions (`@v4 -> @v7`, `setup-msbuild@v2 -> @v3`, etc.)** Apply them
   independently in each layer. They are local solutions, not auto-synced. In the fork
   (layer 3), keep SHA-pinned actions (`@<sha> # vN`) for public-repo security. In rdgen-vendor
   (layer 1), keep whatever upstream ships.

### What NOT to do

- Do not edit `rdgen/.github/workflows/*` by hand (see rule §1 above).
- Do not push local `github-build/windows-min-test.yml` into the fork via `gh api PUT`.
  The files diverge structurally (job names, env vars); it is not a drop-in copy.
- Do not confuse `windows-min-test.yml` (layer 2, original rdgen-style reference) with
  `rustqs-windows-min-test.yml` (layer 3, actually executed in the fork).
- Do not keep expanding `rustqs-windows-min-test.yml` (layer 3) until it becomes prod.
  Once min-test has done its job (3 injection layers + encryption confirmed), the move to prod
  should be a separate workflow file based on `generator-windows.yml` (layer 4), not a bloated
  min-test. Min-test should remain a lightweight future smoke test.

### How to update a file in the fork (layer 3) without `git clone`

```powershell
# 1. get sha and base64 content
gh api repos/bashrusakh/rustdesk/contents/.github/workflows/<file>.yml?ref=rustqs/min-test --jq "{sha, content}" > meta.json

# 2. decode, edit locally
#    (decode base64 -> edit -> encode base64)

# 3. PUT it back - payload must be WITHOUT BOM
#    (important: Set-Content in PS 5.1 adds BOM, JSON will not parse).
#    Use [System.IO.File]::WriteAllText with UTF8Encoding($false).
gh api -X PUT repos/bashrusakh/rustdesk/contents/.github/workflows/<file>.yml --input payload.json
```

### Fork bump log

- **2026-06-13** - `microsoft/setup-msbuild` v2 -> v3 (SHA `30375c6...`) in
  `third-party-RustDeskTempTopMostWindow.yml`. Parallel bump in rdgen-vendor:
  [DeskForge#904e9fa](https://github.com/bashrusakh/DeskForge/commit/904e9faefe091af03da5c40bfc753358e653be69).
  The `upload-artifact` bumps from the same commit were **not** carried over, because the fork
  already used a newer SHA-pinned v7, while rdgen-vendor only moved to v4.
