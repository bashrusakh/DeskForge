# NEXT-STEPS - bring up Docker and verify the GitHub pipeline through the UI

> Status as of 2026-06-12: §8.8.5 is wired together in code, but not compilation-tested.
> Local Docker has been cleaned (volumes + images). Standalone win-build is frozen.

## 0. Prepare in advance

- [ ] **GitHub PAT** (fine-grained) for `bashrusakh/rustdesk`. Scope:
  - `Actions: Read and write`
  - `Secrets: Read and write` (required for `Push to GitHub Secrets`)
  - `Metadata: Read-only` (granted automatically)
- [ ] Copy the existing key from the local file if you do not want to change the fork secret:
  - path: `offline-kit/artifacts/workflow-payload.key` (43-char base64). Otherwise in step 5 you can click `Generate` and create a new one.
- [ ] Do **not** start the old Windows agent. Leave `build-win` out of `docker-compose.yml`
  (or run `docker compose up server linux-build`).

## 1. Bring up Docker

```powershell
cd E:\_projects\full_Server\docker
docker compose build server     # rebuild because of new Go code (model + service + controller)
docker compose up -d server     # no build-win
docker compose logs -f server   # watch startup
```

- [ ] Logs should show `Migrating....268` (`DatabaseVersion = 268`). If not, migration failed.
- [ ] If Go compilation fails, read the error, fix it, repeat. Most likely causes:
  - import typo (`gorm.io/gorm`, `golang.org/x/crypto/nacl/box`, `golang.org/x/crypto/pbkdf2`)
  - missed registration in `service/service.go` Service struct
  - `archive/zip` / `bytes` / `time` in `custom_build.go`
- [ ] Health check: `docker compose ps` -> `server` should be `Up (healthy)`.

## 2. Open admin-ui

- [ ] Browser: `http://localhost:21114/admin/`
- [ ] Log in as admin
- [ ] Left nav: **Server -> GitHub Build** (new item, Connection icon)

## 3. Fill the form

- [ ] Repository: `bashrusakh/rustdesk`
- [ ] Workflow filename: `rustqs-windows-min-test.yml`
- [ ] Branch: `rustqs/min-test`
- [ ] GitHub Token: paste the PAT from step 0
- [ ] Encryption key: leave empty (generate below) OR paste the old value from the file
- [ ] **Save**

## 4. Test connection

- [ ] Click **Test connection**
- [ ] Expected: green `ok` message
- [ ] If `HTTP 401/403`: PAT is wrong or lacks permissions
- [ ] If `HTTP 404`: repository typo

## 5. Encryption key

Option A (new key, recommended):
- [ ] **Generate new key** -> a base64 value appears in the field
- [ ] **Push to GitHub Secrets** -> expect `WORKFLOW_PAYLOAD_KEY synced`
- [ ] Verify at `github.com/bashrusakh/rustdesk/settings/secrets/actions` that
  `WORKFLOW_PAYLOAD_KEY` was updated "less than a minute ago"

Option B (reuse the key from `offline-kit/artifacts/workflow-payload.key`, already present in the fork):
- [ ] Paste the value into the Encryption key field -> Save
- [ ] `Push to Secrets` can be skipped

## 6. Trigger test build (sanity check for `workflow_dispatch`)

- [ ] Click **Trigger test build**
- [ ] Expected: `Run started: id=...` with a GitHub link
- [ ] Open the link and confirm a fresh `rustqs windows min test` run exists
- [ ] Do not wait for completion here, it takes 25-30 minutes

## 7. Full end-to-end through Custom Client

- [ ] Nav: **Custom Client -> New Build**
- [ ] Platform: **Windows**
- [ ] App name: `rustqs`
- [ ] Custom JSON (important: strict format). At this stage Go expects `server`, `key`, and
  `custom_txt` at the root of `CustomJson`, for example:
  ```json
  {"server":"your.server:21116","key":"your_RS_PUB_KEY_base64","custom_txt":"eyJwYXNzd29yZCI6InRlc3QifQ=="}
  ```
  (`custom_txt` is base64 of `{"password":"..."}`)
- [ ] Create -> a row should appear with status `building`
- [ ] In Go logs: `github run id: <number>` means `workflow_dispatch` started
- [ ] Wait ~30 minutes (server-side polling every 30 seconds)
- [ ] Status should become `done`, with Download available

## 8. Verify the artifact

- [ ] On the server (inside the container): `ls /rdgen-data/output/<id>/`
- [ ] Expected files: `rustqs.exe`, `custom_.txt`, several `.dll` files
- [ ] Download through the UI -> run on Windows -> it should connect to the baked-in server
  using the baked-in password

## If something fails

| Symptom | Where to look | Likely cause |
|---|---|---|
| `Migrating....` missing or failing | `docker compose logs server` | Go code did not compile; inspect `docker compose build` |
| Test connection -> `401/403` | UI alert | PAT invalid or missing permissions |
| Test connection -> `404` | UI alert | typo in `repo` |
| Push to Secrets -> `403` | UI alert | PAT lacks `Secrets: write` |
| Build stuck in `building` >40 min | `docker compose logs server`; GitHub Actions UI | poller crashed or run is stuck |
| Build -> `failed` immediately | `b.BuildLog` in Custom Client entry | dispatch error, often invalid `enc_payload` |
| Build -> `failed` after the run | `BuildLog` | exe not found in zip, or 90-minute timeout |

## After success

- [ ] Commit the local code into your `DeskForge` repo (`git init` if needed).
- [ ] Mark `[~] 8.8.5` as `[x]` in `PLAN.md`.
- [ ] Decide whether to keep the `[Debug]` plain inputs in the workflow for troubleshooting,
  or remove them and rely on `enc_payload` only.
