#!/usr/bin/env bash
# Read-only, fail-closed verifier for offline-kit.

set -u -o pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
cd "$SCRIPT_DIR"
_artifacts_override=${ARTIFACTS_DIR-}
_repo_override=${RUSTDESK_REPO-}
_ref_override=${RUSTDESK_REF-}
# shellcheck disable=SC1091
source ./versions.env
[ -n "$_artifacts_override" ] && ARTIFACTS_DIR="$_artifacts_override"
[ -n "$_repo_override" ] && RUSTDESK_REPO="$_repo_override"
[ -n "$_ref_override" ] && RUSTDESK_REF="$_ref_override"
ARTIFACTS_DIR=${ARTIFACTS_DIR:?}
RUSTDESK_REPO=${RUSTDESK_REPO:?}
RUSTDESK_REF=${RUSTDESK_REF:?}
RUSTDESK_COMMIT=${RUSTDESK_COMMIT-}
RUST_VERSION=${RUST_VERSION-}
LLVM_VERSION=${LLVM_VERSION-}
FLUTTER_VERSION=${FLUTTER_VERSION-}
FLUTTER_RUST_BRIDGE_VERSION=${FLUTTER_RUST_BRIDGE_VERSION-}
NDK_VERSION=${NDK_VERSION-}
CARGO_NDK_VERSION=${CARGO_NDK_VERSION-}
VCPKG_BASELINE=${VCPKG_BASELINE-}
VCPKG_TRIPLET=${VCPKG_TRIPLET-}
TOPMOST_COMMIT=${TOPMOST_COMMIT-}
GIT_CHECK_TIMEOUT_SECONDS=${GIT_CHECK_TIMEOUT_SECONDS:-10}
MANIFEST_SCHEMA_VERSION=2
OUT=$ARTIFACTS_DIR
SRC="$OUT/rustdesk-src"
MANIFEST="$OUT/MANIFEST.txt"
KIT_ROOT="$SCRIPT_DIR"
ERRORS=0
MANIFEST_COMPATIBLE=1

ok()   { printf '%s\n' "[verify][OK] $*"; }
fail() { printf '%s\n' "[verify][FAIL] $*" >&2; ERRORS=$((ERRORS + 1)); }
have() { command -v "$1" >/dev/null 2>&1; }

usage() {
    printf '%s\n' "Usage: bash verify.sh [--manifest PATH]"
    printf '%s\n' "Read-only: never downloads, rewrites MANIFEST, or accepts TOFU."
}

file_size() {
    if stat -c '%s' "$1" 2>/dev/null; then return 0; fi
    wc -c < "$1" | tr -d '[:space:]'
}

dir_size() {
    if du -sb "$1" 2>/dev/null | awk '{print $1}'; then return 0; fi
    du -sk "$1" | awk '{print $1 * 1024}'
}

git_bounded() {
    timeout "$GIT_CHECK_TIMEOUT_SECONDS" git "$@"
}

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        fail "sha256sum or shasum is required"
        printf '%s\n' ""
    fi
}

content_hash_dir() {
    local dir=$1 listing file rel digest rc bad
    bad=$(LC_ALL=C find -P "$dir" \( -type l -o \( ! -type f ! -type d \) \) -print -quit 2>/dev/null) || return 1
    [ -z "$bad" ] || return 1
    listing=$(mktemp "${TMPDIR:-/tmp}/offline-kit-dir.XXXXXX") || return 1
    (
        cd "$dir" || exit 1
        LC_ALL=C find -P . \( -type f -o -type l \) -print | LC_ALL=C sort |
            while IFS= read -r file; do
                rel=${file#./}
                case "$rel" in
                    *$'\n'*) exit 1 ;;
                esac
                is_secret_path "$rel" && exit 1
                [ -f "$file" ] || exit 1
                digest=$(sha256_file "$file") || exit 1
                printf 'F\t%s\t%s\n' "$rel" "$digest"
            done
    ) > "$listing"
    rc=$?
    if [ "$rc" -eq 0 ]; then
        sha256_file "$listing"
        rc=$?
    fi
    rm -f "$listing"
    return "$rc"
}

directory_record() {
    local path=$1 tree
    if tree=$(git_bounded -C "$path" rev-parse --verify --quiet HEAD^{tree} 2>/dev/null); then
        printf 'git-tree:%s\n' "$tree"
    else
        tree=$(content_hash_dir "$path") || return 1
        printf 'content-sha256:%s\n' "$tree"
    fi
}

is_secret_path() {
    case "$1" in
        custom_.txt|*/custom_.txt|*.key|*.pem|*.p12|*.pfx|*/.env|.env|*workflow-payload*) return 0 ;;
        *) return 1 ;;
    esac
}

is_generated_source_path() {
    case "$1" in
        vendor|vendor/*|.cargo/config.vendor.toml) return 0 ;;
        *) return 1 ;;
    esac
}

canonical_path() {
    if command -v realpath >/dev/null 2>&1; then
        realpath -m "$1"
    else
        case "$1" in
            /*) printf '%s\n' "$1" ;;
            *) printf '%s/%s\n' "$SCRIPT_DIR" "$1" ;;
        esac
    fi
}

path_has_symlink() {
    local path=$1 absolute component current
    case "$path" in
        /*) absolute=$path ;;
        *) absolute="$SCRIPT_DIR/$path" ;;
    esac
    current=
    IFS=/ read -r -a components <<< "${absolute#/}"
    for component in "${components[@]}"; do
        [ -n "$component" ] || continue
        current="$current/$component"
        [ -L "$current" ] && return 0
    done
    return 1
}

validate_existing_path_components() {
    local path=$1 absolute component current
    case "$path" in
        /*) absolute=$path ;;
        *) absolute="$SCRIPT_DIR/$path" ;;
    esac
    current=
    IFS=/ read -r -a components <<< "${absolute#/}"
    for component in "${components[@]}"; do
        [ -n "$component" ] || continue
        current="$current/$component"
        if [ -L "$current" ]; then
            return 1
        fi
        if [ -e "$current" ] && [ ! -d "$current" ] && [ "$current" != "$absolute" ]; then
            return 1
        fi
    done
    return 0
}

validate_path_location() {
    local label=$1 path=$2 canonical
    case "$path" in
        *$'\n'*) fail "$label path contains a newline"; return 1 ;;
    esac
    canonical=$(canonical_path "$path") || return 1
    case "$canonical" in
        "$KIT_ROOT"|"$KIT_ROOT"/*) ;;
        *) fail "$label path escapes kit root ([redacted path])"; return 1 ;;
    esac
    if ! validate_existing_path_components "$path"; then
        fail "$label path contains a symlink ([redacted path])"
        return 1
    fi
    return 0
}

validate_regular_tree() {
    local label=$1 path=$2 bad
    bad=$(LC_ALL=C find -P "$path" \( -type l -o \( ! -type f ! -type d \) \) -print -quit 2>/dev/null) || {
        fail "$label cannot be inspected safely"
        return 1
    }
    [ -z "$bad" ] || {
        fail "$label contains a symlink or non-regular entry"
        return 1
    }
    return 0
}

validate_expected_path() {
    local label=$1 path=$2 kind=$3
    validate_path_location "$label" "$path" || return 1
    if [ -L "$path" ]; then
        fail "$label is a symlink; expected a regular $kind"
        return 1
    fi
    if [ -e "$path" ]; then
        case "$kind" in
            file|generated-file)
                [ -f "$path" ] || { fail "$label is not a regular file"; return 1; } ;;
            git-tree|content-sha256)
                [ -d "$path" ] || { fail "$label is not a regular directory"; return 1; }
                validate_regular_tree "$label" "$path" || return 1 ;;
            *) fail "$label has an unknown manifest kind"; return 1 ;;
        esac
    fi
    return 0
}

validate_file_target() {
    local label=$1 path=$2
    validate_path_location "$label" "$path" || return 1
    if [ -L "$path" ]; then
        fail "$label is a symlink"
        return 1
    fi
    if [ -e "$path" ] && [ ! -f "$path" ]; then
        fail "$label is not a regular file"
        return 1
    fi
    return 0
}

validate_optional_directory() {
    local label=$1 path=$2
    validate_path_location "$label" "$path" || return 1
    if [ -e "$path" ]; then
        [ -d "$path" ] || {
            fail "$label is not a regular directory"
            return 1
        }
        validate_regular_tree "$label" "$path"
    fi
}

check_secret_presence() {
    local secret
    secret=$(LC_ALL=C find -P "$KIT_ROOT" \( -type f -o -type l \) \
        \( -name 'custom_.txt' -o -name '*.key' -o -name '*.pem' -o -name '*.p12' -o -name '*.pfx' \
        -o -name '.env' -o -name 'workflow-payload*' \) -print -quit 2>/dev/null) || {
        fail "secret-bearing path scan could not be completed"
        return
    }
    [ -z "$secret" ] || fail "secret-bearing local file detected under kit/artifact handoff ([redacted path]); refusing secret exclusion"
}

expected_digest_for() {
    case "$1" in
        "$OUT/rustdesk-$RUSTDESK_REF.bundle") printf '%s\n' "${RUSTDESK_BUNDLE_SHA256-}" ;;
        "$OUT/vendor-$RUSTDESK_REF.tar.zst"|"$OUT/vendor-$RUSTDESK_REF.tar.gz") printf '%s\n' "${VENDOR_TARBALL_SHA256-}" ;;
        "$OUT/windows-x64-release.zip") printf '%s\n' "${FLUTTER_ENGINE_WIN_SHA256-}" ;;
        "$OUT/flutter_windows_$FLUTTER_VERSION-stable.zip") printf '%s\n' "${FLUTTER_SDK_WIN_SHA256-}" ;;
        "$OUT/flutter_linux_$FLUTTER_VERSION-stable.tar.xz") printf '%s\n' "${FLUTTER_SDK_LINUX_SHA256-}" ;;
        "$OUT/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi") printf '%s\n' "${RUST_WIN_MSI_SHA256-}" ;;
        "$OUT/RustDeskTempTopMostWindow.bundle") printf '%s\n' "${TOPMOST_BUNDLE_SHA256-}" ;;
        "$OUT/usbmmidd_v2.zip") printf '%s\n' "${USBMMIDD_SHA256-}" ;;
        "$OUT/rustdesk_printer_driver_v4-1.4.zip") printf '%s\n' "${PRINTER_DRIVER_SHA256-}" ;;
        "$OUT/printer_driver_adapter.zip") printf '%s\n' "${PRINTER_ADAPTER_SHA256-}" ;;
        "$OUT/printer_sha256sums") printf '%s\n' "${PRINTER_SUMS_SHA256-}" ;;
        *) printf '%s\n' "" ;;
    esac
}

rows() {
    local vendor_path="$OUT/vendor-$RUSTDESK_REF.tar.gz"
    [ -f "$OUT/vendor-$RUSTDESK_REF.tar.zst" ] && vendor_path="$OUT/vendor-$RUSTDESK_REF.tar.zst"
    printf '%s\t%s\t%s\t%s\n' rustdesk-source "$OUT/rustdesk-src" git-tree ""
    printf '%s\t%s\t%s\t%s\n' rustdesk-bundle "$OUT/rustdesk-$RUSTDESK_REF.bundle" file "${RUSTDESK_BUNDLE_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' vendor-source "$OUT/rustdesk-src/vendor" content-sha256 ""
    printf '%s\t%s\t%s\t%s\n' vendor-config "$OUT/rustdesk-src/.cargo/config.vendor.toml" generated-file ""
    printf '%s\t%s\t%s\t%s\n' vendor-tarball "$vendor_path" file "${VENDOR_TARBALL_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' flutter-engine-win "$OUT/windows-x64-release.zip" file "${FLUTTER_ENGINE_WIN_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' flutter-sdk-win "$OUT/flutter_windows_$FLUTTER_VERSION-stable.zip" file "${FLUTTER_SDK_WIN_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' flutter-sdk-linux "$OUT/flutter_linux_$FLUTTER_VERSION-stable.tar.xz" file "${FLUTTER_SDK_LINUX_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' vcpkg-src "$OUT/vcpkg" git-tree ""
    printf '%s\t%s\t%s\t%s\n' rust-win-msvc "$OUT/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi" file "${RUST_WIN_MSI_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' topmost-src "$OUT/RustDeskTempTopMostWindow" git-tree ""
    printf '%s\t%s\t%s\t%s\n' topmost-bundle "$OUT/RustDeskTempTopMostWindow.bundle" file "${TOPMOST_BUNDLE_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' usbmmidd "$OUT/usbmmidd_v2.zip" file "${USBMMIDD_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' printer-driver "$OUT/rustdesk_printer_driver_v4-1.4.zip" file "${PRINTER_DRIVER_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' printer-adapter "$OUT/printer_driver_adapter.zip" file "${PRINTER_ADAPTER_SHA256-}"
    printf '%s\t%s\t%s\t%s\n' printer-sums "$OUT/printer_sha256sums" file "${PRINTER_SUMS_SHA256-}"
}

check_pins() {
    local key expected found
    [[ "$RUSTDESK_COMMIT" =~ ^[0-9a-fA-F]{40}$ ]] || fail "RUSTDESK_COMMIT is not a full commit id"
    [[ "$VCPKG_BASELINE" =~ ^[0-9a-fA-F]{40}$ ]] || fail "VCPKG_BASELINE is not a full commit id"
    for key in RUSTDESK_REPO RUSTDESK_REF RUST_VERSION LLVM_VERSION FLUTTER_VERSION \
        FLUTTER_RUST_BRIDGE_VERSION NDK_VERSION CARGO_NDK_VERSION VCPKG_TRIPLET; do
        eval "expected=\${$key-}"
        [ -n "$expected" ] || fail "required pin is empty: $key"
    done
    while IFS=$'\t' read -r _label _path kind expected; do
        [ "$_label" = rustdesk-source ] && continue
        [ "$kind" = file ] || continue
        if ! [[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]]; then
            fail "expected digest is missing or invalid for $_label"
        fi
    done < <(rows)
    [ -f "$MANIFEST" ] || return
    for key in RUSTDESK_REF RUSTDESK_COMMIT RUST_VERSION LLVM_VERSION FLUTTER_VERSION \
        FLUTTER_RUST_BRIDGE_VERSION NDK_VERSION CARGO_NDK_VERSION VCPKG_BASELINE VCPKG_TRIPLET; do
        eval "expected=\${$key-}"
        found=$(awk -v key="$key" '$0 ~ "^# pin " key "=" {sub("^# pin " key "=", ""); print; exit}' "$MANIFEST")
        [ "$found" = "$expected" ] || fail "manifest pin mismatch or missing: $key"
    done
}

check_manifest_compatibility() {
    local schema_count key pin_count pin_value
    if ! validate_path_location "manifest" "$MANIFEST"; then
        MANIFEST_COMPATIBLE=0
        return
    fi
    [ -f "$MANIFEST" ] || return
    schema_count=$(awk -v version="$MANIFEST_SCHEMA_VERSION" \
        '$0 == "# schema=" version { count++ } END { print count + 0 }' "$MANIFEST")
    if [ "$schema_count" -ne 1 ]; then
        fail "incompatible MANIFEST.txt (legacy or unsupported schema); migrate/re-freeze into a new empty artifact directory, then rerun"
        MANIFEST_COMPATIBLE=0
        return
    fi
    for key in RUSTDESK_REF RUSTDESK_COMMIT RUST_VERSION LLVM_VERSION FLUTTER_VERSION \
        FLUTTER_RUST_BRIDGE_VERSION NDK_VERSION CARGO_NDK_VERSION VCPKG_BASELINE VCPKG_TRIPLET; do
        pin_count=$(awk -v key="$key" '$0 ~ "^# pin " key "=" { count++ } END { print count + 0 }' "$MANIFEST")
        pin_value=$(awk -v key="$key" '$0 ~ "^# pin " key "=" { sub("^# pin " key "=", ""); print; exit }' "$MANIFEST")
        if [ "$pin_count" -ne 1 ] || [ -z "$pin_value" ]; then
            fail "incompatible MANIFEST.txt (missing or duplicate required pin record $key); migrate/re-freeze into a new empty artifact directory, then rerun"
            MANIFEST_COMPATIBLE=0
        fi
    done
}

check_manifest() {
    local label size checksum path rest expected_path expected_kind expected_digest actual record_path found actual_record
    declare -A expected_label seen_path seen_size seen_checksum seen_label
    validate_path_location "manifest" "$MANIFEST" || return
    [ -f "$MANIFEST" ] || { fail "manifest missing (local artifact set unavailable): $MANIFEST"; return; }
    while IFS=$'\t' read -r label expected_path expected_kind expected_digest; do
        expected_label["$label"]=$expected_kind
    done < <(rows)
    while IFS= read -r line || [ -n "$line" ]; do
        case "$line" in ''|\#*) continue ;; esac
        IFS=' ' read -r label size checksum path rest <<< "$line"
        [ -n "${rest-}" ] && fail "manifest has unexpected fields for $label"
        [ -n "${label-}" ] || { fail "manifest has an empty label"; continue; }
        [ -n "${expected_label[$label]-}" ] || { fail "manifest entry is unexpected: $label"; continue; }
        [ -z "${seen_label[$label]-}" ] || { fail "manifest entry is duplicated: $label"; continue; }
        if is_secret_path "${path-}"; then
            fail "manifest contains a secret-bearing entry; refusing to inspect it"
            continue
        fi
        seen_label["$label"]=1
        seen_path["$label"]=$path
        seen_size["$label"]=$size
        seen_checksum["$label"]=$checksum
    done < "$MANIFEST"
    while IFS=$'\t' read -r label expected_path expected_kind expected_digest; do
        record_path=${seen_path[$label]-}
        [ -n "$record_path" ] || { fail "manifest entry missing: $label"; continue; }
        [ "$(canonical_path "$record_path")" = "$(canonical_path "$expected_path")" ] || {
            fail "manifest path mismatch: $label"; continue;
        }
        path="$record_path"
        validate_expected_path "$label" "$path" "$expected_kind" || continue
        [ -e "$path" ] || {
            case "$label" in
                flutter-engine-win) fail "local-only engine asset missing: $label" ;;
                flutter-*|rust-win-msvc|usbmmidd|printer-*) fail "provider asset unavailable locally: $label" ;;
                *) fail "local-only artifact missing: $label" ;;
            esac
            continue
        }
        size=${seen_size[$label]}
        [[ "$size" =~ ^[0-9]+$ ]] || { fail "manifest size is not exact bytes: $label"; continue; }
        if { [ "$expected_kind" = file ] || [ "$expected_kind" = generated-file ]; } && [ -f "$path" ]; then
            [ "$(file_size "$path")" = "$size" ] || fail "manifest size mismatch: $label"
            checksum=${seen_checksum[$label]}
            [[ "$checksum" =~ ^sha256:[0-9a-fA-F]{64}$ ]] || { fail "manifest SHA-256 missing/invalid: $label"; continue; }
            actual=$(sha256_file "$path")
            [ "sha256:$actual" = "$checksum" ] || fail "manifest SHA-256 mismatch: $label"
            if [ "$expected_kind" = file ]; then
                [ -n "$expected_digest" ] || fail "expected digest missing: $label"
                [ "$actual" = "$expected_digest" ] || fail "expected digest mismatch: $label"
            fi
        elif [ -d "$path" ]; then
            checksum=${seen_checksum[$label]}
            if [ "$expected_kind" = git-tree ] && [[ "$checksum" =~ ^git-tree:[0-9a-fA-F]{40}$ ]]; then
                actual_record=$(git_bounded -C "$path" rev-parse --verify --quiet HEAD^{tree} 2>/dev/null) || actual_record=""
                [ "git-tree:$actual_record" = "$checksum" ] || fail "manifest Git tree mismatch: $label"
            elif [ "$expected_kind" = content-sha256 ] && [[ "$checksum" =~ ^content-sha256:[0-9a-fA-F]{64}$ ]]; then
                actual_record=$(content_hash_dir "$path") || actual_record=""
                [ "content-sha256:$actual_record" = "$checksum" ] || fail "manifest content hash mismatch: $label"
            else
                fail "manifest content record missing/invalid: $label"
            fi
            [[ "$size" =~ ^[0-9]+$ ]] || fail "manifest directory size is not exact bytes: $label"
            [ "$(dir_size "$path")" = "$size" ] || fail "manifest directory size mismatch: $label"
        else
            fail "manifest path is neither file nor directory: $label"
        fi
    done < <(rows)
}

verify_bundle_commit() {
    local bundle=$1 commit=$2 tmp bundle_path rc=0
    [[ "$commit" =~ ^[0-9a-fA-F]{40}$ ]] || { fail "bundle commit pin is not a full commit id"; return; }
    [ -f "$bundle" ] || { fail "bundle missing: $bundle"; return; }
    have git || { fail "git is required for bundle verification"; return; }
    have timeout || { fail "timeout is required for bounded git checks"; return; }
    bundle_path=$(canonical_path "$bundle")
    tmp=$(mktemp -d "${TMPDIR:-/tmp}/offline-kit-bundle.XXXXXX") || { fail "cannot create temporary bundle verification directory"; return; }
    git_bounded init -q "$tmp/repo" >/dev/null 2>&1 || rc=1
    if [ "$rc" -eq 0 ]; then
        git_bounded -C "$tmp/repo" bundle verify "$bundle_path" >/dev/null 2>&1 || rc=1
    fi
    if [ "$rc" -eq 0 ]; then
        git_bounded -C "$tmp/repo" fetch -q --no-tags "$bundle_path" 'refs/*:refs/remotes/bundle/*' >/dev/null 2>&1 || rc=1
    fi
    if [ "$rc" -eq 0 ]; then
        git_bounded -C "$tmp/repo" cat-file -e "$commit^{commit}" >/dev/null 2>&1 || rc=1
    fi
    rm -rf "$tmp"
    if [ "$rc" -ne 0 ]; then
        fail "bundle does not prove pinned commit"
        return 1
    fi
    return 0
}

check_source_clean_except_generated() {
    local path dirty=0
    while IFS= read -r path; do
        [ -z "$path" ] && continue
        if ! is_generated_source_path "$path"; then
            fail "source checkout has unexpected dirty or untracked path ([redacted path])"
            dirty=1
        fi
    done < <(
        git_bounded -C "$SRC" diff --name-only HEAD -- 2>/dev/null
        git_bounded -C "$SRC" diff --cached --name-only HEAD -- 2>/dev/null
        git_bounded -C "$SRC" status --porcelain=v1 --untracked-files=all 2>/dev/null |
            while IFS= read -r path; do
                path=${path:3}
                path=${path#\"}
                path=${path%\"}
                printf '%s\n' "$path"
            done
    )
    [ "$dirty" -eq 0 ]
}

check_git_dependency_tree() {
    local label=$1 path=$2 expected=$3 head status
    validate_path_location "$label" "$path" || return 1
    [ -d "$path" ] || { fail "$label source is missing"; return 1; }
    validate_regular_tree "$label" "$path" || return 1
    [ -e "$path/.git" ] || {
        fail "$label is not an initialized Git checkout"
        return 1
    }
    git_bounded -C "$path" rev-parse --git-dir >/dev/null 2>&1 || {
        fail "$label is not an initialized Git checkout"
        return 1
    }
    [ "$(git_bounded -C "$path" rev-parse --is-shallow-repository 2>/dev/null)" = false ] ||
        fail "$label checkout is shallow or shallow state is unreadable"
    head=$(git_bounded -C "$path" rev-parse --verify HEAD 2>/dev/null) || head=
    [ "$head" = "$expected" ] || fail "$label commit mismatch"
    status=$(git_bounded -C "$path" status --porcelain=v1 --untracked-files=all --ignored=matching 2>/dev/null) || {
        fail "$label checkout status could not be inspected"
        return 1
    }
    [ -z "$status" ] || fail "$label checkout has unexpected untracked or modified files"
    if [ -f "$path/.gitmodules" ]; then
        git_bounded -C "$path" submodule status --recursive 2>/dev/null |
            awk 'substr($0, 1, 1) ~ /[-+U]/ { bad=1 } END { exit bad }' ||
            fail "$label has an uninitialized, modified, or mismatched recursive submodule"
        git_bounded -C "$path" submodule foreach --quiet --recursive \
            'test -z "$(git status --porcelain=v1 --untracked-files=all --ignored=matching)"' >/dev/null 2>&1 ||
            fail "$label recursive submodule working tree is dirty"
    fi
}

check_vendor_config() {
    local config=$1
    validate_path_location "vendor-config" "$config" || return
    [ -f "$config" ] || { fail "vendor config is missing: vendor-config"; return; }
    [ -L "$config" ] && { fail "vendor config is a symlink: vendor-config"; return; }
    awk '
        function trim(value) {
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
            return value
        }
        function quoted(value) {
            return value ~ /^"[^"\n]*"$/
        }
        function unquote(value) {
            return substr(value, 2, length(value) - 2)
        }
        function mark_source(section) {
            if (section == "net") {
                source_kind = "net"
                net_sections++
            } else if (section == "source.crates-io") {
                source_kind = "crates"
                crates_sections++
            } else if (section == "source.vendored-sources") {
                source_kind = "vendor"
                vendor_sections++
            } else if (section ~ /^source\.\"git\+https:\/\/[^"]+\"$/) {
                source_kind = "git"
                git_sections++
            } else {
                bad = 1
                source_kind = "unknown"
            }
        }
        {
            line = $0
            sub(/\r$/, "", line)
            sub(/[[:space:]]+#.*$/, "", line)
            line = trim(line)
            if (line == "") next
            if (line ~ /^\[[^]]+\]$/) {
                section = substr(line, 2, length(line) - 2)
                mark_source(section)
                next
            }
            if (line !~ /^[A-Za-z0-9_.-]+[[:space:]]*=/) {
                bad = 1
                next
            }
            if (source_kind == "unknown" || source_kind == "") {
                bad = 1
                next
            }
            equals = index(line, "=")
            key = trim(substr(line, 1, equals - 1))
            value = trim(substr(line, equals + 1))
            if (source_kind == "net") {
                # Cargo offline switch is a TOML boolean, not a string.
                if (value != "true") {
                    bad = 1
                    next
                }
            } else {
                if (!quoted(value)) {
                    bad = 1
                    next
                }
                value = unquote(value)
            }
            if (source_kind == "net") {
                if (key != "offline" || value != "true" || ++net_offline > 1)
                    bad = 1
            } else if (source_kind == "crates") {
                if (key != "replace-with" || value != "vendored-sources" || ++crates_replace > 1)
                    bad = 1
            } else if (source_kind == "vendor") {
                if (key != "directory" || value != "vendor" || ++vendor_directory > 1)
                    bad = 1
            } else if (source_kind == "git") {
                if (key == "replace-with") {
                    if (value != "vendored-sources" || ++git_replace > git_sections)
                        bad = 1
                } else if (key == "git") {
                    if (value !~ /^https:\/\/[^[:space:]"]+$/ || ++git_url > git_sections)
                        bad = 1
                } else if (key == "branch" || key == "tag") {
                    if (++git_selector > git_sections || value == "") bad = 1
                } else if (key == "rev") {
                    if (value !~ /^[0-9a-fA-F]{7,64}$/ || ++git_rev > git_sections) bad = 1
                } else {
                    bad = 1
                }
            }
        }
        END {
            if (bad || net_sections != 1 || net_offline != 1 ||
                crates_sections != 1 || vendor_sections != 1 ||
                crates_replace != 1 || vendor_directory != 1 ||
                git_replace != git_sections || git_url != git_sections ||
                git_selector > git_sections || git_rev > git_sections) exit 1
        }
    ' "$config" >/dev/null 2>&1 || {
        fail "vendor config contains unknown or network-capable settings; only local vendored-sources replacement is allowed"
        return
    }
}

archive_list() {
    local archive=$1 output=$2
    case "$archive" in
        *.tar.gz) tar --null -tzf "$archive" > "$output" 2>/dev/null ;;
        *.tar.zst)
            have zstd || return 1
            zstd -dc "$archive" 2>/dev/null | tar --null -tf - > "$output" 2>/dev/null
            ;;
        *) return 1 ;;
    esac
}

check_vendor_tarball() {
    local archive=$1 listing records entry staged_hash archive_hash rc=0
    validate_path_location "vendor-tarball" "$archive" || return
    [ -f "$archive" ] || { fail "vendor tarball missing"; return; }
    listing=$(mktemp "${TMPDIR:-/tmp}/offline-kit-vendor-list.XXXXXX") || {
        fail "cannot create temporary vendor archive listing"
        return
    }
    if ! archive_list "$archive" "$listing"; then
        rm -f "$listing"
        fail "vendor tarball cannot be listed safely"
        return
    fi
    if LC_ALL=C sort -z "$listing" | LC_ALL=C uniq -z -d | grep -q .; then
        rm -f "$listing"
        fail "vendor tarball contains duplicate entries"
        return
    fi
    while IFS= read -r -d '' entry; do
        case "$entry" in
            *$'\n'*|/*|../*|*/../*|*/..|..|vendor/../*|vendor/..)
                rc=1
                ;;
            vendor|vendor/*) ;;
            *) rc=1 ;;
        esac
        is_secret_path "$entry" && rc=1
    done < "$listing"
    if [ "$rc" -ne 0 ]; then
        rm -f "$listing"
        fail "vendor tarball contains an unsafe, secret-bearing, or unexpected path"
        return
    fi
    if ! have sha256sum; then
        rm -f "$listing"
        fail "sha256sum is required for deterministic vendor archive comparison"
        return
    fi
    records=$(mktemp "${TMPDIR:-/tmp}/offline-kit-vendor-records.XXXXXX") || {
        rm -f "$listing"
        fail "cannot create temporary vendor archive records"
        return
    }
    case "$archive" in
        *.tar.gz)
            tar -xzf "$archive" --to-command='
                case "$TAR_FILETYPE" in
                    f)
                        digest=$(sha256sum | awk "{print \$1}") || exit 1
                        printf "F\\t%s\\t%s\\n" "${TAR_FILENAME#vendor/}" "$digest"
                        ;;
                    d) ;;
                    *) exit 1 ;;
                esac
            ' > "$records" 2>/dev/null || rc=1
            ;;
        *.tar.zst)
            zstd -dc "$archive" 2>/dev/null | tar -x --to-command='
                case "$TAR_FILETYPE" in
                    f)
                        digest=$(sha256sum | awk "{print \$1}") || exit 1
                        printf "F\\t%s\\t%s\\n" "${TAR_FILENAME#vendor/}" "$digest"
                        ;;
                    d) ;;
                    *) exit 1 ;;
                esac
            ' - > "$records" 2>/dev/null || rc=1
            ;;
    esac
    if [ "$rc" -eq 0 ]; then
        staged_hash=$(content_hash_dir "$SRC/vendor") || rc=1
        LC_ALL=C sort "$records" -o "$records"
        archive_hash=$(sha256_file "$records") || rc=1
        [ "$staged_hash" = "$archive_hash" ] || rc=1
    fi
    rm -f "$records"
    rm -f "$listing"
    [ "$rc" -eq 0 ] || fail "vendor tarball contents mismatch with staged vendor tree"
}

check_source_and_vendor() {
    local head ref_commit bundle submodule_status workflow workflow_pin vendor_path
    have git || { fail "git is required for source verification"; return; }
    have timeout || { fail "timeout is required for bounded git checks"; return; }
    validate_path_location "rustdesk-src" "$SRC" || return
    [ -d "$SRC" ] || { fail "local-only source missing: $SRC"; return; }
    validate_regular_tree "rustdesk-src" "$SRC" || return
    validate_optional_directory "rustdesk-src/.cargo" "$SRC/.cargo" || return
    validate_optional_directory "rustdesk-src/vendor" "$SRC/vendor" || return
    validate_file_target "vendor-config" "$SRC/.cargo/config.vendor.toml" || return
    git_bounded -C "$SRC" rev-parse --git-dir >/dev/null 2>&1 || { fail "source is not a Git checkout: $SRC"; return; }
    [ "$(git_bounded -C "$SRC" rev-parse --is-shallow-repository 2>/dev/null)" = false ] ||
        fail "source checkout is shallow or shallow state is unreadable"
    check_source_clean_except_generated
    head=$(git_bounded -C "$SRC" rev-parse --verify HEAD 2>/dev/null) || head=""
    [ "$head" = "$RUSTDESK_COMMIT" ] || fail "source HEAD mismatch: expected $RUSTDESK_COMMIT"
    git_bounded -C "$SRC" check-ref-format --allow-onelevel "refs/tags/$RUSTDESK_REF" >/dev/null 2>&1 ||
        fail "RUSTDESK_REF is not a valid immutable tag name"
    ref_commit=$(git_bounded -C "$SRC" rev-parse --verify --quiet "refs/tags/$RUSTDESK_REF^{commit}" 2>/dev/null) || ref_commit=""
    [ "$ref_commit" = "$RUSTDESK_COMMIT" ] || fail "source tag/commit mismatch: $RUSTDESK_REF"
    [ -d "$SRC/vendor" ] || fail "vendor/source inconsistency: source vendor directory missing"
    [ -f "$SRC/.cargo/config.vendor.toml" ] || fail "vendor/source inconsistency: vendor config missing"
    [ -f "$SRC/.cargo/config.vendor.toml" ] && check_vendor_config "$SRC/.cargo/config.vendor.toml"
    workflow="$SRC/.github/workflows/third-party-RustDeskTempTopMostWindow.yml"
    [ -f "$workflow" ] || fail "active RustDesk TopMostWindow workflow is missing from frozen source"
    if [ -f "$workflow" ]; then
        workflow_pin=$(awk '
            { for (i = 1; i <= NF - 2; i++)
                if ($i == "git" && $(i + 1) == "checkout" && $(i + 2) ~ /^[0-9a-fA-F]{40}$/) {
                    print $(i + 2); count++
                }
            }
            END { if (count != 1) exit 1 }
        ' "$workflow" 2>/dev/null) || workflow_pin=""
        [ "$workflow_pin" = "$TOPMOST_COMMIT" ] || fail "TopMostWindow pin mismatches active RustDesk workflow"
    fi
    if [ -f "$SRC/.gitmodules" ]; then
        submodule_status=$(git_bounded -C "$SRC" submodule status --recursive 2>/dev/null) || submodule_status=""
        printf '%s\n' "$submodule_status" | awk 'substr($0, 1, 1) ~ /[-+U]/ { bad=1 } END { exit bad }' ||
            fail "recursive source submodule is uninitialized, modified, or mismatched"
        git_bounded -C "$SRC" submodule foreach --quiet --recursive \
            'test -z "$(git status --porcelain=v1 --untracked-files=all)"' >/dev/null 2>&1 ||
            fail "recursive source submodule working tree is dirty"
    fi
    bundle="$OUT/rustdesk-$RUSTDESK_REF.bundle"
    [ -f "$bundle" ] || fail "source bundle missing: $bundle"
    [ -f "$bundle" ] && verify_bundle_commit "$bundle" "$RUSTDESK_COMMIT"
    bundle="$OUT/RustDeskTempTopMostWindow.bundle"
    [ -f "$bundle" ] && verify_bundle_commit "$bundle" "$TOPMOST_COMMIT"
    check_git_dependency_tree "vcpkg source" "$OUT/vcpkg" "$VCPKG_BASELINE"
    check_git_dependency_tree "TopMostWindow source" \
        "$OUT/RustDeskTempTopMostWindow" "$TOPMOST_COMMIT"
    if [ -f "$OUT/vendor-$RUSTDESK_REF.tar.zst" ]; then
        vendor_path="$OUT/vendor-$RUSTDESK_REF.tar.zst"
    elif [ -f "$OUT/vendor-$RUSTDESK_REF.tar.gz" ]; then
        vendor_path="$OUT/vendor-$RUSTDESK_REF.tar.gz"
    else
        fail "vendor tarball missing"
        vendor_path=""
    fi
    [ -n "$vendor_path" ] && check_vendor_tarball "$vendor_path"
}

main() {
    local arg
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --help|-h) usage; return 0 ;;
            --manifest) [ "$#" -ge 2 ] || { usage >&2; return 2; }; MANIFEST=$2; shift ;;
            *) usage >&2; return 2 ;;
        esac
        shift
    done
    check_secret_presence
    check_manifest_compatibility
    if [ "$MANIFEST_COMPATIBLE" -eq 0 ]; then
        printf '%s\n' "[verify] FAIL: $ERRORS check(s) failed; no network or manifest mutation was performed" >&2
        return 1
    fi
    check_pins
    check_manifest
    check_source_and_vendor
    if [ "$ERRORS" -ne 0 ]; then
        printf '%s\n' "[verify] FAIL: $ERRORS check(s) failed; no network or manifest mutation was performed" >&2
        return 1
    fi
    ok "offline-kit manifest, pins, source commit, bundle, and vendor consistency verified"
}

main "$@"
