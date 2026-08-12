import { defineStore, acceptHMRUpdate } from 'pinia'
import { current, login, logout as logoutRequest } from '@/api/user'
import { getToken, setToken, removeToken, setCode, removeCode } from '@/utils/auth'
import { useRouteStore } from '@/store/router'
import { useAppStore } from '@/store/app'
import { oidcAuth, oidcQuery } from '@/api/login'

let pendingLogout = null

export const useUserStore = defineStore({
  id: 'user',
  state: () => ({
    nickname: '',
    username: '',
    email: '',
    token: '',
    role: '',
    avatar: '',
    route_names: [],
  }),

  actions: {
    async logout () {
      if (pendingLogout) {
        return pendingLogout
      }

      const token = this.token || getToken()
      pendingLogout = (async () => {
        let revoked = true
        try {
          if (token) {
            await logoutRequest()
          }
        } catch (_) {
          // Local cleanup is the fallback when the server cannot revoke the session.
          revoked = false
        } finally {
          removeToken()
          removeCode()
          localStorage.removeItem('user_info')
          this.$reset()
          pendingLogout = null
        }
        return revoked
      })()

      return pendingLogout
    },

    saveUserData (userData) {
      // useAppStore().getAppConfig()
      setToken(userData.token)
      //
      localStorage.setItem('user_info', JSON.stringify({ name: userData.username }))
      this.$patch({
        ...userData,
      })
      if (userData.route_names && userData.route_names.length) {
        useRouteStore().addRoutes(userData.route_names)
      }
    },

    async login (form) {
      const res = await login(form).catch(e => e)
      if (!res.code) {
        useAppStore().loadConfig()
        const userData = res.data
        this.saveUserData(userData)
        return userData
      } else {
        return Promise.reject(res)
      }
    },
    async info () {
      const res = await current().catch(_ => false)
      if (res) {
        useAppStore().loadConfig()
        const userData = res.data
        setToken(userData.token)
        this.$patch({
          ...userData,
        })
        useRouteStore().addRoutes(userData.route_names)
        return userData
      }
      return false
    },
    async oidc (provider, platform, browser) {
      // oidc data need to be implement
      const data = {
        deviceInfo: {
          name: navigator.userAgent, // 使用浏览器的 User-Agent 作为设备名
          os: platform, // 获取操作系统信息
          type: 'webadmin', // any vaule
        },
        id: `${platform}-${browser}`,
        op: provider, // 传入的 provider
        uuid: '',//crypto.randomUUID(), // 自动生成 UUID
      }
      const res = await oidcAuth(data).catch(_ => false)
      if (res) {
        const { code, url } = res.data
        setCode(code)
        if (provider == 'webauth') {
          window.open(url)
        } else {
          window.location.href = url
        }
      }
    },
    async query (code) {
      const params = { 'code': code, uuid: '' }
      const res = await oidcQuery(params).catch(_ => false)
      if (res) {
        removeCode()
        useAppStore().loadConfig()
        const userData = res.data
        this.saveUserData(userData)
        return userData
      }
      return false
    },
  },
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useUserStore, import.meta.hot))
}
