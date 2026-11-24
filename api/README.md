# DeskForge API

Go REST API server of [DeskForge](../README.md). Gin + GORM on port 21114; it also serves
the admin UI (embedded Vue build) and the web client.

**Stack:** Go 1.25 · Gin 1.9 · GORM 1.25 · JWT · LDAP · OIDC · Swagger.
**Databases:** SQLite, MySQL, PostgreSQL (`RUSTDESK_API_GORM_TYPE`). Migrations are only
tested on SQLite.

## Build and test

```bash
cd api
go build -o release/apimain cmd/apimain.go
GOWORK=off go vet ./...
GOWORK=off go test ./...
```

Redis integration tests and benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is set.

## Endpoints

| Path | Description |
| --- | --- |
| `/admin/` | Admin UI (SPA) |
| `/api/admin/*` | Admin REST API (admin only) |
| `/api/*` | PC client API (login, address book, peers) |
| `/admin/swagger/*` | Swagger docs |
| `/webclient/` | Web client |

## CLI

```bash
./apimain reset-admin-pwd <password>   # reset the admin password
./apimain -h                            # help
```

## Layout

- `http/controller/`: parse the request, call a service, return the response
- `service/`: business logic and validation
- `model/`: GORM schema
- `lib/`: cache, JWT, ORM, logger, lock, upload

More: [AGENTS.md](../AGENTS.md). Swagger annotations in the controllers are the source of
the generated `api/docs/**`; regenerate it when they change.

## Custom-build API

All routes are under `/api`. Swagger annotations use `/admin/...` paths and add `/api`
through the Swagger `basePath`. Responses use the `response.Response` envelope. For unknown
records or keys the controllers keep a legacy HTTP 200 error envelope.

### Digests

Three different SHA-256 values exist. None of them is a signature.

| Digest | Covers |
| --- | --- |
| `X-DeskForge-Manifest-SHA256` | exact bytes of the manifest response body, envelope included |
| `X-DeskForge-Archive-SHA256` | exact bytes of the public ZIP served by that response |
| stored `published_digest` | canonical published output, including the private `custom_.txt` (metadata only; the file is never exported) |

### Handoff manifest

`GET /api/admin/custom_build/manifest/{id}` (token required) returns the redacted
`service.BuildHandoffManifest`. Errors: `400` invalid ID, `409` no completed verified
handoff, `500` internal failure.

### Public capability routes

No admin token: possessing the opaque `key` is the capability. New keys expire after
`download-key-ttl` (seven days when unset or invalid); legacy rows with expiry `0` never expire.

- `GET /api/admin/custom_build/public/detailByKey/{key}` returns `model.CustomBuildPublic`.
  `409`: build not ready; `410`: key expired.
- `GET /api/admin/custom_build/public/download/{key}` returns the redacted ZIP
  (`application/zip`, with `Content-Length`, attachment `Content-Disposition` and the
  `X-DeskForge-Archive-SHA256` header plus `-Scope`). Errors: `408` cancelled or timed out,
  `409` output unavailable, `410` expired, `500` packaging or validation failure.

### Workflow approval

Provider-backed builds run only from an approved, verified workflow tag.

`GET /api/admin/github_build_config/workflow_tags` (token required) returns safe
`service.WorkflowTagOption` values (`data.tags`); raw refs, SHAs and secrets are never
returned. Provider or configuration failures give `500`, `502` or `503`.

`POST /api/admin/github_build_config/approve_workflow_ref` (token required):

```json
{"confirm": true, "workflow_tag": "<label selected from workflow_tags>"}
```

Approval requires all of:

- an annotated tag that the provider reports as `verified=true` with an accepted reason;
- a modern repository ruleset protecting exactly that tag, with `bypass_actors: []`,
  `current_user_can_bypass: "never"` and no tag/branch name collision;
- the mapped workflow present at the tag's immutable commit, declaring `workflow_dispatch`
  and containing the marker `# deskforge-workflow-identity-guard: v1` (legacy tags without
  it are rejected).

Any unknown or malformed ruleset data fails closed. The marker is re-checked at the immutable
SHA before approval, preparation and every secret-bearing dispatch. GitHub's tag-based dispatch
is not atomically bound to that SHA, so the verified tag and the ruleset are compensating controls.

Success returns the secret-free `model.GithubBuildConfigSafe`. Invalid selectors give `400`;
malformed JSON or `confirm != true` keeps the legacy HTTP 200 error envelope.

Dispatch sends only `ref` and the required `inputs`; the API-version pin is described in
[PLAN.md](../PLAN.md#2-architecture). Known limitation: a provider dispatch can succeed while
the local `pending → building` write fails, because there is no outbox or distributed lease.

### GitHub fine-grained PAT permissions

Repository permissions for the stored token:

- **Metadata**: Read
- **Contents**: Read
- **Actions**: Read and write (write is needed to dispatch workflows)
- **Administration**: Read and write (GitHub returns ruleset `bypass_actors` only to callers
  with write access, so read alone cannot verify protection)
- **Secrets**: Read and write (to sync repository secrets)

## Secrets and schema

Secrets are encrypted at rest (`SECRET_ENCRYPTION_KEY`) and never returned by the safe
configuration view; see [docs/DEPLOYMENT.md](../docs/DEPLOYMENT.md). Schema versions and
migration notes: [PLAN.md](../PLAN.md#2-architecture).
