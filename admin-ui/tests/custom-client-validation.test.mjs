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
function fixture(platform = 'linux') {
  const logs = []
  const context = {
    reactive, ref, computed, nextTick, watch, atob, btoa, URL, T: value => value,
    form: reactive({ platform, hide_cm: false }), formRef: ref(null),
    serverConfigDefaults: reactive({ key: '' }),
    document: { getElementById: () => null },
    axios, console: { error: (...args) => logs.push(args) },
    logs,
  }
  vm.createContext(context)
  vm.runInContext(between('    const requiredFieldNames', '    const useServerKey') +
    between('    const useServerKey', '    const isFieldInvalid') +
    between('    const isFieldInvalid', '    const validateBuildForm') + `
    globalThis.api = { rules, requiredFieldSet, invalidFields, serverFieldErrors,
      serverFieldError, serverFieldMessage, applyServerFieldErrors,
      snapshotSubmittedFields, reportCreateBuildFailure,
      useServerKey, isFieldInvalid, isRequiredField, clearFieldError, syncFieldAria };
    globalThis.logs = logs;
  `, context)
  return context
}
const key = Buffer.alloc(32, 7).toString('base64')

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
