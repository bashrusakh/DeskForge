import request from '@/utils/request'

export function list (params) {
  return request({
    url: '/my/peer/list',
    params,
  })
}

export function remove (data) {
  return request({
    url: '/my/peer/delete',
    method: 'post',
    data,
  })
}

export function batchRemove (data) {
  return request({
    url: '/my/peer/batchDelete',
    method: 'post',
    data,
  })
}
