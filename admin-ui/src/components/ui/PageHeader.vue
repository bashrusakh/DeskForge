<template>
  <section class="page-header">
    <div class="page-header__copy">
      <div v-if="eyebrow" class="page-header__eyebrow">
        <connection-pulse v-if="pulse" :status="pulse" :animated="pulse === 'online'" />
        {{ eyebrow }}
      </div>
      <h1>{{ title }}</h1>
      <p v-if="subtitle">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="page-header__actions">
      <slot name="actions"></slot>
    </div>
  </section>
</template>

<script setup>
  import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'

  defineProps({
    title: {
      type: String,
      required: true,
    },
    subtitle: {
      type: String,
      default: '',
    },
    eyebrow: {
      type: String,
      default: '',
    },
    pulse: {
      type: String,
      default: '',
    },
  })
</script>

<style scoped lang="scss">
.page-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 18px;
}

.page-header__copy {
  min-width: 0;
}

.page-header__eyebrow {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  color: var(--color-muted);
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

h1 {
  margin: 0;
  color: var(--color-text);
  font-size: clamp(24px, 4vw, 34px);
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.04em;
}

p {
  max-width: 720px;
  margin: 10px 0 0;
  color: var(--color-muted);
  font-size: 14px;
  line-height: 1.6;
}

.page-header__actions {
  display: flex;
  flex: 0 0 auto;
  gap: 10px;
}

@media (max-width: 720px) {
  .page-header {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
