#!/bin/sh
# DeskForge api longrun: Go REST API.
#
# Depends on hbbs being up (because the API reads the ID server's public key
# from /data/id_ed25519.pub via RUSTDESK_API_KEY_FILE).
set -eu
cd /app
exec ./apimain
