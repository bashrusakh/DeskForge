import { defineStore, acceptHMRUpdate } from 'pinia'
import logo from '@/assets/logo.png'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'
import ru from 'element-plus/es/locale/lang/ru'
import { admin, app, server } from '@/api/config'

const SIDEBAR_WIDTH = '240px'
const langs = {
  'zh-CN': { name: '中文', value: zhCn, sideBarWidth: SIDEBAR_WIDTH },
  'en': { name: 'English', value: en, sideBarWidth: SIDEBAR_WIDTH },
  'ru': { name: 'Русский', value: ru, sideBarWidth: SIDEBAR_WIDTH },
}
const defaultLang = localStorage.getItem('lang') || 'en'
const defaultThemeMode = localStorage.getItem('theme-mode') || 'auto'
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
const mobileViewport = window.matchMedia('(max-width: 768px)')

function resolveTheme (mode) {
  return mode === 'auto' ? (systemTheme.matches ? 'dark' : 'light') : mode
}

function applyTheme (mode) {
  const theme = resolveTheme(mode)
  document.documentElement.dataset.theme = mode
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

export const useAppStore = defineStore({
  id: 'App',
  state: () => ({
    setting: {
      title: 'DeskForge Admin',
      hello: '',
      sideIsCollapse: false,
      isMobile: mobileViewport.matches,
      mobileMenuOpen: false,
      themeMode: defaultThemeMode,
      logo,
      langs: langs,
      lang: defaultLang,
      locale: langs[defaultLang] ? langs[defaultLang] : langs['en'],
      appConfig: {
        web_client: 1,
      },
      rustdeskConfig: {
        'id_server': '',
        'key': '',
        'relay_server': '',
        'api_server': '',
      },
    },
  }),

  actions: {
    initViewport () {
      this.setting.isMobile = mobileViewport.matches
      mobileViewport.addEventListener('change', (event) => {
        this.setting.isMobile = event.matches
        if (!event.matches) {
          this.setting.mobileMenuOpen = false
        }
      })
    },
    initTheme () {
      applyTheme(this.setting.themeMode)
      systemTheme.addEventListener('change', () => {
        if (this.setting.themeMode === 'auto') {
          applyTheme('auto')
        }
      })
    },
    setThemeMode (mode) {
      this.setting.themeMode = mode
      localStorage.setItem('theme-mode', mode)
      applyTheme(mode)
    },
    sideCollapse () {
      this.setting.sideIsCollapse = !this.setting.sideIsCollapse
    },
    toggleNavigation () {
      if (this.setting.isMobile) {
        this.setting.mobileMenuOpen = !this.setting.mobileMenuOpen
      } else {
        this.sideCollapse()
      }
    },
    closeMobileMenu () {
      this.setting.mobileMenuOpen = false
    },
    setLang (lang) {
      console.log('setLang', lang)
      this.setting.lang = lang
      this.setting.locale = langs[lang]
      localStorage.setItem('lang', lang)
    },
    changeLang (v) {
      this.setLang(v)
    },
    loadConfig () {
      this.getAppConfig()
      this.getAdminConfig()
      this.loadRustdeskConfig()
    },
    getAppConfig () {
      console.log('getAppConfig')
      return app().then(res => {
        this.setting.appConfig = res.data
      })
    },
    getAdminConfig () {
      return admin().then(res => {
        this.replaceAdminTitle(res.data.title)
        this.setting.hello = res.data.hello
      }).catch(err => {
        console.warn('Failed to load admin config', err)
      })
    },
    replaceAdminTitle (newTitle) {
      document.title = document.title.replace(`- ${this.setting.title}`, `- ${newTitle}`)
      this.setting.title = newTitle
    },
    async loadRustdeskConfig () {
      const res = await server().catch(_ => false)
      if (!res || !res.data) return
      this.setting.rustdeskConfig = res.data
      const prefix = 'wc-'
      if (res.data.id_server) localStorage.setItem(`${prefix}custom-rendezvous-server`, res.data.id_server)
      if (res.data.key) localStorage.setItem(`${prefix}key`, res.data.key)
      if (res.data.api_server) localStorage.setItem(`${prefix}api-server`, res.data.api_server)
    },
  },
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useAppStore, import.meta.hot))
}
