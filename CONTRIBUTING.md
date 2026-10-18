<!-- generated-by: repo-docs-sync; file: CONTRIBUTING.md -->

# Contributing to DeskForge

## Project Context
Repository: `bashrusakh/DeskForge` — default branch: `main` — observed commit: `eb43c9769c96a48c2f18aefcc3b0df2dde4c7ef3`.

Project purpose: Not recorded in the collected context.

Facts not found in the repository context:
- environments: No environment definitions observed in collected files

Facts the repository does not support:
- policy.version\_file: No version source-of-truth file documented

## Branch Model
Use this stable policy:
- `main` is the protected default branch when it is the repository default branch.
- If the repository default branch has another name, treat that default branch as protected.
- Direct push to the default branch is disabled.
- Force-pushes and branch deletion on protected branches are disabled.
- Every change goes through a Pull Request.
- One approving review is required when the repository policy requires review.
- Emergency hotfixes may temporarily change branch protection only via GitHub UI/admin action.
- This protects against accidental pushes to the wrong branch and keeps a clean history.

## Workflow
```bash
git checkout main
git pull --ff-only
git checkout -b feature/short-description
# or fix/, chore/, docs/
# edit, commit, validate
git push -u origin feature/short-description
gh pr create --fill
```
Replace `main` with the repository's actual default branch when needed. Merge happens after review and CI/checks pass.

Branch prefixes:
- `feature/` — new functionality
- `fix/` — bug fix
- `chore/` — tooling, dependencies, CI, or refactor without behavior change
- `docs/` — documentation only

## Getting Started
Prerequisites observed for repository commands:
- `docker/Dockerfile` (listed by build-docker)
- `docker/docker-compose.yml` (listed by build-docker)
- `server/Cargo.toml` (listed by build-rust)
- `api/go.mod` (listed by build-go)
- `admin-ui/package.json` (listed by dev-admin-ui)

## Dev Commands
**build-docker** — working directory `.`; prerequisites: `docker/Dockerfile`, `docker/docker-compose.yml`; evidence: `AGENTS.md`

```bash
docker compose build
```

**run-docker** — working directory `docker`; prerequisites: `docker/docker-compose.yml`; evidence: `AGENTS.md`

```bash
docker compose up -d
```

**build-rust** — working directory `server`; prerequisites: `server/Cargo.toml`; evidence: `AGENTS.md`

```bash
cargo build --release && cargo clippy && cargo test
```

**build-go** — working directory `api`; prerequisites: `api/go.mod`; evidence: `AGENTS.md`

```bash
go build -o release/apimain cmd/apimain.go
```

**dev-admin-ui** — working directory `admin-ui`; prerequisites: `admin-ui/package.json`; evidence: `AGENTS.md`

```bash
npm install && npm run dev && npm run build
```

## Before Submitting
**go-tests** — working directory `api`; prerequisites: `api/go.mod`; evidence: `.github/workflows/build_test.yml`

```bash
go test ./... && go vet ./...
```
- Check git status.
- Review the full diff.
- Make sure only intended files are included.
- Run the narrowest relevant tests/checks from repository evidence.
- If tests cannot run, report the exact command and exact error.

## Code Style

- Keep changes focused.
- Prefer clear, simple code over clever code.
- Avoid unrelated refactors.
- Do not silently change behavior outside the task scope.
- Reuse existing project style and architecture.
- Do not introduce new dependencies, frameworks, tooling, generated files, or large artifacts unless the change explicitly requires it.
- Do not commit secrets, tokens, credentials, local-only config, logs, cache files, benchmark outputs, or generated runtime artifacts unless explicitly required.

## Commit Messages
Use this convention:
```text
scope: short imperative summary
```
- English only.
- Lowercase scope.
- Short imperative summary.
- No trailing period.
- Keep each commit specific to one logical change.
Observed commit-message scopes:
- `admin-ui` — Vue admin panel changes (commit messages)
- `api` — Go backend/API changes (commit messages)
- `server` — Rust hbbs/hbbr changes (commit messages)
- `docker` — Dockerfile/compose changes (commit messages)
- `workflow` — GitHub Actions/CI changes (commit messages)
- `docs` — Documentation updates (commit messages)

## Pull Requests
PR titles should be concise and describe the change. When the repository uses scoped PR titles, use:
```text
scope: short imperative summary
```
The PR body should include, at minimum:
- summary
- why/context
- validation/tests run
- screenshots or manual verification steps for UI changes, when relevant

This PR guidance is independent of the commit-message convention above. Do not import an unrelated title grammar or require a scope when the repository does not use scoped PR titles.
Do not mark work as ready if checks are unknown or failing.
AI-assisted contributions should include a co-author trailer when appropriate:

```text
Co-Authored-By: opencode
```

## Triage labels
Labels are triage metadata, not a correctness determination or merge approval. Apply the correct type label when opening an issue or pull request, subject to available labels.

Apply exactly one type label:
- `bug` — something is broken or behaves incorrectly.
- `enhancement` — new functionality or a feature request.
- `documentation` — documentation-only change or doc issue.
- `question` — a usage or support question.
- `refactor` — internal change with no behavior change.
- `ci` — CI, build, or tooling change.

Automation-managed labels (do not apply by hand): `needs-info` and `duplicate`.
`confirmed` is not automation-managed. It records human or verification-owned confirmation and must be applied only by maintainers or authorized triagers after relevant verification.
`priority-*` and other operational labels are maintainer-owned.

## Issues
Before opening an issue, verify the problem against the current code/state and search existing issues if possible. Describe facts only; do not invent root cause. If root cause is only suspected, label it as a hypothesis. Include the category's relevant reproduction, expected behavior, affected component, environment/version, evidence, and constraints. Keep the issue actionable and specific.

## Project Policies

### Licensing
The combined DeskForge distribution is identified as AGPL‑3.0 for the covered work; separate components retain upstream MIT or other notices as documented in LICENSE and NOTICE.
evidence: `README.md`

### Commits
Commit messages use the form "scope: short summary" with scopes admin‑ui, api, server, docker, workflow, docs as observed in recent commits.
evidence: `CONTRIBUTING.md`

### Configuration
Key environment variables (RELAY, HBBR\_PORT, HBBS\_PORT, ENCRYPTED\_ONLY, etc.) configure the Rust servers and Go API as listed in README.md.
evidence: `README.md`

### Releases
Every release must include meaningful release notes and be reviewed before publishing; auto‑generated release bodies are considered unfinished.
evidence: `CONTRIBUTING.md`

## Documentation Maintenance
Project structure changes over time. AGENTS.md and CONTRIBUTING.md are guidance, not a replacement for checking the actual tree. If project structure, commands, architecture, test flow, contribution rules, release flow, or important entry points change, update the relevant Markdown file in the same PR. Keep docs useful and stable, and keep Markdown updates small. Do not duplicate the full file tree unless the project explicitly requires it. Describe stable conventions and important entry points, not every file.

## Releases
Every release must include meaningful release notes. Before publishing, check previous notes, review changes since the previous tag, and use actual commits, merged PRs, linked issues, and final code changes. Do not invent features, fixes, impact, or breaking changes. Call out user-visible, breaking, migration, compatibility, security, benchmark/default, and data-format changes when relevant. If a release is created automatically by GitHub Actions, its body must still be reviewed and updated before completion. A GitHub Release with an empty, placeholder, too thin, or unreviewed auto-generated-only body is considered unfinished, even if the tag and assets were created successfully. Before marking release work as done, verify the GitHub Release page contains the correct tag, title, body, and assets.

## Project Structure
Components and important paths observed in the repository:
- **server** — Rust hbbs/hbbr server code (paths: `server/Cargo.toml`; evidence: `server/Cargo.toml`)
- **api** — Go REST API source (paths: `api/go.mod`; evidence: `api/go.mod`)
- **admin-ui** — Vue 3 admin panel code (paths: `admin-ui/package.json`; evidence: `admin-ui/package.json`)
- **docker** — Docker build definition for combined image (paths: `docker/Dockerfile`; evidence: `docker/Dockerfile`)
- **rdgen** — Python/Django reference workflow generator (frozen) (paths: `rdgen/Dockerfile`; evidence: `rdgen/Dockerfile`)

## Security and Runtime Notes
Runtime environments: Not recorded in the collected context.

Security notes: Not found in repository.

## Questions / Support
Not found in repository.

## License
`LICENSE` is present in the repository.

## Preserved from previous CONTRIBUTING

<!-- carry-forward: bashrusakh/DeskForge@eb43c9769c96a48c2f18aefcc3b0df2dde4c7ef3; section: Preserved from previous CONTRIBUTING -->
## Preserved from previous CONTRIBUTING

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: Workflow -->

<!-- carry-forward: bashrusakh/DeskForge@eb43c9769c96a48c2f18aefcc3b0df2dde4c7ef3; section: Workflow -->
## Workflow

```bash
# always start from fresh main
git checkout main
git pull --ff-only

# branch per change
git checkout -b feature/short-description     # or fix/, chore/, docs/
# ... edit, commit ...
git push -u origin feature/short-description

# open PR (via gh CLI)
gh pr create --fill
# merge after CI/checks pass
gh pr merge --squash --delete-branch
```

Branch prefixes (loose convention, not enforced):

| Prefix | When |
|---|---|
| `feature/` | new functionality |
| `fix/` | bug fix |
| `chore/` | tooling, deps, CI, refactor without behavior change |
| `docs/` | docs only |

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: Pull Request titles -->

<!-- carry-forward: bashrusakh/DeskForge@eb43c9769c96a48c2f18aefcc3b0df2dde4c7ef3; section: Pull Request titles -->
## Pull Request titles

Pull Request titles should follow the same template as commit messages.

Template:

```text
<scope>: <imperative summary>
```

Examples:

```text
admin-ui: remove remaining legacy table and dialog remnants
docker: build admin-ui inside production image
workflow: switch packer to single-binary output
```

PR body is free-form, but should usually include:

- summary
- why
- validation

Co-author trailers welcome when AI agents contributed:

```
Co-Authored-By: Claude <noreply@anthropic.com>
```

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: License & attribution (AGPL-3.0) -->

<!-- carry-forward: bashrusakh/DeskForge@eb43c9769c96a48c2f18aefcc3b0df2dde4c7ef3; section: License & attribution (AGPL-3.0) -->
## License & attribution (AGPL-3.0)

The covered DeskForge distribution is identified as **AGPL-3.0** because `server/` is
AGPL-3.0 and is the strongest copyleft in the covered bundle. This does not relicense
separate or independent components; preserve each component's applicable license and
notice. The license inventory is incomplete; no signatures or attestations are recorded,
and no full sovereignty claim is made.

When you add new files derived from upstream sources:

- **Keep** the upstream copyright header at the top of the file.
- **Append** your modification line below it, don't replace.
- New original files: standard AGPL header is fine; add yours.

See [NOTICE](NOTICE) for the full list of bundled components and their copyrights.

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: Local development -->

<!-- carry-forward: bashrusakh/DeskForge@eb43c9769c96a48c2f18aefcc3b0df2dde4c7ef3; section: Local development -->
<!-- carry-forward-stale: references manifest-absent path(s): Dockerfile -->
## Local development

Working tree is **Windows-friendly** (LF/CRLF auto-conversion). If you want
explicit control, the repo doesn't ship a `.gitattributes` yet — feel free to
add one in a PR.

To run the server stack:

```bash
cd docker
docker compose build rustdesk  # build the combined server image
docker compose up -d rustdesk
docker compose logs -f rustdesk
```

For GitHub-based client-build workflow maintenance, the configured RustDesk fork's
`.github/workflows/` files are the sole active executable workflow source. See
[PLAN.md](PLAN.md) §7 for the historical maintenance notes. `github-build/` is
frozen reference/documentation material only, and `rdgen/` is frozen vendored
historical/reference material; neither is an active workflow source.

The published RustDesk client source/ref is 1.4.8 and the published DeskForge API
schema is `DatabaseVersion` 272. The current local corrective candidate targets API
schema 283, which is not published. Schema 283 adds
`idx_custom_presets_user_id_name` on `(user_id, name)` and fails migration preflight
on existing duplicate groups without auto-selecting a row or deleting data. Schema 282 remains
the earlier workflow-approval migration. Live provider execution remains unverified.
Migration evidence is SQLite-only; MySQL/PostgreSQL migration/read/write coverage
remains unverified.

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: What goes where -->
<!-- carry-forward-stale: references manifest-absent path(s): Dockerfile -->

<!-- carry-forward: bashrusakh/DeskForge@eb43c9769c96a48c2f18aefcc3b0df2dde4c7ef3; section: What goes where -->
<!-- carry-forward-stale: references manifest-absent path(s): Dockerfile -->
## What goes where

| Directory | What |
|---|---|
| `server/` | Rust hbbs/hbbr (relay + ID server). AGPL-3.0. |
| `api/` | Go REST API + admin endpoints. MIT. |
| `admin-ui/` | Vue 3 admin panel. MIT. |
| `libs/` | Shared Rust libs. |
| `docker/` | Dockerfiles + compose. |
| `github-build/` | Frozen reference/documentation for fork workflows; no executable workflow copies. |
| `win-builder/` | Frozen manual/historical-only Windows build material; not the API path. |
| `offline-kit/` | Frozen dependency-freeze tool; verification and license inventory are incomplete, with no signature/attestation or full-sovereignty claim. |
| `rdgen/` | Frozen vendored historical/reference workflow material, not the active source; not running as a service. GPL-3.0. |
| `PLAN.md` | Single source of truth for the project plan. |
| `CHANGELOG.md` | Chronological log of changes. |

The current repository has no tracked `vendor/` tree, `rustdesk-deps/` archive, or
client-release directory. Rust and Go manifests still use active third-party upstream
dependencies, and the configured RustDesk fork's `hbb_common` submodule is currently
upstream-referenced, dirty, and unpublished; upstream independence and MySQL/PostgreSQL
cross-database support therefore remain unverified.
