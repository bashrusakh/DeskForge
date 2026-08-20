# Phase 2 — PR2: network/public endpoints

**Status:** ✅ `verified-with-notes`
**Scope:** endpoint representation and public/runtime endpoint checks only. No claim of complete Docker or deployment coverage.

## Behavioral contract

The operator enters/chooses server endpoints in the existing form. The typed API persists and dispatches the literal endpoint supplied by the operator, including an explicit port where valid. The client/runtime derives only the protocol-specific defaults it already owns; normal users are not asked to provide internal WebSocket payloads.

## Delivered contract

- Endpoint values remain typed strings with hostname/IP and port validation at the BuildSpec boundary.
- The canonical path preserves literal endpoint ports; there is **no `stripPort` transformation** in the service canonicalization/dispatch contract.
- `server_ip`, `relay_server`, and `api_server` retain their distinct roles; the native mapping uses the existing key names and does not silently rewrite a supplied endpoint.
- The existing UI/web-client paths were checked for the public WebSocket port contract, including `21119` for relay/public flow and `21118` for the existing bridge flow.
- Docker entrypoint consumption of `custom_json` and the compose port/environment declarations were checked against the API/UI contract.

## Exact implementation/evidence record

Relevant files checked:

- `api/service/custom_build_spec.go`
- `api/service/custom_build_spec_test.go`
- `api/service/custom_build.go`
- `admin-ui/src/views/custom-client/index.vue`
- `admin-ui/src/utils/webclient.js`
- `docker/docker-compose.yml`
- `docker/entrypoint-linux.sh`
- `docker/entrypoint-win.sh`

Focused tests include literal endpoint preservation for host-only and host-with-port values, invalid ports/URLs, and dispatch/persisted-value agreement. The UI build and the Go focused package checks passed as recorded in Phase 1/5.

## Checks and notes

```text
GOWORK=off go test ./cmd ./model ./service ./http/controller/admin
GOWORK=off go test -race ./cmd ./model ./service ./http/controller/admin
go vet ./cmd ./model ./service ./http/controller/admin
npm run build
git diff --check
```

The checks are local/static and focused. Manual browser/runtime endpoint checks, documentation alignment, and the development compose exposure of `21119` remain gaps. The production compose file currently exposes the established `21114`–`21118` set; this phase does not claim that `21119` is published by Docker. No manual live-network or dev-compose run is evidence here.

## Gates and remaining dependencies

- Do not reintroduce port stripping in API normalization, dispatch preparation, or preset restore.
- Keep UI public-host behavior and Docker port/env declarations aligned before enabling additional public flows.
- Manual verification must cover an actual browser/public WebSocket path and both production and dev compose configurations.
- PR6 must settle workflow ownership/config before endpoint defaults are treated as deployment guarantees.
