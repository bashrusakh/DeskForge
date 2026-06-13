# offline-kit — суверенный комплект сборки

Реализация **L1 (исходники) + L3 (тулчейн)** из [PLAN.md](../PLAN.md). Замораживает
всё, что нужно для сборки Windows Flutter-клиента в offline-режиме, **пока upstream
жив** — на случай, если `rustdesk/rustdesk` и `rustdesk-org/*` закроют.

> ⚠️ **Это самый срочный пункт плана (§8.1)** — единственный с внешним дедлайном.
> Артефакты upstream могут исчезнуть в любой день. Запусти заморозку как можно раньше.

## Файлы

| Файл | Назначение |
|---|---|
| `versions.env` | Единая точка пинов (Rust 1.75, Flutter 3.24.5, vcpkg baseline, URL-ы). Меняешь версию клиента — правишь здесь. |
| `freeze.sh` | Идемпотентный скрипт заморозки. Перезапускаемый после обрыва. |
| `artifacts/` | Результат (в git не коммитится — см. §«Хранение»). |

## Как запустить

Нужно окружение с `git` + `cargo`. Проще всего — внутри уже работающего
build-linux контейнера:

```bash
# скопировать offline-kit в контейнер и запустить
docker cp offline-kit docker-build-linux-1:/offline-kit
docker exec -it docker-build-linux-1 bash -c "cd /offline-kit && bash freeze.sh"
```

Или в WSL/Linux с установленным Rust:

```bash
cd offline-kit && bash freeze.sh
```

Отдельные стадии (можно гонять по одной, экономя время/лимиты):

```bash
bash freeze.sh source        # git clone + bundle
bash freeze.sh vendor        # cargo vendor (замораживает hbb_common + rustdesk-org/*)
bash freeze.sh engine        # кастомный Flutter engine
bash freeze.sh flutter_sdk   # Flutter SDK (win + linux)
bash freeze.sh vcpkg         # vcpkg checkout на baseline
bash freeze.sh rust          # Rust toolchain offline installer
```

Для **downstream-форка** — переопредели источник:

```bash
RUSTDESK_REPO=https://github.com/ВЫ/rustdesk.git RUSTDESK_REF=1.4.7 bash freeze.sh
```

## Что замораживается (стадии)

1. **source** — `git clone --recurse-submodules` на тег + `git bundle` (переносимый
   архив со всей историей submodule `hbb_common`).
2. **vendor** — `cargo vendor`: втягивает submodule + ~20 git-зависимостей
   `rustdesk-org/*` в `vendor/`. После этого сборка не обращается к rustdesk-org.
3. **engine** — кастомный Flutter engine rustdesk (`windows-x64-release.zip`),
   которым workflow подменяет стандартный engine.
4. **flutter_sdk** — Flutter SDK 3.24.5 (Windows + Linux архивы).
5. **vcpkg** — checkout vcpkg на baseline `120deac…`. **Binary cache (ffmpeg/hwcodec,
   триплет x64-windows-static) здесь НЕ собирается** — это тяжёлый шаг на Windows-хосте
   с MSVC, выполняется на этапе win-build (PLAN.md §8.3).
6. **rust** — offline-инсталлятор Rust 1.75 для Windows-хоста.

Каждая стадия пишет строку в `artifacts/MANIFEST.txt` с размером и sha256.

## Хранение готового кита

`artifacts/` тяжёлый (десятки ГБ) и **не коммитится в git** (см. корневой
`.gitignore`, PLAN.md §8.6). Варианты долговременного хранения:

- **vendor/** — закоммитить прямо в форк `rustdesk` (он переносит зависимости в репо).
- **Крупные бинари** (engine, Flutter SDK, vcpkg cache) — заливать как **release-assets**
  форка `rustdesk` (`gh release upload`), а не в git-историю.
- **bundle** — резервная копия исходников; хранить вне GitHub (бэкап-диск/S3).

## Offline-сборка (как использовать кит потом)

На Windows-билдере (PLAN.md §8.3), без сети:

```bash
# исходники из bundle вместо clone:
git clone artifacts/rustdesk-1.4.7.bundle rustdesk
# vendor на месте → cargo читает из него:
cargo build --release --offline --locked
# vcpkg в режиме asset cache (X_VCPKG_ASSET_SOURCES) + binary cache
```

Точные команды Windows-сборки — в следующей фазе (§8.3, Dockerfile.build-win).
