# offline-kit — инструмент суверенной заморозки

**Зачем:** если `rustdesk/rustdesk` закроется, `rustdesk-org/*` удалится, или `crates.io`/Google
станут недоступны — собрать кастомный клиент станет невозможно. Этот набор замораживает всё
необходимое **пока upstream ещё жив**.

## Что внутри

| Файл                          | Для чего                                                     |
| ----------------------------- | ------------------------------------------------------------ |
| `freeze.sh`                     | Скачивает исходники, тулчейн, зависимости                    |
| `versions.env`                  | Версии всех компонентов (Rust, Flutter, vcpkg, ...)          |
| `FORK-PROCEDURE.md`             | Инструкция: как сделать форк суверенным                      |
| `artifacts/`                    | Результат freeze.sh (5 GB, **не в git**)                        |

## Как работает схема

```
freeze.sh → offline-kit/artifacts/*  (локально, всё 5 GB)
                ↓ upload (только бинарники: engine, драйверы)
         offline-assets-{tag}        (GitHub Release в форке rustdesk, ~100 MB)
                ↓ download
         GitHub Actions runner → build rustqs.exe
```

- **`offline-kit/`** (эта директория) — **инструмент**: скрипты и конфиги для заморозки. Лёгкий, в git.
- **`offline-assets-{tag}`** — **GitHub Release** в форке `bashrusakh/rustdesk`. Туда залиты тяжёлые
  бинарники (Flutter engine, usbmmidd, драйверы), чтобы CI их скачивал не с `rustdesk.com`, а из нашего релиза.
- Остальное (vendor 2.7 GB, Flutter SDK, Rust MSI, vcpkg) лежит только локально — для standalone fallback.

## Заморозка новой версии

```bash
cd offline-kit
# Правим versions.env: RUSTDESK_REF, версии тулчейна под новый тег
bash freeze.sh source        # git clone + bundle
bash freeze.sh vendor        # cargo vendor
bash freeze.sh engine        # Flutter engine
# Остальные этапы по необходимости
```

Для downstream форка:
```bash
RUSTDESK_REPO=https://github.com/YOUR_ORG/rustdesk.git RUSTDESK_REF=1.5.0 bash freeze.sh
```

## Storage

`artifacts/` в `.gitignore` — тяжёлые файлы не коммитятся.
- `vendor/` — commit прямо в форк rustdesk или release asset.
- Бинарники (engine, usbmmidd, driver) — upload в GitHub Release форка (`offline-assets-{tag}`).
- `bundles` — backup вне GitHub.
