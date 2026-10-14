import request from '@/utils/request'

export function upload (formData) {
  return request({
    url: '/file/upload',
    method: 'post',
    data: formData,
    headers: { 'Content-Type': 'multipart/form-data' },
    // Upload failures are surfaced as a localized per-field inline error by
    // the calling form; the interceptor's global toast would only duplicate
    // an unlocalized server string.
    skipErrorMessage: true,
  })
}
