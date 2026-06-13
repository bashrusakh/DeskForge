# FORK-PROCEDURE — суверенный форк (PLAN.md §8.2)

Как превратить замороженный [offline-kit](README.md) в **вечный самодостаточный форк**,
из которого можно собирать клиент, даже если upstream закроют. И как downstream-форкеры
повторяют это со своим репо.

> Все команды `gh`/`git push` выполняет **владелец** (это его GitHub-аккаунт,
> outward-facing). Здесь — точная последовательность, не автоматизировано.
> Предполагается: установлен `gh` (GitHub CLI, авторизован) и заморожен offline-kit
> (артефакты в `rustdesk-cache:/rustdesk-cache/offline-kit/artifacts/`).

---

## Уровень A — Минимальная суверенность (форк + vendor)

Достаточно, чтобы пережить закрытие upstream и собирать из своего форка.

### A1. Форкнуть клиент и submodule в свою организацию

```bash
gh repo fork rustdesk/rustdesk    --org ВАША_ОРГ --fork-name rustdesk    --clone=false
gh repo fork rustdesk/hbb_common  --org ВАША_ОРГ --fork-name hbb_common  --clone=false
```

### A2. Влить замороженный vendor в форк rustdesk

`vendor/` (2.7G, все ~20 rustdesk-org/* + hbb_common) уже заморожен. Кладём его в форк,
чтобы сборка не обращалась к rustdesk-org никогда.

```bash
# достать исходники тега из bundle + распаковать vendor
git clone artifacts/rustdesk-1.4.7.bundle rustdesk-fork
cd rustdesk-fork && git remote set-url origin https://github.com/ВАША_ОРГ/rustdesk.git
git checkout 1.4.7 && git submodule update --init --recursive
tar -xf ../artifacts/vendor-1.4.7.tar.gz          # → vendor/
# направить cargo на vendored-источники:
mkdir -p .cargo
cat > .cargo/config.toml <<'EOF'
[source.crates-io]
replace-with = "vendored-sources"
[source.vendored-sources]
directory = "vendor"
EOF
git add vendor .cargo/config.toml
git commit -m "Freeze vendored deps (sovereign offline build, tag 1.4.7)"
git push origin 1.4.7    # или в ветку, напр. sovereign/1.4.7
```

> ⚠️ `vendor/` тяжёлый. Если не хочешь раздувать git-историю — вместо коммита в git
> залей `vendor-1.4.7.tar.gz` как release-asset (см. B2) и распаковывай при сборке.

### A3. Направить build-агенты на свой форк

В `offline-kit/versions.env` и в ENV образа build-win (`docker-compose.win.yml`):

```
RUSTDESK_REPO="https://github.com/ВАША_ОРГ/rustdesk.git"
RUSTDESK_REF="1.4.7"
```

Готово: сборка идёт из вашего форка, upstream не нужен.

---

## Уровень B — Полная суверенность (бинарные артефакты в releases)

Кроме исходников, Windows-сборке нужны бинарные артефакты, которые тоже могут исчезнуть.
Заливаем их в releases своего форка.

### B1. Что заливать (всё уже в offline-kit)

| Артефакт | Файл в kit | Зачем |
|---|---|---|
| Flutter engine (кастомный) | `windows-x64-release.zip` | подменяет стандартный engine |
| usbmmidd_v2 | `usbmmidd_v2.zip` | виртуальный дисплей |
| printer driver | `rustdesk_printer_driver_v4-1.4.zip` | печать |
| printer adapter | `printer_driver_adapter.zip` | печать |
| vendor (опц.) | `vendor-1.4.7.tar.gz` | если не коммитить в git |

### B2. Команды

```bash
gh release create offline-assets-1.4.7 --repo ВАША_ОРГ/rustdesk \
    --title "Offline build assets (1.4.7)" --notes "Frozen $(date +%F)" \
    artifacts/windows-x64-release.zip \
    artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip \
    artifacts/printer_driver_adapter.zip \
    artifacts/vendor-1.4.7.tar.gz
```

В build-агенте брать их из этого release (по фиксированному тегу), а не с rustdesk.com.

### B3. Архивный форк зависимостей (опционально, страховка L1)

Для двойной надёжности форкнуть исходные репо (на случай перевендоринга под новую версию):

```bash
for r in RustDeskTempTopMostWindow; do gh repo fork rustdesk-org/$r --org ВАША_ОРГ --clone=false; done
# + ~20 rustdesk-org/* из Cargo.toml (см. PLAN.md §2) при желании
```

`RustDeskTempTopMostWindow` уже заморожен исходниками: `artifacts/RustDeskTempTopMostWindow.bundle`
(пин коммита 53b548a…).

---

## Уровень C — Downstream-форкер повторяет за вами

Кто-то форкает **ваш** `full_Server` и хочет собирать из **своего** rustdesk-форка:

1. Форкает `full_Server` (этот репо) и `ВАША_ОРГ/rustdesk` → `ЕГО_ОРГ/rustdesk`.
2. Меняет в своём `full_Server`:
   ```
   RUSTDESK_REPO="https://github.com/ЕГО_ОРГ/rustdesk.git"
   ```
   (одна строка в `versions.env` + ENV в `docker-compose.win.yml`).
3. Пересобирает образ build-win → его GUI собирает из его форка.

Оригинальный `rustdesk/rustdesk` в этой цепочке не участвует. Это и есть цель §0/§7.

---

## Проверка суверенности (acceptance)

Форк «вечный», если выполнено:

- [ ] `ВАША_ОРГ/rustdesk` @ 1.4.7 с `vendor/` (или vendor в release) + `.cargo/config.toml`.
- [ ] `ВАША_ОРГ/hbb_common` форкнут (submodule).
- [ ] Бинарные артефакты в releases форка (engine, usbmmidd, printer).
- [ ] `versions.env` указывает на ваш форк.
- [ ] Тестовая сборка с `--offline` проходит без обращений к github.com/rustdesk*.
