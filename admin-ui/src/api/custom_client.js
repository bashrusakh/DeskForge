import request from '@/utils/request'

export function list (params) {
  return request({
    url: '/custom_build/list',
    params,
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

export function download (id) {
  return request({
    url: `/custom_build/download/${id}`,
    responseType: 'blob',
  })
}

export function getVersions () {
  return request({
    url: '/custom_build/versions',
  })
}
