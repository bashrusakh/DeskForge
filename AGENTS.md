# DeskForge - AI Agent Reference

## Core purpose

DeskForge is a unified, self-hosted RustDesk server combining four services: Rust signal/relay servers (hbbs/hbbr), a Go REST API (user management, auth, address book), a Vue 3 admin panel, and a Python generator for release assets. Everything ships in a single Docker image with s6-overlay process supervision.

## Runtime architecture (IMPORTANT)

- **Rust servers** (`server/`): Two binaries — `hbbs` (ID/signaling server, TCP/UDP 21116) and `hbbr` (relay server, TCP 21117). They handle peer discovery, NAT traversal, and encrypted relay tunnels.
- **Go API** (`api/`): Gin-based HTTP server (port 21114). Serves the PC client API, admin endpoints, embedded web client, and WebSocket gateway (port 21118). Uses GORM for DB access (SQLite, MySQL, or PostgreSQL).
- **Admin UI** (`admin-ui/`): Vue 3 + Element Plus SPA. Served by the Go API at `/admin/`. Communicates via REST + WebSocket.
- **rdgen** (`rdgen/`): Django generator for RustDesk client release assets. Not a production service — run manually or via build containers.
- **Shared lib** (`libs/hbb_common`): Rust crate shared between hbbs and hbbr.

All four services run inside one Docker container via s6-overlay. The Go API embeds the admin UI dist and the web client dist.

## Tech stack (source of truth: component-level manifests)

- **Rust** (server): 2021 edition, axum 0.5, sqlx 0.6, tokio, tokio-tungstenite, sodiumoxide, openssl
- **Go** (api): 1.23, gin 1.9, gorm 1.25 (sqlite/mysql/postgres), swag for OpenAPI, cobra/viper CLI, jwt, ldap, OIDC
- **Frontend** (admin-ui): Vue 3.5, Element Plus 2.8, Vite 6, Pinia 2.2, vue-router 4, axios
- **Python** (rdgen): Django, gunicorn, pillow, pyzipper
- **Infrastructure**: Docker + s6-overlay, docker-compose, cargo build, go build, vite build

## Monorepo layout

```
├── server/               # Rust hbbs/hbbr (signal + relay servers)
│   ├── src/
│   │   ├── main.rs       # hbbs entry point
│   │   ├── hbbr.rs       # relay server entry point
│   │   ├── rendezvous_server.rs
│   │   ├── relay_server.rs
│   │   ├── database.rs
│   │   ├── jwt.rs
│   │   └── peer.rs
│   └── Cargo.toml
├── api/                  # Go REST API server
│   ├── cmd/apimain.go    # CLI entry point
│   ├── http/
│   │   ├── http.go       # Server bootstrap
│   │   ├── router/       # Route registration (admin.go, api.go)
│   │   ├── controller/   # admin/, api/, web/ handlers
│   │   ├── middleware/    # Auth, logging, CORS
│   │   ├── request/      # Request DTOs
│   │   └── response/     # Response helpers
│   ├── service/          # Business logic (user, peer, addressBook, oauth, ldap, …)
│   ├── model/            # GORM models + custom types
│   ├── lib/              # Utilities (cache, jwt, orm, logger, lock, upload)
│   ├── global/           # Global state/config
│   ├── conf/config.yaml  # Default config
│   └── go.mod
├── admin-ui/             # Vue 3 admin panel
│   ├── src/
│   │   ├── views/        # 16 page modules (user, peer, address_book, group, …)
│   │   ├── api/          # API client wrappers
│   │   ├── components/   # Shared components
│   │   ├── store/        # Pinia stores (user, app, tags, router)
│   │   ├── router/       # Vue Router config
│   │   ├── layout/       # App shell layout
│   │   ├── styles/       # SCSS
│   │   └── utils/        # Helpers (auth, request, export)
│   └── package.json
├── rdgen/                # Python Django release asset generator
│   ├── rdgen/            # Django project (settings, urls, wsgi)
│   └── rdgenerator/      # Django app (models, views, templates)
├── libs/hbb_common/      # Shared Rust library (protobuf, network utils)
├── docker/               # Dockerfiles + compose + entrypoint scripts
├── github-build/         # GitHub Actions workflow for Windows client builds
├── win-builder/          # Native Windows build agent (frozen)
├── offline-kit/          # Sovereign build kit (frozen toolchain + sources)
├── PLAN.md               # Single source of truth for project plan
├── CHANGELOG.md          # Chronological change log
└── CONTRIBUTING.md       # Branch model, workflow, license rules
```

## Documentation map

### server (Rust)

Rust signal and relay servers for the RustDesk protocol.

Key modules:
- `server/src/main.rs` — hbbs binary entry point (ID/signaling server)
- `server/src/hbbr.rs` — hbbr binary entry point (relay server)
- `server/src/rendezvous_server.rs` — Signaling, peer registration, NAT type detection
- `server/src/relay_server.rs` — Encrypted relay tunnel management
- `server/src/database.rs` — SQLite peer persistence
- `server/src/jwt.rs` — JWT token validation for API-authenticated peers
- `server/src/peer.rs` — Peer state and connection tracking

### api (Go)

Go REST API server providing user management, authentication, address book, and admin endpoints.

#### http

HTTP server bootstrap, routing, middleware, and controller layer.

- `api/http/http.go` — Server initialization and startup
- `api/http/router/` — Route registration:
  - `api/http/router/admin.go` — Admin panel routes (CRUD for users, peers, groups, OAuth, tags)
  - `api/http/router/api.go` — PC client API routes (login, address book, peer sync)
- `api/http/controller/admin/` — Admin handlers (user, peer, group, tag, oauth, audit, server command)
- `api/http/controller/api/` — Client API handlers (auth, address book, peer)
- `api/http/controller/web/` — Web client handlers
- `api/http/middleware/` — Auth, CORS, logging, rate limiting

#### service

Business logic layer.

- `api/service/user.go` — User CRUD, password hashing, role management
- `api/service/peer.go` — Peer CRUD, online status, address book sync
- `api/service/addressBook.go` — Address book management, sharing, guest links
- `api/service/oauth.go` — OAuth provider integration (GitHub, Google, OIDC)
- `api/service/ldap.go` — LDAP/AD authentication (OpenLDAP, Active Directory)
- `api/service/group.go` — Group management (shared vs regular groups)
- `api/service/tag.go` — Tag system for peer organization
- `api/service/serverCmd.go` — RustDesk server command execution
- `api/service/audit.go` — Login/connection/file-transfer log queries
- `api/service/custom_build.go` — Custom client build configuration
- `api/service/github_build_config.go` — GitHub Actions build trigger config

#### model

GORM model definitions and database schema.

- `api/model/model.go` — DB initialization, migration, admin seeding
- `api/model/user.go` — User model with roles, status, password
- `api/model/peer.go` — Peer model (device ID, name, tags, groups)
- `api/model/addressBook.go` — Address book with sharing and guest access
- `api/model/oauth.go` — OAuth provider configuration
- `api/model/custom_types/` — Custom GORM value types

#### lib

Shared utility libraries.

- `api/lib/cache/` — Multi-backend cache (memory, Redis, file)
- `api/lib/jwt/` — JWT token generation and validation
- `api/lib/orm/` — Database driver initialization (sqlite.go, mysql.go, postgresql.go)
- `api/lib/logger/` — Structured logging
- `api/lib/lock/` — Distributed locking
- `api/lib/upload/` — File upload handling

#### conf

Configuration files.

- `api/conf/config.yaml` — Default configuration (Gin, GORM, RustDesk, OAuth, JWT)
- `api/conf/admin/` — Admin panel static assets (welcome message)

### admin-ui (Vue 3)

Vue 3 + Element Plus admin dashboard for managing users, peers, address books, and server configuration.

- `admin-ui/src/main.js` — App bootstrap
- `admin-ui/src/App.vue` — Root component
- `admin-ui/src/views/` — 16 page modules:
  - `login/` — Login page
  - `index/` — Dashboard
  - `user/` — User management
  - `peer/` — Device/peer management
  - `address_book/` — Address book management
  - `group/` — Group management
  - `tag/` — Tag management
  - `oauth/` — OAuth provider config
  - `audit/` — Audit logs (login, connection, file transfer)
  - `server/` — Server control and command execution
  - `rustdesk/` — RustDesk server settings
  - `custom-client/` — Custom client build config
  - `share_record/` — Guest sharing records
  - `my/` — Profile and personal settings
  - `register/` — User registration
  - `error-page/` — Error pages
- `admin-ui/src/api/` — Axios-based API client wrappers
- `admin-ui/src/store/` — Pinia stores (user, app, tags, router)
- `admin-ui/src/components/` — Shared Vue components
- `admin-ui/src/layout/` — App shell (sidebar, header, content)
- `admin-ui/src/router/` — Vue Router with auth guards
- `admin-ui/src/styles/` — SCSS styles
- `admin-ui/src/utils/` — Helpers (auth.js, request.js, export.js)

### rdgen (Python)

Django application for generating RustDesk client release assets. Run manually or inside build containers.

- `rdgen/rdgen/` — Django project config (settings.py, urls.py, wsgi.py)
- `rdgen/rdgenerator/` — Django app with models, views, templates, forms

## Build / dev commands

### Docker (primary)

```bash
cd docker
docker compose build          # Full image build (Rust + Go + admin-ui dist + rdgen)
docker compose up -d          # Start all services
docker compose logs -f        # Follow logs
```

Dev mode:

```bash
cd docker
docker compose -f docker-compose-dev.yaml up -d
```

### Rust (server/)

```bash
cd server
cargo build --release         # Build hbbs + hbbr
cargo clippy                  # Lint
cargo test                    # Unit tests
```

### Go (api/)

```bash
cd api
go build -o release/apimain cmd/apimain.go    # Build
go vet ./...                                    # Lint
go test ./...                                   # Test

# Or use shell scripts
./build.sh          # Linux
./build.bat         # Windows

# Generate Swagger docs (optional)
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g cmd/apimain.go
```

### Admin UI (admin-ui/)

```bash
cd admin-ui
npm install
npm run dev        # Dev server (Vite)
npm run build      # Production build (outputs to dist/)
```

### rdgen (rdgen/)

```bash
cd rdgen
pip install -r requirements.txt
python manage.py runserver
```

## Runtime entry points

- **hbbs (ID server)**: `server/src/main.rs`
- **hbbr (relay server)**: `server/src/hbbr.rs`
- **Go API server**: `api/cmd/apimain.go` → `api/http/http.go`
- **Admin UI bootstrap**: `admin-ui/src/main.js`
- **rdgen Django app**: `rdgen/rdgen/wsgi.py`
- **Docker entrypoint**: `docker/entrypoint-linux.sh`

## Key integration points

### Rust ↔ Go API

- Go API reads the Rust public key from `RUSTDESK_API_KEY_FILE` (default: `/data/id_ed25519.pub`)
- Go API connects to hbbs via `RUSTDESK_API_RUSTDESK_ID_SERVER` and hbbr via `RUSTDESK_API_RUSTDESK_RELAY_SERVER`
- JWT tokens: Go API generates them, Rust server validates via `jwt.rs` when peers authenticate
- WebSocket bridge: Go API forwards real-time events to web clients via port 21118

### Admin UI ↔ Go API

- REST API: `/api/` prefix (login, user CRUD, peer CRUD, address book, groups, tags, OAuth)
- Admin API: `/admin/api/` prefix (admin-only endpoints)
- Swagger docs: `/admin/swagger/index.html`
- Auth: JWT tokens in cookies, optional OAuth (GitHub, Google, OIDC)
- WebSocket: Real-time peer status updates

### Go API ↔ Web Client

- Embedded web client served at `/webclient/` or root
- Auto-discovers API server, ID server, and key
- Guest sharing via temporary links

### Environment variables (critical)

| Variable | Purpose | Used by |
|----------|---------|---------|
| `RELAY` | Relay server address | Rust hbbr |
| `ENCRYPTED_ONLY` | Only allow encrypted connections | Rust |
| `MUST_LOGIN` | Require login before connecting | Rust |
| `RUSTDESK_API_RUSTDESK_ID_SERVER` | ID server address | Go API |
| `RUSTDESK_API_RUSTDESK_RELAY_SERVER` | Relay server address | Go API |
| `RUSTDESK_API_RUSTDESK_API_SERVER` | API server URL | Go API |
| `RUSTDESK_API_KEY_FILE` | Path to public key | Go API |
| `RUSTDESK_API_JWT_KEY` | JWT secret key | Go API + Rust |
| `RUSTDESK_API_GORM_TYPE` | Database type (sqlite/mysql/postgres) | Go API |
| `RUSTDESK_API_LANG` | UI language (en/zh-CN) | Go API + Admin UI |

## Agent constraints

- Do not modify upstream dependencies (rustdesk/rustdesk-server, lejianwen/rustdesk-api) directly — fork changes.
- Keep Docker entrypoint scripts in sync with the services they supervise.
- Do not run git/GitHub commands unless explicitly asked.
- Never log or commit secrets (JWT keys, passwords, private keys, OAuth client secrets).

## Agent code of conduct

- Prefer the smallest correct change.
- Preserve working behavior before improving structure.
- Do not add cleverness where a direct implementation is enough.
- Do not infer critical state from weak signals when a stronger source exists.
- Do not hide data loss, partial failure, or fallback behavior. Make it explicit in code.
- Finish work end-to-end: implementation, verification, and cleanup.

## Development rules

- Keep diffs tight; avoid drive-by refactors.
- Follow local precedent; inspect nearby code before introducing new patterns.
- Cross-language changes: keep Rust, Go, and Vue behavior consistent when they share contracts.
- **Go**: avoid `interface{}` abuse, prefer typed errors, use `go vet` and `errcheck`.
- **Rust**: prefer `clippy`-clean code, avoid `unwrap()` in production paths, use `?` for error propagation.
- **Vue**: prefer Composition API (`<script setup>`), use Pinia for state, Element Plus for UI components.
- **Python** (rdgen): follow Django conventions, keep it minimal — it's a generator, not a production service.
- No new dependencies unless asked.
- Never add secrets or log sensitive data.

## Architecture patterns

### Clean layered architecture (Go API)

- **Controller** (`http/controller/`): HTTP handlers — parse request, call service, return response. No business logic.
- **Service** (`service/`): Business logic — validation, authorization, data transformation. No HTTP concerns.
- **Model** (`model/`): Data structures and GORM schema. No HTTP or service logic.
- **Lib** (`lib/`): Shared utilities — cache, JWT, ORM, logging. No domain logic.
- **Router** (`http/router/`): Route registration and middleware wiring. No handler logic.

### Embedded UI pattern

- The Go API embeds the admin-ui `dist/` and serves it at `/admin/`.
- Build admin-ui first, copy dist to `api/resources/admin/`, then build Go binary.
- Docker handles this automatically in multi-stage builds.

### Multi-database support

- The Go API supports SQLite, MySQL, and PostgreSQL via GORM.
- Config-driven: `gorm.type` in config.yaml or `RUSTDESK_API_GORM_TYPE` env var.
- Database-specific drivers live in `api/lib/orm/` (sqlite.go, mysql.go, postgresql.go).
- Never use raw SQL that isn't cross-compatible; prefer GORM query builder.

### OAuth and LDAP integration

- OAuth providers (GitHub, Google, OIDC) configured via admin panel → stored in DB.
- LDAP/AD authentication with fallback to local users.
- JWT tokens issued after successful auth; used by both Go API and Rust server.

### Command execution pattern

- Admin panel can execute RustDesk server commands (`serverCmd.go`).
- Commands are validated against an allowlist in `api/service/serverCmd.go`.
- Custom commands can be added via admin panel UI.

## Regression-prevention checklist

- When changing Go API routes, verify: does the admin UI still work? Does the PC client API still work?
- When modifying GORM models, verify: does migration run cleanly on all 3 DB types?
- When changing JWT logic, verify: does the Rust server still accept tokens?
- When editing admin-ui components, verify: does the build produce correct dist/ output?
- When modifying Docker scripts, verify: does s6-overlay still start all services in order?
- When adding environment variables, verify: are they documented in README and passed through docker-compose?
- When changing peer/address-book logic, verify: does the web client still display data correctly?

## Recent changes

- See `CHANGELOG.md` for chronological history.
- Recent commits: `git log --oneline` (latest tags in `CHANGELOG.md`).
