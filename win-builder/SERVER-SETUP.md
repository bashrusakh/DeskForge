# SERVER-SETUP — ❄️ FROZEN / FALLBACK

> **Не актуально.** Активный путь — GitHub Actions.
> Оставлено как reference если когда-нибудь понадобится standalone Windows сборщик.

---

## Если активировать: чеклист

- [ ] Windows Server 2022 или Win 11 Pro
- [ ] 8+ vCPU, 32 GB RAM, 250 GB NVMe
- [ ] Hyper-V VM или отдельная машина
- [ ] `LongPathsEnabled`, `git config core.longpaths true`
- [ ] Исключения Defender: `C:\rustdesk-build`, `C:\vcpkg`, `%USERPROFILE%\.cargo`
- [ ] `setup.ps1 -KitPath D:\offline-kit\artifacts`
- [ ] Samba на Linux: `/etc/samba/smb.conf` → `path = /var/lib/docker/volumes/rdgen-data/_data`
- [ ] Монтирование SMB: `net use Z: \\PROD_HOST\rdgen-data`
- [ ] Патчи `rdgen/.github/patches/*` → `Z:\rdgen-data\patches\`
- [ ] Scheduled Task `rustqs-build-agent`
- [ ] Тест: `Z:\rdgen-data\jobs\test-001.json` → `Z:\rdgen-data\output\test-001\rustqs.exe`

## Известные TODO при первом тесте

- Сборка `RustDeskTempTopMostWindow` (msbuild из bundle)
- Полный branding `sed` (сейчас сокращённый, см. `rdgen/generator-windows.yml`)
- Пути Rust (MSI vs rustup) на реальном хосте
- `vcpkg` overlay ports

## Безопасность

- Только private network. Никакого публичного RDP/SMB.
- После установки интернет можно отключить (`cargo build --offline`).
- SMB пользователь `builder` — минимальные права на share.
