# Phase 6 — PR6: workflow ownership/config

**Status:** ✅ `verified-with-notes`
**Scope:** workflow YAML ownership and mutable GitHub configuration simplification.

## Behavioral contract

The operator chooses a known version/capability. The catalog and immutable build snapshot provide the effective repository, workflow, and ref. Workflow files consume that validated identity; normal users do not choose a raw branch/ref or edit workflow payload fields. Global configuration may provide credentials, but must not redirect an existing build.

## Implemented boundary from PR5

- The existing workflow `version` input remains temporarily as a PR6 boundary and is overwritten from the resolved immutable identity immediately before dispatch.
- Fixed workflow mapping is application-owned while executable files remain in the configured rustdesk fork.
- The API contract exposes only Repo, PAT, and PayloadKey; legacy workflow/ref columns are retained but unread.
- The catalog resolves repository-specific release/source identity before persistence; dispatch overwrites version from that immutable identity and keeps the source `BuildRef` separate from workflow execution identity.
- GitHub's `workflow_dispatch` REST body requires `ref` to be a branch or tag selector; a raw SHA is not a valid selector. The persisted `WorkflowRef` is sent in the body, while resolved `WorkflowSHA` is used to check the workflow file and to reject any run whose `head_sha` differs.

## Required work and gates

- Define one canonical owner for repository, workflow filenames, execution ref, and version propagation; remove duplicate/hardcoded fallback authority without breaking legacy reads. ✅
- Remove DeskForge `github-build/rustqs-{windows-min-test,linux,android}.yml` duplicates; the configured rustdesk fork owns executable workflows. ✅
- Remove `rdgen/.github/workflows/sync-workflows.yml`; no workflow-file or PAT synchronization path remains. ✅
- Keep `rdgen/.github/workflows/bridge.yml` because vendored rdgen generator workflows reference it as their manual/reference workflow. ✅ deferred duplicate cleanup.
- Preserve the upstream-compatible bridge contract: no guessed `inputs.version` requirement and no checkout of the wrong repository.
- Historical/deferred note (superseded by `plans/deskforge-workflow-migration`): prior verification targeted `master` discovery, `rustqs/min-test` execution, and `rustqs/master-workflows` mirror ownership; live provider validation remains deferred.
- Keep provider payloads encrypted and keep workflow logs free of secrets.
- Do not re-enable Linux/Android UI choices merely because YAML exists; real CI runs and artifact paths are a later gate.

## Dependencies and acceptance

PR6 depends on PR5's stored version/build-ref and exact PR3 dispatch contract. Local acceptance is complete with focused Go/UI/static checks. Live GitHub dispatch, polling, artifact, and real Linux/Android workflow results remain unverified and are deferred to PR7/PR11.
