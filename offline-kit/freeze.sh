#!/usr/bin/env bash
# offline-kit/freeze.sh — "online freeze stage" (PLAN.md §6, §8.1).
#
# Выкачивает и замораживает ВСЁ, нужное для суверенной offline-сборки
# Windows Flutter-клиента, ПОКА upstream rustdesk/rustdesk жив.
#
# Запускать в окружении с git + cargo (например, внутри контейнера
# docker-build-linux-1, либо в WSL/Linux с установленным Rust). Все стадии
# идемпотентны: уже скачанное пропускается, можно перезапускать после обрыва.
#
# НЕ запускает тяжёлый vcpkg install (ffmpeg/hwcodec) — это делается на
# Windows-билдере (PLAN.md §8.3). Здесь только git-checkout vcpkg на baseline.
#
# Использование:
#   bash freeze.sh                 # все стадии
#   bash freeze.sh vendor          # только стадия vendor
#   RUSTDESK_REPO=... bash freeze.sh   # переопределить источник (downstream-форк)

set -uo pipefail

cd "$(dirname "$0")"
# shellcheck disable=SC1091
source ./versions.env

# ENV переопределяет значения из versions.env (для downstream-форкеров)
RUSTDESK_REPO="${RUSTDESK_REPO:?}"
RUSTDESK_REF="${RUSTDESK_REF:?}"

OUT="$ARTIFACTS_DIR"
SRC="$OUT/rustdesk-src"
MANIFEST="$OUT/MANIFEST.txt"
mkdir -p "$OUT"

log()  { echo "[freeze] $*"; }
warn() { echo "[freeze][WARN] $*" >&2; }
die()  { echo "[freeze][FAIL] $*" >&2; exit 1; }

have() { command -v "$1" >/dev/null 2>&1; }

record() {
    # record <label> <path> — идемпотентно: заменяет существующую строку этого label
    local label="$1" path="$2" line=""
    if [ -f "$path" ]; then
        local sum size
        sum=$( (sha256sum "$path" 2>/dev/null || shasum -a 256 "$path") | awk '{print $1}')
        size=$(du -h "$path" | awk '{print $1}')
        line="$label  $size  sha256:$sum  $path"
    elif [ -d "$path" ]; then
        local size
        size=$(du -sh "$path" | awk '{print $1}')
        line="$label  $size  (dir)  $path"
    else
        return
    fi
    # убрать прежнюю строку с этим label (первое поле), затем дописать свежую
    if [ -f "$MANIFEST" ]; then
        grep -v "^$label  " "$MANIFEST" > "$MANIFEST.tmp" 2>/dev/null && mv "$MANIFEST.tmp" "$MANIFEST"
    fi
    echo "$line" >> "$MANIFEST"
}

dl() {
    # dl <url> <output> — resumable download, skip if present & non-empty
    local url="$1" out="$2"
    if [ -s "$out" ]; then log "skip (exists): $out"; return 0; fi
    log "download: $url"
    if have curl; then
        curl -fSL --retry 3 -C - -o "$out" "$url" || { warn "download failed: $url"; return 1; }
    elif have wget; then
        wget -c -O "$out" "$url" || { warn "download failed: $url"; return 1; }
    else
        die "neither curl nor wget available"
    fi
}

# ---------------------------------------------------------------------------
stage_source() {
    log "=== STAGE: source (git clone + submodules, pin $RUSTDESK_REF) ==="
    have git || die "git not found"
    # ВАЖНО: полный клон (НЕ --depth 1). Из shallow-клона git bundle --all получается
    # неполным ("remote did not send all necessary objects" при клоне обратно).
    if [ ! -d "$SRC/.git" ]; then
        git clone --branch "$RUSTDESK_REF" --recurse-submodules \
            "$RUSTDESK_REPO" "$SRC" || die "clone failed"
    else
        log "source already present at $SRC (если был shallow — bundle пересобери на полном клоне)"
    fi
    # git bundle — единый переносимый архив репозитория. Требует ПОЛНОЙ истории.
    local bundle="$OUT/rustdesk-$RUSTDESK_REF.bundle"
    if [ ! -s "$bundle" ]; then
        ( cd "$SRC" && git bundle create "../rustdesk-$RUSTDESK_REF.bundle" --all ) \
            && log "bundle created: $bundle" || warn "bundle failed"
    fi
    record "rustdesk-bundle" "$bundle"
}

stage_vendor() {
    log "=== STAGE: vendor (cargo vendor → замораживает hbb_common + ~20 rustdesk-org/*) ==="
    [ -d "$SRC" ] || die "run stage_source first (no $SRC)"
    have cargo || die "cargo not found — запусти в build-linux контейнере или WSL с Rust"
    if [ ! -d "$SRC/vendor" ]; then
        ( cd "$SRC" && cargo vendor vendor > .cargo/config.vendor.toml 2>/dev/null ) \
            || warn "cargo vendor вернул ненулевой код (проверь .cargo/config.vendor.toml)"
    else
        log "vendor already present"
    fi
    # tarball для хранения отдельно от git
    local tar="$OUT/vendor-$RUSTDESK_REF.tar.zst"
    if [ ! -s "$tar" ] && [ -d "$SRC/vendor" ]; then
        if have zstd; then
            tar -C "$SRC" -cf - vendor | zstd -q -o "$tar" && log "vendor tarball: $tar"
        else
            tar="$OUT/vendor-$RUSTDESK_REF.tar.gz"
            tar -C "$SRC" -czf "$tar" vendor && log "vendor tarball (gz): $tar"
        fi
    fi
    record "vendor-tarball" "$tar"
}

stage_engine() {
    log "=== STAGE: flutter engine (кастомный rustdesk, Windows x64) ==="
    dl "$FLUTTER_ENGINE_WIN_URL" "$OUT/windows-x64-release.zip"
    record "flutter-engine-win" "$OUT/windows-x64-release.zip"
}

stage_flutter_sdk() {
    log "=== STAGE: flutter SDK ($FLUTTER_VERSION) ==="
    dl "$FLUTTER_SDK_WIN_URL"   "$OUT/flutter_windows_$FLUTTER_VERSION-stable.zip"
    dl "$FLUTTER_SDK_LINUX_URL" "$OUT/flutter_linux_$FLUTTER_VERSION-stable.tar.xz"
    record "flutter-sdk-win"   "$OUT/flutter_windows_$FLUTTER_VERSION-stable.zip"
    record "flutter-sdk-linux" "$OUT/flutter_linux_$FLUTTER_VERSION-stable.tar.xz"
}

stage_vcpkg() {
    log "=== STAGE: vcpkg (checkout на baseline $VCPKG_BASELINE) ==="
    local vdir="$OUT/vcpkg"
    if [ ! -d "$vdir/.git" ]; then
        git clone "$VCPKG_REPO" "$vdir" || { warn "vcpkg clone failed"; return 1; }
    fi
    ( cd "$vdir" && git fetch --depth 1 origin "$VCPKG_BASELINE" 2>/dev/null; \
      git checkout "$VCPKG_BASELINE" 2>/dev/null ) \
        || warn "vcpkg checkout $VCPKG_BASELINE не удался (нужен полный fetch)"
    log "vcpkg готов на baseline. ВНИМАНИЕ: binary cache (ffmpeg/hwcodec, триплет"
    log "$VCPKG_TRIPLET) собирается на Windows-билдере, не здесь (PLAN.md §8.3)."
    record "vcpkg-src" "$vdir"
}

stage_rust() {
    log "=== STAGE: rust toolchain offline installer ($RUST_VERSION) ==="
    # rustup-init + offline-инсталлятор для Windows-хоста (MSVC host).
    # Для полного offline нужен host x86_64-pc-windows-msvc; здесь качаем dist-архив.
    local base="https://static.rust-lang.org/dist"
    dl "$base/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi" \
       "$OUT/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi" \
        || warn "rust msi: если недоступен, ставь через rustup на билдере"
    record "rust-win-msvc" "$OUT/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi"
}

stage_thirdparty() {
    log "=== STAGE: thirdparty (TopMostWindow src + usbmmidd + printer drivers) ==="
    # RustDeskTempTopMostWindow — собирается на win-агенте, замораживаем исходники
    local tmw="$OUT/RustDeskTempTopMostWindow"
    if [ ! -d "$tmw/.git" ]; then
        git clone "$TOPMOST_REPO" "$tmw" || warn "TopMostWindow clone failed"
    fi
    if [ -d "$tmw/.git" ]; then
        ( cd "$tmw" && git checkout "$TOPMOST_COMMIT" 2>/dev/null ) || warn "TopMost checkout $TOPMOST_COMMIT failed"
        ( cd "$tmw" && git bundle create "../RustDeskTempTopMostWindow.bundle" --all ) 2>/dev/null
    fi
    record "topmost-bundle" "$OUT/RustDeskTempTopMostWindow.bundle"
    # release-assets (бинарные)
    dl "$USBMMIDD_URL"        "$OUT/usbmmidd_v2.zip"
    dl "$PRINTER_DRIVER_URL"  "$OUT/rustdesk_printer_driver_v4-1.4.zip"
    dl "$PRINTER_ADAPTER_URL" "$OUT/printer_driver_adapter.zip"
    dl "$PRINTER_SUMS_URL"    "$OUT/printer_sha256sums"
    record "usbmmidd"         "$OUT/usbmmidd_v2.zip"
    record "printer-driver"   "$OUT/rustdesk_printer_driver_v4-1.4.zip"
    record "printer-adapter"  "$OUT/printer_driver_adapter.zip"
}

write_manifest_header() {
    # Заголовок пишется только если манифеста нет — частичные прогоны не затирают записи.
    [ -f "$MANIFEST" ] && return
    {
        echo "# offline-kit MANIFEST — суверенный комплект сборки rustdesk $RUSTDESK_REF"
        echo "# Создан: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        echo "# Источник: $RUSTDESK_REPO @ $RUSTDESK_REF"
        echo "# Пины: Rust $RUST_VERSION, Flutter $FLUTTER_VERSION, LLVM $LLVM_VERSION,"
        echo "#       vcpkg baseline $VCPKG_BASELINE, triplet $VCPKG_TRIPLET"
        echo "#"
        echo "# label  size  checksum  path"
    } > "$MANIFEST"
}

main() {
    write_manifest_header
    local stages=("$@")
    if [ ${#stages[@]} -eq 0 ]; then
        stages=(source vendor engine flutter_sdk vcpkg rust thirdparty)
    fi
    for s in "${stages[@]}"; do
        case "$s" in
            source)      stage_source ;;
            vendor)      stage_vendor ;;
            engine)      stage_engine ;;
            flutter_sdk) stage_flutter_sdk ;;
            vcpkg)       stage_vcpkg ;;
            rust)        stage_rust ;;
            thirdparty)  stage_thirdparty ;;
            *) warn "unknown stage: $s" ;;
        esac
    done
    log "=== ГОТОВО. Манифест: $MANIFEST ==="
    [ -f "$MANIFEST" ] && cat "$MANIFEST"
}

main "$@"
