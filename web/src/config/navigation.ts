// Side-navigation configuration — the single injection point for the app
// shell's menu. Downstream forks override THIS data file (same relative path)
// to add, remove or regroup menu entries; AppShell.tsx renders whatever is
// defined here. Re-merging an upstream release then costs one diff against a
// flat list instead of a 300-line component fork.
//
// Permission gates follow the backend catalog codes (see types/api.ts
// PermissionCode). A gate may be one code or an array meaning ANY-of.
// Absent gate = visible to every authenticated user.

import type { ComponentType } from 'react'
import {
  Building,
  Dashboard,
  Education,
  Settings,
  UserMultiple,
} from '@carbon/icons-react'

/** Carbon icon components accept a `size` prop in the app shell. */
export type NavIcon = ComponentType<{ size?: number | string }>

/** Any-of permission gate; absent = visible to all authenticated users. */
export type PermissionGate = string | string[]

export interface NavItem {
  path: string
  /** i18n key (common namespace, e.g. 'nav.classroomList'). */
  i18nKey?: string
  /** Literal label used when no i18n key exists (downstream entries). */
  label?: string
  permission?: PermissionGate
}

export interface NavGroup {
  i18nKey?: string
  label?: string
  icon: NavIcon
  /** Gate for the whole group; hidden entirely when it fails. */
  permission?: PermissionGate
  items: NavItem[]
}

/** A top-level entry is either a plain link or an expandable group. */
export type NavEntry =
  | ({ kind: 'link'; path: string; icon: NavIcon } & Omit<NavItem, 'path'>)
  | ({ kind: 'group'; icon: NavIcon } & Omit<NavGroup, 'icon'>)

export const navigation: NavEntry[] = [
  {
    kind: 'link',
    path: '/',
    i18nKey: 'nav.overview',
    icon: Dashboard,
  },
  {
    kind: 'group',
    i18nKey: 'nav.classroomManagement',
    icon: Building,
    items: [
      { path: '/classrooms', i18nKey: 'nav.classroomList' },
      { path: '/bookings', i18nKey: 'nav.classroomBooking' },
      {
        path: '/repairs',
        i18nKey: 'nav.classroomRepair',
        permission: ['repair:create', 'repair:assign'],
      },
    ],
  },
  {
    kind: 'group',
    i18nKey: 'nav.teachingManagement',
    icon: Education,
    items: [
      { path: '/courses', i18nKey: 'nav.courseManagement' },
      { path: '/timetable', i18nKey: 'nav.timetable' },
      { path: '/schedule-config', i18nKey: 'nav.scheduleConfig' },
      { path: '/imports', i18nKey: 'nav.dataImport', permission: 'course:manage' },
      { path: '/imports/split', i18nKey: 'nav.jwcSplit', permission: 'course:manage' },
      { path: '/attendance', i18nKey: 'nav.attendance', permission: 'attendance:read' },
      {
        path: '/attendance/report',
        i18nKey: 'nav.attendanceReport',
        permission: 'attendance:read',
      },
      { path: '/observations', i18nKey: 'nav.observations', permission: 'observation:read' },
    ],
  },
  {
    kind: 'group',
    i18nKey: 'nav.orgManagement',
    icon: UserMultiple,
    items: [
      { path: '/users', i18nKey: 'nav.userManagement', permission: 'user:read' },
      { path: '/admin-classes', i18nKey: 'nav.adminClasses' },
      { path: '/teaching-classes', i18nKey: 'nav.teachingClasses' },
      { path: '/roles', i18nKey: 'nav.roleManagement', permission: 'role:manage' },
      { path: '/groups', i18nKey: 'nav.groupManagement', permission: 'group:manage' },
    ],
  },
  {
    kind: 'group',
    i18nKey: 'nav.systemSettings',
    icon: Settings,
    permission: 'log:read',
    items: [
      { path: '/settings', i18nKey: 'nav.parameters', permission: '*' },
      { path: '/logs', i18nKey: 'nav.auditLogs' },
    ],
  },
]
