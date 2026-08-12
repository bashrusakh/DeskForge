# Contributing to DeskForge

## Branch model

`main` is the protected default branch:

- direct push is **disabled** (force-pushes and branch deletion too)
- every change goes through a Pull Request
- 0 approving reviews required (solo-maintainer setup) — open + merge in one click
- admin can temporarily lift the rule via GitHub UI for emergency hotfixes

This protects against accidental `git push` to the wrong branch and gives a clean
history of "what landed when".

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

## Commit messages

Commits should use one standard template.

Template:

```text
<scope>: <imperative summary>
```

Rules:

- English only
- lowercase scope
- short imperative summary
- no trailing period
- keep it specific to one logical change

Preferred scopes are based on the touched area:

| Scope | When |
|---|---|
| `admin-ui` | Vue admin panel changes |
| `api` | Go backend/API changes |
| `server` | Rust hbbs/hbbr changes |
| `docker` | Dockerfiles / compose |
| `workflow` | GitHub Actions / CI |
| `docs` | docs only |
| `fix(<area>)` | focused bug fix when that reads better |

Examples:

```text
admin-ui: migrate remaining tables to DataTable
docker: fix build-win copy paths
api: add nocache middleware for /admin/*
fix(custom-client): enforce hostname-only server_ip
docs: clarify L2 custom_.txt flow
```

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
schema is `DatabaseVersion` 272. The local uncommitted corrective worktree targets
API schema 282; that local schema target is not published. Live provider execution and
MySQL/PostgreSQL migration/read/write coverage remain unverified.

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
