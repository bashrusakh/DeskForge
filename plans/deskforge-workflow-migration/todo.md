# DeskForge Workflow Migration — Todo

**Plan:** `plans/deskforge-workflow-migration/plan.md`
**Status:** Prior workflow migration and local two-layer workflow SHA-guard implementation are complete. Two live branch runs validate guard sequencing, but production artifact output and readiness remain unverified.

## Current state

- [x] Branch renamed from `rustqs/min-test` to `rustqs/workflows`; exact head is `b11be6aef84aa110884bec8fa5fe827663b8ff01`.
- [x] Accepted workflow is `rustqs-windows.yml`; artifact is `rustqs-windows`. External migration commits are `4b77b40` and `b11be6a`.
- [x] Local references, tests, and documentation are migrated; local commit `460b424` was pushed to DeskForge PR #59.
- [x] PR #5 is open on `rustqs/workflows`.
- [x] Approved GPG key is registered; signed annotated `workflow-v1.0.0` targets `b11be6aef84aa110884bec8fa5fe827663b8ff01` and GitHub reports `verification=true`/`reason=valid`.
- [x] Active restored Ruleset `20901403` protects `refs/tags/workflow-*` with deletion and update protection; fetch/merge updates are disallowed and there are no bypass actors.
- [x] Provider-derived workflow-tag display and approval are verified; no new GUI architecture is needed. Production use still requires live tag/ruleset re-verification.
- [x] Published RustDesk branch `fix/workflow-sha-guard` is at commit `8ad23a826d5df1e311a727e507861c0c6bc35c76`.

## In-progress two-layer workflow SHA guard

- [x] DeskForge dispatch overwrites the encrypted inner `workflow_sha` from `identity.WorkflowSHA`, sends the same provider-derived public outer input, and rejects caller-authored raw SHA parameters.
- [x] The active RustDesk Windows flow runs a no-secret/no-decrypt/no-checkout outer SHA guard before bridge/build jobs, and bridge/direct build recheck the authenticated inner value before exports, checkout, source fetch, or build use.
- [x] Focused Go, workflow contract, and YAML parser checks provide local evidence only.
- [x] Live mismatch run [32333446480](https://github.com/bashrusakh/rustdesk/actions/runs/32333446480) failed in `verify_workflow_identity` with `outer workflow_sha does not match github.sha`; bridge, build, and topmost were skipped, and no secret-bearing job ran.
- [x] Live matching run [32334155671](https://github.com/bashrusakh/rustdesk/actions/runs/32334155671) passed the outer guard and reached the intentional post-inner validation `encrypted payload is missing source_sha`; bridge checkout, build, and topmost were skipped.
- [x] These branch runs prove outer/inner guard sequencing only, not production artifact output.

## SHA-guard limits

- [x] The guard is defense in depth, not an atomic defense against a malicious workflow file or GitHub's selector-based dispatch TOCTOU.
- [x] Verified annotated workflow tags and the immutable no-bypass ruleset remain required protections and are not replaced by this guard; the branch runs do not prove a newly signed/tag-approved production selector or live ruleset enforcement.
- [ ] Before production use, run a newly signed/tag-approved selector and re-verify tags and rulesets live.

## Validation and publication

- [x] Current passing runs: Build `31889460990`, analyzer `31889459181`, and CodeQL `95023538774`.
- [x] Credentials/approval and publication gates were verified before prior tag publication.
- [x] PR #59 body updated with the final commits, files, behavior, scope, and validation.

## Explicit unresolved TOCTOU

- [ ] The workflow-dispatch provider selector is not atomically bound to a commit; this remains explicitly excluded and unresolved, and the recorded branch runs are not production readiness evidence.
