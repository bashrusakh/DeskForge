# DeskForge Workflow Migration Plan

**Date:** 2026-08-16
**Status:** Local and external migration verified; signing, tag, ruleset, and provider verification remain pending
**Scope:** Local DeskForge contract reconciliation plus recorded external workflow migration. Only these plan artifacts are being updated; no source, workflow, secret, or external-repository changes are authorized here.

## Verified current facts

- External branch rename `rustqs/min-test` → `rustqs/workflows` and workflow filename/artifact migration are verified at branch head `b11be6a`.
- The external migration was committed through the GitHub Contents API in commits `4b77b40` and `b11be6a`.
- Accepted workflow filename: `rustqs-windows.yml`.
- PR #5 was automatically retargeted to `rustqs/workflows` and remains open.
- Local DeskForge references, tests, and current documentation are migrated.
- Local commit `460b424` was pushed to DeskForge PR #59, and its PR body is synchronized.
- Fresh PR Build run `31888415635`, analyzer run `31888414373`, and CodeQL run `95021027686` passed.
- The admin GUI’s provider-derived workflow-tag display and approval flow are verified; no new GUI architecture is needed.
- No `workflow-v*` tags or repository rulesets currently exist.
- The local GPG key has no secret key. The current GitHub token lacks the `admin:gpg_key` scope.

## Remaining publication assumptions and gates

- Create an independent `workflow-v1.0.0` tag on exact post-PR#4 workflow migration commit `b11be6a`.
- The tag must be signed and annotated, with GitHub key verification completed before acceptance or push.
- Establish a repository ruleset for `refs/tags/workflow-v*` with tag update and deletion blocked and no bypass actors.
- Complete final provider/tag verification; no tag, ruleset, or provider acceptance claim is made yet.

## Behavioral contract

- **Natural operator action:** select and approve the workflow revision for a build.
- **Value source:** the provider supplies the workflow tag and resolves its commit; the existing GUI displays the provider-derived tag and approval state.
- **Existing pattern:** retain provider-derived selection, immutable identity, approval, revalidation, and fail-closed behavior.
- **Boundary:** no new raw/manual workflow-ref editor or GUI architecture is needed.

## Follow-up work packages

1. **Completed migration and validation**
   - Preserve the verified local references/tests/docs, external branch/file migration, commit `460b424` push, PR #59 body sync, and passed validation runs as the current baseline.
2. **Establish immutable tag policy**
   - Configure an approved signing key with the required GitHub access, then create and verify signed annotated `workflow-v1.0.0` on `b11be6a`.
   - Establish and verify the `refs/tags/workflow-v*` ruleset with update/deletion blocked and no bypass actors.
3. **Final provider and PR state**
   - Complete final provider/tag verification while keeping PR #5 open and accurately represented.
   - Do not claim release readiness or provider/tag acceptance before the pending gates pass.

## Explicit exclusion

The workflow-dispatch selector TOCTOU remains excluded. The provider’s branch/tag selector is not treated as atomically bound to a commit; this plan does not claim to solve that limitation or authorize secret-bearing dispatch/live readiness.

## Gates and blockers

- **GPG/signing key:** the local GPG configuration has no secret key, and the current GitHub token lacks `admin:gpg_key`; signed-tag work is blocked.
- **Signed tag:** no `workflow-v*` tag exists; signed annotated tag creation and GitHub verification remain pending.
- **Ruleset:** no repository ruleset exists; protected `refs/tags/workflow-v*` policy remains pending.
- **Provider verification:** final provider/tag verification remains pending.
- **PR #5:** remains open after automatic retargeting to `rustqs/workflows`.
- **Publication:** tag creation, ruleset mutation, push, or PR updates require credentials and explicit user approval.
