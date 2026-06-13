<template>
  <div class="monitoring-page">
    <page-header
        :title="T('ConnectionHistory')"
        subtitle="Trace remote sessions by device, source peer, IP, connection type, and close time."
        eyebrow="Monitoring"
        pulse="warning"
    />
    <filter-bar
        :title="T('Filters')"
        :subtitle="T('Filter connection events before export or cleanup.')"
        :fields="filterFields"
        :filters="listQuery"
        @filter="handlerQuery"
    >
      <template #actions>
        <el-button type="danger" @click="toBatchDelete">{{ T('BatchDelete') }}</el-button>
        <el-button type="success" @click="toExport">{{ T('Export') }}</el-button>
      </template>
    </filter-bar>
    <page-section class="list-body" :title="T('ConnectionHistory')" :subtitle="`${listRes.total} records`">
      <el-table :data="listRes.list" v-loading="listRes.loading" border @selection-change="handleSelectionChange">
        <el-table-column type="selection" align="center" width="50"/>
        <el-table-column prop="id" label="ID" align="center" width="100"/>
        <el-table-column :label="T('Peer')" prop="peer_id" align="center" width="120"/>
        <el-table-column :label="T('FromPeer')" prop="from_peer" align="center" width="120"/>
        <el-table-column :label="T('FromName')" prop="from_name" align="center" width="120"/>
        <el-table-column :label="T('Ip')" prop="ip" align="center" width="120"/>
        <el-table-column pop="type" :label="T('Type')" align="center" width="120">
          <template #default="{row}">
            <el-tag v-if="row.type === 1" type="warning">{{ T('File') }}</el-tag>
            <el-tag v-else>{{ T('Common') }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="uuid" label="uuid" align="center" width="120" show-overflow-tooltip/>
        <el-table-column prop="created_at" :label="T('CreatedAt')" align="center"/>
        <el-table-column :label="T('CloseTime')" prop="close_time" align="center"/>
        <el-table-column :label="T('Actions')" align="center" width="150">
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
import { onActivated, onMounted, ref, watch } from 'vue'
import { useRepositories } from '@/views/audit/reponsitories'
import { T } from '@/utils/i18n'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageSection from '@/components/ui/PageSection.vue'
import FilterBar from '@/components/ui/FilterBar.vue'

const {
  listRes,
  listQuery,
  getList,
  handlerQuery,
  del,
  batchdel,
  toExport,
} = useRepositories()

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
    key: 'peer_id',
    label: 'Peer',
    component: 'el-input',
    clearable: true,
    placeholder: 'Peer ID',
  },
  {
    key: 'from_peer',
    label: 'From Peer',
    component: 'el-input',
    clearable: true,
    placeholder: 'From Peer ID',
  },
]
</script>

<style scoped lang="scss">

</style>
