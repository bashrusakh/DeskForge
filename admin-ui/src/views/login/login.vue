<template>
  <div class="login-container">
    <theme-switch class="auth-theme" />
    <section class="auth-visual">
      <div class="visual-kicker"><connection-pulse status="online" /> Remote access server</div>
      <h1>Operate your RustDesk network from one console.</h1>
      <p>Sign in to manage devices, users, security events, server controls, and custom client builds.</p>
      <div class="signal-card">
        <div><span>ID</span><strong>ready</strong></div>
        <div><span>Relay</span><strong>available</strong></div>
        <div><span>API</span><strong>protected</strong></div>
      </div>
    </section>

    <div class="login-card">
      <img src="@/assets/logo.png" alt="logo" class="login-logo"/>
      <h2>{{ T('Login') }}</h2>

      <el-form v-if="!disablePwd" label-position="top" class="login-form">
        <el-form-item :label="T('Username')">
          <el-input v-model="form.username" type="username" class="login-input"></el-input>
        </el-form-item>

        <el-form-item :label="T('Password')">
          <el-input v-model="form.password" type="password" @keyup.enter.native="login" show-password
                    class="login-input"></el-input>
        </el-form-item>
        <el-form-item :label="T('Captcha')" v-if="captchaCode">
          <el-input v-model="form.captcha" @keyup.enter.native="login"  class="login-input captcha-input">
            <template #append>
              <img :src="captchaCode.b64" @click="loadCaptcha" class="captcha" alt="captcha"/>
            </template>
          </el-input>
        </el-form-item>
        <el-form-item>
          <el-button @click="login" type="primary" class="login-button">{{ T('Login') }}</el-button>
          <el-button v-if="allowRegister" @click="register" class="login-button">{{ T('Register') }}</el-button>
        </el-form-item>
      </el-form>

      <div class="divider" v-if="options.length > 0 && !disablePwd">
        <span>{{ T('or login in with') }}</span>
      </div>

      <div class="oidc-options">
        <div v-for="(option, index) in options" :key="index" class="oidc-option">
          <el-button @click="handleOIDCLogin(option.name)" class="oidc-btn">
            <img :src="getProviderImage(option.name)" alt="provider" class="oidc-icon"/>
            <span>{{ T(option.name) }}</span>
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
  import { reactive, onMounted, ref } from 'vue'
  import { useUserStore } from '@/store/user'
  import { ElMessage } from 'element-plus'
  import { T } from '@/utils/i18n'
  import { useRoute, useRouter } from 'vue-router'
  import { loginOptions, captcha } from '@/api/login'
  import { getCode, removeCode } from '@/utils/auth'

  const oauthInfo = ref({})
  const userStore = useUserStore()
  const route = useRoute()
  const router = useRouter()
  const options = reactive([]) // 存储 OIDC 登录选项

  let platform = window.navigator.platform
  if (navigator.platform.indexOf('Mac') === 0) {
    platform = 'mac'
  } else if (navigator.platform.indexOf('Win') === 0) {
    platform = 'windows'
  } else if (navigator.platform.indexOf('Linux armv') === 0) {
    platform = 'android'
  } else if (navigator.platform.indexOf('Linux') === 0) {
    platform = 'linux'
  }
  const userAgent = navigator.userAgent
  let browser = 'Unknown Browser'
  if (/chrome|crios/i.test(userAgent)) browser = 'Chrome'
  else if (/firefox|fxios/i.test(userAgent)) browser = 'Firefox'
  else if (/safari/i.test(userAgent) && !/chrome/i.test(userAgent)) browser = 'Safari'
  else if (/edg/i.test(userAgent)) browser = 'Edge'

  const form = reactive({
    username: '',
    password: '',
    platform: platform,
    captcha: '',
    captcha_id: ''
  })

  const captchaCode = ref('')
  const redirect = route.query?.redirect
  const login = async () => {
    const res = await userStore.login(form).catch(e => e)
    if (!res.code) {
      ElMessage.success(T('LoginSuccess'))
      router.push({ path: redirect || '/dashboard', replace: true })
      return
    }
    if (res.code === 110) {
      // need captcha
      loadCaptcha()
    }
  }

  const loadCaptcha = async () => {
    const captchaRes = await captcha().catch(_ => false)
    console.log(captchaRes)
    captchaCode.value = captchaRes.data.captcha
    form.captcha_id = captchaRes.data.captcha.id
  }

  const handleOIDCLogin = (provider) => {
    userStore.oidc(provider, platform, browser)
  }

  import googleImage from '@/assets/google.png'
  import githubImage from '@/assets/github.png'
  import oidcImage from '@/assets/oidc.png'
  import webauthImage from '@/assets/webauth.png'
  import defaultImage from '@/assets/oidc.png'
  import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'
  import ThemeSwitch from '@/components/ui/ThemeSwitch.vue'

  const providerImageMap = {
    google: googleImage,
    github: githubImage,
    oidc: oidcImage,
    // WebAuth: webauthImage,
    default: defaultImage,
  }

  const getProviderImage = (provider) => {
    return providerImageMap[provider.toLowerCase()] || providerImageMap.default
  }

  const allowRegister = ref(false)
  const disablePwd = ref(false)
  const loadLoginOptions = async () => {
    try {
      const res = await loginOptions().catch(_ => false)
      if (!res || !res.data) return console.error('No valid response received')
      res.data.ops.map(option => (options.push({ name: option }))) // 创建新的对象数组
      if (res.data.auto_oidc) {
        // 如果有自动OIDC登录选项，直接调用第一个
        handleOIDCLogin(res.data.ops[0])
      }
      disablePwd.value = res.data.disable_pwd
      allowRegister.value = res.data.register
      if (res.data.need_captcha) {
        loadCaptcha()
      }
    } catch (error) {
      console.error('Error loading login options:', error.message)
    }
  }

  onMounted(async () => {
    const code = getCode()
    if (code) {
      // 如果code存在，进行query获取user info
      const res = await userStore.query(code)
      if (res) {
        // 删除code，确保跳转之前对code进行清楚
        removeCode()
        ElMessage.success(T('LoginSuccess'))
        router.push({ path: redirect || '/dashboard', replace: true })
      }
    } else {
      // 如果code不存在, 现实登陆页面
      loadLoginOptions() // 组件挂载后调用登录选项加载函数
    }
  })

  const register = () => {
    router.push('/register')
  }
</script>

<style scoped lang="scss">
.login-container {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 420px;
  gap: 34px;
  align-items: center;
  min-height: 100vh;
  padding: clamp(20px, 5vw, 64px);
  background:
    radial-gradient(circle at 18% 14%, color-mix(in srgb, var(--color-primary) 22%, transparent), transparent 28rem),
    radial-gradient(circle at 84% 78%, color-mix(in srgb, var(--color-success) 12%, transparent), transparent 24rem),
    var(--color-bg);
}

.auth-theme {
  position: absolute;
  top: 20px;
  right: 20px;
}

.auth-visual {
  max-width: 680px;
  color: var(--color-text);
}

.visual-kicker {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  color: var(--color-muted);
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.auth-visual h1 {
  max-width: 640px;
  margin: 0;
  font-size: clamp(40px, 6vw, 72px);
  line-height: 0.95;
  letter-spacing: -0.06em;
}

.auth-visual p {
  max-width: 520px;
  margin: 20px 0 0;
  color: var(--color-muted);
  font-size: 16px;
  line-height: 1.7;
}

.signal-card {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  max-width: 520px;
  margin-top: 34px;

  div {
    padding: 16px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    background: color-mix(in srgb, var(--color-surface) 86%, transparent);
  }

  span,
  strong {
    display: block;
  }

  span {
    color: var(--color-muted);
    font-family: var(--font-mono);
    font-size: 11px;
    text-transform: uppercase;
  }

  strong {
    margin-top: 8px;
    color: var(--color-success);
    font-size: 18px;
  }
}

.login-card {
  width: 100%;
  max-width: 420px;
  padding: 34px;
  border: 1px solid var(--color-border);
  border-radius: 24px;
  background: color-mix(in srgb, var(--color-surface) 94%, transparent);
  box-shadow: var(--shadow-card);
  text-align: center;
  backdrop-filter: blur(18px);
}

h2 {
  margin: 0 0 24px;
  color: var(--color-text);
  font-size: 24px;
  font-weight: 700;
}

.login-form {
  margin-bottom: 20px;
}

.login-input {
  width: 100%;
  .captcha{
    cursor: pointer;
    width: 150px;
  }
}
.captcha-input{
  :deep(.el-input-group__append) {
    border-radius: 5px;
    padding: 0;
    overflow: hidden;
  }
}

.login-button {
  width: 100%;
  height: 40px;
  margin-bottom: 20px;
  margin-left: 0;
}

.divider {
  display: flex;
  align-items: center;
  margin: 20px 0;
  font-size: 14px;
  color: var(--color-muted);

  &::before,
  &::after {
    content: '';
    flex: 1;
    height: 1px;
    background-color: var(--color-border);
  }

  &::before {
    margin-right: 10px;
  }

  &::after {
    margin-left: 10px;
  }
}

.oidc-options {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.oidc-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  width: 100%;
  height: 50px;
  background-color: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  color: var(--color-text);
  font-size: 14px;
  box-shadow: 0 2px 4px var(--shadow-card);
}

.oidc-icon {
  width: 24px;
  height: 24px;
  margin-right: 10px;
}

.login-logo {
  width: 80px;
  height: 80px;
  margin: 0 auto 20px;
  display: block;
}

.el-form-item {
  ::v-deep(.el-form-item__label) {
    color: var(--color-text);
    font-weight: 600;
  }

  .el-input {
    ::v-deep(.el-input__wrapper) {
      border: 1px solid var(--color-border);
      background: var(--color-bg);
    }

    ::v-deep(input) {
      color: var(--color-text);
    }
  }
}

@media (max-width: 980px) {
  .login-container {
    grid-template-columns: 1fr;
  }

  .auth-visual {
    display: none;
  }

  .login-card {
    margin: 0 auto;
  }
}

@media (max-width: 520px) {
  .login-container {
    padding: 76px 16px 20px;
  }

  .login-card {
    padding: 24px;
  }
}
</style>
