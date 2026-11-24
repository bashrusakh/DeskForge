# win-builder: historical, not in use

A standalone Windows builder for `rustqs.exe` on a separate Windows Server, fed through an
SMB share. It is superseded by GitHub Actions ([github-build/](../github-build/README.md)),
is not a fallback, and its scripts are untested (no Windows host was available).

## Files

- `setup.ps1`: installs the toolchain (VS BuildTools, Flutter, Rust, vcpkg, LLVM)
- `agent.ps1`: polls the SMB queue, applies the three injection layers, builds

## Flow

```
admin-ui → Go API writes job.json → Z:\rdgen-data\jobs\ (SMB)
  → agent.ps1 → clone → config.rs → custom_.txt → branding
  → build.py → rustqs.exe → Z:\rdgen-data\output\<id>\ → admin-ui download
```

## Setup checklist (for reference)

- Windows Server 2022 or Win 11 Pro, 8+ vCPU, 32 GB RAM, 250 GB NVMe (VM or physical)
- `LongPathsEnabled` and `git config core.longpaths true`
- Defender exclusions: `C:\rustdesk-build`, `C:\vcpkg`, `%USERPROFILE%\.cargo`
- `setup.ps1 -KitPath D:\offline-kit\artifacts`
- On the Linux host, export the `rdgen-data` volume over Samba
  (`path = /var/lib/docker/volumes/rdgen-data/_data`); on Windows, `net use Z: \\PROD_HOST\rdgen-data`
- Copy `rdgen/.github/patches/*` to `Z:\rdgen-data\patches\`
- Run `agent.ps1` as the Scheduled Task `rustqs-build-agent`
- Smoke test: put `Z:\rdgen-data\jobs\test-001.json`, expect `Z:\rdgen-data\output\test-001\rustqs.exe`

Known gaps: `RustDeskTempTopMostWindow` is not built, the branding `sed` is shortened
(see `rdgen/.github/workflows/generator-windows.yml`), Rust install paths and vcpkg overlay
ports are unverified.

Security: private network only, no public RDP/SMB; the SMB user `builder` gets minimum
rights on the share.
