# ============================================================================
# win-builder/setup.ps1 — установка тулчейна на НАТИВНЫЙ headless Windows Server
# ============================================================================
# PLAN.md §8.3 (вариант: нативно + SMB, выбран владельцем). Заменяет контейнерный
# Dockerfile.build-win-native. Ставит всё для сборки актуального Flutter Windows-клиента.
#
# Запуск (от администратора, один раз при развёртывании сервера):
#   powershell -ExecutionPolicy Bypass -File setup.ps1
#   powershell -ExecutionPolicy Bypass -File setup.ps1 -KitPath D:\offline-kit\artifacts
#
# -KitPath: если указан и артефакты есть — ставит из offline-kit (rust MSI, flutter zip,
# vcpkg), иначе качает из сети. Для суверенной установки используйте -KitPath.
#
# ⚠️ DESIGN-АРТЕФАКТ, НЕ ПРОВЕРЕН на живом Windows-сервере. [VERIFY] = места риска.
# Версии — пины из offline-kit/versions.env (тег rustdesk 1.4.7).
# ============================================================================
param(
    [string]$KitPath = ""
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# --- Пины (синхронны offline-kit/versions.env) ---
$RUST_VERSION   = '1.75.0'
$FLUTTER_VERSION= '3.24.5'
$LLVM_VERSION   = '15.0.6'
$FRB_VERSION    = '1.80.1'
$CARGO_EXPAND   = '1.0.95'
$VCPKG_BASELINE = '120deac3062162151622ca4860575a33844ba10b'

function Have($cmd) { return [bool](Get-Command $cmd -ErrorAction SilentlyContinue) }
function Log($m) { Write-Host "[setup] $m" -ForegroundColor Cyan }
function FromKit($name) { if ($KitPath -and (Test-Path (Join-Path $KitPath $name))) { return (Join-Path $KitPath $name) } return $null }

# --- 1. Chocolatey ---
if (-not (Have choco)) {
    Log "install Chocolatey"
    Set-ExecutionPolicy Bypass -Scope Process -Force
    [System.Net.ServicePointManager]::SecurityProtocol = 3072
    iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    $env:Path += ";$env:ProgramData\chocolatey\bin"
}

# --- 2. VS 2022 Build Tools + Desktop C++ (MSVC; нужен для cargo msvc + flutter windows) ---
# [VERIFY] Критичен набор: VCTools + Windows 11 SDK. ~8 ГБ.
Log "install VS 2022 Build Tools (VCTools)"
choco install -y visualstudio2022buildtools --package-parameters `
    '--add Microsoft.VisualStudio.Workload.VCTools --add Microsoft.VisualStudio.Component.Windows11SDK.22621 --includeRecommended'
choco install -y git 7zip nasm cmake

# --- 3. LLVM/Clang (bindgen) ---
Log "install LLVM $LLVM_VERSION"
choco install -y llvm --version=$LLVM_VERSION
[Environment]::SetEnvironmentVariable('LIBCLANG_PATH', 'C:\Program Files\LLVM\bin', 'Machine')

# --- 4. Python 3 ---
choco install -y python3

# --- 5. Rust 1.75 (host + target x86_64-pc-windows-msvc) ---
Log "install Rust $RUST_VERSION"
$rustMsi = FromKit "rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi"
if ($rustMsi) {
    Log "  from kit: $rustMsi"
    Start-Process msiexec.exe -ArgumentList "/i `"$rustMsi`" /quiet /norestart" -Wait
} else {
    Invoke-WebRequest -Uri https://static.rust-lang.org/rustup/dist/x86_64-pc-windows-msvc/rustup-init.exe -OutFile $env:TEMP\rustup-init.exe
    & $env:TEMP\rustup-init.exe -y --default-toolchain $RUST_VERSION --default-host x86_64-pc-windows-msvc --profile minimal
}
$cargoBin = "$env:USERPROFILE\.cargo\bin"
$env:Path = "$cargoBin;C:\Program Files\Rust stable MSVC 1.75\bin;$env:Path"
if (Have rustup) { rustup target add x86_64-pc-windows-msvc }

# --- 6. flutter_rust_bridge_codegen + cargo-expand ---
# [VERIFY] долгий cargo install из исходников
Log "install flutter_rust_bridge_codegen $FRB_VERSION + cargo-expand"
cargo install cargo-expand --version $CARGO_EXPAND --locked
cargo install flutter_rust_bridge_codegen --version $FRB_VERSION --features uuid --locked

# --- 7. Flutter SDK 3.24.5 ---
Log "install Flutter $FLUTTER_VERSION"
if (-not (Test-Path C:\flutter)) {
    $flutterZip = FromKit "flutter_windows_$FLUTTER_VERSION-stable.zip"
    if (-not $flutterZip) {
        $flutterZip = "$env:TEMP\flutter.zip"
        Invoke-WebRequest -Uri "https://storage.googleapis.com/flutter_infra_release/releases/stable/windows/flutter_windows_$FLUTTER_VERSION-stable.zip" -OutFile $flutterZip
    }
    Expand-Archive -Path $flutterZip -DestinationPath C:\ -Force
}
[Environment]::SetEnvironmentVariable('Path', "$([Environment]::GetEnvironmentVariable('Path','Machine'));C:\flutter\bin;$cargoBin", 'Machine')
$env:Path = "C:\flutter\bin;$env:Path"
flutter config --no-analytics --enable-windows-desktop
flutter doctor -v

# --- 8. vcpkg на baseline (нативные libs ставятся agent'ом из vcpkg.json при сборке) ---
Log "clone vcpkg @ $VCPKG_BASELINE"
if (-not (Test-Path C:\vcpkg)) {
    $vcpkgKit = if ($KitPath -and (Test-Path (Join-Path $KitPath 'vcpkg'))) { Join-Path $KitPath 'vcpkg' } else { $null }
    if ($vcpkgKit) { Copy-Item $vcpkgKit C:\vcpkg -Recurse } else { git clone https://github.com/microsoft/vcpkg C:\vcpkg }
}
Push-Location C:\vcpkg; git checkout $VCPKG_BASELINE; .\bootstrap-vcpkg.bat -disableMetrics; Pop-Location
[Environment]::SetEnvironmentVariable('VCPKG_ROOT', 'C:\vcpkg', 'Machine')
[Environment]::SetEnvironmentVariable('VCPKG_DEFAULT_TRIPLET', 'x64-windows-static', 'Machine')

Log "=== DONE. Перелогиньтесь (или перезапустите сессию) для применения PATH/env. ==="
Log "Далее: настройте SMB-доступ к job-очереди и запустите agent.ps1 (см. README.md)."
