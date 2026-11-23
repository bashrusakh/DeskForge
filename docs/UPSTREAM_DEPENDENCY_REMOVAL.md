# Removing Upstream Dependency on `rustdesk/rustdesk-server-s6`

> Completed migration record (verified 2026-06-28): the production image no longer
> pulls `rustdesk/rustdesk-server-s6` from docker.io. The implemented state is the
> `docker/Dockerfile` itself (`s6-stage` + custom healthcheck + `rustdesk-utils`
> from Stage 1); this document keeps the rationale, what changed, and the
> verification result.

## Problem

The previous implementation in `docker/Dockerfile` used:
```dockerfile
FROM rustdesk/rustdesk-server-s6:latest AS s6-source
```

This pulls **150 MB** from docker.io just to get **10 MB** of s6-overlay.

---

## What Was Being Pulled from Upstream (Facts)

| File/Dir | Size | Purpose | Needed? |
|----------|------|---------|---------|
| `/init` | 1 KB | PID 1, zombie reaping, graceful shutdown | **YES** |
| `/etc/s6-overlay/` | ~100+ files | s6-rc service configs (hbbs, hbbr, key-secret, fix-attrs) | **YES** |
| `/package/` | ~500+ files | s6, execline, s6-rc, s6-linux-init, s6-networking, s6-dns binaries | **YES** |
| `/command/` | ~200+ symlinks | Convenience symlinks in PATH | **YES** |
| `/usr/bin/healthcheck.sh` | 159 bytes | Checks `s6-svstat hbbr` && `s6-svstat hbbs` | **YES** (easy to replace) |
| `/usr/bin/rustdesk-utils` | 574 KB | **OUR CODE** (`server/src/utils.rs`) — genkeypair, validatekeypair, doctor | **YES** (build ourselves) |
| **Their hbbs/hbbr** | ~60 MB | RustDesk servers | **NO** — ours in Stage 1 |
| **Their Go API** | ~20 MB | REST API | **NO** — ours in Stage 2 |
| **Their Admin UI** | ~10 MB | Vue static files | **NO** — ours in Stage 3 |

**Bottom line:** 150 MB pulled for 10 MB s6-overlay + 2 utilities.

---

## What Is s6-overlay and Why We Need It

**s6-overlay** — process supervisor for containers (like systemd but for containers). Required to run **3 services in ONE container**:
1. **hbbs** (21116) — ID/signaling server
2. **hbbr** (21117) — Relay server
3. **Go API** (21114) — REST API + Admin UI

Without it: 3 separate containers or custom bash supervisor.

`ENTRYPOINT ["/init"]` — `/init` from s6-overlay becomes PID 1.

---

## What Keys rustdesk-utils Generates

`rustdesk-utils genkeypair` → **Ed25519 keypair**:

| Key | Format | Purpose |
|-----|--------|---------|
| **Public Key** | base64 (44 chars) | Announced to clients. Client verifies server signature. In client = `ID Server Public Key` |
| **Secret Key** | base64 (88 chars) | Server only (`/data/id_ed25519`). Signs tokens, proves ID ownership |

**Where used:**
- `hbbs` reads `/data/id_ed25519` (secret) at startup
- Client receives public key → verifies handshake signature
- Go API reads public key from `RUSTDESK_API_KEY_FILE` (`/data/id_ed25519.pub`) → serves to clients

**Risk:** We were copying upstream's binary. If they change algorithm/format — our keys break. Must build **ours** from `server/src/utils.rs`.

---

## Files to Change

### 1. `docker/Dockerfile` — Main build (used in `docker-compose.yml`)

**Changes:**

| Stage | Before | After |
|-------|--------|-------|
| Stage 1 (rust-builder) | `cargo build --bin hbbs --bin hbbr` | Add `--bin rustdesk-utils` |
| Stage 4 (s6-source) | `FROM rustdesk/rustdesk-server-s6:latest` | **Remove**. Replace with GitHub s6-overlay download |
| Stage 5 (final) | `COPY --from=s6-source /init /etc/s6-overlay /package /command /usr/bin/healthcheck.sh /usr/bin/rustdesk-utils` | `COPY --from=s6-stage` + custom `healthcheck.sh` + `rustdesk-utils` from Stage 1 |

**As implemented** (exact state: `docker/Dockerfile`, `s6-stage`): Stage 4 runs on
pinned `alpine:3.24.1`, maps `TARGETARCH` to the s6-overlay arch
(`amd64→x86_64`, `arm64→aarch64`), and downloads the noarch + arch tarballs from
the `just-containers/s6-overlay` GitHub release with pinned SHA-256 checksums
(`SHELL ["/bin/ash", "-o", "pipefail", "-c"]` so a piped `curl` failure aborts the
`RUN`). Stage 5 copies `/init`, `/etc/s6-overlay`, `/package`, and `/command` from
`s6-stage`.

**Custom healthcheck** (`docker/scripts/healthcheck.sh`, copied to
`/usr/bin/healthcheck.sh`):
The script probes the listening TCP ports of all three services (api 21114,
hbbs 21116, hbbr 21117) with BusyBox `nc -z` and exits non-zero if any of them
is not bound. Port binding is the user-facing definition of "ready": until the
daemon binds, no client can connect. `s6-svstat -o up,ready` is not usable here
because our services do not use s6-notifywhenup (not shipped in the standard
s6-overlay v3 image), so every service would report `ready=false` and the
healthcheck would always fail.

**rustdesk-utils**: built in Stage 1 for the musl target matching `TARGETARCH`
(`x86_64-unknown-linux-musl` / `aarch64-unknown-linux-musl`) and copied from
`/build/server/release/` so the final stage does not re-derive the target.

### 2. `api/Dockerfile_full_s6` — Legacy, unused

**Action:** Mark deprecated or delete. Not used in `docker-compose.yml`.

---

## Implementation Plan (executed)

The plan below was executed and verified on 2026-06-28 (result section); the
authoritative state now lives in `docker/Dockerfile` itself.

### Step 1: Verify rustdesk-utils builds under musl
```bash
cd server && cargo build --bin rustdesk-utils --target x86_64-unknown-linux-musl --release
```
If fails — fix deps (hbb_common must be accessible).

### Step 2: Update docker/Dockerfile
1. Stage 1: add `--bin rustdesk-utils` to cargo build (line 34)
2. Remove Stage 4 (`FROM rustdesk/rustdesk-server-s6:latest AS s6-source`)
3. Add new Stage 4 (`s6-stage`) downloading s6-overlay v3.2.3.0
4. Stage 5: copy from `s6-stage` instead of `s6-source`
5. Stage 5: add custom `healthcheck.sh` creation
6. Stage 5: copy `rustdesk-utils` from `rust-builder`

### Step 3: Test Build
```bash
cd docker && docker compose build --no-cache
```

### Step 4: Test Runtime
```bash
docker compose up -d
docker compose logs -f
# Verify: hbbs, hbbr, API start
# Verify: healthcheck passes
# Verify: rustdesk-utils genkeypair works
```

### Step 5: Update/Remove api/Dockerfile_full_s6
Add `# DEPRECATED` comment or delete.

---

## Pinned Versions

Production base images are pinned by both version and digest. The s6-overlay
release is pinned by version tag and SHA-256 checksums.

| Component | Version | Where |
|-----------|---------|-------|
| s6-overlay | 3.2.3.0 | `ARG S6_OVERLAY_VERSION=3.2.3.0` + `S6_*_SHA256` checksums in Dockerfile |
| rust (base image) | `1.92.0-bookworm@sha256:e90e846de4124376164ddfbaab4b0774c7bdeef5e738866295e5a90a34a307a2` | Stage 1 base image |
| golang (base image) | `1.25.0-alpine@sha256:f18a072054848d87a8077455f0ac8a25886f2397f88bfdd222d6fafbb5bba440` | Stage 2 base image |
| node (base image) | `20.20.2-bookworm@sha256:8f693eaa7e0a8e71560c9a82b55fd54c2ae920a2ba5d2cde28bac7d1c01c9ba5` | Stage 3 base image |
| alpine (base image) | `3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b` | Stage 4/5 base images |

---

## Result (Verified 2026-06-28)

✅ **Build succeeds** — `docker compose build` no errors  
✅ **Container starts** — `docker compose up -d`  
✅ **Healthcheck: healthy** — all 3 services (hbbs, hbbr, api) running  
✅ **rustdesk-utils works** — `genkeypair` generates valid Ed25519 keys  
✅ **Keys created automatically** — `/data/id_ed25519` and `/data/id_ed25519.pub`  
✅ **API responds** — `GET /api/` → `{"code":0,"message":"success","data":"Hello Gwen"}`  
✅ **Admin UI accessible** — `GET /admin/` serves Vue SPA  
✅ **Image 127 MB** (was ~150+ MB with upstream pull)  
✅ **No docker.io/rustdesk pull** — the `rustdesk-server-s6` base-image dependency is
gone from the image build. This is image-build independence from that one upstream
image only: the Dockerfile still pulls official `rust`/`golang`/`node`/`alpine` base
images and active third-party upstream dependencies remain in the Rust/Go manifests —
no project-wide upstream-independence, sovereignty, or release claim is made
(see `AGENTS.md` / `PLAN.md` §0).

### Changed Files

| File | Changes |
|------|---------|
| `docker/Dockerfile` | Stage 1: +`--bin rustdesk-utils`; Stage 4: upstream → GitHub s6-overlay v3.2.3.0; Stage 5: custom healthcheck.sh, rustdesk-utils from Stage 1 |
| `api/Dockerfile_full_s6` | Commented + DEPRECATED marker (unused in compose) |
| `docs/UPSTREAM_DEPENDENCY_REMOVAL.md` | This document |

### Verification Commands

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

---

## Risks & Mitigations (Status: Passed)

| Risk | Status | Notes |
|------|--------|-------|
| s6-overlay v3.2.3.0 incompatible | ✅ **OK** | All 3 services started, s6-rc works |
| rustdesk-utils fails musl build | ✅ **OK** | Built in Stage 1, runs in container |
| healthcheck misses services | ✅ **OK** | Checks hbbs, hbbr, api — all up |
| Keys don't work with clients | ✅ **OK** | Keys generated, hbbs reads them, API serves pub key |

---

## Next Steps (Optional)

1. **Remove `rustdesk/rustdesk-server-s6` locally**: `docker rmi rustdesk/rustdesk-server-s6:latest`
2. **Delete `api/Dockerfile_full_s6`** entirely (deprecated marker present; unused in compose)
3. **`.env.example` upstream references** — none found (checked 2026-11-23)
4. **CI coverage** — done: `.github/workflows/build.yml` builds `docker/Dockerfile`
