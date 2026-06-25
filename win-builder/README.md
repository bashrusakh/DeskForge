# win-builder — ❄️ FROZEN / FALLBACK

> **Не используется.** Активный путь сборки — GitHub Actions
> ([github-build/](../github-build/README.md)).
>
> Это запасной вариант на случай если GitHub Actions станет недоступен.
> Код не тестирован (нет Windows хоста).

## Что это

Standalone Windows сборщик `rustqs.exe` на отдельном Windows Server без Docker.
Канал с API — SMB шаринг `rdgen-data` volume.

## Файлы

- `setup.ps1` — установка toolchain (VS BuildTools, Flutter, Rust, vcpkg, LLVM)
- `agent.ps1` — poller SMB очереди: 3 слоя инъекции + build
- `SERVER-SETUP.md` — детальный гайд деплоя

## Если активировать

1. Windows Server 2022 / Win 11 Pro, 8+ core, 32 GB, 250 GB NVMe
2. `setup.ps1 -KitPath D:\offline-kit\artifacts`
3. SMB шары на Linux: экспортировать `rdgen-data` volume
4. `agent.ps1` как Scheduled Task

```powershell
net use Z: \\PROD_HOST\rdgen-data /user:builder *
Register-ScheduledTask -TaskName rustqs-build-agent ...
```

## Поток

```
admin-ui → Go API пишет job.json → Z:\rdgen-data\jobs\ (SMB)
  → agent.ps1 → clone → L1 config.rs → L2 custom.txt → L3 branding
  → build.py → rustqs.exe → Z:\rdgen-data\output\<id>\ → admin-ui Download
```
