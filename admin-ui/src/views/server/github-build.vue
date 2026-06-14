<template>
  <div class="app-container">
    <page-header
        title="GitHub Build Integration"
        subtitle="Settings for building Windows clients via GitHub Actions and receiving rustqs artifacts back on this server."
        eyebrow="Server"
        pulse="warning"
    />

    <page-section title="Workflow settings" subtitle="Repository, workflow, branch, PAT, and encrypted payload key.">

      <el-form ref="formRef" :model="form" label-position="top" v-loading="loading">
        <el-form-item label="Repository (owner/name)">
          <el-input v-model="form.repo" placeholder="bashrusakh/rustdesk" />
        </el-form-item>

        <el-form-item label="Workflow filename">
          <el-input v-model="form.workflow_filename" placeholder="rustqs-windows-min-test.yml" />
        </el-form-item>

        <el-form-item label="Branch">
          <el-input v-model="form.branch" placeholder="master or rustqs/min-test" />
        </el-form-item>

        <el-form-item label="GitHub Token (PAT)">
          <el-input
            v-model="form.token"
            type="password"
            show-password
            :placeholder="info.has_token ? '(already saved — empty = keep current)' : 'github_pat_...'"
          />
          <div class="hint-text">
            Fine-grained PAT, scope: <code>Actions: Read &amp; Write</code> on the repo above.
            Empty value keeps the existing token.
          </div>
        </el-form-item>

        <el-form-item label="Encryption key (WORKFLOW_PAYLOAD_KEY)">
          <el-input
            v-model="form.payload_key"
            type="password"
            show-password
            :placeholder="info.has_payload_key ? '(already saved — empty = keep current)' : 'paste or click Generate'"
          />
          <div class="hint-text">
            Must match the GitHub Secret <code>WORKFLOW_PAYLOAD_KEY</code> in the fork.
            Click Generate to create a fresh key — you'll need to copy it to
            github.com/&lt;repo&gt;/settings/secrets/actions.
          </div>
          <el-button size="small" @click="onGenerate" :loading="generating">Generate new key</el-button>
          <el-button size="small" @click="onSyncSecret" :loading="syncing">Push to GitHub Secrets</el-button>
          <div v-if="generatedKey" class="generated-key">
            <strong>New key (will be auto-pushed to GitHub Secrets if you click "Push" above, or copy manually):</strong>
            <el-input v-model="generatedKey" readonly>
              <template #append>
                <el-button @click="copyKey">Copy</el-button>
              </template>
            </el-input>
            <p class="warn">This is the only time the key is shown. Save it now.</p>
          </div>
          <el-alert v-if="syncResult" :type="syncResult.ok ? 'success' : 'error'" :closable="true">
            {{ syncResult.message }}
          </el-alert>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" @click="onSave" :loading="saving">Save</el-button>
          <el-button @click="onTest" :loading="testing">Test connection</el-button>
          <el-button @click="onDispatchTest" :loading="dispatching">Trigger test build</el-button>
        </el-form-item>

        <el-alert v-if="testResult" :type="testResult.ok ? 'success' : 'error'" :closable="false">
          {{ testResult.message }}
        </el-alert>
        <el-alert
          v-if="dispatchResult"
          :type="dispatchResult.pending ? 'info' : (dispatchResult.ok === true ? 'success' : (dispatchResult.ok === false ? 'error' : (dispatchResult.run_id ? 'info' : 'error')))"
          :closable="false"
        >
          <div v-if="dispatchResult.pending">
            <el-icon class="is-loading"><Loading /></el-icon>
            {{ dispatchResult.message }}
          </div>
          <div v-else-if="dispatchResult.run_id">
            <div v-if="dispatchResult.message">{{ dispatchResult.message }}</div>
            <div>
              Run id={{ dispatchResult.run_id }}
              <span v-if="dispatchResult.conclusion"> · conclusion=<strong>{{ dispatchResult.conclusion }}</strong></span>
              · <a :href="runUrl(dispatchResult.run_id)" target="_blank">Open in GitHub</a>
            </div>
          </div>
          <div v-else>{{ dispatchResult.message || dispatchResult.error || 'Unknown error' }}</div>
        </el-alert>
      </el-form>
    </page-section>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import axios from 'axios'
import { Loading } from '@element-plus/icons-vue'
import * as api from '@/api/github_build_config'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageSection from '@/components/ui/PageSection.vue'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const dispatching = ref(false)
const generating = ref(false)
const syncing = ref(false)
const syncResult = ref(null)

const info = reactive({ has_token: false, has_payload_key: false })
const form = reactive({
  repo: '',
  workflow_filename: '',
  branch: '',
  token: '',
  payload_key: '',
})
const generatedKey = ref('')
const testResult = ref(null)
const dispatchResult = ref(null)

function runUrl (runId) {
  return form.repo ? `https://github.com/${form.repo}/actions/runs/${runId}` : '#'
}

async function load () {
  loading.value = true
  try {
    const res = await api.get()
    const d = res.data || res
    form.repo = d.repo || 'bashrusakh/rustdesk'
    form.workflow_filename = d.workflow_filename || 'rustqs-windows-min-test.yml'
    form.branch = d.branch || 'rustqs/min-test'
    info.has_token = !!d.has_token
    info.has_payload_key = !!d.has_payload_key
  } finally {
    loading.value = false
  }
}

async function onSave () {
  saving.value = true
  try {
    await api.save({
      repo: form.repo,
      workflow_filename: form.workflow_filename,
      branch: form.branch,
      token: form.token,
      payload_key: form.payload_key,
    })
    form.token = ''
    form.payload_key = ''
    await load()
  } finally {
    saving.value = false
  }
}

async function onGenerate () {
  generating.value = true
  try {
    const res = await api.generateKey()
    const d = res.data || res
    generatedKey.value = d.payload_key
    info.has_payload_key = true
  } finally {
    generating.value = false
  }
}

function copyKey () {
  if (generatedKey.value) navigator.clipboard.writeText(generatedKey.value)
}

async function onSyncSecret () {
  syncing.value = true
  syncResult.value = null
  try {
    const res = await api.syncSecret()
    syncResult.value = res.data || res
  } catch (e) {
    syncResult.value = { ok: false, message: e.message || String(e) }
  } finally {
    syncing.value = false
  }
}

async function onTest () {
  testing.value = true
  testResult.value = null
  try {
    const res = await api.test()
    testResult.value = res.data || res
  } finally {
    testing.value = false
  }
}

async function onDispatchTest () {
  dispatching.value = true
  dispatchResult.value = { pending: true, message: '⏳ Build running, polling for completion (up to 90 min)...' }
  try {
    // Сервер поллит GitHub внутри запроса до 90 мин → увеличиваем таймаут axios.
    // Берём базовый URL из env и path из api.dispatchTest, чтобы выставить кастомный таймаут.
    const base = import.meta.env.VITE_SERVER_API
    const path = '/admin/github_build_config/dispatch_test'
    const token = (await import('@/utils/auth')).getToken()
    const userStore = (await import('@/store/user')).useUserStore((await import('@/store')).pinia)
    const tok = token || userStore.token
    const res = await axios.post(base + path, {}, {
      timeout: 95 * 60 * 1000,
      withCredentials: true,
      headers: { 'api-token': tok },
    })
    const body = res.data
    dispatchResult.value = (body && body.data) || body
  } catch (e) {
    dispatchResult.value = { ok: false, message: e.message || String(e) }
  } finally {
    dispatching.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.hint { color: var(--color-muted); font-size: 0.9em; }
.hint-text { color: var(--color-muted); font-size: 0.85em; margin-top: 4px; }
.generated-key { margin-top: 12px; padding: 12px; background: var(--color-code-bg); border-radius: 12px; }
.warn { color: var(--color-danger); margin-top: 4px; font-size: 0.85em; }
code { background: var(--color-code-bg); padding: 1px 4px; border-radius: 6px; }
</style>
