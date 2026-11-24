# Deployment & environment

Operational deep reference for running DeskForge. The quick summary (commands, ports,
key variables) lives in the [README](../README.md).

## Quick start details

For a fresh checkout, create the repository-root `.env` from `.env.example` before starting Compose (preserve any existing `.env`).
Set the optional `RUSTDESK_API_RUSTDESK_PUBLIC_*` values there to prefill the Custom Client Builder with client-reachable addresses.

When Compose is run from `docker/`, the combined service loads root `.env` through `env_file: ../.env`.
These values prefill builder fields; they do not replace the operational server addresses.
`RUSTDESK_API_RUSTDESK_PUBLIC_API_SERVER` is also the branding-asset origin for the build workflow.

Full semantics and validation rules: [`RUSTDESK_API_RUSTDESK_PUBLIC_*` below](#rustdesk_api_rustdesk_public_).

Admin login, RustDesk client settings, and the public-key command are in the
[README quick start](../README.md).

## Upgrading an existing deployment

The Compose file bind-mounts `../data/uploads` to `/app/resources/public/upload`.
On an existing deployment, the mount shadows the uploads already stored inside the old container.
Branding assets referenced before the mount (`GET /upload/...`) return 404.
Copy them out of the old container before the first `docker compose up -d` with the new mount:

```bash
mkdir -p ../data/uploads
docker cp <old-rustdesk-container>:/app/resources/public/upload/. ../data/uploads/
docker compose up -d
```

This migration applies only to the upload bind mount; other data (`/data`, the
database) stays on its existing volumes.

## Environment variables in depth

The key-variable table lives in the [README](../README.md) ("Environment" section).
The semantics below apply on top of it.

### `SECRET_ENCRYPTION_KEY`

New non-empty secret writes and secret-bearing Custom Builder operations require this
key and are rejected rather than stored as plaintext when it is missing. Legacy plaintext
rows remain readable; saving them again encrypts them when the key exists.

### `RUSTDESK_API_RUSTDESK_PUBLIC_*`

The `RUSTDESK_API_RUSTDESK_PUBLIC_*` variables are optional external/public addresses.
They prefill the Custom Client Builder's Host / Relay / API server fields.
They are needed behind Docker/NAT, where the server cannot discover its own external address.

They are intentionally distinct from `RUSTDESK_API_RUSTDESK_ID_SERVER` / `RUSTDESK_API_RUSTDESK_RELAY_SERVER` / `RUSTDESK_API_RUSTDESK_API_SERVER`.
Those operational values may legitimately hold internal or Docker addresses consumed by the dashboard and server commands.

`RUSTDESK_API_RUSTDESK_PUBLIC_API_SERVER` additionally serves as the public origin that resolves a server-relative branding asset (`/upload/...`) into an absolute URL for the build workflow.
The build runner fetches that URL, so the origin must be a public http(s) address the runner can reach:

- A literal address on a non-public special-purpose range is refused at create and dispatch, without a DNS lookup.
  Non-public ranges: loopback, unspecified, private, link-local, shared, documentation, benchmarking, multicast, reserved, NAT64/6to4, ...
- A DNS hostname passes that syntactic check and is finally constrained when the workflow fetches the asset.

The operational `RUSTDESK_API_RUSTDESK_API_SERVER` is never used as a fallback.

**Fallback rule:** when a `PUBLIC_*` value is unset, the builder falls back to the operational value for its prefill fields.
The asset origin has no such fallback.

The builder's "Local IP" button additionally resolves the server's own network interface addresses server-side.
In Docker bridge setups those are container addresses.
The picker labels each candidate with its source interface.

### `SOURCE_DATE_EPOCH`

Rust build metadata uses `SOURCE_DATE_EPOCH` when supplied. Without it, active
builds use the deterministic value `unknown`; wall-clock metadata is available
only for explicitly non-reproducible local debug builds with
`RUSTDESK_NON_REPRODUCIBLE_DEBUG=1`.
