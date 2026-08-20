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
- the owned workflow is present and ready at the provider-resolved commit.

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

Workflow-ref approval requires a provider-derived, verified annotated tag and
one modern provider-verified ruleset surface for the exact tag label. The modern
ruleset surface is authoritative and the only supported protection surface;
initial list, later page, detail, policy, and contract failures all fail closed:

- modern `GET /repos/{owner}/{repo}/rulesets?targets=tag&includes_parents=true`, bounded to
  three pages and 256 rulesets, discovers candidate IDs only. DeskForge then
  consumes `GET /repos/{owner}/{repo}/rulesets/{id}?includes_parents=true` and
  validates the repository-scoped detail metadata (`name`, `source`, and
  `source_type` in `Repository`, `Organization`, or `Enterprise`; documented
  `created_at` and `updated_at` values are type-checked when present). The
  repository-scoped `includes_parents=true` response establishes applicability
  at the provider boundary. DeskForge requires `conditions.ref_name` for tag
  matching and does not independently evaluate `repository_name`,
  `repository_id`, or `repository_property` selectors. The separate
  organization authoring endpoint, `POST /orgs/{org}/rulesets`, uses repository
  selectors to define where a ruleset applies; that create-schema requirement
  must not be imposed on repository-scoped inherited detail responses. Unknown
  condition fields and malformed condition values fail closed. Excludes win.
  `tag_name_pattern` is validated only as rule metadata; it is not the ref
  selector, and no `fnmatch` rule is required for selection. For tag rules,
  `update` without parameters is valid; when parameters are present they must
  be a strict object containing the boolean
  `update_allows_fetch_and_merge`. Null, empty, malformed, or unknown parameter
  objects fail closed. Branch-target rules continue to require that parameter.
  Every applicable ruleset must expose `bypass_actors: []` and
  `current_user_can_bypass: "never"`; any applicable bypass actor or other
  current-user bypass mode rejects approval. Effective `update` and `deletion`
  rules may be split across matching rulesets. Other documented known rules are
  neutral or stronger additions, while unknown or malformed rules fail closed.
  Inherited parent rulesets use the same checks;
- the provider must reject a tag/branch collision for the same label, and the
  tag's verification must report `verified=true` with the accepted provider
  verification reason. Raw SHA selectors are not approval inputs.

The former tag-protection LIST compatibility path is **CONFIRMED OBSOLETE**:
the GitHub.com provider is hard-wired to `https://api.github.com`, DeskForge has
no configurable GHES base URL or provider, and current GitHub documentation marks
that legacy surface as closing down/removed. A GHES installation may have a
historical `enabled`/`pattern` schema, but it is separate context rather than a
DeskForge capability. A mismatch, absent evidence, any bypass actor, or
an active applicable ruleset without effective update and deletion protection
never proves protection.

### Recorded ruleset provider evidence

The endpoint distinction follows the [organization ruleset creation
documentation](https://docs.github.com/en/rest/orgs/rules#create-an-organization-repository-ruleset),
the [repository ruleset detail documentation](https://docs.github.com/en/rest/repos/rules#get-a-repository-ruleset),
and the [tag protection LIST documentation](https://docs.github.com/en/rest/repos/tags#get-all-tag-protection-states-for-a-repository).

On 2026-08-14, an authenticated observation of `github/docs` returned the
following summary from the repository-scoped list endpoint:

```text
GET https://api.github.com/repos/github/docs/rulesets?targets=tag&includes_parents=true&per_page=100
=> id=18281681, target=tag, source_type=Organization, source=github, enforcement=active
```

The summary did not contain conditions, rules, or bypass metadata. The matching
repository-scoped detail was:

```text
GET https://api.github.com/repos/github/docs/rulesets/18281681?includes_parents=true
```

It was ruleset `18281681` (`Enterprise Tags`) with
`conditions.ref_name` only and these exact include patterns:

```text
refs/tags/enterprise-[0-9].*-freeze
refs/tags/enterprise-[0-9].[0-9].[0-9]
refs/tags/enterprise-[0-9].[0-9].[0-9][0-9]
refs/tags/enterprise-[0-9].[0-9][0-9].[0-9]
refs/tags/enterprise-[0-9].[0-9][0-9].[0-9][0-9]
refs/tags/enterprise-[0-9].*.pre[0-9]
refs/tags/enterprise-[0-9].*.pre[0-9][0-9]
refs/tags/enterprise-[0-9].*.gm[0-9]
refs/tags/enterprise-[0-9].*.gm[0-9][0-9]
refs/tags/enterprise-[0-9].*.rc[0-9]
refs/tags/enterprise-[0-9].*.rc[0-9][0-9]
```

Its rules were `deletion`, `non_fast_forward`, `creation`, and `update` with
no parameters; `created_at` was `2026-06-30T07:19:50.340+11:00`,
`updated_at` was `2026-06-30T07:20:37.936+11:00`, and
`current_user_can_bypass` was `never`. `bypass_actors` was omitted because the
observing token lacked write access. DeskForge therefore uses
`bypass_actors: []` as the positive write-authorized contract fixture and fails
closed when the field is omitted. This observation is provider evidence, not a
completed live approval, dispatch, or release claim. Initial and later ruleset
list failures, detail failures, policy failures, and contract failures remain
failures.

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
worktree has focused fake-transport coverage but no live GitHub/provider
approval, dispatch, run, or artifact pass. Dispatch requires the provider's
exact HTTP 200 run-details response with `return_run_details=true`; a standard
204 response and latest-run inference are unsupported. The local pending →
building identity write is atomic, but there is no durable outbox or distributed
lease coordinating provider dispatch with that database write. A provider
dispatch can therefore succeed while the local write fails; no end-to-end
atomic-dispatch claim is made.

## Schema and secret compatibility evidence

The published DeskForge API schema is `DatabaseVersion` **272**. The current
uncommitted corrective worktree targets local schema **282** and is not published
or live-provider evidence. The additive workflow-approval fields are:

- **280** — `workflow_ref_approved`, the persisted approval status;
- **281** — `workflow_ref_provider_verified`, the persisted provider-policy
  verification status;
- **282** — `workflow_ref_approval_sha`, the provider-resolved commit recorded
  for the approved tag.

`AutoMigrate` keeps legacy rows readable. Legacy plaintext PAT/payload-key
metadata remains readable, and metadata-only configuration saves preserve those
legacy values without requiring `SECRET_ENCRYPTION_KEY`. New or replaced
non-empty secret writes require that key; when it is available, resaving a
legacy row encrypts its plaintext secrets. Secret values are never returned in
the safe configuration view.

## Quick start

```bash
cd api
go build -o release/apimain cmd/apimain.go
# Broad local checks:
GOWORK=off go vet ./...
GOWORK=off go test ./...
```

Both broad checks pass after test-only cache diagnostics and opt-in Redis test
changes. Redis integration tests and benchmarks run only when
`DESKFORGE_TEST_REDIS_ADDR` is configured; no live Redis run is recorded.
MySQL/PostgreSQL, live provider, and the other release/build gates remain
unverified.
