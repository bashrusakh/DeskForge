<template>
  <div class="access-page">
    <page-header
        :title="T('AddressBook')"
        subtitle="Organize address book collections and review sharing rules from one place."
        eyebrow="Access"
        pulse="online"
    />
    <page-section class="list-query" title="Filters" subtitle="Filter collections by owner before opening share rules.">
      <el-form inline label-width="80px">
        <el-form-item :label="T('Owner')">
          <el-select v-model="listQuery.user_id" clearable>
            <el-option
                v-for="item in allUsers"
                :key="item.id"
                :label="item.username"
                :value="item.id"
            ></el-option>
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" :title="T('AddressBook')" :subtitle="`${listRes.total} collections`">
      <data-table
          :data="listRes.list"
          :loading="listRes.loading"
          row-key="id"
          :columns="[
            { prop: 'id', label: 'ID', align: 'center', width: 100 },
            { label: T('Owner'), align: 'center', slot: 'owner' },
            { prop: 'name', label: T('AddressBook'), align: 'center' },
            { prop: 'created_at', label: T('CreatedAt'), align: 'center' },
            { label: T('Actions'), align: 'center', className: 'table-actions', width: 320, fixed: 'right', slot: 'actions' }
          ]"
      >
        <template #owner="{ row }">
          <span v-if="row.user_id"> <el-tag>{{ allUsers?.find(u => u.id === row.user_id)?.username }}</el-tag> </span>
        </template>
        <template #actions="{ row }">
          <el-space wrap>
            <el-button type="primary" @click="showRules(row)">{{ T('ShareRules') }}</el-button>
            <el-dropdown trigger="click">
              <el-button>
                {{ T('More') }}<el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item @click="toEdit(row)">{{ T('Edit') }}</el-dropdown-item>
                  <el-dropdown-item divided @click="del(row)">{{ T('Delete') }}</el-dropdown-item>
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
    <app-dialog
        v-model="formVisible"
        :title="!formData.id ? T('Create') : T('Update')"
        width="800"
        @confirm="submit"
    >
      <el-form class="dialog-form" ref="form" :model="formData" label-width="120px">
        <el-form-item :label="T('Owner')" prop="user_id" required>
          <el-select v-model="formData.user_id">
            <el-option
                v-for="item in allUsers"
                :key="item.id"
                :label="item.username"
                :value="item.id"
            ></el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="T('Name')" prop="name" required>
          <el-input v-model="formData.name"></el-input>
        </el-form-item>
      </el-form>
    </app-dialog>
    <app-dialog
        v-model="rulesVisible"
        :title="T('ShareRules')"
        width="80%"
        destroy-on-close
        :hide-footer="true"
    >
      <Rule :collection="clickRow" :is_my="0"></Rule>
    </app-dialog>

  </div>
</template>

<script setup>
  import { T } from '@/utils/i18n'
  import { ref } from 'vue'
  import { useRepositories } from '@/views/address_book/collection'
  import { onActivated, onMounted, watch } from 'vue'
  import Rule from '@/views/address_book/rule.vue'
  import { loadAllUsers } from '@/global'
  import { ArrowDown } from '@element-plus/icons-vue'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const { allUsers, getAllUsers } = loadAllUsers()
  getAllUsers()
  const {
    listRes,
    listQuery,
    getList,
    handlerQuery,
    del,
    formVisible,
    formData,
    toEdit,
    toAdd,
    submit,
  } = useRepositories('admin')

  listQuery.is_my = 0

  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)

  const clickRow = ref({})
  const rulesVisible = ref(false)
  const showRules = (row) => {
    clickRow.value = row
    rulesVisible.value = true
    console.log('showRules')
  }

</script>

<style scoped lang="scss">
.access-page {
  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
