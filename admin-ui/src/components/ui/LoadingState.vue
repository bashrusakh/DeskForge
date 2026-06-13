<template>
  <div class="loading-state">
    <el-icon class="loading-state__icon is-loading"><Loading /></el-icon>
    <div class="loading-state__copy">
      <div class="loading-state__title">{{ title }}</div>
      <div v-if="description" class="loading-state__description">{{ description }}</div>
    </div>
  </div>
</template>

<script setup>
  import { Loading } from '@element-plus/icons-vue'

  defineProps({
    title: {
      type: String,
      default: 'Loading',
    },
    description: {
      type: String,
      default: '',
    },
  })
</script>

<style scoped lang="scss">
.loading-state {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--color-border);
  border-radius: 14px;
  background: var(--color-surface);
  color: var(--color-text);
}

.loading-state__icon {
  color: var(--color-primary);
  font-size: 22px;
}

.loading-state__title {
  font-weight: 700;
}

.loading-state__description {
  margin-top: 2px;
  color: var(--color-muted);
  font-size: 12px;
}
</style>
