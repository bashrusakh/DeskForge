import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { T } from '@/utils/i18n'

export function useBulkRemove ({ removeApi, getList, label, onAfterRemove, selectionRef }) {
  const _selectedRows = ref([])
  const selectedRows = selectionRef || _selectedRows

  const removeOne = async (id) => {
    return removeApi({ id }).catch(() => false)
  }

  const confirmAndRemove = async (rows) => {
    if (!rows.length) return 0
    const count = rows.length
    const cf = await ElMessageBox.confirm(
      T('Confirm?', { param: `${T('Delete')} (${count})${label ? ' ' + label : ''}` }),
      { confirmButtonText: T('Confirm'), cancelButtonText: T('Cancel'), type: 'warning' }
    ).catch(() => false)
    if (!cf) return 0
    const results = await Promise.all(rows.map(r => removeOne(r.id)))
    const ok = results.filter(Boolean).length
    selectedRows.value = []
    if (ok) {
      ElMessage.success(T('OperationSuccess'))
      if (onAfterRemove) onAfterRemove()
      if (getList) getList()
    }
    return ok
  }

  const bulkRemove = () => confirmAndRemove(selectedRows.value)

  return { selectedRows, bulkRemove }
}
