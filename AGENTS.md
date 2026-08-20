# DeskForge — AI Agent Reference

## Core purpose

DeskForge — unified self-hosted RustDesk server: Rust hbbs/hbbr, Go REST API, Vue 3 admin panel, rdgen reference.
Everything in one Docker image via s6-overlay.

## Runtime architecture

- **Rust servers** (`server/`): hbbs (ID/signaling, TCP/UDP 21116) + hbbr (relay, TCP 21117)
- **Go API** (`api/`): Gin on port 21114. GORM (SQLite/MySQL/PostgreSQL). JWT, LDAP, OIDC.
- **Admin UI** (`admin-ui/`): Vue 3 + Element Plus. Served at `/admin/`. REST + WebSocket.
- **rdgen** (`rdgen/`): vendored historical/reference workflow material (not a service; frozen).
- **Shared lib** (`libs/hbb_common`): Rust crate shared between hbbs and hbbr.

## Tech stack

| Component     | Stack                                                             |
| ------------- | ----------------------------------------------------------------- |
| Rust (server) | 2021 edition, axum 0.5, sqlx 0.6, tokio, sodiumoxide, openssl    |
| Go (api)      | 1.25, gin 1.9, gorm 1.25, swag, cobra/viper, jwt, ldap, OIDC     |
| Admin UI      | Vue 3.5, Element Plus 2.8, Vite 6, Pinia 2.2, vue-router 4, axios|
| Python (rdgen)| Django (vendored reference, not a service)                      |
| Infra         | Docker + s6-overlay, docker compose                               |

## Monorepo layout

```
server/          — Rust hbbs/hbbr
├── src/main.rs  — hbbs entry
├── src/hbbr.rs  — relay entry
├── rendezvous_server.rs, relay_server.rs, database.rs, jwt.rs, peer.rs
└── Cargo.toml

api/             — Go REST API
├── cmd/apimain.go
├── http/        — bootstrap, router, controller, middleware
├── service/     — user, peer, addressBook, oauth, ldap, group, tag, serverCmd, audit, custom_build, github_build_config
├── model/       — GORM + custom types
├── lib/         — cache, jwt, orm, logger, lock, upload
├── global/      — global state
└── conf/config.yaml

admin-ui/        — Vue 3 admin panel
├── src/views/   — 16 pages (login, index, user, peer, address_book, group, tag, oauth, audit, server, custom-client, my, ...)
├── src/components/ui/ — DataTable, AppDialog, AppDrawer, FilterBar, PageHeader, PageSection, DangerZone, ConnectionPulse, ...
├── src/store/   — Pinia (user, app, tags, router)
├── src/api/     — axios wrappers
├── src/styles/  — SCSS (design tokens, light/dark)
└── src/utils/   — auth, request, export, i18n (en/ru/zh_CN)

rdgen/           — ❄️ frozen vendored historical/reference workflow material (patches, generator-*.yml)
libs/hbb_common/ — tracked shared Rust library in DeskForge (not a submodule here)
docker/          — Dockerfile + compose + entrypoint scripts
github-build/    — reference/documentation only; no executable workflow copies
win-builder/     — ❄️ frozen manual/historical-only standalone builder
offline-kit/     — ❄️ frozen dependency-freeze tool; verification is incomplete
```

DeskForge tracks `libs/hbb_common/` directly. The configured RustDesk fork has its
own `libs/hbb_common` git submodule, currently recorded against upstream
`rustdesk/hbb_common`; its local state is dirty and unpublished, so clean fork
provenance is an explicit reproducibility gate. The fork's `.github/workflows/` files
are the sole executable source for active client-build workflows; `github-build/` and
`rdgen/` are reference/frozen material only. No current repository `vendor/` tree,
`rustdesk-deps/` archive, or client-release directory is tracked here.
The published RustDesk client source/ref is 1.4.8 and the published DeskForge API
schema is `DatabaseVersion` 272. The current local corrective candidate targets API
schema 283, which is not published or live-provider evidence. Schema 283 adds
`idx_custom_presets_user_id_name` on `(user_id, name)`; before `AutoMigrate`, its
preflight fails on existing duplicate groups without auto-selecting a row or deleting
data. The earlier schema-282 workflow-approval migration remains historical
context. SQLite is the only recorded migration evidence; MySQL/PostgreSQL migration
and read/write coverage remain unverified.

## Build / dev commands

### Docker (primary)

```bash
cd docker
docker compose build          # full build
docker compose up -d          # start
cd ../api && docker compose -f docker-compose-dev.yaml up -d   # dev API stack
```

### Rust

```bash
cd server && cargo build --release && cargo clippy && cargo test
```

### Go

```bash
cd api && go build -o release/apimain cmd/apimain.go
# Full local checks:
GOWORK=off go vet ./...
GOWORK=off go test ./...
# Redis integration tests and benchmarks are opt-in. Configure
# DESKFORGE_TEST_REDIS_ADDR to run them; no live Redis endpoint is assumed.
```

### Admin UI

```bash
cd admin-ui && npm install && npm run dev && npm run build
```

## Environment variables (critical)

| Variable | Purpose | Used by |
|----------|---------|---------|
| `RELAY` | Relay server address | Rust hbbr |
| `HBBR_PORT` | Relay server port | Rust hbbr |
| `HBBS_PORT` | ID/Rendezvous server port | Rust hbbs |
| `ENCRYPTED_ONLY` | Only encrypted connections | Rust |
| `MUST_LOGIN` | Require login before connect | Rust |
| `RUSTDESK_API_RUSTDESK_ID_SERVER` | ID server address | Go API |
| `RUSTDESK_API_RUSTDESK_RELAY_SERVER` | Relay server address | Go API |
| `RUSTDESK_API_RUSTDESK_API_SERVER` | API server URL | Go API |
| `RUSTDESK_API_KEY_FILE` | Path to public key file | Go API |
| `RUSTDESK_API_JWT_KEY` | JWT secret key | Go + Rust |
| `RUSTDESK_API_GORM_TYPE` | sqlite/mysql/postgresql | Go API |
| `RUSTDESK_API_LANG` | en/ru/zh-CN | Go + UI |
| `SECRET_ENCRYPTION_KEY` | AES-GCM key for secrets at rest | Go API |

New non-empty secret writes and secret-bearing Custom Builder operations require this
key and are rejected rather than stored as plaintext when it is missing. Legacy plaintext
rows remain readable; saving them again encrypts them when the key exists.

## Key integration points

### Rust ↔ Go API

- Go reads public key from `RUSTDESK_API_KEY_FILE` (`/data/id_ed25519.pub`)
- Go connects to hbbs/hbbr via `RUSTDESK_API_RUSTDESK_ID_SERVER`/`RUSTDESK_API_RUSTDESK_RELAY_SERVER`
- JWT: Go generates, Rust validates (`jwt.rs`)
- WebSocket bridge: port 21118; relay WebSocket: port 21119

### Admin UI ↔ Go API

- REST: `/api/` (PC client) + `/api/admin/` (admin-only)
- Auth: JWT in cookie, optional OAuth
- Swagger: `/admin/swagger/index.html`
- WebSocket: real-time peer status

## Agent constraints

- Do not modify upstream directly (`rustdesk/rustdesk-server`, `lejianwen/rustdesk-api`) — only forks.
- Active third-party upstream dependencies remain in the Rust/Go manifests; a complete
  upstream-independent or offline dependency bundle is not currently verified.
- The combined DeskForge distribution is identified as AGPL-3.0 for the covered work;
  separate API/UI/reference components retain their applicable upstream licenses and notices.
  The license inventory is incomplete; no signatures or attestations are recorded, and no
  full sovereignty claim is made.
- Keep Docker entrypoint scripts in sync with the services they supervise.
- Never log or commit secrets.
- Document env vars in README + docker-compose.

## Development rules

- **Go:** avoid `interface{}`, use typed errors, `go vet + errcheck`.
- **Rust:** `clippy`-clean, no `unwrap()` in production, `?` for errors.
- **Vue:** Composition API (`<script setup>`), Pinia, Element Plus.
- **Python (rdgen):** Django conventions, minimal.

## Architecture patterns

- **Clean layered (Go):** Controller → Service → Model. Do not mix layers.
- **Embedded UI:** Go embeds `admin-ui/dist/` and `web/`.
- **Configured multi-DB:** GORM with SQLite/MySQL/PostgreSQL drivers, no raw SQL;
  SQLite is locally exercised, while MySQL/PostgreSQL migration and read/write
  coverage remain unverified.
- **OAuth/LDAP:** configured via admin panel → DB. Falls back to local users.
- **Server commands:** allowlist in `serverCmd.go`.

## Regression-prevention

- Changing Go routes? Check admin-ui and PC client API.
- Changing GORM models? Check migration on all 3 DB types.
- Changing JWT? Verify Rust still validates tokens.
- Changing admin-ui? `npm run build` must pass.
- Changing Docker? s6-overlay must start all services.
- Adding env var? Update README + docker-compose.

## New upstream version workflow

See [PLAN.md §7](PLAN.md#7-historical-fork-maintenance-notes-for-a-new-upstream-rustdesk-client-release).
