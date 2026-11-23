# Forks, Pins, and Sync — fork topology and pin bookkeeping

Canonical definition of DeskForge's fork/pin/sync model for the RustDesk shared
library (`hbb_common`) and the client sources: topology, rules, and procedures live
here, and [`libs/hbb_common/UPSTREAM.md`](../libs/hbb_common/UPSTREAM.md) is the
canonical pin-and-patch record for the tracked copy. Other documents cross-link and
may carry short operator summaries of the patch set (PLAN.md §7, §5 below) instead of
restating the record.

## 1. Topology

### `hbb_common` (shared library)

```
rustdesk/hbb_common                        — upstream (standalone repo since 2025-01-20; no version tags)
└── bashrusakh/hbb_common                  — mirror fork of upstream (sync mandate §3; no behavioral patches)
    └── DeskForge libs/hbb_common/         — copy of the fork at a recorded pin + local patch set + UPSTREAM.md record
```

| Layer | Role | What it may contain |
|---|---|---|
| [`rustdesk/hbb_common`](https://github.com/rustdesk/hbb_common) | Upstream source of truth | Upstream development only |
| [`bashrusakh/hbb_common`](https://github.com/bashrusakh/hbb_common) | Mirror fork of upstream | On `main`: upstream history + non-behavioral infrastructure commits only (license metadata; the sync workflow file) — never behavioral patches. No behavioral patches anywhere in the fork (library content); documented exceptions in §3 |
| `libs/hbb_common/` (this repo) | Tracked copy of the fork at a recorded pin | Upstream content at the pin + DeskForge's patch set + the `UPSTREAM.md` pin record (the patch set and the record are DeskForge content, absent from the fork) — [`libs/hbb_common/UPSTREAM.md`](../libs/hbb_common/UPSTREAM.md) |

The mirror fork is **just a fork of `rustdesk/hbb_common`** — on `main` it mirrors
upstream, plus only the permitted non-behavioral infrastructure commits (table above).
The rule "no behavioral patches" applies to library content anywhere in the fork;
since the `webrtc` branch was deleted (deletion record in §1), the `main` invariant
covers the whole fork. (Fork-side tag history is a client-fork divergence class of
`bashrusakh/rustdesk`, recorded in §1 below.) DeskForge's local patches exist only in
the `libs/hbb_common/` copy.

Verified state (GitHub API + clones, 2026-11-23):

- **Upstream:** `main` HEAD `bb416bc94e48` (2026-11-10). Upstream has **no version
  tags at all** — every `hbb_common` pin in the ecosystem is a commit SHA.
- **Fork:** created 2026-06-11. Fork `main` @ `5a2d082` = upstream `main` @
  `bb416bc94e48` + three non-behavioral infrastructure commits: `36ab6ed` (license
  metadata — content in §3), `b53e044` (superseded sync draft "ci: auto-sync main
  with upstream on schedule" — content retrievable in history), `5a2d082` (sync
  workflow). Linear history: `5a2d082 → b53e044 → 36ab6ed → bb416bc`.
- **Branches:** `main` only. The `webrtc` branch is a dated **deletion record**:
  deleted 2026-11-23 by owner decision after evidence showed it fully superseded —
  exactly one commit `5a78ec4` (2026-05-18, "feat: support WebRTC rendezvous
  signaling") vs merge-base `9043c15acc6d` (upstream, 2026-05-16): +14/−0
  `protos/rendezvous.proto`, +110/−36 `src/webrtc.rs`. Upstream `main` (`bb416bc`)
  carries the conceptual delta merged and evolved (e.g. `PunchHoleRequest.
  webrtc_sdp_offer` renumbered 11→12, so the branch's wire numbering is incompatible
  with upstream's merged WebRTC) and adds DTLS-fingerprint identity binding the branch
  lacked; upstream also has its own `webrtc` branch (`470612b`). Nothing unique of
  value. Recoverable tip reference: `5a78ec42303ca046d808742e911a156636a2432b`.
- **Tags:** junk tags `driver`/`dirver` (upstream's historical typo tags) are mirrored
  by the fork and carry no meaning.
- **DeskForge fork point:** `rustdesk/hbb_common@fae47e024dd83cef6aaecde0aa7d11731df11649`
  (2026-10-28) — the `rustdesk/rustdesk` **1.5.1 / 1.5.3** `libs/hbb_common` pin
  (timeline §6). An ancestor of the fork's `main` (verified
  `git merge-base --is-ancestor`, 2026-11-23), so the tree was materialized by
  fetching the explicit SHA from `bashrusakh/hbb_common`. History: the original
  import pin `387603f47cbb15c0d3dc3d67ae3396d3eb707daf` (2026-06-04) — the exact
  `rustdesk/rustdesk` **1.4.8** pin, verified then by blob comparison + exhaustive
  ref scan; superseded 2026-11-23.
- **Checkout cleanliness:** any particular local checkout is **not tracked and remains
  unverified**; the published refs above are the provenance of record.

### `rustdesk` (client)

```
rustdesk/rustdesk            — upstream client
└── bashrusakh/rustdesk      — owned client fork; intentional divergence: gitlink repoint + workflow/branding branches
```

[`bashrusakh/rustdesk`](https://github.com/bashrusakh/rustdesk) diverges from upstream
only intentionally, in three classes: the `libs/hbb_common` gitlink is repointed at our
mirror fork; fork-side workflow/branding branches (`rustqs/*`) carry the client-build
workflows; and fork-side tag history records fork-side work (tag `1.5.0` = `9253e1fc`,
a CI-workflow merge commit — record in §3).
`libs/hbb_common` gitlink records (URL + SHA) in the fork:

| Fork ref | Fork ref SHA | `libs/hbb_common` gitlink |
|---|---|---|
| `master` | — | `bashrusakh/hbb_common` @ `36ab6ed` (2026-11-23) |
| tag `1.4.8` | `3c574a4` (identical to upstream 1.4.8) | `rustdesk/hbb_common` @ `387603f` |
| tag `1.5.0` | `9253e1fc` (fork's own CI-workflow merge commit, record §3 — **not** upstream's `1.5.0` = `fada664d`) | `rustdesk/hbb_common` @ `229b9045` |

**Tag names are not identities.** The fork's `1.5.0` records the same `hbb_common` pin
as upstream's `1.5.0` (`229b9045`), but the tagged commits differ. Always pin by SHA.

## 2. Fork creation rules

Creating a mirror fork in this topology (downstream, or rebuilding the mirror):

1. Fork with GitHub's fork tooling (e.g.
   `gh repo fork rustdesk/hbb_common --org YOUR_ORG --fork-name hbb_common --clone=false`)
   so the fork keeps upstream history and pin SHAs stay meaningful on both sides.
2. Enable automatic upstream sync in the fork where technically possible
   (`sync-upstream.yml`, §3). This is a mandate for our `bashrusakh/hbb_common`
   mirror — for it, a mirror without sync is a defect, not a variant — and is not a
   requirement imposed on third-party downstream rebuilds.
3. Never carry behavioral patches in the fork (§3): on `main`, only non-behavioral
   infrastructure commits (license metadata; the sync workflow file) beyond upstream
   history.
4. Record every pin by SHA (§4).

## 3. Fork maintenance

### Auto-sync (standing rule)

`bashrusakh/hbb_common` **must be kept in sync with upstream automatically, where
technically possible, by `sync-upstream.yml`** in the fork repository (a scheduled
GitHub Actions workflow living in the fork itself). Sync is **merge-based** —
`upstream/main` merged into `main`, never history-rewriting — so consumer SHA pins
remain valid. A documented manual fallback covers cases where automation cannot run.

> **Status 2026-11-23: sync workflow installed & enabled.** `sync-upstream.yml` landed
> on `main` as `5a2d082` (merge-based, daily 04:17 UTC + manual); workflow state
> `active`; smoke run `45125365177` succeeded ("Already in sync … Nothing to do.").
> **Pending:** the schedule-fire proof — the first `schedule`-triggered run at/after
> the next 04:17 UTC window (cron `17 4 * * *`).

### Manual fallback

```bash
git clone https://github.com/bashrusakh/hbb_common.git && cd hbb_common
git remote add upstream https://github.com/rustdesk/hbb_common.git   # once, if missing
git fetch upstream main
git checkout main && git merge upstream/main
git push origin main
```

Never rebase, force-push, or otherwise rewrite published `main`: consumer pins are
SHAs and must stay valid. Because the fork carries no behavioral patches, a sync
conflict can only touch the non-behavioral keep-list — `LICENSE`, the `license` field
in `Cargo.toml`, and `.github/workflows/sync-upstream.yml` (e.g. add/add): resolve it
by keeping upstream's content and re-applying exactly those.

### Fork-content invariant, license-metadata policy, documented exceptions

Scoped to `bashrusakh/hbb_common`, the mirror fork (the client fork's intentional
divergence is a separate class — §1):

- **`main` invariant:** on the mirror fork's `main`, upstream history + non-behavioral
  infrastructure commits only — license metadata and the sync workflow file —
  **never behavioral patches**.
- **No behavioral patches anywhere in the fork** is the rule for its library content
  (every branch). DeskForge's local behavior changes live exclusively in the
  `libs/hbb_common/` copy (§5), never in the fork.
- **License-metadata policy:** the license-metadata class is a `LICENSE` file and
  `license = "AGPL-3.0"` in `Cargo.toml`. The single existing example is fork-side
  commit `36ab6ed` ("chore: declare AGPL-3.0 license", 2026-11-23): `git show`
  confirms it adds exactly `license = "AGPL-3.0"` to `Cargo.toml` plus a 661-line
  AGPL-3.0 `LICENSE` file (probe 2026-11-23).
- **Documented exceptions/records**, named per repository (none behavioral in library
  content):
  - In `bashrusakh/hbb_common`: **none remain** — the `webrtc` branch was deleted
    2026-11-23 (deletion record in §1), so the `main` invariant now covers the whole
    fork.
  - In `bashrusakh/rustdesk` (client fork; divergence class recorded in §1): fork-side
    tag `1.5.0` (`9253e1fc`) — a merge commit of
    `bashrusakh/fix/rustqs-windows-bridge-call` ("fix(ci): drop undeclared enc_payload
    input from bridge workflow_call in rustqs-windows") changing
    `.github/workflows/rustqs-windows.yml`: fork-side CI-workflow content (GitHub API
    probe, 2026-11-23), not upstream's `1.5.0` (`fada664d`).

## 4. Pin bookkeeping

All `hbb_common` pins are commit SHAs (abbreviated in prose; the pin of record is the
full 40-character SHA). Client release tags (e.g. `1.5.0`, unprefixed) exist only on
`rustdesk/rustdesk`, and tag names are not identities (§1).

| Pin | Where recorded |
|---|---|
| DeskForge copy fork point + patch set | [`libs/hbb_common/UPSTREAM.md`](../libs/hbb_common/UPSTREAM.md) |
| Client fork `libs/hbb_common` gitlink (URL + SHA) | gitlink entry in the `bashrusakh/rustdesk` tree, per branch/tag |
| Client source ref + commit for offline freezes | `offline-kit/versions.env` (`RUSTDESK_REPO`, `RUSTDESK_REF`, `RUSTDESK_COMMIT`) |
| Reference pin timeline | §6 below |

Note: `offline-kit/versions.env` also names `HBB_COMMON_REPO` (line 13), a frozen-kit
default pointing at upstream `rustdesk/hbb_common`. Content is identical to the mirror
at a given SHA (the fork has no behavioral patches), so this is a provenance-labeling
question only; the frozen config file is deliberately not changed here.

## 5. Updating the DeskForge copy to a new pin

Moving `libs/hbb_common/` to a new pin (full client-release maintenance procedure:
[PLAN.md §7](../PLAN.md#7-fork-maintenance-procedure-for-a-new-upstream-rustdesk-client-release)):

1. **Sync the fork** (§3) so `bashrusakh/hbb_common` contains the target upstream
   commit — automatic via `sync-upstream.yml`, or the manual fallback.
2. **Re-point the client fork's gitlink by SHA** in `bashrusakh/rustdesk` (gitlink URL:
   `https://github.com/bashrusakh/hbb_common.git`, gitlink SHA: the chosen pin):

   ```bash
   # In bashrusakh/rustdesk, on the branch being updated (e.g. master):
   git fetch upstream --tags                       # rustdesk/rustdesk
   # bashrusakh/hbb_common — fetch the pin explicitly: pins may be off fork-main lineage
   git -C libs/hbb_common fetch origin <40-char-pin-SHA>
   #   (or: git -C libs/hbb_common fetch origin --tags '+refs/heads/*:refs/remotes/origin/*')
   git -C libs/hbb_common checkout <40-char-pin-SHA>
   git add libs/hbb_common                         # records URL + SHA
   git commit -m "chore: point hbb_common at <pin-SHA>"
   ```

3. **Save the pin record, replace the DeskForge copy, then rewrite the record.**
   `libs/hbb_common/UPSTREAM.md` is DeskForge content that does not exist in the fork,
   so the `rm -rf` below deletes it: save it first, and rewrite it with the new fork
   point (the record must change anyway):

   ```bash
   # In DeskForge:
   cp libs/hbb_common/UPSTREAM.md /tmp/UPSTREAM.md.bak
   rm -rf libs/hbb_common
   git clone https://github.com/bashrusakh/hbb_common.git libs/hbb_common
   # fetch the pin explicitly: pins may be off fork-main lineage
   git -C libs/hbb_common fetch origin <40-char-pin-SHA>
   #   (or: git -C libs/hbb_common fetch origin --tags '+refs/heads/*:refs/remotes/origin/*')
   git -C libs/hbb_common checkout <40-char-pin-SHA>
   rm -rf libs/hbb_common/.git
   cp /tmp/UPSTREAM.md.bak libs/hbb_common/UPSTREAM.md
   # then rewrite the fork-point row and patch rows in libs/hbb_common/UPSTREAM.md
   ```

4. **Re-apply the patch set** — the canonical record is
   [`libs/hbb_common/UPSTREAM.md`](../libs/hbb_common/UPSTREAM.md): re-decide every row
   of it on the record's carry-over rule (re-applied only if both still relevant against
   the new pin and still needed), apply exactly the kept rows, and record every
   kept/dropped verdict with its justification in the record. The record's kept-row
   table is the authoritative list of what to re-apply — no inline summary is kept
   here. The ⚠️ `gen_version()` row must survive every re-pin — the `server/` build's
   `$OUT_DIR/version.rs` include coupling is detailed in the record.
5. **Verify**: run `cd libs/hbb_common && cargo test` — the three `gen_version()`
   unit tests live in that package and are **not** run by `server/`'s `cargo test`
   (path dependency, no workspace membership); run them explicitly or they rot across
   re-pins. Then run the documented `server/` checks
   (`cd server && cargo build --release && cargo clippy && cargo test`): what these
   prove is the coupling, not the tests — `server/build.rs` calls
   `hbb_common::gen_version()`, so the `server/` build exercises the coupling and the
   `$OUT_DIR/version.rs` `include!` fails loudly if the patch is lost.
6. **Re-freeze offline-kit when the client line changes** — update
   `offline-kit/versions.env` (`RUSTDESK_REF`, `RUSTDESK_COMMIT`) and re-run the freeze
   ([PLAN.md §7](../PLAN.md#7-fork-maintenance-procedure-for-a-new-upstream-rustdesk-client-release)).

## 6. Reference pin timeline

`hbb_common` pin recorded by each upstream ref (context, verified 2026-11-23):

| Source ref | `hbb_common` pin |
|---|---|
| `rustdesk/rustdesk` 1.4.7 | `df6badca` |
| `rustdesk/rustdesk` 1.4.8 | `387603f` — DeskForge's original import pin (superseded 2026-11-23) |
| `rustdesk/rustdesk` 1.4.9 | `7e1c392c` |
| `rustdesk/rustdesk` 1.5.0 | `229b9045` |
| `rustdesk/rustdesk` 1.5.1 / 1.5.3 | `fae47e02` — DeskForge's current fork point |
| `rustdesk/rustdesk` master | `450cc1d7` |
| `rustdesk/rustdesk-server` 1.1.15 / 1.1.16 | `83419b65` |
| `rustdesk/rustdesk-server` master | `69cea8da` |

SHAs are abbreviated here; the pin of record is the full 40-character SHA.

## 7. Status and non-claims

- Facts in §1 are verified as of 2026-11-23 from the GitHub API and local clones,
  including the fork-state and deletion-stage probes. Unverified beyond that record:
  local-checkout cleanliness (§1). The former `webrtc`-branch relation question is
  resolved by the §1 deletion record: fully superseded by upstream's merged WebRTC
  work.
- No upstream-independence claim: active third-party upstream dependencies remain in
  the Rust/Go manifests (see [CONTRIBUTING.md](../CONTRIBUTING.md)).
