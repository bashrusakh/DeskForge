# DeskForge — AI Agent Reference

## Core purpose

DeskForge — unified self-hosted RustDesk сервер: Rust hbbs/hbbr, Go REST API, Vue 3 админка, rdgen reference.
Всё в одном Docker образе через s6-overlay.

## Runtime architecture

- **Rust servers** (`server/`): hbbs (ID/signaling, TCP/UDP 21116) + hbbr (relay, TCP 21117)
- **Go API** (`api/`): Gin на порту 21114. GORM (SQLite/MySQL/PostgreSQL). JWT, LDAP, OIDC.
- **Admin UI** (`admin-ui/`): Vue 3 + Element Plus. Served at `/admin/`. REST + WebSocket.
- **rdgen** (`rdgen/`): vendored reference workflow (не сервис).
- **Shared lib** (`libs/hbb_common`): Rust crate между hbbs и hbbr.

## Tech stack

| Компонент     | Стек                                                              |
| ------------- | ----------------------------------------------------------------- |
| Rust (server) | 2021 edition, axum 0.5, sqlx 0.6, tokio, sodiumoxide, openssl    |
| Go (api)      | 1.23, gin 1.9, gorm 1.25, swag, cobra/viper, jwt, ldap, OIDC     |
| Admin UI      | Vue 3.5, Element Plus 2.8, Vite 6, Pinia 2.2, vue-router 4, axios|
| Python (rdgen)| Django (vendored reference, не сервис)                           |
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

admin-ui/        — Vue 3 админка
├── src/views/   — 16 страниц (login, index, user, peer, address_book, group, tag, oauth, audit, server, custom-client, my, ...)
├── src/components/ui/ — DataTable, AppDialog, AppDrawer, FilterBar, PageHeader, PageSection, DangerZone, ConnectionPulse, ...
├── src/store/   — Pinia (user, app, tags, router)
├── src/api/     — axios wrappers
├── src/styles/  — SCSS (design tokens, light/dark)
└── src/utils/   — auth, request, export, i18n (en/ru/zh_CN)

rdgen/           — vendored reference workflow (патчи, generator-*.yml)
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
docker compose build          # полная сборка
docker compose up -d          # запуск
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

- Go читает public key из `RUSTDESK_API_KEY_FILE` (`/data/id_ed25519.pub`)
- Go подключается к hbbs/hbbr по `RUSTDESK_API_RUSTDESK_ID_SERVER`/`RELAY_SERVER`
- JWT: Go генерирует, Rust валидирует (`jwt.rs`)
- WebSocket bridge: порт 21118

### Admin UI ↔ Go API

- REST: `/api/` (PC client) + `/admin/api/` (admin-only)
- Auth: JWT в cookie, опционально OAuth
- Swagger: `/admin/swagger/index.html`
- WebSocket: real-time peer status

## Agent constraints

- Не модифицировать upstream напрямую (`rustdesk/rustdesk-server`, `lejianwen/rustdesk-api`) — только форки.
- Docker entrypoint синхронизировать с сервисами.
- Не логировать/коммитить secrets.
- Документировать env vars в README + docker-compose.

## Development rules

- **Go:** избегать `interface{}`, typed errors, `go vet + errcheck`.
- **Rust:** `clippy`-clean, без `unwrap()` в production, `?` для ошибок.
- **Vue:** Composition API (`<script setup>`), Pinia, Element Plus.
- **Python (rdgen):** Django conventions, minimal.

## Architecture patterns

- **Clean layered (Go):** Controller → Service → Model. Не смешивать.
- **Embedded UI:** Go встраивает `admin-ui/dist/` и `web/`.
- **Multi-DB:** GORM, без raw SQL.
- **OAuth/LDAP:** через админку → DB. Fallback на локальных юзеров.
- **Server commands:** allowlist в `serverCmd.go`.

## Regression-prevention

- Меняешь Go routes → проверь admin-ui и PC client API.
- Меняешь GORM models → проверь миграцию на всех 3 DB.
- Меняешь JWT → проверь что Rust валидирует.
- Меняешь admin-ui → `npm run build` проходит.
- Меняешь Docker → s6-overlay стартует все сервисы.
- Добавляешь env var → README + docker-compose.

## New upstream version workflow

См. [PLAN.md §7](PLAN.md#7-workflow-вышла-новая-версия-upstream-rustdesk-client).
