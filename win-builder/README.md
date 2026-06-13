# win-builder — нативный Windows build-агент (PLAN.md §8.3, §8.4)

Сборка актуального **Flutter Windows-клиента** (`rustqs.exe`) на выделенном
**headless Windows Server** — без Docker-контейнеров (решение владельца: нативно + SMB).
Канал с прод-API — общая **SMB-папка** job-очереди.

> Почему нативно, а не Windows-контейнер: Flutter desktop в servercore капризничает
> (hyperv-isolation, недостающие компоненты), а нативная установка headless-дружелюбна
> и проще для одного билд-сервера. Спецификация тулчейна — `setup.ps1` (бывший
> Dockerfile.build-win-native, преобразован).

## Файлы

| Файл | Назначение |
|---|---|
| `setup.ps1` | Установка тулчейна (один раз при развёртывании). Поддерживает `-KitPath` для offline-kit. |
| `agent.ps1` | Поллер job-очереди: 3 слоя инъекции конфига + сборка → `rustqs.exe`. |

> 📘 **Подробное руководство по серверу** (какой, специфика железа, провижининг,
> длинные пути, антивирус, безопасность, первый тест): [SERVER-SETUP.md](SERVER-SETUP.md).
> Ниже — краткая версия.

## Развёртывание

### 1. Подготовить Windows Server (headless)

Windows Server 2022 (или Win 11). RDP/GUI не нужен для сборки. От администратора:

```powershell
# скопировать offline-kit\artifacts на сервер (напр. D:\offline-kit\artifacts)
powershell -ExecutionPolicy Bypass -File setup.ps1 -KitPath D:\offline-kit\artifacts
# перелогиниться для применения PATH/env
```

### 2. Настроить SMB-канал job-очереди (§8.4)

Прод-API (Linux) и Windows-агент работают через общую папку `rdgen-data`
(`jobs/`, `output/`, `patches/`). Рекомендуемая схема — **Linux хостит Samba**
(данные уже там), Windows монтирует:

**На Linux-проде** (экспорт rdgen-data по Samba):
```
# /etc/samba/smb.conf
[rdgen-data]
   path = /var/lib/docker/volumes/rdgen-data/_data
   valid users = builder
   writable = yes
```
```bash
sudo smbpasswd -a builder && sudo systemctl restart smbd
```

**На Windows-агенте** (примонтировать как диск Z:):
```powershell
net use Z: \\PROD_HOST\rdgen-data /user:builder * /persistent:yes
```

> Альтернатива: Windows хостит share, Linux монтирует через `cifs` — тоже рабочее.
> Главное — обе стороны видят один `jobs/`. Никаких открытых демонов/портов Docker.

### 3. Положить rdgen-патчи в очередь

`allowCustom.py` и др. из `rdgen/.github/patches/` скопировать в `<DataRoot>\patches\`
(нужен для L2 — приёма подписанного custom.txt).

### 4. Запустить агента (как службу)

Через Scheduled Task «At startup», от имени build-пользователя:
```powershell
$action  = New-ScheduledTaskAction -Execute 'powershell.exe' `
    -Argument '-ExecutionPolicy Bypass -File C:\win-builder\agent.ps1 -DataRoot Z:\rdgen-data -KitPath D:\offline-kit\artifacts'
$trigger = New-ScheduledTaskTrigger -AtStartup
Register-ScheduledTask -TaskName 'rustqs-build-agent' -Action $action -Trigger $trigger -RunLevel Highest
Start-ScheduledTask -TaskName 'rustqs-build-agent'
```

## Поток (PLAN.md §4)

```
admin-ui → Go API пишет job.json → Z:\rdgen-data\jobs\ (SMB)
  → agent.ps1 поллит → clone(bundle) → L1 config.rs → L2 custom.txt → L3 branding
  → vcpkg install → bridge codegen → build.py → rustqs.exe
  → Z:\rdgen-data\output\<job>\rustqs.exe → admin-ui Download
```

Прод-API **не меняется**: он уже пишет job в том `rdgen-data`. SMB лишь делает этот том
видимым Windows-агенту. Это и есть весь канал §8.4.

## Статус

`setup.ps1` и `agent.ps1` — **спроектированы, НЕ протестированы** (нет Windows-хоста у
автора). Места риска помечены `[VERIFY]`. Открытые TODO для первого теста:
- сборка `RustDeskTempTopMostWindow` (msbuild из kit-бандла) и размещение артефакта;
- полный набор branding-sed (сейчас сокращённый — см. `rdgen/generator-windows.yml`);
- smoke-тест готового `rustqs.exe` (§8.5).
