# Removing Upstream Dependency on `rustdesk/rustdesk-server-s6`

> Completed migration record (verified 2026-06-28): the production image no longer pulls
> `rustdesk/rustdesk-server-s6` from docker.io. The implemented state is the `docker/Dockerfile`
> itself (`s6-stage` + custom healthcheck + `rustdesk-utils` from Stage 1). This document keeps the
> problem, why-s6 rationale, what changed, pinned versions, and verification commands; the executed
> plan, verification result, changed-files list, and risk pass are archived in
> [CHANGELOG.md](../CHANGELOG.md) ("Upstream s6 dependency removal — archived from this file, 2026-11-24").

## Problem

The previous `docker/Dockerfile` used `FROM rustdesk/rustdesk-server-s6:latest AS s6-source` — this pulls **150 MB** from docker.io just to get **10 MB** of s6-overlay + 2 utilities. Their bundled hbbs/hbbr (~60 MB), Go API (~20 MB), and Admin UI (~10 MB) are superseded by our own Stage 1/2/3 builds; only `/init`, `/etc/s6-overlay/`, `/package/`, `/command/`, `/usr/bin/healthcheck.sh`, and `/usr/bin/rustdesk-utils` (574 KB — **our code**, `server/src/utils.rs`) were needed.

## What Is s6-overlay and Why We Need It

**s6-overlay** — process supervisor for containers (like systemd but for containers). Required to run **3 services in ONE container**: **hbbs** (21116, ID/signaling), **hbbr** (21117, relay), and the **Go API** (21114, REST API + Admin UI). Without it: 3 separate containers or a custom bash supervisor. `ENTRYPOINT ["/init"]` — `/init` from s6-overlay becomes PID 1.

`rustdesk-utils genkeypair` generates an **Ed25519 keypair**: a base64 public key (44 chars) announced to clients (the client's `ID Server Public Key`, verifying the server signature) and a base64 secret key (88 chars) at `/data/id_ed25519`, read by `hbbs` at startup to sign tokens and prove ID ownership; `rustdesk-utils` also provides `validatekeypair` and `doctor`. The Go API reads the public key from `RUSTDESK_API_KEY_FILE` (`/data/id_ed25519.pub`) and serves it to clients. We build **ours** from `server/src/utils.rs` — copying upstream's binary means an upstream algorithm/format change breaks our keys.

## What Changed

| Stage | Before | After |
|-------|--------|-------|
| Stage 1 (rust-builder) | `cargo build --bin hbbs --bin hbbr` | Add `--bin rustdesk-utils` |
| Stage 4 (s6-source) | `FROM rustdesk/rustdesk-server-s6:latest` | **Remove**. Replace with GitHub s6-overlay download |
| Stage 5 (final) | `COPY --from=s6-source /init /etc/s6-overlay /package /command /usr/bin/healthcheck.sh /usr/bin/rustdesk-utils` | `COPY --from=s6-stage` + custom `healthcheck.sh` + `rustdesk-utils` from Stage 1 |

**As implemented** (exact state as of 2026-06-28: `docker/Dockerfile`, `s6-stage`): Stage 4 runs on pinned `alpine:3.24.1`, maps `TARGETARCH` to the s6-overlay arch (`amd64→x86_64`, `arm64→aarch64`), and downloads the noarch + arch tarballs from the `just-containers/s6-overlay` GitHub release with pinned SHA-256 checksums (`SHELL ["/bin/ash", "-o", "pipefail", "-c"]` so a piped `curl` failure aborts the `RUN`); Stage 5 copies `/init`, `/etc/s6-overlay`, `/package`, `/command` from `s6-stage`. `rustdesk-utils` is built in Stage 1 for the musl target matching `TARGETARCH` and copied from `/build/server/release/`. `api/Dockerfile_full_s6` is legacy and unused (deprecated marker).

**Custom healthcheck** (`docker/scripts/healthcheck.sh`, copied to `/usr/bin/healthcheck.sh`): probes the listening TCP ports of all three services (api 21114, hbbs 21116, hbbr 21117) with BusyBox `nc -z`, exiting non-zero if any is not bound — port binding is the user-facing definition of "ready": until the daemon binds, no client can connect. `s6-svstat -o up,ready` is not usable because our services do not use s6-notifywhenup (not shipped in the standard s6-overlay v3 image), so every service would report `ready=false` and the healthcheck would always fail.

## Pinned Versions

Production base images are pinned by both version and digest; the s6-overlay release is pinned by version tag and SHA-256 checksums. The table below is the 2026-06-28 snapshot, not a re-verified current state.

| Component | Version | Where |
|-----------|---------|-------|
| s6-overlay | 3.2.3.0 | `ARG S6_OVERLAY_VERSION=3.2.3.0` + `S6_*_SHA256` checksums in Dockerfile |
| rust (base image) | `1.92.0-bookworm@sha256:e90e846de4124376164ddfbaab4b0774c7bdeef5e738866295e5a90a34a307a2` | Stage 1 base image |
| golang (base image) | `1.25.0-alpine@sha256:f18a072054848d87a8077455f0ac8a25886f2397f88bfdd222d6fafbb5bba440` | Stage 2 base image |
| node (base image) | `20.20.2-bookworm@sha256:8f693eaa7e0a8e71560c9a82b55fd54c2ae920a2ba5d2cde28bac7d1c01c9ba5` | Stage 3 base image |
| alpine (base image) | `3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b` | Stage 4/5 base images |

## Verification Commands

```bash
# Build
cd docker && docker compose build --no-cache

# Run
docker compose up -d

# Health check
docker compose ps
docker inspect <container> --format='{{.State.Health.Status}}'

# Test utilities
docker exec <container> /usr/bin/rustdesk-utils genkeypair
docker exec <container> /usr/bin/healthcheck.sh

# Test API
curl http://localhost:21114/api/
curl http://localhost:21114/admin/

# Logs
docker compose logs -f
```
