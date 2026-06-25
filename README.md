# DeskForge

Unified self-hosted RustDesk-совместимый сервер.
Всё в одном Docker образе (s6-overlay): Rust hbbs/hbbr + Go API + Vue 3 админка + кастомный клиент.

---

## Быстрый старт

```bash
git clone https://github.com/bashrusakh/DeskForge.git
cd DeskForge/docker
# docker-compose.yml: заменить your-server, your-secret-jwt-key-change-this
docker compose up -d
# Получить public key:
docker compose logs | grep "Public Key"
```

**Admin:** `http://your-server:21114/admin/` — логин `admin`, пароль в логах.

**RustDesk client:** ID Server `your-server:21116`, Relay `:21117`, API `http://your-server:21114`, Key — из логов.

---

## Порты

| Port  | Protocol | Назначение        |
| ----- | -------- | ----------------- |
| 21114 | TCP      | API + Web Admin   |
| 21115 | TCP      | NAT type test     |
| 21116 | TCP/UDP  | ID Server (hbbs)  |
| 21117 | TCP      | Relay Server(hbbr)|
| 21118 | TCP      | WebSocket         |

---

## Ключевые env vars

| Variable                           | Назначение                          |
| ---------------------------------- | ----------------------------------- |
| `RELAY`                              | Адрес relay сервера                 |
| `ENCRYPTED_ONLY`                     | Только шифрованные соединения       |
| `MUST_LOGIN`                         | Требовать логин перед коннектом     |
| `RUSTDESK_API_RUSTDESK_ID_SERVER`    | ID сервер (hbbs)                    |
| `RUSTDESK_API_JWT_KEY`              | JWT секрет                          |
| `RUSTDESK_API_GORM_TYPE`            | sqlite / mysql / postgres           |
| `RUSTDESK_API_LANG`                 | en / ru / zh-CN                     |
| `SECRET_CRYPT_KEY`                  | Ключ шифрования secrets at rest     |

---

## Что реализовано

**Сервер (Rust + Go):** user CRUD, JWT, OAuth (GitHub/Google/OIDC), LDAP, группы, теги,
адресная книга (личная + общая с коллекциями), peer-UUID binding, audit (login/connection/file-transfer),
server команды с персистентностью и аудитом, encrypted-at-rest secrets, SQLite/MySQL/PostgreSQL,
captcha + brute-force protection.

**Админка (Vue 3):** Login, Dashboard, Devices, Users, Groups, Tags, OAuth, Server Config,
Audit, Custom Client Builder, Profile, My Workspace, Guest Sharing.
3 локали (en/ru/zh_CN). Light/Dark/Auto темы.
Shared UI: DataTable, AppDialog, AppDrawer, FilterBar, ActionsToolbar.

**Кастомный клиент:** GitHub Actions → форк rustdesk → `rustqs.exe` (Windows).
Сборка с твоим сервером, ключом, permanent password. Single-binary через portable-packer (23 MB).
Linux/Android — в разработке.

**Не реализовано (vs RustDesk Pro):** 2FA, RBAC, session recording, device policy, remote script,
HA, backup/restore.

---

## Структура репозитория

```
server/          — Rust hbbs/hbbr (signal + relay)
api/             — Go REST API (Gin + GORM)
admin-ui/        — Vue 3 + Element Plus админка
libs/hbb_common/ — общая Rust библиотека (сабмодуль)
docker/          — Dockerfile + compose + entrypoint
github-build/    — активный workflow сборки клиента
win-builder/     — ❄️ FROZEN: standalone Windows сборщик
offline-kit/     — ❄️ FROZEN: инструмент заморозки зависимостей (страховка от смерти upstream)
rdgen/           — vendored reference workflow (не сервис)
```

---

## Building

```bash
cd docker
docker compose build          # полная сборка
docker compose up -d          # запуск
```

---

## Форки (для сборки кастомного клиента)

- [`bashrusakh/rustdesk`](https://github.com/bashrusakh/rustdesk) — форк rustdesk/rustdesk @ 1.4.7
- [`bashrusakh/hbb_common`](https://github.com/bashrusakh/hbb_common) — форк rustdesk/hbb_common

Подробнее: [PLAN.md §7 — workflow новой версии](PLAN.md#7-workflow-вышла-новая-версия-upstream-rustdesk-client)

---

## Лицензия

AGPL-3.0 (сервер) + MIT (api/admin-ui). См. [LICENSE](LICENSE) и [NOTICE](NOTICE).

Проект основан на:
- [rustdesk/rustdesk-server](https://github.com/rustdesk/rustdesk-server) (AGPL-3.0)
- [lejianwen/rustdesk-api](https://github.com/lejianwen/rustdesk-api) (MIT)
