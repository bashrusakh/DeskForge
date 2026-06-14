<template>
  <el-card class="page-section" shadow="hover">
    <div v-if="title || subtitle || $slots.actions" class="page-section__header">
      <div>
        <div v-if="title" class="page-section__title">{{ title }}</div>
        <div v-if="subtitle" class="page-section__subtitle">{{ subtitle }}</div>
      </div>
      <div v-if="$slots.actions" class="page-section__actions">
        <slot name="actions"></slot>
      </div>
    </div>
    <slot></slot>
  </el-card>
</template>

<script setup>
  defineProps({
    title: {
      type: String,
      default: '',
    },
    subtitle: {
      type: String,
      default: '',
    },
  })
</script>

<style scoped lang="scss">
.page-section {
  border-radius: var(--radius-lg);
}

.page-section__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.page-section__title {
  color: var(--color-text);
  font-size: 16px;
  font-weight: 700;
}

.page-section__subtitle {
  margin-top: 4px;
  color: var(--color-muted);
  font-size: 13px;
}

.page-section__actions {
  display: flex;
  flex: 0 0 auto;
  gap: 8px;
}

@media (max-width: 720px) {
  .page-section__header {
    flex-direction: column;
  }
}
</style>
