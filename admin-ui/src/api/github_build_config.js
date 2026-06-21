import request from '@/utils/request'

export function get () {
  return request({ url: '/github_build_config/get' })
}

export function save (data) {
  return request({ url: '/github_build_config/save', method: 'post', data })
}

export function generateKey () {
  return request({ url: '/github_build_config/generate_key', method: 'post' })
}

export function test () {
  return request({ url: '/github_build_config/test', method: 'post' })
}

export function syncSecret () {
  return request({ url: '/github_build_config/sync_secret', method: 'post' })
}

export function dispatchTest () {
  // B-009: confirm=true — это реальный билд (тратит минуты Actions), не дешёвый чек.
  return request({ url: '/github_build_config/dispatch_test', method: 'post', data: { confirm: true } })
}
