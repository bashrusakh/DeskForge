# PLAN.md — DeskForge: Single Source of Truth

> Last updated: 2026-08-11
> Related: [CHANGELOG.md](CHANGELOG.md) · [BUGS.md](BUGS.md) · [CONTRIBUTING.md](CONTRIBUTING.md)

> **Evidence labels (2026-08-10):** The published RustDesk client source/ref is
> 1.4.8 and the published DeskForge API schema is `DatabaseVersion` 272. The local
> uncommitted corrective worktree targets API schema 282; that local schema target is
> not a public current-schema or release claim.
> The current API path is provider dispatch → exact run/artifact retrieval → local
> validation and publication. Runner callbacks, local file-queue fallback, and
> sole-artifact selection are not current production behavior. Windows is the only
> production capability admitted by the API gate; Linux/Android remain gated pending
> PR11 evidence. The dispatch contract under test requires
> `return_run_details=true` and an exact HTTP 200 run-details response containing
> the run identity; standard 204 is intentionally unsupported because it cannot
> correlate the exact run. No normal GitHub operation is verified. The Go API owns
> port 21114; Rust services use 21115–21119, with relay WebSocket on 21119; current
> production Compose exposure remains
> 21114–21118. No live provider run or clean-environment build proof is claimed.
> Workflow approval requires a provider-derived verified annotated tag, the
> aggregate of all applicable active protected-tag rulesets with effective update
> and deletion protections, no bypass actors, and rejection of tag/branch label
> collisions. The local gate is not
> live-provider evidence. Its pending → building identity write is atomic, but
> provider dispatch and that database write are not an end-to-end atomic
> transaction: there is no durable outbox or distributed lease for the
> post-dispatch database-failure window.

---

## 0. Project goal

Self-hosted RustDesk server (hbbs/hbbr + API + admin panel) + **custom client builder**
that is intended to reduce dependence on `rustdesk/rustdesk`, `rustdesk-org/*`, and
`rustdesk.com`; current source manifests still reference active third-party upstream
repositories, so full upstream independence is not verified.

**Active client build path:** the API dispatches an owned workflow in the
configured RustDesk fork, polls the provider run, downloads the exact artifact
through the provider API, then validates, extracts, and publishes it locally.
`github-build/` and `rdgen/` are reference/documentation material only;
`win-builder/` and `linux-build` are manual/historical-only builders outside the
production API path.

GitHub-first because:
- free Windows runners
- fork workflow is configured; the min-test result below is historical
- standalone requires a separate Windows Server, not deployed

---

## 1. Repository map

```
bashrusakh/
├── DeskForge              ← this repo (server, api, admin, docker)
├── rustdesk               ← owned RustDesk fork and active workflow source
│   └── .github/workflows/ ← rustqs-windows.yml, rustqs-linux.yml, rustqs-android.yml
└── libs/hbb_common        ← tracked shared source in DeskForge; not a submodule here
```

**Published client source/ref:** 1.4.8, aligned with `offline-kit/versions.env`. The
local uncommitted corrective worktree targets API schema 282; the published DeskForge
schema remains `DatabaseVersion` 272.
The configured RustDesk fork's `libs/hbb_common` is a separate git submodule currently
recorded against upstream `rustdesk/hbb_common`; its local checkout is dirty and
unpublished. Active RustDesk and DeskForge manifests also reference third-party upstream
repositories/modules. Provider-side live execution and clean-build proof are not recorded
here. On 2026-08-11, `GOWORK=off go vet ./...` and `GOWORK=off go test ./...`
passed after test-only cache diagnostics and opt-in Redis test changes. Redis
integration tests and benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is
configured; no live Redis run is recorded. MySQL/PostgreSQL, live provider, and
the other release/build gates remain unverified. DeskForge does not currently track a repository `vendor/` tree, client-release
directory, or `rustdesk-deps/` archive.

The combined DeskForge distribution is identified as AGPL-3.0 for the covered work;
separate or independent API/UI/reference components retain their applicable upstream
licenses and notices. The license inventory is incomplete; no signatures or attestations
are recorded, and no full sovereignty claim is made. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

---

## 2. Architecture

### Current path (GitHub Actions)

```
admin-ui (Custom Client form)
   ↓ custom-build request
Go API (DeskForge)
   ↓ workflow_dispatch + authenticated DFP1 enc_payload (AES-256-CBC + PBKDF2 + HMAC)
GitHub Actions [configured RustDesk fork, owned platform workflow]
   ↓ L1: config.rs (server + key)
   ↓ L2: custom_.txt (permanent password, allowCustom patch)
   ↓ L3: branding (rustqs, portable-packer)
provider run ← Go API polls provider status
provider artifact API → Go API downloads the exact artifact
Go API validates/extracts/publishes locally → admin-ui Download
```

**Security:** password never published — `enc_payload`, decrypted inside runner via
GitHub Secret `WORKFLOW_PAYLOAD_KEY`. The runner does not callback to the API;
the API retrieves the artifact through the provider API and publishes it locally,
not to a public release.

### Workflow approval and schema evidence

Before a provider-backed build can run, the API accepts only a provider-derived
verified annotated tag. The provider must report an accepted verification reason,
the aggregate of all applicable active protected tag rulesets for that label with
effective update and deletion protections, and no bypass actors. A tag/branch
collision with the same label is
rejected. The API rechecks the provider policy and exact workflow contents at the
resolved immutable commit before dispatch; raw refs, SHAs, credentials, and
workflow internals are not normal UI inputs.

The published DeskForge schema is `DatabaseVersion` **272**. This uncommitted
corrective worktree targets local schema **282**, which is not published or live-
provider evidence. The additive fields are **280**
(`workflow_ref_approved`, approval status), **281**
(`workflow_ref_provider_verified`, provider-policy status), and **282**
(`workflow_ref_approval_sha`, the provider-resolved approval commit). Legacy
metadata-only configuration saves remain compatible with plaintext secret rows:
they can preserve legacy values without `SECRET_ENCRYPTION_KEY`; new or replaced
non-empty secret writes require the key, and a resave with the key encrypts legacy
plaintext values.

Focused fake-transport and SQLite checks do not prove live GitHub/provider
approval, dispatch, polling, or artifact delivery. Dispatch requires the exact
HTTP 200 run-details contract (`return_run_details=true`), not a standard 204 or
latest-run inference. The local identity write is atomic, but no durable outbox
or distributed lease makes provider dispatch plus the database write one atomic
operation; a post-dispatch database failure remains a documented limitation.

### Historical/frozen builders (not used by the current API)

```
admin-ui → Go API → jobs/{id}.json → SMB share → standalone Windows builder
                                                   or Docker linux-build
```

These local-queue builders are not a production fallback for the current API.
They remain only as historical/frozen material and must not be treated as an
alternative workflow source.

---

## 3. Component status

| Component                      | Status         | Notes                                |
| ------------------------------ | -------------- | ------------------------------------ |
| hbbs/hbbr (Rust)               | ✅ running     | ports 21115-21119; relay WebSocket is 21119 |
| Go API                         | ✅ running     | users, address book, OAuth, LDAP, audit |
| Admin UI (Vue 3)               | ✅ running     | 16 pages, 3 locales, DataTable, FilterBar |
| GitHub build (Windows)         | 🟡 implemented | API path is provider-backed; no current live provider/clean-build proof |
| GitHub build (Linux)           | 🟡 gated       | workflow mapping exists; production capability remains disabled    |
| GitHub build (Android)         | 🟡 gated       | workflow mapping exists; production capability remains disabled    |
| `github-build/`                | 📘 reference   | documentation only; no executable workflow copies                |
| `rdgen/`                       | 📘 reference   | vendored historical/reference material, not the active source      |
| win-builder standalone         | ❄️ frozen      | manual/historical only; not API path |
| linux-build (Docker)           | ❄️ frozen      | manual/historical only; not API path |
| offline-kit                    | ❄️ frozen      | re-freeze when client version changes |

---

## 4. Three injection layers for custom client

| Layer | What we change        | Mechanism                                                    |
| ----- | --------------------- | ------------------------------------------------------------ |
| L1    | server + key          | `sed` in `libs/hbb_common/src/config.rs` — `RENDEZVOUS_SERVERS`, `RS_PUB_KEY` |
| L2    | quick-support password| `custom_.txt` (signature checked — `allowCustom.py` patch removes check) |
| L3    | branding (rustqs)     | `Cargo.toml`, `Runner.rc`, portable-packer (`libs/portable/generate.py`) |

Historical reference: `rdgen/.github/workflows/generator-windows.yml`.
It is not the executable workflow source and must not be copied into the fork.

---

## 5. ✅ Completed milestones

- [x] Published RustDesk client source/ref is 1.4.8; local corrective API schema target is
      `DatabaseVersion 282`, while the published DeskForge schema remains 272
- [x] Offline-kit freeze/verify tooling and local artifact set retained; real-kit
      verification remains blocked under PR10
- [x] GitHub min-test Windows: historical green run, ~33 min, single-binary rustqs.exe
- [x] Go API: provider workflow dispatch and polling; provider API artifact download;
      local validation, extraction, publication, and capability-URL TTL
- [x] Admin UI redesign: design tokens, DataTable, AppDialog, FilterBar, 16 pages
- [x] Security: encrypted-at-rest (AES-GCM), OAuth delete guard, audit, TTL
- [x] Rust server: atomic blocklist, aur-fix, JWT
- [x] Local schema target: `DatabaseVersion 282`; SQLite exercised locally,
      MySQL/PostgreSQL configured but migration/read-write support remains unverified

---

## 6. Open roadmap

- [ ] **Linux + Android capability validation** — live workflow/artifact evidence + guarded platform picker
- [ ] **Full client rebrand** — About, URLs, icons — in workflow, not in the fork
- [ ] **Smoke test** for built binary (`--version`)
- [ ] **Ballast cleanup** — remove MinGW leftovers, test containers

---

## 7. Historical fork maintenance notes for a new upstream rustdesk-client release

The steps below are historical maintenance notes for the owned RustDesk fork. They are not
the DeskForge build execution path; active workflow files live only in that
fork. Do not copy workflow files from `github-build/` or `rdgen/`.

When `rustdesk/rustdesk` publishes a new tag (e.g. 1.5.0), follow these steps:

### 7.1. Fork sync

```bash
# In bashrusakh/rustdesk:
git fetch upstream --tags
git checkout v1.5.0
git push origin v1.5.0

# In bashrusakh/hbb_common:
git fetch upstream --tags
git checkout v1.5.0   # or matching tag
git push origin v1.5.0
```

### 7.2. Repoint submodule

In the rustdesk fork:
```bash
# .gitmodules → url = https://github.com/bashrusakh/hbb_common.git, branch = v1.5.0
git submodule sync && git submodule update --init --recursive
git add .gitmodules libs/hbb_common
git commit -m "chore: point hbb_common to v1.5.0"
git push origin v1.5.0
```

### 7.3. Update vendor (historical procedure; not current repository state)

```bash
# On a machine with Rust:
cargo vendor vendor/
git add vendor/ && git commit -m "chore: vendor deps for v1.5.0"
git push origin v1.5.0
```

Or if vendor is too heavy — historically, upload `vendor-1.5.0.tar.gz` as a release
asset. DeskForge does not currently contain that vendor tree or release asset.

### 7.4. Update offline-kit

```bash
cd DeskForge/offline-kit
# versions.env: RUSTDESK_REF=v1.5.0; check MSRV, Flutter, vcpkg baseline
bash freeze.sh source vendor engine
```

### 7.5. Update offline-assets release (historical operator procedure)

```bash
# Upload engine/usbmmidd/driver to the fork:
gh release create offline-assets-1.5.0 --repo bashrusakh/rustdesk \
    --title "Offline build assets (1.5.0)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

> **Note:** After publishing the release, the version catalog can expose it in
> the admin UI when the configured repository contains the matching
> `offline-assets-*` release and source tag. The catalog is repository-derived;
> there is no hardcoded repository or obsolete version fallback list.

### 7.6. Adapt workflow

Compare upstream `build-for-windows-flutter` with `rustqs-windows.yml`:
- New system dependencies?
- Changed `build.py` flags?
- Changed `config.rs` / `custom_.txt` format?

Port changes to the fork workflow.

> **Important:** `bridge.yml` must stay **without `inputs.version`** — same as upstream.
> Bridge and build must work from the same code (the fork). Do not add `repository:` to checkout.

### 7.7. Workflow ownership

The `.github/workflows/` files in the owned RustDesk fork are the sole
executable source. `github-build/` is reference/documentation only, and the
vendored `rdgen` workflows are historical/reference material. Maintain active
workflow logic in the fork; do not create deployment copies in this repository.

### 7.8. Verify

- [ ] GitHub Actions run ✅ (no `startup_failure`)
- [ ] `VERSION` in logs matches the version selected in admin UI
- [ ] Binary arrived at the server
- [ ] `rustqs.exe`, ~23 MB, `custom_.txt` packed inside
- [ ] Smoke test on clean Windows

### 7.9. Update DeskForge reference

- [ ] `offline-kit/versions.env` — new `RUSTDESK_REF`
- [ ] `offline-kit/FORK-PROCEDURE.md` — update versions in examples
- [ ] `PLAN.md` — update current tag in §1
- [ ] `github-build/README.md` — update patch URLs if changed

---

## 8. What are offline-kit and offline-assets

| Entity                  | What it is                                           | Where stored                          |
| ----------------------- | ---------------------------------------------------- | ------------------------------------- |
| `offline-kit/`          | Scripts (`freeze.sh`) + config (`versions.env`)      | In git, in this repo                  |
| `offline-kit/artifacts/`| Local freeze output (ignored/untracked)              | Local operator storage only           |
| `offline-assets-{tag}`  | Provider-side asset naming convention                | Availability and release proof unverified |

**Why:** without this insurance, loss of upstream source or services can block a custom
client build. The kit is frozen local operator material; active third-party upstream
dependencies remain in the source manifests, and its current incomplete verification
does not prove upstream independence, sovereignty, a network-denied full build, a
signature, or release readiness.

---

## 9. Abandoned (do not repeat)

| Approach                     | Why dead                                           |
| ---------------------------- | -------------------------------------------------- |
| MinGW cross-compile Flutter  | Flutter Windows requires MSVC, cannot cross-compile|
| `windows-x86` target         | 32-bit not supported in 2026                      |
| standalone win-builder       | frozen — GitHub-first                              |

---

## 10. Reference facts

- `custom.json` in `flutter/lib/` — no-op, not read by the code.
- Real mechanism: `custom_.txt` + `config.rs`.
- `read_custom_client` checks signature — `allowCustom.py` patch removes the check.
- Patches in `rdgen/.github/patches/`: allowCustom, hidecm, removeSetupServerTip,
  removeNewVersionNotif, cycle_monitor, xoffline, privacyScreen.
