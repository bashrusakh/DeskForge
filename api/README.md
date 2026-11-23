# DeskForge API

Go REST API server — part of [DeskForge](../README.md).
Embeds admin-ui dist (Vue 3) and web client. Gin + GORM, port 21114.

## Stack

Go 1.25 · Gin 1.9 · GORM 1.25 · JWT · LDAP · OIDC · Swag

## Databases

SQLite / MySQL / PostgreSQL — `RUSTDESK_API_GORM_TYPE`.

## Key endpoints

| Path                           | Description                                  |
| ------------------------------ | -------------------------------------------- |
| `/admin/`                        | Admin UI (SPA)                               |
| `/api/admin/*`                   | Admin REST API (admin-only)                  |
| `/api/*`                         | PC client API (login, address book, peer)    |
| `/admin/swagger/*`               | Swagger docs                                 |
| `/webclient/`                    | Web client                                   |

## CLI

```bash
./apimain reset-admin-pwd <password>   # reset admin password
./apimain -h                            # help
```

## Architecture

- **Controller** (`http/controller/`) — parse request → call service → return response
- **Service** (`service/`) — business logic, validation
- **Model** (`model/`) — GORM schema
- **Lib** (`lib/`) — cache, JWT, ORM, logger, lock, upload

Details: [AGENTS.md](../AGENTS.md).

## Build handoff provenance

The admin build handoff exports deterministic non-secret file metadata and
immutable source/workflow identity. `custom_.txt` is never exported. The stored
publication digest may nevertheless include its private configuration bytes as
part of the service-owned output proof; that digest is metadata, not disclosure
of the file contents.

## Custom-build API contracts

The API is mounted under `/api`. The Swagger source annotations use the existing
`/admin/...` paths with `/api` supplied by the Swagger `basePath`; the concrete HTTP
paths below include the full `/api` prefix.

### Admin handoff manifest

`GET /api/admin/custom_build/manifest/{id}` (`@Security token`)

- Returns the `response.Response` envelope with `data` as the redacted
  `service.BuildHandoffManifest`.
- `X-DeskForge-Manifest-SHA256` is lowercase hexadecimal SHA-256 over the exact
  response body bytes sent on the wire, including the response envelope. It is not
  a signature and is not the stored publication digest.
- The stored `published_digest` covers the service-owned canonical published
  output (including private `custom_.txt` when present). It has a distinct scope
  from both the manifest response-byte digest and the public ZIP digest.
- HTTP errors are `400` for an invalid ID, `409` when a completed verified handoff
  is unavailable, and `500` for an internal handoff failure. Unknown records retain
  the legacy HTTP 200 error envelope used by the controller.

### Public capability routes

These routes require no admin token. Possession of the opaque `key` is the
capability. New capability keys use `download-key-ttl`; when unset or invalid,
newly created builds use a seven-day TTL. Legacy rows with an expiry of `0` remain
unexpired.

`GET /api/admin/custom_build/public/detailByKey/{key}`

- Returns `response.Response` with redacted `data` as `model.CustomBuildPublic`.
- `409` means the build is not publicly ready; `410` means the capability has
  expired. Unknown keys retain the controller's legacy HTTP 200 error envelope.

`GET /api/admin/custom_build/public/download/{key}`

- Returns the exact redacted ZIP bytes with `Content-Type: application/zip`.
- `X-DeskForge-Archive-SHA256` hashes the actual ZIP bytes served by this response,
  not the JSON detail, not a provider asset digest, and not the stored publication
  digest.
- `X-DeskForge-Archive-SHA256-Scope` states that same exact-archive scope and
  explicitly distinguishes it from the stored publication digest.
- Other response headers include the exact `Content-Length` and an attachment
  `Content-Disposition` filename. Errors are `408` for cancellation/timeouts,
  `409` for unavailable completion/output provenance, `410` for expiry, and `500`
  for packaging or validation failure. Unknown keys retain the legacy HTTP 200
  error envelope.

Neither digest is a signature. These contracts document the service/provider
boundary only: there is no signature, attestation, or live provider execution/
approval claim in this documentation.

### Protected verified-tag approval

`GET /api/admin/github_build_config/workflow_tags` (`@Security token`)

- Returns `response.Response` with `data.tags` containing only safe
  `service.WorkflowTagOption` values derived from the configured provider.
- Raw refs, raw SHAs, provider verification objects, PATs, and payload keys are
  not normal client inputs or response values. Provider/configuration failures are
  reported as `500`, `502`, or `503` according to the controller classification.

`POST /api/admin/github_build_config/approve_workflow_ref` (`@Security token`)

Request body:

```json
{"confirm": true, "workflow_tag": "<label selected from workflow_tags>"}
```

Approval requires all of the following at the provider boundary:

- a provider-derived annotated tag with `verified=true` and an accepted provider
  verification reason;
- positive protection evidence for that exact tag from the supported modern
  ruleset surface, with no bypass actor and no tag/branch collision;
- the owned mapped workflow is present and ready at the provider-resolved
  immutable commit, declares `workflow_dispatch`, and contains the exact
  `# deskforge-workflow-identity-guard: v1` marker. Legacy tags without that
  guard are rejected.

The same marker check uses workflow content resolved at the immutable workflow SHA
before approval, preparation, and secret-bearing dispatch. It fails legacy unguarded
tags closed; it does not make GitHub's selector-based dispatch atomically bound to that
SHA or establish live-provider readiness.

The successful response is `response.Response` with `data` as the secret-free
`model.GithubBuildConfigSafe`. Invalid selectors or approval requests return
`400`; provider/configuration failures return `500`, `502`, or `503`. The legacy
validation envelope for malformed JSON or `confirm != true` remains HTTP 200 with
a nonzero response code.

This is a contract description, not live-provider evidence. No live GitHub/provider
approval, dispatch, run, artifact, signature, or attestation claim is made.

## Swagger source and generated output

The controller comments are the source Swagger annotation updates for these routes.
The generated Swagger documents under `api/docs/**` are synchronized with those
source annotations and are regenerated from them when the annotations change.

## Workflow protection evidence

Workflow-ref approval requires a provider-derived, verified annotated tag plus one modern provider-verified ruleset surface for the exact tag label — the modern ruleset surface is the only supported protection surface, and ruleset discovery/detail/policy/contract failures all fail closed: every applicable ruleset must expose `bypass_actors: []` and `current_user_can_bypass: "never"`, tag/branch collisions for the same label are rejected, and unknown or malformed conditions/rules/parameters fail closed (the former tag-protection LIST compatibility path is **CONFIRMED OBSOLETE** — the GitHub.com provider is hard-wired, DeskForge has no configurable GHES base URL, and GitHub marks that legacy surface as removed).
Recorded provider evidence: guard runs [32333446480](https://github.com/bashrusakh/rustdesk/actions/runs/32333446480) (outer `workflow_sha` mismatch, no secret-bearing job ran) and [32334155671](https://github.com/bashrusakh/rustdesk/actions/runs/32334155671) (passed the outer guard to the intentional post-inner failure) — together they evidence outer/inner guard sequencing only, not production artifact output — plus Repository Ruleset `20901403` (`DeskForge workflow tags`, protecting `refs/tags/workflow-*`, `bypass_actors=[]`, `current_user_can_bypass=never`).
Full ruleset rules (discovery bounds, applicability, fail-closed conditions, rule parameters, bypass, tag verification) and the observed `github/docs` ruleset `18281681` contract transcript are archived in [CHANGELOG.md](../CHANGELOG.md) ("Recorded workflow-protection evidence — archived from `api/README.md`, 2026-11-24"). This is a provider-boundary contract description, not a completed live approval, dispatch, run, artifact, signature, attestation, or release claim.

### GitHub fine-grained PAT permissions

Configure the stored fine-grained PAT with this repository permission checklist:

- **Metadata — Read**
- **Contents — Read**
- **Actions — Read and write** — read supports the checks; write is required for
  workflow dispatch.
- **Administration — Read and write** — write is required to retrieve ruleset
  bypass metadata; Administration read alone is insufficient.
- **Secrets — Read and write** — write is required for repository secret
  synchronization.

GitHub's Get repository ruleset endpoint only returns `bypass_actors` when the
caller has write access to the ruleset, so Administration write is required to
observe bypass actors and fail closed. Exact fine-grained PAT availability and
permissions are repository/provider settings, not proven by local fake-transport
tests.

This policy is a provider-boundary contract, not live-provider evidence: the
worktree has focused fake-transport coverage but no live GitHub/provider approval,
dispatch, run, or artifact pass.

Dispatch sends only the `ref` and required `inputs`; see
[PLAN.md §2](../PLAN.md#2-architecture) for the dispatch API-version contract and its
rationale. Local atomicity limitation: the `pending → building` identity write is
atomic, but no durable outbox or distributed lease coordinates provider dispatch with
that database write — a provider dispatch can succeed while the local write fails, so
no end-to-end atomic-dispatch claim is made.

## Schema and secret compatibility evidence

Published vs candidate schema state, the migration table (280–285), and migration-test evidence: [PLAN.md §2](../PLAN.md#2-architecture) — the single canonical statement. Migration tests are SQLite-only; **MySQL/PostgreSQL migration and read/write coverage remain unverified.**

`AutoMigrate` keeps legacy rows readable: metadata-only configuration saves preserve legacy plaintext PAT/payload-key values without requiring `SECRET_ENCRYPTION_KEY`, new or replaced non-empty secret writes require that key, and secret values are never returned in the safe configuration view — full semantics: [docs/DEPLOYMENT.md](../docs/DEPLOYMENT.md).

## Quick start

```bash
cd api
go build -o release/apimain cmd/apimain.go
# Broad local checks:
GOWORK=off go vet ./...
GOWORK=off go test ./...
```

Both broad checks passed (recorded 2026-08-11/12, after test-only cache diagnostics and opt-in Redis
test changes); that pass predates the schema-284/285 code of 2026-10-13 and is therefore stale rather
than contradicted (evidence dating: `PLAN.md:69-71`).
Redis integration tests and benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is configured; no live
Redis run is recorded. MySQL/PostgreSQL, live provider, and the other release/build gates remain
unverified.
