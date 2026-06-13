#!/bin/bash
# Patches applied to /opt/rustdesk-src after git clone.
# We can't use sed with complex patterns in Dockerfile RUN because of bash escaping
# issues (single-quotes get nested). Instead we run python for surgical replacements.
set -e

# 1. Drop the "wayland" feature from scrap dep (rustdesk unconditionally enables it,
#    but on Windows it drags in linux-only dbus/gstreamer code).
python3 - <<'PYEOF'
import re, pathlib
p = pathlib.Path("/opt/rustdesk-src/Cargo.toml")
s = p.read_text()
new = re.sub(
    r'scrap = \{ path = "libs/scrap", features = \["wayland"\] \}',
    'scrap = { path = "libs/scrap" }',
    s,
)
if new != s:
    p.write_text(new)
    print("Patched Cargo.toml (removed wayland feature from scrap dep)")
else:
    print("Cargo.toml already patched or pattern not found")
PYEOF

# 2. Fix scrap's build.rs bug: ALL four cfg!() checks are based on the HOST
#    (Linux), not the TARGET. So when cross-compiling to Windows-gnu, NONE of
#    android/dxgi/quartz/x11 cfgs get enabled, and the wrong module branch
#    gets taken in common/mod.rs (no x11, no dxgi, no anything → fallback
#    which leaves PixelBuffer undefined for common/convert.rs, common/camera.rs,
#    and the Frame enum in common/mod.rs:177).
#    Fix: replace cfg!(...) with target_os comparisons (which build.rs already
#    has as `target_os` from CARGO_CFG_TARGET_OS).
python3 - <<'PYEOF'
import pathlib
p = pathlib.Path("/opt/rustdesk-src/libs/scrap/build.rs")
s = p.read_text()

replacements = [
    # x11: only when target is unix-like (linux/*)
    ('} else if cfg!(unix) {',
     '} else if target_os == "linux" || target_os == "freebsd" || target_os == "netbsd" || target_os == "dragonfly" {'),
    # dxgi: only on Windows
    ('} else if cfg!(windows) {',
     '} else if target_os == "windows" {'),
    # quartz: only on macOS
    ('} else if cfg!(target_os = "macos") {',
     '} else if target_os == "macos" {'),
    # android: only on Android
    ('} else if target_os == "android" {',
     '} else if target_os == "android" {'),
]

# 3. Fix find_package() — same cfg!() HOST vs TARGET bug.
#    Without this fix, if someone passes --features linux-pkg-config during
#    cross-compile to non-Linux, cfg!(target_os = "linux") is true on the Linux
#    host, but the link_pkg_config() function body is compiled with #[cfg] on
#    the TARGET (which is not Linux) → the unimplemented!() stub gets called → panic.
replacements += [
    ('if cfg!(all(target_os = "linux", feature = "linux-pkg-config"))',
     'if std::env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("linux") && cfg!(feature = "linux-pkg-config")'),
]

changed = False
for old, new in replacements:
    if old in s:
        # NOTE: we must NOT check `new not in s` as a guard — the new string may
        # coincidentally appear elsewhere in the file (e.g. `target_os == "windows"`
        # in link_vcpkg). We replace exactly one occurrence of the old string.
        s = s.replace(old, new, 1)
        changed = True
        print(f"Patched: {old[:50]}...")

if changed:
    p.write_text(s)
    print("Wrote build.rs")
else:
    print("Already patched or no changes")
PYEOF

# 4. Fix rustdesk's own build.rs: #[cfg(windows)] fn build_windows() is a HOST
#    compile-time gate. On a Linux host, this function is NEVER compiled, so
#    windows.cc (which defines ALL the missing extern "C" functions like
#    get_current_session, is_local_system, etc.) is never compiled and never
#    linked. Result: ~50 "undefined reference" linker errors.
#
#    Fix: remove #[cfg(windows)] from fn build_windows() and guard the call in
#    main() with a CARGO_CFG_TARGET_OS runtime check instead. This is the exact
#    same bug pattern as scrap/build.rs — cfg attributes in build.rs evaluate on
#    the HOST, not the TARGET.
#
#    We also need to add more Windows system libraries to the link step because
#    windows.cc uses APIs from shell32, userenv, etc. that are not in the default
#    MinGW link set.
python3 - <<'PYEOF'
import pathlib, re

p = pathlib.Path("/opt/rustdesk-src/build.rs")
s = p.read_text()

# 4a. Remove #[cfg(windows)] from fn build_windows()
old_fn = '#[cfg(windows)]\nfn build_windows()'
if old_fn in s:
    s = s.replace(old_fn, 'fn build_windows()', 1)
    print("Patched: removed #[cfg(windows)] from fn build_windows()")
else:
    print("WARNING: #[cfg(windows)] fn build_windows() not found in build.rs")

# 4b. Add extra Windows system libraries that windows.cc needs.
#    The original code only links WtsApi32, but windows.cc also uses:
#    - shell32  (AddRecentDocument → SHAddToRecentDocs)
#    - userenv  (CreateEnvironmentBlock, DestroyEnvironmentBlock in LaunchProcessWin)
#    - ole32    (CoInitializeEx, CoCreateInstance in PrintXPSRawData)
#    We leave #[cfg(all(windows, feature = "inline"))] on build_manifest() alone
#    because winres depends on winapi which is Windows-only.

# Add extra link libs after the WtsApi32 line
old_link = 'println!("cargo:rustc-link-lib=WtsApi32");'
new_link = '''println!("cargo:rustc-link-lib=WtsApi32");
    println!("cargo:rustc-link-lib=shell32");
    println!("cargo:rustc-link-lib=userenv");
    println!("cargo:rustc-link-lib=ole32");'''
if old_link in s:
    s = s.replace(old_link, new_link, 1)
    print("Patched: added shell32, userenv, ole32 link libs to build_windows()")

# 4c. Rewrite main() to use CARGO_CFG_TARGET_OS runtime check instead of #[cfg(windows)].
#    Original:
#      fn main() {
#          hbb_common::gen_version();
#          install_android_deps();
#          #[cfg(all(windows, feature = "inline"))]
#          build_manifest();
#          #[cfg(windows)]
#          build_windows();
#          let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap();
#          if target_os == "macos" { ... }
#          println!("cargo:rerun-if-changed=build.rs");
#      }
#
#    Patched:
#      fn main() {
#          hbb_common::gen_version();
#          install_android_deps();
#          #[cfg(all(windows, feature = "inline"))]
#          build_manifest();
#          let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
#          if target_os == "windows" {
#              build_windows();
#          }
#          if target_os == "macos" { ... }
#          println!("cargo:rerun-if-changed=build.rs");
#      }

old_main = '''fn main() {
    hbb_common::gen_version();
    install_android_deps();
    #[cfg(all(windows, feature = "inline"))]
    build_manifest();
    #[cfg(windows)]
    build_windows();
    let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap();
    if target_os == "macos" {
        #[cfg(target_os = "macos")]
        build_mac();
        println!("cargo:rustc-link-lib=framework=ApplicationServices");
    }
    println!("cargo:rerun-if-changed=build.rs");
}'''

new_main = '''fn main() {
    hbb_common::gen_version();
    install_android_deps();
    #[cfg(all(windows, feature = "inline"))]
    build_manifest();
    let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
    if target_os == "windows" {
        build_windows();
    }
    if target_os == "macos" {
        #[cfg(target_os = "macos")]
        build_mac();
        println!("cargo:rustc-link-lib=framework=ApplicationServices");
    }
    println!("cargo:rerun-if-changed=build.rs");
}'''

if old_main in s:
    s = s.replace(old_main, new_main, 1)
    print("Patched: rewrote main() with CARGO_CFG_TARGET_OS check for build_windows()")
else:
    print("WARNING: could not find original main() in build.rs")

p.write_text(s)
print("Wrote build.rs (main crate)")
PYEOF

# 5. Fix MinGW cross-compilation issues in windows.cc:
#    a) #include <comdef.h> uses _com_error which needs sprintf_s (MSVC-only)
#    b) PWTSINFOEXW / WTSSessionInfoEx / WTS_SESSIONSTATE_LOCK not defined in MinGW headers
#    c) IXpsOMObjectFactory / IXpsPrintJob / IXpsPrintJobStream are forward-declared
#       in MinGW's xpsprint.h but the COM interface vtables are not defined
#    d) `#include <Windows.h>` (capital W) doesn't exist on case-sensitive MinGW includes
#
#    Approach: create a mingw_compat.h header that provides the missing definitions,
#    and patch windows.cc to include it + skip the XPS Print section for MinGW.
#    Also fix windows_delete_test_cert.cc case issue.
python3 - <<'PYEOF'
import pathlib

# 5a. Create mingw_compat.h in src/platform/ with missing Win32 definitions
compat_header = r'''#ifndef MINGW_COMPAT_H
#define MINGW_COMPAT_H

#ifdef __MINGW32__

// PWTSINFOEXW / WTS_SESSIONSTATE_LOCK are missing from MinGW wtsapi32.h
// https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/ns-wtsapi32-wtsinfoexw
#ifndef WTS_SESSIONSTATE_LOCK
#define WTS_SESSIONSTATE_LOCK 0
#endif

typedef struct _WTSINFOEX_LEVEL1_W {
    DWORD SessionFlags;
    DWORD SessionId;
} WTSINFOEX_LEVEL1W, *PWTSINFOEX_LEVEL1W;

typedef struct _WTSINFOEXW {
    DWORD Level;
    union {
        WTSINFOEX_LEVEL1W WTSInfoExLevel1;
    } Data;
} WTSINFOEXW, *PWTSINFOEXW;

#ifndef WTSSessionInfoEx
#define WTSSessionInfoEx ((WTS_INFO_CLASS)33)
#endif

#endif // __MINGW32__
#endif // MINGW_COMPAT_H
'''

compat_path = pathlib.Path("/opt/rustdesk-src/src/platform/mingw_compat.h")
compat_path.write_text(compat_header)
print("Created mingw_compat.h")

# 5b. Patch windows.cc to work with MinGW
p = pathlib.Path("/opt/rustdesk-src/src/platform/windows.cc")
s = p.read_text()
changed = False

# Fix case: Windows.h -> windows.h (MinGW headers are case-sensitive on Linux)
if '#include <Windows.h>' in s:
    s = s.replace('#include <Windows.h>', '#include <windows.h>', 1)
    changed = True
    print("Patched windows.cc: Windows.h -> windows.h")

# Wrap #include <xpsprint.h> with MinGW guard (MinGW only has forward declarations)
old_xps_inc = '#include <xpsprint.h>'
if old_xps_inc in s and '#ifndef __MINGW32__' not in s:
    s = s.replace(
        old_xps_inc,
        '#ifndef __MINGW32__\n// xpsprint.h only has forward declarations in MinGW\n#include <xpsprint.h>\n#endif',
        1,
    )
    changed = True
    print("Patched windows.cc: wrapped xpsprint.h with __MINGW32__ guard")

# Add mingw_compat.h include after other includes (before the extern "C" blocks)
# We add it right before the first extern "C" block
if '#include "mingw_compat.h"' not in s:
    # Insert just before the first extern "C" { block
    # The first extern "C" block starts with the GetSessionUserTokenWin function groups
    s = s.replace(
        '\n// ultravnc has rdp support\n',
        '\n#include "mingw_compat.h"\n// ultravnc has rdp support\n',
        1,
    )
    changed = True
    print("Patched windows.cc: added mingw_compat.h include")

# Fix _com_error usage: MinGW's comdef.h has sprintf_s which doesn't exist.
# The only place _com_error is used is in PrintXPSRawData's PRINT_XPS_CHECK_HR macro.
# Wrap the entire XPS Print section with #ifndef __MINGW32__
# NOTE: guard string must NOT match the xpsprint.h wrapper guard (we use a specific
# marker string for the code section vs the include section)
xps_code_guard = '// XPS Print section removed for MinGW'
if xps_code_guard not in s and 'PrintXPSRawData' in s:
    # Find start of XPS section - the "// Remote printing" comment
    xps_start = '// Remote printing'
    idx = s.find(xps_start)
    if idx > 0:
        # The XPS section occupies the rest of the file.
        # It starts with:
        #   // Remote printing
        #   extern "C"
        #   {
        #   ...
        # And ends at EOF.
        # Wrap the rest of file from XPS start with #ifndef __MINGW32__
        xps_block = s[idx:]

        stub = f'// XPS Print section removed for MinGW\nextern "C" {{\nvoid CleanupXpsPrint() {{}}\n}}'
        new_block = f'#ifndef __MINGW32__\n{xps_block}\n#else\n{stub}\n#endif\n'

        s = s[:idx] + new_block
        changed = True
        print("Patched windows.cc: wrapped XPS Print section (rest of file) with __MINGW32__ guard")

# Fix goto/jump scope issue: The cleanup label in GetProcessUserName has variable
# declarations after goto targets. MinGW g++ is stricter about this.
# Fix by wrapping the problematic section.
# Actually the error is about jumping past initialization of variables.
# We need to add { } scopes around variables declared mid-function.

# Fix 'min' macro not found: use std::min with explicit cast for type safety
if 'std::min<' not in s:
    import re
    # Pattern 1: nout = min(nin, n)  (in get_active_user, get_session_user_info)
    # -> std::min<uint32_t>(nin, n)
    s = re.sub(r'\bnout = std::min\(nin, n\)', 'nout = std::min<uint32_t>(nin, n)', s)
    changed = True
    print("Patched windows.cc: std::min -> std::min<uint32_t> for type safety")

if changed:
    p.write_text(s)
    print("Wrote windows.cc")
else:
    print("windows.cc: no changes needed")

# 5c. Patch windows_delete_test_cert.cc: case-sensitive include
p2 = pathlib.Path("/opt/rustdesk-src/src/platform/windows_delete_test_cert.cc")
s2 = p2.read_text()
if '#include <Windows.h>' in s2:
    s2 = s2.replace('#include <Windows.h>', '#include <windows.h>', 1)
    p2.write_text(s2)
    print("Patched windows_delete_test_cert.cc: Windows.h -> windows.h")
else:
    print("windows_delete_test_cert.cc: already patched")
PYEOF

# 5d. Extra safety: sed-based fix for std::min(nin, n) -> std::min<uint32_t>(nin, n)
# The python regex above should catch this, but as a fallback, use sed directly.
sed -i 's/std::min(nin, n)/std::min<uint32_t>(nin, n)/g' /opt/rustdesk-src/src/platform/windows.cc
grep -c 'std::min<uint32_t>' /opt/rustdesk-src/src/platform/windows.cc && echo "Extra fix applied: std::min<uint32_t>"

# 6. Fix build.rs: add cc::Build flags for MinGW cross-compilation (C++17, 
#    define UNICODE, silence fallthrough warnings)
python3 - <<'PYEOF'
import pathlib

p = pathlib.Path("/opt/rustdesk-src/build.rs")
s = p.read_text()

# Add cc::Build flags for the windows.cc compilation
# The original just does cc::Build::new().file(file).file(file2).compile("windows")
# We need to add:
#   .cpp(true)          - compile as C++
#   .std("c++17")       - C++17 standard
#   .flag("-Wno-implicit-fallthrough")  - suppress fallthrough warnings
#   (UNICODE is already defined)
old_compile = '    cc::Build::new().file(file).file(file2).compile("windows");'
new_compile = '''    cc::Build::new()
        .file(file)
        .file(file2)
        .cpp(true)
        .std("c++17")
        .flag("-Wno-implicit-fallthrough")
        .flag("-Wno-unused-function")
        .compile("windows");'''

if old_compile in s:
    s = s.replace(old_compile, new_compile, 1)
    p.write_text(s)
    print("Patched build.rs: added cc::Build flags for MinGW C++ compilation")
else:
    print("WARNING: could not find cc::Build compile line in build.rs")
PYEOF