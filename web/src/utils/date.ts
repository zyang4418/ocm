// Week-grid date helpers shared by the timetable page and the session-form
// timetable picker.

/** Formats a Date as "YYYY-MM-DD" (local time). */
export function fmt(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

/** Returns the Monday (00:00 local) of d's week. */
export function mondayOf(d: Date): Date {
  const r = new Date(d)
  r.setHours(0, 0, 0, 0)
  r.setDate(r.getDate() - ((r.getDay() + 6) % 7))
  return r
}

/** Parses "YYYY-MM-DD" as a local Date, or null when malformed/empty. */
export function parseDate(s: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return null
  const y = Number(s.slice(0, 4))
  const m = Number(s.slice(5, 7))
  const d = Number(s.slice(8, 10))
  const r = new Date(y, m - 1, d)
  // Reject e.g. 2026-02-31, where the constructor rolls over to the next month.
  if (r.getFullYear() !== y || r.getMonth() !== m - 1 || r.getDate() !== d) return null
  return r
}

/** Returns a new Date n days after d (time of day preserved). */
export function addDays(d: Date, n: number): Date {
  const r = new Date(d)
  r.setDate(r.getDate() + n)
  return r
}
