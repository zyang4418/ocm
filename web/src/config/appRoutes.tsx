// Authenticated route table — the injection point for page routing.
// Two injection mechanisms coexist (mirroring src/brand):
//
//   1. Injection file (file DI, preferred): create `appRoutes.override.ts[x]`
//      next to this file exporting `overrideRoutes: AppRoute[]`. Vite resolves
//      the glob below at build time — when the file is absent the glob is
//      empty and nothing is added; when present its entries are appended after
//      the built-in routes. A downstream fork adds pages as NEW files and
//      never touches this one.
//   2. Whole-file override: overriding THIS data file (same relative path)
//      keeps working exactly as before.
//
// App.tsx renders the resulting list verbatim.
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
import IotDevicesPage from '../pages/IotDevicesPage'
import IotDeviceDetailPage from '../pages/IotDeviceDetailPage'

export interface AppRoute {
  path: string
  element: ReactNode
}

const builtInRoutes: AppRoute[] = [
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
  { path: '/iot', element: <IotDevicesPage /> },
  { path: '/iot/:id', element: <IotDeviceDetailPage /> },
  { path: '/logs', element: <LogsPage /> },
  { path: '/settings', element: <SettingsPage /> },
]

// File DI: entries from any appRoutes.override.* file in this directory are
// appended after the built-in routes (see the header comment for the contract).
const overrideModules = import.meta.glob<{ overrideRoutes?: AppRoute[] }>(
  './appRoutes.override.*',
  { eager: true },
)
const overrideRoutes: AppRoute[] = Object.values(overrideModules).flatMap(
  (m) => m.overrideRoutes ?? [],
)

/** Effective routes: built-ins first, then every override file's entries. */
export const appRoutes: AppRoute[] = [...builtInRoutes, ...overrideRoutes]
