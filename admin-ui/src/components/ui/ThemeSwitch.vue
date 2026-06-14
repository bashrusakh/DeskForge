<template>
  <el-radio-group v-model="themeMode" size="small" class="theme-switch">
    <el-radio-button value="auto">Auto</el-radio-button>
    <el-radio-button value="light">Light</el-radio-button>
    <el-radio-button value="dark">Dark</el-radio-button>
  </el-radio-group>
</template>

<script setup>
  import { computed } from 'vue'
  import { useAppStore } from '@/store/app'

  const appStore = useAppStore()
  const themeMode = computed({
    get: () => appStore.setting.themeMode,
    set: (mode) => appStore.setThemeMode(mode),
  })
</script>

<style scoped lang="scss">
.theme-switch {
  padding: 3px;
  border: 1px solid var(--color-border);
  border-radius: 12px;
  background: var(--color-surface);

  :deep(.el-radio-button__inner) {
    border: 0;
    border-radius: 9px;
    background: transparent;
    color: var(--color-muted);
    box-shadow: none;
    font-weight: 600;
  }

  :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
    background: var(--color-primary-soft);
    color: var(--color-primary);
  }
}

@media (max-width: 860px) {
  .theme-switch {
    :deep(.el-radio-button__inner) {
      padding-left: 8px;
      padding-right: 8px;
    }
  }
}
</style>
