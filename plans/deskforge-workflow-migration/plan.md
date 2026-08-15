# DeskForge Workflow Migration Plan

**Date:** 2026-08-16
**Status:** Workflow migration verified; signing, ruleset, and PR publication remain pending
**Scope:** Local DeskForge contract reconciliation plus recorded external workflow migration. Only these plan artifacts are being updated; no source, workflow, secret, or Git-history changes are authorized here.

## Verified current facts

- Workflow files existed on `bashrusakh/rustdesk` branch `rustqs/min-test` after merged PR #4; the branch rename to `rustqs/workflows` has now succeeded.
- External workflow filename/artifact migration was committed through the GitHub Contents API in commits `4b77b40` and `b11be6a`; the renamed branch now heads at `b11be6a`.
- Accepted workflow filename: `rustqs-windows.yml`.
- Open PR #5 automatically updated its base to `rustqs/workflows` and remains open; mergeability is currently recalculating.
- Local DeskForge references, tests, and current documentation are migrated.
- The admin GUI’s provider-derived workflow-tag display and approval flow are verified; no new GUI architecture is needed.
- No `workflow-v*` tags or repository rulesets currently exist.
- No local GPG secret key or `signingkey` is configured. The GitHub GPG API requires the `admin:gpg_key` scope.

## Remaining publication assumptions and gates

- Create an independent `workflow-v1.0.0` tag on the exact post-PR#4 workflow migration commit at branch head `b11be6a`; do not infer the target from an unrelated ref.
- The tag must be signed and annotated. GitHub key verification is required before acceptance or push.
- Establish a repository ruleset for `refs/tags/workflow-v*` with tag update and deletion blocked and no bypass actors. This is a required target state, not current evidence.
- Keep PR #5 open until mergeability recalculates and publication/coordination gates are explicitly cleared.

## Behavioral contract

- **Natural operator action:** select and approve the workflow revision for a build.
- **Value source:** the provider supplies the workflow tag and resolves its commit; the existing GUI displays the provider-derived tag and approval state.
- **Existing pattern:** retain provider-derived selection, immutable identity, approval, revalidation, and fail-closed behavior.
- **Boundary:** no new raw/manual workflow-ref editor or GUI architecture is needed.

## Follow-up work packages

1. **Record completed migration**
   - Preserve the verified branch rename, Contents API commits, branch head, local references/tests/docs, and existing GUI behavior as the current baseline.
2. **Establish immutable tag policy**
   - Obtain an approved GPG signing key with the required GitHub access, then create and verify the signed annotated `workflow-v1.0.0` tag on `b11be6a`.
   - Establish and verify the `refs/tags/workflow-v*` ruleset with update/deletion blocked and no bypass actors.
3. **Synchronize PR state and validate**
   - Re-check PR #5 after mergeability recalculates; keep its title/body aligned with actual commits, files, scope, and validation before publication.
   - Run focused reference/static checks, relevant DeskForge checks, documentation consistency checks, and `git diff --check`.

## Explicit exclusion

The workflow-dispatch selector TOCTOU remains excluded. The provider’s branch/tag selector is not treated as atomically bound to a commit; this plan does not claim to solve that limitation or authorize secret-bearing dispatch/live readiness.

## Gates and blockers

- **Signed tag:** pending an approved signing key and GitHub verification; no `workflow-v*` tag exists.
- **Ruleset:** pending creation and verification; no repository ruleset exists.
- **GPG access:** no local key is configured, and GitHub’s GPG API requires `admin:gpg_key` scope.
- **PR #5:** remains open while mergeability recalculates; PR publication/merge coordination is pending.
- **Publication:** tag creation, ruleset mutation, push, or PR updates require credentials and explicit user approval.
