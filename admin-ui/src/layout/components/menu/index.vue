<template>
  <el-menu
           class="menus"
           :collapse="isCollapse"
           :default-active="activeIndex"
           router
  >
    <menu-item v-for="(route,index) in routes" :key="route.name" :route="route"></menu-item>
  </el-menu>
</template>

<script>
  import { defineComponent, ref, onMounted, watch, computed } from 'vue'
  import { useRouteStore } from '@/store/router'
  import MenuItem from '@/layout/components/menu/item.vue'
  import { useRoute } from 'vue-router'
  import { useAppStore } from '@/store/app'

  export default defineComponent({
    name: 'Menu',
    created () {
    },
    components: { MenuItem },
    setup () {
      const routes = ref([])
      const route = useRoute()
      const app = useAppStore()
      const isCollapse = computed(() => app.setting.sideIsCollapse)
      const activeIndex = computed(() => route.name)

      routes.value = useRouteStore().routes
      return {
        routes,
        activeIndex,
        isCollapse,
      }
    },

  })
</script>

<style lang="scss" scoped>
  .menus {
    min-height: calc(100vh - var(--sidebar-brand-height));
    border-right: none;
    background: var(--color-sidebar);
    --el-menu-bg-color: var(--color-sidebar);
    --el-menu-text-color: var(--color-sidebar-text);
    --el-menu-active-color: var(--color-primary);
    --el-menu-hover-bg-color: var(--color-sidebar-hover);

    &:not(.el-menu--collapse) {
      width: var(--sideBarWidth);
    }

    :deep(.el-menu-item),
    :deep(.el-sub-menu__title) {
      height: 44px;
      margin: 4px 10px;
      border-radius: 12px;
      font-weight: 600;
    }

    :deep(.el-menu-item.is-active) {
      background: var(--color-primary-soft);
    }

  }
</style>
<style>
</style>
