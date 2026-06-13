# full_Server - Unified RustDesk Server + API
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

# Build the server binaries with musl target
ENV DATABASE_URL=sqlite:///tmp/dummy.db
RUN rm -f Cargo.lock && cargo build --release --target x86_64-unknown-linux-musl --bin hbbs --bin hbbr

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

# Stage 3: Web Admin (Vue 3 + Element Plus) — pre-built on host
# We copy the pre-built dist/ directly to avoid npm bugs in Alpine/musl.
# To rebuild: cd admin-ui && rm -rf node_modules package-lock.json && npm install && npm run build
FROM scratch AS web-dist
COPY admin-ui/dist /dist

# Stage 4: Get s6-overlay from official image
FROM rustdesk/rustdesk-server-s6:latest AS s6-source

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

# Copy Go API binary
COPY --from=go-builder /build/api/apimain /app/apimain

# Copy API resources first (templates, i18n, public, version)
COPY api/resources /app/resources

# Copy Web Admin files LAST so they always overwrite any stale admin files from api/resources
COPY --from=web-dist /dist /app/resources/admin/

# Create empty web2 dir (Gin serves it when web-client=1, prevents crash)
RUN mkdir -p /app/resources/web2

# Copy s6-overlay from official image
COPY --from=s6-source /init /init
COPY --from=s6-source /etc/s6-overlay /etc/s6-overlay
COPY --from=s6-source /package /package
COPY --from=s6-source /command /command
COPY --from=s6-source /usr/bin/healthcheck.sh /usr/bin/healthcheck.sh
COPY --from=s6-source /usr/bin/rustdesk-utils /usr/bin/rustdesk-utils

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
    echo "/package/admin/s6/command/s6-svstat /run/s6-rc/servicedirs/api || exit 1" >> /usr/bin/healthcheck.sh && \
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
