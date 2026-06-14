<template>
  <div class="header-main">
    <button class="sidebar-toggle" type="button" @click="toggleNavigation" :aria-label="setting.sideIsCollapse ? 'Expand menu' : 'Collapse menu'">
      <el-icon>
        <el-icon-expand v-if="setting.sideIsCollapse"></el-icon-expand>
        <el-icon-fold v-else></el-icon-fold>
      </el-icon>
    </button>
    <div class="header-title">
      <div class="title">{{ T(route.meta?.title) || setting.title }}</div>
    </div>
  </div>
  <div class="header-right">
    <div class="header-status">
      <connection-pulse status="online" />
      <span>Remote access console</span>
    </div>
    <Setting></Setting>
  </div>
</template>

<script>
  import { defineComponent, computed } from 'vue'
  import HeaderMenu from '@/layout/components/menu/index.vue'
  import Setting from '@/layout/components/setting/index.vue'
  import { useAppStore } from '@/store/app'
  import GTags from '@/layout/components/tags/index.vue'
  import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'
  import { useRoute } from 'vue-router'
  import { T } from '@/utils/i18n'

  export default defineComponent({
    name: 'LayerHeader',
    created () {
    },
    components: { HeaderMenu, Setting, GTags, ConnectionPulse },
    watch: {},
    setup (props) {
      const appStore = useAppStore()
      const route = useRoute()
      const setting = computed(() => appStore.setting)
      const toggleNavigation = () => {
        appStore.toggleNavigation()
      }
      return {
        setting,
        route,
        toggleNavigation,
        T,
      }
    },

  })
</script>

<style scoped lang="scss">
.header-main {
  display: flex;
  align-items: center;
  min-width: 0;
}

.sidebar-toggle {
  width: 40px;
  height: 40px;
  border: 1px solid var(--color-border);
  border-radius: 12px;
  background: var(--color-surface);
  color: var(--color-text);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-right: 14px;
  font-size: 18px;
  cursor: pointer;
  transition: border-color 0.2s ease, color 0.2s ease, transform 0.2s ease;

  &:hover {
    border-color: var(--color-primary);
    color: var(--color-primary);
    transform: translateY(-1px);
  }
}

.header-title {
  min-width: 0;

  .title {
    color: var(--color-text);
    font-size: 20px;
    font-weight: 700;
    line-height: 1.2;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.header-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-status {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--color-muted);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  white-space: nowrap;
}

@media (max-width: 720px) {
  .sidebar-toggle {
    margin-right: 8px;
  }

  .header-title .title {
    font-size: 16px;
  }

  .header-right {
    gap: 6px;
  }
}

@media (max-width: 480px) {
  .header-status span {
    display: none;
  }

  .header-status {
    gap: 0;
  }
}


</style>
<style lang="scss">

</style>
