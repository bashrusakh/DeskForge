<template>
  <el-dialog
      :model-value="modelValue"
      @update:model-value="$emit('update:modelValue', $event)"
      :title="title"
      :width="width"
      :close-on-click-modal="closeOnClickModal"
      :close-on-press-escape="closeOnPressEscape"
      :show-close="showClose"
      :destroy-on-close="destroyOnClose"
      class="app-dialog"
      :class="{ 'app-dialog--danger': danger }"
      @open="$emit('open')"
      @close="$emit('close')"
  >
    <div class="app-dialog__body">
      <slot></slot>
    </div>
    <template v-if="!hideFooter" #footer>
      <div class="app-dialog__footer">
        <slot name="footer">
          <el-button @click="$emit('update:modelValue', false)">{{ cancelText || T('Cancel') }}</el-button>
          <el-button
              v-if="showConfirm"
              :type="danger ? 'danger' : 'primary'"
              :loading="loading"
              @click="$emit('confirm')"
          >
            {{ confirmText || T('Confirm') }}
          </el-button>
        </slot>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { T } from '@/utils/i18n'

defineProps({
  modelValue: {
    type: Boolean,
    required: true,
  },
  title: {
    type: String,
    default: '',
  },
  width: {
    type: [String, Number],
    default: '600',
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
    default: undefined,
  },
  cancelText: {
    type: String,
    default: undefined,
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
  destroyOnClose: {
    type: Boolean,
    default: false,
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
.app-dialog {
  border-radius: var(--radius-lg);
}

.app-dialog :deep(.el-dialog__header) {
  padding: 20px 24px 16px;
  margin-right: 0;
  border-bottom: 1px solid var(--color-border);
}

.app-dialog :deep(.el-dialog__title) {
  color: var(--color-text);
  font-size: 16px;
  font-weight: 700;
}

.app-dialog :deep(.el-dialog__headerbtn) {
  top: 20px;
  right: 20px;
}

.app-dialog :deep(.el-dialog__body) {
  padding: 24px;
}

.app-dialog__body {
  max-height: 60vh;
  overflow-y: auto;
}

.app-dialog__footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 16px 24px;
  border-top: 1px solid var(--color-border);
}

.app-dialog--danger :deep(.el-dialog__title) {
  color: var(--color-danger);
}

.app-dialog--danger :deep(.el-dialog__header) {
  border-bottom-color: color-mix(in srgb, var(--color-danger) 20%, transparent);
}
</style>
