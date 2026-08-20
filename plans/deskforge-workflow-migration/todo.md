# DeskForge Workflow Migration — Todo

**Plan:** `plans/deskforge-workflow-migration/plan.md`
**Status:** Prior workflow migration and external verification are complete; local two-layer workflow SHA-guard implementation and validation are complete, while live provider execution remains intentionally unperformed.

## Current state

- [x] Branch renamed from `rustqs/min-test` to `rustqs/workflows`; exact head is `b11be6aef84aa110884bec8fa5fe827663b8ff01`.
- [x] Accepted workflow is `rustqs-windows.yml`; artifact is `rustqs-windows`. External migration commits are `4b77b40` and `b11be6a`.
- [x] Local references, tests, and documentation are migrated; local commit `460b424` was pushed to DeskForge PR #59.
- [x] PR #5 is open on `rustqs/workflows`.
- [x] Approved GPG key is registered; signed annotated `workflow-v1.0.0` targets `b11be6aef84aa110884bec8fa5fe827663b8ff01` and GitHub reports `verification=true`/`reason=valid`.
- [x] Active restored Ruleset `20901403` protects `refs/tags/workflow-*` with deletion and update protection; fetch/merge updates are disallowed and there are no bypass actors.
- [x] Provider-derived workflow-tag display and approval are verified; no new GUI architecture is needed. Final external verification is complete.

## In-progress two-layer workflow SHA guard

- [x] DeskForge dispatch overwrites the encrypted inner `workflow_sha` from `identity.WorkflowSHA`, sends the same provider-derived public outer input, and rejects caller-authored raw SHA parameters.
- [x] The active RustDesk Windows flow runs a no-secret/no-decrypt/no-checkout outer SHA guard before bridge/build jobs, and bridge/direct build recheck the authenticated inner value before exports, checkout, source fetch, or build use.
- [x] Focused Go, workflow contract, and YAML parser checks provide local evidence only; no live provider run, secret operation, commit, or push is part of this work.
- [ ] Live provider execution remains intentionally excluded from this local work package.

## SHA-guard limits

- [x] The guard is defense in depth, not an atomic defense against a malicious workflow file or GitHub's selector-based dispatch TOCTOU.
- [x] Verified annotated workflow tags and the immutable no-bypass ruleset remain required protections and are not replaced by this guard.

## Validation and publication

- [x] Current passing runs: Build `31889460990`, analyzer `31889459181`, and CodeQL `95023538774`.
- [x] Credentials/approval and publication gates were verified before tag publication and final external verification.
- [x] PR #59 body updated with the final commits, files, behavior, scope, and validation.

## Explicit unresolved TOCTOU

- [ ] The workflow-dispatch provider selector is not atomically bound to a commit; this remains explicitly excluded and unresolved.
