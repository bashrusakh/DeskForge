# DeskForge Workflow Migration — Todo

**Plan:** `plans/deskforge-workflow-migration/plan.md`
**Status:** Workflow migration verified; signing, ruleset, and PR publication remain pending.

## Verified current state

- [x] Branch rename `rustqs/min-test` → `rustqs/workflows` succeeded.
- [x] External workflow filename/artifact migration was committed through the GitHub Contents API in `4b77b40` and `b11be6a`; branch head is `b11be6a`.
- [x] Accepted workflow filename is `rustqs-windows.yml`.
- [x] PR #5 base automatically updated to `rustqs/workflows`; PR #5 remains open and mergeability is recalculating.
- [x] Local DeskForge references, tests, and current documentation are migrated.
- [x] Admin GUI provider-derived workflow-tag display and approval flow are verified; no new GUI architecture is needed.
- [x] No `workflow-v*` tags exist.
- [x] No repository rulesets exist.
- [x] No local GPG secret key or `signingkey` is configured; GitHub GPG API access requires `admin:gpg_key`.

## Remaining work

- [ ] Create independent signed annotated `workflow-v1.0.0` on exact post-PR#4 migration commit `b11be6a`.
- [ ] Verify the tag through GitHub key status; fail closed on unsigned, unverified, lightweight, or ambiguous tags.
- [ ] Establish and verify a repository ruleset matching `refs/tags/workflow-v*`.
- [ ] Verify tag update is blocked, deletion is blocked, and no bypass actors are configured.
- [x] Run the concise documentation audit for filenames, branches, workflow ownership, tag policy, GUI wording, and stale readiness claims.
- [x] Preserve the existing provider-derived GUI selection/approval contract; no new GUI architecture or raw workflow-ref editor is needed.
- [ ] Re-check PR #5 after mergeability recalculates and complete publication/coordination gates.
- [ ] Keep the workflow-dispatch selector TOCTOU explicitly excluded and unresolved.

## Validation and synchronization gates

- [x] Run focused reference/static checks, relevant DeskForge checks, documentation consistency checks, and `git diff --check` after implementation.
- [ ] Before tag, push, or PR update, verify credentials/approval, remote, branch, base, commit range, changed files, and validation results.
- [ ] Synchronize PR #5 title/body with actual commits, files, behavior, scope, and validation before publication.
- [ ] Do not create/publish a tag, mutate rulesets, push, or update a PR without explicit user approval.

## Open blockers

- [ ] **Signed tag:** no `workflow-v*` tag exists; signed annotated tag creation and GitHub verification are pending.
- [ ] **Ruleset:** no repository ruleset exists; protected `refs/tags/workflow-v*` policy is pending.
- [ ] **GPG:** no local secret key/signingkey is configured; GitHub GPG API requires `admin:gpg_key` scope.
- [ ] **PR #5:** remains open while mergeability recalculates; publication and merge coordination remain pending.
- [ ] **Publication approval/credentials:** required for tag creation, ruleset mutation, push, or PR updates.
- [ ] **TOCTOU:** provider-side atomic selector-to-commit binding remains excluded and must not be claimed closed.
