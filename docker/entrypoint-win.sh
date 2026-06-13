#!/bin/bash
# Build Agent: Windows (.exe + .msi)
# Cross-compiles from Linux using MinGW
# Note: do NOT use `set -e` — any subcommand failure would kill PID 1
# and restart the whole container. We use explicit error handling instead.

JOBS_DIR="/rdgen-data/jobs"
OUTPUT_DIR="/rdgen-data/output"
CACHE_DIR="/rustdesk-cache"
PATCHES_DIR="/rdgen-data/patches"

mkdir -p "$JOBS_DIR" "$OUTPUT_DIR" "$CACHE_DIR"

echo "Build Agent Windows: started. Watching $JOBS_DIR for jobs..."

process_job() {
    local job_file="$1"
    local job_id
    job_id=$(basename "$job_file" .json)
    local platform version app_name custom_json
    platform=$(jq -r '.platform' "$job_file")
    version=$(jq -r '.version' "$job_file")
    app_name=$(jq -r '.app_name' "$job_file")
    custom_json=$(jq -r '.custom_json' "$job_file")

    local output_dir="$OUTPUT_DIR/$job_id"
    mkdir -p "$output_dir"

    echo "Job $job_id: platform=$platform version=$version app=$app_name"

    if [ -z "$platform" ] || [ "$platform" = "null" ]; then
        echo "Job $job_id: invalid job file (no platform), skipping"
        echo "failed" > "$output_dir/status"
        rm -f "$job_file"
        return
    fi

    if [ "$platform" != "windows" ] && [ "$platform" != "windows-x86" ]; then
        echo "Job $job_id: skipping $platform job on Windows agent"
        rm -f "$job_file"
        return
    fi

    # Update status to building
    echo "building" > "$output_dir/status"

    # Use pre-baked rustdesk source (baked into image during Docker build)
    local src_dir="/opt/rustdesk-src"
    echo "Using baked rustdesk source at $src_dir (version: baked)"

    # Persist target/ across container restarts (avoids full rebuild each time)
    local cache_target="$CACHE_DIR/target-win"
    mkdir -p "$cache_target"
    # Remove stale symlink or empty dir if present
    if [ -L "$src_dir/target" ] || [ -d "$src_dir/target" ]; then
        rm -rf "$src_dir/target"
    fi
    ln -sf "$cache_target" "$src_dir/target"
    echo "Cached target: $cache_target"

    cd "$src_dir" || { echo "Job $job_id: cd to $src_dir failed"; echo "failed" > "$output_dir/status"; rm -f "$job_file"; return; }

    # Apply patches (optional)
    if [ -d "$PATCHES_DIR" ]; then
        for patch in "$PATCHES_DIR"/*.diff; do
            if [ -f "$patch" ]; then
                echo "Applying patch: $(basename "$patch")"
                patch -p1 < "$patch" >/dev/null 2>&1 || echo "Warning: patch $(basename "$patch") failed (continuing)"
            fi
        done
    fi

    # Write custom config
    mkdir -p flutter/lib
    echo "$custom_json" > flutter/lib/custom.json

    echo "Cross-compiling Windows ($platform) .exe..."

    # Runtime fix: bare min(nin,n) type deduction fails with MinGW (DWORD != uint32_t)
    # Add NOMINMAX to prevent windows.h from defining min/max macros (conflicts with std::min)
    sed -i '1s/^/#define NOMINMAX\n/' "$src_dir/src/platform/windows.cc"
    sed -i 's/\bmin(nin, n)/std::min<uint32_t>(nin, n)/g' "$src_dir/src/platform/windows.cc"

    # Replace comdef.h (MinGW _com_error uses sprintf_s which is MSVC-only) with oleauto.h
    sed -i 's/#include <comdef.h>/#include <oleauto.h>\n#include "mingw_compat.h"/' "$src_dir/src/platform/windows.cc"

    # Add compiler flags for MinGW compatibility (narrowing, goto-crosses-init, wsprintfW deprecation)
    sed -i '/\.flag("-Wno-unused-function")/a\        .flag("-Wno-narrowing")\n        .flag("-DSTRSAFE_NO_DEPRECATE")' "$src_dir/build.rs"

    # Fix C++17 goto-crosses-init: move DWORD declarations before first goto
    python3 << 'PYEOF'
import pathlib
p = pathlib.Path("/opt/rustdesk-src/src/platform/windows.cc")
s = p.read_text()
insert_after = "wchar_t *domainName = NULL;"
dwords = "    DWORD tokenInfoLength = 0;\n    DWORD userSize = 0;\n    DWORD domainSize = 0;"
s = s.replace(insert_after, insert_after + "\n" + dwords, 1)
# Remove the LAST occurrence of each duplicate (the original, not our inserted one)
for line in ["DWORD tokenInfoLength = 0;", "DWORD userSize = 0;", "DWORD domainSize = 0;"]:
    idx = s.rfind("    " + line)
    if idx > 0:
        eol = s.find("\n", idx)
        s = s[:idx] + s[eol:]
p.write_text(s)
print("Moved DWORD declarations to top of GetProcessUserName")
PYEOF

    # Don't patch vendored build.rs files (cargo checks source hashes).
    # Instead, provide missing extern "C" stubs in windows.cc for functions
    # that vendored crates expect but don't compile for MinGW cross-compilation.
    # Also fix linker for MinGW lowercase lib names.
    sed -i 's/if std::env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("windows") {/#[cfg(target_os = "windows")]\n    build_c_impl();/g' "$src_dir/libs/clipboard/build.rs"
    sed -i 's/        build_c_impl();//' "$src_dir/libs/clipboard/build.rs"
    sed -i 's/    }//' "$src_dir/libs/clipboard/build.rs"
    python3 << 'PYEOF'
# Re-write clipboard/build.rs cleanly
import pathlib
p = pathlib.Path("/opt/rustdesk-src/libs/clipboard/build.rs")
p.write_text('''#[cfg(target_os = "windows")]
fn build_c_impl() {{
    let mut build = cc::Build::new();
    build.file("src/windows/wf_cliprdr.c");
    {{
        build.flag_if_supported("-Wno-c++0x-extensions");
        build.flag_if_supported("-Wno-return-type-c-linkage");
        build.flag_if_supported("-Wno-invalid-offsetof");
        build.flag_if_supported("-Wno-unused-parameter");
        if build.get_compiler().is_like_msvc() {{
            build.define("WIN32", "");
            build.flag("-Z7");
            build.flag("-GR-");
        }} else {{
            build.flag("-fPIC");
        }}
        build.compile("mycliprdr");
    }}
    println!("cargo:rerun-if-changed=src/windows/wf_cliprdr.c");
}}

fn main() {{
    #[cfg(target_os = "windows")]
    build_c_impl();
}}
''')
print("Reverted clipboard/build.rs to original (uses stubs instead)")
PYEOF

    # Fix linker: MinGW uses lowercase wtsapi32, not WtsApi32
    sed -i 's/cargo:rustc-link-lib=WtsApi32/cargo:rustc-link-lib=wtsapi32/' "$src_dir/build.rs"

    # Add missing stub functions for MinGW (XPS Print only — clipboard stubs removed since
    # clipboard/build.rs now properly compiles wf_cliprdr.c for MinGW after the fix above)
    python3 << 'PYEOF'
import pathlib
p = pathlib.Path("/opt/rustdesk-src/src/platform/windows.cc")
s = p.read_text()

# The XPS section stub (empty CleanupXpsPrint) needs PrintXPSRawData for Rust linkage
stub_marker = "// XPS Print section removed for MinGW"
if stub_marker in s:
    # Replace the stub block with one that also provides PrintXPSRawData
    old_stub = 'void CleanupXpsPrint() {}'
    new_stub = '''void CleanupXpsPrint() {}
DWORD PrintXPSRawData(LPCWSTR printer_name, const BYTE* raw_data, DWORD data_size) {
    return ERROR_NOT_SUPPORTED;
}'''
    s = s.replace(old_stub, new_stub, 1)
    print("Added PrintXPSRawData stub for MinGW")

p.write_text(s)
PYEOF

    # Add clipboard stubs to windows.cc (clipboard C file has MinGW portability issues)
    python3 << 'PYEOF'
import pathlib
p = pathlib.Path("/opt/rustdesk-src/src/platform/windows.cc")
s = p.read_text()
stub = '\nextern "C" {\nvoid empty_cliprdr() {}\nvoid uninit_cliprdr() {}\nvoid init_cliprdr() {}\nint MachineUidIsWow64() { return 0; }\n}\n'
if "init_cliprdr" not in s or "MachineUidIsWow64" not in s:
    s = s.rstrip() + stub
    p.write_text(s)
    print("Added clipboard + MachineUidIsWow64 stubs for MinGW")
else:
    print("stubs already present")
PYEOF

    # scrap's build.rs already emits cargo:rustc-link-lib for vpx/libyuv/aom via vcpkg.
    # But GNU ld ordering means -lvpx comes BEFORE scrap rlib → symbols not resolved.
    # Add --whole-archive -lvpx at END to force re-scan of archive after all rlibs.
    export VCPKG_LIB=/opt/vcpkg/installed/x64-windows-static/lib
    export RUSTFLAGS="-L $VCPKG_LIB -C link-args=-Wl,--whole-archive,-lvpx,-lyuv,-laom,--no-whole-archive"

    # Force cargo to re-run rustdesk's build.rs (invalidate cached cc::Build output)
    # The cached target may have a stale windows.o from the previous build.
    rm -rf "$src_dir/target/x86_64-pc-windows-gnu/release/build/rustdesk-"*

    # Persist full build output so failures are diagnosable from rdgen-data/output/<job>/build.log
    # (without this, the only copy lives in `docker logs` of the build-win container).
    # tee preserves container stdout; PIPESTATUS[0] gives cargo's real exit code, not tee's.
    local build_log="$output_dir/build.log"
    echo "Writing build log to $build_log"
    cargo build --release --target x86_64-pc-windows-gnu 2>&1 | tee "$build_log"
    if [ "${PIPESTATUS[0]}" -eq 0 ]; then
        if cp target/x86_64-pc-windows-gnu/release/rustdesk.exe "$output_dir/rustdesk.exe"; then
            # NSIS packaging (optional — non-fatal if fails)
            makensis -DVERSION="$version" -DAPP_NAME="$app_name" \
                -DOUTFILE="$output_dir/rustdesk-setup.exe" \
                /usr/share/nsis/InstallOptions.nsi 2>/dev/null || echo "Warning: NSIS packaging failed (continuing)"

            echo "done" > "$output_dir/status"
            rm -f "$job_file"
            echo "Job $job_id finished successfully"
        else
            echo "Job $job_id: cp rustdesk.exe failed"
            echo "failed" > "$output_dir/status"
            rm -f "$job_file"
        fi
    else
        echo "Job $job_id: cargo build failed"
        echo "failed" > "$output_dir/status"
        rm -f "$job_file"
    fi
}

# Use glob that returns empty if no match (so for loop doesn't iterate over literal pattern)
shopt -s nullglob

while true; do
    for job_file in "$JOBS_DIR"/*.json; do
        if [ -f "$job_file" ]; then
            process_job "$job_file"
        fi
    done
    sleep 5
done
