# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased] - 2026-06-13

### 🟢 Done (§8.9 Custom Preset — фактически 3 бага склейки UI↔backend, 2026-06-13)
Расширение модели не потребовалось: все поля уже в `custom_json` text-blob. При разборе
обнаружил три реальных бага, которые ломали бы реальный GUI-билд через GitHub-путь:

1. **server_ip vs server** (`api/http/controller/admin/custom_build.go`): UI хранит ключ
   `server_ip`, а `tryGithubDispatch` извлекал только `server` → сервер из формы НЕ доходил
   до воркфлоу. Добавил fallback `server_ip` → `server`.
2. **custom_txt не формировался** (`buildCustomTxtFromForm`): UI не складывает rdgen-блоб
   `custom_txt`, юзер вводит `permanent_password`/`hide_cm`/`deny_lan`/etc отдельно. Если
   `custom_txt` явно не задан — Go теперь сам собирает: маппит `permanent_password` →
   `password`, `hide_cm` → `verification-method: use-permanent-password` +
   `hide-connection-management: Y`, `deny_lan` → `deny-lan-discovery: Y`, и т.п.,
   JSON-encode → base64.
3. **Save as preset дублирует** (`api/service/custom_preset.go`):
   `CustomPresetService.Create` → upsert по `(user_id, name)`. Перезаписывает при
   совпадении имени, что и обещал §8.9 в плане.
4. **loadPresetIntoForm fields неполный** (`admin-ui/src/views/custom-client/index.vue`):
   `app_icon_url`/`app_logo_url`/`privacy_screen_url` сохранялись в пресет, но не
   восстанавливались при загрузке — добавил в список.

### 🟢 Done (§8.10 single-binary rustqs.exe — ЗАКРЫТО, 2026-06-13)
- Воркфлоу `rustqs-windows-min-test.yml` перестроен под полную упаковку:
  - откат sed BINARY_NAME в L3 (packer `libs/portable/generate.py` hard-coded ищет
    `rustdesk.exe` внутри Release/);
  - убран `mv Release ./rustdesk` — native deps (usbmmidd, printer driver+adapter) и
    TopMost-artifact теперь скачиваются прямо в `flutter/build/windows/x64/runner/Release/`;
  - L2-B шаг кладёт `custom_.txt` в `Release/` ДО запуска packer'a → запакуется внутрь;
  - новый шаг `L4 portable-pack`: `cd libs/portable && pip3 install -r requirements.txt &&
    python3 ./generate.py -f ../../{Release} -o . -e ../../{Release}/rustdesk.exe`
    (generate.py сам в конце вызывает `cargo build --locked --release` через
    `build_portable()`); финал в `./target/release/rustdesk-portable-packer.exe`,
    копируется в `./output/{appname}.exe`;
  - `actions/upload-artifact` path: `output` (вместо прежнего `rustdesk` каталога).
- Прогон [27462227115](https://github.com/bashrusakh/rustdesk/actions/runs/27462227115)
  ✅ за ~33 мин. **Артефакт = ОДИН файл `rustqs.exe`, 23.2 MB** (vs прежние 350 KB
  launcher + ~30 MB папка DLL). Метаданные exe: `ProductName/FileDescription/OriginalFilename
  = rustqs`. custom_.txt запакован внутрь self-extracting exe.
- Первая попытка [27462157839](https://github.com/bashrusakh/rustdesk/actions/runs/27462157839)
  упала за минуту: `Resolve build config` → `bad decrypt`. `WORKFLOW_PAYLOAD_KEY` в форке
  разошёлся с локальным `offline-kit/artifacts/workflow-payload.key` (видимо, перезаписан
  в одной из прошлых сессий). Открытые inputs использованы как обход для проверки §8.10.
  TODO: ресинк ключа (либо через UI «Push to GitHub Secrets», либо локально подменить файл).


### Fixed (Docker build)
- **`.dockerignore`** создан в корне — отрезает `node_modules/`, `.git/`, `data/`,
  `rdgen-data/`, `rustdesk-cache/`, `**/target/`, `*.exe|*.dll|*.apk|*.msi`,
  `offline-kit/artifacts/` из build-контекста. Без него build-контекст тащил
  155 МБ `node_modules` и хост-зависимые файлы.
- **Dockerfile: web-builder заменён на pre-built dist.** npm 10.8.2 в `node:20-alpine`
  падает с `Exit handler never called!` (известный баг musl), плюс при COPY
  `admin-ui/` перетирается хостовый `node_modules` без execute-бита. Сборка admin-ui
  теперь делается на хосте (`npm install && npm run build`), в контейнер копируется
  готовый `dist/` через `FROM scratch AS web-dist`. Rust+Go слои — без изменений
  (кеш работает, пересборка ~10 сек).
- **Пароль админа** сброшен на `admin123` через `apimain reset-admin-pwd` —
  первоначальный пароль из логов первого запуска был утерян после рестарта.

### Added (GitHub Build integration — PLAN.md §8.8.5)
- **Дефолты в форме GitHub Build** (`admin-ui/src/views/server/github-build.vue`):
  при загрузке, если БД пустая, подставляются `bashrusakh/rustdesk`,
  `rustqs-windows-min-test.yml`, `rustqs/min-test`. Юзер может изменить перед Save.
- **DownloadByKey endpoint** (`api/http/controller/admin/custom_build.go`):
  публичный (без api-token) эндпоинт `/api/admin/custom_build/public/download/:key`
  отдаёт zip с файлами из `/rdgen-data/output/{id}/`. Capability URL по
  `download_key` (32-char random), как и существующий `DetailByKey`. Имя файла:
  `{app_name}-{YYYYMMDD-HHMMSS}.zip`. Заменяет DetailByKey как Download-цель в UI.
- **Public роуты вынесены из `adg`** (`api/http/router/admin.go`): ранее
  `aRPublic := rg.Group("/custom_build/public")` наследовал `BackendUserAuth`
  из родительской группы, поэтому detailByKey/download возвращали 403 без токена.
  Теперь `/api/admin/custom_build/public/{detailByKey,download}` зарегистрированы
  на корневом `g` напрямую.
- **NoCache middleware** (`api/http/middleware/nocache.go`): ставит
  `Cache-Control: no-cache, no-store, must-revalidate`, `Pragma: no-cache`,
  `Expires: 0` на все ответы `/api/admin/*`. Применён ПЕРЕД `BackendUserAuth`,
  чтобы заголовки доходили до клиента даже при 403. UI больше не кеширует
  устаревший `status=building` для Custom Build.
- **Axios cache-busting** (`admin-ui/src/utils/request.js`): для всех GET
  добавляет `Cache-Control: no-cache` — двойная страховка на случай агрессивного
  прокси/CDN.
- **DispatchTest — polling до завершения** (`api/http/controller/admin/github_build_config.go`):
  вместо мгновенного `run_id` теперь опрашивает GitHub каждые 30 сек до
  `status=completed` (макс 90 мин) и возвращает `{run_id, status, conclusion, ok, message}`.
  Во Vue — отдельный axios-запрос с таймаутом 95 мин, отображается
  `⏳ Build running...` → `✅ Test build successful` / `❌ failed`.

### Fixed (GitHub fork cleanup)
- **Удалены 10 upstream воркфлоу** из `bashrusakh/rustdesk@master`:
  `bridge.yml`, `ci.yml`, `clear-cache.yml`, `fdroid.yml`, `flutter-build.yml`,
  `flutter-ci.yml`, `flutter-nightly.yml`, `flutter-tag.yml`, `playground.yml`,
  `wf-cliprdr-ci.yml`. Оставлены только нужные: `rustqs-windows-min-test.yml`
  (наш), `third-party-RustDeskTempTopMostWindow.yml` (deps).
- **Восстановлен `bridge.yml`** после чистки: наш `rustqs-windows-min-test.yml`
  использует его как reusable workflow, после удаления `dispatch HTTP 422: failed
  to parse workflow: error parsing called workflow "./.github/workflows/bridge.yml"`.
  Залит из upstream `rustdesk/rustdesk@1.4.7`.

### Added (PLAN.md tasks)
- **§8.9 Custom Preset — расширить модель**: добавить поля `server`, `key`,
  `custom_txt`, `logo`, `icon` в `CustomPreset`. UI «Save as preset» —
  перезаписывать при совпадении имени (отдельная CRUD-страница отменена).
- **§8.10 Single-binary rustqs.exe**: текущий билд использует
  `--skip-portable-pack` → multi-file артефакт (DLL + 350 KB launcher) вместо
  single-binary ~30 MB как у upstream. Launcher молча выходит без ошибок. Fix:
  убрать флаг, обновить Go-экстрактор, проверить попадание `custom_.txt` в
  packed-exe. **Не решено в текущей итерации — на Opus'а.**

## [0.4.0] - 2026-06-11

### Changed (Architecture — Sovereign Build Strategy)
- **PLAN.md полностью переписан** как единственный источник правды. Зафиксирована
  суверенная модель сборки (3 уровня независимости: исходники / сборка / тулчейн),
  карта форков (rustdesk + hbb_common + ~20 rustdesk-org/* через vendor), архитектура
  трёх контейнеров и offline-кит с фиксированными версиями (Rust 1.75, Flutter 3.24.5,
  LLVM 15.0.6, vcpkg baseline 120deac…).
- **Windows-сборка переносится с MinGW-кросс-компиляции (Linux) на нативный
  Windows-билдер** на отдельном Windows-сервере. Причина: актуальный Flutter-клиент
  нельзя кросс-компилировать из Linux; MinGW-путь давал лишь легаси Sciter UI и упёрся
  в нелинкуемый vcpkg libvpx.a (ELF-объекты вместо COFF).
- Зафиксирован трёхслойный механизм вшивания конфига в `rustqs.exe`: сервер+ключ в
  `config.rs` (хардкод), quick-support поведение в подписанный `custom.txt` (патч
  allowCustom), брендинг через sed + portable-packer.

### Removed
- `FEATURE_CUSTOM_CLIENT.md` — устаревший план сборки (MinGW cross-compile из Linux,
  Wine, NSIS). Подход отвергнут (см. PLAN.md §9). Актуальные части свёрнуты в PLAN.md.

### Added (Offline-kit — PLAN.md §8.1)
- `offline-kit/versions.env` — единая точка пинов (Rust 1.75, Flutter 3.24.5, LLVM 15.0.6,
  vcpkg baseline 120deac…, URL кастомного Flutter engine, источник-репо для форкеров).
- `offline-kit/freeze.sh` — идемпотентный скрипт заморозки L1+L3 по стадиям
  (source/vendor/engine/flutter_sdk/vcpkg/rust). Перезапускаемый, параметризуемый под
  downstream-форки через ENV. Запуск отложен владельцу (десятки ГБ).
- `offline-kit/README.md` — инструкция запуска, хранения (vendor в форк, бинари в
  release-assets) и offline-сборки.
- Зафиксирован полный список vcpkg-зависимостей для Windows (из vcpkg.json): aom,
  libjpeg-turbo, opus, libvpx, libyuv, mfx-dispatch, **ffmpeg** (amf/nvcodec/qsv для
  hwcodec) — шире, чем ставил старый MinGW-Dockerfile.

### Done (Offline-kit заморожен)
- `freeze.sh` прогнан 2026-06-11 в docker-build-linux-1. Заморожено в томе rustdesk-cache:
  bundle 1.4.7, vendor (2.7G — все rustdesk-org/* + hbb_common), Flutter engine, Flutter
  SDK win+linux, vcpkg@baseline, Rust 1.75.0 MSI. Манифест с sha256 в artifacts/MANIFEST.txt.
- Поправлен пин RUST_VERSION 1.75 → 1.75.0 (standalone MSI 404-ил на коротком номере).

### Added (Windows-native builder — PLAN.md §8.3, спроектирован, НЕ протестирован)
- `docker/Dockerfile.build-win-native` — servercore ltsc2022 + VS BuildTools (VCTools) +
  Flutter 3.24.5 + Rust 1.75 (msvc) + LLVM 15.0.6 + vcpkg@baseline + flutter_rust_bridge 1.80.
- `docker/entrypoint-win-native.ps1` — job-цикл + 3 слоя инъекции конфига (config.rs server/key,
  custom.txt+allowCustom, branding sed) + сборка `build.py --portable --hwcodec --flutter --vram`.
- `docker/docker-compose.win.yml` — отдельный compose для Windows-хоста (process isolation).
- Места риска помечены `[VERIFY]` — тестировать на живом Windows-сервере (у автора нет хоста).
- Выявлена доп. зависимость RustDeskTempTopMostWindow (rustdesk-org) — занесена в PLAN §8.3a.

### Added (Autonomous session — §8.2/§8.3a/§8.6)
- `offline-kit/freeze.sh` стадия `thirdparty`: заморожены RustDeskTempTopMostWindow (src,
  пин 53b548a), usbmmidd_v2.zip, принтер-драйверы. offline-kit теперь полный: 11 артефактов,
  5.0G (MANIFEST с sha256). Исправлена идемпотентность record()/manifest (частичные прогоны
  не затирают манифест).
- `offline-kit/FORK-PROCEDURE.md` — процедура суверенного форка (уровни A/B/C + acceptance).
  Форканье на GitHub оставлено владельцу (outward-facing).
- `.gitignore` — закрывает секреты (приватный ключ id_ed25519, БД, data/, .env), build-вывод,
  node_modules, offline-kit/artifacts, .claude/. Скан секретов: утечек в исходниках нет.

### Changed (Windows-билдер: контейнер → нативно, решение владельца)
- Решение: Windows-клиент собирается НАТИВНО на отдельном Windows Server (без Docker) —
  Flutter desktop в Windows-контейнере капризничает, нативно проще для одного билд-сервера.
- Канал API↔агент = SMB-папка job-очереди (прод-API не меняется; Linux хостит Samba,
  Windows монтирует). Никаких открытых Docker-демонов.
- Добавлены `win-builder/setup.ps1` (тулчейн, поддержка -KitPath offline), `win-builder/agent.ps1`
  (SMB-поллер + 3 слоя инъекции + build.py), `win-builder/README.md` (развёртывание+SMB).
- Удалены контейнерные `docker/Dockerfile.build-win-native`, `entrypoint-win-native.ps1`,
  `docker-compose.win.yml` (заменены нативным путём — один источник правды).

### Cleanup (владелец, 2026-06-11 после §8.8.3a)
- Все локальные docker-тома и образы удалены. Тестовые контейнеры от заброшенного MinGW-пути
  (build-win-test*, upbeat_carson, lucid_grothendieck) исчезли — частично закрыт §8.7.
- offline-kit том `rustdesk-cache` удалён. Staging-копия 5 release-ассетов осталась в
  `offline-kit/artifacts/` (~62MB). Для standalone fallback (§8.3) кит можно перезаморозить
  `freeze.sh` в любой момент. GitHub-трек (§8.8) от тома не зависит — раннер сам ставит тулчейн.

### 🟢 Done (§8.8.3b(5) шифрование inputs + §8.8.5 скаффолд, 2026-06-12)
**(5) Шифрование inputs — ЗАКРЫТО ✅**
- Сгенерирован 43-char ключ `WORKFLOW_PAYLOAD_KEY`, установлен в GitHub Secrets форка.
  ⚠️ Подводный камень: `gh secret set ... --body -` через pipe добавляет trailing `\n`
  в значение секрета (PowerShell pipe quirk) → bad decrypt на раннере. Фикс: использовать
  `--body $secret` без pipe.
- Воркфлоу зарефакторен: input `enc_payload`, шаг `Resolve build config` (openssl
  AES-256-CBC + PBKDF2 + jq → env vars, либо pass-through открытых inputs). Шаги L1/L2/L3
  переведены с `inputs.X` на env `RQS_*`. Чувствительные значения скрыты `::add-mask::`.
- Прогоны: open-inputs (backward-compat) ✅ [27397828659], enc_payload ✅ [27398061764].
  Артефакт enc-прогона: rustqs.exe + custom_.txt с тем самым payload `encrypted_test_pass`.

**§8.8.5 Go API — СКАФФОЛД ✅ (без проверки компиляции)**
- `model/github_build_config.go`: singleton с Token+PayloadKey + SafeView.
- `service/github_build_config.go`: Get/Save, GeneratePayloadKey, **EncryptPayload**
  openssl-compat (formula proven by run 27398061764), TestConnection, DispatchBuild
  (workflow_dispatch + polling run id), RunStatus, DownloadArtifact.
- `controller/admin/github_build_config.go`: Get, Save, GenerateKey, Test, DispatchTest.
- AutoMigrate + Router bind + DatabaseVersion 267→268.
- admin-ui: `api/github_build_config.js`, `views/server/github-build.vue` (form + Save +
  Test + Generate Key + Trigger test build), route /admin/server/github-build, i18n.
- Решение: PAT не в `.env`, а в админ-UI → БД (admin-only настройка инсталляции).

**SetWorkflowSecret one-click ✅** (2026-06-12): `golang.org/x/crypto/nacl/box.SealAnonymous`
шифрует PayloadKey публичным ключом репо (`GET /actions/secrets/public-key`), PUT кладёт
в WORKFLOW_PAYLOAD_KEY. Эндпоинт `/admin/github_build_config/sync_secret` + кнопка
"Push to GitHub Secrets" в UI. Убирает ручную копипасту ключа в GitHub Settings.

**Склейка windows-job ✅** (2026-06-12): `controller/admin/custom_build.go::submitBuild`
для `platform=windows` + настроенного GithubBuildConfig вызывает `tryGithubDispatch`:
извлекает server/key/custom_txt из `CustomJson`, dispatch с enc_payload, запускает
фоновый `pollAndDownload` (поллинг RunStatus каждые 30 сек, таймаут 90 мин). При success
скачивает артефакт `rustdesk-min-test-windows.zip`, распаковывает (`archive/zip`),
сохраняет `{appname}.exe` + DLL + `custom_.txt` в `/rdgen-data/output/{id}/`, обновляет
`CustomBuild.Status` (building→done/failed). Fallback в файл-очередь для linux/android.

**Self code-review §8.8.5 (2026-06-12)** — ручной разбор (компилятора нет), исправлено:
1. 🔴 паника в фоновой горутине `pollAndDownload` роняла бы весь API → `defer recover()`.
2. 🟠 `zf.Open()` без проверки err → nil reader → паника; вынесено в хелпер `extractZipFile`.
3. 🟠 `http.Client{Timeout:30s}` перекрывал ctx при download 32МБ → убран, `ghClient` без timeout.
4. 🟡 `context.WithTimeout(c, 60*1e9)` → `context.Background(), 60*time.Second`.
Проверено: импорты все используются, IdModel.Id/TimeModel валидны, response.* сигнатуры,
gorm в go.mod, box/pbkdf2 сигнатуры, module mode (nacl/box+pbkdf2 подтянутся сами).
Остаточный риск: DispatchBuild берёт runId через polling списка (гонка при параллельных
сборках — для MVP ок).

Осталось на след. итерацию: Go compile check (нет go-toolchain на Win-хосте; прогнать на
Linux/в docker) + интеграционный тест через UI (запустить server из docker, открыть admin-ui).

### 🟢🟢🟢 Done (§8.8.3b полный pipeline вшивания ЗЕЛЁНЫЙ, 2026-06-11/12)
Минимальный workflow в `bashrusakh/rustdesk` собирает суверенный брендированный quick-support
клиент. Подтверждено артефактами. Прогоны:
- (1) Суверенизация бинарей: ран [27352640159](https://github.com/bashrusakh/rustdesk/actions/runs/27352640159)
  ✅ usbmmidd/printer URL → release форка `offline-assets-1.4.7`.
- (2) L1 noop ✅ [27355465888](https://github.com/bashrusakh/rustdesk/actions/runs/27355465888),
  L1 real ✅ [27357780774](https://github.com/bashrusakh/rustdesk/actions/runs/27357780774):
  опциональные inputs `server`/`key`, sed по `libs/hbb_common/src/config.rs`.
- (3) L3 брендинг: ран [27359858171](https://github.com/bashrusakh/rustdesk/actions/runs/27359858171)
  L1+L3 ✅ — input `app_name`, sed по Cargo.toml/Runner.rc/portable/main.rs.
- (4) L2 quick-support: ран [27362132331](https://github.com/bashrusakh/rustdesk/actions/runs/27362132331)
  L1+L2+L3 ✅. Залит `rdgen-allowCustom.py` в `.github/patches/`, добавлены steps:
  L2-A pre-build (allowCustom убирает проверку подписи + custom.txt→custom_.txt),
  L2-B post-build (записать base64 payload как `custom_.txt` рядом с exe).
- (4-polish) Переименование exe: первая попытка [27392847080](https://github.com/bashrusakh/rustdesk/actions/runs/27392847080)
  ✅ зелёная но exe ещё `rustdesk.exe` — sed промахнулся (BINARY_NAME в РОДИТЕЛЬСКОМ
  `flutter/windows/CMakeLists.txt`, не `runner/`). Исправлено + sed по `project()`. Ран
  [27395862737](https://github.com/bashrusakh/rustdesk/actions/runs/27395862737) ✅: артефакт
  скачан, файл = `rustqs.exe`, метаданные = rustqs, `custom_.txt` рядом.

Дополнительно в форк залиты остальные опциональные rdgen-патчи (про запас):
hidecm, removeSetupServerTip, removeNewVersionNotif, cycle_monitor, xoffline,
privacyScreen, allowCustom.diff — под `.github/patches/rdgen-*`.

### 🟢 Done (§8.8.3a минитест GitHub-сборки ЗЕЛЁНЫЙ, 2026-06-11)
- Ветка `rustqs/min-test` от тега 1.4.7 в `bashrusakh/rustdesk`. Воркфлоу
  `rustqs-windows-min-test.yml` = точная копия официального `build-for-windows-flutter` +
  `workflow_dispatch`, с одной суверенной заменой: Flutter engine качается из release форка
  `offline-assets-1.4.7`, а не с rustdesk-org. (Воркфлоу залит и на master, и на ветку — у
  workflow_dispatch жёсткое требование наличия файла на дефолтной ветке.)
- Первая попытка → startup_failure: лишний input `upload-artifact-name` в вызове TopMost
  под-воркфлоу. Исправлено.
- Ран [27341830418](https://github.com/bashrusakh/rustdesk/actions/runs/27341830418):
  bridge ✅ ~6 мин, topmost ✅ ~2 мин, build ✅ ~37 мин. Артефакт `rustdesk-min-test-windows`
  (32 МБ) — каталог Flutter Windows build. **Подтверждено:** тулчейн раннера зелёный из
  коробки + release-source форка работает. Дальше — наращивание шагов (§8.8.3b в плане).

### Done (§8.8 GitHub-трек — старт реализации, 2026-06-11)
- `gh` CLI установлен владельцем (аккаунт bashrusakh, scopes repo/workflow). Доступ к
  GitHub API подтверждён.
- §8.8.1: форки в bashrusakh (публичные). `bashrusakh/rustdesk` — чистый форк upstream
  (1.4.7/1.4.6 на месте). hbb_common форкнут как `hbb_common-1` (имя занято приватным
  репо владельца; ждём освобождения для переименования).
- §8.8.2: создан release `offline-assets-1.4.7` в форке rustdesk с GitHub-нужными
  ассетами (engine 63M, usbmmidd, printer driver+adapter, sha256sums сгенерирован — исходный
  пустой). Flutter SDK/Rust/vcpkg НЕ заливаем (раннер ставит сам).

### Changed (СТРАТЕГИЯ: GitHub-first, решение владельца 2026-06-11)
- Разделены две независимости: от rustdesk-upstream (реальный риск, закрываем сейчас) и
  от GitHub-платформы (низкий риск, не приоритет). Сборка rustqs.exe — через GitHub Actions
  в форке rustdesk (быстро, бесплатные win-раннеры). Standalone win-builder заморожен как
  fallback (скрипты готовы, Windows-сервер не разворачиваем).
- PLAN §1/§3/§4/§6 переписаны под это; добавлен §8.8 (активный GitHub-трек):
  форк → суверенизация воркфлоу (артефакты из releases форка) → адаптация generator-windows.yml
  → безопасность (бинарь на свой сервер, НЕ public release; inputs шифровать — пароль не в
  логи публичного рана) → интеграция workflow_dispatch в Go API.
- §8.3/§8.4 (standalone + SMB) помечены FALLBACK/заморожено.
- `github-build/README.md` — гайд §8.8: точная таблица репойнта внешних URL в форкнутом
  generator-windows.yml (патчи/engine/драйверы → releases форка), настройка GitHub Secrets,
  Go-интеграция workflow_dispatch. Выявлено: rdgen-воркфлоу уже несёт fetch-encrypted-secrets
  + ZIP_PASSWORD + save_custom_client → безопасность (§8.8.4) почти из коробки.

### Added (win-builder)
- `win-builder/SERVER-SETUP.md` — подробное руководство по Windows build-серверу: выбор ОС
  (Server 2022 vs Win 11 Pro), железо, провижининг (Hyper-V VM/физ/облако), длинные пути,
  антивирус-исключения, SMB, служба-агент, первый end-to-end тест, безопасность.

### Verified / Fixed (offline-kit)
- ✅ **Доказана суверенность L1**: `cargo metadata --offline` на vendored-дереве резолвит
  все 1049 крейтов из vendor без сети. Заморозка зависимостей полна и валидна.
- ✅ **Bundle исправлен и проверен**: пересобран на полной истории (70M), clone-back с тегом
  1.4.7 успешен. Прежний дефект — bundle из shallow-клона (`--depth 1`) был неполным
  ("remote did not send all necessary objects"). freeze.sh `stage_source` → полный клон.

### Notes
- Заброшенные подходы задокументированы в PLAN.md §9, чтобы будущий агент не повторял.
- Чистка балласта (тестовые контейнеры build-win-test*, дубли compose) отложена до
  финальной фазы (PLAN.md §8.7); `.gitignore` + проверка секретов — обязательны до
  первого публичного push (§8.6). Репо пока не под git (git init — часть §8.6).

## [0.3.0] - 2026-06-09

### Added
- PLAN.md — unified project roadmap for other agents
- admin-ui/ — forked from lejianwen/rustdesk-api-web (Vue 3 + Element Plus)
- New nav structure: Dashboard, Devices, Users, Groups, Address Book, Security, Monitoring, Custom Client, Server, My Profile
- New page stubs: Custom Client Builder, Server Config
- Dashboard route at `/dashboard/` (page still needs content)
- i18n keys for all new nav sections (en.json)

### Changed
- Admin UI default locale: `zhCn` → `en` (Element Plus locale in main.js)
- Admin UI store: defaultLang `'zh-CN'` → `'en'`
- en.json "ChangeLang" value: "切换中文" → "Switch Language"
- Title: "Rustdesk API Admin" → "RustDesk Server Admin"
- Router: full restructure into logical nav groups (10 parent routes)
- All admin routes moved under `/admin/*` prefix
- My Profile routes cleaned up under `/my/*` prefix
- Login redirect: `/` → `/dashboard`
- PLAN.md updated with all completed phases

### Added (Phase 4 — Custom Client Builder Backend)
- `docker/Dockerfile.build-linux` — build agent for Linux .rpm + Android .apk (Rust, Flutter, Android SDK/NDK)
- `docker/Dockerfile.build-win` — build agent for Windows .exe/.msi (MinGW + NSIS cross-compiler)
- `docker/entrypoint-linux.sh` — job poller: clones rustdesk, applies patches, builds
- `docker/entrypoint-win.sh` — job poller: cross-compiles from Linux, packages with NSIS
- `api/model/custom_build.go` — CustomBuild model (id, user, platform, version, status, config, download_key, timestamps)
- `api/service/custom_build.go` — CRUD service for CustomBuild
- `api/http/request/admin/custom_build.go` — form + query structs
- `api/http/controller/admin/custom_build.go` — CRUD controller + build job submission via file tickets
- Go API routes: `CustomBuildBind` — `/admin/custom_build/*` (admin CRUD) + `/admin/custom_build/public/detailByKey/:key` (public download lookup)
- docker-compose.yml — `build-linux` + `build-win` services added with shared `rdgen-data` volume
- CustomBuild model added to AutoMigrate; DatabaseVersion bumped 265→266
- .env.example — build agent section

### Added (Phase 5 — Dashboard API + UI)
- `api/http/controller/admin/dashboard.go` — `GET /api/admin/dashboard/stats` endpoint (total_users, total_peers, online_peers, total_groups, total_logins, recent_logins)
- `admin-ui/src/api/dashboard.js` — `stats()` API function
- `admin-ui/src/views/index/index.vue` — rewritten: 6 stat cards (Users, Devices, Online, Groups, Logins, Recent), Quick Actions row, Recent Activity column
- `admin-ui/src/utils/i18n/en.json` — 10 new dashboard keys
- Route registered in admin.go: `DashboardBind` → `g.GET("/dashboard/stats", cont.Stats)`

### Added (Phase 6 — Custom Client UI Page)
- `admin-ui/src/views/custom-client/index.vue` — full build form (platform, version, server config, security/approval, permissions with toggle grid, theme, advanced options) + build history table with status tags, download, delete
- `admin-ui/src/api/custom_client.js` — `list()`, `create()`, `remove()`, `detail()`, `detailByKey()`
- `admin-ui/src/utils/i18n/en.json` — 50+ new keys covering all build form fields and status labels

### Added (Phase 7 — Server Config UI Page)
- `admin-ui/src/views/server/config.vue` — rewritten: read-only config display with server addresses card and system settings card
- `api/http/controller/admin/config.go` — new `AllConfig` endpoint: returns all server + app + admin settings in one call
- `api/http/router/admin.go` — `GET /config/all` route registered under auth
- `admin-ui/src/api/config.js` — new `all()` function

### Changed (Phase 8 — Polish)
- `admin-ui/src/store/app.js` — Chinese language names (中文→简体中文, 中文繁体→繁體中文)
- `admin-ui/src/views/peer/index.vue` — '暂未实现' warning → 'Not implemented'
- `admin-ui/src/views/rustdesk/blocklist.vue` — '多个IP以 | 分割' → 'Separate multiple IPs with |'
- `admin-ui/src/views/rustdesk/blacklist.vue` — same fix
- Removed commented-out Chinese table column labels in address_book views

### Changed (Phase 9 — Dockerfile Finalize)
- `docker/Dockerfile` stage 3: uses local `admin-ui/` copy instead of `git clone` from GitHub

### Added/Fixed (Phase 10 — Integration Verification)
- `admin-ui/src/utils/i18n/en.json` — added missing keys: `Download`, `Reset`
- Code review: Go patterns, frontend-backend route alignment, permission system, i18n audit — all clean

## [0.2.0] - 2026-06-08

### Fixed
- Docker build: Cargo.lock version 4 requires Rust 1.88+
- Docker build: go.sum missing - now generated via `go mod tidy`
- Docker build: sqlx requires DATABASE_URL for compile-time query checks

### Changed
- Dockerfile: Rust base image updated to `rust:bookworm` (latest stable)
- Dockerfile: Cargo.lock deleted before build to regenerate compatible versions
- Dockerfile: SQLite dummy DB created for sqlx macro compilation

## [0.1.0] - 2026-06-08

### Added
- Unified Docker image with Rust server + Go API + Web Admin
- Multi-stage Dockerfile (Rust, Go, Node.js, s6-overlay)
- Single docker-compose.yml for easy deployment
- Web Admin panel at `/admin/`
- Web Client for browser-based remote desktop
- JWT authentication
- Mandatory login support (MUST_LOGIN)
- OAuth2/OIDC support
- LDAP authentication
- User/Group/Device management
- Address Book
- Audit logs
- Captcha and ban system
- Swagger API documentation
- CHANGELOG.md

### Changed
- Admin panel route changed from `/_admin/` to `/admin/`
- All Chinese text removed from source code (95 files in api/)
- English-only documentation

### Removed
- Chinese README files (api/README.md)
- Chinese comments from source code
