# FORK-PROCEDURE — historical fork/dependency procedure (frozen reference)

> **FROZEN** — examples below use 1.4.8 and are historical reference for a new version
> or downstream forkers. The commands are not a claim that a current release or complete
> offline build exists; commands are executed only by the repo owner.
>
> This procedure is not evidence of upstream independence, a network-denied full build,
> a verified license bundle, or a release. The offline kit has no signature claim;
> signature verification is still a gap.

---

## Level A — historical fork + vendor candidate (not current proof)

### A1. Fork rustdesk + hbb_common

```bash
gh repo fork rustdesk/rustdesk   --org YOUR_ORG --fork-name rustdesk   --clone=false
gh repo fork rustdesk/hbb_common --org YOUR_ORG --fork-name hbb_common --clone=false
```

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

## Level B — historical candidate asset handoff (not current sovereignty proof)

### B1. What to upload (historical candidate handoff)

From `offline-kit/artifacts/`, after the local verification gates pass:

| Artifact                         | Why                                |
| -------------------------------- | ---------------------------------- |
| `windows-x64-release.zip`        | Custom Flutter engine              |
| `usbmmidd_v2.zip`                | Virtual display driver             |
| `rustdesk_printer_driver_v4-*.zip`| Printer driver                    |
| `printer_driver_adapter.zip`     | Printer adapter                    |
| `vendor-*.tar.gz`                | (optional, if not in git)          |

### B2. Historical release command (not a publication or readiness claim)

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

The version list in the admin UI (Custom Client → Version dropdown) is loaded
dynamically from the configured provider repository's `offline-assets-*` catalog.
Matching source-tag and required-asset checks determine the entries; provider
catalog failure returns an empty/error state rather than a hardcoded fallback.

**For downstream forkers:**
- After a matching provider asset release and source tag are available, the version
  may appear in the UI after the provider-derived catalog refreshes
- If the provider API is unavailable, no obsolete version fallback is shown
- No hardcoded values in code need to be changed

### C2. Active workflow ownership

The configured RustDesk fork's `.github/workflows/` files are the sole executable
source. `github-build/` and `rdgen/` are reference/frozen material and must not be
copied into the active fork as deployment sources. The API preserves the configured
branch/tag selector and separately checks the resolved workflow SHA; immutable
workflow-ref protection remains a gate, not a current readiness claim.

The following three-branch model is historical reference only:

| Branch | Purpose |
|---|---|
| `master` | API discovery (workflow must exist on default branch) |
| `rustqs/min-test` | Execution — all dispatches go here |
| `rustqs/master-workflows` | Mirror — backup for applying after upstream sync |

Historical bridge/checkout constraints are retained for maintenance context:
`bridge.yml` must be without `inputs.version`, and checkout must use the current
fork repository rather than upstream. They do not make a live run or support claim.

---

## Sovereignty verification checklist (not a current claim)

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
