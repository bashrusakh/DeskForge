<template>
  <el-card class="danger-zone" shadow="hover">
    <div class="danger-zone__header">
      <div>
        <div class="danger-zone__eyebrow">Danger Zone</div>
        <div class="danger-zone__title">{{ title }}</div>
        <div v-if="subtitle" class="danger-zone__subtitle">{{ subtitle }}</div>
      </div>
      <slot name="actions"></slot>
    </div>
    <slot></slot>
  </el-card>
</template>

<script setup>
  defineProps({
    title: {
      type: String,
      default: 'Dangerous actions',
    },
    subtitle: {
      type: String,
      default: '',
    },
  })
</script>

<style scoped lang="scss">
.danger-zone {
  border-color: color-mix(in srgb, var(--color-danger) 34%, var(--color-border));
  border-radius: var(--radius-lg);
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--color-danger) 8%, transparent), transparent 46%),
    var(--color-surface);
}

.danger-zone__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.danger-zone__eyebrow {
  color: var(--color-danger);
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.danger-zone__title {
  margin-top: 6px;
  color: var(--color-text);
  font-size: 18px;
  font-weight: 700;
}

.danger-zone__subtitle {
  max-width: 760px;
  margin-top: 6px;
  color: var(--color-muted);
  font-size: 13px;
  line-height: 1.6;
}

@media (max-width: 720px) {
  .danger-zone__header {
    flex-direction: column;
  }
}
</style>
