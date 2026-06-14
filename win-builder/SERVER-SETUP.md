# SERVER-SETUP - Windows build server deployment (detailed)

Complete guide: which server to use, what to install, and how to configure it to build
`rustqs.exe` (PLAN.md §8.3/§8.4). Complements [README.md](README.md), which contains the short version.

---

## 1. Which server to use

### OS - two options

| | Windows Server 2022 | Windows 11 Pro |
|---|---|---|
| Match with upstream CI | ✅ exact (`windows-2022` / `ltsc2022`) | close enough, equivalent binary |
| License | separate license required | **already available** (Pro for Workstations) |
| Headless | native | yes (GUI can remain unused) |
| Recommended | for a dedicated "proper" build box | pragmatic, already paid for |

**Conclusion:** both work. Server 2022 is closer to the official build;
Windows 11 Pro is cheaper (already licensed) and is known to build Flutter Windows successfully.
If there is no reason to pay for Server, choose **Windows 11 Pro**.

### Headless - yes, but NOT Server Core initially

The build is headless-friendly (Flutter build is CLI-only, no GUI or GPU needed). But:
- **Do NOT install Server Core right now.** VS Build Tools and part of the Flutter toolchain
  touch GUI-adjacent components, installers are more temperamental on Core, and the scripts
  still contain `[VERIFY]` points. The first build will be a debugging session, and desktop
  access will save time.
- Use **Desktop Experience / Windows 11 Pro** and operate it **physically headless**:
  no monitor, manage through RDP/SSH. You can trim it down later once everything is green.
- **Agent (session 0):** a Scheduled Task runs in a non-interactive session. That is fine for
  `cargo`/Flutter builds, but rare steps (signing, packer) behave better interactively.
  For the FIRST builds, log in through RDP and run `agent.ps1` in-session; convert it into a
  service only after the build has succeeded (see §5).

### Hardware (minimum / recommended)

| Resource | Minimum | Recommended | Why |
|---|---|---|---|
| CPU | 4 cores | **8+ cores** | Rust + `vcpkg` (`ffmpeg`) parallelize heavily |
| RAM | 16 GB | **32 GB** | Rust linking + Flutter + `vcpkg` create peaks |
| Disk | 150 GB SSD | **250 GB NVMe** | see breakdown below; NVMe speeds up builds |
| GPU | - | - | not required (`hwcodec` builds libs; no GPU needed during build) |
| Network | private LAN to prod | - | for SMB; internet only needed for installation |

**Disk layout (~250 GB):** Windows ~40, toolchain (VS BuildTools + Flutter + Rust + LLVM) ~20,
`vcpkg` buildtrees (`ffmpeg`/`hwcodec`) ~20, `offline-kit` ~5, per-job `target/` cache 5-15 each,
plus reserve. Below 150 GB will be tight.

### Where to host it (options)

- **A. Hyper-V VM on your current machine** (you have 1.5 TB and strong hardware). Fastest
  start. Docker Desktop (WSL2) and a Hyper-V VM on Windows 11 coexist fine. Allocate
  8 vCPU / 32 GB / 250 GB. Recommended as the starting point.
- **B. Separate physical machine.** Maximum isolation and speed.
- **C. Cloud Windows VM** (Azure/AWS). If builds are rare, pay by the hour.

---

## 2. Prepare the OS (before `setup.ps1`)

Run in PowerShell as administrator.

### 2.1. Updates + long paths (REQUIRED)

```powershell
# Flutter/Rust/vcpkg create very long paths -> without this, the build fails on MAX_PATH
New-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' `
    -Name 'LongPathsEnabled' -Value 1 -PropertyType DWORD -Force
git config --system core.longpaths true
```

### 2.2. Network and hostname

- Static IP in the private network facing prod (for example `192.168.x.x`).
- Workgroup is enough (domain not required). Hostname, for example, `WINBUILD`.
- Antivirus/Defender: add exclusions for `C:\rustdesk-build`, `C:\vcpkg`,
  `C:\flutter`, `%USERPROFILE%\.cargo`, otherwise scanning slows builds dramatically and
  may false-flag the portable-packer exe.

### 2.3. User

- Local user `builder` (administrator during setup).
- Optional: OpenSSH Server for remote admin:
  `Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0`.

---

## 3. Install the toolchain (`setup.ps1`)

1. Copy to the server: the `win-builder\` directory and `offline-kit\artifacts`
   (for example to `C:\win-builder` and `D:\offline-kit\artifacts`).
2. Run as administrator:

```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force
C:\win-builder\setup.ps1 -KitPath D:\offline-kit\artifacts
```

Installs: Chocolatey -> VS 2022 Build Tools (VCTools + Win11 SDK) -> git/7zip/nasm/cmake ->
LLVM 15.0.6 -> Python 3 -> Rust 1.75 (msvc) -> `cargo-expand` +
`flutter_rust_bridge_codegen` 1.80 -> Flutter 3.24.5 -> `vcpkg` at the pinned baseline.
With `-KitPath`, Rust/Flutter/`vcpkg` are taken from the offline kit.

3. **Log out and back in** to apply PATH/env.
4. Verify:

```powershell
flutter doctor -v        # Windows toolchain should be fully green (VS, Windows SDK)
rustc --version          # 1.75.0
cargo --version
flutter_rust_bridge_codegen --version   # 1.80.x
& $env:VCPKG_ROOT\vcpkg.exe version
```

> The first `vcpkg` build (inside `agent.ps1`) takes a long time:
> `ffmpeg`/`hwcodec` may compile for 30-60+ minutes, then it is cached in `C:\vcpkg\installed`.

---

## 4. Configure the SMB channel (§8.4)

The production API writes jobs into the `rdgen-data` volume on Linux. Make it visible to the Windows agent.

**On Linux prod** - export via Samba:
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
# firewall: open 445/tcp ONLY for the Windows agent private subnet
```

**On the Windows agent** - mount as `Z:` persistently:
```powershell
net use Z: \\PROD_HOST\rdgen-data /user:builder * /persistent:yes
# check
Test-Path Z:\jobs
```

> Alternative: Windows hosts the share, Linux mounts it with `cifs`.
> The key point is that both sides see the same `jobs/` folder.
> No exposed Docker daemons/ports.

**rdgen patches** (for L2: accepting signed `custom.txt`): copy
`rdgen/.github/patches/*` into `Z:\rdgen-data\patches\` (or keep them locally and point the agent there).

---

## 5. Run the agent as a service

Create a Scheduled Task "At startup" running as `builder` with highest privileges:

```powershell
$action  = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument `
  '-ExecutionPolicy Bypass -File C:\win-builder\agent.ps1 -DataRoot Z:\rdgen-data -KitPath D:\offline-kit\artifacts'
$trigger = New-ScheduledTaskTrigger -AtStartup
$set     = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName 'rustqs-build-agent' -Action $action -Trigger $trigger `
  -RunLevel Highest -User builder -Password '<password>' -Settings $set
Start-ScheduledTask -TaskName 'rustqs-build-agent'
```

The agent log appears in the task console; per-build logs are written to
`Z:\rdgen-data\output\<job>\build.log`.

---

## 6. First verification (end-to-end)

1. Drop a test job manually (simulating the API):
```powershell
@'
{ "platform":"windows", "src_ref":"1.4.7", "server":"your.server:21116",
  "key":"YOUR_PUBLIC_KEY", "app_name":"rustqs" }
'@ | Set-Content Z:\rdgen-data\jobs\test-001.json
```
2. The agent picks it up -> builds -> `Z:\rdgen-data\output\test-001\rustqs.exe`, status `done`.
3. Smoke test (§8.5): run `rustqs.exe` on a clean Windows machine. It should start and
   show your baked-in server without any manual setup.

---

## 7. Security

- **Private network only.** No public RDP/SMB/445 exposed to the internet.
- The build server is semi-trusted (it builds configs from the admin UI). Keep it in an
  isolated segment with no access to sensitive resources.
- After `setup.ps1`, internet can be disabled. With the offline kit, the build works without
  network access. That is the whole point of sovereignty; `cargo build --offline` is already
  confirmed at L1.
- The SMB user `builder` should have the minimum rights needed for the `rdgen-data` share.

---

## 8. Known TODOs (first test, `[VERIFY]` in scripts)

- Build `RustDeskTempTopMostWindow` (msbuild from the kit bundle) and place the artifact.
- Bring over the full branding `sed` set (`agent.ps1` currently uses a shortened version; see `rdgen/generator-windows.yml`).
- Confirm exact Rust install paths (PATH after MSI vs `rustup`) on a real host.
- Verify that `vcpkg` overlay ports `res/vcpkg` and overrides (`ffnvcodec`/`amd-amf`) are picked up in manifest mode.
