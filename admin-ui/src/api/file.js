import request from '@/utils/request'

export function upload (formData) {
  return request({
    url: '/file/upload',
    method: 'post',
    data: formData,
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}
