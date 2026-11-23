# PLAN.md — DeskForge: Single Source of Truth

> Last updated: 2026-08-20
> Related: [CHANGELOG.md](CHANGELOG.md) · [BUGS.md](BUGS.md) · [CONTRIBUTING.md](CONTRIBUTING.md)

> **Evidence labels (2026-08-20):** The published RustDesk client source/ref is
> 1.4.8 and the published DeskForge API schema is `DatabaseVersion` 272. The current
> local corrective candidate targets API schema 285; it is not a public current-schema
> or release claim. Schema 283 adds `idx_custom_presets_user_id_name` on
> `(user_id, name)` and fails preflight on existing duplicate groups before
> `AutoMigrate`, without auto-selecting a row or deleting data. Schema 282 is the earlier
> workflow-approval migration preserved as history.
> The current API path is provider dispatch → exact run/artifact retrieval → local
> validation and publication. Runner callbacks, local file-queue fallback, and
> sole-artifact selection are not current production behavior. Windows is the only
> production capability admitted by the API gate; Linux/Android remain gated pending
> PR11 evidence. The dispatch contract under test sends only the `ref` selector
> and required `inputs` (no `return_run_details`) and pins the dispatch request to
> the `X-GitHub-Api-Version: 2026-03-10` version (the client-wide pin stays
> `2022-11-28`). GitHub's 2026-03-10 breaking change removed the
> `return_run_details` parameter and made the endpoint always return HTTP 200 with
> the run details, so the dispatch reads the run details directly from an always-200
> response and no longer sends the retired parameter; this is a deliberate contract
> modernization, not a #80 requirement (the pinned `2022-11-28` version with
> `return_run_details=true` also returns the run details). Standard 204 is
> intentionally unsupported because it cannot correlate the exact run. No
> normal GitHub operation is verified. The Go API owns
> port 21114; Rust services use 21115–21119, with relay WebSocket on 21119; current
> production Compose exposure remains
> 21114–21118. No live provider run or clean-environment build proof is claimed.
> Workflow approval requires a provider-derived verified annotated tag, the
> aggregate of all applicable active protected-tag rulesets with effective update
> and deletion protections, no bypass actors, and rejection of tag/branch label
> collisions. At the resolved immutable workflow SHA, the exact provider-owned
> `# deskforge-workflow-identity-guard: v1` marker is required before approval,
> preparation, or secret-bearing dispatch; legacy unguarded tags fail closed. The
> local gate is not
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
│   └── libs/hbb_common/   ← tracked copy of our hbb_common fork @ pin + patches; not a submodule here
├── rustdesk               ← owned RustDesk fork and active workflow source
│   └── .github/workflows/ ← rustqs-windows.yml, rustqs-linux.yml, rustqs-android.yml
└── hbb_common             ← mirror fork of rustdesk/hbb_common (must be kept in sync with upstream; main = upstream + non-behavioral infra commits; never behavioral patches)
```

**Published client source/ref:** 1.4.8, aligned with `offline-kit/versions.env`. The
current local corrective candidate targets API schema 285; the published DeskForge
schema remains `DatabaseVersion` 272. Schema 283 adds
`idx_custom_presets_user_id_name` on `(user_id, name)` and fails preflight on existing
duplicate groups before `AutoMigrate`, without auto-selecting a row or deleting data. Schema 282
remains the earlier workflow-approval migration.
The configured RustDesk fork's `libs/hbb_common` is a separate git submodule whose
gitlink is repointed at our mirror fork `bashrusakh/hbb_common` by SHA (`master` →
`36ab6ed`, 2026-11-23); DeskForge's own `libs/hbb_common/` copy is pinned at `387603f`
plus local patches (pin record: `libs/hbb_common/UPSTREAM.md`; topology and rules:
`docs/FORKS-AND-PINS.md`). Active RustDesk and DeskForge manifests also reference
third-party upstream repositories/modules. Provider-side live execution and clean-build proof are not recorded
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
   ↓ workflow_dispatch + provider-derived public workflow_sha + authenticated DFP1 enc_payload (AES-256-CBC + PBKDF2 + HMAC)
GitHub Actions [configured RustDesk fork, owned platform workflow]
   ↓ L1: config.rs (server + key)
   ↓ L2: custom_.txt (permanent password, allowCustom patch)
   ↓ L3: branding (rustqs, portable-packer)
provider run ← Go API polls provider status
provider artifact API → Go API downloads the exact artifact
Go API validates/extracts/publishes locally → admin-ui Download
```

**Security:** password never published — `enc_payload`, decrypted inside runner via
GitHub Secret `WORKFLOW_PAYLOAD_KEY`. Before approval, preparation, or secret-bearing
dispatch, the API requires the exact provider-owned
`# deskforge-workflow-identity-guard: v1` marker in workflow content resolved at the
immutable workflow SHA; legacy unguarded tags fail closed. The provider-derived outer
`workflow_sha` is checked against `github.sha` before secret-bearing jobs; the same
authenticated inner payload field is checked again before exports, checkout, or build
use. This is defense in depth, not an atomic defense against a malicious workflow file
or GitHub's selector/SHA binding; the verified tag and active no-bypass ruleset remain
required controls. The runner does not callback to the API; the API retrieves the
artifact through the provider API and publishes it locally, not to a public release.

### Workflow approval and schema evidence

Before a provider-backed build can run, the API accepts only a provider-derived
verified annotated tag. The provider must report an accepted verification reason,
the aggregate of all applicable active protected tag rulesets for that label with
effective update and deletion protections, and no bypass actors. A tag/branch
collision with the same label is rejected. At the resolved immutable workflow SHA,
the exact provider-owned `# deskforge-workflow-identity-guard: v1` marker is also
required before approval, preparation, or secret-bearing dispatch; legacy unguarded
tags fail closed. Raw refs, SHAs, credentials, and workflow internals are not normal
UI inputs.

The published DeskForge schema is `DatabaseVersion` **272**. The current local
corrective candidate targets schema **285**, which is not published or live-provider
evidence. The historical workflow-approval additive fields are **280**
(`workflow_ref_approved`, approval status), **281**
(`workflow_ref_provider_verified`, provider-policy status), and **282**
(`workflow_ref_approval_sha`, the provider-resolved approval commit). Schema **283**
adds the `idx_custom_presets_user_id_name` composite index on `(user_id, name)`.
Before `AutoMigrate`, migration preflight fails if it finds an existing duplicate
group; it does not automatically select or delete a row. Legacy metadata-only
configuration saves remain compatible with plaintext secret rows: they can preserve
legacy values without `SECRET_ENCRYPTION_KEY`; new or replaced non-empty secret writes
require the key, and a resave with the key encrypts legacy plaintext values.

Schema 284 adds create-time `preset_id` and `preset_name` provenance snapshots to
`custom_builds`. Schema 285 runs the custom-client branding-field preflight before
`AutoMigrate`, failing closed when an existing `custom_builds` or `custom_presets`
row already carries a branding field value the shared branding seam rejects.
SQLite-only tests cover the schema-283 duplicate-data preflight/unique index,
schema-284 provenance columns (including migration and readback of a populated
pre-provenance `custom_builds` row), and the schema-285 branding preflight/version
gate: an install already at 284 fails closed on a rejected stored branding value
without recording 285, an accepted one upgrades and records 285, and a fresh, 283,
or 284 install runs the preflight while a 285 install skips it. MySQL/PostgreSQL
migration and read/write coverage remain unverified. Local workflow manifests,
bridge/helper source, Android app-ID/runtime-path checks, and package assertions
are static evidence only; no APK/package/install/runtime evidence exists.

Focused fake-transport and SQLite checks do not prove live GitHub/provider
approval, dispatch, polling, or artifact delivery. Dispatch relies on the exact
HTTP 200 run-details contract, pins the dispatch request to the
`X-GitHub-Api-Version: 2026-03-10` version (the client-wide pin stays
`2022-11-28`), and sends only the ref/inputs body: GitHub's 2026-03-10 breaking
change removed `return_run_details` and made the endpoint always return HTTP 200
with the run details, so the dispatch reads them directly and no longer sends the
retired parameter. This is a deliberate contract modernization, not a #80
requirement (the pinned `2022-11-28` version with `return_run_details=true` also
returns the run details). A standard 204 or latest-run inference is unsupported.
The local identity write is atomic, but no durable outbox
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

- [x] Published RustDesk client source/ref is 1.4.8; current local corrective API schema target is
      `DatabaseVersion 285`, while the published DeskForge schema remains 272
- [x] Offline-kit freeze/verify tooling and local artifact set retained; real-kit
      verification remains blocked under PR10
- [x] GitHub min-test Windows: historical green run, ~33 min, single-binary rustqs.exe
- [x] Go API: provider workflow dispatch and polling; provider API artifact download;
      local validation, extraction, publication, and capability-URL TTL
- [x] Admin UI redesign: design tokens, DataTable, AppDialog, FilterBar, 16 pages
- [x] Security: encrypted-at-rest (AES-GCM), OAuth delete guard, audit, TTL
- [x] Rust server: atomic blocklist, aur-fix, JWT
- [x] Current local schema target: `DatabaseVersion 285`; the schema-283 composite-index
      migration is exercised only on SQLite, while MySQL/PostgreSQL migration/read/write
      support remains unverified

---

## 6. Open roadmap

- [ ] **Linux + Android capability validation** — live workflow/artifact evidence + guarded platform picker
- [ ] **Full client rebrand** — About, URLs, icons — in workflow, not in the fork
- [ ] **Smoke test** for built binary (`--version`)
- [ ] **Ballast cleanup** — remove MinGW leftovers, test containers

---

## 7. Fork maintenance procedure for a new upstream rustdesk-client release

Maintenance procedure for the owned forks and DeskForge's `libs/hbb_common/` copy. It is
not the DeskForge build execution path; active workflow files live only in that
fork. Do not copy workflow files from `github-build/` or `rdgen/`. Fork topology, sync
rules, pin bookkeeping, and the DeskForge re-pin mechanics are defined in
[docs/FORKS-AND-PINS.md](docs/FORKS-AND-PINS.md); the current pin and patch record is
[`libs/hbb_common/UPSTREAM.md`](libs/hbb_common/UPSTREAM.md).

**Pins are SHAs.** Upstream `rustdesk/hbb_common` has no version tags at all, so every
`hbb_common` pin is a commit SHA. Client release tags (e.g. `1.5.0`, unprefixed) exist
only on `rustdesk/rustdesk` — and tag names are not identities: the fork's `1.5.0`
(`9253e1fc`) is a fork commit, not upstream's `1.5.0` (`fada664d`). Record the SHA.

When `rustdesk/rustdesk` publishes a new tag, follow these steps:

### 7.1. Sync the `hbb_common` fork

Standing rule: [`bashrusakh/hbb_common`](https://github.com/bashrusakh/hbb_common) must
be kept in sync with upstream automatically, where technically possible, by
`sync-upstream.yml` in the fork repository — merge-based (`upstream/main` into `main`,
never history-rewriting), so consumer SHA pins stay valid. Installation status, manual
fallback (merge-based commands and conflict rules):
[docs/FORKS-AND-PINS.md §3](docs/FORKS-AND-PINS.md#3-fork-maintenance).

### 7.2. Re-point the RustDesk fork's gitlink by SHA

In `bashrusakh/rustdesk`, bring the branch up to the new client tag (fetch
`upstream --tags`, then merge or push the mirrored tag — mind the tag-collision caution
above) and record the chosen `hbb_common` pin SHA in the `libs/hbb_common` gitlink
(URL: `https://github.com/bashrusakh/hbb_common.git`). Tag-collision resolution: never
overwrite an existing published fork tag (nothing is history-rewritten) — if the fork
already owns a different commit under the upstream tag name (as with `1.5.0`), record
the collision and the upstream tag's SHA, and if a mirror ref is needed use a
disambiguating ref name (e.g. `upstream-<tag>`). Commands:
[docs/FORKS-AND-PINS.md §5](docs/FORKS-AND-PINS.md#5-updating-the-deskforge-copy-to-a-new-pin).

### 7.3. Re-pin DeskForge's `libs/hbb_common/` copy

Replace the tracked copy with the fork's content at the pin SHA, record the new fork
point in [`libs/hbb_common/UPSTREAM.md`](libs/hbb_common/UPSTREAM.md), and re-apply the
documented patch set (`src/lib.rs` reproducible `gen_version()`, `src/config.rs`
external-endpoint neutralization, `src/webrtc.rs` `DEFAULT_ICE_SERVERS = []`):
[docs/FORKS-AND-PINS.md §5](docs/FORKS-AND-PINS.md#5-updating-the-deskforge-copy-to-a-new-pin).

> **Important:** the `gen_version()` patch must survive every re-pin; the `server/`
> build coupling it guarantees is recorded in
> [`libs/hbb_common/UPSTREAM.md`](libs/hbb_common/UPSTREAM.md).

### 7.4. Update vendor (historical procedure; not current repository state)

```bash
# On a machine with Rust:
cargo vendor vendor/
git add vendor/ && git commit -m "chore: vendor deps for <client-tag>"
git push origin <client-tag>
```

Or if vendor is too heavy — historically, upload `vendor-<client-tag>.tar.gz` as a
release asset. DeskForge does not currently contain that vendor tree or release asset.

### 7.5. Re-freeze offline-kit (when the client line changes)

```bash
cd DeskForge/offline-kit
# versions.env: RUSTDESK_REF=<client-tag>, RUSTDESK_COMMIT=<full SHA>; check MSRV, Flutter, vcpkg baseline
bash freeze.sh source vendor engine
```

### 7.6. Update offline-assets release (historical operator procedure)

```bash
# Upload engine/usbmmidd/driver to the fork:
gh release create offline-assets-<client-tag> --repo bashrusakh/rustdesk \
    --title "Offline build assets (<client-tag>)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

> **Note:** After publishing the release, the version catalog can expose it in
> the admin UI when the configured repository contains the matching
> `offline-assets-*` release and source tag. The catalog is repository-derived;
> there is no hardcoded repository or obsolete version fallback list.

### 7.7. Adapt workflow

Compare upstream `build-for-windows-flutter` with `rustqs-windows.yml`:
- New system dependencies?
- Changed `build.py` flags?
- Changed `config.rs` / `custom_.txt` format?

Port changes to the fork workflow.

> **Important:** `bridge.yml` must stay **without `inputs.version`** — same as upstream.
> Bridge and build must work from the same code (the fork). Do not add `repository:` to checkout.

### 7.8. Workflow ownership

The `.github/workflows/` files in the owned RustDesk fork are the sole
executable source. `github-build/` is reference/documentation only, and the
vendored `rdgen` workflows are historical/reference material. Maintain active
workflow logic in the fork; do not create deployment copies in this repository.

### 7.9. Verify

- [ ] GitHub Actions run ✅ (no `startup_failure`)
- [ ] `VERSION` in logs matches the version selected in admin UI
- [ ] Binary arrived at the server
- [ ] `rustqs.exe`, ~23 MB, `custom_.txt` packed inside
- [ ] Smoke test on clean Windows

### 7.10. Update DeskForge reference

- [ ] `libs/hbb_common/UPSTREAM.md` — new fork point + patch record
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
