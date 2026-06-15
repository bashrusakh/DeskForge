<template>
  <div class="access-page">
    <page-header
        :title="T('Tags')"
        subtitle="Maintain color-coded address book tags used to group and find remote devices faster."
        eyebrow="Access"
        pulse="online"
    />
    <page-section class="list-query" title="Filters" subtitle="Filter tags by owner and address book.">
      <el-form inline label-width="120px">
        <el-form-item :label="T('Owner')">
          <el-select v-model="listQuery.user_id" clearable @change="changeUser">
            <el-option
                v-for="item in allUsers"
                :key="item.id"
                :label="item.username"
                :value="item.id"
            ></el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="T('Name')">
          <el-select v-model="listQuery.collection_id" clearable>
            <el-option :value="0" :label="T('MyAddressBook')"></el-option>
            <el-option v-for="c in collectionListRes.list" :key="c.id" :label="c.name" :value="c.id"></el-option>
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" :title="T('Tags')" :subtitle="`${listRes.total} tags`">
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
            { prop: 'id', label: 'ID', align: 'center', width: 100 },
            { label: T('Owner'), align: 'center', slot: 'owner' },
            { label: T('Name'), align: 'center', width: 150, slot: 'collection' },
            { prop: 'name', label: T('Name'), align: 'center' },
            { label: T('Color'), align: 'center', slot: 'color' },
            { prop: 'created_at', label: T('CreatedAt'), align: 'center' },
            { prop: 'updated_at', label: T('UpdatedAt'), align: 'center' },
            { label: '', align: 'center', width: 60, slot: 'actions' }
          ]"
          @selection-change="selectedRows = $event"
      >
        <template #owner="{ row }">
          <span v-if="row.user_id"> <el-tag>{{ allUsers?.find(u => u.id === row.user_id)?.username }}</el-tag> </span>
        </template>
        <template #collection="{ row }">
          <span v-if="row.collection_id === 0">{{ T('MyAddressBook') }}</span>
          <span v-else>{{ row.collection?.name }}</span>
        </template>
        <template #color="{ row }">
          <div class="colors">
            <div style="background-color: var(--tag-bg-color)" class="colorbox">
              <div :style="{backgroundColor: row.color}" class="dot"></div>
            </div>
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
        <el-form-item :label="T('Owner')" prop="user_id" required>
          <el-select v-model="formData.user_id" @change="changeUserForUpdate">
            <el-option
                v-for="item in allUsers"
                :key="item.id"
                :label="item.username"
                :value="item.id"
            ></el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="T('Name')" prop="collection_id" required>
          <el-select v-model="formData.collection_id" clearable>
            <el-option :value="0" :label="T('MyAddressBook')"></el-option>
            <el-option v-for="c in collectionListResForUpdate.list" :key="c.id" :label="c.name" :value="c.id"></el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="T('Name')" prop="name" required>
          <el-input v-model="formData.name"></el-input>
        </el-form-item>
        <el-form-item :label="T('Color')" prop="color" required>
          <el-color-picker v-model="formData.color" show-alpha @active-change="activeChange"></el-color-picker>
          <div class="colors">
            <div style="background-color: var(--tag-bg-color)" class="colorbox">
              <div :style="{backgroundColor: currentColor}" class="dot"></div>
            </div>
          </div>
        </el-form-item>
      </el-form>
    </app-dialog>
  </div>
</template>

<script setup>
  import { onMounted, reactive, watch, ref, onActivated } from 'vue'
  import { useRepositories } from '@/views/tag/index'
  import { T } from '@/utils/i18n'
  import { loadAllUsers } from '@/global'
  import { useBulkRemove } from '@/composables/useBulkRemove'
  import { remove as apiRemove } from '@/api/tag'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import ActionsToolbar from '@/components/ui/ActionsToolbar.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const { allUsers, getAllUsers } = loadAllUsers()
  onMounted(getAllUsers)
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
    activeChange,
    currentColor,

    collectionListRes,
    changeUser,
    // getCollectionList,

    collectionListResForUpdate,
    changeUserForUpdate,
    // getCollectionListForUpdate,
  } = useRepositories('admin')

  const selectedRows = ref([])

  const { bulkRemove: bulkDel } = useBulkRemove({
    removeApi: apiRemove,
    getList,
    label: T('Tag'),
  })

  const handleRowAction = (cmd, row) => {
    if (cmd === 'edit') return toEdit(row)
    if (cmd === 'delete') {
      selectedRows.value = selectedRows.value.filter(r => r.id !== row.id)
      return del(row)
    }
  }

  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)


</script>

<style scoped lang="scss">
.list-query .el-select {
  --el-select-width: 160px;
}

.access-page {
  :deep(.list-page .el-card__body) {
    display: flex;
    justify-content: flex-end;
  }
}

.colors {
  display: flex;
  justify-content: center;
  align-items: center;

  .colorbox {
    width: 50px;
    height: 30px;
    display: flex;
    justify-content: center;
    align-items: center;

    .dot {
      width: 10px;
      height: 10px;
      display: block;
      border-radius: 50%;
    }
  }

}

</style>
