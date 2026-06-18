<template>
  <div class="custom-client">
    <page-header
        title="Custom Client Builder"
        subtitle="Create branded, RustDesk-compatible client builds with pinned server, security, permissions, and branding settings."
        eyebrow="Client Builder"
        pulse="warning"
    />
    <page-section class="mb-20" :title="T('NewBuild')" subtitle="Configure the build payload and optionally save or load reusable presets.">
      <el-form :model="form" label-width="180px" v-loading="submitting">
        <el-row :gutter="20" class="mb-10">
          <el-col :span="12">
            <el-form-item :label="T('LoadPreset') || 'Load preset'" label-width="120px">
              <el-select v-model="selectedPresetId" placeholder="Select preset" clearable style="width:calc(100% - 180px)" @change="onPresetSelect">
                <el-option v-for="p in presets" :key="p.id" :label="p.name + ' (' + p.platform + ')'" :value="p.id">
                  <span style="float:left">{{ p.name }} <el-tag size="small" effect="plain" style="margin-left:6px">{{ p.platform }}</el-tag></span>
                  <el-button type="danger" link size="small" style="float:right" @click.stop="deletePreset(p)">{{ T('Delete') }}</el-button>
                </el-option>
              </el-select>
              <el-button type="primary" plain size="small" style="margin-left:8px" @click="saveCurrentAsPreset">{{ T('SaveAsPreset') || 'Save as preset' }}</el-button>
            </el-form-item>
          </el-col>
        </el-row>
        <el-divider content-position="left">{{ T('Platform') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('Platform')">
              <el-select v-model="form.platform" style="width:100%">
                <el-option label="Windows 64Bit" value="windows" />
                <el-option label="Windows 32Bit" value="windows-x86" />
                <el-option label="Linux" value="linux" />
                <el-option label="Android" value="android" />
                <el-option label="macOS" value="macos" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('Version')">
              <el-select v-model="form.version" style="width:100%">
                <el-option :label="T('Nightly')" value="master" />
                <el-option v-for="v in versions" :key="v" :label="v" :value="v" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('AppName')">
              <el-input v-model="form.app_name" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">{{ T('CustomServer') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('Host')">
              <el-input v-model="form.server_ip" placeholder="e.g. your-server.com" @blur="stripServerPort">
                <template #append>
                  <el-tooltip :content="T('HostnameOnlyHint')" placement="top">
                    <el-icon><InfoFilled /></el-icon>
                  </el-tooltip>
                </template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('Key')">
              <el-input v-model="form.key" placeholder="encryption key" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('ApiServer')">
              <el-input v-model="form.api_server" placeholder="https://your-server:21114" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('RelayServer')">
              <el-input v-model="form.relay_server" placeholder="your-server:21117" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('CompanyName')">
              <el-input v-model="form.company_name" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('CustomUrl')">
              <el-input v-model="form.download_url" placeholder="download URL" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">{{ T('Security') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('PasswordApproveMode')">
              <el-select v-model="form.pass_approve_mode" style="width:100%">
                <el-option :label="T('Password')" value="password" />
                <el-option :label="T('Click')" value="click" />
                <el-option :label="T('PasswordAndClick')" value="password-click" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('PermanentPassword')">
              <el-input v-model="form.permanent_password" type="password" show-password />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('Direction')">
              <el-select v-model="form.direction" style="width:100%">
                <el-option :label="T('IncomingOnly')" value="incoming" />
                <el-option :label="T('OutgoingOnly')" value="outgoing" />
                <el-option :label="T('Bidirectional')" value="both" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20">
          <el-col :span="6">
            <el-form-item :label="T('DenyLan')">
              <el-switch :active-value="true" :inactive-value="false" v-model="form.deny_lan" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item :label="T('EnableDirectIP')">
              <el-switch :active-value="true" :inactive-value="false" v-model="form.enable_direct_ip" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item :label="T('AutoClose')">
              <el-switch :active-value="true" :inactive-value="false" v-model="form.auto_close" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item>
              <template #label>
                <el-tooltip content="Remove the connection management UI from the client tray/menu" placement="top">
                  <span>{{ T('HideConnectionManagement') }}</span>
                </el-tooltip>
              </template>
              <el-switch :active-value="true" :inactive-value="false" v-model="form.hide_cm" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">{{ T('Theme') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('Theme')">
              <el-select v-model="form.theme" style="width:100%">
                <el-option :label="T('Light')" value="light" />
                <el-option :label="T('Dark')" value="dark" />
                <el-option :label="T('FollowSystem')" value="system" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('RemoveWallpaper')">
              <el-switch :active-value="true" :inactive-value="false" v-model="form.remove_wallpaper" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">{{ T('Permissions') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('PermissionType')">
              <el-select v-model="form.permissions_type" style="width:100%">
                <el-option :label="T('Custom')" value="custom" />
                <el-option :label="T('FullAccess')" value="full" />
                <el-option :label="T('ScreenShare')" value="view" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20" v-if="form.permissions_type === 'custom'">
          <el-col :span="6"><el-form-item :label="T('Keyboard')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_keyboard" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('Clipboard')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_clipboard" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('FileTransfer')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_file_transfer" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('Audio')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_audio" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('TCPTunneling')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_tcp" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('RemoteRestart')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_remote_restart" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('Recording')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_recording" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('BlockingInput')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_blocking_input" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('RemoteModification')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_remote_modi" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('Printer')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_printer" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('Camera')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_camera" /></el-form-item></el-col>
          <el-col :span="6"><el-form-item :label="T('Terminal')"><el-switch :active-value="true" :inactive-value="false" v-model="form.enable_terminal" /></el-form-item></el-col>
        </el-row>

        <el-divider content-position="left">{{ T('Branding') || 'Branding' }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('AppIcon') || 'App Icon (PNG)'">
              <el-input v-model="form.app_icon_url" placeholder="/upload/20260101/icon.png" clearable>
                <template #append>
                  <el-upload :show-file-list="false" :auto-upload="true" :http-request="(opts) => uploadImage(opts, 'app_icon_url')" accept="image/png">
                    <el-button>{{ T('Upload') || 'Upload' }}</el-button>
                  </el-upload>
                </template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('AppLogo') || 'App Logo (PNG)'">
              <el-input v-model="form.app_logo_url" placeholder="/upload/20260101/logo.png" clearable>
                <template #append>
                  <el-upload :show-file-list="false" :auto-upload="true" :http-request="(opts) => uploadImage(opts, 'app_logo_url')" accept="image/png">
                    <el-button>{{ T('Upload') || 'Upload' }}</el-button>
                  </el-upload>
                </template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('PrivacyScreen') || 'Privacy screen (PNG)'">
              <el-input v-model="form.privacy_screen_url" placeholder="/upload/20260101/privacy.png" clearable>
                <template #append>
                  <el-upload :show-file-list="false" :auto-upload="true" :http-request="(opts) => uploadImage(opts, 'privacy_screen_url')" accept="image/png">
                    <el-button>{{ T('Upload') || 'Upload' }}</el-button>
                  </el-upload>
                </template>
              </el-input>
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">{{ T('Other') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="6">
            <el-form-item>
              <template #label>
                <el-tooltip content="Press Tab to cycle through monitors during remote session" placement="top">
                  <span>{{ T('CycleMonitor') }}</span>
                </el-tooltip>
              </template>
              <el-switch :active-value="true" :inactive-value="false" v-model="form.cycle_monitor" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item>
              <template #label>
                <el-tooltip content="Enable X11 offline/headless mode for headless servers" placement="top">
                  <span>{{ T('XOffline') }}</span>
                </el-tooltip>
              </template>
              <el-switch :active-value="true" :inactive-value="false" v-model="form.x_offline" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item>
              <template #label>
                <el-tooltip content="Suppress 'new version available' prompt in the client" placement="top">
                  <span>{{ T('RemoveNewVersionNotif') }}</span>
                </el-tooltip>
              </template>
              <el-switch :active-value="true" :inactive-value="false" v-model="form.remove_new_version_notif" />
            </el-form-item>
          </el-col>
          <el-col :span="6">
            <el-form-item>
              <template #label>
                <el-tooltip content="Custom package name for Android APK (default: com.carriez.flutter_hbb)" placement="top">
                  <span>{{ T('AndroidAppId') }}</span>
                </el-tooltip>
              </template>
              <el-input v-model="form.android_app_id" placeholder="com.carriez.flutter_hbb" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item>
          <el-button type="primary" @click="submitBuild" :loading="submitting">{{ T('Create') }}</el-button>
          <el-button @click="resetForm">{{ T('Reset') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>

    <page-section class="build-history" :title="T('BuildHistory')" :subtitle="`${total} builds`">
      <data-table
          :data="builds"
          :loading="loading"
          row-key="id"
          :columns="[
            { prop: 'id', label: 'ID', width: 60, align: 'center' },
            { label: T('Platform'), prop: 'platform', width: 120, align: 'center' },
            { label: T('Version'), prop: 'version', width: 100, align: 'center' },
            { label: T('AppName'), prop: 'app_name', minWidth: 140 },
            { label: T('BuildStatus'), width: 120, align: 'center', slot: 'status' },
            { label: T('CreatedAt'), prop: 'created_at', width: 160, align: 'center' },
            { label: T('Actions'), width: 200, align: 'center', slot: 'actions' }
          ]"
      >
        <template #status="{ row }">
          <el-tag :type="statusType(row.status)" size="small">{{ T(statusLabel(row.status)) }}</el-tag>
        </template>
        <template #actions="{ row }">
          <el-button v-if="row.status === 'done'" type="success" size="small" @click="downloadBuild(row)">{{ T('Download') }}</el-button>
          <el-button type="danger" size="small" @click="deleteBuild(row)">{{ T('Delete') }}</el-button>
        </template>
      </data-table>
      <el-pagination background
                     layout="prev, pager, next, sizes, jumper"
                     :page-sizes="[10,20,50,100]"
                     v-model:page-size="pageSize"
                      v-model:current-page="page"
                      :total="total" />
    </page-section>
  </div>
</template>

<script>
import { defineComponent, ref, reactive, onMounted, watch } from 'vue'
import { list, create, remove } from '@/api/custom_client'
import { list as listPresets, create as createPreset, remove as removePreset, detail as detailPreset } from '@/api/custom_preset'
import { all as fetchConfig } from '@/api/config'
import { upload as uploadFile } from '@/api/file'
import { ElMessage, ElMessageBox } from 'element-plus'
import { T } from '@/utils/i18n'
import { InfoFilled } from '@element-plus/icons-vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageSection from '@/components/ui/PageSection.vue'
import DataTable from '@/components/ui/DataTable.vue'

const VERSIONS = ['1.4.7','1.4.6','1.4.5','1.4.4','1.4.3','1.4.2','1.4.1','1.4.0','1.3.9','1.3.8','1.3.7','1.3.6','1.3.5','1.3.4','1.3.3']

export default defineComponent({
  name: 'CustomClientBuilds',
  components: { PageHeader, PageSection, DataTable, InfoFilled },
  setup () {
    const form = reactive({
      platform: 'windows',
      version: '1.4.7',
      app_name: '',
      server_ip: '',
      key: '',
      api_server: '',
      relay_server: '',
      company_name: '',
      download_url: '',
      direction: 'both',
      pass_approve_mode: 'password-click',
      permanent_password: '',
      deny_lan: false,
      enable_direct_ip: false,
      auto_close: false,
      hide_cm: false,
      theme: 'system',
      remove_wallpaper: true,
      permissions_type: 'custom',
      enable_keyboard: true,
      enable_clipboard: true,
      enable_file_transfer: true,
      enable_audio: true,
      enable_tcp: true,
      enable_remote_restart: true,
      enable_recording: true,
      enable_blocking_input: true,
      enable_remote_modi: false,
      enable_printer: true,
      enable_camera: true,
      enable_terminal: true,
      cycle_monitor: false,
      x_offline: false,
      remove_new_version_notif: false,
      android_app_id: '',
      app_icon_url: '',
      app_logo_url: '',
      privacy_screen_url: '',
    })

    const stripPort = (host) => host ? host.replace(/:\d+$/, '').trim() : ''
    const stripServerPort = () => { form.server_ip = stripPort(form.server_ip) }

    const builds = ref([])
    const loading = ref(false)
    const submitting = ref(false)
    const page = ref(1)
    const pageSize = ref(10)
    const total = ref(0)
    const versions = VERSIONS

    const loadBuilds = async () => {
      loading.value = true
      try {
        const res = await list({ page: page.value, page_size: pageSize.value })
        builds.value = res.data.list || []
        total.value = res.data.total || 0
      } catch (e) {
        console.error(e)
      } finally {
        loading.value = false
      }
    }

    const presets = ref([])
    const selectedPresetId = ref(null)
    const loadPresets = async () => {
      try {
        const res = await listPresets({ page: 1, page_size: 100 })
        presets.value = res.data.list || []
      } catch (e) {
        console.error(e)
      }
    }

    // Single source of truth for the fields persisted inside custom_json.
    // Used by loadPresetIntoForm + saveCurrentAsPreset + submitBuild — extending one
    // without the other reintroduces the "saved but not restored" bug (audit §8.9).
    // platform/version/app_name are stored on the preset record itself, not in custom_json.
    const PRESET_FIELDS = ['server_ip','key','api_server','relay_server','company_name','download_url','direction','pass_approve_mode','permanent_password','deny_lan','enable_direct_ip','auto_close','hide_cm','theme','remove_wallpaper','remove_new_version_notif','permissions_type','enable_keyboard','enable_clipboard','enable_file_transfer','enable_audio','enable_tcp','enable_remote_restart','enable_recording','enable_blocking_input','enable_remote_modi','enable_printer','enable_camera','enable_terminal','cycle_monitor','x_offline','android_app_id','app_icon_url','app_logo_url','privacy_screen_url']

    const loadPresetIntoForm = (preset) => {
      if (!preset) return
      try {
        const cfg = JSON.parse(preset.custom_json || '{}')
        for (const f of PRESET_FIELDS) {
          if (f in cfg && cfg[f] !== undefined) form[f] = cfg[f]
        }
        // platform/version/app_name live on the preset record, not in custom_json
        if (preset.platform) form.platform = preset.platform
        if (preset.version) form.version = preset.version
        if (preset.app_name !== undefined) form.app_name = preset.app_name
        ElMessage.success(T('OperationSuccess'))
      } catch (e) {
        console.error('preset custom_json parse error', e)
      }
    }

    const onPresetSelect = (id) => {
      if (!id) return
      const preset = presets.value.find(p => p.id === id)
      if (preset) loadPresetIntoForm(preset)
    }

    const saveCurrentAsPreset = async () => {
      try {
        const name = await ElMessageBox.prompt(T('PresetName') || 'Preset name', T('SaveAsPreset') || 'Save as preset', { inputPlaceholder: 'My Preset' })
        if (!name || !name.value) return
        // Derived from PRESET_FIELDS so submit + save preset stay in sync.
        const customPayload = {}
        for (const f of PRESET_FIELDS) customPayload[f] = form[f]
        const customJson = JSON.stringify(customPayload)
        await createPreset({
          name: name.value,
          platform: form.platform,
          version: form.version,
          app_name: form.app_name,
          custom_json: customJson,
        })
        ElMessage.success(T('OperationSuccess'))
        await loadPresets()
      } catch (e) {
        if (e !== 'cancel') console.error(e)
      }
    }

    const deletePreset = async (preset) => {
      try {
        await ElMessageBox.confirm(T('Confirm?'), { type: 'warning' })
        await removePreset({ id: preset.id })
        if (selectedPresetId.value === preset.id) selectedPresetId.value = null
        ElMessage.success(T('OperationSuccess'))
        await loadPresets()
      } catch (e) {
        if (e !== 'cancel') console.error(e)
      }
    }

    const uploadImage = async (opts, field) => {
      try {
        const file = opts.file
        if (!file) return
        if (file.type !== 'image/png' && !file.name.toLowerCase().endsWith('.png')) {
          ElMessage.error('PNG only')
          return
        }
        const fd = new FormData()
        fd.append('file', file)
        const res = await uploadFile(fd)
        if (res?.data?.url) {
          form[field] = res.data.url
          ElMessage.success(T('OperationSuccess'))
        } else {
          ElMessage.error('Upload failed')
        }
      } catch (e) {
        console.error(e)
        ElMessage.error('Upload failed: ' + (e?.message || e))
      }
    }

    const submitBuild = async () => {
      form.server_ip = stripPort(form.server_ip)
      submitting.value = true
      try {
        // Derived from PRESET_FIELDS so submit + save preset stay in sync.
        const customPayload = {}
        for (const f of PRESET_FIELDS) customPayload[f] = form[f]
        const customJson = JSON.stringify(customPayload)
        await create({
          name: form.app_name || `${form.platform}-${form.version}`,
          platform: form.platform,
          version: form.version,
          app_name: form.app_name,
          custom_json: customJson,
        })
        ElMessage.success(T('OperationSuccess'))
        resetForm()
        loadBuilds()
      } catch (e) {
        console.error(e)
      } finally {
        submitting.value = false
      }
    }

    const deleteBuild = async (row) => {
      try {
        await ElMessageBox.confirm(T('Confirm?'), { type: 'warning' })
        await remove({ id: row.id })
        ElMessage.success(T('OperationSuccess'))
        loadBuilds()
      } catch (e) {
        if (e !== 'cancel') console.error(e)
      }
    }

    const resetForm = () => {
      form.platform = 'windows'
      form.version = '1.4.7'
      form.app_name = ''
      form.server_ip = ''
      form.key = ''
      form.api_server = ''
      form.relay_server = ''
      form.company_name = ''
      form.download_url = ''
      form.direction = 'both'
      form.pass_approve_mode = 'password-click'
      form.permanent_password = ''
      form.deny_lan = false
      form.enable_direct_ip = false
      form.auto_close = false
      form.hide_cm = false
      form.theme = 'system'
      form.remove_wallpaper = true
      form.permissions_type = 'custom'
      form.enable_keyboard = true
      form.enable_clipboard = true
      form.enable_file_transfer = true
      form.enable_audio = true
      form.enable_tcp = true
      form.enable_remote_restart = true
      form.enable_recording = true
      form.enable_blocking_input = true
      form.enable_remote_modi = false
      form.enable_printer = true
      form.enable_camera = true
      form.enable_terminal = true
      form.cycle_monitor = false
      form.x_offline = false
      form.remove_new_version_notif = false
      form.android_app_id = ''
      form.app_icon_url = ''
      form.app_logo_url = ''
      form.privacy_screen_url = ''
    }

    const downloadBuild = (row) => {
      window.open(`/api/admin/custom_build/public/download/${row.download_key}`, '_blank')
    }

    const statusType = (s) => {
      switch (s) {
        case 'pending': return 'info'
        case 'building': return 'warning'
        case 'done': return 'success'
        case 'failed': return 'danger'
        default: return 'info'
      }
    }

    const statusLabel = (s) => {
      switch (s) {
        case 'pending': return 'Pending'
        case 'building': return 'Building'
        case 'done': return 'Done'
        case 'failed': return 'Failed'
        default: return s
      }
    }

    watch([page, pageSize], loadBuilds)
    onMounted(async () => {
      loadBuilds()
      loadPresets()
      try {
        const res = await fetchConfig()
        if (res?.data) {
          form.server_ip = stripPort(res.data.id_server || '')
          form.key = res.data.key || ''
          form.api_server = res.data.api_server || ''
          form.relay_server = res.data.relay_server || ''
        }
      } catch (e) {
        // user can fill manually
      }
    })

    return {
      form, builds, loading, submitting, page, pageSize, total, versions,
      submitBuild, deleteBuild, resetForm, downloadBuild, stripServerPort,
      statusType, statusLabel, T,
      presets, selectedPresetId, onPresetSelect, saveCurrentAsPreset, deletePreset, uploadImage,
    }
  },
})
</script>

<style scoped lang="scss">
.mb-20 {
  margin-bottom: 20px;
}

.build-history {
  :deep(.el-pagination) {
    justify-content: flex-end;
    margin-top: 16px;
  }
}
</style>
