// Hand-written companions to the generated api.d.ts. The generated file holds
// the OpenAPI contract (swaggo -> swagger2openapi -> openapi-typescript);
// this barrel adds what the spec cannot express (generic paged envelope,
// ApiError class) and re-exports schemas under clean names.
//
// Regenerate the contract with `npm run gen:types` after backend changes.

import type { components } from './api.d'

// ---- Generated schemas, re-exported without package prefixes ----

export type Checkin = components['schemas']['attendance.CheckinView']
export type CheckinCounts = components['schemas']['attendance.Counts']
export type CheckinInput = components['schemas']['attendance.CheckinInput']
export type CheckinRecord = components['schemas']['attendance.CheckinRecordView']
export type ScanResult = components['schemas']['attendance.ScanResult']
export type OfferingSummary = components['schemas']['attendance.OfferingSummary']
export type SummaryRow = components['schemas']['attendance.SummaryRow']

// ---- Generated schemas: classrooms / users / iam ----

export type Classroom = components['schemas']['classroom.Classroom']
export type ClassroomInput = components['schemas']['classroom.ClassroomInput']
export type RepairView = components['schemas']['classroom.RepairView']
export type RepairInput = components['schemas']['classroom.RepairInput']
export type RepairUpdateInput = components['schemas']['classroom.RepairUpdateInput']
export type User = components['schemas']['user.User']
export type CreateUserInput = components['schemas']['user.CreateUserInput']
export type UpdateUserInput = components['schemas']['user.UpdateUserInput']
export type UserGrantView = components['schemas']['iam.UserGrantView']
export type UserRolesInput = components['schemas']['user.UserRolesInput']
export type UserPermissionsInput = components['schemas']['user.UserPermissionsInput']
export type Role = components['schemas']['iam.Role']
export type RoleInput = components['schemas']['iam.RoleInput']
export type CatalogPermission = components['schemas']['authz.Permission']

// ---- Generated schemas: course domain / org ----

export type CatalogCourse = components['schemas']['course.CatalogCourse']
export type CatalogInput = components['schemas']['course.CatalogInput']
export type OfferingView = components['schemas']['course.OfferingView']
export type OfferingInput = components['schemas']['course.OfferingInput']
export type SessionView = components['schemas']['course.SessionView']
export type SessionInput = components['schemas']['course.SessionInput']
export type TimetableDay = components['schemas']['course.TimetableDay']
export type TimetableSlot = components['schemas']['course.TimetableSlot']
export type AdminClass = components['schemas']['user.AdminClass']
export type AdminClassInput = components['schemas']['user.AdminClassInput']
export type ClassRef = components['schemas']['user.ClassRef']
export type TeachingClassView = components['schemas']['user.TeachingClassView']
export type TeachingClassInput = components['schemas']['user.TeachingClassInput']
export type StudentProfileView = components['schemas']['user.StudentProfileView']
export type StudentProfileInput = components['schemas']['user.StudentProfileInput']

// ---- Generated schemas: booking / schedule / importer / groups ----

export type BookingView = components['schemas']['booking.BookingView']
export type BookingInput = components['schemas']['booking.BookingInput']
export type ReviewInput = components['schemas']['booking.ReviewInput']
export type DailyCount = components['schemas']['booking.DailyCount']
export type Regime = components['schemas']['schedule.Regime']
export type RegimeInput = components['schemas']['schedule.RegimeInput']
export type Period = components['schemas']['schedule.Period']
export type PeriodInput = components['schemas']['schedule.PeriodInput']
export type PeriodsInput = components['schemas']['schedule.PeriodsInput']
export type ImportJob = components['schemas']['importer.Job']
export type ImportRowError = components['schemas']['importer.RowError']
export type JobAccepted = components['schemas']['importer.JobAccepted']
export type JobErrors = components['schemas']['importer.JobErrors']
export type PreviewRowsPage = components['schemas']['importer.PreviewRowsPage']
export type SplitJobRef = components['schemas']['importer.SplitJobRef']
export type SplitResult = components['schemas']['importer.SplitResult']
export type SplitStats = components['schemas']['jwc.Stats']
export type GroupView = components['schemas']['iam.GroupView']
export type GroupDetail = components['schemas']['iam.GroupDetail']
export type GroupInput = components['schemas']['iam.GroupInput']
export type GroupMemberView = components['schemas']['iam.GroupMemberView']

// ---- Generated schemas: observation / logs / dashboard / settings ----

export type ObservationView = components['schemas']['observation.ObservationView']
export type ObservationInput = components['schemas']['observation.ObservationInput']
export type LogView = components['schemas']['systemlog.LogView']
export type LogRetentionSettings = components['schemas']['systemlog.Settings']
export type DashboardSummary = components['schemas']['dashboard.Summary']
export type DashboardPeriodCount = components['schemas']['dashboard.PeriodCount']
export type AiSettings = components['schemas']['ai.Settings']
export type AiMaskedSettings = components['schemas']['ai.MaskedSettings']
export type MailSettings = components['schemas']['mail.Settings']
export type MailMaskedSettings = components['schemas']['mail.MaskedSettings']
export type StorageSettings = components['schemas']['storage.Settings']
export type StorageMaskedSettings = components['schemas']['storage.MaskedSettings']

// ---- Paged envelope ----

// httpx.RespondPaged wraps every list endpoint as {items, total, page,
// pageSize}. The generated httpx.Paged types items as unknown (the wire type
// is any in Go); this generic restores the item type at the call site.
export interface Paged<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

// ---- Errors ----

// apiFetch rejects with ApiError so callers can branch on HTTP status
// (AuthContext treats 401 as session expiry).
export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// ---- Auth identity ----

// auth.UserView (internal/auth/handler.go): base account fields plus the
// resolved RBAC state, returned by /api/auth/login and /api/auth/me. Permissions
// stays string[] - the backend may emit strings outside the catalog (e.g. '*').
export type RoleBrief = components['schemas']['iam.RoleBrief']
export type GroupBrief = components['schemas']['iam.GroupBrief']
export type CurrentUser = components['schemas']['auth.UserView']
export type LoginResponse = components['schemas']['auth.LoginResponse']

// ---- Permissions ----

// Mirror of backend authz.Catalog (internal/authz/authz.go) plus the '*' wildcard
// that only the system admin role holds. Keep in sync with the catalog when
// adding permissions.
export type Permission =
  | '*'
  | 'user:manage'
  | 'user:read'
  | 'role:read'
  | 'role:manage'
  | 'group:read'
  | 'group:manage'
  | 'classroom:read'
  | 'classroom:manage'
  | 'classroom:book'
  | 'booking:approve'
  | 'course:read'
  | 'course:manage'
  | 'admin_class:read'
  | 'admin_class:manage'
  | 'teaching_class:read'
  | 'teaching_class:manage'
  | 'repair:create'
  | 'repair:assign'
  | 'log:read'
  | 'log:manage'
  | 'ai:chat'
  | 'attendance:read'
  | 'attendance:manage'
  | 'attendance:checkin'
  | 'observation:read'
  | 'observation:write'
  | 'observation:manage'
