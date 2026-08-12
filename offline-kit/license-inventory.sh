#!/usr/bin/env bash
# Deterministic third-party license evidence inventory.
# It reports evidence paths only; it never invents a license conclusion.

set -u -o pipefail
export LC_ALL=C

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd -P)
_root_override=${1-}
[ -n "$_root_override" ] && ROOT=$(CDPATH= cd -- "$_root_override" && pwd -P)

case "$ROOT" in
    "$SCRIPT_DIR/artifacts"|"$SCRIPT_DIR/artifacts"/*)
        printf '%s\n' "license inventory refuses to scan offline-kit/artifacts" >&2
        exit 2
        ;;
esac

# shellcheck disable=SC1091
source "$SCRIPT_DIR/versions.env"

is_secret_value() {
    case "$1" in
        *.key|*.pem|*.p12|*.pfx|*/.env|.env|*workflow-payload*|*://*@*) return 0 ;;
        *) return 1 ;;
    esac
}

display_value() {
    local value=$1 rest host
    if is_secret_value "$value"; then
        case "$value" in
            *://*@*)
                rest=${value#*://}
                host=${rest#*@}
                printf '%s\n' "[redacted]@$host"
                ;;
            *) printf '%s\n' '[redacted]' ;;
        esac
    else
        printf '%s\n' "$value"
    fi
}

printf '%s\n' "# DeskForge offline-kit third-party evidence inventory"
printf '%s\n' "# Deterministic output; no signatures or license conclusions are claimed."
printf '%s\n' "# Status is COVERED only when a license evidence path is present; otherwise GAP."
printf '%s\n' "# Overall coverage: INCOMPLETE; GAP entries are never promoted to complete coverage."
printf '%s\n' "component|source or pin|license evidence|status"

root_license=""
for candidate in LICENSE LICENCE COPYING NOTICE; do
    if [ -f "$ROOT/$candidate" ]; then root_license=$candidate; break; fi
done

if [ -n "$root_license" ]; then
    printf '%s\n' "DeskForge|tracked repository|$root_license|COVERED"
else
    printf '%s\n' "DeskForge|tracked repository|no tracked license evidence|GAP"
fi

printf '%s\n' "RustDesk client|$(display_value "${RUSTDESK_REPO}@${RUSTDESK_COMMIT}")|ignored local source bundle not scanned|GAP"
printf '%s\n' "hbb_common|$(display_value "$HBB_COMMON_REPO")|no pinned license evidence in offline-kit|GAP"
printf '%s\n' "RustDesk vendored crates|vendor-$RUSTDESK_REF.tar.*|ignored local vendor payload not scanned|GAP"
printf '%s\n' "Flutter custom engine|$(display_value "$FLUTTER_ENGINE_WIN_URL")|provider asset license evidence not supplied|GAP"
printf '%s\n' "Flutter SDK Windows|$(display_value "$FLUTTER_SDK_WIN_URL")|provider asset license evidence not supplied|GAP"
printf '%s\n' "Flutter SDK Linux|$(display_value "$FLUTTER_SDK_LINUX_URL")|provider asset license evidence not supplied|GAP"
printf '%s\n' "vcpkg|$(display_value "${VCPKG_REPO}@${VCPKG_BASELINE}")|ignored local source not scanned|GAP"
printf '%s\n' "Rust Windows MSI|rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi|provider asset license evidence not supplied|GAP"
printf '%s\n' "RustDeskTempTopMostWindow|$(display_value "${TOPMOST_REPO}@${TOPMOST_COMMIT}")|provider source license evidence not supplied|GAP"
printf '%s\n' "usbmmidd|$(display_value "$USBMMIDD_URL")|provider asset license evidence not supplied|GAP"
printf '%s\n' "RustDesk printer driver|$(display_value "$PRINTER_DRIVER_URL")|provider asset license evidence not supplied|GAP"
printf '%s\n' "RustDesk printer adapter|$(display_value "$PRINTER_ADAPTER_URL")|provider asset license evidence not supplied|GAP"
printf '%s\n' "RustDesk printer checksum sidecar|printer_sha256sums|provider asset license evidence not supplied|GAP"
