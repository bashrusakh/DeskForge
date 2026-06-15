<template>
  <div class="share-rules-page">
    <page-section class="list-query" :title="T('ShareRules')" :subtitle="props.collection.name || T('Name')">
      <el-form inline label-width="80px">
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" title="Rules" :subtitle="`${listRes.total} rules`">
      <actions-toolbar :selected="selectedRows">
        <template #default="{ disabled }">
          <el-button type="danger" :disabled="disabled" @click="bulkDel">
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
            { label: T('Rule'), align: 'center', slot: 'rule' },
            { label: T('Type'), align: 'center', slot: 'type' },
            { label: T('ShareTo'), align: 'center', slot: 'shareTo' },
            { prop: 'created_at', label: T('CreatedAt'), align: 'center' },
            { label: '', align: 'center', width: 60, slot: 'actions' }
          ]"
          @selection-change="selectedRows = $event"
      >
        <template #rule="{ row }">
          {{ rules.find(r => r.value === row.rule)?.label }}
        </template>
        <template #type="{ row }">
          {{ types.find(t => t.value === row.type)?.label }}
        </template>
        <template #shareTo="{ row }">
          <div v-if="row.type===TYPE_U">
            {{ users.find(u => u.id === row.to_id)?.username }}
          </div>
          <div v-else-if="row.type===TYPE_G">
            {{ groups.find(g => g.id === row.to_id)?.name }}
          </div>
        </template>
        <template #actions="{ row }">
          <el-dropdown trigger="click" @command="(cmd) => handleRowAction(cmd, row)">
            <el-button text>{{ T('More') }}</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="edit">{{ T('Edit') }}</el-dropdown-item>
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
    <app-dialog
        v-model="formVisible"
        :title="!formData.id ? T('Create') : T('Update')"
        width="800"
        @confirm="submit"
    >
      <el-form class="dialog-form" ref="form" :model="formData" label-width="120px">
        <el-form-item :label="T('Name')">
          {{ props.collection.name }}
        </el-form-item>
        <el-form-item :label="T('Rule')" prop="rule" required>
          <el-radio-group v-model="formData.rule">
            <el-radio v-for="item in rules" :key="item.value" :value="item.value">
              {{ item.label }}
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="T('Type')" prop="type" required>
          <el-radio-group v-model="formData.type">
            <el-radio v-for="item in types" :key="item.value" :value="parseInt(item.value)">
              {{ item.label }}
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="T('ShareTo')" prop="g_id" required>
          <div style="width: 30%">
            <el-select v-model="formData.g_id" @change="changeGId">
              <el-option
                  v-for="item in groups"
                  :key="item.id"
                  :label="item.name"
                  :value="item.id"
              ></el-option>
            </el-select>
          </div>
          <div style="width: 30%;margin-left: 20px">
            <el-select v-model="formData.u_id" v-if="formData.type===TYPE_U">
              <el-option
                  v-for="item in users.filter(u => u.group_id === formData.g_id)"
                  :key="item.id"
                  :label="item.username"
                  :value="item.id"
              ></el-option>
            </el-select>
          </div>
        </el-form-item>
      </el-form>
    </app-dialog>
  </div>
</template>

<script setup>

  import { T } from '@/utils/i18n'
  import { useRepositories } from '@/views/address_book/rule'
  import { onActivated, onMounted, ref, watch } from 'vue'
  import { useBulkRemove } from '@/composables/useBulkRemove'
  import { remove as apiRemove } from '@/api/address_book_collection_rule'
  import PageSection from '@/components/ui/PageSection.vue'
  import ActionsToolbar from '@/components/ui/ActionsToolbar.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const props = defineProps({
    collection: {
      type: Object,
      required: true,
    },
    is_my: {
      type: Number,
      default: 0,
    },
  })
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
    rules,
    types,
    groups,
    users,
    getGroupUsers,
    TYPE_G,
    TYPE_U,
    changeGId,
  } = useRepositories(props.is_my ? 'my' : 'admin')

  formData.collection_id = props.collection.id
  formData.user_id = props.collection.user_id
  listQuery.collection_id = props.collection.id

  onMounted(getGroupUsers)
  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)

  const selectedRows = ref([])

  const { bulkRemove: bulkDel } = useBulkRemove({
    removeApi: apiRemove,
    getList,
    label: T('Rule'),
  })

  const handleRowAction = (cmd, row) => {
    if (cmd === 'edit') return toEdit(row)
    if (cmd === 'delete') {
      selectedRows.value = selectedRows.value.filter(r => r.id !== row.id)
      return del(row)
    }
  }

</script>

<style scoped lang="scss">
.share-rules-page {
  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
