<template>
  <div class="access-page">
    <page-header
        :title="T('AddressBook')"
        subtitle="Manage saved devices, owners, aliases, tags, and quick connection actions."
        eyebrow="Access"
        pulse="online"
    />
    <page-section class="list-query" title="Filters" subtitle="Narrow entries by owner, address book, device ID, username, or hostname.">
      <el-form inline label-width="120px">
        <el-form-item :label="T('Owner')">
          <el-select v-model="listQuery.user_id" clearable @change="changeQueryUser">
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
        <el-form-item :label="T('Id')">
          <el-input v-model="listQuery.id" clearable></el-input>
        </el-form-item>
        <el-form-item :label="T('Username')">
          <el-input v-model="listQuery.username" clearable></el-input>
        </el-form-item>
        <el-form-item :label="T('Hostname')">
          <el-input v-model="listQuery.hostname" clearable></el-input>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handlerQuery">{{ T('Filter') }}</el-button>
          <el-button type="danger" @click="toAdd">{{ T('Add') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" :title="T('AddressBook')" :subtitle="`${listRes.total} entries`">
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
            { label: 'ID', align: 'center', width: 200, slot: 'id' },
            { label: T('Owner'), align: 'center', width: 200, slot: 'owner' },
            { label: T('Name'), align: 'center', width: 150, slot: 'collection' },
            { prop: 'username', label: T('Username'), align: 'center', width: 150 },
            { prop: 'hostname', label: T('Hostname'), align: 'center', width: 150 },
            { prop: 'tags', label: T('Tags'), align: 'center' },
            { prop: 'alias', label: T('Alias'), align: 'center', width: 150 },
            { prop: 'peer.version', label: T('Version'), align: 'center', width: 100 },
            { prop: 'hash', label: T('Hash'), align: 'center', width: 150, showOverflowTooltip: true },
            { label: '', align: 'center', width: 220, slot: 'actions' }
          ]"
          @selection-change="selectedRows = $event"
      >
        <template #id="{ row }">
          <div class="device-id-cell">
            <PlatformIcons :name="platformList.find(p=>p.label===row.platform)?.icon" style="width: 20px;height: 20px;display: inline-block" color="var(--basicBlack)"/>
            <copyable-text :text="row.id" />
          </div>
        </template>
        <template #owner="{ row }">
          <span v-if="row.user_id"> <el-tag>{{ allUsers?.find(u => u.id === row.user_id)?.username }}</el-tag> </span>
        </template>
        <template #collection="{ row }">
          <span v-if="row.collection_id === 0">{{ T('MyAddressBook') }}</span>
          <span v-else>{{ row.collection?.name }}</span>
        </template>
        <template #actions="{ row }">
          <el-space wrap>
            <el-button type="success" size="small" @click="connectByClient(row.id)">{{ T('Link') }}</el-button>
            <el-dropdown trigger="click" @command="(cmd) => handleRowAction(cmd, row)">
              <el-button size="small">
                {{ T('More') }}<el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item v-if="appStore.setting.appConfig.web_client" command="webClient">Web Client</el-dropdown-item>
                  <el-dropdown-item command="edit">{{ T('Edit') }}</el-dropdown-item>
                  <el-dropdown-item divided command="delete">{{ T('Delete') }}</el-dropdown-item>
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
        :title="!formData.row_id ? T('Create') : T('Update')"
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
        <el-form-item :label="T('Name')">
          <el-select v-model="formData.collection_id" clearable @change="changeCollectionForUpdate">
            <el-option :value="0" :label="T('MyAddressBook')"></el-option>
            <el-option v-for="c in collectionListResForUpdate.list" :key="c.id" :label="c.name" :value="c.id"></el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="ID" prop="id" required>
          <el-input v-model="formData.id"></el-input>
        </el-form-item>
        <el-form-item :label="T('Username')" prop="username">
          <el-input v-model="formData.username"></el-input>
        </el-form-item>
        <el-form-item :label="T('Alias')" prop="alias">
          <el-input v-model="formData.alias"></el-input>
        </el-form-item>
        <el-form-item :label="T('Hash')" prop="hash">
          <el-input v-model="formData.hash"></el-input>
        </el-form-item>
        <el-form-item :label="T('Hostname')" prop="hostname">
          <el-input v-model="formData.hostname"></el-input>
        </el-form-item>
        <el-form-item :label="T('Platform')" prop="platform">
          <el-select v-model="formData.platform">
            <el-option
                v-for="item in platformList"
                :key="item.value"
                :label="item.label"
                :value="item.value"
            ></el-option>
          </el-select>
        </el-form-item>

        <el-form-item :label="T('Tags')" prop="tags">
          <el-select v-model="formData.tags" multiple>
            <el-option
                v-for="item in tagListRes.list"
                :key="item.name"
                :label="item.name"
                :value="item.name"
            ></el-option>
          </el-select>
        </el-form-item>
      </el-form>
    </app-dialog>
  </div>
</template>

<script setup>
  import { onActivated, onMounted, ref, watch } from 'vue'
  import { useRepositories } from '@/views/address_book/index'
  import { toWebClientLink } from '@/utils/webclient'
  import { T } from '@/utils/i18n'
  import { useRoute } from 'vue-router'
  import { connectByClient } from '@/utils/peer'
  import { useAppStore } from '@/store/app'
  import { useBulkRemove } from '@/composables/useBulkRemove'
  import { remove as apiRemove } from '@/api/address_book'
  import { ArrowDown } from '@element-plus/icons-vue'
  import PlatformIcons from '@/components/icons/platform.vue'
  import { loadAllUsers } from '@/global'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import ActionsToolbar from '@/components/ui/ActionsToolbar.vue'
  import CopyableText from '@/components/ui/CopyableText.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const appStore = useAppStore()
  const route = useRoute()
  const { allUsers, getAllUsers } = loadAllUsers()

  const {
    listRes,
    listQuery,
    getList,
    handlerQuery,
    collectionListRes,

    del,
    formVisible,
    platformList,
    formData,
    toEdit,
    toAdd,
    submit,
    changeUserForUpdate,
    changeCollectionForUpdate,
    collectionListResForUpdate,
    tagListRes,

    changeQueryUser,
  } = useRepositories('admin')

  if (route.query?.user_id) {
    listQuery.user_id = parseInt(route.query.user_id)
  }

  const selectedRows = ref([])

  const { bulkRemove: bulkDel } = useBulkRemove({
    removeApi: apiRemove,
    getList,
    label: T('AddressBook'),
  })

  const handleRowAction = (cmd, row) => {
    if (cmd === 'edit') return toEdit(row)
    if (cmd === 'delete') {
      selectedRows.value = selectedRows.value.filter(r => r.id !== row.id)
      return del(row)
    }
    if (cmd === 'webClient') return toWebClientLink(row)
  }

  onMounted(getAllUsers)
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

.device-id-cell {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
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
