# DeskForge — AI Agent Reference

## Core purpose

DeskForge — unified self-hosted RustDesk server: Rust hbbs/hbbr, Go REST API, Vue 3 admin panel, rdgen reference.
Everything in one Docker image via s6-overlay.

## Runtime architecture

- **Rust servers** (`server/`): hbbs (ID/signaling, TCP/UDP 21116) + hbbr (relay, TCP 21117)
- **Go API** (`api/`): Gin on port 21114. GORM (SQLite/MySQL/PostgreSQL). JWT, LDAP, OIDC.
- **Admin UI** (`admin-ui/`): Vue 3 + Element Plus. Served at `/admin/`. REST + WebSocket.
- **rdgen** (`rdgen/`): vendored reference workflow (not a service).
- **Shared lib** (`libs/hbb_common`): Rust crate shared between hbbs and hbbr.

## Tech stack

| Component     | Stack                                                             |
| ------------- | ----------------------------------------------------------------- |
| Rust (server) | 2021 edition, axum 0.5, sqlx 0.6, tokio, sodiumoxide, openssl    |
| Go (api)      | 1.23, gin 1.9, gorm 1.25, swag, cobra/viper, jwt, ldap, OIDC     |
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

rdgen/           — vendored reference workflow (patches, generator-*.yml)
libs/hbb_common/ — shared Rust library (submodule)
docker/          — Dockerfile + compose + entrypoint scripts
github-build/    — active CI workflow for client builds
win-builder/     — ❄️ frozen standalone Windows builder
offline-kit/     — ❄️ frozen sovereign build kit
```

## Build / dev commands

### Docker (primary)

```bash
cd docker
docker compose build          # full build
docker compose up -d          # start
docker compose -f docker-compose-dev.yaml up -d   # dev
```

### Rust

```bash
cd server && cargo build --release && cargo clippy && cargo test
```

### Go

```bash
cd api && go build -o release/apimain cmd/apimain.go && go vet ./... && go test ./...
```

### Admin UI

```bash
cd admin-ui && npm install && npm run dev && npm run build
```

## Environment variables (critical)

| Variable | Purpose | Used by |
|----------|---------|---------|
| `RELAY` | Relay server address | Rust hbbr |
| `ENCRYPTED_ONLY` | Only encrypted connections | Rust |
| `MUST_LOGIN` | Require login before connect | Rust |
| `RUSTDESK_API_RUSTDESK_ID_SERVER` | ID server address | Go API |
| `RUSTDESK_API_RUSTDESK_RELAY_SERVER` | Relay server address | Go API |
| `RUSTDESK_API_RUSTDESK_API_SERVER` | API server URL | Go API |
| `RUSTDESK_API_KEY_FILE` | Path to public key file | Go API |
| `RUSTDESK_API_JWT_KEY` | JWT secret key | Go + Rust |
| `RUSTDESK_API_GORM_TYPE` | sqlite/mysql/postgres | Go API |
| `RUSTDESK_API_LANG` | en/ru/zh-CN | Go + UI |
| `SECRET_CRYPT_KEY` | AES-GCM key for secrets at rest | Go API |

## Key integration points

### Rust ↔ Go API

- Go reads public key from `RUSTDESK_API_KEY_FILE` (`/data/id_ed25519.pub`)
- Go connects to hbbs/hbbr via `RUSTDESK_API_RUSTDESK_ID_SERVER`/`RUSTDESK_API_RUSTDESK_RELAY_SERVER`
- JWT: Go generates, Rust validates (`jwt.rs`)
- WebSocket bridge: port 21118

### Admin UI ↔ Go API

- REST: `/api/` (PC client) + `/api/admin/` (admin-only)
- Auth: JWT in cookie, optional OAuth
- Swagger: `/admin/swagger/index.html`
- WebSocket: real-time peer status

## Agent constraints

- Do not modify upstream directly (`rustdesk/rustdesk-server`, `lejianwen/rustdesk-api`) — only forks.
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
- **Multi-DB:** GORM, no raw SQL.
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

See [PLAN.md §7](PLAN.md#7-workflow-new-upstream-rustdesk-client-release).
