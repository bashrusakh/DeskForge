<template>
  <div class="monitoring-page">
    <page-header
        :title="T('LoginHistory')"
        subtitle="Audit sign-ins by user, client, device, IP address, platform, and time."
        eyebrow="Monitoring"
        pulse="warning"
    />
    <filter-bar
        :title="T('Filters')"
        :subtitle="T('Narrow login events before exporting or deleting records.')"
        :fields="filterFields"
        :filters="listQuery"
        @filter="handlerQuery"
    >
      <template #actions>
        <el-button type="danger" @click="toBatchDelete">{{ T('BatchDelete') }}</el-button>
        <el-button type="success" @click="toExport">{{ T('Export') }}</el-button>
      </template>
    </filter-bar>
    <page-section class="list-body" title="Login events" :subtitle="`${listRes.total} records`">
      <el-table :data="listRes.list" v-loading="listRes.loading" border @selection-change="handleSelectionChange">
        <el-table-column type="selection" align="center" width="50"/>
        <el-table-column prop="id" label="ID" align="center" width="100"/>
        <el-table-column :label="T('Owner')" align="center" width="120">
          <template #default="{row}">
            <span v-if="row.user_id"> <el-tag>{{ allUsers?.find(u => u.id === row.user_id)?.username }}</el-tag> </span>
          </template>
        </el-table-column>
        <el-table-column prop="client" label="client" align="center" width="120"/>
        <el-table-column prop="peer.id" :label="T('Peer')" align="center">
          <template #default="{row}">
            {{ row.device_id ? row.device_id : peer?.id }}
          </template>
        </el-table-column>
        <el-table-column prop="uuid" label="uuid" align="center"/>
        <el-table-column prop="ip" label="ip" align="center" width="150"/>
        <el-table-column prop="type" label="type" align="center" width="100"/>
        <el-table-column prop="platform" label="Platform/UA" align="center" width="120" show-overflow-tooltip/>
        <el-table-column prop="created_at" :label="T('CreatedAt')" align="center"/>
        <el-table-column :label="T('Actions')" align="center" width="400">
          <template #default="{row}">
            <el-button type="danger" @click="del(row)">{{ T('Delete') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
    </page-section>
    <page-section class="list-page">
      <el-pagination background
                     layout="prev, pager, next, sizes, jumper"
                     :page-sizes="[10,20,50,100]"
                     v-model:page-size="listQuery.page_size"
                     v-model:current-page="listQuery.page"
                     :total="listRes.total">
      </el-pagination>
    </page-section>
  </div>
</template>

<script setup>
  import { onActivated, onMounted, ref, watch, reactive, computed } from 'vue'
  import { loadAllUsers } from '@/global'
  import { useRepositories } from '@/views/login/log.js'
  import { T } from '@/utils/i18n'
  import { list } from '@/api/peer'
  import { downBlob, jsonToCsv } from '@/utils/file'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import FilterBar from '@/components/ui/FilterBar.vue'

  const { allUsers, getAllUsers } = loadAllUsers()
  getAllUsers()

  const {
    listRes,
    listQuery,
    getList,
    handlerQuery,
    del,
    batchdel,
    toExport,
  } = useRepositories('admin')

  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)
  const multipleSelection = ref([])
  const handleSelectionChange = (val) => {
    multipleSelection.value = val
  }
  const toBatchDelete = () => {
    if (multipleSelection.value.length === 0) {
      return
    }
    batchdel(multipleSelection.value)
  }

  const filterFields = [
    {
      key: 'user_id',
      label: 'User',
      component: 'el-select',
      clearable: true,
      get options() {
        return allUsers.value.map(u => ({
          label: u.username,
          value: u.id
        }))
      }
    },
  ]

</script>

<style scoped lang="scss">
.list-query .el-select {
  --el-select-width: 160px;
}


</style>
