# DeskForge API

Go REST API сервер — часть [DeskForge](../README.md).
Встраивает admin-ui dist (Vue 3) и web client. Gin + GORM, порт 21114.

## Стек

Go 1.23 · Gin 1.9 · GORM 1.25 · JWT · LDAP · OIDC · Swag

## Базы данных

SQLite / MySQL / PostgreSQL — `RUSTDESK_API_GORM_TYPE`.

## Ключевые эндпоинты

| Path                           | Назначение                                    |
| ------------------------------ | --------------------------------------------- |
| `/admin/`                        | Admin UI (SPA)                                |
| `/admin/api/*`                   | REST API для админки (admin-only)             |
| `/api/*`                         | PC client API (login, address book, peer)     |
| `/admin/swagger/*`               | Swagger документация                          |
| `/webclient/`                    | Web client                                    |

## CLI

```bash
./apimain reset-admin-pwd <password>   # сброс пароля админа
./apimain -h
```

## Архитектура

- **Controller** (`http/controller/`) — парсинг запроса → вызов service → ответ
- **Service** (`service/`) — бизнес-логика, валидация
- **Model** (`model/`) — GORM схемы
- **Lib** (`lib/`) — cache, JWT, ORM, logger, lock, upload

Подробнее: [AGENTS.md](../AGENTS.md).

## Быстрый старт

```bash
cd api
go build -o release/apimain cmd/apimain.go
go vet ./...
go test ./...
```
