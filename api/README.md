# DeskForge API

Go REST API server — part of [DeskForge](../README.md).
Embeds admin-ui dist (Vue 3) and web client. Gin + GORM, port 21114.

## Stack

Go 1.23 · Gin 1.9 · GORM 1.25 · JWT · LDAP · OIDC · Swag

## Databases

SQLite / MySQL / PostgreSQL — `RUSTDESK_API_GORM_TYPE`.

## Key endpoints

| Path                           | Description                                  |
| ------------------------------ | -------------------------------------------- |
| `/admin/`                        | Admin UI (SPA)                               |
| `/api/admin/*`                   | Admin REST API (admin-only)                  |
| `/api/*`                         | PC client API (login, address book, peer)    |
| `/admin/swagger/*`               | Swagger docs                                 |
| `/webclient/`                    | Web client                                   |

## CLI

```bash
./apimain reset-admin-pwd <password>   # reset admin password
./apimain -h                            # help
```

## Architecture

- **Controller** (`http/controller/`) — parse request → call service → return response
- **Service** (`service/`) — business logic, validation
- **Model** (`model/`) — GORM schema
- **Lib** (`lib/`) — cache, JWT, ORM, logger, lock, upload

Details: [AGENTS.md](../AGENTS.md).

## Quick start

```bash
cd api
go build -o release/apimain cmd/apimain.go
go vet ./...
go test ./...
```
