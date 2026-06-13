# SERVER-SETUP — развёртывание Windows build-сервера (подробно)

Полное руководство: какой сервер, что поставить, как настроить для сборки
`rustqs.exe` (PLAN.md §8.3/§8.4). Дополняет [README.md](README.md) (там краткие шаги).

---

## 1. Какой сервер

### ОС — два варианта

| | Windows Server 2022 | Windows 11 Pro |
|---|---|---|
| Совпадение с upstream CI | ✅ точное (CI = windows-2022 / ltsc2022) | близкое, бинарь эквивалентен |
| Лицензия | нужна отдельная | **у вас уже есть** (Pro for Workstations) |
| Headless | штатно | да (GUI можно не трогать) |
| Рекоменд. | для выделенного «правильного» билд-бокса | прагматичный, уже оплачен |

**Вывод:** оба дают рабочий результат. Server 2022 — ближе к официальной сборке;
Windows 11 Pro — дешевле (уже лицензирован) и проверенно собирает Flutter Windows.
Если нет причины платить за Server — берите **Windows 11 Pro**.

### Headless — да, но НЕ Server Core (на старте)

Сборка headless-дружелюбна (Flutter build = CLI, ни GUI, ни GPU не нужны). Но:
- **НЕ ставьте Server Core сейчас.** VS Build Tools и часть Flutter-тулинга задевают
  GUI-смежные компоненты, инсталляторы на Core капризнее, а скрипты ещё `[VERIFY]` —
  первая сборка будет отладочной, и доступ к рабочему столу сэкономит время.
- Берите **Desktop Experience / Win 11 Pro** и эксплуатируйте **физически headless** —
  без монитора, через RDP/SSH. Урезать до Core можно потом, когда всё зелёное.
- **Агент (session 0):** Scheduled Task крутится в неинтерактивной сессии — для
  cargo/flutter build ок, но редкие шаги (подпись, packer) лучше идут в интерактивной.
  Для ПЕРВЫХ сборок залогиньтесь по RDP и запустите `agent.ps1` в сессии; в службу
  переводите после того, как сборка прошла (см. §5).

### Железо (минимум / рекомендуется)

| Ресурс | Минимум | Рекомендуется | Почему |
|---|---|---|---|
| CPU | 4 ядра | **8+ ядер** | Rust + vcpkg(ffmpeg) сильно параллелятся |
| RAM | 16 ГБ | **32 ГБ** | Rust линковка + Flutter + vcpkg дают пики |
| Диск | 150 ГБ SSD | **250 ГБ NVMe** | см. раскладку ниже; NVMe = скорость сборки |
| GPU | — | — | не нужен (hwcodec собирает libs, GPU при сборке не требуется) |
| Сеть | приватный LAN к проду | — | для SMB; интернет только на установку |

**Раскладка диска (~250 ГБ):** Windows ~40, тулчейн (VS BuildTools+Flutter+Rust+LLVM) ~20,
vcpkg buildtrees (ffmpeg/hwcodec) ~20, offline-kit ~5, per-job `target/` кэш 5-15 каждый,
запас. Меньше 150 ГБ будет тесно.

### Где поднять (варианты)

- **A. Hyper-V VM на вашей текущей машине** (у вас 1.5 ТБ и мощное железо). Самый быстрый
  старт. Docker Desktop (WSL2) и Hyper-V VM на Win 11 уживаются. Выделите 8 vCPU / 32 ГБ /
  250 ГБ. ← рекомендую для начала.
- **B. Отдельная физическая машина.** Максимум изоляции и скорости.
- **C. Облачная Windows-VM** (Azure/AWS). Если сборки редкие — платите по часам.

---

## 2. Подготовка ОС (до setup.ps1)

От администратора, PowerShell:

### 2.1. Обновления + длинные пути (ОБЯЗАТЕЛЬНО)

```powershell
# Flutter/Rust/vcpkg создают очень длинные пути → без этого сборка падает на MAX_PATH
New-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' `
    -Name 'LongPathsEnabled' -Value 1 -PropertyType DWORD -Force
git config --system core.longpaths true
```

### 2.2. Сеть и имя

- Статический IP в приватной сети к проду (например 192.168.x.x).
- Рабочая группа (домен НЕ нужен). Имя хоста, напр. `WINBUILD`.
- Антивирус/Defender: добавьте в исключения `C:\rustdesk-build`, `C:\vcpkg`,
  `C:\flutter`, `%USERPROFILE%\.cargo` — иначе сканер сильно тормозит сборку и иногда
  ложно флагует portable-packer exe.

### 2.3. Пользователь

- Локальный пользователь `builder` (для setup — администратор).
- (Опц.) OpenSSH Server для удалённого админа: `Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0`.

---

## 3. Установка тулчейна (setup.ps1)

1. Скопируйте на сервер: каталог `win-builder\` и `offline-kit\artifacts` (напр. в
   `C:\win-builder` и `D:\offline-kit\artifacts`).
2. Запустите (от администратора):

```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force
C:\win-builder\setup.ps1 -KitPath D:\offline-kit\artifacts
```

Ставит: Chocolatey → VS 2022 Build Tools (VCTools + Win11 SDK) → git/7zip/nasm/cmake →
LLVM 15.0.6 → Python 3 → Rust 1.75 (msvc) → cargo-expand + flutter_rust_bridge_codegen 1.80 →
Flutter 3.24.5 → vcpkg @ baseline. С `-KitPath` берёт Rust/Flutter/vcpkg из offline-кита.

3. **Перелогиньтесь** (применить PATH/env).
4. Проверка:

```powershell
flutter doctor -v        # Windows toolchain — всё зелёное (VS, Windows SDK)
rustc --version          # 1.75.0
cargo --version
flutter_rust_bridge_codegen --version   # 1.80.x
& $env:VCPKG_ROOT\vcpkg.exe version
```

> Первый запуск vcpkg-сборки (внутри agent.ps1) долгий: ffmpeg/hwcodec компилируется
> 30-60+ мин, потом кэшируется в `C:\vcpkg\installed`.

---

## 4. Настройка SMB-канала (§8.4)

Прод-API пишет job в том `rdgen-data` (Linux). Делаем его видимым Windows-агенту.

**На Linux-проде** — экспорт по Samba:
```
# /etc/samba/smb.conf
[rdgen-data]
   path = /var/lib/docker/volumes/rdgen-data/_data
   valid users = builder
   writable = yes
   create mask = 0664
   directory mask = 0775
```
```bash
sudo apt install samba && sudo smbpasswd -a builder && sudo systemctl restart smbd
# firewall: открыть 445/tcp ТОЛЬКО для приватной подсети Windows-агента
```

**На Windows-агенте** — примонтировать как Z: (постоянно):
```powershell
net use Z: \\PROD_HOST\rdgen-data /user:builder * /persistent:yes
# проверка
Test-Path Z:\jobs
```

> Альтернатива: Windows хостит share, Linux монтирует `cifs`. Главное — обе стороны
> видят один `jobs/`. Никаких Docker-демонов/портов наружу.

**Патчи rdgen** (для L2 — приёма подписанного custom.txt): скопировать
`rdgen/.github/patches/*` в `Z:\rdgen-data\patches\` (или локально и указать агенту).

---

## 5. Запуск агента как службы

Scheduled Task «при старте», от имени `builder`, с наивысшими правами:

```powershell
$action  = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument `
  '-ExecutionPolicy Bypass -File C:\win-builder\agent.ps1 -DataRoot Z:\rdgen-data -KitPath D:\offline-kit\artifacts'
$trigger = New-ScheduledTaskTrigger -AtStartup
$set     = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName 'rustqs-build-agent' -Action $action -Trigger $trigger `
  -RunLevel Highest -User builder -Password '<пароль>' -Settings $set
Start-ScheduledTask -TaskName 'rustqs-build-agent'
```

Лог агента — в консоли задачи; build-логи каждой сборки — в `Z:\rdgen-data\output\<job>\build.log`.

---

## 6. Первая проверка (end-to-end)

1. Положить тестовый job вручную (имитируя API):
```powershell
@'
{ "platform":"windows", "src_ref":"1.4.7", "server":"ваш.сервер:21116",
  "key":"ВАШ_ПУБ_КЛЮЧ", "app_name":"rustqs" }
'@ | Set-Content Z:\rdgen-data\jobs\test-001.json
```
2. Агент подхватит → соберёт → `Z:\rdgen-data\output\test-001\rustqs.exe`, статус `done`.
3. Smoke-тест (§8.5): запустить `rustqs.exe` на чистой Windows — должен стартовать и
   показывать ваш сервер вшитым (без ручной настройки).

---

## 7. Безопасность

- **Только приватная сеть.** Никакого публичного RDP/SMB/445 в интернет.
- Build-сервер полу-доверенный (собирает конфиги из admin-UI) — держать в изолированном
  сегменте, без доступа к чувствительным ресурсам.
- После `setup.ps1` интернет можно отключить — с offline-китом сборка идёт без сети
  (это и есть суверенность; проверка — `cargo build --offline` уже подтверждена на L1).
- SMB-пользователь `builder` — минимальные права, только на share `rdgen-data`.

---

## 8. Известные TODO (при первом тесте, [VERIFY] в скриптах)

- Сборка `RustDeskTempTopMostWindow` (msbuild из kit-бандла) + размещение артефакта.
- Полный набор branding-sed (в agent.ps1 сокращённый — см. `rdgen/generator-windows.yml`).
- Точные пути установки Rust (PATH после MSI vs rustup) — выверить на живом хосте.
- vcpkg overlay-ports `res/vcpkg` + overrides (ffnvcodec/amd-amf) — проверить, что
  manifest-режим их подхватывает.
