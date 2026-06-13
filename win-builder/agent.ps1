# ============================================================================
# win-builder/agent.ps1 — build-агент на нативном Windows Server (PLAN.md §8.3/§8.4)
# ============================================================================
# Поллит SMB-папку job-очереди, собирает актуальный Flutter Windows-клиент с тремя
# слоями вшивания конфига → rustqs.exe, кладёт результат обратно в общую папку.
#
# Запуск вручную:
#   powershell -ExecutionPolicy Bypass -File agent.ps1 -DataRoot Z:\rdgen-data -KitPath D:\offline-kit\artifacts
# Как служба: зарегистрировать через Scheduled Task "At startup" (см. README.md).
#
# -DataRoot : корень общей очереди (SMB-маунт). Внутри: jobs\*.json, output\, patches\.
# -KitPath  : offline-kit artifacts (bundle, engine, drivers). Без него — онлайн-fallback.
#
# ⚠️ DESIGN-АРТЕФАКТ, НЕ ПРОВЕРЕН. [VERIFY] = места риска. Последовательность сборки —
# официальная: build.py --portable --hwcodec --flutter --vram + генерация bridge.
# ============================================================================
param(
    [Parameter(Mandatory=$true)][string]$DataRoot,
    [string]$KitPath = "",
    [string]$WorkRoot = "C:\rustdesk-build"
)
$ErrorActionPreference = 'Continue'

$JOBS    = Join-Path $DataRoot 'jobs'
$OUTPUT  = Join-Path $DataRoot 'output'
$PATCHES = Join-Path $DataRoot 'patches'   # vendored из rdgen/.github/patches (allowCustom и др.)
New-Item -ItemType Directory -Force -Path $JOBS,$OUTPUT,$WorkRoot | Out-Null

function Log($m) { Write-Host "[agent $((Get-Date).ToString('HH:mm:ss'))] $m" }
function KitFile($n) { if ($KitPath -and (Test-Path (Join-Path $KitPath $n))) { return (Join-Path $KitPath $n) } return $null }
function Step($desc, $script) { Log ">>> $desc"; & $script; if ($LASTEXITCODE -ne 0) { throw "step failed ($LASTEXITCODE): $desc" } }

Log "Windows build agent started. DataRoot=$DataRoot Kit=$KitPath"

function Process-Job($jobFile) {
    $jobId = [IO.Path]::GetFileNameWithoutExtension($jobFile)
    $job   = Get-Content $jobFile -Raw | ConvertFrom-Json
    if ($job.platform -ne 'windows') { Log "skip $($job.platform)"; Remove-Item $jobFile -Force; return }

    $outDir = Join-Path $OUTPUT $jobId
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null
    'building' | Set-Content (Join-Path $outDir 'status')
    $buildLog = Join-Path $outDir 'build.log'

    $srcRepo = if ($job.src_repo) { $job.src_repo } else { 'https://github.com/rustdesk/rustdesk.git' }
    $srcRef  = if ($job.src_ref)  { $job.src_ref }  else { '1.4.7' }
    $server  = $job.server; $key = $job.key
    $appName = if ($job.app_name) { $job.app_name } else { 'rustqs' }
    $customTxtB64 = $job.custom_txt

    $work = Join-Path $WorkRoot $jobId
    if (Test-Path $work) { Remove-Item $work -Recurse -Force }

    try {
        # --- Исходники: из offline bundle или clone ---
        $bundle = KitFile 'rustdesk-1.4.7.bundle'
        if ($bundle) {
            Step "clone from offline bundle" { git clone $bundle $work }
            Push-Location $work; git checkout $srcRef; git submodule update --init --recursive; Pop-Location
        } else {
            Step "git clone $srcRepo @ $srcRef" { git clone --branch $srcRef --recurse-submodules $srcRepo $work }
        }
        Push-Location $work

        # offline vendor (если в kit есть tar) → .cargo/config.toml на vendored-sources
        $vendorTar = KitFile 'vendor-1.4.7.tar.gz'
        if ($vendorTar) {
            Step "extract vendor" { tar -xf $vendorTar }
            New-Item -ItemType Directory -Force -Path .cargo | Out-Null
            "[source.crates-io]`nreplace-with = `"vendored-sources`"`n[source.vendored-sources]`ndirectory = `"vendor`"" | Set-Content .cargo\config.toml
        }

        # ===== L1: сервер + ключ в config.rs =====
        if ($server) { (Get-Content libs/hbb_common/src/config.rs) -replace 'rs-ny\.rustdesk\.com',$server | Set-Content libs/hbb_common/src/config.rs; Log "L1 server=$server" }
        if ($key)    { (Get-Content libs/hbb_common/src/config.rs) -replace 'OeVuKk5nlHiXp\+APNn0Y3pC1Iwpwn44JGqrQCsWqmBw=',$key | Set-Content libs/hbb_common/src/config.rs; Log "L1 key embedded" }

        # ===== L3: брендинг → rustqs (сокращённо; полный набор — rdgen/generator-windows.yml:161-241) =====
        foreach ($f in @('Cargo.toml','libs/portable/Cargo.toml')) {
            if (Test-Path $f) { (Get-Content $f) -replace 'ProductName = "RustDesk"',"ProductName = `"$appName`"" -replace 'OriginalFilename = "rustdesk.exe"',"OriginalFilename = `"$appName.exe`"" | Set-Content $f }
        }
        foreach ($f in @('flutter/pubspec.lock','flutter/pubspec.yaml')) {  # fix flutter_gpu_texture_renderer (3.24.x)
            if (Test-Path $f) { (Get-Content $f) -replace '2ded7f146437a761ffe6981e2f742038f85ca68d','08a471bb8ceccdd50483c81cdfa8b81b07b14b87' | Set-Content $f }
        }
        Log "L3 branding=$appName"

        # ===== L2: allowCustom + подписанный custom.txt =====
        $allow = Join-Path $PATCHES 'allowCustom.py'
        if (Test-Path $allow) { Step "L2 allowCustom" { python $allow } } else { Log "[VERIFY] allowCustom.py нет в $PATCHES — custom.txt не примут" }
        if ($customTxtB64) { [IO.File]::WriteAllBytes((Join-Path $work 'custom.txt'), [Convert]::FromBase64String($customTxtB64)); Log "L2 custom.txt" }

        # ===== Сторонние артефакты из kit (engine, usbmmidd, printer, TopMostWindow) =====
        $engine = KitFile 'windows-x64-release.zip'
        if ($engine) {
            $engDir = "C:\flutter\bin\cache\artifacts\engine\windows-x64-release"
            Expand-Archive -Path $engine -DestinationPath $env:TEMP\eng -Force
            Copy-Item "$env:TEMP\eng\*" $engDir -Recurse -Force; Log "custom flutter engine installed"
        } else { Log "[VERIFY] engine из kit не найден" }
        foreach ($pair in @(@('usbmmidd_v2.zip','.'), @('rustdesk_printer_driver_v4-1.4.zip','.'), @('printer_driver_adapter.zip','.'))) {
            $z = KitFile $pair[0]; if ($z) { Expand-Archive -Path $z -DestinationPath $work -Force }
        }
        # [VERIFY] RustDeskTempTopMostWindow: собрать msbuild'ом из kit-бандла и положить артефакт
        # рядом (см. third-party-RustDeskTempTopMostWindow.yml). TODO при первом тесте.

        # ===== Сборка =====
        Step "vcpkg install (manifest from vcpkg.json)" { & "$env:VCPKG_ROOT\vcpkg.exe" install --triplet x64-windows-static --x-install-root="$env:VCPKG_ROOT\installed" }
        Step "flutter_rust_bridge_codegen" { flutter_rust_bridge_codegen --rust-input ./src/flutter_ffi.rs --dart-output ./flutter/lib/generated_bridge.dart --c-output ./flutter/macos/Runner/bridge_generated.h }
        Step "flutter pub get" { Push-Location flutter; flutter pub get; Pop-Location }
        Step "build.py (flutter windows + portable)" { python .\build.py --portable --hwcodec --flutter --vram 2>&1 | Tee-Object $buildLog }

        # ===== Результат → rustqs.exe =====
        $inst = Get-ChildItem -Path . -Filter 'rustdesk-*-install.exe' -Recurse | Select-Object -First 1
        if ($inst) {
            Copy-Item $inst.FullName (Join-Path $outDir "$appName.exe")
            'done' | Set-Content (Join-Path $outDir 'status'); Log "Job ${jobId}: SUCCESS → $appName.exe"
        } else { throw "installer not found after build" }
    }
    catch {
        Log "Job ${jobId}: FAILED — $_"
        'failed' | Set-Content (Join-Path $outDir 'status'); $_ | Out-File -Append $buildLog
    }
    finally {
        Pop-Location -ErrorAction SilentlyContinue
        Remove-Item $jobFile -Force -ErrorAction SilentlyContinue
    }
}

while ($true) {
    Get-ChildItem -Path $JOBS -Filter '*.json' -ErrorAction SilentlyContinue | ForEach-Object { Process-Job $_.FullName }
    Start-Sleep -Seconds 5
}
