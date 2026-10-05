// Run with: node --test tests/custom-client-validation.test.mjs
// Exercise the SFC's actual logic with Vue reactivity, not a validator copy.
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import vm from 'node:vm'
import { reactive, ref, computed, nextTick } from 'vue'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'

const sfc = readFileSync(new URL('../src/views/custom-client/index.vue', import.meta.url), 'utf8')
const between = (start, end) => {
  assert.ok(sfc.includes(start) && sfc.includes(end), `missing SFC boundary: ${start}`)
  return sfc.slice(sfc.indexOf(start), sfc.indexOf(end, sfc.indexOf(start)))
}
function fixture(platform = 'linux') {
  const context = {
    reactive, ref, computed, nextTick, atob, btoa, URL, T: value => value,
    form: reactive({ platform, hide_cm: false }), formRef: ref(null),
    serverConfigDefaults: reactive({ key: '' }), clearFieldError: () => {},
  }
  vm.createContext(context)
  vm.runInContext(between('    const requiredFieldNames', '    const useServerKey') +
    between('    const useServerKey', '    const isFieldInvalid') +
    between('    const isFieldInvalid', '    // formatCheckedFields participates') + `
    globalThis.api = { rules, formatCheckedFields, isFieldFormatInvalid,
      isValidEndpointFormat, isValidApiServerFormat, isValidPublicKeyFormat,
      useServerKey, invalidFields, syncFieldAria };
    globalThis.presetErrors = () => {
      ${between('        const formatInvalidEntries', '        if (Object.keys(formatInvalidEntries)')}
      return formatInvalidEntries;
    };
  `, context)
  return context
}
const key = Buffer.alloc(32, 7).toString('base64')
const endpoints = ['', 'host/', 'ho[st', 'ho]st', 'host/:21116', 'ho[st:21116',
  ' ', '\t', '\r\n', 'example.com', 'under_score', 'localhost', '192.0.2.1',
  'example.com:21116', '192.0.2.1:080', '::1', '[2001:db8::1]:21116']
const urls = ['', ' ', '\t', '\r\n', 'https://example.com', 'http://localhost:21114/path']
const keys = ['', '\r\n', ' ', '\t', key, key + '\r\n', key + ' ']

test('differential evidence from extracted production Go functions', () => {
  const service = readFileSync(new URL('../../api/service/custom_build_spec.go', import.meta.url), 'utf8')
  const config = readFileSync(new URL('../../api/config/rustdesk.go', import.meta.url), 'utf8')
  const goFunction = (source, name) => {
    const match = source.match(new RegExp(`func ${name}\\([^]*?\\n\\}`))
    assert.ok(match, `missing Go function ${name}`)
    return match[0]
  }
  const dir = mkdtempSync(join(process.env.TMPDIR || tmpdir(), 'deskforge-validator-'))
  try {
    const errorType = config.slice(config.indexOf('type PublicKeyConfigurationError'), config.indexOf('// NormalizePublicKey'))
    const size = config.match(/rustDeskPublicKeyBytes = \d+/)?.[0]
    assert.ok(size)
    writeFileSync(join(dir, 'main.go'), `package main
import ("fmt"; "net"; "net/url"; "strconv"; "strings"; "unicode"; "encoding/base64"; "encoding/json"; "os")
${errorType}
const ${size}
${['validateEndpoint', 'validateHost', 'validateAPIURL'].map(n => goFunction(service, n)).join('\n')}
${['NormalizePublicKey', 'ValidatePublicKeyMaterial'].map(n => goFunction(config, n)).join('\n')}
func main() {
  var input map[string][]string
  if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil { panic(err) }
  output := map[string][]bool{}
  for name, values := range input {
    for _, value := range values {
      var valid bool
      switch name {
      case "endpoints": valid = validateEndpoint("server_ip", value) == nil
      case "urls": valid = validateAPIURL("api_server", value) == nil
      case "keys": valid = ValidatePublicKeyMaterial(value) == nil
      case "keyPresence": valid = NormalizePublicKey(value) != ""
      }
      output[name] = append(output[name], valid)
    }
  }
  if err := json.NewEncoder(os.Stdout).Encode(output); err != nil { panic(err) }
}`)
    const result = spawnSync('go', ['run', join(dir, 'main.go')], {
      input: JSON.stringify({ endpoints, urls, keys, keyPresence: keys }), encoding: 'utf8',
      env: { ...process.env, GOWORK: 'off' },
    })
    assert.equal(result.status, 0, result.stderr)
    const go = JSON.parse(result.stdout)
    const { api } = fixture()
    endpoints.forEach((value, i) => assert.equal(api.isValidEndpointFormat(value), go.endpoints[i], `endpoint ${JSON.stringify(value)}`))
    urls.forEach((value, i) => assert.equal(api.isValidApiServerFormat(value), go.urls[i], `URL ${JSON.stringify(value)}`))
    keys.forEach((value, i) => assert.equal(api.isValidPublicKeyFormat(value), !go.keyPresence[i] || go.keys[i], `optional key ${JSON.stringify(value)}`))
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('rules, callbacks and preset highlights validate material whitespace on every platform', () => {
  for (const platform of ['windows', 'linux', 'android']) {
    for (const field of ['server_ip', 'relay_server', 'api_server', 'key']) {
      for (const value of [' ', '\t', ' \r\n']) {
        const ctx = fixture(platform)
        ctx.form[field] = value
        const rules = ctx.api.rules.value[field] || []
        assert.ok(rules.some(rule => rule.validator), `${platform} ${field} missing format rule`)
        assert.equal(rules.some(rule => rule.required), platform === 'windows')
        rules.find(rule => rule.validator).validator({}, value, error => assert.ok(error))
        assert.equal(ctx.api.isFieldFormatInvalid(field, value), true)
        assert.ok(ctx.presetErrors()[field], `${platform} ${field} missing preset warning`)
      }
    }
  }
})

test('empty optional values and normalized keys remain supported without required rules', () => {
  for (const platform of ['linux', 'android']) {
    const ctx = fixture(platform)
    for (const field of ['server_ip', 'relay_server', 'api_server', 'key']) {
      ctx.form[field] = ''
      assert.equal(ctx.api.rules.value[field], undefined)
      assert.equal(ctx.api.isFieldFormatInvalid(field, ''), false)
    }
    ctx.form.key = '\r\n'
    assert.equal(ctx.api.rules.value.key, undefined)
    ctx.form.key = key + '\r\n'
    const rules = ctx.api.rules.value.key
    assert.equal(rules.some(rule => rule.required), false)
    rules[0].validator({}, ctx.form.key, error => assert.equal(error, undefined))
    assert.deepEqual(Object.keys(ctx.presetErrors()), [])
  }
  const windows = fixture('windows')
  for (const field of ['server_ip', 'relay_server', 'api_server', 'key']) {
    const rules = windows.api.rules.value[field]
    assert.ok(rules.some(rule => rule.required))
    rules.find(rule => rule.validator).validator({}, '', error => assert.equal(error, undefined))
  }
})

test('server-key action still uses exposed defaults and validates only key', async () => {
  const ctx = fixture()
  const calls = []
  ctx.serverConfigDefaults.key = key
  ctx.clearFieldError = field => calls.push(['clear', field])
  ctx.formRef.value = { validateField: async field => calls.push(['validate', field]) }
  await ctx.api.useServerKey()
  assert.equal(ctx.form.key, key)
  assert.deepEqual(calls, [['clear', 'key'], ['validate', 'key']])
  assert.match(sfc, /v-if="serverConfigDefaults.key"/)
  assert.match(sfc, /@click="useServerKey"/)
  const { descriptor } = parse(sfc)
  const script = compileScript(descriptor, { id: 'custom-client-regression' })
  const template = compileTemplate({ source: descriptor.template.content, filename: 'index.vue', id: 'custom-client-regression', compilerOptions: { bindingMetadata: script.bindings } })
  assert.deepEqual(template.errors, [])
  assert.match(between('    return {\n      form, formRef', '\n  },\n})'), /\bserverConfigDefaults,/) // setup return, not merely declaration
})

test('ARIA reflects optional field errors without changing required policy', async () => {
  const ctx = fixture()
  const attributes = {}
  ctx.document = { getElementById: id => id === 'custom-client-server_ip-input' ? {
    setAttribute: (name, value) => { attributes[name] = value },
    removeAttribute: name => { delete attributes[name] },
  } : null }
  ctx.form.server_ip = ' '
  ctx.api.invalidFields.value = ctx.presetErrors()
  await ctx.api.syncFieldAria()
  assert.equal(attributes['aria-required'], 'false')
  assert.equal(attributes['aria-invalid'], 'true')
  assert.equal(attributes['aria-describedby'], 'custom-client-server_ip-error')
  ctx.api.invalidFields.value = {}
  await ctx.api.syncFieldAria()
  assert.equal(attributes['aria-invalid'], 'false')
  assert.equal(attributes['aria-describedby'], undefined)
})
