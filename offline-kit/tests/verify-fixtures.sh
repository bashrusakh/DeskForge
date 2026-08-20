#!/usr/bin/env bash
# Temporary, local-only regression fixtures for offline-kit verification.

set -e -u -o pipefail

TEST_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
KIT_DIR=$(CDPATH= cd -- "$TEST_DIR/.." && pwd -P)
TMP_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/offline-kit-fixtures.XXXXXX")
trap 'rm -rf "$TMP_ROOT"' EXIT

die() { printf '%s\n' "[fixture][FAIL] $*" >&2; exit 1; }
pass() { printf '%s\n' "[fixture][OK] $*"; }

git_init() {
    local dir=$1
    git init -q "$dir" || die "git init failed"
    git -C "$dir" config user.name fixture
    git -C "$dir" config user.email fixture@example.invalid
}

hash_file() { sha256sum "$1" | awk '{print $1}'; }

content_hash_dir() {
    local dir=$1 listing file rel digest target
    listing=$(mktemp "${TMPDIR:-/tmp}/offline-kit-fixture-dir.XXXXXX") || return 1
    (
        cd "$dir" || exit 1
        LC_ALL=C find -P . \( -type f -o -type l \) -print | LC_ALL=C sort |
            while IFS= read -r file; do
                rel=${file#./}
                if [ -L "$file" ]; then
                    target=$(readlink "$file") || exit 1
                    printf 'L\t%s\t%s\n' "$rel" "$target"
                else
                    digest=$(hash_file "$file") || exit 1
                    printf 'F\t%s\t%s\n' "$rel" "$digest"
                fi
            done
    ) > "$listing" || { rm -f "$listing"; return 1; }
    hash_file "$listing"
    rm -f "$listing"
}

write_file_record() {
    local label=$1 path=$2
    printf '%s  %s  sha256:%s  %s\n' "$label" "$(stat -c '%s' "$path")" "$(hash_file "$path")" "${path#"$KIT_FIXTURE/"}"
}

make_fixture() {
    local name=$1 root source submodule vcpkg top commit vcpkg_commit top_commit
    KIT_FIXTURE="$TMP_ROOT/$name/kit"
    root="$TMP_ROOT/$name"
    source="$root/source-origin"
    submodule="$root/submodule-origin"
    vcpkg="$root/vcpkg-origin"
    top="$root/topmost-origin"
    mkdir -p "$root" "$KIT_FIXTURE/artifacts"

    git_init "$submodule"
    printf '%s\n' fixture-submodule > "$submodule/submodule.txt"
    git -C "$submodule" add submodule.txt
    git -C "$submodule" commit -q -m submodule

    git_init "$source"
    mkdir -p "$source/.cargo" "$source/.github/workflows" "$source/vendor/crate" "$source/libs"
    cat > "$source/.cargo/config.vendor.toml" <<'EOF'
[net]
offline = true

[source.crates-io]
replace-with = "vendored-sources"

[source.vendored-sources]
directory = "vendor"
EOF
    printf '%s\n' 'license evidence' > "$source/vendor/crate/LICENSE"
    printf '%s\n' 'fixture crate' > "$source/vendor/crate/lib.rs"
    printf '%s\n' 'tracked source' > "$source/tracked.txt"
    cat > "$source/.github/workflows/third-party-RustDeskTempTopMostWindow.yml" <<'EOF'
name: fixture
steps:
  - run: git checkout 2222222222222222222222222222222222222222
EOF
    git -C "$source" -c protocol.file.allow=always submodule add -q "$submodule" libs/hbb_common
    git -C "$source" add .gitmodules .github/workflows/third-party-RustDeskTempTopMostWindow.yml tracked.txt libs/hbb_common
    git -C "$source" commit -q -m source
    git -C "$source" tag test-ref
    commit=$(git -C "$source" rev-parse HEAD)

    git_init "$vcpkg"
    printf '%s\n' fixture-vcpkg > "$vcpkg/vcpkg.txt"
    git -C "$vcpkg" add vcpkg.txt
    git -C "$vcpkg" commit -q -m vcpkg
    vcpkg_commit=$(git -C "$vcpkg" rev-parse HEAD)

    git_init "$top"
    printf '%s\n' fixture-topmost > "$top/topmost.txt"
    git -C "$top" add topmost.txt
    git -C "$top" commit -q -m topmost
    top_commit=$(git -C "$top" rev-parse HEAD)
    # The workflow pin is deliberately independent of this fixture repository;
    # the bundle only needs to prove the configured commit below.
    git -C "$top" commit --allow-empty -q -m workflow-pin
    top_commit=$(git -C "$top" rev-parse HEAD)
    awk -v pin="$top_commit" '{gsub(/2222222222222222222222222222222222222222/, pin); print}' \
        "$source/.github/workflows/third-party-RustDeskTempTopMostWindow.yml" > "$source/.github/workflows/pin.tmp"
    mv "$source/.github/workflows/pin.tmp" "$source/.github/workflows/third-party-RustDeskTempTopMostWindow.yml"
    git -C "$source" add .github/workflows/third-party-RustDeskTempTopMostWindow.yml
    git -C "$source" commit -q -m workflow-pin
    commit=$(git -C "$source" rev-parse HEAD)
    git -C "$source" tag -f test-ref

    git -C "$source" bundle create "$KIT_FIXTURE/artifacts/rustdesk-test-ref.bundle" --all >/dev/null
    git -C "$top" bundle create "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow.bundle" --all >/dev/null
    git -c protocol.file.allow=always clone -q --recurse-submodules "$source" "$KIT_FIXTURE/artifacts/rustdesk-src"
    git clone -q "$vcpkg" "$KIT_FIXTURE/artifacts/vcpkg"
    git clone -q "$top" "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow"
    mkdir -p "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo" "$KIT_FIXTURE/artifacts/rustdesk-src/vendor"
    cp "$source/.cargo/config.vendor.toml" "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml"
    cp -R "$source/vendor/." "$KIT_FIXTURE/artifacts/rustdesk-src/vendor/"

    printf '%s\n' fixture-engine > "$KIT_FIXTURE/artifacts/windows-x64-release.zip"
    printf '%s\n' fixture-flutter-win > "$KIT_FIXTURE/artifacts/flutter_windows_3.24.5-stable.zip"
    printf '%s\n' fixture-flutter-linux > "$KIT_FIXTURE/artifacts/flutter_linux_3.24.5-stable.tar.xz"
    printf '%s\n' fixture-rust > "$KIT_FIXTURE/artifacts/rust-1.75.0-x86_64-pc-windows-msvc.msi"
    printf '%s\n' fixture-usb > "$KIT_FIXTURE/artifacts/usbmmidd_v2.zip"
    printf '%s\n' fixture-driver > "$KIT_FIXTURE/artifacts/rustdesk_printer_driver_v4-1.4.zip"
    printf '%s\n' fixture-adapter > "$KIT_FIXTURE/artifacts/printer_driver_adapter.zip"
    printf '%s\n' fixture-sums > "$KIT_FIXTURE/artifacts/printer_sha256sums"
    tar -C "$KIT_FIXTURE/artifacts/rustdesk-src" -czf "$KIT_FIXTURE/artifacts/vendor-test-ref.tar.gz" vendor

    cp "$KIT_DIR/verify.sh" "$KIT_FIXTURE/verify.sh"
    cp "$KIT_DIR/freeze.sh" "$KIT_FIXTURE/freeze.sh"
    cat > "$KIT_FIXTURE/versions.env" <<EOF
RUSTDESK_REPO="file://fixture/source"
RUSTDESK_REF="test-ref"
RUSTDESK_COMMIT="$commit"
HBB_COMMON_REPO="file://fixture/hbb_common"
RUST_VERSION="1.75.0"
LLVM_VERSION="15.0.6"
FLUTTER_VERSION="3.24.5"
FLUTTER_RUST_BRIDGE_VERSION="1.80"
NDK_VERSION="r28c"
CARGO_NDK_VERSION="3.1.2"
FLUTTER_ENGINE_WIN_URL="https://example.invalid/engine"
FLUTTER_SDK_WIN_URL="https://example.invalid/flutter-win"
FLUTTER_SDK_LINUX_URL="https://example.invalid/flutter-linux"
VCPKG_REPO="file://fixture/vcpkg"
VCPKG_BASELINE="$vcpkg_commit"
VCPKG_TRIPLET="x64-windows-static"
TOPMOST_REPO="file://fixture/topmost"
TOPMOST_COMMIT="$top_commit"
USBMMIDD_URL="https://example.invalid/usb"
PRINTER_DRIVER_URL="https://example.invalid/driver"
PRINTER_ADAPTER_URL="https://example.invalid/adapter"
PRINTER_SUMS_URL="https://example.invalid/sums"
ARTIFACTS_DIR="artifacts"
RUSTDESK_BUNDLE_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/rustdesk-test-ref.bundle")"
VENDOR_TARBALL_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/vendor-test-ref.tar.gz")"
FLUTTER_ENGINE_WIN_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/windows-x64-release.zip")"
FLUTTER_SDK_WIN_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/flutter_windows_3.24.5-stable.zip")"
FLUTTER_SDK_LINUX_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/flutter_linux_3.24.5-stable.tar.xz")"
RUST_WIN_MSI_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/rust-1.75.0-x86_64-pc-windows-msvc.msi")"
TOPMOST_BUNDLE_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow.bundle")"
USBMMIDD_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/usbmmidd_v2.zip")"
PRINTER_DRIVER_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/rustdesk_printer_driver_v4-1.4.zip")"
PRINTER_ADAPTER_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/printer_driver_adapter.zip")"
PRINTER_SUMS_SHA256="$(hash_file "$KIT_FIXTURE/artifacts/printer_sha256sums")"
EOF

    local source_tree vendor_tree vcpkg_tree topmost_tree
    source_tree=$(git -C "$KIT_FIXTURE/artifacts/rustdesk-src" rev-parse HEAD^{tree})
    vendor_tree=$(content_hash_dir "$KIT_FIXTURE/artifacts/rustdesk-src/vendor")
    vcpkg_tree=$(git -C "$KIT_FIXTURE/artifacts/vcpkg" rev-parse HEAD^{tree})
    topmost_tree=$(git -C "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow" rev-parse HEAD^{tree})
    {
        printf '%s\n' '# fixture manifest'
        printf '%s\n' '# schema=2'
        printf '%s\n' "# pin RUSTDESK_REF=test-ref"
        printf '%s\n' "# pin RUSTDESK_COMMIT=$commit"
        printf '%s\n' '# pin RUST_VERSION=1.75.0'
        printf '%s\n' '# pin LLVM_VERSION=15.0.6'
        printf '%s\n' '# pin FLUTTER_VERSION=3.24.5'
        printf '%s\n' '# pin FLUTTER_RUST_BRIDGE_VERSION=1.80'
        printf '%s\n' '# pin NDK_VERSION=r28c'
        printf '%s\n' '# pin CARGO_NDK_VERSION=3.1.2'
        printf '%s\n' "# pin VCPKG_BASELINE=$vcpkg_commit"
        printf '%s\n' '# pin VCPKG_TRIPLET=x64-windows-static'
        printf '%s  %s  git-tree:%s  artifacts/rustdesk-src\n' rustdesk-source "$(du -sb "$KIT_FIXTURE/artifacts/rustdesk-src" | awk '{print $1}')" "$source_tree"
        write_file_record rustdesk-bundle "$KIT_FIXTURE/artifacts/rustdesk-test-ref.bundle"
        printf '%s  %s  content-sha256:%s  artifacts/rustdesk-src/vendor\n' vendor-source "$(du -sb "$KIT_FIXTURE/artifacts/rustdesk-src/vendor" | awk '{print $1}')" "$vendor_tree"
        write_file_record vendor-config "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml"
        write_file_record vendor-tarball "$KIT_FIXTURE/artifacts/vendor-test-ref.tar.gz"
        write_file_record flutter-engine-win "$KIT_FIXTURE/artifacts/windows-x64-release.zip"
        write_file_record flutter-sdk-win "$KIT_FIXTURE/artifacts/flutter_windows_3.24.5-stable.zip"
        write_file_record flutter-sdk-linux "$KIT_FIXTURE/artifacts/flutter_linux_3.24.5-stable.tar.xz"
        printf '%s  %s  git-tree:%s  artifacts/vcpkg\n' vcpkg-src "$(du -sb "$KIT_FIXTURE/artifacts/vcpkg" | awk '{print $1}')" "$vcpkg_tree"
        printf '%s  %s  git-tree:%s  artifacts/RustDeskTempTopMostWindow\n' topmost-src "$(du -sb "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow" | awk '{print $1}')" "$topmost_tree"
        write_file_record rust-win-msvc "$KIT_FIXTURE/artifacts/rust-1.75.0-x86_64-pc-windows-msvc.msi"
        write_file_record topmost-bundle "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow.bundle"
        write_file_record usbmmidd "$KIT_FIXTURE/artifacts/usbmmidd_v2.zip"
        write_file_record printer-driver "$KIT_FIXTURE/artifacts/rustdesk_printer_driver_v4-1.4.zip"
        write_file_record printer-adapter "$KIT_FIXTURE/artifacts/printer_driver_adapter.zip"
        write_file_record printer-sums "$KIT_FIXTURE/artifacts/printer_sha256sums"
    } > "$KIT_FIXTURE/artifacts/MANIFEST.txt"
}

expect_failure() {
    local name=$1 marker=$2 output
    output="$TMP_ROOT/$name.out"
    shift 2
    if "$@" > "$output" 2>&1; then
        printf '%s\n' "[fixture][FAIL] expected failure: $name" >&2
        return 1
    fi
    awk -v marker="$marker" 'index($0, marker) { found = 1 } END { exit found ? 0 : 1 }' "$output" || {
        printf '%s\n' "[fixture][FAIL] missing failure marker for $name" >&2
        return 1
    }
}

run_verify() {
    local fixture=$1
    ( cd "$fixture" && ./verify.sh )
}

run_freeze_source() {
    local fixture=$1
    ( cd "$fixture" && bash freeze.sh source )
}

run_freeze_stage() {
    local fixture=$1 stage=$2
    ( cd "$fixture" && bash freeze.sh "$stage" )
}

run_freeze_args() {
    local fixture=$1
    shift
    ( cd "$fixture" && bash freeze.sh "$@" )
}

run_verify_overrides() {
    local fixture=$1 repo=$2 ref=$3
    ( cd "$fixture" && ARTIFACTS_DIR="$fixture/artifacts" \
        RUSTDESK_REPO="$repo" RUSTDESK_REF="$ref" ./verify.sh )
}

replace_manifest_file_record() {
    local label=$1 path=$2 manifest=$3
    awk -v label="$label" -v size="$(stat -c '%s' "$path")" \
        -v digest="$(hash_file "$path")" \
        '$1 == label { printf "%s  %s  sha256:%s  %s\n", label, size, digest, $4; next } { print }' \
        "$manifest" > "$manifest.tmp"
    mv "$manifest.tmp" "$manifest"
}

replace_manifest_directory_size() {
    local label=$1 path=$2 manifest=$3 size
    size=$(du -sb "$path" | awk '{print $1}')
    awk -v label="$label" -v size="$size" \
        '$1 == label { $2 = size; print; next } { print }' \
        "$manifest" > "$manifest.tmp"
    mv "$manifest.tmp" "$manifest"
}

make_fixture valid
chmod +x "$KIT_FIXTURE/verify.sh"
valid_output_before=$(run_verify "$KIT_FIXTURE" 2>&1)
valid_output_after=$(run_verify "$KIT_FIXTURE" 2>&1)
[ "$valid_output_before" = "$valid_output_after" ] || die 'verification output is not deterministic'
run_verify "$KIT_FIXTURE" >/dev/null
pass 'generated vendor/config paths are allowed and verification is deterministic'

make_fixture vendor-config-generated
cat > "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" <<'EOF'
[net]
offline = true

[source.crates-io]
replace-with = "vendored-sources"

[source."git+https://example.invalid/fixture-crate?rev=abcdef0"]
git = "https://example.invalid/fixture-crate"
rev = "abcdef0"
replace-with = "vendored-sources"

[source.vendored-sources]
directory = "vendor"
EOF
replace_manifest_file_record vendor-config \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" \
    "$KIT_FIXTURE/artifacts/MANIFEST.txt"
replace_manifest_directory_size rustdesk-source \
    "$KIT_FIXTURE/artifacts/rustdesk-src" "$KIT_FIXTURE/artifacts/MANIFEST.txt"
run_verify "$KIT_FIXTURE" >/dev/null
pass 'cargo-generated git source replacement remains allowed only when locally vendored'

run_verify_overrides "$KIT_FIXTURE" 'https://mutable.example.invalid/main' test-ref >/dev/null
pass 'verify honors repository and artifact/ref overrides without using a mutable source as proof'
expect_failure override-ref 'manifest pin mismatch or missing: RUSTDESK_REF' \
    run_verify_overrides "$KIT_FIXTURE" 'https://mutable.example.invalid/main' wrong-ref
pass 'ref override cannot bypass immutable manifest/tag equality'

make_fixture acquisition-dirty
printf '%s\n' unexpected > "$KIT_FIXTURE/artifacts/rustdesk-src/acquisition-dirty.txt"
expect_failure acquisition-dirty 'source checkout has unexpected dirty or untracked path' run_freeze_source "$KIT_FIXTURE"
pass 'freeze rejects unexpected source dirt before staging'

make_fixture dirty
printf '%s\n' dirty >> "$KIT_FIXTURE/artifacts/rustdesk-src/tracked.txt"
expect_failure dirty 'source checkout has unexpected dirty or untracked path' run_verify "$KIT_FIXTURE"
pass 'unexpected dirty source file fails closed'

make_fixture untracked
printf '%s\n' unexpected > "$KIT_FIXTURE/artifacts/rustdesk-src/unexpected.txt"
expect_failure untracked 'source checkout has unexpected dirty or untracked path' run_verify "$KIT_FIXTURE"
pass 'unexpected untracked source file fails closed'

make_fixture legacy-manifest
awk '$0 != "# schema=2"' "$KIT_FIXTURE/artifacts/MANIFEST.txt" > "$KIT_FIXTURE/artifacts/MANIFEST.legacy"
mv "$KIT_FIXTURE/artifacts/MANIFEST.legacy" "$KIT_FIXTURE/artifacts/MANIFEST.txt"
legacy_manifest_before=$(hash_file "$KIT_FIXTURE/artifacts/MANIFEST.txt")
expect_failure legacy-manifest 'incompatible MANIFEST.txt' run_verify "$KIT_FIXTURE"
[ "$legacy_manifest_before" = "$(hash_file "$KIT_FIXTURE/artifacts/MANIFEST.txt")" ] || die 'verify changed a legacy manifest'
expect_failure legacy-manifest-freeze 'incompatible MANIFEST.txt' run_freeze_source "$KIT_FIXTURE"
[ "$legacy_manifest_before" = "$(hash_file "$KIT_FIXTURE/artifacts/MANIFEST.txt")" ] || die 'freeze changed a legacy manifest'
pass 'legacy manifest is refused without mutation and requires re-freeze'

make_fixture vendor-mismatch
mkdir -p "$TMP_ROOT/vendor-mismatch-tree"
cp -R "$KIT_FIXTURE/artifacts/rustdesk-src/vendor" "$TMP_ROOT/vendor-mismatch-tree/"
printf '%s\n' archive-only > "$TMP_ROOT/vendor-mismatch-tree/vendor/archive-only.txt"
tar -C "$TMP_ROOT/vendor-mismatch-tree" -czf "$KIT_FIXTURE/artifacts/vendor-test-ref.tar.gz" vendor
vendor_digest=$(hash_file "$KIT_FIXTURE/artifacts/vendor-test-ref.tar.gz")
awk -v digest="$vendor_digest" '$0 ~ /^VENDOR_TARBALL_SHA256=/ { print "VENDOR_TARBALL_SHA256=\"" digest "\""; next } { print }' \
    "$KIT_FIXTURE/versions.env" > "$KIT_FIXTURE/versions.env.tmp"
mv "$KIT_FIXTURE/versions.env.tmp" "$KIT_FIXTURE/versions.env"
replace_manifest_file_record vendor-tarball "$KIT_FIXTURE/artifacts/vendor-test-ref.tar.gz" "$KIT_FIXTURE/artifacts/MANIFEST.txt"
expect_failure vendor-mismatch 'vendor tarball contents mismatch with staged vendor tree' run_verify "$KIT_FIXTURE"
pass 'vendor tarball contents are compared with the staged vendor tree'

make_fixture vendor-config-mismatch
awk '{ if ($0 ~ /^directory[[:space:]]*=/) print "directory = \"../remote-vendor\""; else print }' \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" > \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml.tmp"
mv "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml.tmp" \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml"
replace_manifest_file_record vendor-config \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" \
    "$KIT_FIXTURE/artifacts/MANIFEST.txt"
expect_failure vendor-config-mismatch 'vendor config contains unknown or network-capable settings' run_verify "$KIT_FIXTURE"
pass 'vendor config must select local vendor sources without remote replacement'

make_fixture symlink-escape
printf '%s\n' outside > "$TMP_ROOT/symlink-target"
rm -f "$KIT_FIXTURE/artifacts/windows-x64-release.zip"
ln -s "$TMP_ROOT/symlink-target" "$KIT_FIXTURE/artifacts/windows-x64-release.zip"
expect_failure symlink-escape 'flutter-engine-win path escapes kit root' run_verify "$KIT_FIXTURE"
pass 'symlinked expected artifact escaping the kit fails before hashing'

make_fixture symlink-output-entry
rm -f "$KIT_FIXTURE/artifacts/windows-x64-release.zip"
ln -s "$TMP_ROOT/symlink-output-target" "$KIT_FIXTURE/artifacts/windows-x64-release.zip"
expect_failure symlink-output-entry 'flutter-engine-win path escapes kit root' \
    run_freeze_stage "$KIT_FIXTURE" engine
pass 'symlinked output is rejected before download/write'

make_fixture symlink-directory
mv "$KIT_FIXTURE/artifacts/rustdesk-src/vendor" "$TMP_ROOT/vendor-target"
ln -s "$TMP_ROOT/vendor-target" "$KIT_FIXTURE/artifacts/rustdesk-src/vendor"
expect_failure symlink-directory 'vendor-source path escapes kit root' run_verify "$KIT_FIXTURE"
pass 'symlinked expected directory escaping the kit fails before hashing'

make_fixture symlink-entry
printf '%s\n' outside > "$TMP_ROOT/vendor-entry-target"
ln -s "$TMP_ROOT/vendor-entry-target" "$KIT_FIXTURE/artifacts/rustdesk-src/vendor/crate/escape"
expect_failure symlink-entry 'vendor-source contains a symlink' run_verify "$KIT_FIXTURE"
pass 'symlinked directory entry fails before content hashing'

make_fixture symlink-rustdesk-src
mv "$KIT_FIXTURE/artifacts/rustdesk-src" "$TMP_ROOT/rustdesk-src-target"
ln -s "$TMP_ROOT/rustdesk-src-target" "$KIT_FIXTURE/artifacts/rustdesk-src"
expect_failure symlink-rustdesk-src 'rustdesk-src path escapes kit root' run_freeze_source "$KIT_FIXTURE"
pass 'symlinked rustdesk-src is rejected at freeze stage entry'

make_fixture symlink-cargo
mv "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo" "$TMP_ROOT/cargo-target"
ln -s "$TMP_ROOT/cargo-target" "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo"
expect_failure symlink-cargo 'rustdesk-src contains a symlink or non-regular entry' run_freeze_stage "$KIT_FIXTURE" vendor
pass 'symlinked .cargo is rejected before vendor generation'

make_fixture symlink-vendor
mv "$KIT_FIXTURE/artifacts/rustdesk-src/vendor" "$TMP_ROOT/vendor-target"
ln -s "$TMP_ROOT/vendor-target" "$KIT_FIXTURE/artifacts/rustdesk-src/vendor"
expect_failure symlink-vendor 'rustdesk-src contains a symlink or non-regular entry' run_freeze_stage "$KIT_FIXTURE" vendor
pass 'symlinked vendor directory is rejected before vendor generation'

make_fixture special-entry
mkfifo "$KIT_FIXTURE/artifacts/rustdesk-src/vendor/crate/special"
expect_failure special-entry 'vendor-source contains a symlink or non-regular entry' run_verify "$KIT_FIXTURE"
pass 'special vendor entries fail before content hashing'

make_fixture vendor-config-network
cat > "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" <<'EOF'
[source.crates-io]
replace-with = "vendored-sources"

[source.vendored-sources]
directory = "vendor"

[net]
offline = false
EOF
replace_manifest_file_record vendor-config \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" \
    "$KIT_FIXTURE/artifacts/MANIFEST.txt"
expect_failure vendor-config-network 'vendor config contains unknown or network-capable settings' run_verify "$KIT_FIXTURE"
pass 'network-enabled Cargo config is rejected'

make_fixture vendor-config-remote-replacement
cat > "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" <<'EOF'
[net]
offline = true

[source.crates-io]
replace-with = "remote-crates"

[source.remote-crates]
registry = "https://example.invalid/index"

[source.vendored-sources]
directory = "vendor"
EOF
replace_manifest_file_record vendor-config \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" \
    "$KIT_FIXTURE/artifacts/MANIFEST.txt"
expect_failure vendor-config-remote-replacement 'vendor config contains unknown or network-capable settings' run_verify "$KIT_FIXTURE"
pass 'remote Cargo source replacement is rejected even with offline mode'

make_fixture vendor-config-unknown
cat > "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" <<'EOF'
[source.crates-io]
replace-with = "vendored-sources"

[source.vendored-sources]
directory = "vendor"

[registries.crates-io]
index = "https://example.invalid/index"
EOF
replace_manifest_file_record vendor-config \
    "$KIT_FIXTURE/artifacts/rustdesk-src/.cargo/config.vendor.toml" \
    "$KIT_FIXTURE/artifacts/MANIFEST.txt"
expect_failure vendor-config-unknown 'vendor config contains unknown or network-capable settings' run_verify "$KIT_FIXTURE"
pass 'unknown Cargo registry config is rejected'

make_fixture stage-stop
stage_stop_output="$TMP_ROOT/stage-stop.out"
if run_freeze_args "$KIT_FIXTURE" unknown engine > "$stage_stop_output" 2>&1; then
    die 'unknown stage unexpectedly succeeded'
fi
if awk 'index($0, "STAGE: flutter engine") { found = 1 } END { exit found ? 0 : 1 }' \
    "$stage_stop_output"; then
    die 'freeze continued after a failed stage-entry validation'
fi
pass 'freeze stops immediately after a failed stage-entry validation'

make_fixture secret
printf '%s\n' secret > "$KIT_FIXTURE/artifacts/fixture-secret.key"
secret_output="$TMP_ROOT/secret.out"
if run_verify "$KIT_FIXTURE" > "$secret_output" 2>&1; then
    die 'secret presence unexpectedly verified'
fi
awk 'index($0, "[redacted path]") { found = 1 } END { exit found ? 0 : 1 }' "$secret_output" || die 'secret failure was not redacted'
awk 'index($0, "fixture-secret.key") { found = 1 } END { exit found ? 1 : 0 }' "$secret_output" || die 'secret filename leaked in verifier output'
pass 'secret presence fails closed with a redacted reason'

make_fixture secret-custom-txt
printf '%s\n' secret > "$KIT_FIXTURE/artifacts/custom_.txt"
custom_secret_output="$TMP_ROOT/secret-custom-txt.out"
if run_verify "$KIT_FIXTURE" > "$custom_secret_output" 2>&1; then
    die 'custom_.txt secret presence unexpectedly verified'
fi
awk 'index($0, "[redacted path]") { found = 1 } END { exit found ? 0 : 1 }' \
    "$custom_secret_output" || die 'custom_.txt secret failure was not redacted'
awk 'index($0, "custom_.txt") { found = 1 } END { exit found ? 1 : 0 }' \
    "$custom_secret_output" || die 'custom_.txt filename leaked in verifier output'
pass 'custom_.txt is secret-bearing by filename without content inspection'

make_fixture shallow
rm -rf "$KIT_FIXTURE/artifacts/rustdesk-src"
git -c protocol.file.allow=always clone -q --depth 1 --recurse-submodules \
    "file://$TMP_ROOT/shallow/source-origin" "$KIT_FIXTURE/artifacts/rustdesk-src"
expect_failure shallow 'source checkout is shallow' run_verify "$KIT_FIXTURE"
pass 'shallow source fails closed'

make_fixture shallow-vcpkg
rm -rf "$KIT_FIXTURE/artifacts/vcpkg"
git clone -q --depth 1 "file://$TMP_ROOT/shallow-vcpkg/vcpkg-origin" \
    "$KIT_FIXTURE/artifacts/vcpkg"
expect_failure shallow-vcpkg 'vcpkg source checkout is shallow' run_verify "$KIT_FIXTURE"
pass 'shallow vcpkg source fails closed'

make_fixture shallow-topmost
rm -rf "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow"
git clone -q --depth 1 "file://$TMP_ROOT/shallow-topmost/topmost-origin" \
    "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow"
expect_failure shallow-topmost 'TopMostWindow source checkout is shallow' run_verify "$KIT_FIXTURE"
pass 'shallow TopMostWindow source fails closed'

make_fixture vcpkg-dirty
printf '%s\n' unexpected > "$KIT_FIXTURE/artifacts/vcpkg/unexpected.txt"
vcpkg_manifest_before=$(hash_file "$KIT_FIXTURE/artifacts/MANIFEST.txt")
expect_failure vcpkg-dirty 'vcpkg source checkout has unexpected untracked or modified files' \
    run_freeze_stage "$KIT_FIXTURE" vcpkg
[ "$vcpkg_manifest_before" = "$(hash_file "$KIT_FIXTURE/artifacts/MANIFEST.txt")" ] ||
    die 'vcpkg preflight changed the manifest'
pass 'dirty vcpkg source fails before staging mutation'

make_fixture topmost-dirty
printf '%s\n' unexpected > "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow/unexpected.txt"
expect_failure topmost-dirty 'TopMostWindow source checkout has unexpected untracked or modified files' \
    run_freeze_stage "$KIT_FIXTURE" thirdparty
pass 'dirty TopMostWindow source fails before staging mutation'

make_fixture vcpkg-uninitialized
mv "$KIT_FIXTURE/artifacts/vcpkg/.git" "$TMP_ROOT/vcpkg-uninitialized-git"
expect_failure vcpkg-uninitialized 'vcpkg source is not an initialized Git checkout' run_verify "$KIT_FIXTURE"
pass 'uninitialized vcpkg source fails closed'

make_fixture topmost-uninitialized
mv "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow/.git" "$TMP_ROOT/topmost-uninitialized-git"
expect_failure topmost-uninitialized 'TopMostWindow source is not an initialized Git checkout' run_verify "$KIT_FIXTURE"
pass 'uninitialized TopMostWindow source fails closed'

make_fixture vcpkg-mismatched
git -C "$KIT_FIXTURE/artifacts/vcpkg" commit --allow-empty -q -m mismatch
expect_failure vcpkg-mismatched 'vcpkg source commit mismatch' run_verify "$KIT_FIXTURE"
pass 'mismatched vcpkg pin fails closed'

make_fixture topmost-mismatched
git -C "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow" commit --allow-empty -q -m mismatch
expect_failure topmost-mismatched 'TopMostWindow source commit mismatch' run_verify "$KIT_FIXTURE"
pass 'mismatched TopMostWindow pin fails closed'

make_fixture submodule-uninitialized
git -C "$KIT_FIXTURE/artifacts/rustdesk-src" submodule deinit -f -q -- libs/hbb_common
expect_failure submodule-uninitialized 'recursive source submodule is uninitialized' run_verify "$KIT_FIXTURE"
pass 'uninitialized recursive submodule fails closed'

make_fixture submodule-modified
printf '%s\n' modified >> "$KIT_FIXTURE/artifacts/rustdesk-src/libs/hbb_common/submodule.txt"
expect_failure submodule-modified 'source checkout has unexpected dirty or untracked path' run_verify "$KIT_FIXTURE"
pass 'modified recursive submodule fails closed'

make_fixture submodule-mismatched
git -C "$KIT_FIXTURE/artifacts/rustdesk-src/libs/hbb_common" config user.name fixture
git -C "$KIT_FIXTURE/artifacts/rustdesk-src/libs/hbb_common" config user.email fixture@example.invalid
git -C "$KIT_FIXTURE/artifacts/rustdesk-src/libs/hbb_common" commit --allow-empty -q -m mismatch
expect_failure submodule-mismatched 'recursive source submodule is uninitialized, modified, or mismatched' run_verify "$KIT_FIXTURE"
pass 'mismatched recursive submodule fails closed'

make_fixture hash
printf '%s\n' changed > "$KIT_FIXTURE/artifacts/windows-x64-release.zip"
expect_failure hash 'manifest SHA-256 mismatch: flutter-engine-win' run_verify "$KIT_FIXTURE"
pass 'content hash mismatch fails closed'

make_fixture entrypoint
chmod 644 "$KIT_FIXTURE/verify.sh"
manifest_before=$(hash_file "$KIT_FIXTURE/artifacts/MANIFEST.txt")
source_status_before=$(git -C "$KIT_FIXTURE/artifacts/rustdesk-src" status --porcelain=v1 --untracked-files=all)
vcpkg_status_before=$(git -C "$KIT_FIXTURE/artifacts/vcpkg" status --porcelain=v1 --untracked-files=all)
topmost_status_before=$(git -C "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow" status --porcelain=v1 --untracked-files=all)
( cd "$KIT_FIXTURE" && bash freeze.sh --verify ) >/dev/null
[ "$manifest_before" = "$(hash_file "$KIT_FIXTURE/artifacts/MANIFEST.txt")" ]
[ "$source_status_before" = "$(git -C "$KIT_FIXTURE/artifacts/rustdesk-src" status --porcelain=v1 --untracked-files=all)" ]
[ "$vcpkg_status_before" = "$(git -C "$KIT_FIXTURE/artifacts/vcpkg" status --porcelain=v1 --untracked-files=all)" ]
[ "$topmost_status_before" = "$(git -C "$KIT_FIXTURE/artifacts/RustDeskTempTopMostWindow" status --porcelain=v1 --untracked-files=all)" ]
pass 'freeze --verify invokes a non-executable verifier robustly'

printf '%s\n' '[fixture] all offline-kit verifier fixtures passed'
