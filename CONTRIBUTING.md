# Contributing to DeskForge

## Project Context

Repository: `bashrusakh/DeskForge` — default branch: `main`.
Project purpose and current state: [README.md](README.md) (showcase) and
[PLAN.md](PLAN.md) (single source of truth).

## Where facts live (cross-link, do not copy)

- env-var table → [`.env.example`](.env.example); env semantics → [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)
- monorepo layout and frozen-status labels → [AGENTS.md](AGENTS.md)
- schema/migration + dispatch evidence → [PLAN.md §2](PLAN.md#2-architecture); fork/pin topology, sync rules, and workflow ownership → [docs/FORKS-AND-PINS.md](docs/FORKS-AND-PINS.md)
- pin + patch record → [`libs/hbb_common/UPSTREAM.md`](libs/hbb_common/UPSTREAM.md); new-upstream maintenance procedure → [PLAN.md §7](PLAN.md#7-fork-maintenance-procedure-for-a-new-upstream-rustdesk-client-release)

## Branch Model

- `main` is the protected default branch: direct push, force-pushes, and branch deletion are disabled.
- Every change goes through a Pull Request; one approving review is required when the repository policy requires review.
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

Branch prefixes (loose convention, not enforced): `feature/` new functionality ·
`fix/` bug fix · `chore/` tooling, deps, CI, refactor without behavior change · `docs/` docs only.

## Getting Started

Prerequisites: Docker (`docker/Dockerfile`, `docker/docker-compose.yml`), Rust (`server/Cargo.toml`),
Go (`api/go.mod`), Node (`admin-ui/package.json`). Working directories: `docker/` for Compose, `server/`, `api/`, `admin-ui/`.

## Dev Commands

```bash
# Docker (full image + dev API stack)
cd docker && docker compose build && docker compose up -d
cd ../api && docker compose -f docker-compose-dev.yaml up -d

# Rust
cd server && cargo build --release && cargo clippy && cargo test

# Go
cd api && go build -o release/apimain cmd/apimain.go
GOWORK=off go vet ./...
GOWORK=off go test ./...

# Admin UI
cd admin-ui && npm install && npm run dev && npm run build
```

Redis integration tests/benchmarks are opt-in (`DESKFORGE_TEST_REDIS_ADDR`); no live Redis run is recorded.

## Before Submitting

- Check `git status`, review the full diff, and make sure only intended files are included.
- Run the narrowest relevant tests/checks from repository evidence.
- If tests cannot run, report the exact command and exact error.

## Code Style

- Keep changes focused; prefer clear, simple code over clever code; avoid unrelated refactors.
- Do not silently change behavior outside the task scope; reuse existing project style and architecture.
- Do not introduce new dependencies, frameworks, tooling, generated files, or large artifacts unless the change explicitly requires it.
- Do not commit secrets, tokens, credentials, local-only config, logs, cache files, benchmark outputs, or generated runtime artifacts unless explicitly required.

## Commit Messages

Convention: `scope: short imperative summary` — English only, lowercase scope, short
imperative summary, no trailing period, one logical change per commit. Observed scopes:
`admin-ui`, `api`, `server`, `docker`, `workflow`, `docs`, `chore` (7), `fix` (3).

## Pull Requests

PR titles follow the same scoped template as commit messages (`<scope>: <imperative summary>`),
e.g. `admin-ui: remove remaining legacy table and dialog remnants`,
`docker: build admin-ui inside production image`, `workflow: switch packer to single-binary output`.
The PR body should include, at minimum:

- summary
- why/context
- validation/tests run
- screenshots or manual verification steps for UI changes, when relevant

Do not mark work as ready if checks are unknown or failing. AI-assisted contributions
should include a co-author trailer when appropriate: `Co-Authored-By: opencode`.

## Triage labels

Labels are triage metadata, not a correctness determination or merge approval. Apply exactly one type label when opening an issue or pull request, subject to available labels:

- `bug` — something is broken or behaves incorrectly.
- `enhancement` — new functionality or a feature request.
- `documentation` — documentation-only change or doc issue.
- `question` — a usage or support question.
- `refactor` — internal change with no behavior change.
- `ci` — CI, build, or tooling change.

Automation-managed labels (do not apply by hand): `needs-info` and `duplicate`.
`confirmed` is not automation-managed — it records human or verification-owned confirmation
and must be applied only by maintainers or authorized triagers after relevant verification.
`priority-*` and other operational labels are maintainer-owned.

## Issues

Before opening an issue:

- Verify the problem against the current code/state and search existing issues if possible; keep it actionable and specific.
- Describe facts only; do not invent root cause — if root cause is only suspected, label it as a hypothesis.
- Include the category's relevant reproduction, expected behavior, affected component, environment/version, evidence, and constraints.

## Documentation Maintenance

Project structure changes over time: AGENTS.md and CONTRIBUTING.md are guidance, not a
replacement for checking the actual tree. When structure, commands, architecture, test
flow, contribution rules, release flow, or entry points change, update the relevant
Markdown file in the same PR; keep updates small and do not duplicate the full file tree.

## Releases

Every release must include meaningful release notes:

- Before publishing, check previous notes and review changes since the previous tag; use actual commits, merged PRs, linked issues, and final code changes.
- Do not invent features, fixes, impact, or breaking changes; call out user-visible, breaking, migration, compatibility, security, benchmark/default, and data-format changes when relevant.
- If a release is created automatically by GitHub Actions, its body must still be reviewed and updated before completion: a GitHub Release with an empty, placeholder, too-thin, or unreviewed auto-generated-only body is considered unfinished even if the tag and assets were created successfully.
- Before marking release work as done, verify the GitHub Release page contains the correct tag, title, body, and assets.

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: License & attribution (AGPL-3.0) -->
## License & attribution (AGPL-3.0)

See [LICENSE](LICENSE) and [NOTICE](NOTICE). The covered DeskForge distribution is
identified as **AGPL-3.0** because `server/` is AGPL-3.0 and is the strongest copyleft in
the covered bundle — this does not relicense separate or independent components, and the
inventory is incomplete with no signatures, attestations, or full-sovereignty claim
(canonical wording: [README.md](README.md) "License"). When you add new files derived from
upstream sources:

- **Keep** the upstream copyright header at the top of the file.
- **Append** your modification line below it, don't replace.
- New original files: standard AGPL header is fine; add yours.

See [NOTICE](NOTICE) for the full list of bundled components and their copyrights.

<!-- carry-forward: bashrusakh/DeskForge@9017b71340563bced7ea6571f271087ceb4d692c; section: Local development -->
## Local development

Working tree is **Windows-friendly** (LF/CRLF auto-conversion). If you want
explicit control, the repo doesn't ship a `.gitattributes` yet — feel free to
add one in a PR. To run the server stack, see "Dev Commands" above.
