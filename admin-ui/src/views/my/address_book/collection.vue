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
      <el-table :data="list" v-loading="listRes.loading" border>
        <!--        <el-table-column prop="id" label="ID" align="center"/>-->
        <el-table-column prop="name" :label="T('Name')" align="center"/>
        <el-table-column prop="created_at" :label="T('CreatedAt')" align="center"/>

        <el-table-column :label="T('Actions')" align="center" class-name="table-actions" width="320" fixed="right">
          <template #default="{row}">
            <template v-if="row.id>0">
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
    <el-dialog v-model="formVisible" width="800" :title="!formData.id?T('Create') :T('Update') ">
      <el-form class="dialog-form" ref="form" :model="formData" label-width="120px">
        <el-form-item :label="T('Name')" prop="name" required>
          <el-input v-model="formData.name"></el-input>
        </el-form-item>
        <el-form-item>
          <el-button @click="formVisible = false">{{ T('Cancel') }}</el-button>
          <el-button @click="submit" type="primary">{{ T('Submit') }}</el-button>
        </el-form-item>
      </el-form>
    </el-dialog>
    <el-dialog v-model="rulesVisible" :title="T('ShareRules')" destroy-on-close top="5vh" width="80%">
      <Rule :collection="clickRow" :is_my="1"></Rule>
    </el-dialog>

  </div>
</template>

<script setup>
  import { T } from '@/utils/i18n'
  import { computed, ref } from 'vue'
  import { useRepositories } from '@/views/address_book/collection'
  import { onActivated, onMounted, watch } from 'vue'
  import Rule from '@/views/address_book/rule.vue'
  import { ArrowDown } from '@element-plus/icons-vue'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'

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
