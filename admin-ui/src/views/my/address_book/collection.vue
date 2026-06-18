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
        <template #default="{ disabled, selected }">
          <template v-if="selected.length === 1">
            <el-button type="primary" @click="showRules(selected[0])">{{ T('ShareRules') }}</el-button>
            <el-button type="primary" @click="toEdit(selected[0])">{{ T('Edit') }}</el-button>
          </template>
          <el-button type="danger" :disabled="disabled" @click="bulkDel">
            {{ T('DeleteSelected') }} ({{ selected.length }})
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
            { prop: 'created_at', label: T('CreatedAt'), align: 'center' }
          ]"
          @selection-change="selectedRows = $event"
      >
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
    selectionRef: selectedRows,
    warningMessage: T('DeletingCollectionsWarning') !== 'DeletingCollectionsWarning' ? T('DeletingCollectionsWarning') : 'Deleting this collection will also permanently remove ALL address book entries and sharing rules within it.',
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

</script>

<style scoped lang="scss">
.workspace-page {
  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
