<template>
  <div class="custom-client">
    <page-header
        title="Custom Client Builder"
        subtitle="Create branded, RustDesk-compatible client builds with pinned server, security, permissions, and branding settings."
        eyebrow="Client Builder"
        pulse="warning"
    />
    <page-section class="mb-20" :title="T('NewBuild')" subtitle="Configure the build payload and optionally save or load reusable presets.">
      <el-form ref="formRef" :model="form" :rules="rules" :validate-on-rule-change="false" label-width="180px" v-loading="submitting">
        <el-row :gutter="20" class="mb-10">
          <el-col :span="12">
            <el-form-item :label="T('LoadPreset')" label-width="120px" class="preset-form-item">
              <div class="preset-controls">
                <el-select ref="presetSelectRef" v-model="selectedPresetId" class="preset-select" placeholder="Select preset" clearable @change="onPresetSelect">
                <el-option v-for="p in presets" :key="p.id" :label="p.name + ' (' + p.platform + ')'" :value="p.id">
                  <span style="float:left">{{ p.name }} <el-tag size="small" effect="plain" style="margin-left:6px">{{ p.platform }}</el-tag></span>
                </el-option>
                </el-select>
                <div class="preset-actions">
                  <div class="preset-action-group preset-action-group--save">
                    <el-button type="primary" plain size="small" @click="saveCurrentAsPreset">{{ T('SaveAsPreset') }}</el-button>
                  </div>
                  <div v-if="selectedPreset" class="preset-action-group preset-action-group--danger">
                    <el-button type="danger" plain size="small" :aria-label="`${T('Delete')}: ${selectedPreset.name}`" @click="deletePreset(selectedPreset)">{{ T('Delete') }}</el-button>
                  </div>
                </div>
              </div>
            </el-form-item>
          </el-col>
        </el-row>
        <el-divider content-position="left">{{ T('Platform') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('Platform')" prop="platform">
              <!--
                Windows x64 is validated end-to-end (GitHub Actions). Linux/Android remain
                typed legacy values, but are unavailable for production builds pending PR11
                evidence. 32-bit Windows and macOS are not supported (PLAN.md §8.15).
              -->
              <el-tooltip :content="requiredMessage('platform')" :disabled="!isFieldInvalid('platform')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-select
                  v-model="form.platform"
                  style="width:100%"
                  :id="fieldInputId('platform')"
                  :aria-required="isRequiredField('platform')"
                  :aria-invalid="isFieldInvalid('platform')"
                  :aria-describedby="isFieldInvalid('platform') ? fieldErrorId('platform') : undefined"
                  :validate-event="false"
                  @change="onPlatformChange"
                >
                  <el-option :label="T('PlatformWindows')" value="windows" />
                  <el-option :label="T('PlatformLinuxUnavailable')" value="linux" disabled />
                  <el-option :label="T('PlatformAndroidUnavailable')" value="android" disabled />
                </el-select>
              </el-tooltip>
              <template #error="{ error }">
                <span :id="fieldErrorId('platform')" aria-live="polite">{{ error }}</span>
              </template>
              <div v-if="form.platform && !productionPlatformReady" class="version-hint version-hint--error" role="alert">
                {{ T('ProductionPlatformUnavailable') }}
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('Version')" prop="version">
              <el-tooltip :content="requiredMessage('version')" :disabled="!isFieldInvalid('version')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-select
                  v-model="form.version"
                  style="width:100%"
                  :aria-busy="versionsState === 'loading'"
                  :id="fieldInputId('version')"
                  :aria-required="isRequiredField('version')"
                  :aria-invalid="isFieldInvalid('version')"
                  :aria-describedby="isFieldInvalid('version') ? fieldErrorId('version') : undefined"
                  :validate-event="false"
                  @change="clearFieldError('version')"
                >
                  <el-option v-for="v in versions" :key="v.version" :label="v.version" :value="v.version" />
                </el-select>
              </el-tooltip>
              <template #error="{ error }">
                <span :id="fieldErrorId('version')" aria-live="polite">{{ error }}</span>
              </template>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('AppName')" prop="app_name">
              <el-tooltip :content="requiredMessage('app_name')" :disabled="!isFieldInvalid('app_name')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-input
                  v-model="form.app_name"
                  :id="fieldInputId('app_name')"
                  :aria-required="isRequiredField('app_name')"
                  :aria-invalid="isFieldInvalid('app_name')"
                  :aria-describedby="isFieldInvalid('app_name') ? fieldErrorId('app_name') : undefined"
                  :validate-event="false"
                  @input="clearFieldError('app_name')"
                />
              </el-tooltip>
              <template #error="{ error }">
                <span :id="fieldErrorId('app_name')" aria-live="polite">{{ error }}</span>
              </template>
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider content-position="left">{{ T('CustomServer') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('Host')" prop="server_ip">
              <el-tooltip :content="requiredMessage('server_ip')" :disabled="!isFieldInvalid('server_ip')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-input
                  v-model="form.server_ip"
                  :placeholder="T('HostEndpointPlaceholder')"
                  :id="fieldInputId('server_ip')"
                  :aria-required="isRequiredField('server_ip')"
                  :aria-invalid="isFieldInvalid('server_ip')"
                  :aria-describedby="isFieldInvalid('server_ip') ? fieldErrorId('server_ip') : undefined"
                  :validate-event="false"
                  @input="clearFieldError('server_ip')"
                >
                  <template #append>
                    <el-tooltip :content="T('HostEndpointHint')" placement="top" :trigger="['hover', 'focus']">
                      <button type="button" class="endpoint-hint-trigger" :aria-label="T('HostEndpointHint')">
                        <el-icon aria-hidden="true"><InfoFilled /></el-icon>
                      </button>
                    </el-tooltip>
                  </template>
                </el-input>
              </el-tooltip>
              <template #error="{ error }">
                <span :id="fieldErrorId('server_ip')" aria-live="polite">{{ error }}</span>
              </template>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('Key')" prop="key">
              <el-tooltip :content="requiredMessage('key')" :disabled="!isFieldInvalid('key')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-input
                  v-model="form.key"
                  placeholder="encryption key"
                  :id="fieldInputId('key')"
                  :aria-required="isRequiredField('key')"
                  :aria-invalid="isFieldInvalid('key')"
                  :aria-describedby="isFieldInvalid('key') ? fieldErrorId('key') : undefined"
                  :validate-event="false"
                  @input="clearFieldError('key')"
                />
              </el-tooltip>
              <template #error="{ error }">
                <span :id="fieldErrorId('key')" aria-live="polite">{{ error }}</span>
              </template>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('ApiServer')" prop="api_server">
              <el-tooltip :content="requiredMessage('api_server')" :disabled="!isFieldInvalid('api_server')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-input
                  v-model="form.api_server"
                  placeholder="https://your-server:21114"
                  :id="fieldInputId('api_server')"
                  :aria-required="isRequiredField('api_server')"
                  :aria-invalid="isFieldInvalid('api_server')"
                  :aria-describedby="isFieldInvalid('api_server') ? fieldErrorId('api_server') : undefined"
                  :validate-event="false"
                  @input="clearFieldError('api_server')"
                />
              </el-tooltip>
              <template #error="{ error }">
                <span :id="fieldErrorId('api_server')" aria-live="polite">{{ error }}</span>
              </template>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('RelayServer')" prop="relay_server">
              <el-tooltip :content="requiredMessage('relay_server')" :disabled="!isFieldInvalid('relay_server')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-input
                  v-model="form.relay_server"
                  :placeholder="T('RelayEndpointPlaceholder')"
                  :id="fieldInputId('relay_server')"
                  :aria-required="isRequiredField('relay_server')"
                  :aria-invalid="isFieldInvalid('relay_server')"
                  :aria-describedby="isFieldInvalid('relay_server') ? fieldErrorId('relay_server') : undefined"
                  :validate-event="false"
                  @input="clearFieldError('relay_server')"
                >
                  <template #append>
                    <el-tooltip :content="T('RelayEndpointHint')" placement="top" :trigger="['hover', 'focus']">
                      <button type="button" class="endpoint-hint-trigger" :aria-label="T('RelayEndpointHint')">
                        <el-icon aria-hidden="true"><InfoFilled /></el-icon>
                      </button>
                    </el-tooltip>
                  </template>
                </el-input>
              </el-tooltip>
              <template #error="{ error }">
                <span :id="fieldErrorId('relay_server')" aria-live="polite">{{ error }}</span>
              </template>
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
            <el-form-item :label="T('PermanentPassword')" prop="permanent_password">
              <el-tooltip :content="requiredMessage('permanent_password')" :disabled="!isFieldInvalid('permanent_password')" placement="top" :trigger="['hover', 'focus']" :trigger-keys="[]">
                <el-input
                  v-model="form.permanent_password"
                  :type="showPermanentPassword ? 'text' : 'password'"
                  :id="fieldInputId('permanent_password')"
                  :aria-required="isRequiredField('permanent_password')"
                  :aria-invalid="isFieldInvalid('permanent_password')"
                  :aria-describedby="isFieldInvalid('permanent_password') ? fieldErrorId('permanent_password') : undefined"
                  :validate-event="false"
                  @input="clearFieldError('permanent_password')"
                >
                  <template #append>
                    <button
                      type="button"
                      class="secret-toggle"
                      :aria-label="T(showPermanentPassword ? 'HidePassword' : 'ShowPassword')"
                      :aria-pressed="showPermanentPassword"
                      @click="showPermanentPassword = !showPermanentPassword"
                    >
                      {{ T(showPermanentPassword ? 'HidePassword' : 'ShowPassword') }}
                    </button>
                  </template>
                </el-input>
              </el-tooltip>
              <el-button
                v-if="selectedPreset?.has_permanent_password === true"
                type="warning"
                link
                size="small"
                class="clear-saved-password"
                @click="clearSavedPresetPassword"
              >
                {{ T('ClearSavedPassword') }}
              </el-button>
              <template #error="{ error }">
                <span :id="fieldErrorId('permanent_password')" aria-live="polite">{{ error }}</span>
              </template>
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
              <el-switch :active-value="true" :inactive-value="false" v-model="form.hide_cm" @change="onHideConnectionManagementChange" />
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

        <el-divider content-position="left">{{ T('Branding') }}</el-divider>
        <el-row :gutter="20">
          <el-col :span="8">
            <el-form-item :label="T('AppIcon')">
              <el-input v-model="form.app_icon_url" placeholder="/upload/20260101/icon.png" clearable>
                <template #append>
                  <el-upload :show-file-list="false" :auto-upload="true" :http-request="(opts) => uploadImage(opts, 'app_icon_url')" accept="image/png">
                    <el-button>{{ T('Upload') }}</el-button>
                  </el-upload>
                </template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('AppLogo')">
              <el-input v-model="form.app_logo_url" placeholder="/upload/20260101/logo.png" clearable>
                <template #append>
                  <el-upload :show-file-list="false" :auto-upload="true" :http-request="(opts) => uploadImage(opts, 'app_logo_url')" accept="image/png">
                    <el-button>{{ T('Upload') }}</el-button>
                  </el-upload>
                </template>
              </el-input>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item :label="T('PrivacyScreen')">
              <el-input v-model="form.privacy_screen_url" placeholder="/upload/20260101/privacy.png" clearable>
                <template #append>
                  <el-upload :show-file-list="false" :auto-upload="true" :http-request="(opts) => uploadImage(opts, 'privacy_screen_url')" accept="image/png">
                    <el-button>{{ T('Upload') }}</el-button>
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
          <el-button type="primary" @click="submitBuild" :loading="submitting" :disabled="!versionsReady || (!!form.platform && !productionPlatformReady) || submitting">{{ T('StartBuild') }}</el-button>
          <el-button @click="resetForm">{{ T('Reset') }}</el-button>
          <span class="version-status" role="status" aria-live="polite" aria-atomic="true">
            <span v-if="versionsState === 'loading'" class="version-hint version-hint--loading">{{ T('VersionListLoading') }}</span>
            <span v-else-if="versionsState === 'empty'" class="version-hint version-hint--empty">{{ T('VersionListEmpty') }}</span>
            <span v-else-if="versionsState === 'error'" class="version-hint version-hint--error">{{ versionsError || T('VersionListError') }}</span>
          </span>
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
          <el-popover v-if="row.build_log" placement="top" trigger="click" :width="480" popper-class="build-log-popover">
            <template #default>
              <pre style="margin:0;max-width:480px;max-height:300px;overflow:auto;white-space:pre-wrap;word-break:break-word;font-size:12px;line-height:1.4">{{ row.build_log }}</pre>
            </template>
            <template #reference>
              <button
                type="button"
                class="build-status-trigger"
                aria-haspopup="dialog"
                 :aria-label="`${T('ViewBuildLog')}: #${row.id} — ${row.app_name || ''} — ${T(statusLabel(row.status))}`"
              >
                <el-tag :class="['build-status-tag', `build-status-tag--${statusType(row.status)}`]" :type="statusType(row.status)" size="small">{{ T(statusLabel(row.status)) }}</el-tag>
              </button>
            </template>
          </el-popover>
          <el-tag v-else :class="['build-status-tag', `build-status-tag--${statusType(row.status)}`]" :type="statusType(row.status)" size="small">{{ T(statusLabel(row.status)) }}</el-tag>
        </template>
        <template #actions="{ row }">
          <el-button v-if="row.status === 'done'" type="success" size="small" :loading="downloadingBuildId === row.id" @click="downloadBuild(row)">{{ T('Download') }}</el-button>
          <el-button type="danger" size="small" @click="deleteBuild(row)">{{ T('Delete') }}</el-button>
        </template>
      </data-table>
      <p class="build-history-status" role="status" aria-live="polite" aria-atomic="true">{{ buildHistoryStatus }}</p>
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
import { defineComponent, ref, reactive, computed, nextTick, onMounted, onUnmounted, watch } from 'vue'
import { list, create, remove, download, getVersions } from '@/api/custom_client'
import { list as listPresets, create as createPreset, remove as removePreset } from '@/api/custom_preset'
import { all as fetchConfig } from '@/api/config'
import { upload as uploadFile } from '@/api/file'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'
import { T } from '@/utils/i18n'
import { downBlob } from '@/utils/file'
import { InfoFilled } from '@element-plus/icons-vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageSection from '@/components/ui/PageSection.vue'
import DataTable from '@/components/ui/DataTable.vue'

// Версии загружаются с API (GET /admin/custom_build/versions) при монтировании.
// Список формируется из GitHub-релизов форка с тегами offline-assets-*.
// Состояние загрузки выражено через versionsState: 'loading' | 'ready' | 'empty' | 'error'.
// Никаких фолбэков: при 'empty'/'error' старт билда блокируется, а preset
// сохраняет только явно сохранённые version/app_name значения.

const extractApiError = (error, fallbackKey) => {
  const envelopes = [error?.response?.data, Number.isInteger(error?.code) ? error : null]
  for (const envelope of envelopes) {
    const messages = [envelope?.message, envelope?.data?.message]
    const message = messages.find(value => typeof value === 'string' && value.trim())
    if (message) return message.trim()
  }
  return T(fallbackKey)
}

export default defineComponent({
  name: 'CustomClientBuilds',
  components: { PageHeader, PageSection, DataTable, InfoFilled },
  setup () {
    const formRef = ref(null)
    const showPermanentPassword = ref(false)
    const FORM_DEFAULTS = {
      platform: '',
      version: '',
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
    }
    const form = reactive({ ...FORM_DEFAULTS })
    const explicitPresetFields = ref(new Set())
    const serverConfigDefaults = reactive({
      server_ip: '',
      key: '',
      api_server: '',
      relay_server: '',
    })
    const presetPasswordClearIntent = ref(false)

    const requiredFieldNames = ['platform', 'version', 'app_name', 'server_ip', 'key', 'api_server', 'relay_server', 'permanent_password']
    const requiredMessageKeys = {
      platform: 'CustomClientPlatformRequired',
      version: 'CustomClientVersionRequired',
      app_name: 'CustomClientAppNameRequired',
      server_ip: 'CustomClientHostRequired',
      key: 'CustomClientKeyRequired',
      api_server: 'CustomClientApiServerRequired',
      relay_server: 'CustomClientRelayServerRequired',
      permanent_password: 'CustomClientPermanentPasswordRequired',
    }
    const invalidFields = ref({})
    const requiredMessage = (field) => T(requiredMessageKeys[field])
    const requiredFieldSet = computed(() => {
      const fields = new Set()
      if (!form.platform || form.platform === 'windows') {
        fields.add('platform')
      }
      if (form.platform === 'windows') {
        for (const field of ['version', 'app_name', 'server_ip', 'key', 'api_server', 'relay_server']) {
          fields.add(field)
        }
      }
      if (form.hide_cm) fields.add('permanent_password')
      return fields
    })
    const isRequiredField = (field) => requiredFieldSet.value.has(field)
    const fieldErrorId = (field) => `custom-client-${field}-error`
    const requiredRule = (field) => ({
      required: true,
      whitespace: true,
      message: requiredMessage(field),
      trigger: ['blur', 'change'],
    })
    const rules = computed(() => Object.fromEntries(
      requiredFieldNames
        .filter(isRequiredField)
        .map((field) => [field, [requiredRule(field)]])
    ))

    const isFieldInvalid = (field) => Boolean(invalidFields.value[field])
    const fieldInputId = (field) => `custom-client-${field}-input`
    const syncFieldAria = async () => {
      if (typeof document === 'undefined') return
      await nextTick()
      for (const field of requiredFieldNames) {
        const control = document.getElementById(fieldInputId(field))
        if (!control) continue
        const invalid = isFieldInvalid(field)
        control.setAttribute('aria-required', String(isRequiredField(field)))
        control.setAttribute('aria-invalid', String(invalid))
        if (invalid) control.setAttribute('aria-describedby', fieldErrorId(field))
        else control.removeAttribute('aria-describedby')
      }
    }
    watch([requiredFieldSet, invalidFields], syncFieldAria, { deep: true, flush: 'post' })
    const clearFieldError = (field) => {
      if (invalidFields.value[field]) {
        const nextInvalidFields = { ...invalidFields.value }
        delete nextInvalidFields[field]
        invalidFields.value = nextInvalidFields
      }
      formRef.value?.clearValidate?.(field)
      syncFieldAria()
    }
    const onHideConnectionManagementChange = () => {
      clearFieldError('permanent_password')
    }
    const onPlatformChange = () => {
      invalidFields.value = {}
      formRef.value?.clearValidate?.()
      syncFieldAria()
    }
    const focusFirstInvalid = async (fields) => {
      const firstInvalidField = requiredFieldNames.find((field) => fields?.[field])
      if (!firstInvalidField) return
      await nextTick()
      const field = formRef.value?.fields?.find((entry) => entry.prop === firstInvalidField)
      const control = document.getElementById(fieldInputId(firstInvalidField))
        || field?.$el?.querySelector('input, textarea, [role="combobox"], button')
      control?.focus?.({ preventScroll: true })
      formRef.value?.scrollToField?.(firstInvalidField)
    }
    const validateBuildForm = async () => {
      if (!formRef.value) return false
      invalidFields.value = {}
      const valid = await formRef.value.validate().catch(async (fields) => {
        invalidFields.value = fields || {}
        await focusFirstInvalid(fields)
        return false
      })
      return valid !== false
    }
    const canPreservePresetPassword = (presetName) =>
      !presetPasswordClearIntent.value
      && !String(form.permanent_password ?? '').trim()
      && presets.value.find((preset) => preset.name === presetName)?.has_permanent_password === true

    const validatePresetPassword = async (presetName) => {
      if (!form.hide_cm || String(form.permanent_password ?? '').trim() || canPreservePresetPassword(presetName)) return true
      invalidFields.value = {
        ...invalidFields.value,
        permanent_password: [requiredMessage('permanent_password')],
      }
      try {
        await formRef.value?.validateField?.('permanent_password')
      } catch (_) {
        // The form item renders the existing concise rule message.
      }
      await focusFirstInvalid({ permanent_password: [requiredMessage('permanent_password')] })
      return false
    }

    const resetFormFields = () => {
      Object.assign(form, FORM_DEFAULTS)
      explicitPresetFields.value = new Set()
      presetPasswordClearIntent.value = false
      showPermanentPassword.value = false
      invalidFields.value = {}
      formRef.value?.clearValidate?.()
    }

    const builds = ref([])
    const buildHistoryStatus = ref('')
    const loading = ref(false)
    const submitting = ref(false)
    const downloadingBuildId = ref(null)
    // versionsState: 'loading' | 'ready' | 'empty' | 'error'.
    // Нужно различать
    // "ещё грузится" и "загрузилось, но пусто/ошибка", чтобы:
    //   - loadPresetIntoForm не подменял сохранённую версию значением каталога
    //     при 'empty'/'error' (иначе форма скрывает проблему);
    //   - submitBuild и StartBuild были заблокированы для ВСЕХ не-ready состояний.
    const versionsState = ref('loading')
    const versionsReady = computed(() => versionsState.value === 'ready')
    // Linux/Android remain valid persisted enum values for legacy presets, but
    // the backend production capability gate keeps them unavailable until PR11.
    const productionPlatformReady = computed(() => form.platform === 'windows')
    const page = ref(1)
    const pageSize = ref(10)
    const total = ref(0)
    const versions = ref([])
    const versionsError = ref('')
    // One component guard for all async state operations. Per-operation generations
    // let the initial requests run in parallel while invalidating older refreshes.
    const requestGenerations = Object.create(null)
    const lifecycleGuard = {
      mounted: false,
      start (key) {
        requestGenerations[key] = (requestGenerations[key] || 0) + 1
        return requestGenerations[key]
      },
      isCurrent (key, generation) {
        return lifecycleGuard.mounted && requestGenerations[key] === generation
      },
    }

    // B-017: при silent=true не дёргаем спиннер — для фонового поллинга.
    const loadBuilds = async (silent = false) => {
      if (!lifecycleGuard.mounted) return null
      const generation = lifecycleGuard.start('builds')
      if (!silent) loading.value = true
      try {
        const res = await list({ page: page.value, page_size: pageSize.value })
        if (!lifecycleGuard.isCurrent('builds', generation)) return generation
        const nextBuilds = res.data.list || []
        if (silent) announceBuildStatusChanges(builds.value, nextBuilds)
        builds.value = nextBuilds
        total.value = res.data.total || 0
      } catch (e) {
        if (lifecycleGuard.isCurrent('builds', generation)) console.error(e)
      } finally {
        if (lifecycleGuard.isCurrent('builds', generation) && !silent) loading.value = false
      }
      if (lifecycleGuard.isCurrent('builds', generation)) ensurePolling()
      return generation
    }

    // B-017: история билдов не обновлялась сама — строки висели в pending/building,
    // пока пользователь не перезагрузит. Поллим список пока есть незавершённые
    // билды и останавливаемся, когда все терминальные.
    const POLL_MS = 12000
    let pollTimer = null
    const hasActiveBuilds = () =>
      builds.value.some((b) => ['pending', 'building', 'downloading', 'extracting'].includes(b.status))
    const announceBuildStatusChanges = (previousBuilds, nextBuilds) => {
      const previousById = new Map(previousBuilds.map((build) => [build.id, build.status]))
      const changedBuilds = nextBuilds.filter((build) =>
        previousById.has(build.id) && previousById.get(build.id) !== build.status
      )
      if (!changedBuilds.length) return
      buildHistoryStatus.value = T('BuildHistoryStatusChanged', {
        param: changedBuilds
          .map((build) => `${build.app_name || `#${build.id}`} — ${T(statusLabel(build.status))}`)
          .join('; '),
      })
    }
    const stopPolling = () => {
      if (pollTimer) {
        clearInterval(pollTimer)
        pollTimer = null
      }
    }
    const ensurePolling = () => {
      if (!lifecycleGuard.mounted) return
      if (!hasActiveBuilds()) {
        stopPolling()
        return
      }
      if (pollTimer) return
      pollTimer = setInterval(async () => {
        if (!lifecycleGuard.mounted) {
          stopPolling()
          return
        }
        const generation = await loadBuilds(true)
        if (generation !== null && lifecycleGuard.isCurrent('builds', generation) && !hasActiveBuilds()) stopPolling()
      }, POLL_MS)
    }
    onUnmounted(() => {
      lifecycleGuard.mounted = false
      stopPolling()
    })

    const presets = ref([])
    const selectedPresetId = ref(null)
    const presetSelectRef = ref(null)
    const selectedPreset = computed(() => presets.value.find((preset) => preset.id === selectedPresetId.value) || null)
    const clearSavedPresetPassword = () => {
      if (selectedPreset.value?.has_permanent_password !== true) return
      form.permanent_password = ''
      presetPasswordClearIntent.value = true
      showPermanentPassword.value = false
      clearFieldError('permanent_password')
    }
    const loadPresets = async () => {
      if (!lifecycleGuard.mounted) return
      const generation = lifecycleGuard.start('presets')
      try {
        const res = await listPresets({ page: 1, page_size: 100 })
        if (!lifecycleGuard.isCurrent('presets', generation)) return
        presets.value = res.data.list || []
      } catch (e) {
        if (lifecycleGuard.isCurrent('presets', generation)) console.error(e)
      }
    }

    // Single source of truth for the fields persisted inside custom_json.
    // Used by loadPresetIntoForm + saveCurrentAsPreset + submitBuild — extending one
    // without the other reintroduces the "saved but not restored" bug (audit §8.9).
    // platform/version/app_name are stored on the preset record itself, not in custom_json.
    const PRESET_FIELDS = ['server_ip','key','api_server','relay_server','company_name','download_url','direction','pass_approve_mode','permanent_password','deny_lan','enable_direct_ip','auto_close','hide_cm','theme','remove_wallpaper','remove_new_version_notif','permissions_type','enable_keyboard','enable_clipboard','enable_file_transfer','enable_audio','enable_tcp','enable_remote_restart','enable_recording','enable_blocking_input','enable_remote_modi','enable_printer','enable_camera','enable_terminal','cycle_monitor','x_offline','android_app_id','app_icon_url','app_logo_url','privacy_screen_url']

    const applyServerConfigDefaults = () => {
      for (const field of Object.keys(serverConfigDefaults)) {
        if (!explicitPresetFields.value.has(field) && !form[field]) {
          form[field] = serverConfigDefaults[field]
        }
      }
    }

    const loadPresetIntoForm = (preset) => {
      if (!preset) return
      try {
        const parsedConfig = JSON.parse(preset.custom_json || '{}')
        const cfg = parsedConfig && typeof parsedConfig === 'object' && !Array.isArray(parsedConfig)
          ? parsedConfig
          : {}
        const explicitFields = new Set()
        // A partial/legacy preset starts from the same blank/default state as a
        // new form. Never let values from the previously edited form leak into it.
        resetFormFields()
        for (const f of PRESET_FIELDS) {
          if (Object.prototype.hasOwnProperty.call(cfg, f) && cfg[f] !== undefined) {
            form[f] = cfg[f]
            explicitFields.add(f)
          }
        }
        // platform/version/app_name live on the preset record, not in custom_json.
        // Restore only explicit stored values; a preset must never invent a value
        // when its record is empty or its version is not in the current catalog.
        for (const field of ['platform', 'version', 'app_name']) {
          if (Object.prototype.hasOwnProperty.call(preset, field)) {
            form[field] = preset[field] ?? ''
            explicitFields.add(field)
          }
        }
        explicitPresetFields.value = explicitFields
        applyServerConfigDefaults()
        invalidFields.value = {}
        formRef.value?.clearValidate?.()
        ElMessage.success(T('OperationSuccess'))
      } catch (e) {
        console.error('preset custom_json parse error', e)
      }
    }

    const onPresetSelect = (id) => {
      if (!id) {
        resetForm()
        return
      }
      const preset = presets.value.find(p => p.id === id)
      if (preset) loadPresetIntoForm(preset)
    }

    const saveCurrentAsPreset = async () => {
      try {
        const name = await ElMessageBox.prompt(T('PresetName'), T('SaveAsPreset'), { inputPlaceholder: 'My Preset' })
        if (!name || !name.value) return
        if (!await validatePresetPassword(name.value)) return
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
          preserve_permanent_password: canPreservePresetPassword(name.value),
        })
        ElMessage.success(T('OperationSuccess'))
        await loadPresets()
      } catch (e) {
        if (e !== 'cancel') console.error(e)
      }
    }

    const deletePreset = async (preset) => {
      try {
        await ElMessageBox.confirm(T('Confirm?', { param: T('Delete') }), { type: 'warning' })
        await removePreset({ id: preset.id })
        if (selectedPresetId.value === preset.id) selectedPresetId.value = null
        ElMessage.success(T('OperationSuccess'))
        await loadPresets()
        await nextTick()
        presetSelectRef.value?.focus?.()
      } catch (e) {
        // Errors are toasted globally by the axios interceptor; just log here.
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
          throw new Error('Upload response did not include a URL')
        }
      } catch (e) {
        const isInterceptorHandledFailure = axios.isAxiosError(e)
          || Boolean(e?.response || e?.request)
          || (Number.isInteger(e?.code) && e.code !== 0)
        if (!isInterceptorHandledFailure) ElMessage.error('Upload failed')
      }
    }

    const submitBuild = async () => {
      if (!await validateBuildForm()) return
      if (!productionPlatformReady.value) {
        ElMessage.warning(T('ProductionPlatformUnavailable'))
        return
      }
      if (versionsState.value !== 'ready') {
        // Различаем loading / empty / error, чтобы пользователь понимал, чего ждать.
        const key = versionsState.value === 'loading'
          ? 'VersionListLoading'
          : versionsState.value === 'empty'
            ? 'VersionListEmpty'
            : 'VersionListError'
        ElMessage.warning(T(key))
        return
      }
      submitting.value = true
      try {
        // Derived from PRESET_FIELDS so submit + save preset stay in sync.
        const customPayload = {}
        for (const f of PRESET_FIELDS) customPayload[f] = form[f]
        const customJson = JSON.stringify(customPayload)
        await create({
          name: form.app_name,
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
        await ElMessageBox.confirm(T('Confirm?', { param: T('Delete') }), { type: 'warning' })
        await remove({ id: row.id })
        ElMessage.success(T('OperationSuccess'))
        loadBuilds()
      } catch (e) {
        // Backend/network errors are already toasted globally by the axios
        // response interceptor (utils/request.js); only log unexpected JS errors.
        if (e !== 'cancel') console.error(e)
      }
    }

    const resetForm = () => {
      selectedPresetId.value = null
      resetFormFields()
    }

    const downloadBuild = async (row) => {
      if (downloadingBuildId.value === row.id) return
      downloadingBuildId.value = row.id
      try {
        const res = await download(row.id)
        const contentType = (res.headers?.['content-type'] || '').split(';', 1)[0].trim().toLowerCase()
        if (!['application/zip', 'application/octet-stream'].includes(contentType)) {
          throw new Error('Download failed')
        }
        const disposition = res.headers?.['content-disposition'] || ''
        const filename = disposition.match(/filename="?([^";]+)"?/i)?.[1] || `custom-build-${row.id}.zip`
        downBlob(res.data, filename)
      } catch (e) {
        const isInterceptorHandledFailure = axios.isAxiosError(e)
          || Boolean(e?.interceptorHandled)
          || Boolean(e?.response || e?.request)
          || (Number.isInteger(e?.code) && e.code !== 0)
        if (!isInterceptorHandledFailure) ElMessage.error('Download failed')
      } finally {
        downloadingBuildId.value = null
      }
    }

    const statusType = (s) => {
      switch (s) {
        case 'pending': return 'info'
        case 'building': return 'warning'
        case 'downloading': return 'warning'
        case 'extracting': return 'warning'
        case 'done': return 'success'
        case 'failed': return 'danger'
        default: return 'info'
      }
    }

    const statusLabel = (s) => {
      switch (s) {
        case 'pending': return 'Pending'
        case 'building': return 'Building'
        case 'downloading': return 'Downloading'
        case 'extracting': return 'Extracting'
        case 'done': return 'Done'
        case 'failed': return 'Failed'
        default: return s
      }
    }

    const loadVersions = async () => {
      if (!lifecycleGuard.mounted) return
      const generation = lifecycleGuard.start('versions')
      versionsError.value = ''
      try {
        const res = await getVersions()
        if (!lifecycleGuard.isCurrent('versions', generation)) return
        const data = res?.data || {}
        const list = data.versions || []
        versions.value = list
        if (data.error || data.message) {
          versionsError.value = extractApiError(data, 'VersionListError')
          versionsState.value = 'error'
        } else {
          versionsError.value = ''
          versionsState.value = list.length > 0 ? 'ready' : 'empty'
        }
      } catch (e) {
        if (!lifecycleGuard.isCurrent('versions', generation)) return
        versionsError.value = extractApiError(e, 'VersionListError')
        versions.value = []
        versionsState.value = 'error'
      }
    }

    const loadConfig = async () => {
      if (!lifecycleGuard.mounted) return
      const generation = lifecycleGuard.start('config')
      try {
        const res = await fetchConfig()
        if (!lifecycleGuard.isCurrent('config', generation)) return
        if (res?.data) {
          // B-016: retain server defaults separately so a partial preset can reset
          // the form and reapply them without overwriting explicit preset fields.
          const cfg = res.data
          Object.assign(serverConfigDefaults, {
            server_ip: cfg.id_server || '',
            key: cfg.key || '',
            api_server: cfg.api_server || '',
            relay_server: cfg.relay_server || '',
          })
          applyServerConfigDefaults()
        }
      } catch (e) {
        if (lifecycleGuard.isCurrent('config', generation)) console.warn('fetchConfig failed:', e)
      }
    }

    onMounted(async () => {
      lifecycleGuard.mounted = true
      syncFieldAria()
      loadBuilds()
      loadPresets()
      versionsState.value = 'loading'
      // Run getVersions and fetchConfig in parallel — server defaults must apply
      // immediately even if GitHub API is slow/unreachable.
      const versionsPromise = loadVersions()
      await loadConfig()
      // Wait for version list to settle (may already be done).
      await versionsPromise
    })

    return {
      form, formRef, rules, builds, buildHistoryStatus, loading, submitting, downloadingBuildId, versionsState, versionsReady, productionPlatformReady,
      page, pageSize, total, versions,
      versionsError,
       submitBuild, deleteBuild, resetForm, downloadBuild,
       statusType, statusLabel, T,
        presets, selectedPresetId, selectedPreset, presetSelectRef, onPresetSelect, saveCurrentAsPreset, deletePreset, uploadImage,
       showPermanentPassword,
       clearSavedPresetPassword,
      requiredMessage, isRequiredField, fieldInputId, fieldErrorId, isFieldInvalid, clearFieldError, onHideConnectionManagementChange, onPlatformChange,
    }
  },
})
</script>

<style scoped lang="scss">
.custom-client :deep(.el-form-item) {
  scroll-margin-top: 96px;
}

.mb-20 {
  margin-bottom: 20px;
}

.version-hint {
  margin-left: 12px;
  font-size: 12px;
  line-height: 1.4;
  vertical-align: middle;

  &--loading { color: var(--color-primary); }
  &--empty   { color: var(--color-text); }
  &--error   { color: var(--color-danger); }
}

:deep(.build-status-tag) {
  font-weight: 600;

  &.el-tag--info {
    color: var(--color-text);
    background-color: var(--color-surface-2);
    border-color: var(--color-border);
  }

  &.el-tag--warning {
    color: var(--color-text);
    background-color: var(--color-warning-soft);
    border-color: var(--color-warning);
  }

  &.el-tag--success {
    color: var(--color-text);
    background-color: var(--color-success-soft);
    border-color: var(--color-success);
  }

  &.el-tag--danger {
    color: var(--color-text);
    background-color: var(--color-danger-soft);
    border-color: var(--color-danger);
  }
}

.preset-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
}

.preset-select {
  flex: 1 1 auto;
  min-width: 0;
}

.preset-actions {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  gap: 8px;
}

.preset-action-group {
  display: flex;
  align-items: center;
}

.preset-action-group--danger {
  margin-left: 4px;
  padding-left: 12px;
  border-left: 1px solid var(--color-border);
}

.preset-actions .el-button + .el-button {
  margin-left: 0;
}

.secret-toggle {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 8px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;

  &:focus-visible {
    outline: 2px solid var(--el-color-primary);
    outline-offset: 2px;
  }
}

.clear-saved-password {
  margin-left: 0;
}

.endpoint-hint-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: help;
  font: inherit;

  &:focus-visible {
    outline: 2px solid var(--el-color-primary);
    outline-offset: 2px;
  }
}

.build-history {
  position: relative;

  :deep(.el-pagination) {
    justify-content: flex-end;
    margin-top: 16px;
  }
}

.build-status-trigger {
  display: inline-flex;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  cursor: pointer;
  font: inherit;

  &:focus-visible {
    outline: 2px solid var(--el-color-primary);
    outline-offset: 2px;
  }
}

:global(.build-log-popover) {
  max-width: calc(100vw - 24px);
}

.build-history-status {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 900px) {
  .custom-client :deep(.el-row) {
    margin-right: 0 !important;
    margin-left: 0 !important;
  }

  .custom-client :deep(.el-col) {
    max-width: 100%;
    flex: 0 0 100%;
    padding-right: 0;
    padding-left: 0;
  }

  .custom-client :deep(.el-form-item__label) {
    width: 100% !important;
    text-align: left;
  }

  .custom-client :deep(.el-form-item__content) {
    width: 100%;
    margin-left: 0 !important;
    min-width: 0;
  }

  .preset-controls {
    align-items: stretch;
    flex-direction: column;
  }

  .preset-select,
  .preset-actions {
    width: 100%;
  }

  .preset-actions .el-button {
    margin-left: 0;
  }

  .preset-action-group--danger {
    margin-top: 4px;
    margin-left: 0;
    padding-top: 8px;
    padding-left: 0;
    border-top: 1px solid var(--color-border);
    border-left: 0;
  }

  .build-history :deep(.el-pagination) {
    justify-content: flex-start;
    max-width: 100%;
    overflow-x: auto;
    flex-wrap: wrap;
    row-gap: 8px;
  }
}
</style>
