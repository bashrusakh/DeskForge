<template>
  <div class="server-control-page">
    <page-header
        :title="T('ServerCommands')"
        subtitle="Operate ID and relay server controls. Advanced commands can affect connected clients and should be used deliberately."
        eyebrow="Server"
        pulse="warning"
    />
    <page-section title="Server command availability" subtitle="Checks whether the admin API can send commands to each RustDesk server process.">
      <div class="server-status-grid">
        <div class="server-status-card" :class="{ 'is-available': canSendIdServerCmd }">
          <span>ID {{ T('Status') }}</span>
          <strong>{{ canSendIdServerCmd ? T('Available') : T('NotAvailable') }}</strong>
          <el-button size="small" text @click="refreshCanSendIdServerCmd">{{ T('Refresh') }}</el-button>
        </div>
        <div class="server-status-card" :class="{ 'is-available': canSendRelayServerCmd }">
          <span>RELAY {{ T('Status') }}</span>
          <strong>{{ canSendRelayServerCmd ? T('Available') : T('NotAvailable') }}</strong>
          <el-button size="small" text @click="refreshCanSendRelayServerCmd">{{ T('Refresh') }}</el-button>
        </div>
      </div>
      <p class="server-command-tip" v-html="T('ServerCmdTips', {wiki: '<a target=\'_blank\' href=\'https://github.com/rustdesk/rustdesk-api/wiki/Rustdesk-Command\'>WIKI</a>'})"></p>
    </page-section>
    <el-tabs
        v-model="activeName"
        type="card"
        class="server-command-tabs"
    >
      <el-tab-pane :label="T('Simple')" name="Simple">
        <page-section title="Simple controls" subtitle="Common server controls are grouped here and remain guarded by per-card availability states.">
          <el-space wrap>
            <RelayServers ref="rs" :can-send="canSendIdServerCmd"/>
            <alwaysUseRelay :can-send="canSendIdServerCmd" @success="handleAlwaysUseRelaySuccess"/>
            <mustLogin :can-send="canControlMustLogin&&canSendIdServerCmd"/>
            <usage :can-send="canSendRelayServerCmd"/>
            <blocklist :can-send="canSendRelayServerCmd"/>
            <blacklist :can-send="canSendRelayServerCmd"/>
          </el-space>
        </page-section>


      </el-tab-pane>
      <el-tab-pane :label="T('Advanced')" name="Advanced">
        <danger-zone title="Advanced server commands" subtitle="Custom commands are sent directly to hbbs/hbbr. Confirm each send and keep destructive operations separate from normal controls.">
        <page-section class="list-query" title="Command toolbar">
          <el-form inline label-width="80px">
            <el-form-item>
              <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
              <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
              <el-button type="success" :disabled="!canSendIdServerCmd" @click="showCmd({cmd:'',option:'',target:ID_TARGET})">{{ T('Send') }} To Id</el-button>
              <el-button type="success" :disabled="!canSendRelayServerCmd" @click="showCmd({cmd:'',option:'',target:RELAY_TARGET})">{{ T('Send') }} To Relay</el-button>
            </el-form-item>
          </el-form>
        </page-section>
        <page-section class="list-body" title="Custom command list" :subtitle="`${listRes.total} commands`">
          <data-table
              :data="listRes.list"
              :loading="listRes.loading"
              row-key="id"
              :columns="[
                { prop: 'cmd', label: 'cmd', align: 'center' },
                { prop: 'alias', label: 'alias', align: 'center' },
                { prop: 'option', label: 'option', align: 'center' },
                { prop: 'explain', label: 'explain', align: 'center' },
                { label: 'actions', align: 'center', slot: 'actions' }
              ]"
          >
            <template #actions="{ row }">
              <el-button type="success" :disabled="!canSendCmd(row.target)" @click="showCmd(row)">{{ T('Send') }}</el-button>
              <el-button v-if="row.id" type="primary" @click="toUpdate(row)">{{ T('Edit') }}</el-button>
              <el-button v-if="row.id" type="danger" @click="del(row)">{{ T('Delete') }}</el-button>
            </template>
          </data-table>

          <app-dialog
              v-model="formVisible"
              @confirm="submit"
          >
            <el-form label-width="150">
              <el-form-item label="cmd">
                <el-input v-model="formData.cmd"></el-input>
              </el-form-item>
              <el-form-item label="alias">
                <el-input v-model="formData.alias"></el-input>
              </el-form-item>
              <el-form-item label="option">
                <el-input v-model="formData.option"></el-input>
              </el-form-item>
              <el-form-item label="target">
                <el-radio-group v-model="formData.target">
                  <el-radio label="id_server" :value="ID_TARGET"></el-radio>
                  <el-radio label="relay_server" :value="RELAY_TARGET"></el-radio>
                </el-radio-group>
              </el-form-item>
              <el-form-item label="explain">
                <el-input v-model="formData.explain"></el-input>
              </el-form-item>
            </el-form>
          </app-dialog>

          <app-dialog
              v-model="showCmdForm"
              :title="T('SendCmd')"
              :show-confirm="false"
              :hide-footer="true"
          >
            <el-form label-width="150" :disabled="!canSendCmd(customCmd.target)">
              <el-alert
                  class="command-target-alert"
                  :closable="false"
                  type="warning"
                  show-icon
              >
                <template #title>
                  {{ customCmd.target === ID_TARGET ? 'ID server' : 'Relay server' }} command target
                </template>
              </el-alert>
              <el-form-item label="cmd">
                <el-input v-model="customCmd.cmd"></el-input>
              </el-form-item>
              <el-form-item label="option">
                <el-input v-model="customCmd.option"></el-input>
                <el-text v-if="customCmd.example.trim()" style="margin-top: 5px">Example:
                  <el-text type="primary">{{ customCmd.example }}</el-text>
                </el-text>
              </el-form-item>
              <el-form-item>
                <el-button type="primary" @click="submitCmd">{{ T('Send') }}</el-button>
              </el-form-item>
              <el-form-item :label="T('Result')">
                <div class="command-result-wrap">
                  <div class="command-result-toolbar">
                    <span>{{ customCmd.res ? `${customCmd.res.length} chars` : 'Waiting for output' }}</span>
                    <div>
                      <el-button size="small" :disabled="!customCmd.res" @click="copyCmdResult">Copy</el-button>
                      <el-button size="small" :disabled="!customCmd.res" @click="clearCmdResult">Clear</el-button>
                    </div>
                  </div>
                  <el-input class="command-result" type="textarea" readonly v-model="customCmd.res" rows="15" placeholder="Command output will appear here."></el-input>
                </div>
              </el-form-item>
            </el-form>
          </app-dialog>
        </page-section>
        </danger-zone>
      </el-tab-pane>
    </el-tabs>

  </div>
</template>


<script setup>
  import { create, list, remove, sendCmd, update } from '@/api/rustdesk'
  import { onMounted, reactive, ref } from 'vue'
  import { T } from '@/utils/i18n'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { ID_TARGET, RELAY_TARGET } from '@/views/rustdesk/options'
  import blocklist from '@/views/rustdesk/blocklist.vue'
  import blacklist from '@/views/rustdesk/blacklist.vue'
  import alwaysUseRelay from '@/views/rustdesk/always_use_relay.vue'
  import RelayServers from '@/views/rustdesk/relay_servers.vue'
  import mustLogin from '@/views/rustdesk/must_login.vue'
  import usage from '@/views/rustdesk/usage.vue'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import DangerZone from '@/components/ui/DangerZone.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const activeName = ref('Simple')

  const canSendIdServerCmd = ref(false)
  const checkCanSendIdServerCmd = async () => {
    const res = await sendCmd({ cmd: 'h', target: ID_TARGET }).catch(_ => false)
    canSendIdServerCmd.value = !!res.data
    if (canSendIdServerCmd.value) {
      const commands = res.data.split('\n').filter(i => i)
      console.log(commands)
      canControlMustLogin.value = commands.some(i => i.includes('must-login'))
    }
  }

  const canControlMustLogin = ref(false)
  const refreshCanSendIdServerCmd = () => {
    checkCanSendIdServerCmd()
  }
  onMounted(refreshCanSendIdServerCmd)

  const canSendRelayServerCmd = ref(false)

  const checkCanSendRelayServerCmd = async () => {
    const res = await sendCmd({ cmd: 'h', target: RELAY_TARGET }).catch(_ => false)
    canSendRelayServerCmd.value = !!res.data
  }
  const refreshCanSendRelayServerCmd = () => {
    checkCanSendRelayServerCmd()
  }
  onMounted(refreshCanSendRelayServerCmd)

  const rs = ref(null)
  const handleAlwaysUseRelaySuccess = () => {
    rs.value.save()
  }

  const canSendCmd = (target) => {
    if (target === ID_TARGET) {
      return canSendIdServerCmd.value
    }
    if (target === RELAY_TARGET) {
      return canSendRelayServerCmd.value
    }
    return false
  }

  const listRes = reactive({
    list: [], total: 0, loading: false,
  })
  const listQuery = reactive({
    page: 1,
    page_size: 10,
  })
  const getList = async () => {
    listRes.loading = true
    const res = await list(listQuery).catch(_ => false)
    listRes.loading = false
    if (res) {
      listRes.list = res.data.list
      listRes.total = res.data.total
    }
  }
  const handlerQuery = () => {
    if (listQuery.page === 1) {
      getList()
    } else {
      listQuery.page = 1
    }
  }
  onMounted(getList)
  const del = async (row) => {
    const cf = await ElMessageBox.confirm(T('Confirm?', { param: T('Delete') }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
      type: 'warning',
    }).catch(_ => false)
    if (!cf) {
      return false
    }

    const res = await remove({ id: row.id }).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      getList()
    }
  }
  const formData = reactive({
    cmd: '',
    alias: '',
    option: '',
    target: '',
    explain: '',
  })
  const formVisible = ref(false)
  const toAdd = () => {
    formVisible.value = true
    formData.cmd = ''
    formData.alias = ''
    formData.option = ''
    formData.explain = ''
  }
  const toUpdate = (row) => {
    formVisible.value = true
    formData.id = row.id
    formData.cmd = row.cmd
    formData.alias = row.alias
    formData.option = row.option
    formData.target = row.target
    formData.explain = row.explain
  }
  const submit = async () => {
    if (!formData.cmd) {
      ElMessage.error(T('ParamRequired', { param: 'cmd' }))
      return
    }
    const api = formData.id ? update : create
    const res = await api(formData).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      formVisible.value = false
      getList()
    }
  }
  const cancel = () => {
    formVisible.value = false
  }

  const showCmdForm = ref(false)
  const customCmd = reactive({
    cmd: '',
    option: '',
    target: '',
    res: '',
    example: '',
  })
  const showCmd = (row) => {
    showCmdForm.value = true
    customCmd.cmd = row.cmd
    customCmd.option = ''
    customCmd.res = ''
    customCmd.target = row.target
    customCmd.example = `${row.cmd} ${row.option}`
  }
  const submitCmd = async () => {
    const cf = await ElMessageBox.confirm(T('Confirm?', { param: T('SendCmd') }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
      type: 'warning',
    }).catch(_ => false)
    if (!cf) {
      return false
    }
    sendCmd(customCmd).then(res => {
      console.log(res)
      customCmd.res = res.data
      ElMessage.success(T('OperationSuccess'))
    })
  }
  const clearCmdResult = () => {
    customCmd.res = ''
  }
  const copyCmdResult = async () => {
    if (!customCmd.res) return
    try {
      await navigator.clipboard.writeText(customCmd.res)
      ElMessage.success(T('CopySuccess'))
    } catch (_) {
      ElMessage.error(T('CopyFailed'))
    }
  }

</script>

<style scoped lang="scss">
.simple-card {
  min-width: 300px;
  margin: 10px;
  min-height: 300px;
}

.server-status-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.server-status-card {
  padding: 16px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-surface-2);

  span,
  strong {
    display: block;
  }

  span {
    color: var(--color-muted);
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  strong {
    margin: 8px 0 10px;
    color: var(--color-danger);
    font-size: 20px;
  }

  &.is-available strong {
    color: var(--color-success);
  }
}

.server-command-tip {
  margin: 14px 0 0;
  color: var(--color-muted);
  line-height: 1.6;
}

.server-command-tabs {
  margin-top: 18px;
}

:deep(.command-result textarea) {
  min-height: 260px !important;
  border-color: transparent;
  background: #0b1020;
  color: #d1e7ff;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
}

.command-target-alert {
  margin-bottom: 16px;
}

.command-result-wrap {
  width: 100%;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 14px;
  background: var(--bg-tertiary);
}

.command-result-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border-secondary);
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 12px;
}

@media (max-width: 720px) {
  .server-status-grid {
    grid-template-columns: 1fr;
  }
}
</style>
