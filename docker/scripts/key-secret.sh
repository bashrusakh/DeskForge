#!/bin/sh
# DeskForge key-secret service: generate the RustDesk Ed25519 keypair on first boot.
#
# Invoked by s6-rc as a oneshot (see docker/Dockerfile, /etc/s6-overlay/s6-rc.d/key-secret/up).
# Idempotent: skipped when /data already contains non-empty key files.

set -eu
umask 077

# Generate only when at least one key file is missing or empty (-s = size > 0).
if [ ! -s /data/id_ed25519 ] || [ ! -s /data/id_ed25519.pub ]; then
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
