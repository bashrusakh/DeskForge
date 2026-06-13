import request from '@/utils/request'

export function stats () {
  return request({
    url: '/dashboard/stats',
    method: 'get',
  })
}
