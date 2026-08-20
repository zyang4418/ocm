import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { apiFetch } from './api'
import type { CurrentUser, LoginResponse, Permission } from '../types/api'
import { clearPublicKeyCache, encryptPassword } from './sm2.js'

const TOKEN_KEY = 'ocm.token'

export interface AuthContextValue {
  token: string | null
  user: CurrentUser | null
  bootstrapping: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  can: (perm: Permission) => boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem(TOKEN_KEY))
  const [user, setUser] = useState<CurrentUser | null>(null)
  const [bootstrapping, setBootstrapping] = useState(() => Boolean(localStorage.getItem(TOKEN_KEY)))

  // Validate a persisted token on startup and hydrate the current user.
  useEffect(() => {
    if (!token) return undefined
    let cancelled = false
    apiFetch<CurrentUser>('/api/auth/me', { token })
      .then((u) => {
        if (!cancelled) setUser(u)
      })
      .catch((err: Error & { status?: number }) => {
        if (cancelled) return
        // Only drop the token when the server says it is unusable (401).
        // Transient failures (5xx, network) keep the token so a reload retries.
        if (err.status === 401) {
          localStorage.removeItem(TOKEN_KEY)
          setToken(null)
        }
        setUser(null)
      })
      .finally(() => {
        if (!cancelled) setBootstrapping(false)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  const login = useCallback(async (username: string, password: string) => {
    const encryptedPassword = await encryptPassword(password)
    let data: LoginResponse
    try {
      data = await apiFetch<LoginResponse>('/api/auth/login', {
        method: 'POST',
        body: {
          username,
          password: encryptedPassword,
          password_encoding: 'sm2-c1c3c2-base64',
        },
      })
    } catch (error) {
      clearPublicKeyCache()
      throw error
    }
    localStorage.setItem(TOKEN_KEY, data.token)
    setToken(data.token)
    setUser(data.user)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY)
    setToken(null)
    setUser(null)
  }, [])

  // can reports whether the current user holds a permission. The backend is
  // the enforcement authority; this only gates UI visibility.
  const can = useCallback(
    (perm: Permission) => {
      const perms = Array.isArray(user?.permissions) ? user.permissions : []
      return perms.includes('*') || perms.includes(perm)
    },
    [user],
  )

  const value = useMemo(
    () => ({ token, user, bootstrapping, login, logout, can }),
    [token, user, bootstrapping, login, logout, can],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
