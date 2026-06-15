<template>
  <div class="workspace-page">
    <page-header
        title="My Address Book Collections"
        subtitle="Create personal collections and manage sharing rules for your saved devices."
        eyebrow="Workspace"
        pulse="online"
    />
    <page-section class="list-query" title="Collection controls" subtitle="Refresh the list or create a new personal collection.">
      <el-form inline label-width="80px">
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" title="My Address Book Collections" :subtitle="`${listRes.total} collections`">
      <el-tag type="danger" effect="light" style="margin-bottom: 10px">{{ T('MyAddressBookTips') }}</el-tag>
      <actions-toolbar :selected="selectedRows">
        <template #default="{ disabled }">
          <el-button type="danger" :disabled="disabled" @click="bulkDel">
            {{ T('DeleteSelected') }} ({{ selectedRows.length }})
          </el-button>
        </template>
      </actions-toolbar>
      <data-table
          :data="list"
          :loading="listRes.loading"
          selectable
          row-key="id"
          :columns="[
            { prop: 'name', label: T('Name'), align: 'center' },
            { prop: 'created_at', label: T('CreatedAt'), align: 'center' },
            { label: '', align: 'center', width: 60, slot: 'actions' }
          ]"
          @selection-change="selectedRows = $event"
      >
        <template #actions="{ row }">
          <template v-if="row.id>0">
            <el-dropdown trigger="click" @command="(cmd) => handleRowAction(cmd, row)">
              <el-button text>{{ T('More') }}</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="rules">{{ T('ShareRules') }}</el-dropdown-item>
                  <el-dropdown-item divided command="edit">{{ T('Edit') }}</el-dropdown-item>
                  <el-dropdown-item divided command="delete">{{ T('Delete') }}</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
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
      <Rule :collection="clickRow" :is_my="1"></Rule>
    </app-dialog>

  </div>
</template>

<script setup>
  import { T } from '@/utils/i18n'
  import { computed, ref } from 'vue'
  import { useRepositories } from '@/views/address_book/collection'
  import { onActivated, onMounted, watch } from 'vue'
  import Rule from '@/views/address_book/rule.vue'
  import { useBulkRemove } from '@/composables/useBulkRemove'
  import { remove as apiRemove } from '@/api/my/address_book_collection'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import ActionsToolbar from '@/components/ui/ActionsToolbar.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

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
  } = useRepositories('my')

  const selectedRows = ref([])

  const { bulkRemove: bulkDel } = useBulkRemove({
    removeApi: apiRemove,
    getList,
    label: T('Collections'),
  })

  onMounted(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)
  const list = computed(_ => {
    if (listQuery.page > 1) {
      return listRes.list
    } else {
      return [
        { id: 0, name: T('MyAddressBook') },
        ...listRes.list,
      ]
    }
  })
  const clickRow = ref({})
  const rulesVisible = ref(false)
  const showRules = (row) => {
    clickRow.value = row
    rulesVisible.value = true
  }

  const handleRowAction = (cmd, row) => {
    if (cmd === 'rules') return showRules(row)
    if (cmd === 'edit') return toEdit(row)
    if (cmd === 'delete') {
      selectedRows.value = selectedRows.value.filter(r => r.id !== row.id)
      return del(row)
    }
  }

</script>

<style scoped lang="scss">
.workspace-page {
  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
