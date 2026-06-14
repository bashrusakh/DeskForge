<template>
  <div class="security-page">
    <page-header
        :title="T('Group')"
        subtitle="Manage user groups used for ownership, access control, and shared address book rules."
        eyebrow="Security"
        pulse="online"
    />
    <page-section class="list-query" title="Group controls" subtitle="Refresh the list or create a new user group.">
      <el-form inline label-width="80px">
        <!--        <el-form-item label="名称">
                  <el-input v-model="listQuery.name"></el-input>
                </el-form-item>-->
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" :title="T('Group')" :subtitle="`${listRes.total} groups`">
      <data-table
          :data="listRes.list"
          :loading="listRes.loading"
          row-key="id"
          :columns="[
            { prop: 'id', label: 'ID', align: 'center', width: 100 },
            { prop: 'name', label: T('Name'), align: 'center' },
            { label: T('Type'), align: 'center', slot: 'type' },
            { prop: 'created_at', label: T('CreatedAt'), align: 'center' },
            { prop: 'updated_at', label: T('UpdatedAt'), align: 'center' },
            { label: T('Actions'), align: 'center', width: 200, fixed: 'right', slot: 'actions' }
          ]"
      >
        <template #type="{ row }">
          <span v-if="row.type === 1">{{ T('CommonGroup') }}</span>
          <span v-else>{{ T('SharedGroup') }}</span>
        </template>
        <template #actions="{ row }">
          <el-button @click="toEdit(row)">{{ T('Edit') }}</el-button>
          <el-button type="danger" @click="del(row)">{{ T('Delete') }}</el-button>
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
        <el-form-item :label="T('Type')" prop="type" required>
          <el-radio-group v-model="formData.type">
            <el-radio v-for="item in groupTypes" :key="item.value" :value="item.value" style="display: block">
              {{ item.label }}
              <span style="font-size: 12px;color: #999">{{ item.note }}</span>
            </el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
    </app-dialog>
  </div>
</template>

<script setup>
  import { onMounted, reactive, watch, ref, onActivated } from 'vue'
  import { list, create, update, detail, remove } from '@/api/group'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { T } from '@/utils/i18n'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const listRes = reactive({
    list: [], total: 0, loading: false,
  })
  const listQuery = reactive({
    page: 1,
    page_size: 10,
  })

  const getList = async () => {
    listRes.loading = true
    const res = await list(listQuery).catch(_ => false)
    listRes.loading = false
    if (res) {
      listRes.list = res.data.list
      listRes.total = res.data.total
    }
  }
  const handlerQuery = () => {
    if (listQuery.page === 1) {
      getList()
    } else {
      listQuery.page = 1
    }
  }

  const del = async (row) => {
    const cf = await ElMessageBox.confirm(T('Confirm?', { param: T('Delete') }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
      type: 'warning',
    }).catch(_ => false)
    if (!cf) {
      return false
    }

    const res = await remove({ id: row.id }).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      getList()
    }
  }
  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)

  const groupTypes = [
    { label: T('CommonGroup'), value: 1, note: T('CommonGroupNote') },
    { label: T('SharedGroup'), value: 2, note: T('SharedGroupNote') },
  ]
  const formVisible = ref(false)
  const formData = reactive({
    id: 0,
    name: '',
    type: 1,
  })

  const toEdit = (row) => {
    formVisible.value = true
    formData.id = row.id
    formData.name = row.name
    formData.type = row.type
  }
  const toAdd = () => {
    formVisible.value = true
    formData.id = 0
    formData.name = ''
    formData.type = 1
  }
  const submit = async () => {
    const api = formData.id ? update : create
    const res = await api(formData).catch(_ => false)
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      formVisible.value = false
      getList()
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
