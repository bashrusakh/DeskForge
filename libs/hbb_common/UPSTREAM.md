# UPSTREAM — provenance record for `libs/hbb_common/`

| Field | Value |
|---|---|
| Source | copy of [`bashrusakh/hbb_common`](https://github.com/bashrusakh/hbb_common) (mirror fork of [`rustdesk/hbb_common`](https://github.com/rustdesk/hbb_common)) |
| Fork point (pin) | `fae47e024dd83cef6aaecde0aa7d11731df11649` (full SHA of record) |
| Fork point date | 2026-10-28 — upstream commit "Merge pull request #619 from fufesou/fix/trusted-device-v2"; the `rustdesk/rustdesk` **1.5.1 / 1.5.3** `libs/hbb_common` submodule pin; an ancestor of the fork's `main`, fetched from `bashrusakh/hbb_common` by explicit SHA |
| Recorded | 2026-11-23 |
| History | original import pin `387603f47cbb15c0d3dc3d67ae3396d3eb707daf` (2026-06-04, the `rustdesk/rustdesk` 1.4.8 pin) — superseded 2026-11-23 by `fae47e024dd8` |

This file is the **canonical pin-and-patch record** for the tracked copy.
Topology, fork/pin/sync rules, and the re-pin procedure:
[`docs/FORKS-AND-PINS.md`](../../docs/FORKS-AND-PINS.md). The mirror fork carries no
behavioral patches in library content (its `main` holds upstream history plus
non-behavioral infrastructure commits only — fork-content invariant in
`docs/FORKS-AND-PINS.md` §3); every DeskForge behavior change is in the patch set below.

## Local patch set (delta above the pin)

Carry-over rule (user-ratified 2026-11-23): a patch is re-applied on a re-pin **only if
it is both still relevant against the new pin and still needed** — relevant = the target
code still has the condition the patch addresses; needed = it serves an established
requirement the pin does not already satisfy and it has no simpler owner. The verdicts
below were decided against `fae47e024dd8` on that rule.

### Kept patches

| File | Change | Why kept (relevant / needed) | Originating commit |
|---|---|---|---|
| `src/lib.rs` | Reproducible `gen_version()`: writes `$OUT_DIR/version.rs` (upstream writes `./src/version.rs` with a wall-clock date); `SOURCE_DATE_EPOCH` / `RUSTDESK_NON_REPRODUCIBLE_DEBUG` handling; default date `"unknown"`; `rerun-if-env-changed` lines; 3 unit tests (`build_date_tests`). **Must survive every re-pin** — coupled consumers `server/src/lib.rs:7` and `server/src/hbbr.rs:8` do `include!(concat!(env!("OUT_DIR"), "/version.rs"))` and `server/build.rs` calls `hbb_common::gen_version()`. | Relevant: the pin's `gen_version` still writes `./src/version.rs` with `chrono::Local::now()`. Needed: without it the `server/` build breaks (the `$OUT_DIR` include has no file) and build provenance hardening regresses (reproducible-builds requirement). | `bfc2a4c` (2026-08-21) |
| `src/config.rs` | `RENDEZVOUS_SERVERS = &[]` (was `&["rs-ny.rustdesk.com"]`) | Relevant: the pin still bakes in `rs-ny.rustdesk.com`, consumed by the rendezvous-server fallback (`config.rs`). Needed: no baked-in external endpoints/hosts in shipped code; the constant is the owning site. | `f74249f` (initial import) |
| `src/config.rs` | `LINK_DOCS_HOME = ""` (was `https://rustdesk.com/docs/en/`) | Relevant: external URL still present. Needed: no baked-in external hosts in shipped code (always-compiled `config` module). | `f74249f` (initial import) |
| `src/config.rs` | `LINK_DOCS_X11_REQUIRED = ""` (was `https://rustdesk.com/docs/en/manual/linux/#x11-required`) | Same as above. | `f74249f` (initial import) |
| `src/websocket.rs` (test only) | `tests::test_check_ws` final block: the fallback check supplies its own `rendezvous-servers` option entry (bumping/restoring the config `serial` that gates that option) instead of reading the baked `RENDEZVOUS_SERVERS` default; the `check_ws(&fallback)` assertion itself is unchanged. | Relevant: the pin's final assertion derives `fallback` from `Config::get_rendezvous_server()`, which with `RENDEZVOUS_SERVERS = &[]` yields `":21116"`; `websocket_port()` then matches the empty host against an empty `own_servers` entry (`:8443` appended) and the pin's non-empty-fallback assumption fails. Needed: keeps the suite green under the established no-external-endpoints requirement while still verifying the same `websocket_port()` host-matching rule (a rendezvous fallback outside the client's configured servers keeps the default port). Test-only — the `RENDEZVOUS_SERVERS` product patch above stays intact. | `DeskForge, 2026-11-23` |

Known interaction (resolved 2026-11-23): the `RENDEZVOUS_SERVERS` neutralization makes
`Config::get_rendezvous_server()` fall through to an empty host, and the pin's upstream
test `websocket::tests::test_check_ws` (final assertion — "with no rendezvous server set,
the public fallback is not one of ours") then fails: with `custom-rendezvous-server`
empty, `websocket_port()`'s own-server match treats host `""` as ours (`:8443` appended)
while the test formats the empty fallback host without a port. The test encodes upstream's
non-empty-fallback assumption; it passes on the pristine pin and with only this hunk
restored (counter-probe 2026-11-23). Owner decision: the neutralization stands (it is
established product behavior; the upstream test's assumption does not define the product
contract), so the **test** is adapted instead — the kept-patch `src/websocket.rs` (test
only) row above makes the final
check supply an explicit `rendezvous-servers` fallback list, while the
`check_ws`/`websocket_port` assertion stays byte-identical, so the host-matching behavior
it verifies is unchanged. CHANGELOG entry for the same date.

### Dropped patches (recorded per the carry-over rule)

| Old item | Verdict | Justification |
|---|---|---|
| `src/config.rs` `LINK_HEADLESS_LINUX_SUPPORT = ""` | Dropped — not relevant | The constant no longer exists at `fae47e024dd8` (upstream removed it and its `HELPER_URL` entry); nothing to neutralize. |
| `src/config.rs` `HELPER_URL = HashMap::from([])` | Dropped — not needed | Once `LINK_DOCS_HOME` / `LINK_DOCS_X11_REQUIRED` are zeroed, the map holds no external endpoints; keeping its key entries preserves key-lookup behavior for consumers (an empty map would turn `[key]`-style lookups into panics). The constants are the simpler owner. |
| `src/webrtc.rs` `DEFAULT_ICE_SERVERS = []` | Dropped — not needed in our build shape | `src/webrtc.rs` is `#[cfg(feature = "webrtc")]` and `server/` takes `hbb_common` with default features only, so the module never compiles in any DeskForge build/test command — no public STUN server can reach our binaries or configuration. Neutralizing it in the new shape would also conflict with the pin's feature-gated `test_webrtc_ice_server_list` (asserts `DEFAULT_ICE_SERVERS[0]`), which must not be weakened. Re-evaluate only if the `webrtc` feature is ever enabled. |
| Formatting-only hunks (`src/tcp.rs`, `src/stream.rs`, `examples/webrtc.rs`, `src/webrtc.rs` test wrap, `src/lib.rs` import reorders / `init_log` wraps) | Dropped — not needed | Semantically inert; keep the delta minimal. Caveat: the CI `cargo fmt --check` gate on `libs/hbb_common` could not be run in the verification environment (rustfmt absent) — if CI's pinned rustfmt flags the pin's or the patch's formatting, that is a formatting-only follow-up. |

Commit SHAs in the origin column are DeskForge repository commits where recorded; two
origins predate this record and are marked "not recorded" where applicable.

## Re-pinning

This file is DeskForge content and does not exist in the fork: save it before the
replacement `rm -rf`, and rewrite it with the new pin (save/restore step in the
procedure below), re-deciding every patch row on the carry-over rule above. Then re-apply
exactly the kept rows (the `src/lib.rs` ↔ `server/` include coupling is the hard
constraint). Procedure:
[`docs/FORKS-AND-PINS.md`](../../docs/FORKS-AND-PINS.md#5-updating-the-deskforge-copy-to-a-new-pin).

Note: the repository root `.gitignore` rules `data/` and `*.pem` swallow the pin's
`tests/data/websocket-{cert,key}.pem` test fixtures (upstream-committed, public), so
staging the re-pin needs `git add -f libs/hbb_common/tests/data/…` for those two files
or the committed tree silently loses them.
