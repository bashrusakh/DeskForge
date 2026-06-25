# SERVER-SETUP — ❄️ FROZEN / FALLBACK

> **Not active.** Active path is GitHub Actions.
> Kept as reference in case a standalone Windows builder is ever needed.

---

## Activation checklist

- [ ] Windows Server 2022 or Win 11 Pro
- [ ] 8+ vCPU, 32 GB RAM, 250 GB NVMe
- [ ] Hyper-V VM or physical machine
- [ ] `LongPathsEnabled`, `git config core.longpaths true`
- [ ] Defender exclusions: `C:\rustdesk-build`, `C:\vcpkg`, `%USERPROFILE%\.cargo`
- [ ] `setup.ps1 -KitPath D:\offline-kit\artifacts`
- [ ] Samba on Linux: `/etc/samba/smb.conf` → `path = /var/lib/docker/volumes/rdgen-data/_data`
- [ ] Mount SMB: `net use Z: \\PROD_HOST\rdgen-data`
- [ ] Patches `rdgen/.github/patches/*` → `Z:\rdgen-data\patches\`
- [ ] Scheduled Task `rustqs-build-agent`
- [ ] Test: `Z:\rdgen-data\jobs\test-001.json` → `Z:\rdgen-data\output\test-001\rustqs.exe`

## Known TODOs on first test

- Build `RustDeskTempTopMostWindow` (msbuild from bundle)
- Full branding `sed` (currently shortened, see `rdgen/.github/workflows/generator-windows.yml`)
- Verify Rust install paths (MSI vs rustup) on a real host
- `vcpkg` overlay ports

## Security

- Private network only. No public RDP/SMB.
- Internet can be disabled after setup (`cargo build --offline`).
- SMB user `builder` — minimum rights on the share.
