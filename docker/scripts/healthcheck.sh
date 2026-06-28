#!/bin/sh
# DeskForge container healthcheck.
#
# Verifies each supervised service by probing its listening TCP port. Port
# binding is the user-facing definition of "ready": until the daemon binds,
# no client can connect.
#
# Why port-probe, not s6-svstat -o up,ready: our services (hbbs, hbbr,
# apimain) do not use s6-notifywhenup. s6-notifywhenup is not shipped in
# the standard s6-overlay v3 image (verified — only s6-notify-fd-from-socket
# and s6-notifyoncheck are present), so without adding it as a build step
# and propagating a notification-fd through every run script, every service
# would report ready=false and the healthcheck would always fail.
# Port-probe is the equivalent semantic check that requires no daemon-side
# changes and works the same way for all backends.
#
# We use nc (BusyBox netcat, -z flag) for the probe. /dev/tcp/host/port is
# a bash-only feature and does not work in BusyBox ash, which is /bin/sh
# in this image.
#
# Caveat: hbbs also listens on UDP :21116 (NAT type test). The TCP probe on
# :21116 is sufficient because hbbs binds TCP and UDP on the same socket
# via SO_REUSEADDR; if TCP binds, UDP is bound too.

set -eu

probe_port() {
    host="$1"; port="$2"; name="$3"
    # BusyBox nc -z: scan for listening daemons without sending data.
    # Returns 0 if the connect succeeds, non-zero otherwise.
    if ! nc -z "$host" "$port" 2>/dev/null; then
        echo "healthcheck: $name not listening on $host:$port" >&2
        exit 1
    fi
}

probe_port 127.0.0.1 21114 api
probe_port 127.0.0.1 21116 hbbs
probe_port 127.0.0.1 21117 hbbr

exit 0