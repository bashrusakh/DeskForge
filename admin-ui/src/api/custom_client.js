import request from '@/utils/request'

export function list (params) {
  return request({
    url: '/custom_build/list',
    params,
  })
}

export function detail (id) {
  return request({
    url: '/custom_build/detail/' + id,
  })
}

export function create (data) {
  return request({
    url: '/custom_build/create',
    method: 'post',
    data,
  })
}

export function remove (data) {
  return request({
    url: '/custom_build/delete',
    method: 'post',
    data,
  })
}

export function detailByKey (key) {
  return request({
    url: '/custom_build/public/detailByKey/' + key,
  })
}
