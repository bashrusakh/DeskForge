# Phase 1 — PR1: BuildSpec/settings

**Status:** ✅ `verified-with-notes`
**Scope:** typed custom-client settings, canonical persisted `custom_json`, platform gate, and preset compatibility prerequisites. No workflow/Docker/RustDesk changes.

## Behavioral contract

The operator chooses a supported platform and edits known custom-client settings in the existing typed form, then saves a build or preset. The UI/API owns user-authored settings; platform/version/app name are record context; native workflow/provider identity is system-derived. Normal users do not edit raw JSON, raw `custom_txt`, or provider fields.

## Delivered contract

- `BuildSpec` is the typed owner of user-authored settings.
- `NormalizeCustomBuildJSON`/`CanonicalizeCustomBuildJSON` separate persisted form JSON from dispatch values and generated native `custom_.txt`.
- L1 `server_ip` and `key` remain available as typed persisted/dispatch values; record context (`platform`, `version`, `app_name`) is not accepted as arbitrary persisted form fields.
- The closed platform domain is `windows`, `linux`, and `android`; unsupported platforms fail validation before persistence or dispatch.
- Preset create/update uses the same canonicalization and preserves intentional empty/zero values rather than silently dropping them.
- Existing typed UI selection remains the normal path; no raw/manual editor was introduced.

## Exact implementation/evidence record

Relevant files checked:

- `api/service/custom_build_spec.go`
- `api/service/custom_build_spec_test.go`
- `api/service/custom_build.go`
- `api/service/custom_persistence_test.go`
- `api/service/custom_preset.go`
- `api/model/custom_build.go`
- `api/model/custom_preset.go`
- `api/http/controller/admin/custom_build.go`
- `api/http/controller/admin/custom_preset.go`
- `api/http/request/admin/custom_build.go`
- `api/http/request/admin/custom_preset.go`
- `admin-ui/src/views/custom-client/index.vue`

Focused evidence covers typed field mapping, field presence, malformed/wrong types, unsupported platforms, raw `custom_txt` rejection, canonical persistence, preset create/update, and empty-value compatibility.

Commands recorded:

```text
GOWORK=off go test ./cmd ./model ./service ./http/controller/admin
GOWORK=off go test -race ./cmd ./model ./service ./http/controller/admin
go vet ./cmd ./model ./service ./http/controller/admin
gofmt -w api/cmd/apimain.go api/http/controller/admin/custom_build.go api/http/controller/admin/custom_preset.go api/http/request/admin/custom_build.go api/http/request/admin/custom_preset.go api/model/custom_build.go api/service/custom_build.go api/service/custom_build_spec.go api/service/custom_preset.go
git diff --check
npm run build
```

The local run passed; the UI build emitted only the existing Rollup pure-comment/chunk-size warnings. This plan record does not claim live provider or deployment validation.

## Gates and remaining dependencies

- Keep all normal inputs typed and capability-shaped; do not expose raw/internal values.
- Keep the platform gate consistent across API, preset, build, and UI paths.
- PR2 must consume the canonical endpoint values without a second normalization layer.
- PR3 must consume typed dispatch parameters rather than reparsing arbitrary persisted JSON.
- PR6 owns workflow YAML/config simplification; PR1 does not change workflows.
