# PLAN.md — full_Server: Единый источник правды

> **Этот файл — единственный авторитетный план проекта.**
> Другие агенты: читайте его первым. Если находите другой `*.md` с планом сборки,
> противоречащим этому файлу — он устарел, сверяйтесь только с PLAN.md.
> Связанный файл: [CHANGELOG.md](CHANGELOG.md) — хронология изменений.
>
> Последнее обновление: 2026-06-11.

---

## 0. Цель проекта (зачем всё это)

Самостоятельный («суверенный») RustDesk-сервер + веб-админка + **сборщик кастомных
клиентов**, который продолжит работать, **даже если upstream `rustdesk/rustdesk` и
`rustdesk-org/*` закроют или удалят целиком**. Конечная ценность — сохранить
последнюю свободную воспроизводимую версию клиента и уметь собирать из неё
брендированный quick-support бинарник одним файлом (`rustqs.exe`), у которого
сервер, ключ и постоянный пароль уже вшиты внутрь.

Дополнительно: проект публикуется на GitHub в открытом доступе, и **любой может
форкнуть его, указать своё репо-источник и собирать клиент через свой GUI**, не
завися от оригинального rustdesk.

---

## 1. Две независимости (НЕ путать) + три уровня суверенности

Главное различие, определяющее стратегию:

- **Независимость от rustdesk upstream** (код, зависимости, engine) — **РЕАЛЬНЫЙ риск**,
  ради него весь проект. Закрывается СЕЙЧАС: форк + vendor + offline-кит.
- **Независимость от GitHub-платформы** (как раннера сборки) — **низкий риск**
  (GitHub в обозримом будущем не закроется). Реализуется, но НЕ приоритет.

**Решение владельца (2026-06-11): GitHub-first.** Собираем `rustqs.exe` через GitHub
Actions в форке rustdesk (быстро, бесплатные Windows-раннеры). Standalone Windows-билдер
полностью подготовлен, но **заморожен как fallback** — активируется, только если GitHub
станет неприемлем. При этом GitHub-путь делается **суверенным от rustdesk** (см. §8.8):
сборка идёт из форка, артефакты — из releases форка, не с rustdesk.com.

| Уровень | Независимость от | Чем достигается | Статус |
|---|---|---|---|
| **L1. Исходники** | удаления `rustdesk/rustdesk` и `rustdesk-org/*` | форк + `cargo vendor` | ✅ заморожено, L1 проверен offline |
| **L2. Сборка** | GitHub Actions / Windows-раннеров | standalone docker/нативный агент | 🧊 подготовлено, заморожено как fallback |
| **L3. Тулчейн** | исчезновения vcpkg/Flutter/Rust | offline-кит с пинами | ✅ заморожено (5.0G, 11 артефактов) |

Зависимости: **vendor + форк одновременно** — `vendor/` рабочий механизм, форки
репозиториев как архивная страховка.

---

## 2. Карта репозиториев: что форкать

```
ВАША ОРГАНИЗАЦИЯ на GitHub
│
├── full_Server                      ← основной проект (этот репозиторий)
│     └── admin-ui, api, server, docker, rdgen...
│
├── rustdesk            (форк rustdesk/rustdesk, пин на тег 1.4.7)
│     ├── vendor/       ← cargo vendor, закоммичен (L1, рабочий путь)
│     └── releases/     ← Flutter engine zip как release-asset
│
├── hbb_common          (форк rustdesk/hbb_common — submodule, ОБЯЗАТЕЛЕН)
│
└── rustdesk-deps/      (архивная страховка L1-backup, ~20 репо)
      └── magnum-opus, rdev, kcp-sys, rust-sciter, arboard, hwcodec,
          parity-tokio-ipc, confy, sysinfo, machine-uid, ...
```

**Факты (проверено в исходниках 1.4.7):**
- Submodule: `rustdesk/hbb_common` (без него не собирается ничего).
- ~20 git-зависимостей `rustdesk-org/*` в `Cargo.toml` / `libs/*/Cargo.toml`.
- Кастомный Flutter engine — отдельный release-asset (НЕ git), workflow подменяет им
  стандартный engine.
- `cargo vendor` втягивает submodule + все git-зависимости в `vendor/` → рабочая
  сборка обращается только к форку rustdesk, не к rustdesk-org.

---

## 3. Архитектура

### Активный путь (GitHub-first) — основной

```
admin-ui (Custom Client форма)
   │  POST /custom_build (server,key,пароль,бренд)
   ▼
Go API (full_Server)
   │  workflow_dispatch + ШИФРОВАННЫЕ inputs (пароль не в логах)
   ▼
GitHub Actions в форке rustdesk (windows-2022 раннер)
   │  build из форка; engine/драйверы/vendor — из releases форка (не rustdesk.com)
   ▼  собранный rustqs.exe → POST обратно на твой сервер (/api/save_custom_client)
Go API сохраняет бинарь → admin-ui Download
```

**Ключевое для безопасности (§8.8):** бинарь НЕ публикуется как public release —
едет на твой сервер. Inputs (server/key/пароль) шифруются (rdgen-модель), расшифровка
секретом внутри рана → на публичном форке ничего чувствительного не утекает.

### Fallback-путь (standalone) — заморожен, не активирован



```
┌─────────────────────────────────────────┐     ┌──────────────────────────────┐
│   PROD-ХОСТ (Linux)                      │     │  WINDOWS SERVER (отдельный)  │
│                                          │     │                              │
│  ┌────────────┐     ┌────────────────┐   │     │   ┌──────────────────────┐   │
│  │  server    │     │  linux-build   │   │     │   │     win-build        │   │
│  │ hbbs/hbbr  │     │ Linux/Android  │   │     │   │  Flutter Windows     │   │
│  │ + Go API   │◄───►│ client,        │   │     │   │  → rustqs.exe        │   │
│  │ + admin-ui │     │ server-forks,  │   │     │   │  servercore +        │   │
│  └─────┬──────┘     │ vendor-валид.  │   │     │   │  VS BuildTools +     │   │
│        │            └────────────────┘   │     │   │  Flutter+Rust+LLVM   │   │
│        │ job-очередь (сетевой канал)      │     │   └──────────┬───────────┘   │
│        └─────────────────────────────────┼─────┼──────────────┘               │
└──────────────────────────────────────────┘     └──────────────────────────────┘
   Docker: Linux-containers mode                  Docker: Windows-containers mode
```

Standalone win-build (на случай отказа от GitHub): ОТДЕЛЬНЫЙ Windows-сервер, НАТИВНО
(без Docker). Flutter desktop в Windows-контейнере капризничает. Канал — SMB-папка (§8.4).
**Сейчас НЕ разворачивается** — скрипты готовы (win-builder/), активируем при нужде.

| Компонент | Хост | Роль | Статус |
|---|---|---|---|
| `server` | Linux (прод) | hbbs/hbbr + Go API + admin-ui | ✅ работает |
| `linux-build` | Linux (прод) | Linux/Android клиент, server-форки, валидация vendor | ✅ собирает Linux-бинарник |
| GitHub Actions | форк rustdesk (раннер GitHub) | **Flutter Windows → rustqs.exe (АКТИВНЫЙ путь)** | 🟡 §8.8, в работе |
| `win-build` standalone | отдельный Windows Server (нативно) | Flutter Windows → rustqs.exe (FALLBACK) | 🧊 готов, не активирован |

---

## 4. Поток данных (жизненный цикл job) — FALLBACK-путь (standalone)

> Описывает standalone-агент (заморожен). Активный GitHub-поток — в §3 и §8.8.

```
1. admin-ui → форма Custom Client (platform=windows, server, key,
              постоянный пароль, имя=rustqs, бренд)
2. Go API (custom_build.go) пишет job.json:
   { platform, src_repo, src_ref, server, key, custom_txt(b64), app_name, ... }
3. job в очередь:
   - linux job  → локальный том prod-хоста → linux-build забирает
   - windows job → СЕТЕВОЙ канал → Windows-сервер → win-build забирает
4. win-build агент:
   a. git clone $src_repo @ $src_ref (или из локального bundle в offline)
   b. sed config.rs: сервер + ключ          ← L1 вшивания
   c. patch allowCustom + записать custom.txt ← L2 вшивания (quick support)
   d. sed Cargo.toml/Runner.rc: бренд rustqs  ← L3 вшивания
   e. cargo build --release (Rust lib) + flutter build windows
   f. portable-packer → один rustqs.exe
5. rustqs.exe → output/{job} → admin-ui показывает Download
```

Единственное архитектурное изменение против текущей модели — шаг 3: канал между
prod-API (Linux) и win-build (Windows). Варианты: общий сетевой том (SMB/NFS),
мини-HTTP-эндпоинт на агенте, или очередь. Текущий механизм «файл в томе»
переносится почти как есть.

---

## 5. Три слоя вшивания конфига (как получить `rustqs.exe`)

Подтверждено по коду 1.4.7 и workflow форка rdgen. **Имя файла как способ задать
сервер — это аварийный fallback, НЕ основной путь.** Основной путь:

1. **Сервер + ключ → хардкод в бинарник.** `sed` по `libs/hbb_common/src/config.rs`:
   - `RENDEZVOUS_SERVERS` (строка `rs-ny.rustdesk.com`) → ваш сервер
   - `RS_PUB_KEY` (строка `OeVuKk5nlHiXp+...`) → ваш ключ
2. **Quick-support поведение → подписанный `custom.txt`** (постоянный пароль,
   `verification-method`, скрытие connection manager). В OSS подпись проверяется
   ключом rustdesk — обходится патчем `allowCustom.py`, который **уже вендорен** в
   `rdgen/.github/patches/`.
3. **Брендинг → `rustqs`.** `sed` по `Cargo.toml`, `Runner.rc`, лангам +
   portable-packer (`libs/portable/generate.py`) заворачивает в один
   self-extracting `rustqs.exe`.

Готовый рецепт всех трёх слоёв уже есть в
[rdgen/.github/workflows/generator-windows.yml](rdgen/.github/workflows/generator-windows.yml) —
его нужно перенести на локальный Windows-билдер (он написан под windows-2022 раннер).

---

## 6. Offline-кит (фундамент L3) — ✅ ЗАМОРОЖЕН

Версии — факты из workflow тега 1.4.7. ✅ Заморожено 2026-06-11 (5.0G, 11 артефактов,
манифест с sha256 в `rustdesk-cache:/rustdesk-cache/offline-kit/artifacts/`). bundle
проверен clone-back, L1 проверен `cargo build --offline`. Дальше: залить в releases
форка (§8.8.2) как источник для GitHub-сборки + хранить как fallback для standalone.

| Артефакт | Версия/пин | Уровень |
|---|---|---|
| git bundle форка rustdesk + hbb_common | тег 1.4.7 | L1 |
| `vendor/` (cargo vendor) | по Cargo.lock | L1 |
| Rust toolchain | **1.75** | L3 |
| LLVM/Clang | **15.0.6** | L3 |
| Flutter SDK | **3.24.5** | L3 |
| Flutter engine (кастомный rustdesk) | release-asset | L3 |
| vcpkg baseline | `120deac3062162151622ca4860575a33844ba10b` | L3 |
| vcpkg downloads + binary cache | под baseline, триплет `x64-windows-static` | L3 |
| pub cache (Flutter packages) | по pubspec.lock | L3 |

---

## 7. Параметризация источника (для downstream-форкеров)

Источник сборки вынесен в параметры, чтобы downstream-форкер указал своё репо:

- **Дефолт агента:** `RUSTDESK_REPO`, `RUSTDESK_REF` в `offline-kit/versions.env` и
  параметрах `win-builder/agent.ps1`.
- **Runtime (поле в job.json + настройка в admin-ui):** `src_repo`, `src_ref` —
  переопределение на конкретную сборку (agent.ps1 уже читает их из job).

Цепочка независимости: вы запекаете образ с `RUSTDESK_REPO=github.com/ВЫ/rustdesk` →
форкер меняет ENV на свой → его GUI собирает из его форка. Оригинальный
`rustdesk/rustdesk` не участвует.

---

## 8. Дорожная карта (что делать дальше)

Порядок по приоритету. Каждый пункт детализируется отдельно перед реализацией.
**Текущий активный приоритет: §8.8 (GitHub-трек).** §8.3/§8.4 (standalone) заморожены
как fallback. §8.1/§8.2/§8.3a (offline-кит, форк-процедура, deps) — фундамент, сделан.

- [~] **8.1. Offline-freeze — СКРИПТ ГОТОВ, ТОМ УДАЛЁН.** ✅ Скрипт:
  [offline-kit/](offline-kit/), идемпотентный. ⚠️ Том `rustdesk-cache` был удалён владельцем
  2026-06-11 после успеха §8.8 (GitHub-сборка не зависит от локального кита). **Что сейчас
  на хосте:** только staging-копия 5 файлов в `offline-kit/artifacts/` (~62MB), залитых в
  GitHub release `offline-assets-1.4.7` форка (engine, usbmmidd, printer driver+adapter,
  sha256sums). **Потеряно из тома** (для standalone fallback): bundle, vendor tarball
  (2.7G), Flutter SDK win+linux, vcpkg checkout, Rust MSI, TopMost bundle. **Перезаморозить
  можно в любой момент** — `bash freeze.sh` на Linux (после хендоффа).
- [x] **8.2. Форк-процедура — ЗАДОКУМЕНТИРОВАНА.** ✅
  [offline-kit/FORK-PROCEDURE.md](offline-kit/FORK-PROCEDURE.md): уровни A (форк+vendor),
  B (бинари в releases), C (downstream-форкер), acceptance-чеклист. **Само форканье — за
  владельцем** (его GitHub-аккаунт, outward-facing; команды `gh`/`git push` в доке готовы).
- [x] **8.3. Windows-билдер standalone (FALLBACK) — СПРОЕКТИРОВАН, ЗАМОРОЖЕН.**
  🧊 Не разворачивается сейчас (GitHub-first, §8.8). Активировать при отказе от GitHub.
  ✅ Решение: нативно на Windows Server, без Docker. Написаны: [win-builder/setup.ps1](win-builder/setup.ps1)
  (тулчейн: VS BuildTools VCTools + Flutter 3.24.5 + Rust 1.75 + LLVM 15.0.6 + vcpkg +
  flutter_rust_bridge 1.80; поддержка `-KitPath` для offline), [win-builder/agent.ps1](win-builder/agent.ps1)
  (поллинг SMB-очереди + 3 слоя инъекции + `build.py --portable --hwcodec --flutter --vram`),
  [win-builder/README.md](win-builder/README.md) (развёртывание + SMB). **НЕ протестировано** —
  нет Windows-хоста у автора; места `[VERIFY]`. Контейнерный вариант (Dockerfile.build-win-native)
  отброшен и удалён. Старый MinGW `Dockerfile.build-win` оставлен до §8.7 (заброшен, §9).
- [x] **8.3a. Доп. зависимости Windows-сборки — ЗАМОРОЖЕНЫ.** ✅ Добавлена стадия
  `thirdparty` в freeze.sh: `RustDeskTempTopMostWindow` (исходники, пин 53b548a),
  `usbmmidd_v2.zip` (виртуальный дисплей), принтер-драйверы (driver+adapter+sha256sums).
  Все в offline-kit (см. MANIFEST). Сборку TopMostWindow (msbuild) встроить в entrypoint
  win-native при тесте на Windows-хосте.
- [x] **8.4. API ↔ standalone-агент канал (FALLBACK) — РЕШЁН: SMB.** 🧊 Относится к
  fallback-пути (§8.3). ✅ Прод-API не меняется; SMB делает том `rdgen-data` видимым
  Windows-агенту (Linux Samba, Windows монтирует Z:). Конфиги — в
  [win-builder/SERVER-SETUP.md](win-builder/SERVER-SETUP.md). Реализуется при развёртывании.

- [~] **8.8. GitHub-трек (АКТИВНЫЙ ПРИОРИТЕТ) — сборка rustqs.exe через форк rustdesk.**
  Модель rdgen: full_Server триггерит `workflow_dispatch` в форке rustdesk, тот собирает
  на windows-2022 раннере и шлёт бинарь обратно на сервер. ✅ Гайд написан:
  [github-build/README.md](github-build/README.md). **Важно:** rdgen-воркфлоу УЖЕ содержит
  шифрование inputs + save_custom_client → §8.8.4 почти готов из коробки.

  > **СОСТОЯНИЕ НА 2026-06-11 (для хендоффа):**
  > - `gh` установлен: `C:\Program Files\GitHub CLI\gh.exe`, аккаунт **bashrusakh**
  >   (scopes repo/workflow). PowerShell ломает `--jq` со скобками → парси JSON в PS.
  > - Форки готовы: `bashrusakh/rustdesk`, `bashrusakh/hbb_common` (публичные, тег 1.4.7).
  > - Release `offline-assets-1.4.7` в форке rustdesk: engine/usbmmidd/драйверы/sha256sums.
  > - **ЗАБЛОКИРОВАНО на 2 ответах владельца** (дал dismiss, ждём): (1) минитест vs полный
  >   pipeline; (2) доступен ли сервер снаружи для save_custom_client.
  > - **Следующие команды §8.8.3** (после ответов): создать build-ветку от тега 1.4.7 в
  >   форке; в `.gitmodules` URL `rustdesk/hbb_common` → `bashrusakh/hbb_common`; перенести
  >   `rdgen/.github/workflows/generator-windows.yml` + `rdgen/.github/patches/*` в форк;
  >   репойнт URL (таблица в github-build/README.md) на release `offline-assets-1.4.7`;
  >   секреты форка GENURL/ZIP_PASSWORD/token; trigger + дебаг.
  > - Staging ассетов на хосте: `offline-kit/artifacts/` (gitignored). Кит в томе
  >   `rustdesk-cache:/rustdesk-cache/offline-kit/artifacts/`.

  Подзадачи:
  - [x] **8.8.1. Форк rustdesk + hbb_common — ГОТОВО.** ✅ `bashrusakh/rustdesk` +
    `bashrusakh/hbb_common` (оба публичные форки upstream, теги 1.4.7/1.4.6 на месте).
    Имя hbb_common освобождено владельцем, форк переименован из `-1`.
  - [~] **8.8.2. Суверенизация воркфлоу — assets ЗАЛИТЫ.** ✅ Release
    [`offline-assets-1.4.7`](https://github.com/bashrusakh/rustdesk/releases/tag/offline-assets-1.4.7)
    в форке: engine (63M), usbmmidd, printer driver+adapter, сгенерированный sha256sums.
    Базовый URL: `…/releases/download/offline-assets-1.4.7/`. **Осталось:** submodule
    hbb_common → на форк (после переименования), репойнт URL в воркфлоу (§8.8.3), vendor.
    (Flutter SDK/Rust/vcpkg в releases НЕ заливаем — раннер GitHub ставит сам.)
  - [x] **8.8.3a. МИНИТЕСТ — 🟢 ЗЕЛЁНЫЙ.** Ветка `rustqs/min-test` от тега 1.4.7,
    воркфлоу [github-build/windows-min-test.yml](github-build/windows-min-test.yml) (точная
    копия официального `build-for-windows-flutter` + `workflow_dispatch`, engine из release
    форка). Залит на master+min-test (workflow_dispatch требует master). Ран
    [27341830418](https://github.com/bashrusakh/rustdesk/actions/runs/27341830418) ✅ за ~45
    мин (bridge ~6 мин, TopMost ~2 мин, build ~37 мин). Артефакт
    `rustdesk-min-test-windows` (32 МБ) — каталог Flutter Windows build (rustdesk.exe +
    dll + WindowInjection.dll). **Подтверждено практикой:** тулчейн зелёный из коробки,
    release `offline-assets-1.4.7` форка работает как источник engine.
    Триггер: `gh api repos/bashrusakh/rustdesk/actions/workflows/rustqs-windows-min-test.yml/dispatches -X POST -f ref=rustqs/min-test`.
  - [~] **8.8.3b. Наращивание pipeline (по одному шагу).** Хендофф на Linux отменён,
    продолжаем здесь.
    - [x] **(1) usbmmidd/printer URL → release форка.** ✅ Ран
      [27352640159](https://github.com/bashrusakh/rustdesk/actions/runs/27352640159) зелёный
      за ~42 мин. **Полная суверенизация бинарных ассетов завершена** — сборка не
      обращается ни к rustdesk.com, ни к rustdesk-org.
    - [x] **(2) L1 инъекция config.rs — ЗАКРЫТ.** ✅ noop
      [27355465888](https://github.com/bashrusakh/rustdesk/actions/runs/27355465888) ✅ real
      [27357780774](https://github.com/bashrusakh/rustdesk/actions/runs/27357780774) (server=
      `rqs-test.example.net`, key=`TestKEY...`). Sed по config.rs работает, компиляция с
      инъекцией проходит. Опциональный шаг через `workflow_dispatch` inputs.
    - [x] **(3) L3 брендинг — ЗАКРЫТ.** ✅ Ран
      [27359858171](https://github.com/bashrusakh/rustdesk/actions/runs/27359858171)
      L1+L3 combined зелёный за ~34 мин. Sed работает по всем 4 файлам.
    - [x] **(4) L2 quick-support — ЗАКРЫТ.** ✅ Ран
      [27362132331](https://github.com/bashrusakh/rustdesk/actions/runs/27362132331)
      L1+L2+L3 combined зелёный за ~32 мин. Все 4 step'а сработали: L1 inject, L2 patch
      allowCustom, L3 brand, L2 payload write custom_.txt. Артефакт скачан, проверен:
      - exe-метаданные: `ProductName/FileDescription/OriginalFilename = rustqs` ✅
      - `custom_.txt` лежит рядом с exe (32 байта = base64 от `{"password":"test123"}`) ✅
      - все нативные deps из release форка (printer_driver_adapter.dll, WindowInjection.dll,
        dylib_virtual_display.dll, drivers/, usbmmidd_v2/) ✅
      Залиты в форк остальные опциональные rdgen-патчи (про запас для будущих фич):
      hidecm, removeSetupServerTip, removeNewVersionNotif, cycle_monitor, xoffline,
      privacyScreen, allowCustom.diff — все под `.github/patches/rdgen-*`.
    - [x] **(5) Шифрование inputs — 🟢 ЗАКРЫТО.**
      - ✅ Сгенерирован 43-char ключ `WORKFLOW_PAYLOAD_KEY`, установлен в GitHub Secrets
        форка (через `gh secret set --body $secret` — БЕЗ pipe, чтобы не было trailing
        newline; первая попытка с `--body -` через pipe дала bad decrypt на раннере).
      - ✅ Воркфлоу зарефакторен: input `enc_payload`, шаг `Resolve build config`
        (decrypt openssl AES-256-CBC + PBKDF2 + jq, либо pass-through открытых inputs).
        L1/L2/L3 переписаны на env-переменные `RQS_*`, чувствительные значения скрыты
        через `::add-mask::`.
      - ✅ Прогоны (последний failed → bad decrypt от лишнего `\n` в secret; исправлено):
        - open-inputs backward-compat ✅ [27397828659](https://github.com/bashrusakh/rustdesk/actions/runs/27397828659)
        - enc_payload ✅ [27398061764](https://github.com/bashrusakh/rustdesk/actions/runs/27398061764)
      - ✅ Артефакт enc-прогона: `rustqs.exe`, `custom_.txt` декодирован = тот самый
        encrypted_test_pass, что был зашифрован на хосте. **Формат openssl AES-256-CBC +
        PBKDF2 совместим с Go-реализацией** (PBKDF2 sha256, iter=10000, salt+IV выводится
        из 48-байтного derived buffer, `Salted__` prefix + 8-byte salt + ciphertext).
    - [x] (6) save_custom_client на сервер — ✅ РАБОТАЕТ. Артефакт передаётся через
      GH token, после билда попадает в UI.

  - [x] **8.8.5. Go API интеграция — ✅ ПОЛНОСТЬЮ РАБОТАЕТ.**
    Go-компиляция проверена, билды прогонялись на GH. Вся цепочка admin-ui → Go API →
    workflow_dispatch → build → артефакт → UI замкнута и работает. Детали реализации:
    - ✅ `api/model/github_build_config.go` — модель singleton (id=1) с `Repo`,
      `WorkflowFilename`, `Branch`, `Token`, `PayloadKey` + `Safe()` view без секретов.
    - ✅ `api/service/github_build_config.go` — `Get/Save`, `GeneratePayloadKey()`,
      **`EncryptPayload()` совместимый с openssl-3 AES-256-CBC + PBKDF2 sha256 iter=10000**
      (доказано рабочим прогоном [27398061764]), `TestConnection()`, `DispatchBuild()`
      (workflow_dispatch + поллинг id рана), `RunStatus()`, `DownloadArtifact()`.
      `SetWorkflowSecret()` пока not implemented (требует libsodium sealed box).
    - ✅ `api/http/controller/admin/github_build_config.go` — `Get`, `Save`, `GenerateKey`,
      `Test`, `DispatchTest`.
    - ✅ Регистрация в `service/service.go`, `cmd/apimain.go` AutoMigrate, DatabaseVersion
      267 → **268**, `http/router/admin.go` GithubBuildConfigBind.
    - ✅ admin-ui: `api/github_build_config.js`, `views/server/github-build.vue`
      (форма + Save / Test / Generate Key / Trigger test build), маршрут `/admin/server/github-build`,
      i18n key `GithubBuildSettings`.

    Осталось (следующая итерация):
    - [x] **Склейка для windows-job — ✅ реализовано.** В `controller/admin/custom_build.go`
      `submitBuild` теперь: для `platform=windows` + настроенного GithubBuildConfig вызывает
      `tryGithubDispatch` (извлекает server/key/custom_txt из CustomJson, dispatch с
      enc_payload), запускает фоновый `pollAndDownload` (поллит RunStatus каждые 30 сек до
      90 мин, при success скачивает артефакт `rustdesk-min-test-windows.zip`, распаковывает,
      кладёт `{appname}.exe` + DLL + `custom_.txt` в `/rdgen-data/output/{id}/`, обновляет
      `CustomBuild.Status` (building→done/failed) + BuildLog). Если GithubBuildConfig не
      настроен — fallback в файл-очередь (linux/android всё ещё через standalone-агента).
    - [x] **SetWorkflowSecret** — ✅ реализовано через `golang.org/x/crypto/nacl/box`.
      `SetWorkflowSecret(c)` берёт публичный X25519 ключ репо
      (`GET /repos/.../actions/secrets/public-key`), шифрует PayloadKey через `box.SealAnonymous`,
      PUT `WORKFLOW_PAYLOAD_KEY`. Эндпоинт `/admin/github_build_config/sync_secret` + кнопка
      «Push to GitHub Secrets» на странице. PAT должен иметь scope `Secrets: read & write`
      на репо (для fine-grained PAT — раздел Repository secrets).
    - [x] **Компиляция Go** — ✅ проверена, билды прогонялись на GitHub.

    - [x] **(4-polish) Переименование exe-файла — ЗАКРЫТ.** Первая попытка
      [27392847080](https://github.com/bashrusakh/rustdesk/actions/runs/27392847080) →
      зелёный, но exe всё ещё `rustdesk.exe` (sed промахнулся путём — BINARY_NAME лежит
      в **родительском** `flutter/windows/CMakeLists.txt`, не в `runner/CMakeLists.txt`).
      Исправлено + добавлен sed на `project(rustdesk LANGUAGES CXX)`. Ран
      [27395862737](https://github.com/bashrusakh/rustdesk/actions/runs/27395862737)
      ✅ за ~28 мин. Артефакт проверен: файл = `rustqs.exe` (357,888 байт), метаданные =
      rustqs, custom_.txt рядом ✅.
    - [ ] (5) шифрование inputs (`fetch-encrypted-secrets.yml` + `ZIP_PASSWORD`);
    - [ ] (6) save_custom_client на сервер (требует доступности сервера снаружи).
  - [x] **8.8.4. Безопасность — ✅ ЗАКРЫТО.** Inputs шифруются (enc_payload, AES-256-CBC
    PBKDF2), расшифровка секретом WORKFLOW_PAYLOAD_KEY из GitHub Secrets. Ресинк ключа
    работает. Бинарь → на сервер по GH token, не public release.
  - [x] **8.8.5 (дубль — см. выше). Интеграция в Go API — ✅ РАБОТАЕТ.** Цепочка замкнута.
  - [ ] **8.8.6. Переход на prod workflow.** После завершения тестов — переключить с
    `rustqs-windows-min-test.yml` (smoke-test) на полный `generator-windows.yml`
    (msi, подпись, все артефакты). См. github-build/README.md — «слой 4».

- [ ] **8.5. Рантайм-проверка бинарника.** Smoke-тест (запуск `--version` в
  контейнере), чтобы «успешная» сборка не оказалась нерабочей.
- [~] **8.6. `.gitignore` + проверка секретов — ЧАСТИЧНО.**
  ✅ [.gitignore](.gitignore) написан (закрывает `data/`, приватный ключ `id_ed25519`,
  базы, `.env`, node_modules, build-вывод, offline-kit/artifacts, `.claude/`).
  ✅ Скан секретов: захардкоженных секретов в исходниках НЕТ; `${{ secrets.X }}` в rdgen —
  ссылки на GitHub Secrets; чувствительное (приватный ключ сервера, БД) — рантайм-файлы,
  закрыты .gitignore. Репо не под git → ключ не коммитился, история чистая.
  **Осталось (за владельцем):** `git init` + первый коммит + создание публичного репо.
- [ ] **8.7. Финальная чистка балласта** (отдельной фазой, в конце). Тестовые
  контейнеры `build-win-test*`, `rdgen-data/output/test-win-*`, дубли compose-файлов,
  экспериментальные скрипты. Чистить в конце, т.к. система сборки ещё перестраивается.

- [ ] **8.11. Полный ребрендинг (выпилить «RustDesk» из исходников) — НА БУДУЩЕЕ.**
  Текущий L3 покрывает только метаданные exe + portable launcher. Этого мало для
  настоящего бренда: остаётся «RustDesk» в About-странице, URLs, лангах, иконке,
  Windows-manifest, copyright. Подход: расширить sed-логику в `rustqs-windows-min-test.yml`
  (а не commit'ить ребрендинг в форк rustdesk — иначе теряется возможность мержить
  upstream-фиксы). **Юридически (AGPL-3.0):** сохранить файл LICENSE, копирайт-нотисы
  оригинала в файлах (можно добавлять свой строкой ниже, но не затирать), указать
  «Modified from RustDesk» в About и README, публичный форк `bashrusakh/rustdesk`
  закрывает требование раскрытия модификаций.

  Что осталось досэдить (точные пути и строки см. `rdgen/.github/workflows/generator-windows.yml`
  161-241 — там это всё уже сделано, нужно перенести):

  | Категория | Файл | Что |
  |---|---|---|
  | About-страница | `flutter/lib/desktop/pages/desktop_setting_page.dart` | `'Purslane Ltd'`, `'RustDesk'`, copyright строки |
  | URLs (Privacy/Download/Homepage) | `flutter/lib/common.dart`, `flutter/lib/desktop/pages/install_page.dart`, `flutter/lib/desktop/pages/desktop_home_page.dart`, `flutter/lib/mobile/pages/*.dart` | `https://rustdesk.com/*` → новые URLs (новые workflow inputs `url_link`, `download_link`) |
  | Lang-файлы | `src/lang/*.rs` | фразы «RustDesk» в строках типа «powered by RustDesk» (опционально — find/sed по `powered_by_me`) |
  | Иконка | `res/icon.ico` + Flutter assets | требует **upload PNG/ICO** — новый workflow input `app_icon_b64` |
  | Windows manifest | `flutter/windows/runner/runner.exe.manifest` | sed по `assemblyIdentity name=...` |
  | Copyright в Runner.rc | `flutter/windows/runner/Runner.rc` | строка `"Copyright © 2025 Purslane Ltd..."` |
  | MSI инсталлятор | `res/msi/Package/License.rtf`, `res/msi/preprocess.py` | только если будем делать MSI; для exe не нужно |
  | About: «Modified from RustDesk» | About-страница (Dart) | **обязательно по AGPL** — добавить строку через sed |

  Новые workflow inputs: `display_name` (брендовое имя для About/Runner), `url_link`,
  `download_link`, `app_icon_b64` (base64 PNG). Делать когда `app_name` + парочка
  тест-билдов докажут полную работоспособность pipeline.

- [ ] **8.12. Ребрендинг СЕРВЕРНОЙ части под DeskForge — НА БУДУЩЕЕ.**
  Делается ОДНОРАЗОВЫМ коммитом в `bashrusakh/DeskForge` (в отличие от §8.11 клиента —
  у клиента sed-на-билд, потому что upstream продолжает обновляться; у нашего сервера
  upstream — слепок rustdesk-server, обновляем редко). Состав:

  **Что можно ребрендить (свободно):**
  | Слой | Файлы | Что |
  |---|---|---|
  | Rust server | `server/src/main.rs`, `server/Cargo.toml` | log-сообщения, CLI-help, баннеры, `description`, `authors` |
  | Имена бинарей | `server/Cargo.toml` | опц. `hbbs` → `deskforge-id`, `hbbr` → `deskforge-relay` (потом обновить docker entry-points + s6-overlay) |
  | Go API | `api/...go` (log-strings, module path в комментариях), `api/conf/config.yaml` | свободно |
  | Env vars | `.env.example`, `docker-compose.yml`, `config.yaml` | `RUSTDESK_API_*` → `DESKFORGE_API_*` (breaking для существующих установок — обновить миграцию docs) |
  | Vue admin-ui | заголовки, About, лого, копирайты, i18n keys типа `"AdminPanel"` | свободно |
  | Docker | compose service name `rustdesk` → `deskforge`, image label | свободно |
  | README/документация | переписать целиком | свободно |

  **Что НЕЛЬЗЯ трогать (wire-protocol совместимость с клиентом!):**
  - `*.proto` файлы и сгенерированные protobuf-структуры
  - Имена в RendezvousMessage, HelloFromHbbs, RelayResponse и т.п.
  - Magic-bytes в handshake
  - Импорты из `hbb_common` (это shared с клиентом — менять = клиент отвалится)
  - Port numbers 21114-21118 (или менять везде: и сервер, и клиент, и docs)

  **Юридически обязательно (AGPL + MIT):**
  - LICENSE = AGPL-3.0 в корне (`server/` AGPL делает combined work AGPL).
  - `NOTICE` или раздел в `README.md`:
    ```
    Includes code from:
    - rustdesk-server (AGPL-3.0) © Purslane Ltd.
    - lejianwen/rustdesk-api (MIT) © Lejianwen
    - lejianwen/rustdesk-api-web (MIT) © Lejianwen / vue-manage-system
    ```
  - Сохранить per-file копирайт-комментарии в `server/src/*`.

  **Лицензии компонентов (на 2026-06-13):**
  | Компонент | Лицензия |
  |---|---|
  | `server/` (rustdesk/rustdesk-server) | AGPL-3.0 |
  | `api/` (lejianwen/rustdesk-api) | MIT |
  | `admin-ui/` (lejianwen/rustdesk-api-web) | MIT (база vue-manage-system MIT) |
  | `rdgen/` (bryangerlach/rdgen) | GPL-3.0 — но у нас не запущен как сервис, берём только воркфлоу-патчи |

- [~] **8.13. admin-ui UI rework — FOUNDATION В PR #3.** Цель — превратить admin-ui из
  типовой CRUD-админки в операционную консоль удалённого доступа (см. `ui-rework.md`).
  ✅ Сделано в PR #3 / ветка `ui-refract`:
  - design tokens для light/dark surface/text/border/status colors, radius, shadows,
    typography (`admin-ui/src/styles/style.scss`);
  - theme system `auto` / `light` / `dark` через `html[data-theme]`, `localStorage`
    (`theme-mode`) и Element Plus dark class sync;
  - `ConnectionPulse`, `ThemeSwitch`, `CopyableText`, `PageHeader`, `PageSection`,
    `DangerZone`, `EmptyState`, `LoadingState` как первые shared UI primitives;
  - shell refresh: sidebar/header/menu/settings на tokens, без всегда включённой tags bar;
  - mobile navigation через `el-drawer`, desktop collapse сохранён;
  - dashboard Quick Connect: `rustdesk://id`, web client `/webclient2/#/{id}`, переход к devices;
  - Devices page: постоянная колонка Status, ConnectionPulse online/offline по `last_online_time`,
    copyable ID, компактные действия Connect + More;
  - Monitoring visual pass: login history, connection history, file transfers и shared sessions
    используют общий page header/section layout;
  - Server visual pass: Server Commands, Server Config и GitHub Build settings используют
    общий page header/section layout; advanced custom commands отделены через `DangerZone`
    и требуют confirm перед `sendCmd`; terminal output получил readonly console styling,
    target hint, Copy/Clear controls и empty-output placeholder;
  - Access visual pass: Address Book entries, collections, share rules и tags используют
    общий page header/section layout; address book IDs переведены на `CopyableText`,
    широкие actions сжаты через `More` dropdown;
  - Users/Security visual pass: Users, API Tokens, OAuth providers, Groups и Device Groups
    используют общий page header/section layout; широкие user actions сжаты через `More` dropdown;
  - Client Builder/Profile visual pass: Custom Client Builder и My Profile используют общий
    page header/section layout; build history pagination выровнен;
  - My Workspace visual pass: My Devices, My Address Book, My Address Book Collections,
    My Tags, My Shared Sessions и My Login History используют общий page header/section layout;
  - 404 refresh: standalone empty-state экран с возвратом на dashboard;
  - Custom Client runtime fix: preset/upload handlers возвращаются из `setup()` и доступны template;
  - login/register/OAuth approve/OAuth bind переведены на token-based auth layout;
  - `ocr review`: high/medium findings нет; low nit исправлен;
  - `npm run build` проходит.
  - Monitoring filter pass: Login History, Connection History, File Transfer History и Shared Sessions получили `FilterBar` primitive.
  - DataTable pass: `admin-ui/src/components/ui/DataTable.vue` added; Users page migrated to DataTable with slot-based custom cells.

  Осталось следующими фазами:
  - [ ] i18n для нового dashboard/auth hero copy;
  - [x] унификация таблиц, фильтров, пагинации, empty/loading states;
  - [x] Devices page: ConnectionPulse status, compact actions, copyable ID, web/native connect, pagination aligned via PageSection;
  - [x] Monitoring: общий page header/section готов; Login History, Connection History, File Transfer History, Shared Sessions получили FilterBar;
  - [x] Server commands: Simple/Advanced/Danger Zone + terminal output polishing готовы;
  - [x] CRUD dialogs unified with AppDialog (zero raw el-dialog in views);
  - [x] DataTable applied to ALL view pages (zero raw el-table except nested inline in fileList);
  - [x] My Profile added to user dropdown menu;
  - [x] Hardcoded colors in control.vue and login.vue replaced with CSS variables;
  - [~] Access/Security CRUD screens: address books/collections/share rules/tags, users,
        API tokens, OAuth providers, groups и device groups page primitives готовы;
        custom client/my profile/my workspace page primitives готовы; remaining form/dialog standards ещё унифицировать;
  - [x] 404 page: tokenized empty-state экран готов;
  - [ ] ручная проверка responsive UI в браузере, не только `npm run build`.
- [x] **8.9. Custom Preset — ЗАКРЫТО.**
  Расширение модели НЕ потребовалось (все поля уже в `custom_json` text-blob). Фактически
  исправил 3 бага, которые ломали бы реальный билд через GUI-форму:
  - **server_ip vs server**: форма хранила `server_ip` в custom_json, а Go-склейка
    (`tryGithubDispatch`) искала ключ `server` → сервер из формы НЕ доходил до воркфлоу.
    Добавил fallback `server_ip` → `server` в `controller/admin/custom_build.go`.
  - **custom_txt не формировался**: воркфлоу ждёт `custom_txt` (base64 JSON rdgen-настроек),
    но UI кладёт в форму `permanent_password`, `hide_cm`, `deny_lan` и т.п. отдельно. Если
    `custom_txt` явно не задан — Go теперь собирает его из этих полей через
    `buildCustomTxtFromForm()` (маппинг на rdgen-схему `password`/`verification-method`/
    `hide-connection-management`/`deny-lan-discovery`/...).
  - **Save as preset плодил дубли** при повторном сохранении с тем же именем.
    `CustomPresetService.Create` теперь делает upsert по `(user_id, name)`.
  - **UI loadPresetIntoForm fields неполный**: сохранял `app_icon_url`/`app_logo_url`/
    `privacy_screen_url`, но не восстанавливал — добавил в список.
- [x] **8.10. Single-binary rustqs.exe — 🟢 ЗАКРЫТО.**
  Воркфлоу перестроен: (a) откатил sed BINARY_NAME в L3 (packer hard-coded ищет
  `rustdesk.exe`); (b) убрал `mv Release ./rustdesk` — нативные deps и TopMost теперь
  скачиваются в `flutter/build/windows/x64/runner/Release/`; (c) L2-B теперь кладёт
  `custom_.txt` в `Release/` (→ запакуется ВНУТРЬ single-exe); (d) новый шаг `L4 portable-pack`
  запускает `libs/portable/generate.py` (он сам делает `cargo build --locked --release`
  для packer'a → `target/release/rustdesk-portable-packer.exe`); (e) копирует в
  `./output/{appname}.exe`, артефакт upload идёт из `./output`.
  Run [27462227115](https://github.com/bashrusakh/rustdesk/actions/runs/27462227115) ✅
  за ~33 мин. Артефакт: **ОДИН файл `rustqs.exe`, 23.2 MB** (vs прежние 350 KB launcher +
  папка). Метаданные = rustqs, custom_.txt запакован внутрь.
  ⚠️ Первая попытка [27462157839](https://github.com/bashrusakh/rustdesk/actions/runs/27462157839)
  упала на Resolve build config с `bad decrypt` — `WORKFLOW_PAYLOAD_KEY` в форке
  разошёлся с локальным `offline-kit/artifacts/workflow-payload.key`. Прогнали через
  debug open-inputs (валидно для проверки §8.10). **Доп. задача:** ресинк ключа (либо
  локально подменить, либо `Push to GitHub Secrets` из UI).
  Текущий билд `rustqs-windows-min-test.yml` собирает с `--skip-portable-pack` — в итоге
  в архиве папка с DLL + маленький launcher 350 KB, а не single-exe как у upstream.
  Симптом: `rustqs.exe` молча выходит без ошибок, оригинальный rustdesk.exe стартует ок.
  Причина: launcher не находит/не может загрузить librustdesk.dll (35 MB), либо
  VC++/manifest не подхватывается вне portable-обёртки.
  - [ ] Убрать `--skip-portable-pack` из `rustqs-windows-min-test.yml` в форке
        `bashrusakh/rustdesk` (ветка `rustqs/min-test`). `build.py` через
        `libs/portable/generate.py` собирает single self-extracting exe.
  - [ ] Определить путь выхода single-exe (build.py / libs/portable/generate.py).
  - [ ] Обновить `Build rustdesk` step в воркфлоу: переместить итоговый exe в `./rustdesk/`
        перед upload, чтобы артефакт `rustdesk-min-test-windows` содержал именно single-exe.
  - [ ] Проверить что Go-экстрактор в `custom_build.go::DownloadByKey` корректно
        зипует single-exe (сейчас зипует всю папку — ок, но проверить что в архиве только exe).
  - [ ] Сделать push в форк, триггернуть новый build, скачать, запустить на Windows —
        должен стартануть как обычное desktop-приложение.
  - [ ] **Дополнительно** (если custom_.txt нужен в single-binary): portable-packer
        должен уметь класть `custom_.txt` рядом с exe ДО упаковки. Уточнить в
        `libs/portable/generate.py` — поддерживает ли он external files.

---

## 9. Заброшенные подходы (НЕ повторять)

> Эти пути проверены и отвергнуты. Запись здесь — чтобы будущий агент не пошёл по
> ним заново.

- **❌ Кросс-компиляция Windows Flutter-клиента из Linux (MinGW).** Тупик. Flutter
  Windows desktop кросс-компилировать из Linux нельзя — нужен Windows-хост с MSVC.
  Linux/MinGW-путь в лучшем случае даёт легаси **Sciter** UI, а не актуальный Flutter.
- **❌ Костыли вокруг битого vcpkg `libvpx.a` в MinGW-сборке.** Прошлые агенты два дня
  обходили следствия: vcpkg собрал `libvpx.a` хостовым linux-gcc (внутри ELF-объекты
  вместо Windows COFF) → нелинкуемо в PE. Наслоённые «фиксы» (`--whole-archive`,
  заглушки `vpx_compat.c`) лечили симптомы; заглушки сделали бы декодер видео мёртвым
  в рантайме. Корень — формат объектов, а не порядок линковки.
- **❌ `x86_64-pc-windows-gnu` как целевой target для финального клиента.** Upstream
  идёт через `x86_64-pc-windows-msvc`; gnu-target в их CI закомментирован.
- **❌ Задание сервера через имя exe** как основной механизм — это fallback. Основной
  путь — хардкод в `config.rs` (§5).

**Текущие тестовые контейнеры** (`build-win-test6/7/8/14`, `upbeat_carson`,
`lucid_grothendieck`) — это следы заброшенного MinGW-пути. Удалить в фазе 8.7.

---

## 10. Completed (исторический лог — что реально построено)

> Сохранено как запись о том, что уже работает. Детали по фазам — в git-истории и
> CHANGELOG.md. Архитектура сборки Windows-клиента ниже частично устарела
> (см. §9), серверная часть и admin-ui актуальны.

### Серверная часть и админка (актуально)
- Docker multi-stage build: Rust (hbbs+hbbr) + Go API + Node admin-ui + s6-overlay. ✅
- `server` контейнер healthy, порты 21114-21118. ✅
- admin-ui форкнут из `lejianwen/rustdesk-api-web`, англ. по умолчанию, навигация
  перестроена (Dashboard, Devices, Users, Groups, Address Book, Security, Monitoring,
  Custom Client, Server, My Profile). ✅
- admin-ui UI rework foundation: design tokens, `auto/light/dark` theme mode,
  `ConnectionPulse`, `ThemeSwitch`, refreshed shell/sidebar/header/menu/settings,
  dashboard Quick Connect, token-based login/register/OAuth screens, mobile drawer nav,
  devices/monitoring/server/access/security/client-builder/profile/my-workspace visual passes,
  refreshed 404 and shared empty/loading primitives. ✅ PR #3.
- Dashboard API+UI, Server Config UI, `GET /api/admin/config/all`. ✅
- Custom Client UI (форма + история), Presets CRUD, Logo/Icon upload. ✅
- Go API: модели/сервисы/контроллеры CustomBuild + CustomPreset, AutoMigrate,
  DatabaseVersion 265→267. ✅
- Модуль переименован `github.com/lejianwen/rustdesk-api/v2` → `rustdesk-server/api`. ✅
- Удалены внешние URL (update check, rendezvous, STUN, Firebase, CDN), весь китайский
  текст. ✅

### Сборка клиента (частично устарело — см. §9)
- `linux-build` агент собирает Linux-бинарник `rustdesk` (~32 МБ), feature
  `linux-pkg-config`. ✅ актуально.
- `win-build` MinGW-агент — доведён до этапа финальной линковки, но упёрся в битый
  `libvpx.a`. ❌ путь заброшен, заменяется на Windows-нативный билдер (§3, §8.3).

---

## 11. Известные факты-ориентиры (быстрая справка)

- Механизм `custom.json` в `flutter/lib/` (как пишет текущий entrypoint) **не
  читается** кодом 1.4.7 — это no-op. Реальный механизм — `custom.txt` + `config.rs`.
- `read_custom_client` в `src/common.rs` проверяет подпись `custom.txt` ключом
  rustdesk; патч `allowCustom.py` (в `rdgen/.github/patches/`) снимает проверку.
- Сервер форкается из имени файла через `src/custom_server.rs` (парсит `host=`,
  `key=` из имени exe) — это и есть fallback-механизм.
- Доступные патчи в `rdgen/.github/patches/`: allowCustom, hidecm,
  removeSetupServerTip, removeNewVersionNotif, cycle_monitor, xoffline,
  privacyScreen, flutter_3.24.4_dropdown_menu_enableFilter.
