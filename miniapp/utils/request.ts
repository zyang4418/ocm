import { apiConfig } from '../config/api'

export interface ApiError {
  statusCode: number
  message: string
  data?: any
}

export interface RequestOptions {
  path: string
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  /** Request body (non-GET). For query params on any method, use `params`. */
  data?: any
  /** Query-string params, appended to `path` for both transports. */
  params?: Record<string, string | number | undefined | null>
  header?: Record<string, string>
  /** Attach the stored JWT. Defaults to true. */
  auth?: boolean
  timeout?: number
}

let onUnauthorized: (() => void) | null = null

/** Register a handler invoked when an authenticated request returns 401. */
export function setOnUnauthorized(fn: () => void) {
  onUnauthorized = fn
}

function buildPath(path: string, params?: RequestOptions['params']): string {
  if (!params) return path
  const parts: string[] = []
  for (const key of Object.keys(params)) {
    const v = params[key]
    if (v === undefined || v === null) continue
    parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(v))}`)
  }
  if (!parts.length) return path
  return path + (path.indexOf('?') >= 0 ? '&' : '?') + parts.join('&')
}

function getToken(): string | null {
  try {
    return wx.getStorageSync('token') || null
  } catch {
    return null
  }
}

interface RawResponse {
  data: any
  statusCode: number
}

// Both callContainer and wx.request deliver HTTP status via the success
// callback (fail is transport-only), and auto-parse JSON bodies. The wrapper
// unifies the two so callers never depend on the transport.
function callTransport(
  opts: RequestOptions,
  fullPath: string,
  header: Record<string, string>,
  timeout: number
): Promise<RawResponse> {
  return new Promise((resolve, reject) => {
    const success = (res: any) => resolve({ data: res.data, statusCode: res.statusCode })
    const fail = () =>
      reject({ statusCode: 0, message: '网络异常，请检查网络后重试' } as ApiError)

    if (apiConfig.transport === 'callContainer') {
      const cloud = (wx as any).cloud
      if (!cloud || typeof cloud.callContainer !== 'function') {
        reject({ statusCode: 0, message: '云能力未初始化' } as ApiError)
        return
      }
      cloud.callContainer({
        config: { env: apiConfig.cloudEnv },
        path: fullPath,
        method: opts.method || 'GET',
        header,
        data: opts.data,
        timeout,
        success,
        fail,
      })
    } else {
      wx.request({
        url: apiConfig.baseUrl + fullPath,
        method: opts.method || 'GET',
        header,
        data: opts.data,
        timeout,
        success,
        fail,
      })
    }
  })
}

export async function request<T = any>(opts: RequestOptions): Promise<T> {
  const fullPath = buildPath(opts.path, opts.params)
  const header: Record<string, string> = {
    'content-type': 'application/json',
    ...(opts.header || {}),
  }
  if (apiConfig.transport === 'callContainer' && apiConfig.serviceName) {
    header['X-WX-SERVICE'] = apiConfig.serviceName
  }
  if (opts.auth !== false) {
    const token = getToken()
    if (token) header['Authorization'] = `Bearer ${token}`
  }
  const timeout = opts.timeout != null ? opts.timeout : apiConfig.timeout

  const res = await callTransport(opts, fullPath, header, timeout)
  const { data, statusCode } = res

  if (statusCode === 401 && opts.auth !== false && onUnauthorized) {
    onUnauthorized()
  }

  if (statusCode >= 200 && statusCode < 300) {
    return data as T
  }

  let message = '请求失败'
  if (data && typeof data === 'object' && typeof data.error === 'string') {
    message = data.error
  } else if (typeof data === 'string' && data) {
    message = data
  }
  throw { statusCode, message, data } as ApiError
}
