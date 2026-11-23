# UPSTREAM — provenance record for `libs/hbb_common/`

| Field | Value |
|---|---|
| Source | copy of [`bashrusakh/hbb_common`](https://github.com/bashrusakh/hbb_common) (mirror fork of [`rustdesk/hbb_common`](https://github.com/rustdesk/hbb_common)) |
| Fork point (pin) | `387603f47cbb15c0d3dc3d67ae3396d3eb707daf` (full SHA of record) |
| Fork point date | 2026-06-04 — upstream commit; the exact `rustdesk/rustdesk` 1.4.8 `libs/hbb_common` pin; present in the fork's history too |
| Recorded | 2026-11-23 |

This file is the **canonical pin-and-patch record** for the tracked copy.
Topology, fork/pin/sync rules, and the re-pin procedure:
[`docs/FORKS-AND-PINS.md`](../../docs/FORKS-AND-PINS.md). The mirror fork carries no
behavioral patches in library content (its `main` holds upstream history plus
non-behavioral infrastructure commits only — fork-content invariant in
`docs/FORKS-AND-PINS.md` §3); every DeskForge behavior change is in the patch set below.

## Local patch set (delta above the fork point)

| File | Change | Originating commit |
|---|---|---|
| `src/lib.rs` | Reproducible `gen_version()`: writes `$OUT_DIR/version.rs` (upstream writes `./src/version.rs` with a wall-clock date); `SOURCE_DATE_EPOCH` / `RUSTDESK_NON_REPRODUCIBLE_DEBUG` handling; default date `"unknown"`; 3 unit tests. **Must survive every re-pin** — coupled consumers `server/src/lib.rs:7` and `server/src/hbbr.rs:8` do `include!(concat!(env!("OUT_DIR"), "/version.rs"))`. | `bfc2a4c` (2026-08-21) |
| `src/config.rs` | External-endpoint neutralization: `RENDEZVOUS_SERVERS = &[]`, `LINK_DOCS_HOME` / `LINK_DOCS_X11_REQUIRED` / `LINK_HEADLESS_LINUX_SUPPORT` = `""`, `HELPER_URL` = empty map | `f74249f` (initial import) |
| `src/webrtc.rs` | `DEFAULT_ICE_SERVERS = []` (public STUN servers disabled; the `src/webrtc.rs:42` comment "STUN servers disabled - use self-hosted or configurable STUN" is kept) | not recorded |
| `src/tcp.rs`, `src/stream.rs`, `examples/webrtc.rs`, `src/webrtc.rs` | Formatting-only hunks (import reorders / wraps); semantically inert | not recorded |

Commit SHAs in this column are DeskForge repository commits where recorded; two patch
origins predate this record and are marked "not recorded" rather than guessed.

## Re-pinning

This file is DeskForge content and does not exist in the fork: save it before the
replacement `rm -rf`, and rewrite it with the new fork point (save/restore step in the
procedure below). Then re-apply exactly this patch set (the `src/lib.rs` ↔ `server/`
include coupling is the hard constraint). Procedure:
[`docs/FORKS-AND-PINS.md`](../../docs/FORKS-AND-PINS.md#5-updating-the-deskforge-copy-to-a-new-pin).
