import request from '@/utils/request'

export function stats () {
  return request({
    url: '/dashboard/stats',
    method: 'get',
  })
}

export function health () {
  return request({
    url: '/dashboard/health',
    method: 'get',
  })
}
