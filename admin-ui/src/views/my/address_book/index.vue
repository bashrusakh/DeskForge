<template>
  <div class="workspace-page">
    <page-header
        title="My Address Book"
        subtitle="Manage your personal saved devices, tags, and quick connection actions."
        eyebrow="Workspace"
        pulse="online"
    />
    <page-section class="list-query" title="Filters" subtitle="Filter personal address book entries by collection, device ID, username, or hostname.">
      <el-form inline label-width="120px">
        <el-form-item :label="T('AddressBookName')">
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
          <el-button type="primary" @click="showBatchEditTags">{{ T('BatchEditTags') }}</el-button>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="list-body" title="My Address Book" :subtitle="`${listRes.total} entries`">
      <data-table
          :data="listRes.list"
          :loading="listRes.loading"
          selectable
          @selection-change="handleSelectionChange"
          row-key="row_id"
          :columns="[
            { label: 'ID', align: 'center', width: 200, slot: 'id' },
            { label: 'Name', align: 'center', width: 150, slot: 'collection' },
            { prop: 'username', label: T('Username'), align: 'center', width: 150 },
            { prop: 'hostname', label: T('Hostname'), align: 'center', width: 150 },
            { prop: 'tags', label: T('Tags'), align: 'center' },
            { prop: 'alias', label: T('Alias'), align: 'center', width: 150 },
            { prop: 'peer.version', label: T('Version'), align: 'center', width: 100 },
            { prop: 'hash', label: T('Hash'), align: 'center', width: 150, showOverflowTooltip: true },
            { label: T('Actions'), align: 'center', width: 420, fixed: 'right', slot: 'actions' }
          ]"
      >
        <template #id="{ row }">
          <div class="device-id-cell">
            <PlatformIcons :name="platformList.find(p=>p.label===row.platform)?.icon" style="width: 20px;height: 20px;display: inline-block" color="var(--basicBlack)"/>
            <copyable-text :text="row.id" />
          </div>
        </template>
        <template #collection="{ row }">
          <span v-if="row.collection_id === 0">{{ T('MyAddressBook') }}</span>
          <span v-else>{{ collectionListRes.list.find(c => c.id === row.collection_id)?.name }}</span>
        </template>
        <template #actions="{ row }">
          <el-space wrap>
            <el-button type="success" @click="connectByClient(row.id)">{{ T('Link') }}</el-button>
            <el-button v-if="appStore.setting.appConfig.web_client" type="primary" @click="toWebClientLink(row)">Web Client</el-button>
            <el-dropdown trigger="click">
              <el-button>
                {{ T('More') }}<el-icon class="el-icon--right"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item v-if="appStore.setting.appConfig.web_client" @click="toShowShare(row)">{{ T('ShareByWebClient') }}</el-dropdown-item>
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
        :title="!formData.row_id ? T('Create') : T('Update')"
        width="800"
        @confirm="submit"
    >
      <el-form class="dialog-form" ref="form" :model="formData" label-width="120px">
        <el-form-item :label="T('AddressBookName')" required prop="collection_id">
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
    <app-dialog
        v-model="shareToWebClientVisible"
        width="900"
        :show-confirm="false"
        :hide-footer="true"
    >
      <shareByWebClient :id="shareToWebClientForm.id"
                        :hash="shareToWebClientForm.hash"
                        @cancel="shareToWebClientVisible=false"
                        @success=""/>
    </app-dialog>
    <app-dialog
        v-model="batchEditTagVisible"
        :title="T('BatchEditTags')"
        width="800"
        @confirm="submitBatchEditTags"
    >
      <el-form :model="batchEditTagsFormData" label-width="120px" class="dialog-form">
        <el-form-item :label="T('Tags')" prop="tags">
          <el-select v-model="batchEditTagsFormData.tags" multiple>
            <el-option
                v-for="item in tagListResForBatchEdit.list"
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
  import { onActivated, onMounted, reactive, ref, watch } from 'vue'
  import { useBatchUpdateTagsRepositories, useRepositories } from '@/views/address_book'
  import { toWebClientLink } from '@/utils/webclient'
  import { T } from '@/utils/i18n'
  import shareByWebClient from '@/views/address_book/components/shareByWebClient.vue'
  import { useAppStore } from '@/store/app'
  import { connectByClient } from '@/utils/peer'
  import { ArrowDown } from '@element-plus/icons-vue'
  import PlatformIcons from '@/components/icons/platform.vue'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import CopyableText from '@/components/ui/CopyableText.vue'
  import DataTable from '@/components/ui/DataTable.vue'
  import AppDialog from '@/components/ui/AppDialog.vue'

  const appStore = useAppStore()
  const {
    listRes,
    listQuery,
    getList,
    handlerQuery,
    collectionListRes,
    getCollectionList,

    del,

    formVisible,
    platformList,
    formData,
    toEdit,
    toAdd,
    submit,
    tagListRes,
    changeCollectionForUpdate,
    getCollectionListForUpdate,
    collectionListResForUpdate,
    // collectionListQuery,

  } = useRepositories('my')

  onMounted(getCollectionList)
  onMounted(getCollectionListForUpdate)
  onMounted(getList)
  onActivated(getList)

  watch(() => listQuery.page, getList)

  watch(() => listQuery.page_size, handlerQuery)

  const shareToWebClientVisible = ref(false)
  const shareToWebClientForm = reactive({
    id: '',
    hash: '',
  })
  const toShowShare = (row) => {
    shareToWebClientForm.id = row.id
    shareToWebClientForm.hash = row.hash
    shareToWebClientVisible.value = true
  }
  const {
    tagListRes: tagListResForBatchEdit,
    getTagList: getTagListForBatchEdit,
    visible: batchEditTagVisible,
    show: showBatchEditTags,
    formData: batchEditTagsFormData,
    submit: _submitBatchEditTags,
  } = useBatchUpdateTagsRepositories()
  onMounted(getTagListForBatchEdit)
  const submitBatchEditTags = async () => {
    const res = await _submitBatchEditTags().catch(_ => false)
    if (res) {
      getList()
    }
  }

  const multipleSelection = ref([])
  const handleSelectionChange = (val) => {
    multipleSelection.value = val

    batchEditTagsFormData.value.row_ids = val.map(v => v.row_id)
  }


</script>

<style scoped lang="scss">

.workspace-page {
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
