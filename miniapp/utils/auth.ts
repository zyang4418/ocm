import { request, setOnUnauthorized } from './request'
import { getToken, setToken, clearToken } from './storage'

const USER_KEY = 'user'

export interface AppUser {
  id: number
  username: string
  displayName: string
  role: string
}

interface AuthResponse {
  token: string
  user: AppUser
}

// Re-exported so existing consumers (e.g. pages/login) keep importing getToken
// from auth; the single implementation now lives in ./storage, avoiding the
// auth.ts <-> request.ts circular import.
export { getToken }

export function getUser(): AppUser | null {
  try {
    return wx.getStorageSync(USER_KEY) || null
  } catch {
    return null
  }
}

function setAuth(token: string, user: AppUser) {
  setToken(token)
  wx.setStorageSync(USER_KEY, user)
}

export function clearAuth() {
  clearToken()
  try {
    wx.removeStorageSync(USER_KEY)
  } catch {}
}

function wxLogin(): Promise<string> {
  return new Promise((resolve, reject) => {
    wx.login({
      success: (res) => {
        if (res && res.code) resolve(res.code)
        else reject(new Error('wx.login 未返回 code'))
      },
      fail: () => reject(new Error('wx.login 调用失败')),
    })
  })
}

/**
 * Silent re-login for a returning user whose WeChat openid is already bound.
 * Rejects with statusCode 404 when the openid is not bound (first time, or
 * after an unbind) -- callers show the bind form in that case.
 */
export async function silentLogin(): Promise<AppUser> {
  const code = await wxLogin()
  const res = await request<AuthResponse>({
    path: '/api/auth/wx-login',
    method: 'POST',
    data: { code },
    auth: false,
  })
  setAuth(res.token, res.user)
  return res.user
}

/** First-time bind: verify credentials and link this WeChat openid. */
export async function bindAccount(username: string, password: string): Promise<AppUser> {
  const code = await wxLogin()
  const res = await request<AuthResponse>({
    path: '/api/auth/wx-bind',
    method: 'POST',
    data: { username, password, code },
    auth: false,
  })
  setAuth(res.token, res.user)
  return res.user
}

/** Unbind the WeChat openid from the current account; forces re-bind next time. */
export async function unbind(): Promise<void> {
  await request({ path: '/api/auth/wx-unbind', method: 'POST' })
  clearAuth()
}

/**
 * Guard for protected pages: resolve true when authenticated, else redirect to
 * the login page. A stored token is presumed valid; the backend 401s if it
 * expired, and the 401 handler routes back through the login page (which
 * silently re-logs in bound users). Only when there is no token do we probe.
 */
export async function ensureAuth(): Promise<boolean> {
  if (getToken()) return true
  try {
    await silentLogin()
    return true
  } catch (err: any) {
    // 404 = openid 未绑定(后端 wxLogin 的确定性返回):通知 login 跳过冗余
    // probe 直接出表单。其余错误(网络 0 / 502 / 500)可能瞬时,仍让 login 重试。
    const notBound = err && err.statusCode === 404
    wx.reLaunch({ url: '/pages/login/login' + (notBound ? '?notBound=1' : '') })
    return false
  }
}

/** Wire the 401 handler. Call once at app launch. */
export function initAuth() {
  setOnUnauthorized(() => {
    clearAuth()
    wx.reLaunch({ url: '/pages/login/login' })
  })
}
