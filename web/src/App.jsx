import { BrowserRouter, Navigate, Route, Routes, Outlet, useLocation } from 'react-router-dom'
import { Loading } from '@carbon/react'
import { AuthProvider, useAuth } from './auth/AuthContext.jsx'
import AppShell from './components/AppShell.jsx'
import LoginPage from './pages/LoginPage.jsx'
import DashboardPage from './pages/DashboardPage.jsx'
import BookingsPage from './pages/BookingsPage.jsx'
import ClassroomsPage from './pages/ClassroomsPage.jsx'
import CourseManagementPage from './pages/CourseManagementPage.jsx'
import ScheduleConfigPage from './pages/ScheduleConfigPage.jsx'
import TimetablePage from './pages/TimetablePage.jsx'
import ImportsPage from './pages/ImportsPage.jsx'
import UsersPage from './pages/UsersPage.jsx'
import RolesPage from './pages/RolesPage.jsx'
import GroupsPage from './pages/GroupsPage.jsx'
import AdminClassesPage from './pages/AdminClassesPage.jsx'
import TeachingClassesPage from './pages/TeachingClassesPage.jsx'
import LogsPage from './pages/LogsPage.jsx'
import SettingsPage from './pages/SettingsPage.jsx'
import AiPage from './pages/AiPage.jsx'
import AttendancePage from './pages/AttendancePage.jsx'
import AttendanceDetailPage from './pages/AttendanceDetailPage.jsx'
import AttendanceReportPage from './pages/AttendanceReportPage.jsx'
import ObservationsPage from './pages/ObservationsPage.jsx'

function RequireAuth({ children }) {
  const { user, bootstrapping } = useAuth()
  const location = useLocation()

  if (bootstrapping) {
    return (
      <div className="app-loading">
        <Loading withOverlay={false} description="正在加载" />
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
            <Route path="/ai" element={<AiPage />} />
            <Route path="/classrooms" element={<ClassroomsPage />} />
            <Route path="/bookings" element={<BookingsPage />} />
            <Route path="/courses" element={<CourseManagementPage />} />
            <Route path="/schedule-config" element={<ScheduleConfigPage />} />
            <Route path="/timetable" element={<TimetablePage />} />
            <Route path="/imports" element={<ImportsPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/roles" element={<RolesPage />} />
            <Route path="/groups" element={<GroupsPage />} />
            <Route path="/admin-classes" element={<AdminClassesPage />} />
            <Route path="/teaching-classes" element={<TeachingClassesPage />} />
            <Route path="/attendance" element={<AttendancePage />} />
            <Route path="/attendance/report" element={<AttendanceReportPage />} />
            <Route path="/attendance/:id" element={<AttendanceDetailPage />} />
            <Route path="/observations" element={<ObservationsPage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
