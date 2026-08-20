# DeskForge Workflow Migration Plan

**Date:** 2026-08-16
**Status:** Prior workflow migration and external verification are complete; local two-layer workflow SHA-guard implementation and validation are complete, while live provider execution remains intentionally unperformed.
**Scope:** Add a provider-derived outer/inner workflow SHA guard to the DeskForge dispatch boundary and active RustDesk Windows/bridge workflows, with focused local tests and documentation. No secret, provider, commit, push, or live-run action is authorized here.

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

## In-progress two-layer workflow SHA guard

- [x] DeskForge overwrites `dispatchParams["workflow_sha"]` from provider-derived `identity.WorkflowSHA` immediately before DFP1 encryption and sends the same value as the public `workflow_dispatch` input. Normal dispatch parameters continue to reject caller-authored `workflow_sha`.
- [x] The active RustDesk Windows flow runs a first no-secret/no-decrypt/no-checkout guard that validates the public SHA format and exact equality with `github.sha`; bridge and build depend on it.
- [x] Bridge and direct Windows build paths validate the decrypted, MAC-authenticated inner SHA before payload export, checkout, source fetch, or build-tool input use.
- [x] Focused Go, workflow-contract, and YAML parsing checks provide local evidence only.
- [ ] Live provider execution remains intentionally excluded from this local work package.

## SHA-guard limits

- [x] This is defense in depth: it does not atomically bind GitHub's selector-based `workflow_dispatch` request or defend atomically against a malicious workflow file.
- [x] Verified annotated workflow tags and the active immutable no-bypass ruleset remain required protections; no claim of live provider execution or protection proof is made by local tests.

## Validation and publication

- [x] Current passing runs: PR Build `31889460990`, analyzer `31889459181`, and CodeQL `95023538774`.
- [x] Credentials/approval, remote, branch, base, commit range, changed files, and validation were checked before publication; publication gates and final external verification are complete.
- [x] PR #59 body synchronization is complete; the final update was applied after the documentation commit.

## Explicit unresolved TOCTOU

- [ ] The workflow-dispatch provider selector is not atomically bound to a commit. This remains out of scope and unresolved; no secret-bearing dispatch or live-readiness claim is made.
