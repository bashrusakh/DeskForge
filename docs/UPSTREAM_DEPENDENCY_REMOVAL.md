# Removing Upstream Dependency on `rustdesk/rustdesk-server-s6`

## Problem

In `docker/Dockerfile:62`:
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
| **Public Key** | base64 (43 chars) | Announced to clients. Client verifies server signature. In client = `ID Server Public Key` |
| **Secret Key** | base64 (86 chars) | Server only (`/data/id_ed25519`). Signs tokens, proves ID ownership |

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
| Stage 5 (final) | `COPY --from=s6-source /init /etc/s6-overlay /package /command /usr/bin/healthcheck.sh /usr/bin/rustdesk-utils` | `COPY --from=s6-stage /init /etc/s6-overlay /package /command` + custom `healthcheck.sh` + `rustdesk-utils` from Stage 1 |

**New Stage 4 (s6-stage):**
```dockerfile
# Stage 4: Install s6-overlay from official GitHub release
FROM alpine:latest AS s6-stage

ARG S6_OVERLAY_VERSION=3.2.3.0
ARG S6_OVERLAY_ARCH=x86_64

# Download and extract s6-overlay
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp/
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-${S6_OVERLAY_ARCH}.tar.xz /tmp/
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz && \
    tar -C / -Jxpf /tmp/s6-overlay-${S6_OVERLAY_ARCH}.tar.xz && \
    rm -f /tmp/s6-overlay-*.tar.xz
```

**Custom healthcheck.sh (in Stage 5 RUN):**
```bash
cat > /usr/bin/healthcheck.sh <<'EOF' && chmod +x /usr/bin/healthcheck.sh
#!/bin/sh
/package/admin/s6/command/s6-svstat /run/s6-rc/servicedirs/hbbr || exit 1
/package/admin/s6/command/s6-svstat /run/s6-rc/servicedirs/hbbs || exit 1
/package/admin/s6/command/s6-svstat /run/s6-rc/servicedirs/api || exit 1
EOF
```

**Copy rustdesk-utils (in Stage 5):**
```dockerfile
COPY --from=rust-builder /build/server/target/x86_64-unknown-linux-musl/release/rustdesk-utils /usr/bin/rustdesk-utils
```

### 2. `api/Dockerfile_full_s6` — Legacy, unused

**Action:** Mark deprecated or delete. Not used in `docker-compose.yml`.

---

## Implementation Plan

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

| Component | Version | Where |
|-----------|---------|-------|
| s6-overlay | 3.2.3.0 | `ARG S6_OVERLAY_VERSION=3.2.3.0` in Dockerfile |
| rust | bookworm | Stage 1 base image |
| alpine | latest | Stage 4/5 base images |

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
✅ **No docker.io/rustdesk pull** — full upstream independence  

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
2. **Delete `api/Dockerfile_full_s6`** entirely (currently commented)
3. **Update `.env.example`** — remove any upstream references
4. **Add to CI** — verify build doesn't pull upstream