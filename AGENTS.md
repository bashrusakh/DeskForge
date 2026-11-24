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
└── src/utils/   — auth, request, i18n (en/ru/zh_CN), clipboard/file/peer/time, webclient/

rdgen/           — ❄️ frozen vendored historical/reference workflow material (patches, generator-*.yml)
libs/hbb_common/ — tracked shared Rust library in DeskForge; copy of our hbb_common mirror fork @ pin + patches (not a submodule here)
docker/          — Dockerfile + compose + entrypoint scripts
github-build/    — reference/documentation only; no executable workflow copies
win-builder/     — ❄️ frozen manual/historical-only standalone builder
offline-kit/     — ❄️ frozen dependency-freeze tool; verification is incomplete
```

DeskForge tracks `libs/hbb_common/` directly — a copy of our mirror fork
`bashrusakh/hbb_common` at a recorded pin plus a documented patch set. Canonical homes:
fork topology, sync rules, pin bookkeeping, and workflow ownership (the fork's
`.github/workflows/` are the sole executable workflow source; `github-build/` and
`rdgen/` are reference/frozen material only) →
[docs/FORKS-AND-PINS.md](docs/FORKS-AND-PINS.md); pin + patch record →
[`libs/hbb_common/UPSTREAM.md`](libs/hbb_common/UPSTREAM.md); current schema/migration
state → [PLAN.md §2](PLAN.md#2-architecture).

## Build / dev commands

Canonical commands: [CONTRIBUTING.md](CONTRIBUTING.md) ("Dev Commands") — Docker
(`docker/`), Rust (`server/`), Go (`api/`), admin UI (`admin-ui/`); component-local
commands also live in [api/README.md](api/README.md) and
[admin-ui/README.md](admin-ui/README.md). Redis integration tests/benchmarks are opt-in
(`DESKFORGE_TEST_REDIS_ADDR`); no live Redis run is recorded.

## Environment variables

The canonical variable table lives in [`.env.example`](.env.example); full semantics
are in [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Key integration points

### Rust ↔ Go API

- Go reads public key from `RUSTDESK_API_KEY_FILE` (`/data/id_ed25519.pub`)
- Go connects to hbbs/hbbr via `RUSTDESK_API_RUSTDESK_ID_SERVER`/`RUSTDESK_API_RUSTDESK_RELAY_SERVER`
- JWT: Go generates, Rust validates (`jwt.rs`)
- WebSocket bridge: port 21118; relay WebSocket: port 21119

### Admin UI ↔ Go API

- REST: `/api/` (PC client) + `/api/admin/` (admin-only)
- Auth: JWT (HS256) in the `api-token` header, token kept client-side in localStorage; optional OAuth
- Swagger: `/admin/swagger/index.html`
- WebSocket: real-time peer status

## Agent constraints

- Do not modify upstream directly (`rustdesk/rustdesk-server`, `lejianwen/rustdesk-api`) — only forks.
- Active third-party upstream dependencies remain in the Rust/Go manifests; a complete
  upstream-independent or offline dependency bundle is not currently verified.
- The combined DeskForge distribution is identified as AGPL-3.0 for the covered work;
  component licenses, the incomplete inventory, and the no-sovereignty non-claim:
  [LICENSE](LICENSE), [NOTICE](NOTICE), [README](README.md) ("License").
- Keep Docker entrypoint scripts in sync with the services they supervise.
- Never log or commit secrets.
- Document env vars in `.env.example` + docker-compose.

## Development rules

- **Go:** avoid `interface{}`, use typed errors, `go vet + errcheck`.
- **Rust:** `clippy`-clean, no `unwrap()` in production, `?` for errors.
- **Vue:** Composition API (`<script setup>`), Pinia, Element Plus.
- **Python (rdgen):** Django conventions, minimal.

## Architecture patterns

- **Clean layered (Go):** Controller → Service → Model. Do not mix layers.
- **Admin UI serving:** no `go:embed` — the production image copies `admin-ui/dist`
  to `/app/resources/admin/` (`docker/Dockerfile`) and Gin serves it as static files
  (`g.StaticFS("/admin", http.Dir(...))`, `api/http/router/router.go`). The webclient
  ships from `api/resources/web` (`COPY api/resources /app/resources`, `docker/Dockerfile`)
  and Gin serves it at `/webclient` (`g.StaticFS("/webclient", http.Dir(...))`, `router.go`).
- **Configured multi-DB:** GORM with SQLite/MySQL/PostgreSQL drivers, no `.Raw()` ORM
  bypass (the migration/repair path does use raw `Exec` statements);
  SQLite is locally exercised, cross-DB coverage as in [PLAN.md §2](PLAN.md#2-architecture).
- **OAuth/LDAP:** configured via admin panel → DB. Falls back to local users.
- **Server commands:** allowlist in `serverCmd.go`.

## Regression-prevention

- Changing Go routes? Check admin-ui and PC client API.
- Changing GORM models? Check migration on all 3 DB types.
- Changing JWT? Verify Rust still validates tokens.
- Changing admin-ui? `npm run build` must pass.
- Changing Docker? s6-overlay must start all services.
- Adding env var? Update `.env.example` + docker-compose.

## New upstream version workflow

See [PLAN.md §7](PLAN.md#7-fork-maintenance-procedure-for-a-new-upstream-rustdesk-client-release)
for the maintenance procedure and [docs/FORKS-AND-PINS.md](docs/FORKS-AND-PINS.md) for
fork topology, sync rules, and pin bookkeeping.
