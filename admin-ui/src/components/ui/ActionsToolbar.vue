<template>
  <div class="actions-toolbar" :class="{ 'actions-toolbar--active': selected.length }">
    <span class="actions-toolbar__count">
      <template v-if="selected.length">
        <el-icon><Check /></el-icon>
        <strong>{{ selected.length }}</strong>
        <span class="actions-toolbar__count-label">{{ T('Selected') }}</span>
      </template>
      <template v-else>
        <span class="actions-toolbar__hint">{{ T('SelectRowsToAction') }}</span>
      </template>
    </span>
    <div class="actions-toolbar__buttons">
      <slot :selected="selected" :disabled="!selected.length"></slot>
    </div>
  </div>
</template>

<script setup>
  import { Check } from '@element-plus/icons-vue'
  import { T } from '@/utils/i18n'

  defineProps({
    selected: {
      type: Array,
      default: () => [],
    },
  })
</script>

<style scoped lang="scss">
.actions-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 14px;
  margin-bottom: 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  transition: border-color 0.15s ease, background 0.15s ease;
}

.actions-toolbar--active {
  border-color: color-mix(in srgb, var(--color-primary, #409eff) 45%, var(--color-border));
  background: color-mix(in srgb, var(--color-primary, #409eff) 6%, var(--color-surface));
}

.actions-toolbar__count {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--color-muted);
  font-size: 13px;
}

.actions-toolbar__count strong {
  color: var(--color-text);
  font-weight: 600;
}

.actions-toolbar__count-label {
  margin-left: 2px;
}

.actions-toolbar__hint {
  font-style: italic;
}

.actions-toolbar__buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
</style>
