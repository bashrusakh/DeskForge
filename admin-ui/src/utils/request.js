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

const getEnvelopeErrorMessage = data => {
  const messages = [data?.message, data?.data?.message]

  return messages.find(message => typeof message === 'string' && message.trim()) || ''
}

const redirectToLogin = config => {
  if (!config?.skipAuthRedirect) {
    removeToken()
    window.location.reload()
  }
}

const redirectOnEnvelopeAuthFailure = (config, code) => {
  if (code === 403) {
    redirectToLogin(config)
  }
}

const redirectOnHttpAuthFailure = (config, status) => {
  if (config?.useServerErrorMessage && (status === 401 || status === 403)) {
    redirectToLogin(config)
  }
}

const GENERIC_RESPONSE_ERROR = 'Unable to process server response'

const markInterceptorHandled = error => {
  Object.defineProperty(error, 'interceptorHandled', { value: true })
  return error
}

const getResponseErrorMessage = async error => {
  if (!error.config?.useServerErrorMessage) return ''
  let data = error.response?.data
  if (error.config?.responseType === 'blob' && typeof Blob !== 'undefined' && data instanceof Blob) {
    try {
      data = JSON.parse(await data.text())
    } catch (_) {
      return ''
    }
  }
  return getEnvelopeErrorMessage(data)
}

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
        let res
        try {
          res = JSON.parse(text)
        } catch (_) {
          const malformedResponseError = new Error(GENERIC_RESPONSE_ERROR)
          if (response.config.useServerErrorMessage && !response.config.skipErrorMessage) {
            ElMessage({
              message: GENERIC_RESPONSE_ERROR,
              type: 'error',
              duration: 5 * 1000,
            })
            return Promise.reject(markInterceptorHandled(malformedResponseError))
          }
          return Promise.reject(malformedResponseError)
        }

        if (res.code !== 0) {
          if (!response.config.skipErrorMessage) {
            ElMessage({
              message: response.config.useServerErrorMessage
                ? getEnvelopeErrorMessage(res) || GENERIC_RESPONSE_ERROR
                : res.message || 'error',
              type: 'error',
              duration: 5 * 1000,
            })
          }

          redirectOnEnvelopeAuthFailure(response.config, res.code)
          return Promise.reject(res)
        }
        return response
      })
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

      redirectOnEnvelopeAuthFailure(response.config, res.code)
      return Promise.reject(res)
    } else {
      return res
    }
  },
  async error => {
    if (error.code === 'ECONNABORTED'
      && error.message.indexOf('timeout') > -1) {
      error.message = 'Connection Time Out!'
    }
    if (!error.config?.skipErrorMessage) {
      const message = await getResponseErrorMessage(error)
      ElMessage({
        message: error.config?.useServerErrorMessage
          ? message || GENERIC_RESPONSE_ERROR
          : error.message,
        type: 'error',
        duration: 5 * 1000,
      })
    }
    redirectOnHttpAuthFailure(error.config, error.response?.status)
    return Promise.reject(error)
  },
)

export default service
