import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { T } from '@/utils/i18n'

export function useBulkRemove ({ removeApi, getList, label, onAfterRemove, selectionRef, getRemovePayload, warningMessage }) {
  const _selectedRows = ref([])
  const selectedRows = selectionRef || _selectedRows
  const payloadFn = getRemovePayload || ((r) => ({ id: r.id }))
  const removing = ref(false)

  const removeOne = async (row) => {
    let payload
    try {
      payload = payloadFn(row)
    } catch (e) {
      console.error('[useBulkRemove] payloadFn error:', e)
      return false
    }
    return removeApi(payload).catch(() => false)
  }

  const confirmAndRemove = async (rows) => {
    if (!rows.length || removing.value) return 0
    removing.value = true
    const count = rows.length
    const msg = T('Confirm?', { param: `${T('Delete')} (${count})${label ? ' ' + label : ''}` }) +
      (warningMessage ? '\n\n' + warningMessage : '')
    const cf = await ElMessageBox.confirm(
      msg,
      { confirmButtonText: T('Confirm'), cancelButtonText: T('Cancel'), type: 'warning' }
    ).catch(() => false)
    if (!cf) { removing.value = false; return 0 }
    const results = await Promise.all(rows.map(r => removeOne(r)))
    const ok = results.filter(Boolean).length
    if (ok) {
      selectedRows.value = []
    }
    if (ok === count) {
      ElMessage.success(T('OperationSuccess'))
    } else if (ok > 0) {
      ElMessage.warning(`${T('OperationSuccess')} (${ok}/${count})`)
    } else {
      ElMessage.error(T('OperationFailed'))
    }
    if (ok) {
      if (onAfterRemove) onAfterRemove()
      if (getList) getList()
    }
    removing.value = false
    return ok
  }

  const bulkRemove = () => confirmAndRemove(selectedRows.value)

  return { selectedRows, bulkRemove }
}
