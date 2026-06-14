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

DeskForge is distributed under **AGPL-3.0** (because `server/` is AGPL-3.0 and it's
the strongest copyleft in the bundle).

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
docker compose build server     # rebuilds Go code; admin-ui needs pre-built dist
docker compose up -d server
docker compose logs -f server
```

For the GitHub-based Windows client builder workflow — see [PLAN.md](PLAN.md) §8.8.

## What goes where

| Directory | What |
|---|---|
| `server/` | Rust hbbs/hbbr (relay + ID server). AGPL-3.0. |
| `api/` | Go REST API + admin endpoints. MIT. |
| `admin-ui/` | Vue 3 admin panel. MIT. |
| `libs/` | Shared Rust libs. |
| `docker/` | Dockerfiles + compose. |
| `github-build/` | Workflow + docs for building Windows client via GitHub Actions. |
| `win-builder/` | Native Windows build agent (fallback path, frozen). |
| `offline-kit/` | Frozen toolchain + sources (sovereign build kit). |
| `rdgen/` | Vendored reference: rdgen workflow patches (not running as a service). GPL-3.0. |
| `PLAN.md` | Single source of truth for the project plan. |
| `CHANGELOG.md` | Chronological log of changes. |
