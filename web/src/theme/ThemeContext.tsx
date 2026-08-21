import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useLayoutEffect,
  useState,
  type ReactNode,
} from 'react'

// The app supports every Carbon theme plus a "system" preference that follows
// the OS color scheme: light resolves to g10 (the app's historical look) and
// dark to g100. Themes are applied as a `cds--<name>` class on <html>; the
// prebuilt @carbon/styles CSS ships all four theme token sets under those
// classes, so switching never touches the build.
export const THEME_NAMES = ['white', 'g10', 'g90', 'g100'] as const
export type ThemeName = (typeof THEME_NAMES)[number]

export const THEME_PREFERENCES = ['system', ...THEME_NAMES] as const
export type ThemePreference = (typeof THEME_PREFERENCES)[number]

const STORAGE_KEY = 'ocm.theme'
const DARK_QUERY = '(prefers-color-scheme: dark)'

export function resolveTheme(preference: ThemePreference, systemDark: boolean): ThemeName {
  if (preference !== 'system') return preference
  return systemDark ? 'g100' : 'g10'
}

export interface ThemeContextValue {
  /** The stored preference: "system" or an explicit theme name. */
  preference: ThemePreference
  /** The concrete Carbon theme currently applied to <html>. */
  theme: ThemeName
  setPreference: (preference: ThemePreference) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function readPreference(): ThemePreference {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if ((THEME_PREFERENCES as readonly string[]).includes(stored ?? '')) {
      return stored as ThemePreference
    }
  } catch {
    // localStorage unavailable (private mode etc.) - fall through to default.
  }
  return 'system'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(readPreference)
  const [systemDark, setSystemDark] = useState(
    () => window.matchMedia(DARK_QUERY).matches,
  )

  // Track the OS color scheme so the "system" preference follows it live.
  useEffect(() => {
    const mq = window.matchMedia(DARK_QUERY)
    const onChange = (e: MediaQueryListEvent) => setSystemDark(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const theme = resolveTheme(preference, systemDark)

  // Apply the resolved theme class to <html> before paint (layout effect, so
  // the header and the content area never show different themes for a frame)
  // and persist the preference. The class is also set by an inline script in
  // index.html so the very first paint already matches; this keeps it in sync
  // afterwards.
  useLayoutEffect(() => {
    const root = document.documentElement
    for (const name of THEME_NAMES) root.classList.remove(`cds--${name}`)
    root.classList.add(`cds--${theme}`)
    try {
      localStorage.setItem(STORAGE_KEY, preference)
    } catch {
      // Ignore storage failures; the in-memory preference still applies.
    }
  }, [theme, preference])

  const setPreference = useCallback((pref: ThemePreference) => setPreferenceState(pref), [])

  const value = useMemo(
    () => ({ preference, theme, setPreference }),
    [preference, theme, setPreference],
  )
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider')
  return ctx
}
