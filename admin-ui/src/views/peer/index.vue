<template>
  <div class="devices-page">
    <el-card class="list-query device-filter-card" shadow="hover">
      <el-form inline label-width="60px">
        <el-form-item label="ID">
          <el-input v-model="listQuery.id" clearable/>
        </el-form-item>
        <el-form-item :label="T('Hostname')">
          <el-input v-model="listQuery.hostname" clearable/>
        </el-form-item>
        <el-form-item label="Online" label-width="70px">
          <el-select v-model="listQuery.time_ago" clearable>
            <el-option
                v-for="item in timeFilters"
                :key="item.value"
                :label="item.text"
                :value="item.value"
                :disabled="item.value === 0"
            ></el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="T('Username')">
          <el-input v-model="listQuery.username" clearable/>
        </el-form-item>
        <el-form-item label="IP">
          <el-input v-model="listQuery.ip" clearable/>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
          <el-button type="success" @click="toExport">{{ T('Export') }}</el-button>
          <el-popover :visible="showImport" placement="bottom" :width="600">
            <el-upload
                class="upload-demo"
                drag
                accept=".csv"
                :before-upload="parseCsv"
            >
              <el-icon class="el-icon--upload">
                <upload-filled/>
              </el-icon>
              <div class="el-upload__text">
                {{ T('Drop file here or click to upload') }}
              </div>
              <template #tip>
                <div class="el-upload__tip">
                  {{ T('Please upload csv file') }} <br>
                  {{ T('Columns') }}: <span style="font-weight: bold;font-size: 15px">id,cpu,hostname,memory,os,username,uuid,version,group_id</span>
                  <br>
                  <span>{{ T('You can reference export file') }}</span>
                </div>
              </template>
            </el-upload>
            <el-button @click="showImport=false" type="primary">{{ T('Cancel') }}</el-button>
            <template #reference>
              <el-button @click="showImport=true" type="danger" :icon="ArrowDown">{{ T('Import') }}</el-button>
            </template>
          </el-popover>
        </el-form-item>
      </el-form>
    </el-card>
    <el-card class="list-body device-table-card" shadow="hover">
      <div class="device-table-toolbar">
        <div>
          <div class="device-table-title">{{ T('AllDevices') }}</div>
          <div class="device-table-subtitle">Device status, identity, ownership, and remote access actions.</div>
        </div>
        <el-button :icon="Setting" @click="showColumnSetting">Columns</el-button>
      </div>
      <actions-toolbar :selected="multipleSelection">
        <template #default="{ disabled, selected }">
          <template v-if="selected.length === 1">
            <el-button type="success" @click="connectByClient(selected[0].id)">Connect</el-button>
            <el-button v-if="appStore.setting.appConfig.web_client" @click="toWebClientLink(selected[0])">Web Client</el-button>
            <el-button @click="toAddressBook(selected[0])">{{ T('AddToAddressBook') }}</el-button>
            <el-button type="primary" @click="toEdit(selected[0])">{{ T('Edit') }}</el-button>
          </template>
          <el-button type="primary" :disabled="disabled" @click="toBatchAddToAB">
            {{ T('BatchAddToAB') }} ({{ selected.length }})
          </el-button>
          <el-button type="danger" :disabled="disabled" @click="toBatchDelete">
            {{ T('DeleteSelected') }} ({{ selected.length }})
          </el-button>
        </template>
      </actions-toolbar>

      <data-table
          :data="listRes.list"
          :loading="listRes.loading"
          border
          size="small"
          selectable
          @selection-change="handleSelectionChange"
          row-key="id"
          :columns="tableColumns"
      >
        <template #status="{ row }">
          <div class="device-status" :class="{ 'is-online': isOnline(row), 'is-offline': !isOnline(row) }">
            <connection-pulse :status="isOnline(row) ? 'online' : 'offline'" :animated="isOnline(row)" />
            <span>{{ isOnline(row) ? 'Online' : 'Offline' }}</span>
          </div>
        </template>
        <template #id="{ row }">
          <copyable-text :text="row.id" />
        </template>
        <template #lastOnlineTime="{ row }">
          <div class="last_oline_time">
            <span> {{ row.last_online_time ? timeAgo(row.last_online_time * 1000) : '-' }}</span>
          </div>
        </template>
        <template #group="{ row }">
          <span v-if="row.group_id"> <el-tag>{{ groupListRes.list?.find(g => g.id === row.group_id)?.name }} </el-tag> </span>
          <span v-else> - </span>
        </template>
      </data-table>
    </el-card>
    <page-section class="list-page">
      <el-pagination background
                     layout="prev, pager, next, sizes, jumper"
                     :page-sizes="[10,20,50,100]"
                     v-model:page-size="listQuery.page_size"
                     v-model:current-page="listQuery.page"
                     :total="listRes.total">
      </el-pagination>
    </page-section>
    <app-dialog
        v-model="formVisible"
        :title="!formData.row_id ? T('Create') : T('Update')"
        width="800"
        @confirm="submit"
    >
      <el-form class="dialog-form" ref="form" :model="formData" label-width="120px">
        <el-form-item label="ID" prop="id" required>
          <el-input v-model="formData.id"></el-input>
        </el-form-item>
        <el-form-item :label="T('Group')" prop="group_id">
          <el-select v-model="formData.group_id">
            <el-option
                v-for="item in groupListRes.list"
                :key="item.id"
                :label="item.name"
                :value="item.id"
            ></el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="T('Username')" prop="username">
          <el-input v-model="formData.username"></el-input>
        </el-form-item>
        <el-form-item :label="T('Hostname')" prop="hostname">
          <el-input v-model="formData.hostname"></el-input>
        </el-form-item>
        <el-form-item label="CPU" prop="cpu">
          <el-input v-model="formData.cpu"></el-input>
        </el-form-item>
        <el-form-item :label="T('Memory')" prop="memory">
          <el-input v-model="formData.memory"></el-input>
        </el-form-item>
        <el-form-item :label="T('Os')" prop="os">
          <el-input v-model="formData.os"></el-input>
        </el-form-item>
        <el-form-item :label="T('Uuid')" prop="uuid">
          <el-input v-model="formData.uuid"></el-input>
        </el-form-item>
        <el-form-item :label="T('Version')" prop="version">
          <el-input v-model="formData.version"></el-input>
        </el-form-item>
        <el-form-item :label="T('Alias')" prop="alias">
          <el-input v-model="formData.alias"></el-input>
        </el-form-item>
      </el-form>
    </app-dialog>

    <app-dialog
        v-model="ABFormVisible"
        :title="T('Create')"
        width="800"
        destroy-on-close
        :show-confirm="false"
        :hide-footer="true"
    >
      <createABForm :peer="clickRow" @success="ABFormVisible=false" @cancel="ABFormVisible=false"></createABForm>
    </app-dialog>

    <app-dialog
        v-model="batchABFormVisible"
        :title="T('Create')"
        width="800"
        @confirm="submitBatchAddToAB"
    >
      <el-form class="dialog-form" ref="form" :model="batchABFormData" label-width="120px">
        <el-form-item :label="T('Owner')" prop="user_id" required>
          <el-select v-model="batchABFormData.user_id" @change="changeUserForBatchCreateAB">
            <el-option
                v-for="item in allUsers"
                :key="item.id"
                :label="item.username"
                :value="item.id"
            ></el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="T('Name')" required prop="collection_id">
          <el-select v-model="batchABFormData.collection_id" clearable>
            <el-option :value="0" :label="T('MyAddressBook')"></el-option>
            <el-option v-for="c in collectionListResForBatchCreateAB.list" :key="c.id" :label="c.name" :value="c.id"></el-option>
          </el-select>
        </el-form-item>
      </el-form>
    </app-dialog>

    <app-dialog
        v-model="columnSettingVisible"
        :title="T('ColumnSetting')"
        @confirm="saveColumnSetting"
    >
      <div v-for="(row, key) in visibleColumns" :key="key" style="margin-bottom: 10px;display: flex;align-items: center">
        <div style="width: 200px">
          <el-checkbox v-model="row.visible" :label="true">{{ T(row.label) }}</el-checkbox>
        </div>
        <div @click="upColumn(key)" style="width: 100px;cursor: pointer">
          <el-icon :size="20">
            <ArrowUp/>
          </el-icon>
        </div>
        <div @click="downColumn(key)" style="width: 100px;cursor: pointer">
          <el-icon :size="20">
            <ArrowDown/>
          </el-icon>
        </div>
      </div>
    </app-dialog>
  </div>
</template>

<script setup>
  import { computed, onActivated, onMounted, reactive, ref, watch } from 'vue'
  import { batchRemove, create, list, remove, update } from '@/api/peer'
  import { list as groupList } from '@/api/device_group'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { toWebClientLink } from '@/utils/webclient'
  import { T } from '@/utils/i18n'
  import { timeAgo } from '@/utils/time'
  import { jsonToCsv, downBlob } from '@/utils/file'
  import { loadAllUsers } from '@/global'
  import { useAppStore } from '@/store/app'
  import { connectByClient } from '@/utils/peer'
  import { ArrowUp, Setting } from '@element-plus/icons-vue'
  import { batchCreateFromPeers } from '@/api/address_book'
  import { useRepositories as useCollectionRepositories } from '@/views/address_book/collection'
  import createABForm from '@/views/peer/createABForm.vue'
  import { UploadFilled } from '@element-plus/icons-vue'
  import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'
  import CopyableText from '@/components/ui/CopyableText.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import ActionsToolbar from '@/components/ui/ActionsToolbar.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const appStore = useAppStore()

  //group
  const groupListRes = reactive({
    list: [], total: 0, loading: false,
  })
  const groupListQuery = reactive({
    page: 1,
    page_size: 999,
  })
  const getGroupList = async () => {
    groupListRes.loading = true
    const res = await groupList(groupListQuery).catch(_ => false)
    groupListRes.loading = false
    if (res) {
      groupListRes.list = res.data.list
      groupListRes.total = res.data.total
    }
  }
  onMounted(getGroupList)
  //

  const listRes = reactive({
    list: [], total: 0, loading: false,
  })
  const listQuery = reactive({
    page: 1,
    page_size: 10,
    time_ago: null,
    id: '',
    hostname: '',
    username: '',
    ip: '',
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

  const del = async (row) => {
    const cf = await ElMessageBox.confirm(T('Confirm?', { param: T('Delete') }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
      type: 'warning',
    }).catch(_ => false)
    if (!cf) {
      return false
    }

    const res = await remove({ row_id: row.row_id }).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      getList()
    }
  }
  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)

  const formVisible = ref(false)
  const formData = reactive({
    row_id: 0,
    group_id: null,
    cpu: '',
    hostname: '',
    id: '',
    memory: '',
    os: '',
    username: '',
    uuid: '',
    version: '',
  })

  const toEdit = (row) => {
    formVisible.value = true
    //将row中的数据赋值给formData
    Object.keys(formData).forEach(key => {
      formData[key] = row[key]
    })
  }
  const toAdd = () => {
    formVisible.value = true
    //重置formData
    formData.row_id = 0
    formData.cpu = ''
    formData.hostname = ''
    formData.id = ''
    formData.memory = ''
    formData.os = ''
    formData.username = ''
    formData.uuid = ''
    formData.version = ''
  }
  const submit = async () => {
    const api = formData.row_id ? update : create
    const res = await api(formData).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      formVisible.value = false
      getList()
    }
  }

  const timeDis = (time) => {
    if (!time) return Number.POSITIVE_INFINITY
    let now = new Date().getTime()
    let after = new Date(time * 1000).getTime()
    return (now - after) / 1000
  }
  const isOnline = (row) => timeDis(row.last_online_time) < 60

  const timeFilters = computed(() => [
    { text: T('MinutesLess', { param: 1 }, 1), value: -60 },
    { text: T('HoursLess', { param: 1 }, 1), value: -3600 },
    { text: T('DaysLess', { param: 1 }, 1), value: -86400 },
    { text: '---------', value: 0 },
    { text: T('MinutesAgo', { param: 1 }, 1), value: 60 },
    { text: T('HoursAgo', { param: 1 }, 1), value: 3600 },
    { text: T('DaysAgo', { param: 1 }, 1), value: 86400 },
    { text: T('MonthsAgo', { param: 1 }, 1), value: 2592000 },
    // { text: T('YearsAgo', { param: 1 }, 1), value: 31536000 },
  ])

  const toExport = async () => {
    const q = { ...listQuery }
    q.page_size = 1000000
    q.page = 1
    const res = await list(q).catch(_ => false)
    if (res) {
      const data = res.data.list.map(item => {
        item.last_online_time = item.last_online_time ? new Date(item.last_online_time * 1000).toLocaleString() : '-'
        delete item.user_id
        delete item.user
        return item
      })
      const csv = jsonToCsv(data)
      downBlob(csv, 'peers.csv')
    }
  }

  const showImport = ref(false)
  const canKeys = ['id', 'cpu', 'hostname', 'memory', 'os', 'username', 'uuid', 'version', 'group_id']
  const parseCsv = (file) => {
    const reader = new FileReader()
    reader.onload = async (e) => {
      const data = e.target.result
      const rows = data.split('\n')
      // strip BOM from the first column header, otherwise UTF-8 BOM
      // files would always fail the missing-columns check below.
      const header = rows[0].replace(/^﻿/, '')
      const keys = header.split(',').map(k => k.trim().replace(/^"|"$/g, ''))
      const missing = canKeys.filter(k => k !== 'group_id' && !keys.includes(k))
      if (missing.length) {
        ElMessage.error(`${T('Import')}: missing columns: ${missing.join(', ')}`)
        return
      }
      const values = rows.slice(1).map(row => {
        const cols = row.split(/,(?=(?:(?:[^"]*"){2})*[^"]*$)/)
        const obj = {}
        keys.forEach((k, i) => {
          obj[k] = (cols[i] || '').trim().replace(/^"|"$/g, '')
        })
        return obj
      }).filter(item => item.id)
      values.forEach(item => {
        item.group_id = parseInt(item.group_id) || 0
        Object.keys(item).forEach(key => {
          if (!canKeys.includes(key)) {
            delete item[key]
          }
        })
      })
      const results = await Promise.allSettled(values.map(item => create(item)))
      const ok = results.filter(r => r.status === 'fulfilled').length
      const fail = results.filter(r => r.status === 'rejected').length
      if (fail === 0) {
        ElMessage.success(T('OperationSuccess'))
      } else if (ok > 0) {
        ElMessage.warning(`${T('Import')}: ${ok} ${T('Success')}, ${fail} ${T('Failed')}`)
      } else {
        ElMessage.error(T('OperationFailed'))
      }
      getList()

    }
    reader.readAsText(file)
    return false
  }

  const ABFormVisible = ref(false)
  const clickRow = ref({})
  const toAddressBook = (row) => {
    clickRow.value = row
    ABFormVisible.value = true
  }

  const multipleSelection = ref([])
  const handleSelectionChange = (val) => {
    multipleSelection.value = val
  }
  const toBatchDelete = async () => {
    if (!multipleSelection.value.length) {
      ElMessage.warning(T('PleaseSelectData'))
      return false
    }
    const cf = await ElMessageBox.confirm(T('Confirm?', { param: T('BatchDelete') }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
      type: 'warning',
    }).catch(_ => false)
    if (!cf) {
      return false
    }

    const res = await batchRemove({ row_ids: multipleSelection.value.map(i => i.row_id) }).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      multipleSelection.value = []
      getList()
    }
  }

  // 批量添加到地址簿 start
  const { allUsers, getAllUsers } = loadAllUsers()
  onMounted(getAllUsers)
  const {
    listRes: collectionListResForBatchCreateAB,
    listQuery: collectionListQueryForBatchCreateAB,
    getList: getCollectionListForBatchCreateAB,
  } = useCollectionRepositories('admin')
  collectionListQueryForBatchCreateAB.page_size = 9999
  const changeUserForBatchCreateAB = (val) => {
    batchABFormData.value.collection_id = 0
    collectionListQueryForBatchCreateAB.user_id = val
    getCollectionListForBatchCreateAB()
  }
  const batchABFormVisible = ref(false)
  const toBatchAddToAB = () => {
    batchABFormVisible.value = true
  }
  const batchABFormData = ref({
    collection_id: 0,
    tags: [],
    peer_ids: [],
    user_id: null,
  })
  const submitBatchAddToAB = async () => {
    if (multipleSelection.value.length === 0) {
      ElMessage.warning(T('PleaseSelectData'))
      return false
    }
    batchABFormData.value.peer_ids = multipleSelection.value.map(i => i.row_id)
    if (!batchABFormData.value.peer_ids.length) {
      ElMessage.warning(T('PleaseSelectData'))
      return false
    }

    const res = await batchCreateFromPeers(batchABFormData.value).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      batchABFormVisible.value = false
    }
  }
  // 批量添加到地址簿 end

  const columnSettingVisible = ref(false)
  const allColumns = ref([
    { name: 'id', visible: true, label: 'Id' },
    { name: 'cpu', visible: true, label: 'Cpu' },
    { name: 'hostname', visible: true, label: 'Hostname' },
    { name: 'memory', visible: true, label: 'Memory' },
    { name: 'os', visible: true, label: 'Os' },
    { name: 'last_online_time', visible: true, label: 'LastOnlineTime' },
    { name: 'last_online_ip', visible: true, label: 'LastOnlineIp' },
    { name: 'username', visible: true, label: 'Username' },
    { name: 'group_id', visible: true, label: 'Group' },
    { name: 'uuid', visible: true, label: 'Uuid' },
    { name: 'version', visible: true, label: 'Version' },
    { name: 'alias', visible: true, label: 'Alias' },
    { name: 'created_at', visible: true, label: 'CreatedAt' },
    { name: 'updated_at', visible: true, label: 'UpdatedAt' },
  ])
  const visibleColumns = ref(JSON.parse(localStorage.getItem('peer_visible_columns')) || allColumns.value)

  const columnProps = {
    id: { prop: 'id', label: 'ID', align: 'left', width: 160, slot: 'id' },
    cpu: { prop: 'cpu', label: 'CPU', align: 'center', width: 100, showOverflowTooltip: true },
    hostname: { prop: 'hostname', label: T('Hostname'), align: 'center', width: 120 },
    memory: { prop: 'memory', label: T('Memory'), align: 'center', width: 120 },
    os: { prop: 'os', label: T('Os'), align: 'center', width: 120, showOverflowTooltip: true },
    last_online_time: { label: T('LastOnlineTime'), align: 'center', minWidth: 120, slot: 'lastOnlineTime' },
    last_online_ip: { prop: 'last_online_ip', label: T('LastOnlineIp'), align: 'center', minWidth: 120 },
    username: { prop: 'username', label: T('Username'), align: 'center', width: 120 },
    group_id: { prop: 'group_id', label: T('Group'), align: 'center', width: 120, slot: 'group' },
    uuid: { prop: 'uuid', label: T('Uuid'), align: 'center', width: 120, showOverflowTooltip: true },
    version: { prop: 'version', label: T('Version'), align: 'center', width: 80 },
    alias: { prop: 'alias', label: T('Alias'), align: 'center', width: 80 },
    created_at: { prop: 'created_at', label: T('CreatedAt'), align: 'center', width: 150 },
    updated_at: { prop: 'updated_at', label: T('UpdatedAt'), align: 'center', width: 150 },
  }

  const tableColumns = computed(() => {
    const statusCol = { label: T('Status'), align: 'left', width: 120, slot: 'status' }
    const dynamicCols = visibleColumns.value
        .filter(c => c.visible)
        .map(c => columnProps[c.name] || { prop: c.name, label: c.label || c.name })
    return [statusCol, ...dynamicCols]
  })
  const showColumnSetting = () => {
    columnSettingVisible.value = true
  }
  const saveColumnSetting = () => {
    localStorage.setItem('peer_visible_columns', JSON.stringify(visibleColumns.value))
    ElMessage.success(T('OperationSuccess'))
    columnSettingVisible.value = false
  }

  const upColumn = (index) => {
    if (index === 0) return
    const col = visibleColumns.value[index]
    visibleColumns.value.splice(index, 1)
    visibleColumns.value.splice(index - 1, 0, col)

  }
  const downColumn = (index) => {
    if (index === visibleColumns.value.length - 1) return
    const col = visibleColumns.value[index]
    visibleColumns.value.splice(index, 1)
    visibleColumns.value.splice(index + 1, 0, col)

  }
</script>

<style scoped lang="scss">
.list-query .el-select {
  --el-select-width: 180px;
}

.devices-page {
  .device-filter-card,
  .device-table-card {
    border-radius: var(--radius-lg);
  }

  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}

.device-table-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.device-table-title {
  color: var(--color-text);
  font-size: 18px;
  font-weight: 700;
}

.device-table-subtitle {
  margin-top: 4px;
  color: var(--color-muted);
  font-size: 13px;
}

.last_oline_time {
  display: flex;
  justify-content: center;
  align-items: center;
}

.device-status {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 999px;
  background: var(--color-surface-2);
  color: var(--color-muted);
  font-size: 12px;
  font-weight: 700;

  &.is-online {
    background: var(--color-success-soft);
    color: var(--color-success);
  }
}

@media (max-width: 720px) {
  .device-table-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
