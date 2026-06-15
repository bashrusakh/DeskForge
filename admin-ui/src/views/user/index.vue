<template>
  <div class="security-page">
    <page-header
        title="Users"
        subtitle="Manage administrator accounts, group membership, status, and account recovery actions."
        eyebrow="Security"
        pulse="warning"
    />
    <page-section class="list-query" title="Filters" subtitle="Search users by username and export the current account inventory.">
      <el-form inline label-width="80px">
        <el-form-item :label="T('Username')">
          <el-input v-model="listQuery.username"></el-input>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
          <el-button type="success" @click="toExport">{{ T('Export') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" title="Users" :subtitle="`${listRes.total} accounts`">
      <actions-toolbar :selected="selectedRows">
        <template #default="{ disabled }">
          <el-button type="danger" :disabled="disabled" @click="bulkRemove">
            {{ T('DeleteSelected') }} ({{ selectedRows.length }})
          </el-button>
        </template>
      </actions-toolbar>
      <data-table
          :data="listRes.list"
          :loading="listRes.loading"
          selectable
          row-key="id"
          :columns="[
            { prop: 'id', label: 'ID', align: 'center', width: 100 },
            { prop: 'username', label: T('Username'), align: 'center' },
            { prop: 'email', label: T('Email'), align: 'center' },
            { prop: 'nickname', label: T('Nickname'), align: 'center' },
            { label: T('Group'), align: 'center', slot: 'group' },
            { label: T('Status'), align: 'center', slot: 'status' },
            { prop: 'remark', label: T('Remark'), align: 'center' },
            { prop: 'created_at', label: T('CreatedAt'), align: 'center' },
            { prop: 'updated_at', label: T('UpdatedAt'), align: 'center' },
            { label: '', align: 'center', width: 60, slot: 'actions' }
          ]"
          @selection-change="selectedRows = $event"
      >
        <template #group="{ row }">
          <span v-if="row.group_id"> <el-tag>{{ listRes.groups?.find(g => g.id === row.group_id)?.name }} </el-tag> </span>
          <span v-else> - </span>
        </template>
        <template #status="{ row }">
          <el-switch v-if="row && (row.status === ENABLE_STATUS || row.status === DISABLE_STATUS)"
                     v-model="row.status"
                     :active-value="ENABLE_STATUS"
                     :inactive-value="DISABLE_STATUS"
                     @change="changeStatus(row)"
          ></el-switch>
        </template>
        <template #actions="{ row }">
          <el-dropdown trigger="click" @command="(cmd) => handleRowAction(cmd, row)">
            <el-button text>
              {{ T('More') }}<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="tags">{{ T('UserTags') }}</el-dropdown-item>
                <el-dropdown-item command="addressBook">{{ T('UserAddressBook') }}</el-dropdown-item>
                <el-dropdown-item divided command="edit">{{ T('Edit') }}</el-dropdown-item>
                <el-dropdown-item command="resetPassword">{{ T('ResetPassword') }}</el-dropdown-item>
                <el-dropdown-item divided command="delete">{{ T('Delete') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </data-table>
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
  import { useRepositories, useDel, useToEditOrAdd, useChangePwd } from '@/views/user/composables'
  import { T } from '@/utils/i18n'
  import { DISABLE_STATUS, ENABLE_STATUS } from '@/utils/common_options'
  import { update } from '@/api/user'
  import { ElMessage } from 'element-plus'
  import { onMounted, ref, watch } from 'vue'
  import { ArrowDown } from '@element-plus/icons-vue'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import ActionsToolbar from '@/components/ui/ActionsToolbar.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import { useBulkRemove } from '@/composables/useBulkRemove'
  import { remove as apiRemove } from '@/api/user'

  const {
    listRes,
    listQuery,
    handlerQuery,
    getList,
    getGroups,
    toExport,
  } = useRepositories()

  onMounted(getGroups)

  onMounted(getList)

  watch(() => listQuery.page, getList)
  watch(() => listQuery.page_size, handlerQuery)

  const { toEdit, toAdd, toAddressBook, toTag } = useToEditOrAdd()

  const { changePass } = useChangePwd()

  const { del } = useDel()

  const { selectedRows, bulkRemove } = useBulkRemove({
    removeApi: apiRemove,
    getList: () => getList(listQuery),
    label: T('User'),
  })

  const remove = async (row) => {
    const res = await del(row.id)
    if (res) {
      selectedRows.value = selectedRows.value.filter(r => r.id !== row.id)
      getList(listQuery)
    }
  }

  const handleRowAction = (cmd, row) => {
    switch (cmd) {
      case 'tags': toTag(row); break
      case 'addressBook': toAddressBook(row); break
      case 'edit': toEdit(row); break
      case 'resetPassword': changePass(row); break
      case 'delete': remove(row); break
    }
  }

  const changeStatus = async (row) => {
    const res = await update(row).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      getList(listQuery)
    }
  }

</script>

<style scoped lang="scss">
.security-page {
  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
