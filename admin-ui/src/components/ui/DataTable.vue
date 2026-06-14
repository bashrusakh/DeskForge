<template>
  <div class="data-table">
    <div class="data-table__scroller">
      <el-table
          v-bind="tableProps"
          :class="['data-table__table', tableClass, density === 'compact' ? 'data-table__table--compact' : '']"
          :data="data"
          :loading="loading"
          :row-key="rowKey || undefined"
          :size="tableSize"
          @selection-change="$emit('selection-change', $event)"
          @sort-change="$emit('sort-change', $event)"
          @row-click="$emit('row-click', $event)"
          @row-dblclick="$emit('row-dblclick', $event)"
      >
        <el-table-column
            v-if="selectable"
            type="selection"
            align="center"
            :width="selectionWidth"
            :fixed="selectionFixed"
            :reserve-selection="reserveSelection"
        />
        <el-table-column
            v-if="showIndex"
            type="index"
            :label="indexLabel"
            align="center"
            :width="indexWidth"
            :fixed="indexFixed"
        />
        <el-table-column
            v-for="column in columns"
            :key="column.key || column.prop || column.label"
            v-bind="getColumnProps(column)"
        >
          <template v-if="column.slot && $slots[column.slot]" #default="scope">
            <slot
                :name="column.slot || 'cell'"
                v-bind="{
                  row: scope.row,
                  column: scope.column,
                  $index: scope.$index,
                  cellValue: getCell(scope.row, column.prop)
                }"
            >
              {{ getCell(scope.row, column.prop) }}
            </slot>
          </template>
        </el-table-column>

        <template #empty>
          <div class="data-table__empty">
            <slot name="empty">
              <empty-state
                  :title="emptyTitle"
                  :description="emptyDescription"
              >
                <template v-if="$slots['empty-actions']" #actions>
                  <slot name="empty-actions"></slot>
                </template>
              </empty-state>
            </slot>
          </div>
        </template>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import EmptyState from './EmptyState.vue'

const props = defineProps({
  data: {
    type: Array,
    required: true,
  },
  columns: {
    type: Array,
    required: true,
  },
  loading: {
    type: Boolean,
    default: false,
  },
  rowKey: {
    type: [String, Function],
    default: '',
  },
  border: {
    type: Boolean,
    default: false,
  },
  stripe: {
    type: Boolean,
    default: false,
  },
  height: {
    type: [String, Number],
    default: '',
  },
  maxHeight: {
    type: [String, Number],
    default: '',
  },
  scrollWidth: {
    type: String,
    default: '100%',
  },
  density: {
    type: String,
    default: 'normal',
    validator: value => ['normal', 'compact'].includes(value),
  },
  selectable: {
    type: Boolean,
    default: false,
  },
  selectionWidth: {
    type: [String, Number],
    default: 44,
  },
  selectionFixed: {
    type: [Boolean, String],
    default: false,
  },
  reserveSelection: {
    type: Boolean,
    default: false,
  },
  showIndex: {
    type: Boolean,
    default: false,
  },
  indexLabel: {
    type: String,
    default: '#',
  },
  indexWidth: {
    type: [String, Number],
    default: 60,
  },
  indexFixed: {
    type: [Boolean, String],
    default: false,
  },
  emptyTitle: {
    type: String,
    default: 'No records',
  },
  emptyDescription: {
    type: String,
    default: 'No rows match the current filters.',
  },
  tableClass: {
    type: String,
    default: '',
  },
})

const emit = defineEmits([
  'selection-change',
  'sort-change',
  'row-click',
  'row-dblclick',
])

const tableSize = computed(() => props.density === 'compact' ? 'small' : 'default')

const tableProps = computed(() => ({
  border: props.border,
  stripe: props.stripe,
  height: props.height || undefined,
  maxHeight: props.maxHeight || undefined,
}))

const getColumnProps = (column) => {
  const { slot, ...columnProps } = column
  return columnProps
}

const getCell = (row, prop) => {
  if (!prop) {
    return ''
  }

  return String(prop)
      .split('.')
      .reduce((value, key) => value?.[key], row) ?? ''
}
</script>

<style scoped lang="scss">
.data-table {
  width: 100%;
}

.data-table__scroller {
  width: v-bind(scrollWidth);
  overflow-x: auto;
}

.data-table__table {
  width: 100%;
}

.data-table__empty {
  width: 100%;
  padding: 24px;
}

:deep(.data-table__empty .empty-state) {
  min-height: 220px;
}

:deep(.data-table__table .el-table__cell) {
  padding: 10px 0;
  font-size: 13px;
}

:deep(.data-table__table .el-table th.el-table__cell) {
  padding: 10px 0;
  font-size: 12px;
}

:deep(.data-table__table.data-table__table--compact .el-table__cell) {
  padding: 8px 0;
}

:deep(.table-actions .el-button) {
  margin: 4px;
}
</style>
