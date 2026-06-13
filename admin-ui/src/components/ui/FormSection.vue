<template>
  <div class="form-section">
    <div v-if="title || subtitle || $slots.header" class="form-section__header">
      <div v-if="title || subtitle">
        <div v-if="title" class="form-section__title">{{ title }}</div>
        <div v-if="subtitle" class="form-section__subtitle">{{ subtitle }}</div>
      </div>
      <div v-if="$slots.header" class="form-section__header-slot">
        <slot name="header"></slot>
      </div>
    </div>
    <div class="form-section__body">
      <slot></slot>
    </div>
  </div>
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
.form-section {
  padding: 20px 0;
  border-bottom: 1px solid var(--color-border);
}

.form-section:last-child {
  border-bottom: none;
}

.form-section__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}

.form-section__title {
  color: var(--color-text);
  font-size: 15px;
  font-weight: 700;
}

.form-section__subtitle {
  margin-top: 4px;
  color: var(--color-muted);
  font-size: 13px;
}

.form-section__body {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

@media (max-width: 720px) {
  .form-section__header {
    flex-direction: column;
    gap: 8px;
  }
}
</style>
