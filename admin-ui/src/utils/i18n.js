import en from '@/utils/i18n/en.json'
import zhCN from '@/utils/i18n/zh_CN.json'
import ru from '@/utils/i18n/ru.json'
import { useAppStore } from '@/store/app'

const trans = {
  'en': en,
  'zh-CN': zhCN,
  'ru': ru,
}
export function T (key, params, num = 0) {
  const appStore = useAppStore()
  const lang = appStore.setting.lang
  const tran = trans[lang]?.[key]
  if (!tran) {
    return key
  }
  const msg = num > 1 ? (tran.Other ? tran.Other : tran.One) : tran.One
  // Guard: missing translation form (e.g. no .One in a locale) — don't crash callers.
  if (typeof msg !== 'string') {
    return key
  }
  //msg 是这样 {name} is name
  //params 是这样 {name: 'zhangsan'}
  //替换. params may be undefined/null/primitive (callers like T('Confirm?') or
  //a typo'd T('X', 'foo')). `k in primitive` throws TypeError, so coerce to {}
  //unless it's actually an object. Use `k in` (not `||`) once we have an object,
  //so valid falsy values (0, false, "") aren't dropped back to the literal
  //placeholder.
  const safeParams = (params && typeof params === 'object') ? params : {}
  return msg.replace(/{(\w+)}/g, function (match, k) {
    return k in safeParams ? safeParams[k] : match
  })
}
