#!/usr/bin/env bash
# offline-kit/freeze.sh — resumable acquisition stage for the frozen kit.
#
# This script may download and update the ignored local artifact directory.
# Verification is deliberately separate: use `bash freeze.sh --verify` for a
# read-only, fail-closed check.

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

RUSTDESK_REPO=${RUSTDESK_REPO:?}
RUSTDESK_REF=${RUSTDESK_REF:?}
ARTIFACTS_DIR=${ARTIFACTS_DIR:?}

OUT=$ARTIFACTS_DIR
SRC="$OUT/rustdesk-src"
MANIFEST="$OUT/MANIFEST.txt"
KIT_ROOT="$SCRIPT_DIR"
ALLOW_TOFU=0
MANIFEST_SCHEMA_VERSION=2

log()  { printf '%s\n' "[freeze] $*"; }
warn() { printf '%s\n' "[freeze][WARN] $*" >&2; }
die()  { printf '%s\n' "[freeze][FAIL] $*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

display_value() {
    local value=$1 rest host
    case "$value" in
        *://*@*)
            rest=${value#*://}
            host=${rest#*@}
            printf '%s\n' "[redacted]@$host"
            ;;
        *workflow-payload*|*.key|*.pem|*.p12|*.pfx)
            printf '%s\n' '[redacted]'
            ;;
        *) printf '%s\n' "$value" ;;
    esac
}

usage() {
    cat <<'EOF'
Usage: bash freeze.sh [--allow-tofu] [--verify | stage ...]

Stages: source vendor engine flutter_sdk vcpkg rust thirdparty

--verify       run the read-only verifier; no network and no MANIFEST rewrite
--allow-tofu   explicit manual exception for an artifact with no expected digest
EOF
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
    if have realpath; then
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
        *$'\n'*) warn "$label path contains a newline; refusing to stage it"; return 1 ;;
    esac
    canonical=$(canonical_path "$path") || return 1
    case "$canonical" in
        "$KIT_ROOT"|"$KIT_ROOT"/*) ;;
        *) warn "$label path escapes kit root: [redacted path]"; return 1 ;;
    esac
    if ! validate_existing_path_components "$path"; then
        warn "$label path contains a symlink; refusing to stage it: [redacted path]"
        return 1
    fi
    return 0
}

validate_regular_tree() {
    local label=$1 path=$2 bad
    bad=$(LC_ALL=C find -P "$path" \( -type l -o \( ! -type f ! -type d \) \) -print -quit 2>/dev/null) || return 1
    [ -z "$bad" ] || {
        warn "$label contains a symlink or non-regular entry; refusing to stage it"
        return 1
    }
    return 0
}

validate_staged_path() {
    local label=$1 path=$2
    validate_path_location "$label" "$path" || return 1
    if [ -L "$path" ]; then
        warn "$label is a symlink; refusing to stage it"
        return 1
    elif [ -f "$path" ]; then
        return 0
    elif [ -d "$path" ]; then
        validate_regular_tree "$label" "$path"
    else
        warn "$label is not a regular file or directory: [redacted path]"
        return 1
    fi
}

validate_file_target() {
    local label=$1 path=$2
    validate_path_location "$label" "$path" || return 1
    if [ -L "$path" ]; then
        warn "$label is a symlink; refusing to stage it"
        return 1
    fi
    if [ -e "$path" ] && [ ! -f "$path" ]; then
        warn "$label is not a regular file"
        return 1
    fi
    return 0
}

validate_optional_directory() {
    local label=$1 path=$2
    validate_path_location "$label" "$path" || return 1
    if [ -e "$path" ]; then
        [ -d "$path" ] || {
            warn "$label is not a regular directory: [redacted path]"
            return 1
        }
        validate_regular_tree "$label" "$path"
    fi
}

check_vendor_config() {
    local config=$1
    validate_path_location "vendor-config" "$config" || return 1
    [ -f "$config" ] || {
        warn "vendor config is missing"
        return 1
    }
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
        warn "vendor config contains unknown or network-capable settings; only local vendored-sources replacement is allowed"
        return 1
    }
}

validate_source_entry_paths() {
    validate_optional_directory "rustdesk-src" "$SRC" || return 1
    if [ -d "$SRC" ]; then
        validate_optional_directory "rustdesk-src/.cargo" "$SRC/.cargo" || return 1
        validate_optional_directory "rustdesk-src/vendor" "$SRC/vendor" || return 1
        validate_file_target "vendor-config" "$SRC/.cargo/config.vendor.toml" || return 1
        if [ -f "$SRC/.cargo/config.vendor.toml" ]; then
            check_vendor_config "$SRC/.cargo/config.vendor.toml" || return 1
        fi
    fi
}

validate_stage_entries() {
    local stage=$1
    case "$stage" in
        source)
            validate_source_entry_paths || return 1
            validate_file_target "rustdesk-bundle" "$OUT/rustdesk-$RUSTDESK_REF.bundle" || return 1
            ;;
        vendor)
            validate_source_entry_paths || return 1
            [ -d "$SRC" ] || { warn "local-only source missing: $SRC"; return 1; }
            validate_file_target "vendor-tarball" "$OUT/vendor-$RUSTDESK_REF.tar.gz" || return 1
            validate_file_target "vendor-tarball" "$OUT/vendor-$RUSTDESK_REF.tar.zst" || return 1
            ;;
        engine)
            validate_file_target "flutter-engine-win" "$OUT/windows-x64-release.zip" || return 1
            ;;
        flutter_sdk)
            validate_file_target "flutter-sdk-win" "$OUT/flutter_windows_$FLUTTER_VERSION-stable.zip" || return 1
            validate_file_target "flutter-sdk-linux" "$OUT/flutter_linux_$FLUTTER_VERSION-stable.tar.xz" || return 1
            ;;
        vcpkg)
            validate_optional_directory "vcpkg source" "$OUT/vcpkg" || return 1
            ;;
        rust)
            validate_file_target "rust-win-msvc" "$OUT/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi" || return 1
            ;;
        thirdparty)
            validate_optional_directory "TopMostWindow source" "$OUT/RustDeskTempTopMostWindow" || return 1
            validate_file_target "topmost-bundle" "$OUT/RustDeskTempTopMostWindow.bundle" || return 1
            validate_file_target "usbmmidd" "$OUT/usbmmidd_v2.zip" || return 1
            validate_file_target "printer-driver" "$OUT/rustdesk_printer_driver_v4-1.4.zip" || return 1
            validate_file_target "printer-adapter" "$OUT/printer_driver_adapter.zip" || return 1
            validate_file_target "printer-sums" "$OUT/printer_sha256sums" || return 1
            ;;
        *)
            warn "unknown stage: $stage"
            return 1
            ;;
    esac
}

file_size() {
    if stat -c '%s' "$1" 2>/dev/null; then return 0; fi
    wc -c < "$1" | tr -d '[:space:]'
}

dir_size() {
    if du -sb "$1" 2>/dev/null | awk '{print $1}'; then return 0; fi
    du -sk "$1" | awk '{print $1 * 1024}'
}

sha256_file() {
    if have sha256sum; then
        sha256sum "$1" | awk '{print $1}'
    elif have shasum; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        die "sha256sum or shasum is required"
    fi
}

content_hash_dir() {
    local dir=$1 listing file rel digest target rc
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
    if tree=$(git -C "$path" rev-parse --verify --quiet HEAD^{tree} 2>/dev/null); then
        printf 'git-tree:%s\n' "$tree"
    else
        tree=$(content_hash_dir "$path") || return 1
        printf 'content-sha256:%s\n' "$tree"
    fi
}

check_source_clean_except_generated() {
    local path dirty=0
    while IFS= read -r path; do
        [ -z "$path" ] && continue
        if ! is_generated_source_path "$path"; then
            warn "source checkout has unexpected dirty or untracked path: [redacted path]"
            dirty=1
        fi
    done < <(
        git -C "$SRC" diff --name-only HEAD -- 2>/dev/null
        git -C "$SRC" diff --cached --name-only HEAD -- 2>/dev/null
        git -C "$SRC" status --porcelain=v1 --untracked-files=all 2>/dev/null |
            while IFS= read -r path; do
                path=${path:3}
                path=${path#\"}
                path=${path%\"}
                printf '%s\n' "$path"
            done
    )
    if [ -f "$SRC/.gitmodules" ]; then
        git -C "$SRC" submodule status --recursive 2>/dev/null |
            awk 'substr($0, 1, 1) ~ /[-+U]/ { bad=1 } END { exit bad }' || {
                warn "recursive source submodule is uninitialized, modified, or mismatched"
                dirty=1
            }
        git -C "$SRC" submodule foreach --quiet --recursive \
            'test -z "$(git status --porcelain=v1 --untracked-files=all)"' >/dev/null 2>&1 || {
                warn "recursive source submodule working tree is dirty"
                dirty=1
            }
    fi
    [ "$dirty" -eq 0 ]
}

check_git_dependency_tree() {
    local label=$1 path=$2 expected=$3 head status
    validate_optional_directory "$label" "$path" || return 1
    [ -e "$path" ] || return 0
    [ -e "$path/.git" ] || {
        warn "$label is not an initialized Git checkout"
        return 1
    }
    git -C "$path" rev-parse --git-dir >/dev/null 2>&1 || {
        warn "$label is not an initialized Git checkout"
        return 1
    }
    [ "$(git -C "$path" rev-parse --is-shallow-repository 2>/dev/null)" = false ] || {
        warn "$label checkout is shallow or shallow state is unreadable"
        return 1
    }
    head=$(git -C "$path" rev-parse --verify HEAD 2>/dev/null) || head=
    [ "$head" = "$expected" ] || {
        warn "$label commit mismatch; expected pinned commit"
        return 1
    }
    status=$(git -C "$path" status --porcelain=v1 --untracked-files=all --ignored=matching 2>/dev/null) || {
        warn "$label checkout status could not be inspected"
        return 1
    }
    [ -z "$status" ] || {
        warn "$label checkout has unexpected untracked or modified files"
        return 1
    }
    if [ -f "$path/.gitmodules" ]; then
        git -C "$path" submodule status --recursive 2>/dev/null |
            awk 'substr($0, 1, 1) ~ /[-+U]/ { bad=1 } END { exit bad }' || {
                warn "$label has an uninitialized, modified, or mismatched recursive submodule"
                return 1
            }
        git -C "$path" submodule foreach --quiet --recursive \
            'test -z "$(git status --porcelain=v1 --untracked-files=all --ignored=matching)"' >/dev/null 2>&1 || {
                warn "$label recursive submodule working tree is dirty"
                return 1
            }
    fi
    return 0
}

preflight_source_dependencies() {
    local stage=$1
    case "$stage" in
        vcpkg)
            check_git_dependency_tree "vcpkg source" "$OUT/vcpkg" "$VCPKG_BASELINE" || return 1
            ;;
        thirdparty)
            check_git_dependency_tree "TopMostWindow source" \
                "$OUT/RustDeskTempTopMostWindow" "$TOPMOST_COMMIT" || return 1
            ;;
    esac
}

check_manifest_compatibility() {
    local schema_count key pin_count pin_value
    [ -f "$MANIFEST" ] || return 0
    schema_count=$(awk -v version="$MANIFEST_SCHEMA_VERSION" \
        '$0 == "# schema=" version { count++ } END { print count + 0 }' "$MANIFEST")
    if [ "$schema_count" -ne 1 ]; then
        die "incompatible MANIFEST.txt (legacy or unsupported schema); migrate/re-freeze into a new empty artifact directory, then rerun"
    fi
    for key in RUSTDESK_REF RUSTDESK_COMMIT RUST_VERSION LLVM_VERSION FLUTTER_VERSION \
        FLUTTER_RUST_BRIDGE_VERSION NDK_VERSION CARGO_NDK_VERSION VCPKG_BASELINE VCPKG_TRIPLET; do
        pin_count=$(awk -v key="$key" '$0 ~ "^# pin " key "=" { count++ } END { print count + 0 }' "$MANIFEST")
        pin_value=$(awk -v key="$key" '$0 ~ "^# pin " key "=" { sub("^# pin " key "=", ""); print; exit }' "$MANIFEST")
        if [ "$pin_count" -ne 1 ] || [ -z "$pin_value" ]; then
            die "incompatible MANIFEST.txt (missing or duplicate required pin record $key); migrate/re-freeze into a new empty artifact directory, then rerun"
        fi
    done
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

require_digest_policy() {
    local label=$1 path=$2 expected
    expected=$(expected_digest_for "$path")
    if [ -z "$expected" ]; then
        if [ "$ALLOW_TOFU" -eq 1 ]; then
            warn "TOFU/manual exception for $label; digest is not evidence: $path"
            return 0
        fi
        warn "missing expected digest for $label; rerun with --allow-tofu only after manual review: $path"
        return 1
    fi
    [[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]] || {
        warn "invalid expected SHA-256 for $label"
        return 1
    }
}

verify_existing_file() {
    local label=$1 path=$2 expected actual
    validate_file_target "$label" "$path" || return 1
    [ -s "$path" ] || return 1
    require_digest_policy "$label" "$path" || return 2
    expected=$(expected_digest_for "$path")
    [ -n "$expected" ] || return 0
    actual=$(sha256_file "$path")
    if [ "$actual" != "$expected" ]; then
        warn "digest mismatch for $label (preserving existing file): $path"
        return 1
    fi
    return 0
}

record() {
    local label=$1 path=$2 line tmp checksum
    is_secret_path "$path" && die "refusing to record a secret-bearing path"
    validate_staged_path "$label" "$path" || return 1
    if [ -f "$path" ]; then
        line="$label  $(file_size "$path")  sha256:$(sha256_file "$path")  $path"
    elif [ -d "$path" ]; then
        checksum=$(directory_record "$path") || return 1
        line="$label  $(dir_size "$path")  $checksum  $path"
    else
        return 1
    fi
    tmp="$MANIFEST.tmp.$$"
    validate_file_target "manifest temporary" "$tmp" || return 1
    if [ -f "$MANIFEST" ]; then
        awk -v label="$label" '$1 != label' "$MANIFEST" > "$tmp" || return 1
        mv -f "$tmp" "$MANIFEST"
    fi
    printf '%s\n' "$line" >> "$MANIFEST"
}

dl() {
    local label=$1 url=$2 out=$3 expected actual
    is_secret_path "$out" && die "refusing to handle a secret-bearing path"
    validate_file_target "$label" "$out" || return 1
    if [ -s "$out" ]; then
        if verify_existing_file "$label" "$out"; then
            log "skip (verified): $out"
            return 0
        else
            local existing_rc=$?
            [ "$existing_rc" -eq 2 ] && return 1
        fi
        warn "resuming after digest mismatch; final digest must match: $out"
    else
        require_digest_policy "$label" "$out" || return 1
    fi
    log "download provider asset: $(display_value "$url")"
    if have curl; then
        curl -fSL --retry 3 -C - -o "$out" "$url" || {
            warn "provider asset unavailable: $label"
            return 1
        }
    elif have wget; then
        wget -c -O "$out" "$url" || {
            warn "provider asset unavailable: $label"
            return 1
        }
    else
        warn "provider asset unavailable: neither curl nor wget is available"
        return 1
    fi
    [ -s "$out" ] || { warn "provider returned an empty asset: $label"; return 1; }
    expected=$(expected_digest_for "$out")
    if [ -z "$expected" ]; then
        [ "$ALLOW_TOFU" -eq 1 ] || return 1
        warn "downloaded $label under explicit TOFU/manual exception"
        return 0
    fi
    actual=$(sha256_file "$out")
    [ "$actual" = "$expected" ] || {
        warn "downloaded asset digest mismatch for $label; preserving file"
        return 1
    }
}

verify_source_identity() {
    local head ref_commit
    [ -d "$SRC/.git" ] || { warn "local-only source missing: $SRC"; return 1; }
    have git || { warn "git not found"; return 1; }
    head=$(git -C "$SRC" rev-parse HEAD 2>/dev/null) || {
        warn "cannot read local source HEAD: $SRC"; return 1;
    }
    [ "$head" = "$RUSTDESK_COMMIT" ] || {
        warn "source commit mismatch: expected $RUSTDESK_COMMIT, got $head"; return 1;
    }
    git -C "$SRC" check-ref-format --allow-onelevel "refs/tags/$RUSTDESK_REF" >/dev/null 2>&1 || {
        warn "source ref is not a valid immutable tag name: $RUSTDESK_REF"; return 1;
    }
    ref_commit=$(git -C "$SRC" rev-parse --verify "refs/tags/$RUSTDESK_REF^{commit}" 2>/dev/null) || {
        warn "source ref/tag is unavailable locally: $RUSTDESK_REF"; return 1;
    }
    [ "$ref_commit" = "$RUSTDESK_COMMIT" ] || {
        warn "source ref/tag mismatch: $RUSTDESK_REF is $ref_commit"; return 1;
    }
}

verify_bundle_commit() {
    local bundle=$1 heads bundle_path
    bundle_path=$(canonical_path "$bundle") || return 1
    git bundle verify "$bundle_path" >/dev/null 2>&1 || {
        warn "source bundle is invalid: $bundle"; return 1;
    }
    heads=$(git bundle list-heads "$bundle_path" 2>/dev/null || true)
    printf '%s\n' "$heads" | awk '{print $1}' | grep -F -x "$RUSTDESK_COMMIT" >/dev/null 2>&1 || {
        warn "source bundle does not contain RUSTDESK_COMMIT"; return 1;
    }
}

stage_source() {
    local bundle="$OUT/rustdesk-$RUSTDESK_REF.bundle" head
    log "=== STAGE: source (pin $RUSTDESK_REF @ $RUSTDESK_COMMIT) ==="
    have git || die "git not found"
    validate_stage_entries source || return 1
    if [ ! -d "$SRC/.git" ]; then
        git clone --branch "$RUSTDESK_REF" --recurse-submodules "$RUSTDESK_REPO" "$SRC" || {
            warn "local-only source acquisition failed"; return 1;
        }
    fi
    verify_source_identity || return 1
    check_source_clean_except_generated || return 1
    validate_file_target "rustdesk-bundle" "$bundle" || return 1
    head=$(git -C "$SRC" rev-parse HEAD) || return 1
    [ "$head" = "$RUSTDESK_COMMIT" ] || return 1
    if [ -s "$bundle" ]; then
        verify_existing_file "rustdesk-bundle" "$bundle" || {
            warn "source bundle mismatch; preserving existing bundle: $bundle"; return 1;
        }
        verify_bundle_commit "$bundle" || return 1
    else
        require_digest_policy "rustdesk-bundle" "$bundle" || return 1
        ( cd "$SRC" && git bundle create "../rustdesk-$RUSTDESK_REF.bundle" --all ) || {
            warn "source bundle creation failed"; return 1;
        }
        verify_existing_file "rustdesk-bundle" "$bundle" || return 1
        verify_bundle_commit "$bundle" || return 1
    fi
    record "rustdesk-source" "$SRC" || return 1
    record "rustdesk-bundle" "$bundle" || return 1
}

stage_vendor() {
    local tarball vendor_stage
    log "=== STAGE: vendor (cargo vendor) ==="
    validate_stage_entries vendor || return 1
    [ -d "$SRC" ] || { warn "local-only source missing: $SRC"; return 1; }
    verify_source_identity || return 1
    check_source_clean_except_generated || return 1
    have cargo || { warn "local-only vendor generation unavailable: cargo not found"; return 1; }
    if [ ! -d "$SRC/vendor" ] || [ ! -f "$SRC/.cargo/config.vendor.toml" ]; then
        vendor_stage=$(mktemp -d "$OUT/.offline-kit-vendor-stage.XXXXXX") || {
            warn "cannot create safe vendor staging directory"; return 1;
        }
        vendor_stage=$(canonical_path "$vendor_stage") || {
            rm -rf "$vendor_stage"; return 1;
        }
        validate_path_location "vendor staging" "$vendor_stage" || {
            rm -rf "$vendor_stage"; return 1;
        }
        validate_path_location "staged vendor" "$vendor_stage/vendor" || {
            rm -rf "$vendor_stage"; return 1;
        }
        validate_file_target "staged vendor-config" "$vendor_stage/config.vendor.toml" || {
            rm -rf "$vendor_stage"; return 1;
        }
        if ! ( cd "$SRC" && cargo vendor "$vendor_stage/vendor" > "$vendor_stage/config.vendor.toml" 2>/dev/null ); then
            rm -rf "$vendor_stage"
            warn "vendor generation failed"
            return 1
        fi
        validate_file_target "staged offline vendor-config" "$vendor_stage/config.vendor.toml.offline" || {
            rm -rf "$vendor_stage"; return 1
        }
        {
            printf '%s\n' '[net]' 'offline = true' ''
            awk '
                BEGIN { in_vendor = 0 }
                {
                    line = $0
                    if (line == "[source.vendored-sources]") in_vendor = 1
                    else if (line ~ /^\[/) in_vendor = 0
                    if (in_vendor && line ~ /^[[:space:]]*directory[[:space:]]*=/) {
                        print "directory = \"vendor\""
                    } else {
                        print line
                    }
                }
            ' "$vendor_stage/config.vendor.toml"
        } > "$vendor_stage/config.vendor.toml.offline" || {
            rm -rf "$vendor_stage"; return 1
        }
        mv -f "$vendor_stage/config.vendor.toml.offline" "$vendor_stage/config.vendor.toml" || {
            rm -rf "$vendor_stage"; return 1
        }
        validate_staged_path "staged vendor" "$vendor_stage/vendor" || {
            rm -rf "$vendor_stage"; return 1;
        }
        validate_file_target "staged vendor-config" "$vendor_stage/config.vendor.toml" || {
            rm -rf "$vendor_stage"; return 1
        }
        check_vendor_config "$vendor_stage/config.vendor.toml" || {
            rm -rf "$vendor_stage"; return 1
        }
        if [ ! -d "$SRC/vendor" ]; then
            validate_path_location "rustdesk-src/vendor" "$SRC/vendor" || {
                rm -rf "$vendor_stage"; return 1
            }
            cp -a -- "$vendor_stage/vendor" "$SRC/vendor" || {
                rm -rf "$vendor_stage"; return 1
            }
        fi
        if [ ! -f "$SRC/.cargo/config.vendor.toml" ]; then
            validate_path_location "rustdesk-src/.cargo" "$SRC/.cargo" || {
                rm -rf "$vendor_stage"; return 1
            }
            if [ ! -e "$SRC/.cargo" ]; then
                mkdir "$SRC/.cargo" || {
                    rm -rf "$vendor_stage"; return 1
                }
            fi
            validate_file_target "rustdesk-src/.cargo/config.vendor.toml" "$SRC/.cargo/config.vendor.toml" || {
                rm -rf "$vendor_stage"; return 1
            }
            cp -- "$vendor_stage/config.vendor.toml" "$SRC/.cargo/config.vendor.toml" || {
                rm -rf "$vendor_stage"; return 1
            }
        fi
        rm -rf "$vendor_stage"
    else
        log "vendor already present"
    fi
    validate_stage_entries vendor || return 1
    check_source_clean_except_generated || return 1
    tarball="$OUT/vendor-$RUSTDESK_REF.tar.gz"
    if [ ! -s "$tarball" ] && [ -s "$OUT/vendor-$RUSTDESK_REF.tar.zst" ]; then
        tarball="$OUT/vendor-$RUSTDESK_REF.tar.zst"
    fi
    if [ ! -s "$tarball" ]; then
        validate_file_target "vendor-tarball" "$tarball" || return 1
        require_digest_policy "vendor-tarball" "$tarball" || return 1
        # Keep the pinned artifact format deterministic; an existing zstd
        # artifact remains resumable/accepted when its digest is pinned.
        tar -C "$SRC" -czf "$tarball" vendor || return 1
        verify_existing_file "vendor-tarball" "$tarball" || return 1
    else
        validate_file_target "vendor-tarball" "$tarball" || return 1
        verify_existing_file "vendor-tarball" "$tarball" || return 1
    fi
    record "rustdesk-source" "$SRC" || return 1
    record "vendor-tarball" "$tarball" || return 1
    record "vendor-source" "$SRC/vendor" || return 1
    record "vendor-config" "$SRC/.cargo/config.vendor.toml" || return 1
}

stage_engine() {
    log "=== STAGE: flutter engine (Windows x64) ==="
    dl "flutter-engine-win" "$FLUTTER_ENGINE_WIN_URL" "$OUT/windows-x64-release.zip" || return 1
    record "flutter-engine-win" "$OUT/windows-x64-release.zip" || return 1
}

stage_flutter_sdk() {
    log "=== STAGE: flutter SDK ($FLUTTER_VERSION) ==="
    dl "flutter-sdk-win" "$FLUTTER_SDK_WIN_URL" "$OUT/flutter_windows_$FLUTTER_VERSION-stable.zip" || return 1
    dl "flutter-sdk-linux" "$FLUTTER_SDK_LINUX_URL" "$OUT/flutter_linux_$FLUTTER_VERSION-stable.tar.xz" || return 1
    record "flutter-sdk-win" "$OUT/flutter_windows_$FLUTTER_VERSION-stable.zip" || return 1
    record "flutter-sdk-linux" "$OUT/flutter_linux_$FLUTTER_VERSION-stable.tar.xz" || return 1
}

stage_vcpkg() {
    local vdir="$OUT/vcpkg"
    log "=== STAGE: vcpkg (baseline $VCPKG_BASELINE) ==="
    have git || return 1
    validate_stage_entries vcpkg || return 1
    preflight_source_dependencies vcpkg || return 1
    if [ ! -e "$vdir/.git" ]; then
        git clone "$VCPKG_REPO" "$vdir" || { warn "provider source unavailable: vcpkg"; return 1; }
    fi
    ( cd "$vdir" && git fetch origin "$VCPKG_BASELINE" 2>/dev/null && git checkout --detach "$VCPKG_BASELINE" 2>/dev/null ) || {
        warn "vcpkg baseline unavailable locally: $VCPKG_BASELINE"; return 1;
    }
    check_git_dependency_tree "vcpkg source" "$vdir" "$VCPKG_BASELINE" || return 1
    record "vcpkg-src" "$vdir" || return 1
}

stage_rust() {
    log "=== STAGE: rust toolchain offline installer ($RUST_VERSION) ==="
    dl "rust-win-msvc" "https://static.rust-lang.org/dist/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi" \
        "$OUT/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi" || return 1
    record "rust-win-msvc" "$OUT/rust-$RUST_VERSION-x86_64-pc-windows-msvc.msi" || return 1
}

stage_thirdparty() {
    local tmw="$OUT/RustDeskTempTopMostWindow"
    log "=== STAGE: thirdparty (TopMostWindow, usbmmidd, printer assets) ==="
    validate_stage_entries thirdparty || return 1
    preflight_source_dependencies thirdparty || return 1
    if [ ! -e "$tmw/.git" ]; then
        git clone "$TOPMOST_REPO" "$tmw" || { warn "provider source unavailable: TopMostWindow"; return 1; }
    fi
    ( cd "$tmw" && git fetch origin "$TOPMOST_COMMIT" 2>/dev/null && git checkout --detach "$TOPMOST_COMMIT" 2>/dev/null ) || {
        warn "TopMostWindow commit unavailable locally: $TOPMOST_COMMIT"; return 1;
    }
    check_git_dependency_tree "TopMostWindow source" "$tmw" "$TOPMOST_COMMIT" || return 1
    validate_file_target "topmost-bundle" "$OUT/RustDeskTempTopMostWindow.bundle" || return 1
    if [ ! -s "$OUT/RustDeskTempTopMostWindow.bundle" ]; then
        require_digest_policy "topmost-bundle" "$OUT/RustDeskTempTopMostWindow.bundle" || return 1
        ( cd "$tmw" && git bundle create "../RustDeskTempTopMostWindow.bundle" --all ) || return 1
        verify_existing_file "topmost-bundle" "$OUT/RustDeskTempTopMostWindow.bundle" || return 1
    else
        verify_existing_file "topmost-bundle" "$OUT/RustDeskTempTopMostWindow.bundle" || return 1
    fi
    dl "usbmmidd" "$USBMMIDD_URL" "$OUT/usbmmidd_v2.zip" || return 1
    dl "printer-driver" "$PRINTER_DRIVER_URL" "$OUT/rustdesk_printer_driver_v4-1.4.zip" || return 1
    dl "printer-adapter" "$PRINTER_ADAPTER_URL" "$OUT/printer_driver_adapter.zip" || return 1
    dl "printer-sums" "$PRINTER_SUMS_URL" "$OUT/printer_sha256sums" || return 1
    record "topmost-bundle" "$OUT/RustDeskTempTopMostWindow.bundle" || return 1
    record "topmost-src" "$tmw" || return 1
    record "usbmmidd" "$OUT/usbmmidd_v2.zip" || return 1
    record "printer-driver" "$OUT/rustdesk_printer_driver_v4-1.4.zip" || return 1
    record "printer-adapter" "$OUT/printer_driver_adapter.zip" || return 1
    record "printer-sums" "$OUT/printer_sha256sums" || return 1
}

write_manifest_header() {
    [ -f "$MANIFEST" ] && return 0
    {
        printf '%s\n' "# offline-kit MANIFEST — frozen rustdesk $RUSTDESK_REF"
        printf '%s\n' "# schema=$MANIFEST_SCHEMA_VERSION"
        printf '%s\n' "# Source: $(display_value "$RUSTDESK_REPO")"
        printf '%s\n' "# label  exact-byte-size  checksum  path"
        printf '%s\n' "# pin RUSTDESK_REF=$RUSTDESK_REF"
        printf '%s\n' "# pin RUSTDESK_COMMIT=$RUSTDESK_COMMIT"
        printf '%s\n' "# pin RUST_VERSION=$RUST_VERSION"
        printf '%s\n' "# pin LLVM_VERSION=$LLVM_VERSION"
        printf '%s\n' "# pin FLUTTER_VERSION=$FLUTTER_VERSION"
        printf '%s\n' "# pin FLUTTER_RUST_BRIDGE_VERSION=$FLUTTER_RUST_BRIDGE_VERSION"
        printf '%s\n' "# pin NDK_VERSION=$NDK_VERSION"
        printf '%s\n' "# pin CARGO_NDK_VERSION=$CARGO_NDK_VERSION"
        printf '%s\n' "# pin VCPKG_BASELINE=$VCPKG_BASELINE"
        printf '%s\n' "# pin VCPKG_TRIPLET=$VCPKG_TRIPLET"
    } > "$MANIFEST"
}

main() {
    local stages=() s
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --help|-h) usage; return 0 ;;
            --verify) exec bash "$SCRIPT_DIR/verify.sh" ;;
            --allow-tofu) ALLOW_TOFU=1 ;;
            --) shift; stages+=("$@"); break ;;
            *) stages+=("$1") ;;
        esac
        shift
    done
    [ "${#stages[@]}" -gt 0 ] || stages=(source vendor engine flutter_sdk vcpkg rust thirdparty)
    validate_path_location "artifact directory" "$OUT" || die "artifact directory is outside the kit or contains a symlink"
    validate_file_target "manifest" "$MANIFEST" || die "manifest path is outside the kit or contains a symlink"
    check_manifest_compatibility
    for s in "${stages[@]}"; do
        validate_stage_entries "$s" || die "stage-entry validation failed; no stage mutation was performed"
        preflight_source_dependencies "$s" || die "source dependency preflight failed; no stage mutation was performed"
    done
    mkdir -p "$OUT" || die "cannot create artifact directory"
    write_manifest_header || die "cannot create MANIFEST"
    for s in "${stages[@]}"; do
        case "$s" in
            source) stage_source || die "stage failed; stopping immediately: source" ;;
            vendor) stage_vendor || die "stage failed; stopping immediately: vendor" ;;
            engine) stage_engine || die "stage failed; stopping immediately: engine" ;;
            flutter_sdk) stage_flutter_sdk || die "stage failed; stopping immediately: flutter_sdk" ;;
            vcpkg) stage_vcpkg || die "stage failed; stopping immediately: vcpkg" ;;
            rust) stage_rust || die "stage failed; stopping immediately: rust" ;;
            thirdparty) stage_thirdparty || die "stage failed; stopping immediately: thirdparty" ;;
            *) die "stage-entry validation failed; no stage mutation was performed" ;;
        esac
    done
    log "=== stages complete; verify with: bash freeze.sh --verify ==="
}

main "$@"
