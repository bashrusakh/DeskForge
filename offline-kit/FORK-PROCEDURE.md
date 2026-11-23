# FORK-PROCEDURE — historical fork/dependency procedure (frozen reference)

> **FROZEN — historical reference only.** Everything below (fork + vendor candidate, asset handoff,
> downstream-forker notes, sovereignty checklist) is a historical procedure for a new version or for
> downstream forkers: examples use 1.4.8, the commands are executed only by the repo owner, and none
> of it is evidence that a current release or complete offline build exists. This procedure is not
> evidence of upstream independence, a network-denied full build, a verified license bundle, or a
> release; the offline kit has no signature claim — signature verification is still a gap.

---

## Level A — historical fork + vendor candidate

### A1. Fork rustdesk + hbb_common

```bash
gh repo fork rustdesk/rustdesk   --org YOUR_ORG --fork-name rustdesk   --clone=false
gh repo fork rustdesk/hbb_common --org YOUR_ORG --fork-name hbb_common --clone=false
```

Fork rules (canonical: [`docs/FORKS-AND-PINS.md`](../docs/FORKS-AND-PINS.md)): the `hbb_common` fork is a mirror kept in sync with upstream automatically where technically possible (enable `sync-upstream.yml` on creation), never carrying behavioral patches; all `hbb_common` pins are SHAs — upstream `rustdesk/hbb_common` has no version tags.

### A2. Vendor into the fork

From offline-kit:
```bash
git clone artifacts/rustdesk-1.4.8.bundle rustdesk-fork
cd rustdesk-fork && git remote set-url origin https://github.com/YOUR_ORG/rustdesk.git
git checkout -b release/1.4.8 1.4.8 && git submodule update --init --recursive
tar -xf ../artifacts/vendor-1.4.8.tar.gz
# .cargo/config.toml → source replacement to vendor/
git add vendor .cargo/config.toml
git commit -m "chore: freeze vendored deps 1.4.8"
git push origin release/1.4.8
```

`vendor/` is heavy — alternatively upload `vendor-{tag}.tar.gz` as a release asset.

### A3. Point versions.env to your fork

```env
RUSTDESK_REPO="https://github.com/YOUR_ORG/rustdesk.git"
RUSTDESK_REF="1.4.8"
# Also record the exact 40-character RUSTDESK_COMMIT and expected local
# artifact SHA-256 values before accepting a freeze.
```

### A4. Verify before handoff

From `offline-kit/`, after the local artifact set has been assembled:

```bash
bash freeze.sh --verify
bash license-inventory.sh > /tmp/deskforge-third-party-inventory.txt
```

Verification is read-only and fail-closed. It checks exact manifest byte sizes,
file SHA-256 values, Git-tree/content-hash records, source/tag/commit equality,
a clean non-shallow source checkout, recursive submodules, the source bundle,
vendor/source consistency, vcpkg baseline, the active TopMostWindow workflow
pin, and required pins. Only generated `vendor/` and
`.cargo/config.vendor.toml` source paths are allowed as working-tree additions;
their content is included in the manifest contract. It never downloads or
rewrites `MANIFEST.txt`. Missing
engine/source/vendor files are local blockers; missing provider assets are
reported as unavailable rather than treated as success.
If an expected digest is missing, only `freeze.sh --allow-tofu ...` may proceed
as an explicit manual exception, and `--verify` remains red until the digest is
manually recorded in `versions.env`.

---

## Level B — historical candidate asset handoff

### B1. What to upload (historical candidate handoff)

From `offline-kit/artifacts/`, after the local verification gates pass:

| Artifact                         | Why                                |
| -------------------------------- | ---------------------------------- |
| `windows-x64-release.zip`        | Custom Flutter engine              |
| `usbmmidd_v2.zip`                | Virtual display driver             |
| `rustdesk_printer_driver_v4-*.zip`| Printer driver                    |
| `printer_driver_adapter.zip`     | Printer adapter                    |
| `vendor-*.tar.gz`                | (optional, if not in git)          |

### B2. Historical release command

```bash
gh release create offline-assets-1.4.8 --repo YOUR_ORG/rustdesk \
    --title "Offline build assets (1.4.8)" \
    artifacts/windows-x64-release.zip artifacts/usbmmidd_v2.zip \
    artifacts/rustdesk_printer_driver_v4-1.4.zip artifacts/printer_driver_adapter.zip
```

### B3. Archive dependency forks (optional, L1 backup)

```bash
for r in RustDeskTempTopMostWindow; do
  gh repo fork rustdesk-org/$r --org YOUR_ORG --clone=false
done
```

---

## Level C — downstream forker

Someone forks **your** DeskForge → changes one line:
```env
RUSTDESK_REPO="https://github.com/THEIR_ORG/rustdesk.git"
```
→ their GUI can be configured to build from their fork. Upstream independence remains
unverified until the source, submodule, dependency, license, and offline-build checklist
passes.

### C1. Versions in admin UI

Version-catalog rules (repository-derived `offline-assets-*` entries, matching source-tag and required-asset checks, empty/error state instead of a hardcoded fallback): [`github-build/README.md`](../github-build/README.md) §Version flow (canonical).

**For downstream forkers:** a published matching provider asset release and source tag may appear in the dropdown after the provider-derived catalog refreshes; no hardcoded code values need changing.

### C2. Active workflow ownership

Workflow ownership, selector/tag rules, and the `rustqs/workflows` backup-ref definition (insurance copy of the fork's `master`, not an execution ref; production execution uses approval-gated, guarded immutable `refs/tags/workflow-*`): [`docs/FORKS-AND-PINS.md`](../docs/FORKS-AND-PINS.md) §1 (canonical), with secondary detail in [`github-build/README.md`](../github-build/README.md). The API preserves the configured branch/tag selector and separately checks the resolved workflow SHA; immutable workflow-ref protection remains a gate, not a current readiness claim. The following three-branch model is historical reference only:

| Branch | Purpose |
|---|---|
| `master` | API discovery (workflow must exist on default branch) |
| `rustqs/min-test` | Execution — all dispatches go here |
| `rustqs/master-workflows` | Mirror — backup for applying after upstream sync |

Historical bridge/checkout constraints are retained for maintenance context: `bridge.yml` must stay without `inputs.version`, and checkout must use the current fork repository rather than upstream. They do not make a live run or support claim.

---

## Sovereignty verification checklist

- [ ] `YOUR_ORG/rustdesk` with independently verified vendor + Cargo source config
- [ ] `YOUR_ORG/hbb_common` (submodule) with clean, published provenance
- [ ] Provider `offline-assets-{tag}` with independently verified candidate assets
- [ ] `versions.env` → your fork
- [ ] `cargo build --offline` passes without `github.com/rustdesk*`
- [ ] `bash freeze.sh --verify` passes in the handoff environment
- [ ] `license-inventory.sh` reviewed; every `GAP` has an owner or accepted risk
- [ ] Current local gaps (missing engine, legacy manifest/trusted expected values,
      license evidence, signature/attestation, and network-denied full-build proof)
      are closed with separate evidence
