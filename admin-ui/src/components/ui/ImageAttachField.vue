<template>
  <div class="image-attach-field">
    <div class="image-attach-field__preview" :aria-label="modelValue && !previewFailed ? undefined : previewEmptyLabel">
      <img
        v-if="modelValue && !previewFailed"
        :src="modelValue"
        alt=""
        class="image-attach-field__image"
        @error="previewFailed = true"
      >
      <span v-else class="image-attach-field__placeholder">
        <el-icon aria-hidden="true"><Picture /></el-icon>
      </span>
      <button
        type="button"
        class="image-attach-field__clear"
        :disabled="!modelValue"
        @click="clear"
      >
        <el-icon aria-hidden="true"><Close /></el-icon>{{ clearLabel }}
      </button>
    </div>
    <div class="image-attach-field__main">
      <el-upload :show-file-list="false" :auto-upload="true" :http-request="uploadRequest" accept="image/png">
        <el-button>{{ fileButtonLabel }}</el-button>
      </el-upload>
      <el-input
        :model-value="modelValue"
        :placeholder="linkPlaceholder"
        clearable
        :id="inputId"
        :aria-required="required"
        :aria-invalid="invalid"
        :aria-describedby="describedBy || undefined"
        :validate-event="false"
        @update:model-value="onInputValue"
        @input="onInputEvent"
      >
        <template v-if="rangeHint" #suffix>
          <el-tooltip :content="rangeHint" placement="top" popper-class="image-attach-field-hint-popper">
            <button type="button" class="image-attach-field__hint" :aria-label="rangeHint">
              <el-icon aria-hidden="true"><InfoFilled /></el-icon>
            </button>
          </el-tooltip>
        </template>
      </el-input>
      <div class="image-attach-field__note">{{ noteText }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { Close, InfoFilled, Picture } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  inputId: { type: String, default: '' },
  required: { type: Boolean, default: false },
  invalid: { type: Boolean, default: false },
  describedBy: { type: String, default: '' },
  noteText: { type: String, default: '' },
  fileButtonLabel: { type: String, default: '' },
  linkPlaceholder: { type: String, default: '' },
  clearLabel: { type: String, default: '' },
  previewEmptyLabel: { type: String, default: '' },
  // Allowed-range hover text (variants/limits). Required format lives in the
  // permanent note below the input; mechanics live in the section note.
  rangeHint: { type: String, default: '' },
  // Upload submission stays in the owning view: the file flows back through
  // this handler into the existing guard/inline-error pipeline untouched.
  uploadRequest: { type: Function, default: () => {} },
})

const emit = defineEmits(['update:modelValue', 'input'])

// A broken preview URL degrades to the neutral placeholder; a new value
// (typed or uploaded) retries the preview automatically.
const previewFailed = ref(false)
watch(() => props.modelValue, () => { previewFailed.value = false })

const clear = () => {
  if (!props.modelValue) return
  previewFailed.value = false
  emit('update:modelValue', '')
  // The plain-input contract: any value change also clears an active
  // field-level upload error in the owning view.
  emit('input', '')
}
const onInputValue = (value) => {
  previewFailed.value = false
  emit('update:modelValue', value)
}
const onInputEvent = (value) => {
  emit('input', value)
}
</script>

<style scoped lang="scss">
:global(.image-attach-field-hint-popper) {
  max-width: 320px;
}

.image-attach-field {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  width: 100%;
}

.image-attach-field__preview {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  min-width: 64px;
}

.image-attach-field__image,
.image-attach-field__placeholder {
  width: 64px;
  height: 64px;
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
  background: var(--el-fill-color-light);
}

.image-attach-field__image {
  object-fit: contain;
}

.image-attach-field__placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);
  font-size: 24px;
}

.image-attach-field__clear {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--el-text-color-secondary);
  font: inherit;
  font-size: 12px;
  line-height: 1.5;
  white-space: nowrap;
  cursor: pointer;

  &:hover:not(:disabled) {
    color: var(--el-color-danger);
  }

  &:disabled {
    cursor: default;
    opacity: 0.6;
  }

  &:focus-visible {
    outline: 2px solid var(--el-color-primary);
    outline-offset: 2px;
  }
}

.image-attach-field__main {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.image-attach-field__note {
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

// Small in-input info trigger for the allowed-range tooltip; mirrors the
// endpoint-hint-trigger convention (transparent, help cursor, visible focus).
.image-attach-field__hint {
  display: inline-flex;
  align-items: center;
  padding: 2px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: help;
  font: inherit;

  &:focus-visible {
    outline: 2px solid var(--el-color-primary);
    outline-offset: 2px;
  }
}
</style>