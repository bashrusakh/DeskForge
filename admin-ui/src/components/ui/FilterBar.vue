<template>
  <page-section class="filter-bar" :title="title" :subtitle="subtitle">
    <el-form
        v-if="!collapsed"
        inline
        :label-width="labelWidth"
        :model="filters"
        size="small"
    >
      <el-form-item v-for="field in fields" :key="field.key" :label="field.label">
        <component
            :is="field.component || 'el-input'"
            v-model="filters[field.key]"
            :clearable="field.clearable !== false"
            :placeholder="field.placeholder"
            :style="field.style || 'width: 200px'"
            v-bind="field.props || {}"
        >
          <template v-if="field.options" #default>
            <el-option
                v-for="opt in field.options"
                :key="opt.value"
                :label="opt.label"
                :value="opt.value"
            />
          </template>
        </component>
      </el-form-item>

      <el-form-item>
        <el-button type="primary" @click="$emit('filter')">{{ T('Filter') }}</el-button>
        <el-button
            v-if="showReset && hasFilters"
            @click="resetFilters"
        >{{ T('Reset') }}</el-button>
        <slot name="actions"></slot>
      </el-form-item>
    </el-form>

    <div v-else class="filter-bar__collapsed">
      <span class="filter-bar__collapsed-text">
        {{ activeFilterCount }} {{ activeFilterCount === 1 ? T('FilterActive') : T('FiltersActive') }}
      </span>
      <el-button type="primary" size="small" @click="collapsed = false">
        {{ T('ShowFilters') }}
      </el-button>
      <el-button
          v-if="hasFilters"
          size="small"
          @click="resetFilters"
      >{{ T('ClearFilters') }}</el-button>
    </div>

    <div class="filter-bar__toggle" @click="collapsed = !collapsed">
      <el-icon :class="{ 'is-active': !collapsed }">
        <ArrowDown />
      </el-icon>
      <span>{{ collapsed ? T('ShowFilters') : T('HideFilters') }}</span>
    </div>
  </page-section>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { T } from '@/utils/i18n'
import { ArrowDown } from '@element-plus/icons-vue'
import PageSection from './PageSection.vue'

defineProps({
  title: { type: String, default: 'Filters' },
  subtitle: { type: String, default: '' },
  fields: {
    type: Array,
    required: true,
  },
  filters: {
    type: Object,
    required: true,
  },
  labelWidth: {
    type: [String, Number],
    default: '80px',
  },
  collapsed: {
    type: Boolean,
    default: false,
  },
  showReset: {
    type: Boolean,
    default: true,
  },
})

defineEmits(['filter', 'update:collapsed'])

const hasFilters = computed(() => {
  return Object.values(filters).some(v => v !== null && v !== undefined && v !== '')
})

const activeFilterCount = computed(() => {
  return Object.values(filters).filter(v => v !== null && v !== undefined && v !== '').length
})

const resetFilters = () => {
  Object.keys(filters).forEach(key => {
    filters[key] = null
  })
  emit('filter')
}
</script>

<style scoped lang="scss">
.filter-bar {
  .el-form-item {
    margin-bottom: 0;
  }

  .el-form-item__content {
    flex: 1;
  }
}

.filter-bar__collapsed {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  font-size: 13px;
  color: var(--color-muted);

  .filter-bar__collapsed-text {
    font-weight: 600;
  }
}

.filter-bar__toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;

  &:hover {
    background: var(--color-surface-2);
  }

  .is-active {
    transform: rotate(180deg);
  }
}

@media (max-width: 768px) {
  .filter-bar__collapsed {
    flex-wrap: wrap;
  }
}
</style>