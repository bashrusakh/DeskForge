# Phase 7 — PR7: Streaming Artifact

**Status:** ✅ `verified-with-notes`
**Scope:** GitHub HTTP-body streaming of the exact build artifact, truthful lifecycle/status handling, bounded transfer, publication proof, and cleanup. Linux/Android capability enablement is a later PR11 boundary. The user contract specifies GitHub HTTP body flow; a runner callback endpoint is explicitly not part of this phase.

## Behavioral contract

The operator starts a known Windows build and can observe whether the provider run is pending, building, downloading, extracting, complete, or failed. GitHub supplies the provider-derived artifact through its HTTP response body; the API streams it to bounded temporary storage, validates and publishes only the exact immutable build/artifact result, and exposes download only after publication proof. An interrupted, mismatched, expired, partial, or over-limit stream fails closed rather than producing a phantom successful build or an unbounded local queue.

A reviewer request for a runner callback is out of scope: it would change the delivery contract, while the user contract explicitly requires the GitHub HTTP body flow.

## Recorded verification

- **Bounded GitHub-body stream:** the exact GitHub artifact response body is streamed to a server-owned temporary `.part` file with a `256 MiB` compressed-response limit, Content-Length/short-body checks, ZIP validation, and atomic `.part` → `.zip` rename. The ZIP is never buffered as a whole in memory.
- **Exact identity:** run ID, artifact ID, artifact name, immutable provenance, and final download identity are checked together. The implementation does not choose a sole artifact, infer a path, or guess a run from a list.
- **Archive limits:** decompressed provider ZIP content is bounded to `4096` entries, `512 MiB` per file, `1 GiB` aggregate, and `1000:1` compression ratio. The public/generated download archive is bounded to `4096` files, `512 MiB` per source file, `1 GiB` source aggregate, and `512 MiB` output.
- **Cleanup and staging:** failed/partial/oversized/cancelled downloads remove temporary files; artifact and build-output staging cleanup has bounded retries and TTL sweeps, while active downloads/builds are protected. Final output is not swept as stale temporary data.
- **Nested cleanup:** `SweepBuildOutputTemps` scans only bounded direct children of inactive numeric
  per-build output directories for interrupted `.part`/download/archive files. Active build IDs,
  fresh files, protected current download archives, symlinks, and published output remain untouched.
- **Digest/publication proof:** the service computes a deterministic SHA-256 manifest over immutable build/run/artifact identity plus canonical output names, sizes, and contents. A write-once publication marker and digest are required, and `done` revalidates current output against that proof.
- **Atomic/idempotent output:** filesystem publication uses temporary paths and rename boundaries; repeating publication with the same exact identity and digest is a no-op, while changed identity/content or incomplete proof fails closed.
- **Lifecycle and guards:** statuses are `pending`, `building`, `downloading`, `extracting`, `done`, and `failed`. Exact run/artifact/status guards reject stale, duplicate, mismatched, expired, interrupted, partial, and zero-row persistence updates; a completion transition requires artifact identity and publication proof.
- **Capability and UI:** production build capability is Windows-only and Linux/Android fail closed until PR11. The admin UI polls active lifecycle states, stops at terminal states, and uses mounted/request-generation guards so stale responses cannot overwrite current state; download is shown only for `done`.
- **Focused checks:** focused Go tests, `go test -race`, `go vet`, and `npm run build` passed locally. Existing UI build warnings are not treated as new failures.

## Explicitly unverified / out of scope

- Broad Go vet/test checks pass with `GOWORK=off` after test-only cache diagnostics and opt-in Redis test changes. Redis integration tests and benchmarks run only when `DESKFORGE_TEST_REDIS_ADDR` is configured; no live Redis run is recorded. Live GitHub/provider dispatch or workflow execution and cross-database MySQL/PostgreSQL evidence remain unverified.
- Crash recovery across process failure, distributed lease coordination, and durable outbox/scheduler semantics remain out of scope; the poll ownership guard is process-local and restart recovery is bounded by the existing resume path.
- Browser/manual UI verification remains a gap; the recorded UI evidence is code-level lifecycle checks plus the focused UI build.
- No runner callback contract, callback endpoint, or callback-specific evidence is required for PR7.

## Dependencies / remaining evidence

PR7 depends on PR3 exact REST errors, PR4 provenance, PR5 immutable version/build-ref, and PR6 workflow ownership/config. Its focused local evidence is recorded above with notes; PR8 and PR9 are verified-with-notes. PR10 and PR11 remain `in-progress`, while PR12 is `verified-with-notes` for documentation reconciliation. Linux/Android workflow capability remains intentionally retained as a PR11 gate.
