# DeskForge Workflow Migration — Todo

**Plan:** `plans/deskforge-workflow-migration/plan.md`
**Status:** Migration and ruleset verified; signing, tag, and final provider verification remain pending.

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
- [x] Ruleset `20889185` (`DeskForge workflow tags`) is active for target `tag`, condition `refs/tags/workflow-*`.
- [x] Ruleset rules are `deletion` and `update` with `update_allows_fetch_and_merge=false`, `bypass_actors=[]`, and `current_user_can_bypass=never`.

## Remaining work

- [ ] Configure an approved GPG signing key; local GPG has no secret key and the current GitHub token lacks `admin:gpg_key`.
- [ ] Create independent signed annotated `workflow-v1.0.0` on exact post-PR#4 migration commit `b11be6a`.
- [ ] Verify the tag through GitHub key status; fail closed on unsigned, unverified, lightweight, or ambiguous tags.
- [ ] Complete final tag/provider verification.
- [x] Preserve the existing provider-derived GUI selection/approval contract; no new GUI architecture or raw workflow-ref editor is needed.
- [ ] Keep the workflow-dispatch selector TOCTOU explicitly excluded and unresolved.

## Validation and synchronization gates

- [x] Run focused reference/static checks, relevant DeskForge checks, documentation consistency checks, and `git diff --check` after implementation.
- [x] Push local commit `460b424` to DeskForge PR #59.
- [x] Synchronize the DeskForge PR #59 body with actual commits, files, behavior, scope, and validation.
- [ ] Before tag or further PR update, verify credentials/approval, remote, branch, base, commit range, changed files, and validation results.
- [ ] Do not create/publish a tag, mutate policy, or update a PR without explicit user approval.

## Open blockers

- [ ] **GPG/signing key:** local GPG has no secret key; the current GitHub token lacks `admin:gpg_key` scope.
- [ ] **Signed tag:** no `workflow-v*` tag exists; signed annotated tag creation and GitHub verification are pending.
- [ ] **Final provider verification:** tag/provider verification remains pending.
- [ ] **PR #5:** remains open after automatic retargeting to `rustqs/workflows`.
- [ ] **Publication approval/credentials:** required for tag creation, push, or PR updates.
- [ ] **TOCTOU:** provider-side atomic selector-to-commit binding remains excluded and must not be claimed closed.
