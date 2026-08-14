import { getUser } from './auth'

/**
 * UI-only permission gate, mirroring the web console's AuthContext.can().
 * The backend enforces permissions on every request; this only drives
 * button/menu visibility. '*' (system admin) passes everything.
 */
export function can(perm: string): boolean {
  const user = getUser()
  if (!user || !Array.isArray(user.permissions)) return false
  return user.permissions.includes('*') || user.permissions.includes(perm)
}

/** Any-of gate, for hub entries reachable with one of several permissions. */
export function canAny(perms: string[]): boolean {
  return perms.some((p) => can(p))
}
