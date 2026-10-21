// Run with: node --test tests/*.test.mjs
// Issue #96 regression guard: a failing admin-ui HTTP request must never print
// the raw axios error.
//
// A raw AxiosError carries `error.config`, and the shared axios request
// interceptor attaches the session token to EVERY request as
// `config.headers['api-token']` (src/utils/request.js). `config.data` carries
// the request body. Printing the object therefore prints the API token and the
// request body into devtools and into any console screenshot taken for support.
//
// The same holds for a JSON.parse failure: the V8 SyntaxError message quotes an
// excerpt of the offending input, so a preset's custom_json (which can hold
// permanent_password/key) must never reach the console through a message either.
//
// These tests drive the REAL implementations. Every audited module is read from
// source, the relevant slice is extracted by string boundaries, evaluated in a
// node:vm context with a stubbed console, and then called with an axios error
// whose config carries a canary token and a canary body. Nothing here
// re-implements the production code path: if a site starts logging the raw
// object again, the runtime assertions below fail.
//
// The same pattern (read the SFC, extract a slice by string boundaries, run it
// in a node:vm context with a stubbed console) is used by
// tests/custom-client-validation.test.mjs.
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { ref, reactive, nextTick } from 'vue'
import axios from 'axios'

// Canary secrets. These values appear nowhere else in the repository: if one of
// them shows up in a console log, an axios error object reached the console.
const LEAK_TOKEN = 'LEAK_CANARY_API_TOKEN_96'
const LEAK_BODY = 'LEAK_CANARY_BODY_96'
// The bounded diagnostic must still carry the useful part of the failure.
const BOUNDED_MESSAGE = 'Request failed with status code 500'

const readSource = (relative) => readFileSync(new URL(relative, import.meta.url), 'utf8')

// Slice a source file between two literal boundaries, the way the existing
// custom-client test does. The end marker is exclusive.
const between = (source, start, end) => {
  assert.ok(source.includes(start) && source.includes(end), `missing source boundary: ${start}`)
  return source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start)))
}

// A real axios error whose config retains the request body and the api-token
// header — the exact shape the shared response interceptor rejects with.
const axiosErrorWithSecrets = () => new axios.AxiosError(
  BOUNDED_MESSAGE,
  'ERR_BAD_RESPONSE',
  { data: LEAK_BODY, headers: { 'api-token': LEAK_TOKEN } },
  {},
  { status: 500, data: { code: 101, message: 'OperationFailed', data: null } },
)

// Capture every console channel so a site that logs through warn/error/info
// is covered by the same assertions.
const makeConsole = () => {
  const calls = []
  const record = (method) => (...args) => { calls.push({ method, args }) }
  return {
    calls,
    console: {
      error: record('error'),
      warn: record('warn'),
      log: record('log'),
      info: record('info'),
      debug: record('debug'),
    },
  }
}

// Render a logged value the way a browser console actually displays it.
//
// This must NOT be plain JSON.stringify: a real Error's `message` and `stack`
// are non-enumerable, so JSON.stringify(error) yields "{}" and would hide the
// exact leak this file guards (the V8 JSON.parse message quotes an excerpt of
// the offending input). A browser console renders the error's message and
// stack, so this renderer does the same for Error-like values.
const renderLoggedValue = (value) => {
  if (typeof value === 'string') return value
  if (value instanceof Error) {
    return `${value.name}: ${value.message}${value.stack ? `\n${value.stack}` : ''}`
  }
  try { return JSON.stringify(value) } catch { return String(value) }
}

const serializedLogs = (calls) => calls
  .map(({ args }) => args.map(renderLoggedValue).join(' '))
  .join('\n')

// Half 1 of the contract: the raw config/headers/body must not be present in
// what was logged, and no raw error object may be handed to the console.
const assertNoRequestSecretsLogged = (calls, label) => {
  const rendered = serializedLogs(calls)
  assert.equal(rendered.includes(LEAK_TOKEN), false,
    `${label}: the api-token must never be logged, got: ${rendered}`)
  assert.equal(rendered.includes(LEAK_BODY), false,
    `${label}: the request body must never be logged, got: ${rendered}`)
  for (const { method, args } of calls) {
    for (const arg of args) {
      assert.equal(typeof arg, 'string',
        `${label}: console.${method} must not receive a raw error object`)
    }
  }
}

// Half 2 of the contract: a bounded diagnostic is still logged, and it carries
// the failure's message so the log stays useful.
const assertBoundedDiagnosticLogged = (calls, label) => {
  assert.ok(calls.length > 0, `${label}: a bounded diagnostic must still be logged`)
  const rendered = serializedLogs(calls)
  assert.ok(rendered.includes(BOUNDED_MESSAGE),
    `${label}: the bounded diagnostic must carry the error message, got: ${rendered}`)
}

// Supplementary source check: inside an audited function, no console call may
// receive the caught error identifier in a form that can carry request
// internals. This is a belt-and-braces guard on top of the runtime assertions
// above, not a replacement for them.
//
// Flagged (leaks):
//   console.error(e)                 — the raw object
//   console.error('m', e)            — the raw object alongside a message
//   console.error('m', e, more)      — the raw object as a NON-final argument
//   console.error(`m ${e}`)          — template interpolation
//   console.error('m', e?.config)    — a property of the raw object
//
// NOT flagged (bounded by construction):
//   console.error('m', e?.message)   — the error's own message
//   console.error('m', e.message)
//   console.error('m', e?.name)      — a class-level identifier
//   console.error('m', e?.name || 'Error')
//
// The argument list is extracted by counting parentheses, so a message argument
// that itself contains parentheses does not truncate the scan.
const CONSOLE_CALL = /console\.\w+\(/g

// Split a call's argument list on top-level commas (commas nested in
// parentheses, brackets, braces, or strings do not separate arguments).
const splitArguments = (argumentText) => {
  const args = []
  let depth = 0
  let quote = null
  let current = ''
  for (let i = 0; i < argumentText.length; i++) {
    const ch = argumentText[i]
    const prev = argumentText[i - 1]
    if (quote) {
      current += ch
      if (ch === quote && prev !== '\\') quote = null
      continue
    }
    if (ch === '"' || ch === "'" || ch === '`') { quote = ch; current += ch; continue }
    if (ch === '(' || ch === '[' || ch === '{') { depth++; current += ch; continue }
    if (ch === ')' || ch === ']' || ch === '}') { depth--; current += ch; continue }
    if (ch === ',' && depth === 0) { args.push(current); current = ''; continue }
    current += ch
  }
  if (current.trim() !== '') args.push(current)
  return args.map(arg => arg.trim())
}

// Find the argument list of every console.<method>(...) call in the source.
const consoleCallArguments = (source) => {
  const calls = []
  let match
  CONSOLE_CALL.lastIndex = 0
  while ((match = CONSOLE_CALL.exec(source)) !== null) {
    let depth = 1
    let i = match.index + match[0].length
    let quote = null
    for (; i < source.length && depth > 0; i++) {
      const ch = source[i]
      const prev = source[i - 1]
      if (quote) {
        if (ch === quote && prev !== '\\') quote = null
        continue
      }
      if (ch === '"' || ch === "'" || ch === '`') { quote = ch; continue }
      if (ch === '(') depth++
      else if (ch === ')') depth--
    }
    if (depth === 0) {
      calls.push({
        call: match[0].replace(/\($/, ''),
        args: splitArguments(source.slice(match.index + match[0].length, i - 1)),
      })
      CONSOLE_CALL.lastIndex = i
    }
  }
  return calls
}

// An argument that references the caught error identifier in an unbounded way.
// A bare identifier, a member access that is not a bounded scalar property, or
// a template literal that interpolates the identifier.
//
// The bounded scalar properties are `message` and `name`: a message is the
// error's own text and a name is a class-level identifier ("SyntaxError").
// `String(e)` is bounded too — it stringifies the error rather than passing
// the object on.
const ERROR_IDENTIFIER = /\b(err|e)\b/g
const BOUNDED_ERROR_PROPERTIES = ['message', 'name']

const isUnboundedErrorReference = (argument) => {
  const text = argument.trim()
  if (text === '') return false
  // Template interpolation: `... ${e} ...` (or ${e?.message}, which is bounded).
  if (text.includes('${')) {
    const interpolations = [...text.matchAll(/\$\{([^}]*)\}/g)].map(m => m[1].trim())
    return interpolations.some(isUnboundedErrorReference)
  }
  // Every occurrence of the error identifier is examined, so an argument that
  // mixes a bounded and an unbounded reference is still flagged.
  let unbounded = false
  ERROR_IDENTIFIER.lastIndex = 0
  let match
  while ((match = ERROR_IDENTIFIER.exec(text)) !== null) {
    // `String(e)` stringifies the error: bounded, not a raw object.
    if (/\bString\(\s*$/.test(text.slice(0, match.index))) continue
    const rest = text.slice(match.index + match[0].length)
    const property = rest.match(/^\s*(?:\?\.)?\.?\s*([A-Za-z_$][\w$]*)/)
    if (property) {
      if (!BOUNDED_ERROR_PROPERTIES.includes(property[1])) unbounded = true
    } else {
      // A bare identifier reference.
      unbounded = true
    }
  }
  return unbounded
}

const assertNoRawErrorArgument = (source, label) => {
  for (const { call, args } of consoleCallArguments(source)) {
    for (const argument of args) {
      assert.equal(isUnboundedErrorReference(argument), false,
        `${label}: ${call}(...) must not receive the caught error unbounded, got: ${argument}`)
    }
  }
}

// --- fixtures ---------------------------------------------------------------

// src/views/custom-client/index.vue: the four request paths named by issue #96
// (custom_preset/list, custom_build/list, custom_preset/delete,
// custom_build/delete), the config/all fetch, and the preset custom_json parse
// (a JSON.parse failure, whose V8 message quotes an excerpt of the input).
const customClientSource = readSource('../src/views/custom-client/index.vue')
const customClientSlices = {
  lifecycleGuard: between(customClientSource,
    '    const requestGenerations = Object.create(null)', '    // B-017: при silent=true'),
  loadBuilds: between(customClientSource,
    '    const loadBuilds = async (silent = false) => {', '\n    // B-017: история билдов не обновлялась сама'),
  loadPresets: between(customClientSource,
    '    const loadPresets = async () => {', '\n    // Single source of truth for the fields persisted inside custom_json.'),
  deletePreset: between(customClientSource,
    '    const deletePreset = async (preset) => {', '\n    const submitBuild = async () => {'),
  deleteBuild: between(customClientSource,
    '    const deleteBuild = async (row) => {', '\n    const resetForm = () => {'),
  loadConfig: between(customClientSource,
    '    const loadConfig = async () => {', '\n    onMounted(async () => {'),
  loadPresetIntoForm: between(customClientSource,
    '    const loadPresetIntoForm = async (preset) => {', '\n    const onPresetSelect = (id) => {'),
}

function customClientFixture (apis) {
  const { calls, console } = makeConsole()
  const context = {
    ref, reactive, nextTick, axios,
    console, calls,
    T: value => value,
    onUnmounted: () => {},
    ElMessage: { success: () => {} },
    ElMessageBox: { confirm: async () => true },
    list: apis.list,
    listPresets: apis.listPresets,
    removePreset: apis.removePreset,
    remove: apis.remove,
    fetchConfig: apis.fetchConfig,
    applyServerConfigDefaults: () => {},
    builds: ref([]),
    total: ref(0),
    loading: ref(false),
    presets: ref([]),
    selectedPresetId: ref(null),
    presetSelectRef: ref(null),
    serverConfigDefaults: reactive({ key: '' }),
    publicApiServerMissing: ref(false),
    page: ref(1),
    pageSize: ref(10),
    // State the preset loader touches.
    form: reactive({}),
    formRef: ref(null),
    explicitPresetFields: ref(new Set()),
    invalidFields: ref({}),
    serverFieldErrors: ref({}),
    resetFormFields: () => {},
    syncFieldAria: () => {},
    ensurePolling: () => {},
    announceBuildStatusChanges: () => {},
    hasActiveBuilds: () => false,
    PRESET_FIELDS: [],
  }
  vm.createContext(context)
  vm.runInContext(
    customClientSlices.lifecycleGuard +
    customClientSlices.loadBuilds +
    customClientSlices.loadPresets +
    customClientSlices.deletePreset +
    customClientSlices.deleteBuild +
    customClientSlices.loadConfig +
    customClientSlices.loadPresetIntoForm + `
    lifecycleGuard.mounted = true;
    globalThis.api = { lifecycleGuard, loadBuilds, loadPresets, deletePreset, deleteBuild, loadConfig, loadPresetIntoForm };
  `, context)
  return context
}

// src/components/dashboard/ServerHealth.vue — the dashboard/health poll.
const serverHealthSource = readSource('../src/components/dashboard/ServerHealth.vue')
const serverHealthSlice = between(serverHealthSource,
  '    const REFRESH_SECONDS = 15', '\n    const refreshNow = () => {')

function serverHealthFixture (fetchHealth) {
  const { calls, console } = makeConsole()
  const context = { ref, console, calls, T: value => value, fetchHealth }
  vm.createContext(context)
  vm.runInContext(serverHealthSlice + '\n    globalThis.api = { load };', context)
  return context
}

// src/views/index/index.vue — the dashboard stats / login-log load.
const dashboardSource = readSource('../src/views/index/index.vue')
const dashboardSlice = between(dashboardSource,
  '    const loadData = async () => {', '\n    const connectNative = () => {')

function dashboardFixture (fetchStats, fetchLoginLogs) {
  const { calls, console } = makeConsole()
  const context = {
    ref, console, calls, T: value => value,
    fetchStats, fetchLoginLogs,
    stats: ref({}),
    recentLogs: ref([]),
    loading: ref(true),
  }
  vm.createContext(context)
  vm.runInContext(dashboardSlice + '\n    globalThis.api = { loadData };', context)
  return context
}

// src/views/server/config.vue — the /config/all load.
const serverConfigSource = readSource('../src/views/server/config.vue')
const serverConfigSlice = between(serverConfigSource,
  '    const loadData = async () => {', '\n    onMounted(loadData)')

function serverConfigFixture (fetchAllConfig) {
  const { calls, console } = makeConsole()
  const context = {
    ref, console, calls, T: value => value,
    onMounted: () => {},
    fetchAllConfig,
    cfg: ref({}),
    loading: ref(true),
  }
  vm.createContext(context)
  vm.runInContext(serverConfigSlice + '\n    globalThis.api = { loadData };', context)
  return context
}

// src/store/app.js — the getAdminConfig action.
const appStoreSource = readSource('../src/store/app.js')
const appStoreSlice = between(appStoreSource,
  '    getAdminConfig () {', '\n    replaceAdminTitle (newTitle) {')

function appStoreFixture (admin) {
  const { calls, console } = makeConsole()
  const context = { console, calls, admin }
  vm.createContext(context)
  vm.runInContext(
    `const __store = { ${appStoreSlice} };\n    globalThis.api = __store;`, context)
  return context
}

// --- the four request paths named by issue #96 -------------------------------

test('custom_build/list failure does not log the raw axios error', async () => {
  const ctx = customClientFixture({
    list: async () => { throw axiosErrorWithSecrets() },
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  await ctx.api.loadBuilds()
  assertNoRequestSecretsLogged(ctx.calls, 'custom_build/list')
  assertBoundedDiagnosticLogged(ctx.calls, 'custom_build/list')
  assertNoRawErrorArgument(customClientSlices.loadBuilds, 'custom_build/list')
})

test('custom_preset/list failure does not log the raw axios error', async () => {
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => { throw axiosErrorWithSecrets() },
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  await ctx.api.loadPresets()
  assertNoRequestSecretsLogged(ctx.calls, 'custom_preset/list')
  assertBoundedDiagnosticLogged(ctx.calls, 'custom_preset/list')
  assertNoRawErrorArgument(customClientSlices.loadPresets, 'custom_preset/list')
})

test('custom_preset/delete failure does not log the raw axios error', async () => {
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => ({}),
    removePreset: async () => { throw axiosErrorWithSecrets() },
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  await ctx.api.deletePreset({ id: 7 })
  assertNoRequestSecretsLogged(ctx.calls, 'custom_preset/delete')
  assertBoundedDiagnosticLogged(ctx.calls, 'custom_preset/delete')
  assertNoRawErrorArgument(customClientSlices.deletePreset, 'custom_preset/delete')
})

test('custom_build/delete failure does not log the raw axios error', async () => {
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => { throw axiosErrorWithSecrets() },
    fetchConfig: async () => ({}),
  })
  await ctx.api.deleteBuild({ id: 9 })
  assertNoRequestSecretsLogged(ctx.calls, 'custom_build/delete')
  assertBoundedDiagnosticLogged(ctx.calls, 'custom_build/delete')
  assertNoRawErrorArgument(customClientSlices.deleteBuild, 'custom_build/delete')
})

// --- the preset custom_json parse path ---------------------------------------

// A preset's custom_json can hold the secret-bearing fields (permanent_password
// and key). A malformed one makes JSON.parse throw a SyntaxError whose message
// quotes an EXCERPT of the offending input, so the excerpt — not the whole
// value — is what leaks.
//
// The canary is chosen so that the excerpt V8 produces is a recognisable
// substring of it. Measured on Node 20, 22 and 24, parsing
// `{"permanent_password":ZQX7LEAKCANARY96}` yields exactly:
//
//   Unexpected token 'Z', ..."password":ZQX7LEAKCA"... is not valid JSON
//
// i.e. the excerpt carries the first 10 characters of the value
// ("ZQX7LEAKCA"). Asserting on the FULL value alone would not discriminate —
// the excerpt is a fragment — so the fragment is asserted as well.
const PRESET_SECRET_CANARY = 'ZQX7LEAKCANARY96'
const PRESET_SECRET_FRAGMENT = 'ZQX7LEAKCA'

test('a malformed preset custom_json never logs the secret or any fragment of it', async () => {
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  // The unquoted value makes V8 quote an excerpt of it in the SyntaxError.
  const malformed = `{"permanent_password":${PRESET_SECRET_CANARY}}`
  await ctx.api.loadPresetIntoForm({ id: 1, custom_json: malformed })

  const rendered = serializedLogs(ctx.calls)
  assert.equal(rendered.includes(PRESET_SECRET_CANARY), false,
    `the preset secret must never be logged, got: ${rendered}`)
  assert.equal(rendered.includes(PRESET_SECRET_FRAGMENT), false,
    `no fragment of the preset secret may be logged, got: ${rendered}`)
  // The bounded diagnostic must still be present: the operation marker and the
  // error TYPE (never the message, which is derived from the input).
  assert.ok(rendered.includes('preset custom_json parse error'),
    `the operation marker must still be logged, got: ${rendered}`)
  assert.ok(rendered.includes('SyntaxError'),
    `the error type must still be logged, got: ${rendered}`)
  // No raw error object may reach the console either.
  for (const { method, args } of ctx.calls) {
    for (const arg of args) {
      assert.equal(typeof arg, 'string',
        `console.${method} must not receive a raw error object`)
    }
  }
  assertNoRawErrorArgument(customClientSlices.loadPresetIntoForm, 'preset custom_json parse')
})

test('a malformed preset custom_json is diagnosed without the input-derived message', async () => {
  // The message is derived from the input, so it must not be logged at all —
  // not even a bounded slice of it. This asserts the message itself is absent
  // while the class-level error name is present.
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  const malformed = `{"permanent_password":${PRESET_SECRET_CANARY}}`
  await ctx.api.loadPresetIntoForm({ id: 1, custom_json: malformed })

  const rendered = serializedLogs(ctx.calls)
  assert.equal(rendered.includes('Unexpected token'), false,
    `the input-derived parse message must never be logged, got: ${rendered}`)
  assert.equal(rendered.includes('is not valid JSON'), false,
    `the input-derived parse message must never be logged, got: ${rendered}`)
  assert.ok(rendered.includes('SyntaxError'),
    `the error type must still be logged, got: ${rendered}`)
})

// --- the remaining audited request failures ----------------------------------

test('the custom-client config load failure does not log the raw axios error', async () => {
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => { throw axiosErrorWithSecrets() },
  })
  await ctx.api.loadConfig()
  assertNoRequestSecretsLogged(ctx.calls, 'config/all')
  assertBoundedDiagnosticLogged(ctx.calls, 'config/all')
  assertNoRawErrorArgument(customClientSlices.loadConfig, 'config/all')
})

test('the server-health poll failure does not log the raw axios error', async () => {
  const ctx = serverHealthFixture(async () => { throw axiosErrorWithSecrets() })
  await ctx.api.load()
  assertNoRequestSecretsLogged(ctx.calls, 'dashboard/health')
  assertBoundedDiagnosticLogged(ctx.calls, 'dashboard/health')
  assertNoRawErrorArgument(serverHealthSlice, 'dashboard/health')
})

test('the dashboard load failure does not log the raw axios error', async () => {
  const ctx = dashboardFixture(
    async () => { throw axiosErrorWithSecrets() },
    async () => ({}),
  )
  await ctx.api.loadData()
  assertNoRequestSecretsLogged(ctx.calls, 'dashboard/stats')
  assertBoundedDiagnosticLogged(ctx.calls, 'dashboard/stats')
  assertNoRawErrorArgument(dashboardSlice, 'dashboard/stats')
})

test('the server-config load failure does not log the raw axios error', async () => {
  const ctx = serverConfigFixture(async () => { throw axiosErrorWithSecrets() })
  await ctx.api.loadData()
  assertNoRequestSecretsLogged(ctx.calls, 'config/all')
  assertBoundedDiagnosticLogged(ctx.calls, 'config/all')
  assertNoRawErrorArgument(serverConfigSlice, 'config/all')
})

test('the admin-config store load failure does not log the raw axios error', async () => {
  const ctx = appStoreFixture(async () => { throw axiosErrorWithSecrets() })
  await ctx.api.getAdminConfig()
  assertNoRequestSecretsLogged(ctx.calls, 'config/admin')
  assertBoundedDiagnosticLogged(ctx.calls, 'config/admin')
  assertNoRawErrorArgument(appStoreSlice, 'config/admin')
})

// --- preserved behaviour -----------------------------------------------------
// The fix must not change what the UI does. These cases pin the neighbouring
// behaviour of the same code paths: a successful request still populates the
// list, a cancelled confirm stays silent, and the stale-response guard still
// suppresses the log for a superseded request.

test('a successful custom_build/list still populates the build list', async () => {
  const ctx = customClientFixture({
    list: async () => ({ data: { list: [{ id: 1 }], total: 1 } }),
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  await ctx.api.loadBuilds()
  assert.equal(ctx.builds.value.length, 1)
  assert.equal(ctx.total.value, 1)
  assert.equal(ctx.calls.length, 0, 'a successful request must not log anything')
})

test('a successful custom_preset/list still populates the preset list', async () => {
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => ({ data: { list: [{ id: 3 }] } }),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  await ctx.api.loadPresets()
  assert.equal(ctx.presets.value.length, 1)
  assert.equal(ctx.calls.length, 0)
})

test('a cancelled delete confirm stays silent', async () => {
  const ctx = customClientFixture({
    list: async () => ({}),
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  // ElMessageBox.confirm rejects with the literal 'cancel' when dismissed.
  ctx.ElMessageBox.confirm = async () => { throw 'cancel' }
  await ctx.api.deletePreset({ id: 7 })
  await ctx.api.deleteBuild({ id: 9 })
  assert.equal(ctx.calls.length, 0, 'a dismissed confirm is not a failure and must stay silent')
})

test('a superseded request is not logged at all', async () => {
  // The per-operation generation guard means a response from a stale request
  // must neither update state nor reach the console: only the CURRENT
  // generation logs, so two overlapping loads produce exactly one bounded
  // diagnostic, not two.
  const ctx = customClientFixture({
    list: async () => { throw axiosErrorWithSecrets() },
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  // Start a first load, then start a second one so the first generation is stale.
  const first = ctx.api.loadBuilds()
  const second = ctx.api.loadBuilds()
  await Promise.all([first, second])
  assert.equal(ctx.calls.length, 1,
    'only the current generation may log a bounded diagnostic')
  assertNoRequestSecretsLogged(ctx.calls, 'superseded custom_build/list')
})

test('a non-axios failure still logs a bounded diagnostic with its message', async () => {
  // A genuinely unexpected JS failure (no axios markers) must still be
  // visible to a developer, but only as a bounded string.
  const ctx = customClientFixture({
    list: async () => { throw new Error('boom in list') },
    listPresets: async () => ({}),
    removePreset: async () => ({}),
    remove: async () => ({}),
    fetchConfig: async () => ({}),
  })
  await ctx.api.loadBuilds()
  assert.equal(ctx.calls.length, 1)
  const rendered = serializedLogs(ctx.calls)
  assert.ok(rendered.includes('boom in list'),
    `the bounded diagnostic must carry the message, got: ${rendered}`)
})
