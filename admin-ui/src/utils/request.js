import axios from 'axios'
import { ElMessage } from 'element-plus'
import { getToken, removeToken } from '@/utils/auth'
import { useUserStore } from '@/store/user'
import { pinia } from '@/store'
import { useAppStore } from '@/store/app'

// create an axios instance
const service = axios.create({
  baseURL: import.meta.env.VITE_SERVER_API,
  withCredentials: true, // send cookies when cross-domain requests
  timeout: 50000, // request timeout
})

// request interceptor
service.interceptors.request.use(
  config => {
    if (!config.headers) {
      config.headers = {}
    }
    const userStore = useUserStore(pinia)

    const token = userStore.token || getToken()
    if (token) {
      config.headers['api-token'] = token
    }

    const app = useAppStore()
    const lang = app.setting.lang
    if (lang) {
      // console.log('lang', lang)
      config.headers['Accept-Language'] = lang
    }

    // Запрещаем кеширование GET-запросов на бэкенде (статусы задач и т.п.)
    // — браузер по умолчанию может закешировать ответ.
    if ((config.method || 'get').toLowerCase() === 'get') {
      config.headers['Cache-Control'] = 'no-cache'
      config.headers['Pragma'] = 'no-cache'
    }

    return config
  },
  error => {
    // do something with request error
    return Promise.reject(error)
  },
)

// response interceptor
service.interceptors.response.use(
  /**
   * If you want to get http information such as headers or status
   * Please return  response => response
   */

  /**
   * Determine the request status by custom code
   * Here is just an example
   * You can also judge the status by HTTP Status Code
   */
  response => {
    // Binary downloads do not use the normal JSON response envelope.
    if (response.config.responseType === 'blob') {
      const contentType = response.headers?.['content-type'] || ''
      if (!contentType.includes('application/json')) {
        return response
      }

      // Auth/API failures can still arrive as a JSON envelope with HTTP 200.
      // Parse those responses so a failed download is not saved as a .zip file.
      return response.data.text().then(text => {
        const res = JSON.parse(text)
        if (res.code !== 0) {
          ElMessage({
            message: res.message || 'error',
            type: 'error',
            duration: 5 * 1000,
          })

          if (res.code === 403) {
            removeToken()
            window.location.reload()
          }
          return Promise.reject(res)
        }
        return response
      }).catch(error => Promise.reject(error))
    }

    const res = response.data

    // for the endpoint /login-options
    // I'm not sure if this is a good idea
    if (Array.isArray(res)) {
      return res;
    }

    // if the custom code is not 20000, it is judged as an error.
    if (res.code !== 0) {
      if (!response.config.skipErrorMessage) {
        ElMessage({
          message: res.message || 'error',
          type: 'error',
          duration: 5 * 1000,
        })
      }

      if (res.code === 403 && !response.config.skipAuthRedirect) {
        removeToken()
        window.location.reload()
      }
      return Promise.reject(res)
    } else {
      return res
    }
  },
  error => {
    if (error.code === 'ECONNABORTED'
      && error.message.indexOf('timeout') > -1) {
      error.message = 'Connection Time Out!'
    }
    if (!error.config?.skipErrorMessage) {
      ElMessage({
        message: error.message,
        type: 'error',
        duration: 5 * 1000,
      })
    }
    return Promise.reject(error)
  },
)

export default service
