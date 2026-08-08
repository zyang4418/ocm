/**
 * Single source of truth for the JWT storage key and token read/write.
 *
 * Extracted as a leaf module (depends only on the `wx` global) so that both
 * `auth.ts` and `request.ts` can share one `getToken` without introducing the
 * circular import auth.ts -> request.ts -> auth.ts, which previously forced
 * `request.ts` to carry a private, literal-keyed duplicate of the function.
 *
 * The token is cached in memory after first read; every mutation goes through
 * setToken/clearToken, so the cache stays in sync with storage and repeated
 * authenticated requests avoid a blocking wx.getStorageSync on the hot path.
 */
export const TOKEN_KEY = 'token'

// undefined = not yet loaded from storage; null = loaded and absent.
let cachedToken: string | null | undefined = undefined

export function getToken(): string | null {
  if (cachedToken === undefined) {
    try {
      cachedToken = wx.getStorageSync(TOKEN_KEY) || null
    } catch {
      cachedToken = null
    }
  }
  return cachedToken
}

export function setToken(token: string) {
  wx.setStorageSync(TOKEN_KEY, token)
  cachedToken = token
}

export function clearToken() {
  try {
    wx.removeStorageSync(TOKEN_KEY)
  } catch {}
  cachedToken = null
}
