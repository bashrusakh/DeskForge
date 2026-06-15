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
      <data-table
          :data="listRes.list"
          :loading="listRes.loading"
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
            { label: T('Actions'), align: 'center', width: 360, fixed: 'right', slot: 'actions' }
          ]"
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
          <el-space wrap>
            <el-button @click="toTag(row)">{{ T('UserTags') }}</el-button>
            <el-button @click="toAddressBook(row)">{{ T('UserAddressBook') }}</el-button>
            <el-dropdown trigger="click">
              <el-button>
                {{ T('More') }}<el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item @click="toEdit(row)">{{ T('Edit') }}</el-dropdown-item>
                  <el-dropdown-item @click="changePass(row)">{{ T('ResetPassword') }}</el-dropdown-item>
                  <el-dropdown-item divided @click="remove(row)">{{ T('Delete') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </el-space>
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
  import { ElMessageBox, ElMessage } from 'element-plus'
  import { onMounted, watch } from 'vue'
  import { ArrowDown } from '@element-plus/icons-vue'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  //列表
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

  //删除
  const { del } = useDel()
  const remove = async (row) => {
    const res = await del(row.id)
    if (res) {
      getList(listQuery)
    }
  }

  const changeStatus = async (row) => {
    /*const confirm = await ElMessageBox.confirm(T('Confirm?', { param: T('Update') }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
    }).catch(_ => false)
    if (!confirm) {
      return false
    }*/
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
