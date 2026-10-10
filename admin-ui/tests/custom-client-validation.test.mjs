// Run with: node --test tests/custom-client-validation.test.mjs
// Exercise the SFC's actual logic with Vue reactivity, not a validator copy.
//
// Design: the server is the single source of truth for field FORMAT validity.
// The create response carries machine-readable per-field reasons
// (`data.fields: [{ field, code }]`) derived from the authoritative Go
// validators (api/service/custom_build_spec.go ValidateCustomBuildInput). The
// form deliberately does NOT parse IPv4/IPv6/host/URL/key itself; it maps those
// server reasons onto `invalidFields` / `:error` / ARIA. These tests therefore
// assert the mapping and the presence/required boundary, not JS parsing parity.
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import vm from 'node:vm'
import { reactive, ref, computed, nextTick, watch } from 'vue'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import axios from 'axios'

const sfc = readFileSync(new URL('../src/views/custom-client/index.vue', import.meta.url), 'utf8')
const between = (start, end) => {
  assert.ok(sfc.includes(start) && sfc.includes(end), `missing SFC boundary: ${start}`)
  return sfc.slice(sfc.indexOf(start), sfc.indexOf(end, sfc.indexOf(start)))
}
// The submitted field set (PRESET_FIELDS + record fields) and the blank form are
// defined in the SFC. Extract them from the source of truth rather than
// hardcoding a narrow copy: a fixture that narrows PRESET_FIELDS (e.g. to ['key'])
// hides exactly the F-D defect where optional in-flight edits were discarded.
const sfcArrayLiteral = (name) => {
  const match = sfc.match(new RegExp(`const ${name} = (\\[[^\\]]*\\])`))
  assert.ok(match, `missing SFC array literal ${name}`)
  return vm.runInNewContext(match[1])
}
const sfcObjectLiteral = (name) => {
  const match = sfc.match(new RegExp(`const ${name} = (\\{[\\s\\S]*?\\n    \\})`))
  assert.ok(match, `missing SFC object literal ${name}`)
  return vm.runInNewContext(`(${match[1]})`)
}
const PRESET_FIELDS = sfcArrayLiteral('PRESET_FIELDS')
const FORM_DEFAULTS = sfcObjectLiteral('FORM_DEFAULTS')
const formState = (platform = 'linux') => ({ ...FORM_DEFAULTS, platform })

function versionsFixture() {
  let resolveVersions, rejectVersions
  const response = new Promise((resolve, reject) => {
    resolveVersions = resolve
    rejectVersions = reject
  })
  const context = {
    ref, computed, form: reactive(formState()), T: value => value,
    getVersions: () => response,
  }
  vm.createContext(context)
  vm.runInContext(between('const extractApiError =', '\nexport default') +
    between('    const versionsState =', '\n    // B-017: при silent=true') +
    between('    const loadVersions =', '\n    const loadConfig =') + `
    lifecycleGuard.mounted = true;
    globalThis.api = { loadVersions, versionsState, versionsReady, versions,
      versionsError, lifecycleGuard };
  `, context)
  return { ...context, resolveVersions, rejectVersions }
}

test('Version has a loading-only native prefix without replacing its value or disabling selection', () => {
  const field = between('            <el-form-item :label="T(\'Version\')"', '\n            </el-form-item>')
  assert.match(field, /<template v-if="versionsState === 'loading'" #prefix>/)
  assert.match(field, /<el-icon class="is-loading" aria-hidden="true"><Loading \/><\/el-icon>/)
  assert.match(field, /v-model="form.version"/)
  assert.match(field, /:aria-busy="versionsState === 'loading'"/)
  const select = field.slice(field.indexOf('<el-select'), field.indexOf('</el-select>'))
  assert.doesNotMatch(select, /:disabled=|v-loading=/)
})

test('Version stays loading until its own request settles and preserves explicit selection', async () => {
  for (const [data, state] of [
    [{ versions: [{ version: '1.4.8' }] }, 'ready'],
    [{ versions: [] }, 'empty'],
    [{ versions: [], error: true, message: 'Provider unavailable' }, 'error'],
  ]) {
    const ctx = versionsFixture()
    ctx.form.version = 'saved-version'
    const pending = ctx.api.loadVersions()
    await nextTick()
    assert.equal(ctx.api.versionsState.value, 'loading')
    assert.equal(ctx.api.versionsReady.value, false)
    ctx.resolveVersions({ data })
    await pending
    assert.equal(ctx.api.versionsState.value, state)
    assert.equal(ctx.api.versionsReady.value, state === 'ready')
    assert.equal(ctx.form.version, 'saved-version')
    assert.equal(ctx.api.versions.value.length, data.versions.length)
    // Keep the existing fallback for catalog errors without an API envelope code.
    assert.equal(ctx.api.versionsError.value, state === 'error' ? 'VersionListError' : '')
  }
})

test('Version request rejection ends loading without choosing a fallback', async () => {
  const ctx = versionsFixture()
  const pending = ctx.api.loadVersions()
  ctx.rejectVersions(new Error('network failed'))
  await pending
  assert.equal(ctx.api.versionsState.value, 'error')
  assert.equal(ctx.api.versionsReady.value, false)
  assert.equal(ctx.api.versionsError.value, 'VersionListError')
  assert.equal(ctx.api.versions.value.length, 0)
  assert.equal(ctx.form.version, '')
})

test('Version response after unmount cannot update the disposed component', async () => {
  const ctx = versionsFixture()
  const pending = ctx.api.loadVersions()
  ctx.api.lifecycleGuard.mounted = false
  ctx.resolveVersions({ data: { versions: [{ version: '1.4.8' }] } })
  await pending
  assert.equal(ctx.api.versionsState.value, 'loading')
  assert.equal(ctx.api.versions.value.length, 0)
  assert.equal(ctx.form.version, '')
})

function fixture(platform = 'linux') {
  const logs = []
  const context = {
    reactive, ref, computed, nextTick, watch, atob, btoa, URL, T: value => value,
    form: reactive(formState(platform)), formRef: ref(null),
    serverConfigDefaults: reactive({ key: '' }),
    document: { getElementById: () => null },
    axios, console: { error: (...args) => logs.push(args) },
    PRESET_FIELDS,
    // Overridden per test; a plain property so the vm's free-variable lookup
    // picks up the test's stub at call time.
    fetchServerAddresses: async () => ({ data: { addresses: [] } }),
    logs,
  }
  vm.createContext(context)
  vm.runInContext(between('    const requiredFieldNames', '    const useServerKey') +
    between('    const useServerKey', '    const isFieldInvalid') +
    between('    const isFieldInvalid', '    const validateBuildForm') + `
    globalThis.api = { rules, requiredFieldSet, invalidFields, serverFieldErrors,
      serverFieldError, serverFieldMessage, applyServerFieldErrors,
      snapshotSubmittedFields, isSubmittedSnapshotCurrent, reportCreateBuildFailure,
      useServerKey, isFieldInvalid, isRequiredField, clearFieldError, syncFieldAria,
      localAddressPicker, openLocalAddressPicker, useLocalAddress, closeLocalAddressPicker,
      localAddressPrefillValue };
    globalThis.logs = logs;
  `, context)
  return context
}
const key = Buffer.alloc(32, 7).toString('base64')

// Exercise the real selection lookup and save handler; only the dialog/API and
// unrelated form-loading effects are stubbed. The prompt spy simulates editing
// the initial value before confirming.
function presetSaveFixture() {
  const calls = { prompts: [], payloads: [], refreshes: 0 }
  const context = {
    ref, computed, T: value => value, PRESET_FIELDS,
    form: reactive(formState()), presetPasswordClearIntent: ref(false),
    ElMessageBox: { prompt: async (message, title, options) => {
      calls.prompts.push({ message, title, options })
      return { value: context.editedName ?? options.inputValue ?? '' }
    } },
    ElMessage: { success: () => {} },
    createPreset: async payload => { calls.payloads.push(payload) },
    loadPresets: async () => { calls.refreshes++ },
    loadPresetIntoForm: () => {},
    resetForm: () => { context.api.selectedPresetId.value = null },
    console,
  }
  vm.createContext(context)
  vm.runInContext(between('    const presets = ref([])', '    const clearSavedPresetPassword') +
    between('    const canPreservePresetPassword', '    const resetFormFields') +
    between('    const onPresetSelect', '    const deletePreset') + `
    globalThis.api = { presets, selectedPresetId, onPresetSelect, saveCurrentAsPreset };
  `, context)
  context.api.presets.value = [
    { id: 1, name: 'First preset', has_permanent_password: true },
    { id: 2, name: 'Second preset' },
  ]
  context.calls = calls
  return context
}

test('Save preset initially fills the selected preset name', async () => {
  const ctx = presetSaveFixture()
  ctx.api.selectedPresetId.value = 1
  ctx.api.onPresetSelect(1)
  await ctx.api.saveCurrentAsPreset()
  assert.equal(ctx.calls.prompts[0].options.inputValue, 'First preset')
  assert.equal(ctx.calls.payloads[0].name, 'First preset')
  assert.equal(ctx.calls.payloads[0].preserve_permanent_password, true)
})

test('Save preset reads the latest selection and clears the initial name after deselection', async () => {
  const ctx = presetSaveFixture()
  for (const id of [1, 2, null]) {
    ctx.api.selectedPresetId.value = id
    ctx.api.onPresetSelect(id)
    await ctx.api.saveCurrentAsPreset()
  }
  assert.deepEqual(ctx.calls.prompts.map(call => call.options.inputValue), ['First preset', 'Second preset', ''])
  assert.deepEqual(ctx.calls.payloads.map(payload => payload.name), ['First preset', 'Second preset'])
})

test('the editable prompt result remains the save name and preserves the payload shape', async () => {
  const ctx = presetSaveFixture()
  ctx.api.selectedPresetId.value = 1
  ctx.editedName = 'User edited name'
  await ctx.api.saveCurrentAsPreset()
  const payload = ctx.calls.payloads[0]
  assert.equal(ctx.calls.prompts[0].options.inputValue, 'First preset')
  assert.equal(payload.name, 'User edited name')
  assert.equal(payload.platform, ctx.form.platform)
  assert.equal(payload.version, ctx.form.version)
  assert.equal(payload.app_name, ctx.form.app_name)
  assert.equal(payload.preserve_permanent_password, false, 'password preservation still uses the confirmed name')
  assert.deepEqual(JSON.parse(payload.custom_json), Object.fromEntries(PRESET_FIELDS.map(field => [field, ctx.form[field]])))
  assert.deepEqual(Object.keys(payload).sort(), ['app_name', 'custom_json', 'name', 'platform', 'preserve_permanent_password', 'version'])
  assert.equal(ctx.calls.refreshes, 1)
})

test('without a selection Save preset keeps the blank editable new-preset prompt', async () => {
  const ctx = presetSaveFixture()
  ctx.editedName = 'New preset'
  await ctx.api.saveCurrentAsPreset()
  assert.equal(ctx.calls.prompts[0].options.inputValue ?? '', '')
  assert.equal(ctx.calls.prompts[0].options.inputPlaceholder, 'My Preset')
  assert.equal(ctx.calls.payloads[0].name, 'New preset')
  assert.equal(ctx.calls.payloads[0].preserve_permanent_password, false)
})

// Fixture for the create-success path (F-D). It runs the same extracted SFC
// logic as `fixture` (so isSubmittedSnapshotCurrent/snapshotSubmittedFields are
// the real implementation), plus submitBuild's own body, with the request layer
// stubbed so the post-await side effects can be observed via spies.
function successFixture(platform = 'linux') {
  const calls = { resetForm: 0, loadBuilds: 0, success: 0 }
  const context = {
    reactive, ref, computed, nextTick, watch, atob, btoa, URL, T: value => value,
    form: reactive(formState(platform)), formRef: ref(null),
    serverConfigDefaults: reactive({ key: '' }),
    document: { getElementById: () => null },
    axios, console: { error: () => {}, warn: () => {} },
    ElMessage: { success: () => { calls.success++ }, warning: () => {}, error: () => {} },
    create: async () => ({ data: {} }),
    submitting: ref(false),
    versionsState: ref('ready'),
    productionPlatformReady: computed(() => true),
    validateBuildForm: async () => true,
    resetForm: () => { calls.resetForm++ },
    loadBuilds: () => { calls.loadBuilds++ },
    PRESET_FIELDS,
  }
  vm.createContext(context)
  vm.runInContext(between('    const requiredFieldNames', '    const useServerKey') +
    between('    const useServerKey', '    const isFieldInvalid') +
    between('    const isFieldInvalid', '    const validateBuildForm') +
    between('    const submitBuild = async () => {', '\n    const deleteBuild') + `
    globalThis.api = { submitBuild, snapshotSubmittedFields, isSubmittedSnapshotCurrent, form };
  `, context)
  context.calls = calls
  return context
}

test('create success with an in-flight edit keeps the form and still refreshes the list', async () => {
  // F-D race (success path): the form stays editable during the request. If the
  // user changes a field while create is in flight, the success handler must not
  // silently discard those edits, but it must still surface the created build.
  const ctx = successFixture('linux')
  ctx.form.key = 'submitted-key'
  let releaseCreate
  let createStarted
  const started = new Promise((resolve) => { createStarted = resolve })
  ctx.create = () => {
    createStarted() // the snapshot was already captured synchronously before this
    return new Promise((resolve) => { releaseCreate = resolve })
  }
  const pending = ctx.api.submitBuild()
  await started
  ctx.form.key = 'edited-while-in-flight'
  releaseCreate()
  await pending
  assert.equal(ctx.form.key, 'edited-while-in-flight', 'the in-flight edit must be preserved')
  assert.equal(ctx.calls.resetForm, 0, 'resetForm must not run when the submitted snapshot is stale')
  assert.equal(ctx.calls.loadBuilds, 1, 'the created build must still appear in the list')
})

test('create success with an unchanged form resets the form and refreshes the list', async () => {
  const ctx = successFixture('linux')
  ctx.form.key = 'submitted-key'
  await ctx.api.submitBuild()
  assert.equal(ctx.calls.resetForm, 1, 'an unchanged form is reset after a successful create')
  assert.equal(ctx.calls.loadBuilds, 1, 'the created build must appear in the list')
})

test('a change to a requirement input (platform) also keeps the form on success', () => {
  // isSubmittedSnapshotCurrent reuses the error path's requirement-input rule:
  // platform/hide_cm decide which fields the server requires, so a change to
  // either invalidates the whole snapshot even if the displayable values match.
  const ctx = successFixture('linux')
  ctx.form.key = 'submitted-key'
  const snapshot = ctx.api.snapshotSubmittedFields()
  assert.equal(ctx.api.isSubmittedSnapshotCurrent(snapshot), true)
  ctx.form.platform = 'windows'
  assert.equal(ctx.api.isSubmittedSnapshotCurrent(snapshot), false)
})

test('an in-flight edit to an optional submitted field keeps the form on success', async () => {
  // F-D regression: submitBuild serializes all of PRESET_FIELDS (not just the
  // required ones), so an edit to ANY sent field during the request must block
  // the success-path resetForm. Sol reproduced company_name/enable_audio/
  // app_icon_url edits being silently discarded. The successFixture now uses the
  // real PRESET_FIELDS so this test would fail against the old narrow guard.
  const cases = [
    ['company_name', 'edited-company'], // string, PRESET_FIELDS, not required
    ['enable_audio', false], // boolean, PRESET_FIELDS, not required
    ['app_icon_url', '/upload/2026/in-flight.png'], // async-upload result field
  ]
  for (const [field, edited] of cases) {
    const ctx = successFixture('linux')
    ctx.form.key = 'submitted-key'
    let releaseCreate
    let createStarted
    const started = new Promise((resolve) => { createStarted = resolve })
    ctx.create = () => {
      createStarted()
      return new Promise((resolve) => { releaseCreate = resolve })
    }
    const pending = ctx.api.submitBuild()
    await started
    ctx.form[field] = edited // edit while create is in flight
    releaseCreate()
    await pending
    assert.equal(ctx.form[field], edited, `${field}: the in-flight edit must be preserved`)
    assert.equal(ctx.calls.resetForm, 0, `${field}: resetForm must not run for an optional submitted edit`)
    assert.equal(ctx.calls.loadBuilds, 1, `${field}: the created build must still appear in the list`)
  }
})

test('a snapshot covers every submitted field, including ones outside requiredFieldNames', () => {
  // The guard must account for the whole create payload. If a future field is
  // added to PRESET_FIELDS or the record columns and not snapshotted, this fails.
  const ctx = successFixture('linux')
  const expected = new Set([...PRESET_FIELDS, 'platform', 'version', 'app_name'])
  const snapshot = ctx.api.snapshotSubmittedFields()
  assert.deepEqual(new Set(Object.keys(snapshot.values)), expected)
  for (const field of ['company_name', 'enable_audio', 'app_icon_url', 'platform', 'version', 'app_name']) {
    assert.ok(field in snapshot.values, `snapshot must capture the submitted ${field}`)
  }
  // Every displayable field the error path reasons about survives the widening.
  for (const field of ['platform', 'version', 'app_name', 'server_ip', 'key', 'api_server', 'relay_server', 'permanent_password']) {
    assert.ok(field in snapshot.values, `required field ${field} must remain in the snapshot`)
  }
})

test('every snapshotted submitted field is a primitive value', () => {
  // The guard compares with strict equality; an object/array field would compare
  // by reference and break the staleness rule. All form fields are primitives.
  const ctx = successFixture('windows')
  for (const [field, value] of Object.entries(ctx.api.snapshotSubmittedFields().values)) {
    assert.equal(
      value === null || ['string', 'boolean', 'number'].includes(typeof value), true,
      `${field} must hold a primitive, got ${typeof value}`,
    )
  }
})

test('the success-path guard actually covers the width of the create payload', () => {
  // Negative control: the old implementation compared only requiredFieldNames,
  // which is narrower than what submitBuild sends. Assert the guard is driven by
  // the submitted payload, not by the required set.
  const guard = between('    const isSubmittedSnapshotCurrent', '    // Classify a create failure')
  assert.equal(guard.includes('requiredFieldNames.every'), false, 'the guard must not be limited to requiredFieldNames')
  assert.match(guard, /Object\.keys\(snapshot\.values\)\.every/)
  const snapshotFn = between('    const snapshotSubmittedFields', '    // True when the live form')
  assert.equal(snapshotFn.includes('for (const field of requiredFieldNames)'), false)
  assert.match(snapshotFn, /submittedFieldNames\(\)/)
})

test('form has no local IPv4/IPv6/host/URL/key format parsers', () => {
  for (const removed of ['isValidIpv6', 'isValidIpv4', 'isValidHost', 'isValidEndpointFormat',
    'isValidApiServerFormat', 'isValidPublicKeyFormat', 'formatValidators', 'formatCheckedFields',
    'isFieldFormatInvalid']) {
    assert.equal(sfc.includes(removed), false, `stale local validator remains: ${removed}`)
  }
})

test('server field errors map to localized per-field messages and invalidFields', async () => {
  const ctx = fixture('windows')
  const applied = await ctx.api.applyServerFieldErrors([
    { field: 'server_ip', code: 'invalid_endpoint' },
    { field: 'key', code: 'required' },
    { field: 'api_server', code: 'invalid_format' },
  ])
  assert.equal(applied, true)
  // `required` reuses the existing per-field required key.
  assert.equal(ctx.api.serverFieldError('key'), 'CustomClientKeyRequired')
  assert.equal(ctx.api.serverFieldError('server_ip'), 'CustomClientHostInvalidFormat')
  assert.equal(ctx.api.serverFieldError('api_server'), 'CustomClientApiServerInvalidFormat')
  // invalidFields follows the existing highlight/ARIA schema.
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value).sort(), ['api_server', 'key', 'server_ip'])
  assert.equal(ctx.api.isFieldInvalid('server_ip'), true)
})

test('unknown or non-displayable server fields are ignored', async () => {
  const ctx = fixture('windows')
  const applied = await ctx.api.applyServerFieldErrors([
    { field: 'android_app_id', code: 'invalid_format' },
    { field: '__internal', code: 'required' },
  ])
  assert.equal(applied, false)
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value), [])
})

test('stale response after a field edit is not applied to the newer value', async () => {
  // F-B race: the request snapshots the submitted values; the user edits a field
  // while the HTTP request is in flight; the late error must not land on the new
  // value.
  const ctx = fixture('linux')
  ctx.form.key = 'edited-by-user'
  const snapshot = ctx.api.snapshotSubmittedFields() // key = 'edited-by-user'
  ctx.form.key = 'user-typed-something-else' // edit while request is in flight
  const applied = await ctx.api.applyServerFieldErrors([{ field: 'key', code: 'invalid_format' }], snapshot)
  assert.equal(applied, false)
  assert.deepEqual(Object.keys(ctx.api.serverFieldErrors.value), [])
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value), [])
})

test('response with an unchanged field is applied', async () => {
  const ctx = fixture('linux')
  ctx.form.key = 'submitted-value'
  const snapshot = ctx.api.snapshotSubmittedFields()
  const applied = await ctx.api.applyServerFieldErrors([{ field: 'key', code: 'invalid_format' }], snapshot)
  assert.equal(applied, true)
  assert.equal(ctx.api.serverFieldError('key'), 'CustomClientKeyInvalidFormat')
  assert.equal(ctx.api.isFieldInvalid('key'), true)
})

test('platform change invalidates a stale response entirely', async () => {
  const ctx = fixture('linux')
  ctx.form.key = 'submitted-value'
  const snapshot = ctx.api.snapshotSubmittedFields()
  ctx.form.platform = 'windows' // platform rules changed while in flight
  const applied = await ctx.api.applyServerFieldErrors([{ field: 'key', code: 'required' }], snapshot)
  assert.equal(applied, false)
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value), [])
})

test('hide_cm change invalidates a stale response entirely', async () => {
  // F-B (follow-up): the server requirement set also depends on hide_cm
  // (permanent_password is required when hide_cm is true), so toggling it while
  // the create request is in flight must invalidate the response just like a
  // platform change.
  const ctx = fixture('linux')
  ctx.form.hide_cm = false
  const snapshot = ctx.api.snapshotSubmittedFields()
  assert.equal(snapshot.hide_cm, false)
  ctx.form.hide_cm = true // requirement set changed while in flight
  const applied = await ctx.api.applyServerFieldErrors(
    [{ field: 'permanent_password', code: 'required' }], snapshot)
  assert.equal(applied, false)
  assert.deepEqual(Object.keys(ctx.api.serverFieldErrors.value), [])
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value), [])
})

test('a partially-stale response applies only the still-unchanged fields', async () => {
  const ctx = fixture('linux')
  ctx.form.key = 'submitted-key'
  ctx.form.app_name = 'submitted-app'
  const snapshot = ctx.api.snapshotSubmittedFields()
  ctx.form.key = 'edited-key' // only key changed
  const applied = await ctx.api.applyServerFieldErrors([
    { field: 'key', code: 'invalid_format' },
    { field: 'app_name', code: 'required' },
  ], snapshot)
  assert.equal(applied, true)
  assert.equal(ctx.api.isFieldInvalid('key'), false)
  assert.equal(ctx.api.isFieldInvalid('app_name'), true)
})

test('without a snapshot the response is applied to the current state', async () => {
  // Legacy/test callers that do not track submit state keep the previous behavior.
  const ctx = fixture('linux')
  const applied = await ctx.api.applyServerFieldErrors([{ field: 'key', code: 'required' }])
  assert.equal(applied, true)
  assert.equal(ctx.api.serverFieldError('key'), 'CustomClientKeyRequired')
})

test('submitBuild wires the bounded create-failure reporter with the snapshot', () => {
  const submit = between('    const submitBuild = async () => {', '\n    const deleteBuild')
  assert.match(submit, /snapshotSubmittedFields\(\)/, 'submitBuild must snapshot the submitted values')
  assert.match(submit, /reportCreateBuildFailure\(e,\s*submittedSnapshot\)/, 'the catch must delegate to the bounded reporter')
  assert.equal(submit.includes('console.error(e)'), false, 'submitBuild must not log the raw error object')
})

// Build a real axios 400 error whose config retains the submitted body and the
// api-token header, the exact shape the response interceptor rejects.
const staleCreateError = (fields) => new axios.AxiosError(
  'Request failed with status code 400',
  'ERR_BAD_REQUEST',
  { data: JSON.stringify({ permanent_password: 'DUMMY_PASSWORD', key: 'DUMMY_KEY' }), headers: { 'api-token': 'DUMMY_TOKEN' } },
  {},
  { status: 400, data: { code: 101, data: { fields } } },
)
const serializedLogs = (logs) => logs
  .map(args => args.map(value => {
    if (typeof value === 'string') return value
    try { return JSON.stringify(value) } catch { return String(value) }
  }).join(' '))
  .join('\n')

test('a stale create response never logs the raw axios error (no secret leak)', async () => {
  const ctx = fixture('linux')
  ctx.form.key = 'submitted-key'
  const snapshot = ctx.api.snapshotSubmittedFields()
  ctx.form.key = 'edited-while-in-flight' // F-B: the response is now stale
  const error = staleCreateError([{ field: 'key', code: 'required' }])
  await ctx.api.reportCreateBuildFailure(error, snapshot)
  assert.equal(ctx.logs.length, 0, 'a stale, already-handled response must not be logged')
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value), [], 'the stale response is dropped')
})

test('a 400 field-error response for the unchanged form is handled, not logged', async () => {
  const ctx = fixture('linux')
  ctx.form.key = 'submitted-key'
  const snapshot = ctx.api.snapshotSubmittedFields()
  const error = staleCreateError([{ field: 'key', code: 'required' }])
  await ctx.api.reportCreateBuildFailure(error, snapshot)
  assert.equal(ctx.logs.length, 0, 'expected per-field validation failures are not logged')
  assert.equal(ctx.api.isFieldInvalid('key'), true, 'the server reason is still rendered')
})

test('a transport/network failure is treated as interceptor-handled and not logged', async () => {
  const ctx = fixture('linux')
  const error = new axios.AxiosError('Network Error', 'ERR_NETWORK', {}, {})
  await ctx.api.reportCreateBuildFailure(error, ctx.api.snapshotSubmittedFields())
  assert.equal(ctx.logs.length, 0, 'the interceptor already surfaced the transport failure')
})

test('an unexpected error is logged only in a bounded form without the raw axios object', async () => {
  const ctx = fixture('linux')
  const error = Object.assign(new Error('kaboom'), {
    config: { data: 'DUMMY_PASSWORD', headers: { 'api-token': 'DUMMY_TOKEN' } },
  })
  await ctx.api.reportCreateBuildFailure(error, ctx.api.snapshotSubmittedFields())
  assert.equal(ctx.logs.length, 1)
  assert.equal(ctx.logs[0][0], 'Custom client build request failed:')
  assert.equal(ctx.logs[0][1], 'kaboom')
  const rendered = serializedLogs(ctx.logs)
  assert.equal(rendered.includes('DUMMY_PASSWORD'), false, 'the request body must never be logged')
  assert.equal(rendered.includes('DUMMY_TOKEN'), false, 'the api-token must never be logged')
})

// request.js rejects a failed envelope as the raw body object (`res`), so an
// envelope create error carries no axios markers and may have data: null: the
// controller returns {code:101,message:'OperationFailed',data:null} for an
// expected create failure. The response interceptor has already toasted the
// bounded message, so this must be handled and never logged as unexpected.
// config is attached here only to prove nothing from it can leak.
const envelopeCreateError = (data) => Object.assign(
  { code: 101, message: 'OperationFailed', data },
  { config: { data: JSON.stringify({ permanent_password: 'DUMMY_PASSWORD', key: 'DUMMY_KEY' }), headers: { 'api-token': 'DUMMY_TOKEN' } } },
)

test('an interceptor-handled envelope error without fields is handled, not logged', async () => {
  const ctx = fixture('linux')
  const error = envelopeCreateError(null)
  await ctx.api.reportCreateBuildFailure(error, ctx.api.snapshotSubmittedFields())
  assert.equal(ctx.logs.length, 0, 'the interceptor already toasted the envelope; no extra log')
  const rendered = serializedLogs(ctx.logs)
  assert.equal(rendered.includes('DUMMY_PASSWORD'), false, 'the request body must never be logged')
  assert.equal(rendered.includes('DUMMY_TOKEN'), false, 'the api-token must never be logged')
})

test('an envelope error that carries data.fields still applies the server reasons', async () => {
  const ctx = fixture('linux')
  ctx.form.key = 'submitted-key'
  const snapshot = ctx.api.snapshotSubmittedFields()
  const error = envelopeCreateError({ fields: [{ field: 'key', code: 'required' }] })
  await ctx.api.reportCreateBuildFailure(error, snapshot)
  assert.equal(ctx.logs.length, 0, 'expected per-field envelope failures are not logged')
  assert.equal(ctx.api.isFieldInvalid('key'), true, 'the server reason is still rendered')
})

test('a zero code object is not misread as a handled envelope', async () => {
  const ctx = fixture('linux')
  await ctx.api.reportCreateBuildFailure(
    { code: 0, message: 'not a failure', data: null }, ctx.api.snapshotSubmittedFields())
  assert.equal(ctx.logs.length, 1, 'only a numeric nonzero envelope code counts as handled')
})

test('clearing a field removes both its server and local error state', () => {
  const ctx = fixture('windows')
  ctx.api.invalidFields.value = { key: ['CustomClientKeyRequired'] }
  ctx.api.serverFieldErrors.value = { key: 'CustomClientKeyRequired' }
  ctx.api.clearFieldError('key')
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value), [])
  assert.deepEqual(Object.keys(ctx.api.serverFieldErrors.value), [])
})

test('required policy matches the server contract per platform', () => {
  // platform/app_name/version + key are required everywhere; key is included
  // because this form's submit action dispatches the build and dispatch requires
  // a non-empty key on every platform (RequireDispatchPublicKey).
  for (const platform of ['windows', 'linux', 'android']) {
    const ctx = fixture(platform)
    assert.ok(ctx.api.isRequiredField('platform'), `${platform} platform required`)
    assert.ok(ctx.api.isRequiredField('version'), `${platform} version required`)
    assert.ok(ctx.api.isRequiredField('app_name'), `${platform} app_name required`)
    assert.ok(ctx.api.isRequiredField('key'), `${platform} key required`)
  }
  // Windows-only endpoint fields.
  const windows = fixture('windows')
  for (const field of ['server_ip', 'api_server', 'relay_server']) {
    assert.ok(windows.api.isRequiredField(field), `windows ${field} required`)
  }
  const linux = fixture('linux')
  for (const field of ['server_ip', 'api_server', 'relay_server']) {
    assert.equal(linux.api.isRequiredField(field), false, `linux ${field} optional`)
  }
})

test('rules only encode presence/required, never a local format validator', () => {
  for (const platform of ['windows', 'linux']) {
    const ctx = fixture(platform)
    for (const field of ['platform', 'version', 'app_name', 'key']) {
      const rules = ctx.api.rules.value[field] || []
      assert.ok(rules.some(rule => rule.required), `${platform} ${field} has a required rule`)
      assert.equal(rules.some(rule => rule.validator), false, `${platform} ${field} must not carry a format rule`)
    }
  }
})

test('required rules carry presence-only semantics (required + whitespace)', () => {
  const ctx = fixture('windows')
  const requiredRule = ctx.api.rules.value.key.find(rule => rule.required)
  assert.equal(requiredRule.whitespace, true)
  assert.equal(requiredRule.validator, undefined)
})

test('server-key action still uses exposed defaults and only checks presence', async () => {
  const ctx = fixture()
  ctx.serverConfigDefaults.key = key
  ctx.api.invalidFields.value = { key: ['CustomClientKeyInvalidFormat'] }
  ctx.api.serverFieldErrors.value = { key: 'CustomClientKeyRequired' }
  await ctx.api.useServerKey()
  assert.equal(ctx.form.key, key)
  assert.deepEqual(Object.keys(ctx.api.invalidFields.value), [])
  assert.deepEqual(Object.keys(ctx.api.serverFieldErrors.value), [])
  assert.match(sfc, /v-if="serverConfigDefaults.key"/)
  assert.match(sfc, /@click="useServerKey"/)
  const { descriptor } = parse(sfc)
  const script = compileScript(descriptor, { id: 'custom-client-regression' })
  const template = compileTemplate({ source: descriptor.template.content, filename: 'index.vue', id: 'custom-client-regression', compilerOptions: { bindingMetadata: script.bindings } })
  assert.deepEqual(template.errors, [])
  assert.match(between('    return {\n      form, formRef', '\n  },\n})'), /\bserverConfigDefaults,/) // setup return, not merely declaration
})

test('ARIA reflects server field errors without changing required policy', async () => {
  const ctx = fixture()
  const attributes = {}
  ctx.document = { getElementById: id => id === 'custom-client-server_ip-input' ? {
    setAttribute: (name, value) => { attributes[name] = value },
    removeAttribute: name => { delete attributes[name] },
  } : null }
  ctx.api.invalidFields.value = { server_ip: ['CustomClientHostInvalidFormat'] }
  await ctx.api.syncFieldAria()
  assert.equal(attributes['aria-required'], 'false')
  assert.equal(attributes['aria-invalid'], 'true')
  assert.equal(attributes['aria-describedby'], 'custom-client-server_ip-error')
  ctx.api.invalidFields.value = {}
  await ctx.api.syncFieldAria()
  assert.equal(attributes['aria-invalid'], 'false')
  assert.equal(attributes['aria-describedby'], undefined)
})

test('every displayable field binds the server error to its form item', () => {
  for (const field of ['platform', 'version', 'app_name', 'server_ip', 'key', 'api_server', 'relay_server', 'permanent_password']) {
    assert.ok(
      sfc.includes(`serverFieldError('${field}')`),
      `field ${field} must bind :error to the server message`
    )
  }
})

test('server still rejects/accepts the #69 IPv6 and URL cases (Go authority)', () => {
  const service = readFileSync(new URL('../../api/service/custom_build_spec.go', import.meta.url), 'utf8')
  const goFunction = (source, name) => {
    const match = source.match(new RegExp(`func ${name}\\([^]*?\\n\\}`))
    assert.ok(match, `missing Go function ${name}`)
    return match[0]
  }
  const dir = mkdtempSync(join(process.env.TMPDIR || tmpdir(), 'deskforge-validator-'))
  try {
    writeFileSync(join(dir, 'main.go'), `package main
import ("encoding/json"; "fmt"; "net"; "net/url"; "strconv"; "strings"; "os")
${goFunction(service, 'validateEndpoint')}
${goFunction(service, 'validateHost')}
${goFunction(service, 'validateAPIURL')}
func main() {
  var input map[string][]string
  if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil { panic(err) }
  output := map[string][]bool{}
  for _, value := range input["endpoints"] { output["endpoints"] = append(output["endpoints"], validateEndpoint("server_ip", value) == nil) }
  for _, value := range input["urls"] { output["urls"] = append(output["urls"], validateAPIURL("api_server", value) == nil) }
  _ = fmt.Sprint
  _ = net.ParseIP
  _ = url.Parse
  _ = strconv.Itoa
  _ = strings.TrimSpace
  if err := json.NewEncoder(os.Stdout).Encode(output); err != nil { panic(err) }
}`)
    const endpoints = ['1:2:3:4:5:6:7::8', '1.2.3.4::1', '1:2:3:4:5:6:1.2.3.4']
    const urls = ['http://host\\path', 'http:///host', 'http://ho%41st', 'http://host:65536', 'http://[fe80::1%25eth0]:80']
    const result = spawnSync('go', ['run', join(dir, 'main.go')], {
      input: JSON.stringify({ endpoints, urls }), encoding: 'utf8',
      env: { ...process.env, GOWORK: 'off' },
    })
    assert.equal(result.status, 0, result.stderr)
    const go = JSON.parse(result.stdout)
    // These are the exact #69 divergent inputs. The UI no longer produces its own
    // verdict for them; the server (Go) is the contract, and the UI only renders
    // the field it reports.
    assert.deepEqual(go.endpoints, [false, false, true])
    assert.deepEqual(go.urls, [false, false, false, true, true])
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

// --- issue #82: public_* env prefill precedence + Local-IP picker ---
// The defaults live in serverConfigDefaults and are computed in loadConfig;
// applyServerConfigDefaults stays a "fill only empty, non-explicit fields"
// function. These tests exercise the SFC's real slices, not a copy.

function configDefaultsFixture(cfg) {
  const context = {
    reactive, ref, T: value => value,
    form: reactive(formState()),
    serverConfigDefaults: reactive({ server_ip: '', key: '', api_server: '', relay_server: '' }),
    explicitPresetFields: ref(new Set()),
    fetchConfig: async () => ({ data: cfg }),
    lifecycleGuard: { mounted: true, start: () => 1, isCurrent: () => true },
    console,
  }
  vm.createContext(context)
  vm.runInContext(
    between('    const applyServerConfigDefaults =', '    const loadPresetIntoForm') +
    between('    const loadConfig =', '    onMounted') + `
    globalThis.api = { applyServerConfigDefaults, loadConfig, serverConfigDefaults, explicitPresetFields };
  `, context)
  return context
}

test('builder defaults prefer the env-configured public_* addresses over operational ones', async () => {
  const ctx = configDefaultsFixture({
    id_server: 'id-internal:21116',
    key: 'server-key',
    api_server: 'http://api-internal:21114',
    relay_server: 'relay-internal:21117',
    public_id_server: 'id.example.com:21116',
    public_api_server: 'https://api.example.com',
    public_relay_server: 'relay.example.com:21117',
  })
  await ctx.api.loadConfig()
  assert.equal(ctx.api.serverConfigDefaults.server_ip, 'id.example.com:21116')
  assert.equal(ctx.api.serverConfigDefaults.api_server, 'https://api.example.com')
  assert.equal(ctx.api.serverConfigDefaults.relay_server, 'relay.example.com:21117')
  // key stays operational-only.
  assert.equal(ctx.api.serverConfigDefaults.key, 'server-key')
  // The computed defaults are the values that fill the form.
  assert.equal(ctx.form.server_ip, 'id.example.com:21116')
  assert.equal(ctx.form.api_server, 'https://api.example.com')
  assert.equal(ctx.form.relay_server, 'relay.example.com:21117')
})

test('builder defaults fall back to operational values when public_* are absent', async () => {
  const ctx = configDefaultsFixture({
    id_server: 'id-internal:21116',
    key: 'server-key',
    api_server: 'http://api-internal:21114',
    relay_server: 'relay-internal:21117',
  })
  await ctx.api.loadConfig()
  assert.equal(ctx.api.serverConfigDefaults.server_ip, 'id-internal:21116')
  assert.equal(ctx.api.serverConfigDefaults.api_server, 'http://api-internal:21114')
  assert.equal(ctx.api.serverConfigDefaults.relay_server, 'relay-internal:21117')
  assert.equal(ctx.form.server_ip, 'id-internal:21116')
})

test('each builder default falls back independently on a mixed config', async () => {
  const ctx = configDefaultsFixture({
    id_server: 'id-internal:21116',
    key: 'server-key',
    api_server: 'http://api-internal:21114',
    relay_server: 'relay-internal:21117',
    public_id_server: 'id.example.com:21116',
    public_api_server: '',
  })
  await ctx.api.loadConfig()
  assert.equal(ctx.api.serverConfigDefaults.server_ip, 'id.example.com:21116')
  assert.equal(ctx.api.serverConfigDefaults.api_server, 'http://api-internal:21114')
  assert.equal(ctx.api.serverConfigDefaults.relay_server, 'relay-internal:21117')
})

test('public_* precedence never overwrites an explicit preset field', async () => {
  const ctx = configDefaultsFixture({
    id_server: 'id-internal:21116',
    key: 'server-key',
    api_server: 'http://api-internal:21114',
    relay_server: 'relay-internal:21117',
    public_id_server: 'id.example.com:21116',
    public_api_server: 'https://api.example.com',
    public_relay_server: 'relay.example.com:21117',
  })
  // A loaded preset stored server_ip explicitly; it must survive defaults.
  ctx.form.server_ip = 'preset-host'
  ctx.api.explicitPresetFields.value = new Set(['server_ip'])
  await ctx.api.loadConfig()
  assert.equal(ctx.form.server_ip, 'preset-host')
  // Empty non-explicit fields are still filled (preserved semantics).
  assert.equal(ctx.form.api_server, 'https://api.example.com')
  assert.equal(ctx.form.relay_server, 'relay.example.com:21117')
})

test('the Local IP button targets Host, Relay and API server and the dialog is bound', () => {
  for (const field of ['server_ip', 'api_server', 'relay_server']) {
    assert.ok(
      sfc.includes(`@click="openLocalAddressPicker('${field}')"`),
      `field ${field} must expose the Local IP picker button`
    )
  }
  assert.match(sfc, /v-model="localAddressPicker\.visible"/)
  // The dialog must say where the addresses came from (Docker bridge caveat).
  assert.match(sfc, /candidate\.interface/)
  assert.match(sfc, /CustomClientLocalIpSource/)
  assert.match(sfc, /CustomClientLocalIpNote/)
})

test('opening the Local IP picker loads the server-resolved candidates', async () => {
  const ctx = fixture()
  const calls = []
  ctx.fetchServerAddresses = async () => {
    calls.push(true)
    return { data: { addresses: [{ interface: 'eth0', address: '172.17.0.1', family: 'ipv4' }] } }
  }
  await ctx.api.openLocalAddressPicker('server_ip')
  assert.equal(calls.length, 1) // resolution is the server-side endpoint's job
  assert.equal(ctx.api.localAddressPicker.visible, true)
  assert.equal(ctx.api.localAddressPicker.field, 'server_ip')
  assert.equal(ctx.api.localAddressPicker.loading, false)
  assert.equal(ctx.api.localAddressPicker.error, '')
  assert.deepEqual(ctx.api.localAddressPicker.addresses, [
    { interface: 'eth0', address: '172.17.0.1', family: 'ipv4' },
  ])
})

test('a picker failure reports a bounded message and leaves the field untouched', async () => {
  const ctx = fixture()
  ctx.fetchServerAddresses = async () => { throw new Error('boom') }
  await ctx.api.openLocalAddressPicker('server_ip')
  assert.equal(ctx.api.localAddressPicker.error, 'CustomClientLocalIpLoadError')
  assert.equal(ctx.api.localAddressPicker.addresses.length, 0)
  assert.equal(ctx.form.server_ip, '')
})

test('selecting a candidate prefills only its field and the field stays editable', async () => {
  const ctx = fixture()
  ctx.fetchServerAddresses = async () => ({
    data: { addresses: [{ interface: 'eth0', address: '172.17.0.1', family: 'ipv4' }] },
  })
  await ctx.api.openLocalAddressPicker('relay_server')
  await ctx.api.useLocalAddress(ctx.api.localAddressPicker.addresses[0])
  assert.equal(ctx.form.relay_server, '172.17.0.1')
  assert.equal(ctx.form.server_ip, '') // other fields untouched
  assert.equal(ctx.form.api_server, '')
  assert.equal(ctx.api.localAddressPicker.visible, false)
  // Prefill only: the value is a plain assignment and remains editable.
  ctx.form.relay_server = 'edited.example.com:21117'
  assert.equal(ctx.form.relay_server, 'edited.example.com:21117')
})

test('api_server prefill wraps the address in the URL scheme the field carries', async () => {
  const ctx = fixture()
  ctx.fetchServerAddresses = async () => ({
    data: { addresses: [{ interface: 'eth0', address: '10.0.0.5', family: 'ipv4' }] },
  })
  // Empty field defaults to https://.
  await ctx.api.openLocalAddressPicker('api_server')
  await ctx.api.useLocalAddress(ctx.api.localAddressPicker.addresses[0])
  assert.equal(ctx.form.api_server, 'https://10.0.0.5')
  // An explicit http:// choice is preserved across another prefill.
  ctx.form.api_server = 'http://old-internal:21114'
  await ctx.api.openLocalAddressPicker('api_server')
  await ctx.api.useLocalAddress(ctx.api.localAddressPicker.addresses[0])
  assert.equal(ctx.form.api_server, 'http://10.0.0.5')
  // Host/Relay fields get the bare address, never a URL wrapper.
  await ctx.api.openLocalAddressPicker('server_ip')
  await ctx.api.useLocalAddress(ctx.api.localAddressPicker.addresses[0])
  assert.equal(ctx.form.server_ip, '10.0.0.5')
})

test('api_server prefill brackets IPv6 literals so the composed URL passes server validation', async () => {
  const ctx = fixture()
  ctx.fetchServerAddresses = async () => ({
    data: { addresses: [{ interface: 'eth0', address: 'fd00::1', family: 'ipv6' }] },
  })
  await ctx.api.openLocalAddressPicker('api_server')
  await ctx.api.useLocalAddress(ctx.api.localAddressPicker.addresses[0])
  // Bracketed literal: https://fd00::1 is deterministically rejected by
  // validateAPIURL (url.ParseRequestURI "invalid port"), https://[fd00::1] is not.
  assert.equal(ctx.form.api_server, 'https://[fd00::1]')
  // Bracketing composes with a preserved http:// scheme too.
  ctx.form.api_server = 'http://old-internal:21114'
  await ctx.api.openLocalAddressPicker('api_server')
  await ctx.api.useLocalAddress(ctx.api.localAddressPicker.addresses[0])
  assert.equal(ctx.form.api_server, 'http://[fd00::1]')
  // Host/Relay keep the bare address contract for the same IPv6 candidate.
  await ctx.api.openLocalAddressPicker('server_ip')
  await ctx.api.useLocalAddress(ctx.api.localAddressPicker.addresses[0])
  assert.equal(ctx.form.server_ip, 'fd00::1')
})

test('the Local IP picker is exposed through the setup return, not merely declared', () => {
  const setupReturn = between('    return {\n      form, formRef', '\n  },\n})')
  for (const name of ['localAddressPicker', 'openLocalAddressPicker', 'useLocalAddress', 'closeLocalAddressPicker']) {
    assert.match(setupReturn, new RegExp(`\\b${name}\\b`), `${name} must be returned from setup`)
  }
})
