import { BrowserRouter, Navigate, Route, Routes, Outlet, useLocation } from 'react-router-dom'
import { Loading } from '@carbon/react'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { AuthProvider, useAuth } from './auth/AuthContext'
import LanguageProvider from './i18n/LanguageProvider'
import { ThemeProvider } from './theme/ThemeContext'
import AppShell from './components/AppShell'
import LoginPage from './pages/LoginPage'
import { appRoutes } from './config/appRoutes'

function RequireAuth({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const { user, bootstrapping } = useAuth()
  const location = useLocation()

  if (bootstrapping) {
    return (
      <div className="app-loading">
        <Loading withOverlay={false} description={t('loading')} />
      </div>
    )
  }
  if (!user) {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  return children
}

export default function App() {
  return (
    <LanguageProvider>
      <ThemeProvider>
        <AuthProvider>
          <BrowserRouter>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route
                element={
                  <RequireAuth>
                    <AppShell>
                      <Outlet />
                    </AppShell>
                  </RequireAuth>
                }
              >
                {/* Route table lives in src/config/appRoutes.tsx — the
                    injection point for downstream pages. */}
                {appRoutes.map(({ path, element }) => (
                  <Route key={path} path={path} element={element} />
                ))}
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </BrowserRouter>
        </AuthProvider>
      </ThemeProvider>
    </LanguageProvider>
  )
}
