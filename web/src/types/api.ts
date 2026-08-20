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
