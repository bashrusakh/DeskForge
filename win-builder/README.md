# win-builder - native Windows build agent (PLAN.md §8.3, §8.4)

Builds the current **Flutter Windows client** (`rustqs.exe`) on a dedicated
**headless Windows Server** without Docker containers (owner decision: native + SMB).
The channel to the production API is a shared **SMB job queue** folder.

> Why native instead of a Windows container: Flutter desktop is temperamental in
> servercore (Hyper-V isolation, missing components), while a native installation is
> headless-friendly and simpler for a single build server. The toolchain spec lives in
> `setup.ps1` (formerly `Dockerfile.build-win-native`, converted).

## Files

| File | Purpose |
|---|---|
| `setup.ps1` | Installs the toolchain (one-time during provisioning). Supports `-KitPath` for `offline-kit`. |
| `agent.ps1` | Job queue poller: 3 config injection layers + build -> `rustqs.exe`. |

> Detailed server guide (which server, hardware sizing, provisioning,
> long paths, antivirus, security, first test): [SERVER-SETUP.md](SERVER-SETUP.md).
> Below is the short version.

## Deployment

### 1. Prepare the Windows Server (headless)

Windows Server 2022 (or Windows 11). RDP/GUI is not required for builds. Run as administrator:

```powershell
# copy offline-kit\artifacts to the server (for example D:\offline-kit\artifacts)
powershell -ExecutionPolicy Bypass -File setup.ps1 -KitPath D:\offline-kit\artifacts
# log out and back in to apply PATH/env
```

### 2. Configure the SMB job queue channel (§8.4)

The production API (Linux) and Windows agent work through a shared `rdgen-data`
folder (`jobs/`, `output/`, `patches/`). Recommended layout: **Linux hosts Samba**
(the data is already there), Windows mounts it:

**On Linux prod** (export `rdgen-data` via Samba):
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

**On the Windows agent** (mount as drive `Z:`):
```powershell
net use Z: \\PROD_HOST\rdgen-data /user:builder * /persistent:yes
```

> Alternative: Windows can host the share and Linux can mount it via `cifs`.
> The key requirement is that both sides see the same `jobs/` folder.
> No exposed Docker daemons/ports.

### 3. Put rdgen patches into the queue

Copy `allowCustom.py` and the other files from `rdgen/.github/patches/` into
`<DataRoot>\patches\` (needed for L2: accepting signed `custom.txt`).

### 4. Start the agent (as a service)

Using a Scheduled Task with the build user account:
```powershell
$action  = New-ScheduledTaskAction -Execute 'powershell.exe' `
    -Argument '-ExecutionPolicy Bypass -File C:\win-builder\agent.ps1 -DataRoot Z:\rdgen-data -KitPath D:\offline-kit\artifacts'
$trigger = New-ScheduledTaskTrigger -AtStartup
Register-ScheduledTask -TaskName 'rustqs-build-agent' -Action $action -Trigger $trigger -RunLevel Highest
Start-ScheduledTask -TaskName 'rustqs-build-agent'
```

## Flow (PLAN.md §4)

```
admin-ui -> Go API writes job.json -> Z:\rdgen-data\jobs\ (SMB)
  -> agent.ps1 polls -> clone(bundle) -> L1 config.rs -> L2 custom.txt -> L3 branding
  -> vcpkg install -> bridge codegen -> build.py -> rustqs.exe
  -> Z:\rdgen-data\output\<job>\rustqs.exe -> admin-ui Download
```

The production API **does not change**: it already writes jobs into the `rdgen-data`
volume. SMB only makes that volume visible to the Windows agent. That is the entire
§8.4 channel.

## Status

`setup.ps1` and `agent.ps1` are **designed but NOT tested** (the author has no Windows host).
Risky spots are marked `[VERIFY]`. Open TODOs for the first test:
- build `RustDeskTempTopMostWindow` (msbuild from the kit bundle) and place the artifact;
- complete the branding `sed` set (currently shortened, see `rdgen/generator-windows.yml`);
- smoke-test the resulting `rustqs.exe` (§8.5).
