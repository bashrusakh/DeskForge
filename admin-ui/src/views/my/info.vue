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
        <el-form-item :label="T('Nickname')">
          <el-input v-model="profileForm.nickname" maxlength="128" style="max-width: 360px" />
        </el-form-item>
        <el-form-item :label="T('Email')">
          <el-input v-model="profileForm.email" maxlength="128" style="max-width: 360px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveProfile">{{ T('Save') }}</el-button>
        </el-form-item>
        <el-form-item :label="T('Password')" prop="password">
          <el-button type="danger" @click="showChangePwd">{{ T('ChangePassword') }}</el-button>
        </el-form-item>
        <el-form-item label="OIDC">
          <data-table
              :data="oidcData"
              row-key="op"
              :columns="[
                { prop: 'op', label: T('IdP'), align: 'center' },
                { label: T('Status'), align: 'center', slot: 'status' },
                { label: T('Actions'), align: 'center', width: 200, slot: 'actions' }
              ]"
          >
            <template #status="{ row }">
              <el-tag v-if="row.status === 1" type="success">{{ T('HasBind') }}</el-tag>
              <el-tag v-else type="danger">{{ T('NoBind') }}</el-tag>
            </template>
            <template #actions="{ row }">
              <el-button v-if="row.status === 1" type="danger" size="small" @click="toUnBind(row)">{{ T('UnBind') }}</el-button>
              <el-button v-else type="success" size="small" @click="toBind(row)">{{ T('ToBind') }}</el-button>
            </template>
          </data-table>
        </el-form-item>
      </el-form>
    </page-section>
    <changePwdDialog v-model:visible="changePwdVisible"></changePwdDialog>
  </div>
</template>

<script setup>
  import changePwdDialog from '@/components/changePwdDialog.vue'
  import { ref, reactive } from 'vue'
  import { useUserStore } from '@/store/user'
  import { bind, unbind } from '@/api/oauth'
  import { myOauth, updateCurrent } from '@/api/user'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import { T } from '@/utils/i18n'
  import PageHeader from '@/components/ui/PageHeader.vue'
  import PageSection from '@/components/ui/PageSection.vue'
  import DataTable from '@/components/ui/DataTable.vue'

  const userStore = useUserStore()
  const changePwdVisible = ref(false)
  const showChangePwd = () => {
    changePwdVisible.value = true
  }

  // AU-M-021: профиль теперь редактируемый (nickname, email).
  const profileForm = reactive({
    nickname: userStore.nickname || '',
    email: userStore.email || '',
  })
  const saving = ref(false)
  const saveProfile = async () => {
    saving.value = true
    const res = await updateCurrent({ ...profileForm }).catch(e => e)
    saving.value = false
    if (res && !res.code) {
      ElMessage.success(T('OperationSuccess'))
      await userStore.info()
      profileForm.nickname = userStore.nickname || ''
      profileForm.email = userStore.email || ''
    } else {
      ElMessage.error((res && (res.msg || res.message)) || T('OperationFailed'))
    }
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

</script>

<style scoped lang="scss">
.info-form {
  max-width: 720px;
  margin: 0 auto;

}
</style>
