<template>
  <div class="workspace-page">
    <page-header
        title="My Login History"
        subtitle="Review login events linked to your account and clean up old records."
        eyebrow="Workspace"
        pulse="warning"
    />
    <page-section class="list-query" title="History controls" subtitle="Refresh or batch-delete selected personal login records.">
      <el-form inline label-width="80px">
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toBatchDelete">{{ T('BatchDelete') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" title="My Login History" :subtitle="`${listRes.total} records`">
      <el-table :data="listRes.list" v-loading="listRes.loading" border @selection-change="handleSelectionChange">
        <el-table-column type="selection" align="center" width="50"/>
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
        <el-table-column :label="T('Actions')" align="center" width="180" fixed="right">
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
  import { useRepositories } from '@/views/login/log.js'
  import { T } from '@/utils/i18n'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'

  const {
    listRes,
    listQuery,
    getList,
    handlerQuery,
    del,
    batchdel,
  } = useRepositories('my')

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
</script>

<style scoped lang="scss">
.list-query .el-select {
  --el-select-width: 160px;
}

.workspace-page {
  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}


</style>
