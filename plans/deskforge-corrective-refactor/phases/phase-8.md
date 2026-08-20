# Phase 8 — PR8: Secret Persistence

**Status:** ✅ `verified-with-notes`
**Scope:** encrypted persistence, lifecycle, access boundaries, and redaction for provider credentials, payload keys, and other build secrets. No raw secret exposure or unrelated auth redesign is part of this phase. PR8 and PR9 are verified-with-notes; PR10 and PR11 remain `in-progress`, while PR12 is `verified-with-notes` for docs only.

## Behavioral contract

An administrator enters a secret through the existing typed configuration path (`Repo`, PAT, and `PayloadKey`). The service encrypts sensitive values before persistence, decrypts only at the provider boundary, and returns redacted state to normal UI/API consumers. A missing, invalid, or undecryptable secret fails closed without logging the secret, plaintext payload, or encrypted payload. The one-time generated payload-key display is intentional; it is not a persistent secret readback.

The canonical API deployment environment key is `SECRET_ENCRYPTION_KEY` for AES-GCM secrets at rest. The GitHub Actions secret is `WORKFLOW_PAYLOAD_KEY` and must match the configured `PayloadKey`; `WORKFLOW_PAYLOAD_KEY` is not reused as the API at-rest key. `RUSTDESK_API_JWT_KEY` remains the JWT key and is not a substitute for either secret-persistence key.

## Required contract

- Secret-at-rest protection uses the canonical `SECRET_ENCRYPTION_KEY` source with an explicit format and migration behavior. New non-empty secret writes fail closed when the current key is missing; legacy plaintext reads remain supported and re-save encrypts them.
- Read responses, errors, logs, artifacts, and workflow output never expose plaintext secrets, tokens, payload keys, or ciphertext unnecessarily. Normal UI/API responses expose typed state such as presence flags, not raw values.
- Key rotation is unsupported and the current key is required; restart and legacy-row behavior are explicit, and existing immutable builds are not redirected.
- `PayloadKey` remains the typed administrative value corresponding to GitHub Secret `WORKFLOW_PAYLOAD_KEY`; provider synchronization remains a typed administrative action, not a raw secret editor or callback path.
- Persistence and provider calls use typed errors and bounded responses, with safe DTOs and explicit raw `json` tags; malformed ciphertext prefixes are rejected. Generated payload keys use `base64.RawURLEncoding`.
- Docker passes `SECRET_ENCRYPTION_KEY` through to the API runtime without logging or transforming the value.
- CustomBuild/CustomPreset save hooks enforce the canonical typed custom-field allowlist in addition
  to service normalization. Unknown neutral fields such as `cookie`, `jwt`, or `signing_key` are
  rejected before persistence; valid public fields retain their existing non-secret behavior,
  canonical secret fields are encrypted, legacy rows remain readable internally, and safe DTOs stay
  redacted.

## Verification notes

- Focused secret-persistence tests, race checks, `go vet`, and `npm run build` passed locally.
- Evidence is SQLite-only; MySQL/PostgreSQL coverage is not established. Broad Go vet/test checks pass with `GOWORK=off` after test-only cache diagnostics and opt-in Redis test changes. Redis integration/benchmarks remain opt-in through `DESKFORGE_TEST_REDIS_ADDR`, and no live Redis run is recorded.

## Gates

- Inventory every secret field, persistence path, encryption/decryption caller, log/error path, provider payload path, and test fixture before changing storage.
- Verify encryption, exact key loading, key-missing, malformed-ciphertext, legacy-row, and concurrent-update behavior with focused tests; key rotation remains an explicit unsupported limitation and the current key is required.
- Confirm provider payloads are encrypted before dispatch and that redaction covers transport errors and bounded response bodies without leaking PATs, payload keys, plaintext payloads, or ciphertext.
- Confirm safe UI/API views preserve typed configuration and never return secret values; verify restart, current-key requirements, and existing immutable-build behavior under key loss.
- Document operational key-loss and migration limits without inventing recovery or backup guarantees.
- This phase is verified-with-notes from focused evidence and the project-required Go race/vet/UI checks; the limitations above remain binding and do not imply cross-DB or full-suite coverage.

## Dependencies / remaining evidence

PR8 depends on PR3 typed REST errors, PR4 immutable provenance, PR6 configuration ownership, and the verified PR7 delivery boundary. Secret persistence implementation and verification are recorded as `verified-with-notes`; no new secret storage or migration was performed by this plan restoration. PR9 reproducibility is verified-with-notes; PR10 and PR11 remain `in-progress`, while PR12 documentation reconciliation is `verified-with-notes`.
