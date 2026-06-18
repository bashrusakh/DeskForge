import { ElMessage, ElMessageBox } from 'element-plus'
import { T } from '@/utils/i18n'

// Shared "confirm + single batch API call + success message + refresh" flow for list
// views. Unlike useBulkRemove (which deletes per row, issuing N requests), this calls a
// real batch endpoint ONCE for all selected items. It exists to stop every list view /
// view-composable from re-implementing the same confirm dialog + message + getList block.
//
// Options:
//   batchApi(payload)      required — performs ONE request for all items.
//   buildPayload(items)    required — maps selected rows/ids to the request body.
//   getList()              optional — refresh callback run on success.
//   label                  optional — confirm subject: string, or (items) => string.
//                                       Defaults to T('BatchDelete').
//   selectionRef           optional — a ref; cleared to [] on success.
//
// Returns: { confirmAndRemove(items) => Promise<boolean> }  (true on success).
export function useBatchRemove ({ batchApi, buildPayload, getList, label, selectionRef }) {
  const confirmAndRemove = async (items) => {
    if (!items || !items.length) {
      ElMessage.warning(T('PleaseSelectData'))
      return false
    }
    const param = typeof label === 'function' ? label(items) : (label || T('BatchDelete'))
    const cf = await ElMessageBox.confirm(T('Confirm?', { param }), {
      confirmButtonText: T('Confirm'),
      cancelButtonText: T('Cancel'),
      type: 'warning',
    }).catch(() => false)
    if (!cf) return false

    const res = await batchApi(buildPayload(items)).catch(() => false)
    if (!res) {
      return false
    }
    if (selectionRef) selectionRef.value = []
    ElMessage.success(T('OperationSuccess'))
    if (getList) getList()
    return true
  }
  return { confirmAndRemove }
}
