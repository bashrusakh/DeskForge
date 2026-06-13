<template>
  <div class="workspace-page">
    <page-header
        title="My Shared Sessions"
        subtitle="Review and revoke personal web-client sharing links."
        eyebrow="Workspace"
        pulse="warning"
    />
    <page-section class="list-query" title="Session controls" subtitle="Refresh or batch-delete selected personal share records.">
      <el-form inline label-width="80px">
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toBatchDelete">{{ T('BatchDelete') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" title="My Shared Sessions" :subtitle="`${listRes.total} records`">
      <el-table :data="listRes.list" v-loading="listRes.loading" border @selection-change="handleSelectionChange">
        <el-table-column type="selection" align="center" width="50"/>
        <el-table-column prop="id" label="ID" align="center" width="100"/>
        <el-table-column prop="peer_id" :label="T('Peer')" align="center"/>
        <el-table-column prop="created_at" :label="T('CreatedAt')" align="center"/>
        <el-table-column :label="`${T('ExpireTime')} (${T('Second')})`" prop="expire" align="center">
          <template #default="{row}">
            <el-tag :type="expired(row)?'info':'success'">{{ row.expire ? row.expire : T('Forever') }}</el-tag>
          </template>
        </el-table-column>
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
  import { onActivated, onMounted, watch } from 'vue'
  import { T } from '@/utils/i18n'
  import { useRepositories } from '@/views/share_record'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'

  const {
    listRes,
    listQuery,
    getList,
    handlerQuery,
    del,
    multipleSelection,
    toBatchDelete,
    expired,
  } = useRepositories('my')

  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)
  const handleSelectionChange = (val) => {
    multipleSelection.value = val
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
