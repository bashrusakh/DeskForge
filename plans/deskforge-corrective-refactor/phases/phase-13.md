# Phase 13 — PR59 Review Remediation

**Status:** 🟡 `in-progress` — DeskForge local remediation is complete in local commits,
but remains unpushed to open/dirty DeskForge PR #59; no DeskForge merge, tag,
live-provider completion, or live artifact proof is recorded.
**Scope:** durable record of the local remediation in commits `da42521`, `9a1ee5e`, and
`d67b6e7` and of their publication and provider gates, including the current DeskForge
candidate, provider-workflow guard, and unresolved PR #59 publication boundary.
**Boundary:** this durable record is not evidence of publication or live readiness: it
does not establish a published schema, a provider-ready workflow, a production dispatch,
or cross-database support.

## Behavioral contract

An operator selects a provider-derived workflow tag; the provider resolves its workflow
SHA and supplies the workflow content. The operator does not type a SHA, marker, or
secret. Before approval, preparation, or secret-bearing dispatch, DeskForge checks the
provider-owned workflow at that resolved SHA and fails closed when the required identity
guard is absent.

## Current local schema candidate

- Published DeskForge schema remains `DatabaseVersion` **272**.
- The current local candidate is `DatabaseVersion` **283**. It adds
  `idx_custom_presets_user_id_name` on `(user_id, name)`.
- Before `AutoMigrate`, the schema-283 migration preflight stops when an existing
  `(user_id, name)` duplicate group is found. It does not auto-select a row or delete
  data. The earlier schema-282 workflow-approval migration is retained as
  history, not rewritten as the current target.
- Prior local validation records passing focused migration success and duplicate-preflight
  failure coverage, plus same-owner upsert, different-owner same-name, and concurrent
  same-owner/name coverage. A focused race check, `go vet`, `gofmt`, and
  `git diff --check` also passed before this documentation-only update.
- This migration evidence is SQLite-only. MySQL/PostgreSQL migration and read/write
  coverage remain unverified.

## Workflow identity guard

- The current local DeskForge candidate requires the exact provider-owned marker
  `# deskforge-workflow-identity-guard: v1` in workflow content resolved at the
  immutable workflow SHA before approval, preparation, or secret-bearing dispatch.
  Legacy unguarded tags fail closed.
- This guard does not atomically bind GitHub's selector-based `workflow_dispatch` to
  the resolved SHA and does not establish live-provider readiness.
- [RustDesk PR #7](https://github.com/bashrusakh/rustdesk/pull/7) is merged into
  `rustqs/workflows` at merge commit
  `ced31ae07f69c20119b88212b10d2eb2df651c97`; its prior source commit
  `6ef1cd7fe` is contained in that merge. The merged change adds the marker, requires
  outer and inner SHA checks in the bridge, and gates draft Linux/Android before
  secret-bearing jobs.

## Stored producer-manifest and canonical-output proof

- Local commit `d67b6e7` requires a stored v2 producer-manifest and exact
  canonical-output proof across publication, recovery, detail, download, reuse, and
  handoff boundaries.
- This local source remediation does not establish a live provider artifact or
  production-output proof.

## PR #59 publication boundary

- DeskForge PR #59 remains open and dirty remotely.
- The merge conflict is resolved only in local commit `2c38c87`.
- Later local commits `da42521`, `9a1ee5e`, and `d67b6e7` are not pushed. This phase
  does not claim that the remote PR contains the custom-preset, workflow-marker, or
  producer-manifest/canonical-output remediation.

## Remaining gates

- `workflow-v1.2.0` does not exist, and no newly signed provider-verified immutable
  protected workflow tag exists yet.
- A newly signed, provider-verified immutable protected tag is still required.
- Reapproval and reverification against the live provider are still required before
  any production secret-bearing dispatch.
- MySQL/PostgreSQL migration and read/write evidence, live workflow execution, live
  artifact proof, and proof that the provider selector/SHA boundary is atomic remain
  absent; the last item is not claimed by this remediation.
