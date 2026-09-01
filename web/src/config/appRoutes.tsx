// Authenticated route table — the single injection point for page routing.
// Downstream forks override THIS data file (same relative path) to register
// their own pages; App.tsx renders the list verbatim. Adding a page upstream
// is one entry here; adding one downstream is one entry in the overlay copy.
import type { ReactNode } from 'react'
import DashboardPage from '../pages/DashboardPage'
import BookingsPage from '../pages/BookingsPage'
import ClassroomsPage from '../pages/ClassroomsPage'
import CourseManagementPage from '../pages/CourseManagementPage'
import ScheduleConfigPage from '../pages/ScheduleConfigPage'
import TimetablePage from '../pages/TimetablePage'
import ImportsPage from '../pages/ImportsPage'
import ImportDetailPage from '../pages/ImportDetailPage'
import SplitPage from '../pages/SplitPage'
import UsersPage from '../pages/UsersPage'
import RolesPage from '../pages/RolesPage'
import GroupsPage from '../pages/GroupsPage'
import AdminClassesPage from '../pages/AdminClassesPage'
import TeachingClassesPage from '../pages/TeachingClassesPage'
import LogsPage from '../pages/LogsPage'
import SettingsPage from '../pages/SettingsPage'
import AttendancePage from '../pages/AttendancePage'
import AttendanceDetailPage from '../pages/AttendanceDetailPage'
import AttendanceReportPage from '../pages/AttendanceReportPage'
import ObservationsPage from '../pages/ObservationsPage'
import RepairsPage from '../pages/RepairsPage'

export interface AppRoute {
  path: string
  element: ReactNode
}

export const appRoutes: AppRoute[] = [
  { path: '/', element: <DashboardPage /> },
  { path: '/classrooms', element: <ClassroomsPage /> },
  { path: '/bookings', element: <BookingsPage /> },
  { path: '/courses', element: <CourseManagementPage /> },
  { path: '/schedule-config', element: <ScheduleConfigPage /> },
  { path: '/timetable', element: <TimetablePage /> },
  { path: '/imports', element: <ImportsPage /> },
  { path: '/imports/split', element: <SplitPage /> },
  { path: '/imports/:id', element: <ImportDetailPage /> },
  { path: '/users', element: <UsersPage /> },
  { path: '/roles', element: <RolesPage /> },
  { path: '/groups', element: <GroupsPage /> },
  { path: '/admin-classes', element: <AdminClassesPage /> },
  { path: '/teaching-classes', element: <TeachingClassesPage /> },
  { path: '/attendance', element: <AttendancePage /> },
  { path: '/attendance/report', element: <AttendanceReportPage /> },
  { path: '/attendance/:id', element: <AttendanceDetailPage /> },
  { path: '/observations', element: <ObservationsPage /> },
  { path: '/repairs', element: <RepairsPage /> },
  { path: '/logs', element: <LogsPage /> },
  { path: '/settings', element: <SettingsPage /> },
]
