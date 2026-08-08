import { request, setOnUnauthorized } from './request'

const TOKEN_KEY = 'token'
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

export function getToken(): string | null {
  try {
    return wx.getStorageSync(TOKEN_KEY) || null
  } catch {
    return null
  }
}

export function getUser(): AppUser | null {
  try {
    return wx.getStorageSync(USER_KEY) || null
  } catch {
    return null
  }
}

function setAuth(token: string, user: AppUser) {
  wx.setStorageSync(TOKEN_KEY, token)
  wx.setStorageSync(USER_KEY, user)
  const app = getApp() as any
  if (app && app.globalData) app.globalData.user = user
}

export function clearAuth() {
  try {
    wx.removeStorageSync(TOKEN_KEY)
    wx.removeStorageSync(USER_KEY)
  } catch {}
  const app = getApp() as any
  if (app && app.globalData) app.globalData.user = null
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
  } catch {
    wx.reLaunch({ url: '/pages/login/login' })
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
