# FORK-PROCEDURE — как сделать форк суверенным

> **FROZEN** — процедура выполнена для 1.4.7/1.4.8. Ниже — reference для новой версии или
> для downstream форкера. Команды выполняет owner.

---

## Level A — fork + vendor (минимум для выживания upstream)

### A1. Форкнуть rustdesk + hbb_common

```bash
gh repo fork rustdesk/rustdesk   --org YOUR_ORG --fork-name rustdesk   --clone=false
gh repo fork rustdesk/hbb_common --org YOUR_ORG --fork-name hbb_common --clone=false
```

### A2. Vendor в форк

Из offline-kit:
```bash
git clone artifacts/rustdesk-1.4.8.bundle rustdesk-fork
cd rustdesk-fork && git remote set-url origin https://github.com/YOUR_ORG/rustdesk.git
git checkout 1.4.8 && git submodule update --init --recursive
tar -xf ../artifacts/vendor-1.4.8.tar.gz
# .cargo/config.toml → source replacement на vendor/
git add vendor .cargo/config.toml
git commit -m "chore: freeze vendored deps 1.4.8"
git push origin 1.4.8
```

`vendor/` тяжёлый — можно вместо коммита залить `vendor-{tag}.tar.gz` как release asset.

### A3. Указать форк в versions.env

```env
RUSTDESK_REPO="https://github.com/YOUR_ORG/rustdesk.git"
RUSTDESK_REF="1.4.8"
```

---

## Level B — полная суверенность (бинарники в release)

### B1. Что залить в release

Из `offline-kit/artifacts/`:

| Артефакт                        | Зачем                          |
| ------------------------------- | ------------------------------ |
| `windows-x64-release.zip`         | Flutter engine (кастомный)     |
| `usbmmidd_v2.zip`                 | Виртуальный дисплей            |
| `rustdesk_printer_driver_v4-*.zip`| Принтер                        |
| `printer_driver_adapter.zip`      | Адаптер принтера               |
| `vendor-*.tar.gz`                 | (опционально, если не в git)   |

### B2. Команда

```bash
gh release create offline-assets-1.4.8 --repo YOUR_ORG/rustdesk \
    --title "Offline build assets (1.4.8)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

### B3. Архивация зависимостей (опционально, L1 backup)

```bash
for r in RustDeskTempTopMostWindow; do
  gh repo fork rustdesk-org/$r --org YOUR_ORG --clone=false
done
```

---

## Level C — downstream форкер

Кто-то форкнул **твой** DeskForge → меняет одну строку:
```env
RUSTDESK_REPO="https://github.com/THEIR_ORG/rustdesk.git"
```
→ их GUI собирает клиент из их форка. Upstream не участвует.

---

## Проверка суверенности

- [ ] `YOUR_ORG/rustdesk` с vendor + `.cargo/config.toml`
- [ ] `YOUR_ORG/hbb_common` (сабмодуль)
- [ ] Release `offline-assets-{tag}` с бинарниками
- [ ] `versions.env` → твой форк
- [ ] `cargo build --offline` проходит без `github.com/rustdesk*`
