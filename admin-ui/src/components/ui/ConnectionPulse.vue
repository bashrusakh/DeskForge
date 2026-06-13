<template>
  <span class="connection-pulse" :class="[`is-${status}`, { 'is-animated': animated }]" aria-hidden="true">
    <span class="connection-pulse__core"></span>
  </span>
</template>

<script setup>
  defineProps({
    status: {
      type: String,
      default: 'online',
    },
    animated: {
      type: Boolean,
      default: true,
    },
  })
</script>

<style scoped lang="scss">
.connection-pulse {
  --pulse-color: var(--color-success);
  position: relative;
  display: inline-flex;
  width: 12px;
  height: 12px;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;

  &::before {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 999px;
    background: color-mix(in srgb, var(--pulse-color) 24%, transparent);
    transform: scale(1);
  }

  &.is-animated::before {
    animation: connection-pulse 1.8s ease-out infinite;
  }

  &.is-offline {
    --pulse-color: var(--color-muted);
  }

  &.is-warning {
    --pulse-color: var(--color-warning);
  }

  &.is-danger {
    --pulse-color: var(--color-danger);
  }
}

.connection-pulse__core {
  position: relative;
  z-index: 1;
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: var(--pulse-color);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--pulse-color) 12%, transparent);
}

@keyframes connection-pulse {
  0% {
    opacity: 0.8;
    transform: scale(0.8);
  }
  72%, 100% {
    opacity: 0;
    transform: scale(2.4);
  }
}

@media (prefers-reduced-motion: reduce) {
  .connection-pulse.is-animated::before {
    animation: none;
  }
}
</style>
