<template>
  <button class="copyable-text" type="button" :title="title || text" @click="copy">
    <span class="copyable-text__value">{{ text || '-' }}</span>
    <el-icon class="copyable-text__icon"><CopyDocument /></el-icon>
  </button>
</template>

<script setup>
  import { ElMessage } from 'element-plus'
  import { CopyDocument } from '@element-plus/icons-vue'
  import { T } from '@/utils/i18n'

  const props = defineProps({
    text: {
      type: [String, Number],
      default: '',
    },
    title: {
      type: String,
      default: '',
    },
  })

  const copy = async () => {
    const value = String(props.text || '')
    if (!value) return

    try {
      await navigator.clipboard.writeText(value)
      ElMessage.success(T('CopySuccess'))
    } catch (_) {
      const input = document.createElement('textarea')
      input.value = value
      input.setAttribute('readonly', 'readonly')
      input.style.position = 'fixed'
      input.style.opacity = '0'
      document.body.appendChild(input)
      input.select()
      const copied = document.execCommand('copy')
      document.body.removeChild(input)
      copied ? ElMessage.success(T('CopySuccess')) : ElMessage.error(T('CopyFailed'))
    }
  }
</script>

<style scoped lang="scss">
.copyable-text {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 8px;
  border: 0;
  border-radius: 10px;
  background: var(--color-code-bg);
  color: var(--color-text);
  cursor: pointer;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1;
  padding: 7px 9px;
  transition: color 0.2s ease, background 0.2s ease;

  &:hover,
  &:focus-visible {
    background: var(--color-primary-soft);
    color: var(--color-primary);
    outline: none;
  }
}

.copyable-text__value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.copyable-text__icon {
  flex: 0 0 auto;
}
</style>
