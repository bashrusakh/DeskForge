<template>
  <el-config-provider :locale="appStore.setting.locale.value">
    <el-container class="app-shell" :style="{'--sideBarWidth': sideBarWidth}">
      <el-aside :width="leftWidth" class="app-left">
        <g-aside></g-aside>
      </el-aside>
      <el-drawer v-model="appStore.setting.mobileMenuOpen" class="mobile-nav-drawer" direction="ltr" size="280px" :with-header="false">
        <g-aside></g-aside>
      </el-drawer>
      <el-container class="app-container ">
        <el-header class="app-header">
          <g-header></g-header>
        </el-header>

        <el-main class="app-main">
          <router-view v-slot="{ Component }">
            <transition mode="out-in" name="el-fade-in-linear">
              <keep-alive :include="cachedTags">
                <component :is="Component"/>
              </keep-alive>
            </transition>
          </router-view>
        </el-main>
      </el-container>
    </el-container>
  </el-config-provider>
</template>

<script setup>
  import { useAppStore } from '@/store/app'
  import { useTagsStore } from '@/store/tags'
  import { ref, computed, watch } from 'vue'
  import { useRoute } from 'vue-router'
  import GAside from '@/layout/components/aside.vue'
  import GHeader from '@/layout/components/header.vue'

  const appStore = useAppStore()
  const tagStore = useTagsStore()
  const route = useRoute()
  const sideBarWidth = computed(() => appStore.setting.locale.sideBarWidth)
  const leftWidth = computed(() => {
    if (appStore.setting.isMobile) {
      return '0'
    }
    return appStore.setting.sideIsCollapse ? '64px' : 'var(--sideBarWidth)'
  })

  const cachedTags = ref([])

  cachedTags.value = tagStore.cached

  watch(() => route.fullPath, () => {
    appStore.closeMobileMenu()
  })
</script>

<style lang="scss" scoped>
.app-shell {
  min-height: 100vh;
  background: transparent;
}

.app-header {
  position: sticky;
  top: 0;
  z-index: 20;
  height: 64px;
  border-bottom: 1px solid var(--color-border);
  background: color-mix(in srgb, var(--color-bg) 84%, transparent);
  color: var(--color-text);
  backdrop-filter: blur(18px);
  display: flex;
  overflow: hidden;
}

.app-left {
  transition: width 0.5s;
  border-right: 1px solid var(--color-border);
  background: var(--color-sidebar);
}

.app-container {
  min-height: 100vh;
}

.app-main {
  padding: var(--spacing-page);
  background: transparent;
}

@media (max-width: 768px) {
  .app-left {
    display: none;
  }

  .app-header {
    height: 58px;
  }

  .app-main {
    padding: 16px;
  }
}

:global(.mobile-nav-drawer .el-drawer__body) {
  padding: 0;
  background: var(--color-sidebar);
}
</style>
