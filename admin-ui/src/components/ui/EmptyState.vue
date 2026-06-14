<template>
  <div class="empty-state">
    <div class="empty-state__icon">
      <slot name="icon">
        <el-icon><FolderOpened /></el-icon>
      </slot>
    </div>
    <div class="empty-state__title">{{ title }}</div>
    <div v-if="description" class="empty-state__description">{{ description }}</div>
    <div v-if="$slots.actions" class="empty-state__actions">
      <slot name="actions"></slot>
    </div>
  </div>
</template>

<script setup>
  import { FolderOpened } from '@element-plus/icons-vue'

  defineProps({
    title: {
      type: String,
      default: 'Nothing here yet',
    },
    description: {
      type: String,
      default: '',
    },
  })
</script>

<style scoped lang="scss">
.empty-state {
  display: flex;
  min-height: 260px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  padding: 40px 24px;
  border: 1px dashed var(--color-border);
  border-radius: var(--radius-lg);
  background:
    radial-gradient(circle at 50% 0, color-mix(in srgb, var(--color-primary) 10%, transparent), transparent 34%),
    var(--color-surface);
  text-align: center;
}

.empty-state__icon {
  display: inline-flex;
  width: 54px;
  height: 54px;
  align-items: center;
  justify-content: center;
  border-radius: 18px;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-size: 26px;
}

.empty-state__title {
  margin-top: 18px;
  color: var(--color-text);
  font-size: 20px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.empty-state__description {
  max-width: 520px;
  margin-top: 8px;
  color: var(--color-muted);
  font-size: 14px;
  line-height: 1.6;
}

.empty-state__actions {
  display: flex;
  justify-content: center;
  gap: 10px;
  margin-top: 22px;
}
</style>
