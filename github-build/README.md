# github-build — active `rustqs.exe` build path via GitHub Actions

Основной способ сборки Windows клиента — GitHub Actions в форке `bashrusakh/rustdesk`.
`win-builder/` — frozen fallback.

---

## Архитектура

```
admin-ui → Go API → workflow_dispatch (encrypted payload) →
  GitHub Actions [rustdesk fork, windows-2022] →
    L1 config.rs (server+key) → L2 custom_.txt (permanent password) → L3 branding →
    rustqs.exe → POST /api/save_custom_client → твой сервер → admin-ui Download
```

Бинарник НЕ публикуется в public release — только на твой сервер.
Пароль/секреты — encrypted payload, дешифруются внутри runner через GitHub Secret.

---

## Где что лежит (workflow layers)

| Layer | Path (в форке rustdesk)                                   | Роль                                          |
| ----- | --------------------------------------------------------- | --------------------------------------------- |
| 1     | `rustqs/min-test/.github/workflows/rustqs-windows-min-test.yml` | ✅ активный, smoke-test                       |
| 2     | `rustqs/min-test/.github/workflows/bridge.yml`            | reusable workflow (из upstream 1.4.7)         |
| 3     | `rustqs/min-test/.github/workflows/third-party-RustDeskTempTopMostWindow.yml` | сборка TopMost |
| 4     | `DeskForge/github-build/windows-min-test.yml`             | локальная копия для code review               |
| 5     | `DeskForge/rdgen/.github/workflows/*.yml`                 | vendored upstream rdgen reference             |

**Правило:** меняешь логику сборки → правишь в форке (layer 1), потом обновляешь локальную копию (layer 4).

---

## Security (REQUIRED для public fork)

- `enc_payload` — AES-256-CBC + PBKDF2, ключ `WORKFLOW_PAYLOAD_KEY` в GitHub Secrets форка.
- `GENURL` — твой сервер (куда слать бинарник).
- `ZIP_PASSWORD` — пароль для шифрования конфига внутри workflow.
- `RS_PUB_KEY` — это публичный ключ, не секрет.
- `SetWorkflowSecret` — кнопка в админке (`Push to GitHub Secrets`) через `nacl/box.SealAnonymous`.

---

## Если новая версия upstream

1. **Fork sync** → `git fetch upstream --tags && git push origin v1.5.0`
2. **Repoint submodule** → `.gitmodules` → `bashrusakh/hbb_common`
3. **Vendor** → `cargo vendor && git add vendor`
4. **Update workflow:** сверить upstream `build-for-windows-flutter` с `rustqs-windows-min-test.yml`
5. **Тест** → `gh workflow run rustqs-windows-min-test.yml --ref v1.5.0`

Подробно: [PLAN.md §7](../PLAN.md#7-workflow-вышла-новая-версия-upstream-rustdesk-client).

---

## Синхронизация workflow (важно!)

1. **Логику сборки меняем в форке** (layer 1), потом копируем в `github-build/` (layer 4).
2. `rdgen/.github/workflows/*` (layer 5) — vendored reference, **не редактировать руками**.
3. Bumping action versions (`@v4 → @v7`) — независимо в каждом слое. В форке — SHA-pinned.
4. Когда min-test стабилен → перейти на полный `generator-windows.yml` (msi, signing).

### Fork bump log

- 2026-06-13: `setup-msbuild` v2→v3, `upload-artifact` → SHA-pinned v7.
