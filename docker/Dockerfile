# DeskForge - Unified RustDesk-compatible server + API
# Multi-stage build: Rust server + Go API + Web Admin + s6-overlay

# Stage 1: Build Rust server (hbbs, hbbr) with musl for Alpine compatibility
FROM rust:bookworm AS rust-builder

RUN apt-get update && apt-get install -y musl-tools sqlite3 perl && rm -rf /var/lib/apt/lists/*

WORKDIR /build/server

# Copy Cargo files and delete lock to regenerate with compatible versions
COPY server/Cargo.toml ./
COPY server/build.rs ./
COPY libs/hbb_common ./libs/hbb_common

# Create dummy src files for dependency caching
RUN mkdir -p src && \
    echo 'fn main() {}' > src/main.rs && \
    echo 'fn main() {}' > src/hbbr.rs

# Build dependencies only (musl target)
RUN rustup target add x86_64-unknown-linux-musl && \
    cargo build --release --target x86_64-unknown-linux-musl 2>/dev/null || true

# Create dummy SQLite DB for sqlx compile-time query checks
RUN rm -f /tmp/dummy.db && \
    sqlite3 /tmp/dummy.db "CREATE TABLE IF NOT EXISTS peer (guid blob primary key not null, id varchar(100) not null, uuid blob not null, pk blob not null, created_at datetime not null default(current_timestamp), user blob, status tinyint, note varchar(300), info text not null);"

# Copy actual source
COPY server/src ./src

# Build the server binaries with musl target (including rustdesk-utils for key generation)
ENV DATABASE_URL=sqlite:///tmp/dummy.db
RUN rm -f Cargo.lock && cargo build --release --target x86_64-unknown-linux-musl --bin hbbs --bin hbbr --bin rustdesk-utils

# Stage 2: Build Go API
FROM golang:1.23-alpine AS go-builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build/api

# Copy source and generate go.sum
COPY api ./
RUN go mod tidy

# Download dependencies
RUN go mod download

# Build the API binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags "-s -w" -o apimain ./cmd/apimain.go

# Stage 3: Web Admin (Vue 3 + Element Plus) — build from source inside Docker
FROM node:20-bookworm AS admin-ui-builder
WORKDIR /build/admin-ui
COPY admin-ui/package*.json ./
RUN npm ci
COPY admin-ui/ ./
RUN npm run build

# Stage 4: Install s6-overlay from official GitHub release (no upstream dependency)
FROM alpine:latest AS s6-stage

ARG S6_OVERLAY_VERSION=3.2.3.0
# s6-overlay arch differs from Docker TARGETARCH naming (x86_64 vs amd64)
# Map TARGETARCH to s6-overlay arch: amd64->x86_64, arm64->aarch64
ARG TARGETARCH=amd64

# SHA256 checksums for s6-overlay v3.2.3.0 (from just-containers/s6-overlay releases)
# noarch is platform-independent
ARG S6_NOARCH_SHA256=b720f9d9340efc8bb07528b9743813c836e4b02f8693d90241f047998b4c53cf
# x86_64 checksum
ARG S6_X86_64_SHA256=a93f02882c6ed46b21e7adb5c0add86154f01236c93cd82c7d682722e8840563
# aarch64 checksum (for arm64 builds)
ARG S6_AARCH64_SHA256=0952056ff913482163cc30e35b2e944b507ba1025d78f5becbb89367bf344581

# Download and extract s6-overlay from just-containers/s6-overlay with checksum verification
RUN case "${TARGETARCH}" in \
    amd64) S6_OVERLAY_ARCH=x86_64 ; S6_ARCH_SHA256="${S6_X86_64_SHA256}" ;; \
    arm64) S6_OVERLAY_ARCH=aarch64 ; S6_ARCH_SHA256="${S6_AARCH64_SHA256}" ;; \
    *) echo "Unsupported TARGETARCH: ${TARGETARCH}" && exit 1 ;; \
  esac && \
  apk add --no-cache curl xz && \
  curl -fsSL -o /tmp/s6-overlay-noarch.tar.xz https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz && \
  echo "${S6_NOARCH_SHA256}  /tmp/s6-overlay-noarch.tar.xz" | sha256sum -c - && \
  curl -fsSL -o /tmp/s6-overlay-${S6_OVERLAY_ARCH}.tar.xz https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-${S6_OVERLAY_ARCH}.tar.xz && \
  echo "${S6_ARCH_SHA256}  /tmp/s6-overlay-${S6_OVERLAY_ARCH}.tar.xz" | sha256sum -c - && \
  tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz && \
  tar -C / -Jxpf /tmp/s6-overlay-${S6_OVERLAY_ARCH}.tar.xz && \
  rm -f /tmp/s6-overlay-*.tar.xz

# Stage 5: Final image
FROM alpine

ARG BUILDARCH
ARG VERSION=dev

WORKDIR /app

# Install dependencies
RUN apk add --no-cache tzdata curl

# Copy Rust server binaries (musl target)
COPY --from=rust-builder /build/server/target/x86_64-unknown-linux-musl/release/hbbs /usr/bin/hbbs
COPY --from=rust-builder /build/server/target/x86_64-unknown-linux-musl/release/hbbr /usr/bin/hbbr
COPY --from=rust-builder /build/server/target/x86_64-unknown-linux-musl/release/rustdesk-utils /usr/bin/rustdesk-utils

# Copy Go API binary
COPY --from=go-builder /build/api/apimain /app/apimain

# Copy API resources first (templates, i18n, public, version)
COPY api/resources /app/resources

# Copy Web Admin files LAST so they always overwrite any stale admin files from api/resources
COPY --from=admin-ui-builder /build/admin-ui/dist /app/resources/admin/


# Copy s6-overlay from GitHub release (no upstream docker.io/rustdesk)
COPY --from=s6-stage /init /init
COPY --from=s6-stage /etc/s6-overlay /etc/s6-overlay
COPY --from=s6-stage /package /package
COPY --from=s6-stage /command /command

# Create our own healthcheck.sh (replaces upstream version)
# NOTE: must list ALL s6 longrun services (hbbr, hbbs, api) - update if services change
RUN cat > /usr/bin/healthcheck.sh <<'EOF' && chmod +x /usr/bin/healthcheck.sh
#!/bin/sh
/package/admin/s6/command/s6-svstat /run/s6-rc/servicedirs/hbbr || exit 1
/package/admin/s6/command/s6-svstat /run/s6-rc/servicedirs/hbbs || exit 1
/package/admin/s6/command/s6-svstat /run/s6-rc/servicedirs/api || exit 1
EOF

# Copy API configuration
COPY api/conf /app/conf

# Create s6 service for API
RUN mkdir -p /etc/s6-overlay/s6-rc.d/api && \
    echo -e "key-secret\nhbbs" > /etc/s6-overlay/s6-rc.d/api/dependencies && \
    echo "longrun" > /etc/s6-overlay/s6-rc.d/api/type && \
    echo "#!/command/with-contenv sh" > /etc/s6-overlay/s6-rc.d/api/run && \
    echo "cd /app" >> /etc/s6-overlay/s6-rc.d/api/run && \
    echo "./apimain" >> /etc/s6-overlay/s6-rc.d/api/run && \
    touch /etc/s6-overlay/s6-rc.d/user/contents.d/api && \
    ln -s /run /var/run

# Create data directories
RUN mkdir -p /data /app/data /app/runtime/log

# Set environment variables
ENV RELAY=relay.example.com
ENV ENCRYPTED_ONLY=0
ENV MUST_LOGIN=N
ENV TZ=UTC

# Expose all necessary ports
# 21114: API server
# 21115: NAT type test
# 21116: ID server (TCP/UDP)
# 21117: Relay server
# 21118: WebSocket
EXPOSE 21114 21115 21116 21116/udp 21117 21118

# Volumes for persistent data
VOLUME /data
VOLUME /app/data

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD /usr/bin/healthcheck.sh

# Entry point
ENTRYPOINT ["/init"]
