import { ElMessage } from 'element-plus'
import { T } from '@/utils/i18n'

export const connectByClient = (id) => {
  let a = document.createElement('a')
  a.href = `rustdesk://${id}`
  a.target = '_self'
  a.click()

  // If the RustDesk client is not installed, the protocol handler does nothing.
  // Show a fallback message after a short delay.
  setTimeout(() => {
    if (!document.hidden) {
      ElMessage.info(T('RustDeskClientNotFound') || 'RustDesk client not found. Please install RustDesk to connect.')
    }
  }, 3000)
}
