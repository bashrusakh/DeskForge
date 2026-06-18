import request from '@/utils/request'

export function groupUsers (data) {
  return request({
    url: '/my/groupUsers',
    method: 'post',
    data,
  })
}
