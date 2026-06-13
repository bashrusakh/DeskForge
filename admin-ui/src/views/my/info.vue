<template>
  <div class="profile-page">
    <page-header
        :title="T('Userinfo')"
        subtitle="Review account details, change your password, and manage connected OIDC identities."
        eyebrow="Profile"
        pulse="online"
    />
    <page-section :title="T('Userinfo')" subtitle="Account identity and authentication bindings.">
      <el-form class="info-form" ref="form" label-width="120px" label-suffix="：">
        <el-form-item :label="T('Username')">
          <div>{{ userStore.username }}</div>
        </el-form-item>
        <el-form-item :label="T('Email')">
          <div>{{ userStore.email }}</div>
        </el-form-item>
        <el-form-item :label="T('Password')" prop="password">
          <el-button type="danger" @click="showChangePwd">{{ T('ChangePassword') }}</el-button>
        </el-form-item>
        <el-form-item label="OIDC">
          <el-table :data="oidcData" border fit>
            <el-table-column :label="T('IdP')" prop="op" align="center"></el-table-column>
            <el-table-column :label="T('Status')" prop="status" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.status === 1" type="success">{{ T('HasBind') }}</el-tag>
                <el-tag v-else type="danger">{{ T('NoBind') }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="T('Actions')" align="center" width="200">
              <template #default="{ row }">
                <el-button v-if="row.status === 1" type="danger" size="small" @click="toUnBind(row)">{{ T('UnBind') }}</el-button>
                <el-button v-else type="success" size="small" @click="toBind(row)">{{ T('ToBind') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-form-item>
      </el-form>
    </page-section>
    <page-section class="hello-section" title="Welcome">
      <div v-html="html"></div>
    </page-section>
    <changePwdDialog v-model:visible="changePwdVisible"></changePwdDialog>
  </div>
</template>

<script setup>
  import changePwdDialog from '@/components/changePwdDialog.vue'
  import { computed, ref } from 'vue'
  import { useUserStore } from '@/store/user'
  import { useAppStore } from '@/store/app'
  import { bind, unbind } from '@/api/oauth'
  import { myOauth } from '@/api/user'
  import { ElMessageBox } from 'element-plus'
  import { T } from '@/utils/i18n'
  import { marked } from 'marked'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'

  const appStore = useAppStore()
  const userStore = useUserStore()
  const changePwdVisible = ref(false)
  const showChangePwd = () => {
    changePwdVisible.value = true
  }
  const oidcData = ref([])
  const getMyOauth = async () => {
    const res = await myOauth().catch(_ => false)
    if (res) {
      oidcData.value = res.data
    }

  }
  getMyOauth()
  const toBind = async (row) => {
    const res = await bind({ op: row.op }).catch(_ => false)
    if (res) {
      const { code, url } = res.data
      window.open(url)
    }
  }
  const toUnBind = async (row) => {
    const cf = await ElMessageBox.confirm(T('Confirm?', { param: T('UnBind') }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
      type: 'warning',
    }).catch(_ => false)
    if (!cf) {
      return false
    }
    const res = await unbind({ op: row.op }).catch(_ => false)
    if (res) {
      getMyOauth()
    }

  }

  const html = computed(_ => marked(appStore.setting.hello||''))

</script>

<style scoped lang="scss">
.info-form {
  max-width: 720px;
  margin: 0 auto;

}

.hello-section {
  margin-top: 20px;
}
</style>
