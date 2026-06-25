# win-builder — ❄️ FROZEN / FALLBACK

> **Not in use.** Active build path is GitHub Actions
> ([github-build/](../github-build/README.md)).
>
> This is a contingency for when GitHub Actions becomes unavailable.
> Code is untested (no Windows host available).

## What it is

Standalone Windows builder for `rustqs.exe` on a separate Windows Server, no Docker.
API channel: SMB share of the `rdgen-data` volume.

## Files

- `setup.ps1` — toolchain install (VS BuildTools, Flutter, Rust, vcpkg, LLVM)
- `agent.ps1` — SMB queue poller: 3 injection layers + build
- `SERVER-SETUP.md` — detailed deployment guide

## If activated

1. Windows Server 2022 / Win 11 Pro, 8+ core, 32 GB, 250 GB NVMe
2. `setup.ps1 -KitPath D:\offline-kit\artifacts`
3. SMB share on Linux: export `rdgen-data` volume
4. `agent.ps1` as a Scheduled Task

```powershell
net use Z: \\PROD_HOST\rdgen-data /user:builder *
Register-ScheduledTask -TaskName rustqs-build-agent ...
```

## Flow

```
admin-ui → Go API writes job.json → Z:\rdgen-data\jobs\ (SMB)
  → agent.ps1 → clone → L1 config.rs → L2 custom_.txt → L3 branding
  → build.py → rustqs.exe → Z:\rdgen-data\output\<id>\ → admin-ui Download
```
