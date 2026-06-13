<template>
  <div class="monitoring-page">
    <page-header
        :title="T('FileTransferHistory')"
        subtitle="Inspect file movement direction, peer IDs, paths, counts, sizes, and transfer time."
        eyebrow="Monitoring"
        pulse="warning"
    />
    <filter-bar
        :title="T('Filters')"
        :subtitle="T('Filter file transfer records before export or cleanup.')"
        :fields="filterFields"
        :filters="listQuery"
        @filter="handlerQuery"
    >
      <template #actions>
        <el-button type="danger" @click="toBatchDelete">{{ T('BatchDelete') }}</el-button>
        <el-button type="success" @click="toExport">{{ T('Export') }}</el-button>
      </template>
    </filter-bar>
    <page-section class="list-body" :title="T('FileTransferHistory')" :subtitle="`${listRes.total} records`">
      <el-table :data="listRes.list" v-loading="listRes.loading" border max-height="750" @selection-change="handleSelectionChange">
        <el-table-column type="selection" align="center" width="50"/>
        <el-table-column prop="id" label="ID" align="center" width="100"/>
        <el-table-column :label="T('Peer')" prop="peer_id" align="center" width="120"/>
        <el-table-column :label="T('FromPeer')" prop="from_peer" align="center" width="120"/>
        <el-table-column :label="T('FromName')" prop="from_name" align="center" width="120"/>
        <el-table-column :label="T('Ip')" prop="ip" align="center" width="120"/>
        <el-table-column prop="type" :label="T('Type')" align="center" width="200">
          <template #default="{row}">
            <el-tag v-if="row.type === 1" type="warning"> {{ T('ToRemote') }}:
              <el-icon>
                <Right/>
              </el-icon>
              {{ row.peer_id }}
            </el-tag>
            <el-tag v-else>{{ T('ToLocal') }}:
              <el-icon>
                <Right/>
              </el-icon>
              {{ row.from_peer }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="num" :label="T('Num')" align="center" width="100"/>
        <el-table-column :label="T('FileInfo')" align="center" width="300">
          <template #default="{row}">
            <template v-if="!row.is_file">
              <el-table size="small" :data="row.info?.files?.filter((v,k) => k<showDirFileNum)" fit>
                <el-table-column prop="0" :label="T('FileName')" align="center" width="150" show-overflow-tooltip></el-table-column>
                <el-table-column prop="1" :label="T('Size')" align="center">
                  <template #default="{row:_row}">
                    {{ sizeFormat(_row[1]) }}
                  </template>
                </el-table-column>
              </el-table>
              <el-button size="small" v-if="row.info.files.length>showDirFileNum" style="width: 100%;margin-top: 5px" type="primary" @click="showAllFile(row.info.files)">
                {{ T('More') }}({{ row.info.files.length - showDirFileNum }})
              </el-button>
            </template>
            <div v-else>
              {{ sizeFormat(row.info.files[0][1]) }}
            </div>

          </template>
        </el-table-column>
        <el-table-column prop="path" :label="T('Path')" align="center" width="150" show-overflow-tooltip/>
        <el-table-column prop="uuid" label="uuid" align="center" width="120" show-overflow-tooltip/>
        <el-table-column prop="created_at" :label="T('CreatedAt')" align="center" min-width="120"/>
        <el-table-column :label="T('Actions')" align="center" width="150" fixed="right">
          <template #default="{row}">
            <el-button type="danger" @click="del(row)">{{ T('Delete') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
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
    <el-dialog v-model="allFilesVisible" :title="T('File')">
      <el-table :data="showFiles" max-height="800px">
        <el-table-column type="index" :label="T('IndexNum')" width="120" align="center"></el-table-column>
        <el-table-column prop="0" :label="T('FileName')" align="center"></el-table-column>
        <el-table-column prop="1" :label="T('Size')" align="center">
          <template #default="{row:_row}">
            {{ sizeFormat(_row[1]) }}
          </template>
        </el-table-column>
      </el-table>
      <el-button @click="allFilesVisible=false" style="margin-top: 20px;width: 100%" type="primary">{{ T('Close') }}</el-button>
    </el-dialog>
  </div>
</template>

<script setup>
  import { onActivated, onMounted, ref, watch } from 'vue'
  import { useFileRepositories } from '@/views/audit/reponsitories'
  import { T } from '@/utils/i18n'
  import { sizeFormat } from '@/utils/file'
  import { Right } from '@element-plus/icons'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageSection from '@/components/ui/PageSection.vue'
import FilterBar from '@/components/ui/FilterBar.vue'

const showDirFileNum = 3
const {
  listRes,
  listQuery,
  getList,
  handlerQuery,
  del,
  batchdel,
  toExport,
} = useFileRepositories()

onMounted(getList)
onActivated(getList)

watch(() => listQuery.page, getList)

watch(() => listQuery.page_size, handlerQuery)

const allFilesVisible = ref(false)
const showFiles = ref([])
const showAllFile = (files) => {
  showFiles.value = files
  allFilesVisible.value = true
}

const multipleSelection = ref([])
const handleSelectionChange = (val) => {
  multipleSelection.value = val
}
const toBatchDelete = () => {
  if (multipleSelection.value.length === 0) {
    return
  }
  batchdel(multipleSelection.value)
}

const filterFields = [
  {
    key: 'peer_id',
    label: 'Peer',
    component: 'el-input',
    clearable: true,
    placeholder: 'Peer ID',
  },
  {
    key: 'from_peer',
    label: 'From Peer',
    component: 'el-input',
    clearable: true,
    placeholder: 'From Peer ID',
  },
]
</script>

<style scoped lang="scss">

</style>
