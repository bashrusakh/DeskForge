import { ElMessage } from 'element-plus'
import { T } from '@/utils/i18n'

export const connectByClient = (id) => {
  let a = document.createElement('a')
  a.href = `rustdesk://${id}`
  a.target = '_self'
  a.click()

  // If the RustDesk client is not installed, the protocol handler does nothing.
  // The browser gives us no reliable signal either way, so this is a
  // best-effort heuristic: after 3s, if the tab is still visible (i.e. the OS
  // didn't switch focus to the client), assume the protocol wasn't handled
  // and prompt the user. False positives are possible on slow client launches
  // and false negatives if the user just changes tabs.
  setTimeout(() => {
    if (!document.hidden) {
      ElMessage.info(T('RustDeskClientNotFound'))
    }
  }, 3000)
}
