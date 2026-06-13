<template>
  <div class="sidebar-shell" :class="{ 'is-collapsed': setting.sideIsCollapse }">
    <div class="sidebar-brand">
      <div class="brand-mark">
        <connection-pulse status="online" />
      </div>
      <div class="brand-copy">
        <div class="brand-title">{{ setting.title }}</div>
        <div class="brand-subtitle">ID / Relay / API</div>
      </div>
    </div>
    <el-scrollbar class="scroll-sidebar" height="calc(100vh - var(--sidebar-brand-height))">
      <menus></menus>
    </el-scrollbar>
  </div>
</template>
<script>
  import Menus from '@/layout/components/menu/index.vue'
  import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'
  import { defineComponent, computed } from 'vue'
  import { useAppStore } from '@/store/app'

  export default defineComponent({
    name: 'GAside',
    components: { Menus, ConnectionPulse },
    setup () {
      const appStore = useAppStore()
      const setting = computed(() => appStore.setting)

      return {
        setting,
      }
    },
  })
</script>

<style scoped lang="scss">
.sidebar-shell {
  min-height: 100vh;
  background: var(--color-sidebar);
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  height: var(--sidebar-brand-height);
  padding: 18px 18px 14px;
  border-bottom: 1px solid var(--color-border);
}

.brand-mark {
  width: 34px;
  height: 34px;
  border-radius: 12px;
  background: var(--color-primary-soft);
  display: flex;
  align-items: center;
  justify-content: center;
}

.brand-copy {
  min-width: 0;
}

.brand-title {
  color: var(--color-text);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brand-subtitle {
  margin-top: 4px;
  color: var(--color-muted);
  font-family: var(--font-mono);
  font-size: 11px;
}

.is-collapsed {
  .sidebar-brand {
    justify-content: center;
    padding-left: 0;
    padding-right: 0;
  }

  .brand-copy {
    display: none;
  }
}

.scroll-sidebar {
  background: var(--color-sidebar);
}
</style>
