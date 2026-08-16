# DeskForge Workflow Migration — Todo

**Plan:** `plans/deskforge-workflow-migration/plan.md`
**Status:** Workflow migration, external verification, and final PR #59 body synchronization complete.

## Current state

- [x] Branch renamed from `rustqs/min-test` to `rustqs/workflows`; exact head is `b11be6aef84aa110884bec8fa5fe827663b8ff01`.
- [x] Accepted workflow is `rustqs-windows.yml`; artifact is `rustqs-windows`. External migration commits are `4b77b40` and `b11be6a`.
- [x] Local references, tests, and documentation are migrated; local commit `460b424` was pushed to DeskForge PR #59.
- [x] PR #5 is open on `rustqs/workflows`.
- [x] Approved GPG key is registered; signed annotated `workflow-v1.0.0` targets `b11be6aef84aa110884bec8fa5fe827663b8ff01` and GitHub reports `verification=true`/`reason=valid`.
- [x] Active restored Ruleset `20901403` protects `refs/tags/workflow-*` with deletion and update protection; fetch/merge updates are disallowed and there are no bypass actors.
- [x] Provider-derived workflow-tag display and approval are verified; no new GUI architecture is needed. Final external verification is complete.

## Validation and publication

- [x] Current passing runs: Build `31889460990`, analyzer `31889459181`, and CodeQL `95023538774`.
- [x] Credentials/approval and publication gates were verified before tag publication and final external verification.
- [x] PR #59 body updated with the final commits, files, behavior, scope, and validation.

## Explicit unresolved TOCTOU

- [ ] The workflow-dispatch provider selector is not atomically bound to a commit; this remains explicitly excluded and unresolved.
