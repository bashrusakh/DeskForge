<template>
  <div class="oauth">
    <theme-switch class="auth-theme" />
    <el-card class="card">
      <div class="card-kicker"><connection-pulse status="warning" /> OAuth binding</div>
      <h2>{{ T('OauthBinding') }}</h2>
      <el-form class="info" label-width="100px">
        <el-form-item :label="T('Op')">
          <div class="impt">{{ oauthInfo.op }}</div>
        </el-form-item>
        <el-form-item :label="T('ThirdName')">
          <div class="impt">{{ oauthInfo.third_name }}</div>
        </el-form-item>
        <el-form-item label-width="0">
          <el-button style="width: 100%" v-if="!resStatus" type="success" size="large" @click="toConfirm">{{ T('Bind') }}</el-button>
        </el-form-item>
        <el-form-item label-width="0">
          <el-button style="width: 100%" size="large" @click="out">{{ T('Close') }}</el-button>
        </el-form-item>
      </el-form>
      {{ T('OauthCloseNote') }}
    </el-card>
  </div>
</template>

<script setup>
  import { ref, onMounted } from 'vue'
  import { info, confirm, bindConfirm } from '@/api/oauth'
  import { useRoute, useRouter } from 'vue-router'
  import { ElMessage } from 'element-plus'
  import { T } from '@/utils/i18n'
  import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'
  import ThemeSwitch from '@/components/ui/ThemeSwitch.vue'

  const oauthInfo = ref({})
  const route = useRoute()
  const router = useRouter()
  const code = route.params?.code
  if (!code) {
    router.push('/')
  }
  const getInfo = async () => {
    const res = await info({ code }).catch(_ => false)
    if (res) {
      oauthInfo.value = res.data
    } else {
      router.push('/')
    }
  }
  getInfo()
  const resStatus = ref(0)
  const toConfirm = async () => {
    const res = await bindConfirm({ code }).catch(_ => false)
    if (res) {
      resStatus.value = 1
      if (res.data.device_type === 'webadmin') {
        ElMessage.success(T('OperationSuccess'))
        //后台登录
        router.push('/')
      } else {
        ElMessage.success(T('OperationSuccessAndCloseAfter3Seconds'))
        setTimeout(_ => {
          out()
        }, 3000)
      }

    }
  }
  const out = () => {
    window.close()
  }
</script>

<style scoped lang="scss">
.oauth {
  position: relative;
  width: 100vw;
  min-height: 100vh;
  background:
    radial-gradient(circle at 24% 18%, color-mix(in srgb, var(--color-primary) 18%, transparent), transparent 26rem),
    var(--color-bg);
  padding: 20vh 20px 40px;
  box-sizing: border-box;

  .auth-theme {
    position: absolute;
    top: 20px;
    right: 20px;
  }

  .card {
    max-width: 500px;
    padding: 24px;
    background: color-mix(in srgb, var(--color-surface) 94%, transparent);
    color: var(--color-text);
    border: 1px solid var(--color-border);
    border-radius: 24px;
    box-shadow: var(--shadow-card);
    margin: 0 auto;
    text-align: center;

    .card-kicker {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      margin-bottom: 10px;
      color: var(--color-muted);
      font-family: var(--font-mono);
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.05em;
      text-transform: uppercase;
    }

    h2 {
      margin: 0 0 24px;
      color: var(--color-text);
      font-weight: 700;
    }

    .info {
      display: block;
      line-height: 30px;
      margin-bottom: 50px;

      ::v-deep(.el-form-item__label) {
        color: var(--color-muted);
        font-weight: 600;
      }
    }

    .impt {
      color: var(--color-text);
      font-family: var(--font-mono);
      font-weight: bold;
      font-size: 20px;
    }
  }
}
</style>
