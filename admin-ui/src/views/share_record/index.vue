<template>
  <div class="monitoring-page">
    <page-header
        :title="T('SharedSessions')"
        subtitle="Review shared web-client sessions, owners, peer IDs, creation time, and expiration state."
        eyebrow="Monitoring"
        pulse="warning"
    />
    <filter-bar
        :title="T('Filters')"
        :subtitle="T('Filter shared sessions before cleanup.')"
        :fields="filterFields"
        :filters="listQuery"
        @filter="handlerQuery"
    >
      <template #actions>
        <el-button type="danger" @click="toBatchDelete">{{ T('BatchDelete') }}</el-button>
      </template>
    </filter-bar>
    <page-section class="list-body" :title="T('SharedSessions')" :subtitle="`${listRes.total} records`">
      <el-table :data="listRes.list" v-loading="listRes.loading" border @selection-change="handleSelectionChange">
        <el-table-column type="selection" align="center" width="50"/>
        <el-table-column prop="id" label="ID" align="center" width="100"/>
        <el-table-column :label="T('User')" align="center" width="120">
          <template #default="{row}">
            <span v-if="row.user_id"> <el-tag>{{ allUsers?.find(u => u.id === row.user_id)?.username }}</el-tag> </span>
          </template>
        </el-table-column>
        <el-table-column prop="peer_id" :label="T('Peer')" align="center"/>
        <el-table-column prop="created_at" :label="T('CreatedAt')" align="center"/>
        <el-table-column :label="`${T('ExpireTime')} (${T('Second')})`" prop="expire" align="center">
          <template #default="{row}">
            <el-tag :type="expired(row)?'info':'success'">{{ row.expire ? row.expire : T('Forever') }}</el-tag>
          </template>
        </el-table-column>
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
  import { onActivated, onMounted, ref, watch, reactive } from 'vue'
  import { loadAllUsers } from '@/global'
  import { T } from '@/utils/i18n'
  import { remove, list, batchDelete } from '@/api/share_record'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { useRepositories } from '@/views/share_record/index'
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
    multipleSelection,
    toBatchDelete,
    expired,
  } = useRepositories('admin')

  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)
  const handleSelectionChange = (val) => {
    multipleSelection.value = val
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
