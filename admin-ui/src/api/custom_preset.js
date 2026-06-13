import request from '@/utils/request'

export function list (params) {
  return request({
    url: '/custom_preset/list',
    params,
  })
}

export function detail (id) {
  return request({
    url: '/custom_preset/detail/' + id,
  })
}

export function create (data) {
  return request({
    url: '/custom_preset/create',
    method: 'post',
    data,
  })
}

export function update (data) {
  return request({
    url: '/custom_preset/update',
    method: 'post',
    data,
  })
}

export function remove (data) {
  return request({
    url: '/custom_preset/delete',
    method: 'post',
    data,
  })
}
