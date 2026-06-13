# github-build — активный путь сборки rustqs.exe через GitHub Actions (PLAN.md §8.8)

Основной (выбранный) способ собирать Windows Flutter-клиент: **GitHub Actions в форке
rustdesk**, по модели rdgen. full_Server триггерит сборку, форк собирает на бесплатном
windows-2022 раннере и шлёт бинарь обратно на твой сервер. Standalone win-builder/ —
заморожен как fallback.

> **Хорошая новость:** воркфлоу rdgen `generator-windows.yml` уже содержит всю «тяжёлую»
> логику — шифрованные секреты (`fetch-encrypted-secrets.yml`), `ZIP_PASSWORD`, заливку
> бинаря на сервер (`save_custom_client`), 3 слоя инъекции конфига. Основная работа §8.8 —
> **форкнуть, перенаправить внешние URL на свой форк и настроить секреты.** Не писать с нуля.

---

## Архитектура (повтор §3)

```
admin-ui → Go API (full_Server) → workflow_dispatch (ШИФР. inputs) →
  GitHub Actions [форк rustdesk, windows-2022] →
    build (config.rs server/key + custom.txt + branding) →
    rustqs.exe → POST /api/save_custom_client → твой сервер → admin-ui Download
```

Бинарь **не публикуется** как public release — едет на твой сервер. Поэтому публичный
форк безопасен (см. §4 ниже).

---

## §8.8.1 — Форк (за владельцем)

```bash
gh repo fork rustdesk/rustdesk   --org ВАША_ОРГ --fork-name rustdesk   --clone=false
gh repo fork rustdesk/hbb_common --org ВАША_ОРГ --fork-name hbb_common --clone=false
```
Перенаправить submodule в форке rustdesk (`.gitmodules`: `rustdesk/hbb_common` →
`ВАША_ОРГ/hbb_common`), закоммитить.

Скопировать воркфлоу-рецепт rdgen (`rdgen/.github/workflows/*` + `.github/patches/*`)
в форк rustdesk (или держать в отдельном «build»-репо — но проще в форке).

---

## §8.8.2 + §8.8.3 — Суверенизация воркфлоу (репойнт URL)

Залить артефакты из offline-кита в **releases форка** (FORK-PROCEDURE §B2), затем в
`generator-windows.yml` заменить внешние URL на свои. Точные строки (на момент 1.4.7):

| Стр. | Сейчас (внешнее) | Заменить на (форк) |
|---|---|---|
| 261,264,383,395 | `raw.githubusercontent.com/bryangerlach/rdgen/.../patches/*` | вендоренные `rdgen/.github/patches/*` (уже в репо) или raw твоего форка |
| 283 | `github.com/rustdesk/engine/releases/.../windows-x64-release.zip` | `github.com/ВАША_ОРГ/rustdesk/releases/download/offline-assets-1.4.7/windows-x64-release.zip` |
| 433 | `github.com/rustdesk-org/rdev/releases/.../usbmmidd_v2.zip` | release форка ↑ |
| 441-443 | `github.com/rustdesk/hbb_common/releases/driver/*` | release форка ↑ |

> Патчи (`allowCustom.py` и др.) **уже вендорены** в `rdgen/.github/patches/` — не тяни
> их из сети bryangerlach, копируй из репо в раннере или с raw своего форка.

Сборка из своего форка обеспечивается тем, что воркфлоу `checkout`-ит сам форк (не
upstream), а cargo берёт зависимости из `vendor/` (закоммичен/в release, FORK-PROCEDURE §A2).

---

## §8.8.4 — Безопасность (на публичном форке ОБЯЗАТЕЛЬНО)

Что уже есть в rdgen-воркфлоу (использовать, не изобретать):
- **Шифрованные inputs** — `fetch-encrypted-secrets.yml` (стр.46) + `ZIP_PASSWORD`
  (стр.96, секрет): конфиг (server/key/**пароль**) передаётся шифр-блобом, расшифровка
  внутри рана секретом из GitHub Secrets → в логи публичного рана пароль не попадает.
- **Бинарь → на сервер**, не в releases: `curl ... ${apiServer}/api/save_custom_client`
  (стр.626). Чужой не скачает бинарь с GitHub.

Настроить **GitHub Secrets** в форке: `GENURL` (URL твоего сервера), `ZIP_PASSWORD`,
токен авторизации к save_custom_client, (опц.) `SIGN_BASE_URL`/`SIGN_API_KEY` для подписи.

> Напоминание: вшитый `RS_PUB_KEY` — публичный ключ (не секрет). Секрет — постоянный
> пароль quick-support; он внутри бинаря, поэтому его получит любой, у кого есть бинарь
> (это твои support-таргеты, ожидаемо). GitHub-логи к этому отношения не имеют, если
> inputs шифрованы.

---

## §8.8.5 — Интеграция в Go API

В `api/service/custom_build.go`: для `platform=windows` вместо записи файла в очередь —
ветка «GitHub backend» (как в rdgen `views.py`):

1. Собрать конфиг (server/key/custom.txt/бренд) → зашифровать (ZIP под `ZIP_PASSWORD`).
2. `POST https://api.github.com/repos/ВАША_ОРГ/rustdesk/actions/workflows/generator-windows.yml/dispatches`
   с `ref` + `inputs` (зашифрованный блоб, app_name, uuid). Заголовок
   `Authorization: Bearer <PAT>`.
3. Поллить статус рана (`GET .../actions/runs`) → обновлять статус job в БД.
4. Принять бинарь на `/api/save_custom_client` (эндпоинт уже есть в API rdgen-модели) →
   положить в `output/{uuid}` → admin-ui показывает Download.

**PAT-токен** — только через env/secret (`.env`, не в код, не в git). Scope минимальный:
`actions:write` на конкретный репо форка.

---

## Что НЕ меняется

- 3 слоя инъекции (config.rs / custom.txt / branding) — те же, что в standalone (§5 PLAN).
- admin-ui форма Custom Client — та же; меняется только backend job (GitHub vs файл-очередь).
- offline-кит — становится источником releases форка; остаётся fallback для standalone.

## Статус подзадач

- [ ] 8.8.1 форк rustdesk+hbb_common (владелец)
- [ ] 8.8.2 залить кит в releases форка (владелец, команды в FORK-PROCEDURE §B2)
- [ ] 8.8.3 репойнт URL в generator-windows.yml (таблица выше)
- [ ] 8.8.4 настроить GitHub Secrets (механизм уже в воркфлоу)
- [ ] 8.8.5 Go API: workflow_dispatch backend (код — при наличии форка для теста)

---

## Где живут workflow-файлы (карта для агентов)

Workflow'ы существуют в **трёх местах**. Это не дублирование — у каждого своя роль.
Перед тем как редактировать любой .yml, разберись, к какому слою он относится.

| Слой | Путь | Что это | Где исполняется |
|---|---|---|---|
| 1. Апстрим-референс | `rdgen/.github/workflows/*.yml` | Вендоренная копия из `bryangerlach/rdgen`. Источник истины для логики (3 слоя инъекции, шифр-secrets, save_custom_client). **Не исполняется** в этом репо. | — (только чтение) |
| 2. Локальная staging-копия | `github-build/windows-min-test.yml` | Снимок активного воркфлоу для код-ревью и истории. **Не исполняется** на GitHub. | — (только для diff) |
| 3. Активный воркфлоу (тест) | `bashrusakh/rustdesk:rustqs/min-test/.github/workflows/rustqs-windows-min-test.yml` | Smoke-test текущего этапа §8.8: минимальный воркфлоу для проверки 3 слоёв инъекции + шифрования. Запускается через `workflow_dispatch`. Туда же — `bridge.yml` и `third-party-RustDeskTempTopMostWindow.yml`. **Временный** — будет заменён слоем 4. | GitHub Actions, форк rustdesk |
| 4. Целевой воркфлоу (prod) | `bashrusakh/rustdesk:rustqs/<prod-branch>/.github/workflows/generator-windows.yml` (планируется) | Полный rdgen-воркфлоу: msi, подпись, save_custom_client, все артефакты. На этот файл перейдём из min-test, когда схема инъекций и Go API будут стабильны. | GitHub Actions, форк rustdesk |

### Правила синхронизации

1. **Изменил логику сборки?** Меняй сначала в форке (слой 3, ветка `rustqs/min-test`).
   Локальный `github-build/windows-min-test.yml` (слой 2) обновляй вслед — это снимок,
   а не источник. Коммить локально только когда форк уже зелёный.
2. **Апстрим rdgen прислал обновление?** Слой 1 (`rdgen/.github/workflows/*`) обновляется
   как чистый vendor-pull (через `git subtree` или ручной cherry-pick). После этого
   **отдельным проходом** решай — переносить ли изменения в слой 3 (форк). Не делай это
   автоматически: воркфлоу в форке сильно упрощён (только windows, плюс 3 слоя инъекции
   на env-переменных, без сторонних эндпоинтов rdgen).
3. **Bump версии actions (`@v4 → @v7`, `setup-msbuild@v2 → @v3` и т.п.).** Применяй
   независимо в каждом слое — это локальные solutions, между слоями не переносятся
   автоматически. В форке (слой 3) держим SHA-pinned (`@<sha> # vN`) по соображениям
   безопасности публичной репы. В rdgen-vendor (слой 1) — что прислал апстрим, не правим.

### Что НЕ делать

- Не редактировать `rdgen/.github/workflows/*` руками (см. §1 правил выше).
- Не пушить локальный `github-build/windows-min-test.yml` в форк через `gh api PUT` —
  файлы расходятся по структуре (имена jobs, env-переменные); это не дроп-ин копия.
- Не путать `windows-min-test.yml` (слой 2, "оригинальный rdgen-вариант для референса")
  и `rustqs-windows-min-test.yml` (слой 3, реально исполняющийся в форке).
- Не наращивать функциональность в `rustqs-windows-min-test.yml` (слой 3) до состояния
  prod. Когда min-test выполнил свою роль (3 слоя инъекции + шифрование подтверждены),
  переход на prod = отдельный воркфлоу-файл на основе `generator-windows.yml` (слой 4),
  а не разрастание min-test. min-test остаётся как лёгкий smoke на будущее.

### Как обновить файл в форке (слой 3) без git clone

```powershell
# 1. получить sha и base64-content
gh api repos/bashrusakh/rustdesk/contents/.github/workflows/<file>.yml?ref=rustqs/min-test --jq "{sha, content}" > meta.json

# 2. декодировать, отредактировать локально
#    (decode base64 → правка → encode base64)

# 3. PUT обратно — payload БЕЗ BOM (важно: Set-Content в PS 5.1 добавит BOM, JSON не распарсится).
#    Используй [System.IO.File]::WriteAllText с UTF8Encoding($false).
gh api -X PUT repos/bashrusakh/rustdesk/contents/.github/workflows/<file>.yml --input payload.json
```

### Журнал bump'ов в форке

- **2026-06-13** — `microsoft/setup-msbuild` v2 → v3 (SHA `30375c6…`) в
  `third-party-RustDeskTempTopMostWindow.yml`. Параллельный bump в rdgen-vendor:
  [DeskForge#904e9fa](https://github.com/bashrusakh/DeskForge/commit/904e9faefe091af03da5c40bfc753358e653be69).
  Bump'ы `upload-artifact` из того же коммита **не переносились** — в форке уже стояла
  более новая v7 (SHA-pinned), а rdgen-vendor поднял только до v4.
