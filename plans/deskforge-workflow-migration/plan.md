# DeskForge Workflow Migration Plan

**Date:** 2026-08-16
**Status:** Workflow migration and external verification complete; final PR #59 body update pending.
**Scope:** Local contract reconciliation and recorded external workflow migration. Only these documentation artifacts are being updated; no source, workflow, secret, or external-repository changes are authorized here.

## Current state

- `rustqs/min-test` was renamed to `rustqs/workflows`. The accepted executable workflow is `rustqs-windows.yml`, producing artifact `rustqs-windows`, at exact head `b11be6aef84aa110884bec8fa5fe827663b8ff01`.
- The external migration was committed through the GitHub Contents API in `4b77b40` and `b11be6a`. Local DeskForge references, tests, and documentation are migrated; local commit `460b424` was pushed to DeskForge PR #59.
- PR #5 is open on `rustqs/workflows` after automatic retargeting.
- The approved GPG key is registered. Signed annotated tag `workflow-v1.0.0` targets RustDesk commit `b11be6aef84aa110884bec8fa5fe827663b8ff01`; GitHub reports `verification=true` and `reason=valid`.
- Active restored Repository Ruleset `20901403` (`DeskForge workflow tags`) protects `refs/tags/workflow-*` with deletion and update protection; `update_allows_fetch_and_merge=false`, `bypass_actors=[]`, and `current_user_can_bypass=never`.
- The provider-derived workflow-tag display and approval flow are verified; no new GUI architecture is needed. Final external verification is complete.

## Behavioral contract

- **Operator action:** select and approve a workflow revision for a build.
- **Value source:** the provider supplies the workflow tag and resolves its commit; the GUI displays the provider-derived tag and approval state.
- **Required behavior:** preserve provider-derived selection, immutable identity, approval, revalidation, and fail-closed handling. Do not add a raw/manual workflow-ref editor or new GUI architecture.

## Validation and publication

- [x] Current passing runs: PR Build `31889460990`, analyzer `31889459181`, and CodeQL `95023538774`.
- [x] Credentials/approval, remote, branch, base, commit range, changed files, and validation were checked before publication; publication gates and final external verification are complete.
- [ ] PR #59 body synchronization remains pending; perform the final update after this documentation commit.

## Explicit unresolved TOCTOU

- [ ] The workflow-dispatch provider selector is not atomically bound to a commit. This remains out of scope and unresolved; no secret-bearing dispatch or live-readiness claim is made.
