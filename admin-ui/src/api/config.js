import request from '@/utils/request'

export function server () {
  return request({
    url: '/config/server',
    method: 'get',
  })
}

export function app () {
  return request({
    url: '/config/app',
    method: 'get',
  })
}

export function admin () {
  return request({
    url: '/config/admin',
    method: 'get',
  })
}

export function all () {
  return request({
    url: '/config/all',
    method: 'get',
  })
}

export function serverAddresses () {
  return request({
    url: '/config/server_addresses',
    method: 'get',
    // One localized error indication: the picker shows CustomClientLocalIpLoadError
    // itself, so the shared interceptor toast is suppressed for this call only
    // (same house style as getVersions).
    skipErrorMessage: true,
    useServerErrorMessage: true,
  })
}
