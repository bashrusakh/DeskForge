#!/bin/sh
# DeskForge key-secret service: generate the RustDesk Ed25519 keypair on first boot.
#
# Invoked by s6-rc as a oneshot (see docker/Dockerfile, /etc/s6-overlay/s6-rc.d/key-secret/up).
# Idempotent: skipped when /data already contains both non-empty key files.

set -eu
umask 077

# Distinguish three /data states explicitly:
#   both missing  → generate a fresh keypair (first boot, or admin wiped /data)
#   both present  → idempotent no-op
#   partial       → refuse to silently rotate the server identity; require
#                   admin to repair /data. Exits 1 so s6 stops the container.
priv_missing=0
pub_missing=0
[ -s /data/id_ed25519 ]     || priv_missing=1
[ -s /data/id_ed25519.pub ] || pub_missing=1

if [ "$priv_missing" -ne "$pub_missing" ]; then
    if [ "$priv_missing" -eq 1 ]; then
        echo "key-secret: partial keypair in /data: /data/id_ed25519.pub exists but /data/id_ed25519 is missing; refusing to rotate server identity" >&2
    else
        echo "key-secret: partial keypair in /data: /data/id_ed25519 exists but /data/id_ed25519.pub is missing; refusing to rotate server identity" >&2
    fi
    echo "key-secret: repair by providing both /data/id_ed25519 and /data/id_ed25519.pub, or remove both to regenerate" >&2
    exit 1
fi

if [ "$priv_missing" -eq 1 ] && [ "$pub_missing" -eq 1 ]; then
    tmpdir="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '$tmpdir'" EXIT

    /usr/bin/rustdesk-utils genkeypair > "$tmpdir/keygen.out" 2>&1

    awk '/Public Key:/  {print $3}' "$tmpdir/keygen.out" > "$tmpdir/id_ed25519.pub"
    awk '/Secret Key:/  {print $3}' "$tmpdir/keygen.out" > "$tmpdir/id_ed25519"

    # Fail closed: don't write empty/short key files into /data.
    if [ ! -s "$tmpdir/id_ed25519" ] || [ ! -s "$tmpdir/id_ed25519.pub" ]; then
        echo "key-secret: rustdesk-utils produced empty key material, refusing to write /data" >&2
        exit 1
    fi

    cp "$tmpdir/id_ed25519"     /data/id_ed25519
    cp "$tmpdir/id_ed25519.pub" /data/id_ed25519.pub
    chmod 600 /data/id_ed25519
    chmod 644 /data/id_ed25519.pub
fi
