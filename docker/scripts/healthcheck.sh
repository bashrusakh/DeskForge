#!/bin/sh
# DeskForge container healthcheck.
#
# Verifies each supervised longrun service (hbbr, hbbs, api) is reported as
# both `up` AND `ready` by s6-svstat. `key-secret` is a oneshot and has no
# servicedir, so it's intentionally not polled here.
#
# `s6-svstat -u <dir>` prints `true` (exit 0) when the service is up and
# ready; prints `false` (exit 1) otherwise. We require the literal output
# `true` so a `down` state (process crashed, supervised restart pending)
# cannot pass the healthcheck.

set -eu

check_service() {
    svc="$1"
    dir="/run/s6-rc/servicedirs/$svc"
    if ! /package/admin/s6/command/s6-svstat -u "$dir" | grep -qx true; then
        echo "healthcheck: $svc is not up/ready" >&2
        /package/admin/s6/command/s6-svstat "$dir" >&2 || true
        exit 1
    fi
}

for svc in hbbr hbbs api; do
    check_service "$svc"
done

exit 0
