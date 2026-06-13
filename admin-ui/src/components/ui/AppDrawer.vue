<template>
  <el-drawer
      :model-value="modelValue"
      @update:model-value="$emit('update:modelValue', $event)"
      :title="title"
      :size="size"
      :direction="direction"
      :close-on-click-modal="closeOnClickModal"
      :close-on-press-escape="closeOnPressEscape"
      :show-close="showClose"
      class="app-drawer"
      :class="{ 'app-drawer--danger': danger }"
      @open="$emit('open')"
      @close="$emit('close')"
  >
    <div class="app-drawer__body">
      <slot></slot>
    </div>
    <template v-if="!hideFooter" #footer>
      <div class="app-drawer__footer">
        <slot name="footer">
          <el-button @click="$emit('update:modelValue', false)">{{ cancelText }}</el-button>
          <el-button
              v-if="showConfirm"
              :type="danger ? 'danger' : 'primary'"
              :loading="loading"
              @click="$emit('confirm')"
          >
            {{ confirmText }}
          </el-button>
        </slot>
      </div>
    </template>
  </el-drawer>
</template>

<script setup>
defineProps({
  modelValue: {
    type: Boolean,
    required: true,
  },
  title: {
    type: String,
    default: '',
  },
  size: {
    type: [String, Number],
    default: '600',
  },
  direction: {
    type: String,
    default: 'rtl',
    validator: value => ['rtl', 'ltr', 'ttb', 'btt'].includes(value),
  },
  danger: {
    type: Boolean,
    default: false,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  confirmText: {
    type: String,
    default: 'Confirm',
  },
  cancelText: {
    type: String,
    default: 'Cancel',
  },
  showConfirm: {
    type: Boolean,
    default: true,
  },
  hideFooter: {
    type: Boolean,
    default: false,
  },
  closeOnClickModal: {
    type: Boolean,
    default: false,
  },
  closeOnPressEscape: {
    type: Boolean,
    default: true,
  },
  showClose: {
    type: Boolean,
    default: true,
  },
})

defineEmits([
  'update:modelValue',
  'confirm',
  'open',
  'close',
])
</script>

<style scoped lang="scss">
.app-drawer :deep(.el-drawer__header) {
  padding: 20px 24px 16px;
  margin-bottom: 0;
  border-bottom: 1px solid var(--color-border);
}

.app-drawer :deep(.el-drawer__title) {
  color: var(--color-text);
  font-size: 16px;
  font-weight: 700;
}

.app-drawer :deep(.el-drawer__body) {
  padding: 24px;
}

.app-drawer__body {
  height: calc(100vh - 140px);
  overflow-y: auto;
}

.app-drawer__footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 16px 24px;
  border-top: 1px solid var(--color-border);
}

.app-drawer--danger :deep(.el-drawer__title) {
  color: var(--color-danger);
}

.app-drawer--danger :deep(.el-drawer__header) {
  border-bottom-color: color-mix(in srgb, var(--color-danger) 20%, transparent);
}
</style>
