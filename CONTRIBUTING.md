# Contributing to DeskForge

## Project Context

Repository: `bashrusakh/DeskForge` — default branch: `main`.
Project purpose and current state: [README.md](README.md) (showcase) and
[PLAN.md](PLAN.md) (single source of truth).

Canonical homes for project facts (cross-link, do not copy): env-var table →
[README.md](README.md) ("Environment" section); env semantics →
[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md); schema/migration evidence and project
plan → [PLAN.md](PLAN.md); fork/pin topology →
[docs/FORKS-AND-PINS.md](docs/FORKS-AND-PINS.md); monorepo layout →
[AGENTS.md](AGENTS.md).

## Branch Model

- `main` is the protected default branch.
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
gh pr merge --squash --delete-branch
```

Merge happens after review and CI/checks pass.

Branch prefixes (loose convention, not enforced):

| Prefix | When |
|---|---|
| `feature/` | new functionality |
| `fix/` | bug fix |
| `chore/` | tooling, deps, CI, refactor without behavior change |
| `docs/` | docs only |

## Getting Started

Prerequisites: Docker (`docker/Dockerfile`, `docker/docker-compose.yml`), Rust
(`server/Cargo.toml`), Go (`api/go.mod`), Node (`admin-ui/package.json`).

## Dev Commands

Working directories: `docker/` for Compose, `server/`, `api/`, `admin-ui/`.

```bash
# Docker (full image + dev API stack)
docker compose build && docker compose up -d
cd api && docker compose -f docker-compose-dev.yaml up -d

# Rust
cd server && cargo build --release && cargo clippy && cargo test

# Go
cd api && go build -o release/apimain cmd/apimain.go
GOWORK=off go vet ./...
GOWORK=off go test ./...

# Admin UI
cd admin-ui && npm install && npm run dev && npm run build
```

Redis integration tests/benchmarks are opt-in (`DESKFORGE_TEST_REDIS_ADDR`); no live
Redis run is recorded.

## Before Submitting

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

Observed scopes: `admin-ui`, `api`, `server`, `docker`, `workflow`, `docs`.

## Pull Requests

PR titles follow the same scoped template as commit messages
(`<scope>: <imperative summary>`). Examples:

```text
admin-ui: remove remaining legacy table and dialog remnants
docker: build admin-ui inside production image
workflow: switch packer to single-binary output
```

The PR body should include, at minimum:

- summary
- why/context
- validation/tests run
- screenshots or manual verification steps for UI changes, when relevant

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

## Documentation Maintenance

Project structure changes over time. AGENTS.md and CONTRIBUTING.md are guidance, not a replacement for checking the actual tree. If project structure, commands, architecture, test flow, contribution rules, release flow, or important entry points change, update the relevant Markdown file in the same PR. Keep docs useful and stable, and keep Markdown updates small. Do not duplicate the full file tree unless the project explicitly requires it. Describe stable conventions and important entry points, not every file.

## Releases

Every release must include meaningful release notes. Before publishing, check previous notes, review changes since the previous tag, and use actual commits, merged PRs, linked issues, and final code changes. Do not invent features, fixes, impact, or breaking changes. Call out user-visible, breaking, migration, compatibility, security, benchmark/default, and data-format changes when relevant. If a release is created automatically by GitHub Actions, its body must still be reviewed and updated before completion. A GitHub Release with an empty, placeholder, too thin, or unreviewed auto-generated-only body is considered unfinished, even if the tag and assets were created successfully. Before marking release work as done, verify the GitHub Release page contains the correct tag, title, body, and assets.

## Project Structure

The canonical monorepo layout is [AGENTS.md](AGENTS.md#monorepo-layout); frozen
directories (`rdgen/`, `win-builder/`, `offline-kit/`) and reference material
(`github-build/`) are marked there. Do not duplicate the tree here.

## License

See [LICENSE](LICENSE) and [NOTICE](NOTICE).

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: License & attribution (AGPL-3.0) -->
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
## Local development

Working tree is **Windows-friendly** (LF/CRLF auto-conversion). If you want
explicit control, the repo doesn't ship a `.gitattributes` yet — feel free to
add one in a PR. To run the server stack, see "Dev Commands" above.

For GitHub-based client-build workflow maintenance, the configured RustDesk fork's
`.github/workflows/` files are the sole active executable workflow source. See
[PLAN.md](PLAN.md) §7 for the maintenance procedure. `github-build/` is
frozen reference/documentation material only, and `rdgen/` is frozen vendored
historical/reference material; neither is an active workflow source. Fork/pin facts
and rules: [docs/FORKS-AND-PINS.md](docs/FORKS-AND-PINS.md); current schema state:
[PLAN.md §2](PLAN.md#2-architecture).
