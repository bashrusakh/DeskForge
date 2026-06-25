# PLAN.md — DeskForge: Single Source of Truth

> Last updated: 2026-06-25
> Related: [CHANGELOG.md](CHANGELOG.md) · [BUGS.md](BUGS.md) · [CONTRIBUTING.md](CONTRIBUTING.md)

---

## 0. Project goal

Сам себе RustDesk-сервер (hbbs/hbbr + API + админка) + **кастомный клиент**,
который работает без `rustdesk/rustdesk`, `rustdesk-org/*` и `rustdesk.com`.

**Активный путь сборки клиента** — GitHub Actions в форке rustdesk. `win-builder/`
и `linux-build` — замороженный fallback.

GitHub-first потому что:
- бесплатные Windows раннеры
- форк уже готов, min-test зелёный
- standalone требует отдельный Windows Server, не развёрнут

---

## 1. Repository map

```
bashrusakh/
├── DeskForge              ← этот репо (сервер, api, админка, docker)
├── rustdesk               ← форк rustdesk/rustdesk, tag 1.4.7 → 1.4.8
│   ├── vendor/            ← cargo vendor (L1, ~20 rustdesk-org/-зависимостей)
│   ├── .github/workflows/ ← rustqs-windows-min-test.yml (активный)
│   └── releases/          ← offline-assets-1.4.7 (engine, usbmmidd, драйверы)
├── hbb_common             ← форк rustdesk/hbb_common (сабмодуль, обязателен)
└── rustdesk-deps/         ← архив ~20 репо из rustdesk-org (L1 backup)
```

**Текущие версии:** форк на 1.4.7 (тег), workflow обновлён под 1.4.8 (chore/bump-client-1.4.8).

---

## 2. Архитектура

### Активный путь (GitHub Actions)

```
admin-ui (Custom Client form)
   ↓ POST /custom_build
Go API (DeskForge)
   ↓ workflow_dispatch + enc_payload (AES-256-CBC + PBKDF2)
GitHub Actions [rustdesk fork, windows-2022]
   ↓ L1: config.rs (server + key)
   ↓ L2: custom.txt (permanent password, allowCustom patch)
   ↓ L3: branding (rustqs, portable-packer)
   ↓ POST /api/save_custom_client (encrypted)
Go API → /rdgen-data/output/{id}/ → admin-ui Download
```

**Security:** password не публикуется — `enc_payload`, дешифруется внутри runner через
GitHub Secret `WORKFLOW_PAYLOAD_KEY`. Бинарник едет на сервер, не в public release.

### Fallback (frozen, не деплоить)

```
admin-ui → Go API → jobs/{id}.json → SMB share → standalone Windows builder
                                                   или Docker linux-build
```

Не активно: `win-builder/` не тестирован (нет Windows хоста), `build-linux` за `--profile fallback`.

---

## 3. Состояние компонентов

| Компонент                      | Статус        | Примечание                              |
| ------------------------------ | ------------- | --------------------------------------- |
| hbbs/hbbr (Rust)               | ✅ работает   | порты 21114-21118                       |
| Go API                         | ✅ работает   | users, address book, OAuth, LDAP, audit |
| Admin UI (Vue 3)               | ✅ работает   | 16 страниц, 3 локали, DataTable, FilterBar |
| GitHub build (Windows)         | ✅ active     | min-test зелёный, 3 слоя, encryption    |
| GitHub build (Linux)           | 🟡 draft      | workflow есть, CI не прогонялся         |
| GitHub build (Android)         | 🟡 draft      | workflow есть, CI не прогонялся         |
| win-builder standalone         | ❄️ frozen     | не деплоить, нет Windows хоста          |
| linux-build (Docker)           | ❄️ frozen     | ручной fallback, за `--profile fallback`  |
| offline-kit                    | ❄️ frozen     | перезаморозить при смене версии клиента |

---

## 4. 3 слоя инъекции в кастомный клиент

| Слой | Что меняем            | Механизм                                                     |
| ---- | --------------------- | ------------------------------------------------------------ |
| L1   | server + key          | `sed` в `libs/hbb_common/src/config.rs` — `RENDEZVOUS_SERVERS`, `RS_PUB_KEY` |
| L2   | quick-support пароль  | `custom_.txt` (подпись проверяется — `allowCustom.py` патч убирает проверку) |
| L3   | Branding (rustqs)     | `Cargo.toml`, `Runner.rc`, portable-packer (`libs/portable/generate.py`) |

Полный рецепт: `rdgen/.github/workflows/generator-windows.yml` (vendored reference).

---

## 5. ✅ Выполнено (ключевые вехи)

- [x] Форки rustdesk + hbb_common (1.4.7)
- [x] Offline kit: L1+L3, 11 артефактов, 5 GB (frozen)
- [x] GitHub min-test windows: зелёный, ~33 min, single-binary rustqs.exe
- [x] Go API: workflow_dispatch, poll, download, capability-URL TTL
- [x] Admin UI редизайн: design tokens, DataTable, AppDialog, FilterBar, 16 страниц
- [x] Security: encrypted-at-rest (AES-GCM), OAuth delete guard, audit, TTL
- [x] Rust server: atomic blocklist, aur-fix, JWT
- [x] Database: ~272 миграции, SQLite/MySQL/PostgreSQL

---

## 6. Open roadmap

- [ ] **Linux + Android GitHub workflows** — CI-валидация + UI выбор платформы
- [ ] **Полный rebrand клиента** — About, URL, иконки — на стороне workflow, не в форке
- [ ] **Smoke test** для бинарника (`--version`)
- [ ] **Ballast cleanup** — удалить MinGW остатки, test контейнеры

---

## 7. Workflow: вышла новая версия upstream rustdesk-client

Когда `rustdesk/rustdesk` выпустил новый тег (например 1.5.0), делать по порядку:

### 7.1. Fork sync

```bash
# В форке bashrusakh/rustdesk:
git fetch upstream --tags
git checkout v1.5.0
git push origin v1.5.0

# В bashrusakh/hbb_common:
git fetch upstream --tags
git checkout v1.5.0   # или соответствующий тег
git push origin v1.5.0
```

### 7.2. Repoint submodule

В форке rustdesk:
```bash
# .gitmodules → url = https://github.com/bashrusakh/hbb_common.git, branch = v1.5.0
git submodule sync && git submodule update --init --recursive
git add .gitmodules libs/hbb_common
git commit -m "chore: point hbb_common to v1.5.0"
git push origin v1.5.0
```

### 7.3. Обновить vendor

```bash
# На машине с Rust:
cargo vendor vendor/
git add vendor/ && git commit -m "chore: vendor deps for v1.5.0"
git push origin v1.5.0
```

Или если vendor тяжёлый — залить `vendor-1.5.0.tar.gz` как release asset.

### 7.4. Обновить offline-kit

```bash
cd DeskForge/offline-kit
# versions.env: RUSTDESK_REF=v1.5.0; проверить MSRV, Flutter, vcpkg baseline
bash freeze.sh source vendor engine
```

### 7.5. Обновить offline-assets release

```bash
# Залить engine/usbmmidd/driver в форк:
gh release create offline-assets-1.5.0 --repo bashrusakh/rustdesk \
    --title "Offline build assets (1.5.0)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

### 7.6. Адаптировать workflow

Сверить upstream `build-for-windows-flutter` с `rustqs-windows-min-test.yml`:
- Новые системные зависимости?
- Изменились флаги `build.py`?
- Изменился формат `config.rs` / `custom.txt`?

Портировать изменения в workflow форка.

### 7.7. Создать ветку сборки + запустить

```bash
git checkout -b rustqs/min-test v1.5.0
# скопировать rustqs-windows-min-test.yml в .github/workflows/
git push origin rustqs/min-test
# Запустить через админку DeskForge (Dispatch Test)
```

### 7.8. Проверить

- [ ] GitHub Actions run ✅
- [ ] Бинарь приехал на сервер
- [ ] `rustqs.exe`, ~23 MB, `custom_.txt` внутри
- [ ] Smoke test на чистой Windows

### 7.9. Обновить DeskForge reference

- [ ] `offline-kit/versions.env` — новый `RUSTDESK_REF`
- [ ] `offline-kit/FORK-PROCEDURE.md` — обновить версии в примерах
- [ ] `PLAN.md` — обновить текущий tag в §1
- [ ] `github-build/README.md` — URL патчей если изменились

---

## 8. Что такое offline-kit и offline-assets (чтобы было понятно)

| Сущность                | Что это                                              | Где хранится                         |
| ----------------------- | ---------------------------------------------------- | ------------------------------------ |
| `offline-kit/`          | Скрипты (`freeze.sh`) + конфиг (`versions.env`)      | В git, в этом репо                   |
| `offline-kit/artifacts/`| Результат freeze.sh: vendor.tar.gz, engine, SDK, MSI | Локально, **не в git**               |
| `offline-assets-{tag}`  | GitHub Release с бинарниками для CI                  | GitHub Releases форка rustdesk       |

**Зачем:** без этой страховки, если `rustdesk/rustdesk` закроется или `rustdesk.com` ляжет,
собрать кастомный клиент станет невозможно. Kit замораживает всё необходимое пока upstream жив.

---

## 9. Abandoned (do not repeat)

| Подход                      | Почему dead                                        |
| --------------------------- | -------------------------------------------------- |
| MinGW cross-compile Flutter | Flutter Windows требует MSVC, не кросскомпилится   |
| `windows-x86` target        | 32-bit не supported в 2026                         |
| standalone win-builder      | frozen — GitHub-first                              |

---

## 10. Reference facts

- `custom.json` в `flutter/lib/` — no-op, не читается кодом.
- Настоящий механизм: `custom_.txt` + `config.rs`.
- `read_custom_client` проверяет подпись — `allowCustom.py` убирает проверку.
- Патчи в `rdgen/.github/patches/`: allowCustom, hidecm, removeSetupServerTip,
  removeNewVersionNotif, cycle_monitor, xoffline, privacyScreen.
