<template>
  <div class="login-container">
    <theme-switch class="auth-theme" />
    <section class="auth-visual">
      <div class="visual-kicker"><connection-pulse status="online" /> Account provisioning</div>
      <h1>Create an admin-ready account.</h1>
      <p>Registration joins the same operational console used for devices, access rules, monitoring, and server safety controls.</p>
      <div class="signal-card">
        <div><span>Identity</span><strong>local</strong></div>
        <div><span>Access</span><strong>scoped</strong></div>
        <div><span>Audit</span><strong>logged</strong></div>
      </div>
    </section>

    <div class="login-card">
      <img src="@/assets/logo.png" alt="logo" class="login-logo"/>
      <h2>{{ T('Register') }}</h2>
      <el-form ref="f" :model="form" label-position="top" class="login-form" :rules="rules">
        <el-form-item :label="T('Username')" prop="username">
          <el-input v-model="form.username" class="login-input"></el-input>
        </el-form-item>

        <el-form-item :label="T('Email')" prop="email">
          <el-input v-model="form.email" class="login-input"></el-input>
        </el-form-item>

        <el-form-item :label="T('Password')" prop="password">
          <el-input v-model="form.password" type="password" show-password
                    class="login-input"></el-input>
        </el-form-item>
        <el-form-item :label="T('ConfirmPassword')" prop="confirm_password">
          <el-input v-model="form.confirm_password" type="password" @keyup.enter.native="submit" show-password
                    class="login-input"></el-input>
        </el-form-item>
        <el-form-item label="">
          <el-button @click="submit" class="login-button" type="success">{{ T('Submit') }}</el-button>
          <el-button @click="toLogin" class="login-button">{{ T('ToLogin') }}</el-button>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<script setup>
  import { reactive, ref } from 'vue'
  import { ElMessage } from 'element-plus'
  import { T } from '@/utils/i18n'
  import { useRoute, useRouter } from 'vue-router'
  import { register } from '@/api/user'
  import { useUserStore } from '@/store/user'
  import { useAppStore } from '@/store/app'
  import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'
  import ThemeSwitch from '@/components/ui/ThemeSwitch.vue'

  const router = useRouter()
  const userStore = useUserStore()
  const form = reactive({
    username: '',
    email: '',
    password: '',
    confirm_password: '',
  })
  const rules = {
    username: [
      { required: true, message: T('ParamRequired', { param: T('Username') }), trigger: 'blur' },
    ],
    // email: [
    //   { required: true, message: T('ParamRequired', { param: T('Email') }), trigger: 'blur' },
    // ],
    password: [
      { required: true, message: T('ParamRequired', { param: T('Password') }), trigger: 'blur' },
    ],
    confirm_password: [
      { required: true, message: T('ParamRequired', { param: T('ConfirmPassword') }), trigger: 'blur' },
      {
        validator: (rule, value, callback) => {
          if (value !== form.password) {
            callback(new Error(T('PasswordNotMatchConfirmPassword')))
          } else {
            callback()
          }
        }, trigger: 'blur',
      },
    ],
  }
  const f = ref(null)
  const submit = async () => {
    const v = await f.value.validate().catch(_ => false)
    if (!v) {
      return
    }
    const res = await register(form).catch(_ => false)
    if (!res) {
      return
    }
    userStore.saveUserData(res.data)
    useAppStore().loadConfig()
    ElMessage.success('Submit')
    router.push('/')
  }
  const toLogin = () => {
    router.push('/login')

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
}

.login-button {
  width: 100%;
  height: 40px;
  margin-bottom: 20px;
  margin-top: 20px;
  margin-left: 0;
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
