import { apiConfig } from '../config/api'
import { getToken } from './storage'
import { notifyUnauthorized, ApiError } from './request'

/**
 * Multipart file upload via wx.uploadFile, used only by the data-import
 * page. wx.cloud.callContainer has no upload equivalent, so this requires
 * the 'http' transport (develop / self-hosted); prod builds will need the
 * backend domain in the uploadFile whitelist (or a http-mode configuration).
 */

export interface UploadOptions {
  path: string
  filePath: string
  /** Multipart field name, defaults to 'file' (the importer's contract). */
  name?: string
  formData?: Record<string, string>
  timeout?: number
}

export function uploadFile<T = any>(opts: UploadOptions): Promise<T> {
  if (apiConfig.transport !== 'http') {
    return Promise.reject({
      statusCode: 0,
      message: '上传需 HTTP 模式（云托管暂不支持文件上传）',
    } as ApiError)
  }
  return new Promise((resolve, reject) => {
    const header: Record<string, string> = {}
    const token = getToken()
    if (token) header['Authorization'] = `Bearer ${token}`
    wx.uploadFile({
      url: apiConfig.baseUrl + opts.path,
      filePath: opts.filePath,
      name: opts.name || 'file',
      formData: opts.formData,
      header,
      timeout: opts.timeout != null ? opts.timeout : 60000,
      success: (res: any) => {
        let data: any = null
        try {
          data = typeof res.data === 'string' && res.data ? JSON.parse(res.data) : res.data
        } catch {
          data = res.data
        }
        if (res.statusCode === 401) notifyUnauthorized()
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(data as T)
          return
        }
        let message = '上传失败'
        if (data && typeof data === 'object' && typeof data.error === 'string') message = data.error
        else if (typeof data === 'string' && data) message = data
        reject({ statusCode: res.statusCode, message, data } as ApiError)
      },
      fail: () => reject({ statusCode: 0, message: '网络异常，请检查网络后重试' } as ApiError),
    })
  })
}
