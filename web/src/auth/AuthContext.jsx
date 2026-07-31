import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { apiFetch } from './api.js'

const TOKEN_KEY = 'ocm.token'
const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_KEY))
  const [user, setUser] = useState(null)
  const [bootstrapping, setBootstrapping] = useState(() => Boolean(localStorage.getItem(TOKEN_KEY)))

  // Validate a persisted token on startup and hydrate the current user.
  useEffect(() => {
    if (!token) return undefined
    let cancelled = false
    apiFetch('/api/auth/me', { token })
      .then((u) => {
        if (!cancelled) setUser(u)
      })
      .catch((err) => {
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

  const login = useCallback(async (username, password) => {
    const data = await apiFetch('/api/auth/login', {
      method: 'POST',
      body: { username, password },
    })
    localStorage.setItem(TOKEN_KEY, data.token)
    setToken(data.token)
    setUser(data.user)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY)
    setToken(null)
    setUser(null)
  }, [])

  const value = useMemo(
    () => ({ token, user, bootstrapping, login, logout }),
    [token, user, bootstrapping, login, logout],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  return useContext(AuthContext)
}
