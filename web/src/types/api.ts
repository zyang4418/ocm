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

// auth.userView (internal/auth/handler.go): base account fields plus the
// resolved RBAC state, returned by /api/auth/login and /api/auth/me. Permissions
// stays string[] - the backend may emit strings outside the catalog (e.g. '*').
export interface RoleBrief {
  id: number
  code: string
  name: string
}

export interface GroupBrief {
  id: number
  name: string
}

export interface CurrentUser {
  id: number
  username: string
  displayName: string
  type: string
  roles: RoleBrief[]
  groups: GroupBrief[]
  permissions: string[]
}

export interface LoginResponse {
  token: string
  user: CurrentUser
}

// ---- Import jobs ----

// importer.Job metadata (internal/importer/model.go) as served by
// GET /api/imports - payload/rows/errorReport travel on dedicated endpoints.
export interface ImportJob {
  id: number
  type: string
  status: string
  filename: string
  totalRows: number
  succeededRows: number
  failedRows: number
  userId: number
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export interface ImportRowError {
  row: number
  error: string
}

// /api/imports/jwc_split summary counts.
export interface SplitStats {
  classrooms: number
  catalogCourses: number
  adminClasses: number
  teachingClasses: number
  offerings: number
  sessions: number
  skippedEmptyAdmin: number
  skippedParallel: number
  noTeacherFilled: number
}

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
  | 'signage:read'
  | 'signage:manage'
