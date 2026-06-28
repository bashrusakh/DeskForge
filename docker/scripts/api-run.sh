#!/bin/sh
# DeskForge api longrun: Go REST API.
#
# Depends on hbbs being up (because the API reads the ID server's public key
# from /data/id_ed25519.pub via RUSTDESK_API_KEY_FILE).
#
# s6-overlay v3 stores operator -e vars in /run/s6/container_environment/
# (filename = name, content = value), but s6-svscan does NOT auto-load this
# dir into supervised-process environ. The Go API uses Viper AutomaticEnv()
# to read RUSTDESK_API_* from os.Environ(), so without s6-envdir the operator
# -e vars are silently ignored (config falls back to api/conf/config.yaml
# defaults, e.g. a random admin password instead of the operator-provided
# RUSTDESK_API_JWT_KEY).
#
# Wrap the exec with s6-envdir so the env dir is loaded into apimain's
# environment, matching the pattern used by hbbs-run.sh.
set -eu
cd /app
exec /command/s6-envdir /run/s6/container_environment ./apimain