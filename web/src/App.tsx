import { BrowserRouter, Navigate, Route, Routes, Outlet, useLocation } from 'react-router-dom'
import { Loading } from '@carbon/react'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { AuthProvider, useAuth } from './auth/AuthContext'
import LanguageProvider from './i18n/LanguageProvider'
import { ThemeProvider } from './theme/ThemeContext'
import AppShell from './components/AppShell'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import BookingsPage from './pages/BookingsPage'
import ClassroomsPage from './pages/ClassroomsPage'
import CourseManagementPage from './pages/CourseManagementPage'
import ScheduleConfigPage from './pages/ScheduleConfigPage'
import TimetablePage from './pages/TimetablePage'
import ImportsPage from './pages/ImportsPage'
import ImportDetailPage from './pages/ImportDetailPage'
import SplitPage from './pages/SplitPage'
import UsersPage from './pages/UsersPage'
import RolesPage from './pages/RolesPage'
import GroupsPage from './pages/GroupsPage'
import AdminClassesPage from './pages/AdminClassesPage'
import TeachingClassesPage from './pages/TeachingClassesPage'
import LogsPage from './pages/LogsPage'
import SettingsPage from './pages/SettingsPage'
import AttendancePage from './pages/AttendancePage'
import AttendanceDetailPage from './pages/AttendanceDetailPage'
import AttendanceReportPage from './pages/AttendanceReportPage'
import ObservationsPage from './pages/ObservationsPage'
import RepairsPage from './pages/RepairsPage'
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
            <Route path="/" element={<DashboardPage />} />
            <Route path="/classrooms" element={<ClassroomsPage />} />
            <Route path="/bookings" element={<BookingsPage />} />
            <Route path="/courses" element={<CourseManagementPage />} />
            <Route path="/schedule-config" element={<ScheduleConfigPage />} />
            <Route path="/timetable" element={<TimetablePage />} />
            <Route path="/imports" element={<ImportsPage />} />
            <Route path="/imports/split" element={<SplitPage />} />
            <Route path="/imports/:id" element={<ImportDetailPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/roles" element={<RolesPage />} />
            <Route path="/groups" element={<GroupsPage />} />
            <Route path="/admin-classes" element={<AdminClassesPage />} />
            <Route path="/teaching-classes" element={<TeachingClassesPage />} />
            <Route path="/attendance" element={<AttendancePage />} />
            <Route path="/attendance/report" element={<AttendanceReportPage />} />
            <Route path="/attendance/:id" element={<AttendanceDetailPage />} />
            <Route path="/observations" element={<ObservationsPage />} />
            <Route path="/repairs" element={<RepairsPage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
    </ThemeProvider>
    </LanguageProvider>
  )
}
