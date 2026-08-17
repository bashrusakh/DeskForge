<template>
  <div class="app-container">
    <page-header
        title="GitHub Build Integration"
        subtitle="Settings for building Windows clients via GitHub Actions and receiving rustqs artifacts back on this server."
        eyebrow="Server"
        pulse="warning"
    />

    <page-section title="GitHub provider settings" subtitle="Configured repository, PAT, and encrypted payload key.">

      <section
        class="workflow-approval"
        :aria-busy="loading || workflowTagsState === 'loading' || approving"
        aria-labelledby="workflow-approval-title"
      >
        <div class="workflow-approval__header">
          <div>
            <h2 id="workflow-approval-title" class="workflow-approval__title">{{ T('WorkflowApprovalTitle') }}</h2>
            <p class="workflow-approval__description">{{ T('WorkflowApprovalDescription') }}</p>
          </div>
          <div class="workflow-approval__status" aria-live="polite">
            <span class="workflow-approval__status-label">{{ T('WorkflowApprovalState') }}</span>
            <el-tag :type="workflowRefStatusType" role="status">{{ workflowRefStatusLabel }}</el-tag>
          </div>
        </div>

        <el-alert
          id="workflow-approval-protection"
          type="info"
          :closable="false"
          show-icon
        >
          {{ T('WorkflowApprovalProtectedExplanation') }}
        </el-alert>

        <el-alert v-if="configError" type="error" :closable="false" show-icon>
          <span>{{ T('WorkflowApprovalConfigError') }}</span>
          <el-button class="workflow-approval__retry" size="small" @click="load">
            {{ T('WorkflowApprovalRetry') }}
          </el-button>
        </el-alert>

        <loading-state
          v-else-if="workflowTagsState === 'loading'"
          :title="T('WorkflowApprovalLoading')"
        />

        <el-alert v-else-if="workflowTagsState === 'error'" type="error" :closable="false" show-icon>
          <span>{{ T('WorkflowApprovalLoadError') }}</span>
          <el-button class="workflow-approval__retry" size="small" @click="load">
            {{ T('WorkflowApprovalRetry') }}
          </el-button>
        </el-alert>

        <empty-state
          v-else-if="workflowTagsState === 'empty'"
          :title="T('WorkflowApprovalEmpty')"
        >
          <template #actions>
            <el-button size="small" @click="load">
              {{ T('WorkflowApprovalRetry') }}
            </el-button>
          </template>
        </empty-state>

        <div v-else class="workflow-approval__controls">
          <label class="workflow-approval__label" for="workflow-tag-select">
            {{ T('WorkflowApprovalTagLabel') }}
          </label>
          <el-select
            id="workflow-tag-select"
            v-model="selectedWorkflowTag"
            class="workflow-approval__select"
            :aria-describedby="'workflow-approval-protection'"
            :aria-label="T('WorkflowApprovalTagLabel')"
            :placeholder="T('WorkflowApprovalTagPlaceholder')"
            :disabled="approving"
          >
            <el-option
              v-for="option in workflowTags"
              :key="option.tag"
              :label="option.label"
              :value="option.tag"
            />
          </el-select>
          <p v-if="currentWorkflowTag" class="workflow-approval__current">
            {{ T('WorkflowApprovalCurrentTag', { param: currentWorkflowTag }) }}
          </p>
          <el-alert v-if="approvalError" class="workflow-approval__feedback" type="error" :closable="false" show-icon>
            {{ T('WorkflowApprovalRequestFailed') }}
          </el-alert>
          <el-button
            class="workflow-approval__action"
            type="primary"
            :loading="approving"
            :disabled="!selectedWorkflowTag"
            @click="onApproveWorkflowRef"
          >
            {{ approving ? T('WorkflowApprovalApproving') : T('WorkflowApprovalApprove') }}
          </el-button>
        </div>
      </section>

      <el-form ref="formRef" :model="form" label-position="top" v-loading="loading">
        <el-form-item label="Repository (owner/name)">
          <el-input v-model="form.repo" placeholder="owner/rustdesk-fork" />
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

        <el-alert v-if="saveError" type="error" :closable="false" show-icon role="alert" aria-live="assertive">
          {{ saveError }}
        </el-alert>
        <el-alert v-if="testResult" :type="testResult.ok ? 'success' : 'error'" :closable="false">
          {{ testResult.message }}
        </el-alert>
        <el-alert
          v-if="dispatchResult"
          :type="dispatchResult.run_id ? 'success' : 'error'"
          :closable="false"
        >
          <div v-if="dispatchResult.message">{{ dispatchResult.message }}</div>
          <div v-if="dispatchResult.run_id">
            Run id={{ dispatchResult.run_id }}
            <template v-if="dispatchResult.html_url">
              · <a :href="dispatchResult.html_url" target="_blank">Open in GitHub</a>
            </template>
          </div>
        </el-alert>
      </el-form>
    </page-section>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive, computed } from 'vue'
import * as api from '@/api/github_build_config'
import { T } from '@/utils/i18n'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageSection from '@/components/ui/PageSection.vue'
import LoadingState from '@/components/ui/LoadingState.vue'
import EmptyState from '@/components/ui/EmptyState.vue'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const dispatching = ref(false)
const generating = ref(false)
const syncing = ref(false)
const approving = ref(false)
const syncResult = ref(null)
const workflowTags = ref([])
const workflowTagsState = ref('loading')
const configError = ref(false)
const approvalError = ref(false)
const selectedWorkflowTag = ref('')
const currentWorkflowTag = ref('')

const info = reactive({ has_token: false, has_payload_key: false, workflow_ref: '', workflow_ref_approved: false, workflow_ref_status: 'approval-required' })
const form = reactive({
  repo: '',
  token: '',
  payload_key: '',
})
const generatedKey = ref('')
const testResult = ref(null)
const dispatchResult = ref(null)
const saveError = ref('')

async function load () {
  loading.value = true
  configError.value = false
  approvalError.value = false
  workflowTagsState.value = 'loading'
  workflowTags.value = []
  selectedWorkflowTag.value = ''
  currentWorkflowTag.value = ''
  info.workflow_ref = ''
  info.workflow_ref_approved = false
  info.workflow_ref_status = 'approval-required'
  let configLoaded = false
  try {
    const res = await api.get()
    const d = res.data || res
    form.repo = d.repo || ''
    info.has_token = !!d.has_token
    info.has_payload_key = !!d.has_payload_key
    applyApprovalState(d)
    configLoaded = true

    const tagsRes = await api.getWorkflowTags()
    const tagsData = tagsRes.data || tagsRes
    workflowTags.value = Array.isArray(tagsData.tags)
      ? tagsData.tags.filter(option => option && typeof option.tag === 'string' && typeof option.label === 'string')
      : []
    workflowTagsState.value = workflowTags.value.length > 0 ? 'ready' : 'empty'
    selectCurrentWorkflowTag()
  } catch (e) {
    workflowTagsState.value = 'error'
    configError.value = !configLoaded
  } finally {
    loading.value = false
  }
}

function applyApprovalState (data) {
  info.workflow_ref = data.workflow_ref || ''
  info.workflow_ref_approved = !!data.workflow_ref_approved
  info.workflow_ref_status = ['approval-required', 'provider-policy-unverified', 'approved'].includes(data.workflow_ref_status)
    ? data.workflow_ref_status
    : (info.workflow_ref_approved ? 'provider-policy-unverified' : 'approval-required')
  currentWorkflowTag.value = info.workflow_ref
}

function selectCurrentWorkflowTag () {
  selectedWorkflowTag.value = workflowTags.value.some(option => option.tag === currentWorkflowTag.value)
    ? currentWorkflowTag.value
    : ''
}

async function onApproveWorkflowRef () {
  if (!selectedWorkflowTag.value) return
  approving.value = true
  approvalError.value = false
  try {
    const res = await api.approveWorkflowRef(selectedWorkflowTag.value)
    const d = res.data || res
    applyApprovalState(d)
    selectCurrentWorkflowTag()
  } catch (e) {
    approvalError.value = true
  } finally {
    approving.value = false
  }
}

const workflowRefStatusLabel = computed(() => {
  switch (info.workflow_ref_status) {
    case 'approved':
      return T('WorkflowApprovalStatusApproved')
    case 'provider-policy-unverified':
      return T('WorkflowApprovalStatusProviderPolicyUnverified')
    default:
      return T('WorkflowApprovalStatusApprovalRequired')
  }
})

const workflowRefStatusType = computed(() => {
  switch (info.workflow_ref_status) {
    case 'approved':
      return 'success'
    case 'provider-policy-unverified':
      return 'warning'
    default:
      return 'info'
  }
})

async function onSave () {
  saving.value = true
  saveError.value = ''
  try {
    await api.save({
      repo: form.repo,
      token: form.token,
      payload_key: form.payload_key,
    })
  } catch (e) {
    saveError.value = extractSaveError(e)
    return
  } finally {
    saving.value = false
  }
  form.token = ''
  form.payload_key = ''
  await load()
}

function extractSaveError (error) {
  const envelopes = [error, error?.response?.data]
  for (const envelope of envelopes) {
    if (Number.isInteger(envelope?.code) && envelope.code !== 0
      && typeof envelope.message === 'string' && envelope.message.trim()) {
      return envelope.message.trim()
    }
  }
  return T('GithubBuildSaveError')
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
  // B-009: это реальная сборка на GitHub Actions (тратит минуты) — подтверждаем.
  if (!window.confirm('This triggers a REAL build on GitHub Actions and consumes Actions minutes. Continue?')) return
  dispatching.value = true
  dispatchResult.value = null
  try {
    const res = await api.dispatchTest()
    dispatchResult.value = res.data || res
  } catch (e) {
    dispatchResult.value = { message: e.message || String(e) }
  } finally {
    dispatching.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.hint { color: var(--color-muted); font-size: 0.9em; }
.workflow-approval { margin-bottom: 24px; }
.workflow-approval__header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; margin-bottom: 16px; }
.workflow-approval__title { margin: 0; color: var(--color-text); font-size: 18px; }
.workflow-approval__description, .workflow-approval__current { color: var(--color-muted); font-size: 0.9em; line-height: 1.5; }
.workflow-approval__description { margin: 4px 0 0; }
.workflow-approval__status { display: flex; flex-direction: column; align-items: flex-end; gap: 6px; flex-shrink: 0; }
.workflow-approval__status-label, .workflow-approval__label { color: var(--color-muted); font-size: 0.85em; font-weight: 600; }
.workflow-approval__controls { margin-top: 18px; }
.workflow-approval__label { display: block; margin-bottom: 6px; }
.workflow-approval__select { width: min(100%, 420px); }
.workflow-approval__current { margin: 8px 0 0; }
.workflow-approval__feedback { margin-top: 16px; }
.workflow-approval__retry { margin-left: 12px; }
.workflow-approval__action { display: block; margin-top: 16px; }
.hint-text { color: var(--color-muted); font-size: 0.85em; margin-top: 4px; }
.generated-key { margin-top: 12px; padding: 12px; background: var(--color-code-bg); border-radius: 12px; }
.warn { color: var(--color-danger); margin-top: 4px; font-size: 0.85em; }
code { background: var(--color-code-bg); padding: 1px 4px; border-radius: 6px; }

@media (max-width: 640px) {
  .workflow-approval__header { flex-direction: column; gap: 12px; }
  .workflow-approval__status { align-items: flex-start; }
  .workflow-approval__select { width: 100%; }
  .workflow-approval__retry { display: block; margin: 10px 0 0; }
}
</style>
