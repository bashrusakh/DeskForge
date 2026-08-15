# DeskForge Workflow Migration — Todo

**Plan:** `plans/deskforge-workflow-migration/plan.md`
**Status:** Local and external migration verified; signing, tag, ruleset, and provider verification remain pending.

## Verified current state

- [x] External branch rename `rustqs/min-test` → `rustqs/workflows` verified at head `b11be6a`.
- [x] External workflow filename/artifact migration verified in Contents API commits `4b77b40` and `b11be6a`.
- [x] Accepted workflow filename is `rustqs-windows.yml`.
- [x] PR #5 automatically retargeted to `rustqs/workflows` and remains open.
- [x] Local DeskForge references, tests, and current documentation are migrated.
- [x] Local commit `460b424` pushed to DeskForge PR #59.
- [x] PR #59 body synchronized.
- [x] PR Build `31888415635`, analyzer `31888414373`, and CodeQL `95021027686` passed.
- [x] Admin GUI provider-derived workflow-tag display and approval flow are verified; no new GUI architecture is needed.
- [x] No `workflow-v*` tags exist.
- [x] No repository rulesets exist.

## Remaining work

- [ ] Configure an approved GPG signing key; local GPG has no secret key and the current GitHub token lacks `admin:gpg_key`.
- [ ] Create independent signed annotated `workflow-v1.0.0` on exact post-PR#4 migration commit `b11be6a`.
- [ ] Verify the tag through GitHub key status; fail closed on unsigned, unverified, lightweight, or ambiguous tags.
- [ ] Establish and verify a repository ruleset matching `refs/tags/workflow-v*`.
- [ ] Verify tag update is blocked, deletion is blocked, and no bypass actors are configured.
- [ ] Complete final provider/tag verification.
- [x] Preserve the existing provider-derived GUI selection/approval contract; no new GUI architecture or raw workflow-ref editor is needed.
- [ ] Keep the workflow-dispatch selector TOCTOU explicitly excluded and unresolved.

## Validation and synchronization gates

- [x] Run focused reference/static checks, relevant DeskForge checks, documentation consistency checks, and `git diff --check` after implementation.
- [x] Push local commit `460b424` to DeskForge PR #59.
- [x] Synchronize the DeskForge PR #59 body with actual commits, files, behavior, scope, and validation.
- [ ] Before tag, ruleset, or further PR update, verify credentials/approval, remote, branch, base, commit range, changed files, and validation results.
- [ ] Do not create/publish a tag, mutate rulesets, or update a PR without explicit user approval.

## Open blockers

- [ ] **GPG/signing key:** local GPG has no secret key; the current GitHub token lacks `admin:gpg_key` scope.
- [ ] **Signed tag:** no `workflow-v*` tag exists; signed annotated tag creation and GitHub verification are pending.
- [ ] **Ruleset:** no repository ruleset exists; protected `refs/tags/workflow-v*` policy is pending.
- [ ] **Provider verification:** final provider/tag verification remains pending.
- [ ] **Publication approval/credentials:** required for tag creation, ruleset mutation, push, or PR updates.
- [ ] **TOCTOU:** provider-side atomic selector-to-commit binding remains excluded and must not be claimed closed.
