/**
 * Date/label formatting shared by list pages, mirroring the helpers
 * scattered across the web console pages (BookingsPage/TimetablePage/...).
 * Labels are copied verbatim from the web pages so both clients read alike.
 */

export interface StatusMeta {
  text: string
  theme: string // status-tag theme: blue | green | red | gray | orange | purple
}

export function pad(n: number): string {
  return String(n).padStart(2, '0')
}

/** Date -> 'YYYY-MM-DD' (the backend's date format everywhere). */
export function fmtDate(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function today(): string {
  return fmtDate(new Date())
}

export function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n)
}

/** Monday of the week containing d (local calendar math, avoids DST issues). */
export function mondayOf(d: Date): Date {
  const dow = d.getDay() // 0 = Sunday
  const diff = dow === 0 ? -6 : 1 - dow
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + diff)
}

const WEEKDAYS = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']

/** 周一..周日 label for a Date. */
export function weekdayLabel(d: Date): string {
  const dow = d.getDay()
  return WEEKDAYS[dow === 0 ? 6 : dow - 1]
}

export function periodLabel(b: { periodStart: number; periodEnd: number } | null | undefined): string {
  if (!b) return ''
  if (b.periodStart === b.periodEnd) return `第 ${b.periodStart} 节`
  return `第 ${b.periodStart}–${b.periodEnd} 节`
}

/** ISO timestamp -> 'YYYY-MM-DD HH:mm' (list rows show createdAt). */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '-'
  const d = new Date(value)
  if (isNaN(d.getTime())) return '-'
  return `${fmtDate(d)} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 第 X 节（HH:MM-HH:MM）— picker option / busy-hint text. */
export function periodOptionLabel(p: { periodIndex: number; startTime: string; endTime: string }): string {
  return `第 ${p.periodIndex} 节（${p.startTime}-${p.endTime}）`
}

export const bookingStatus: Record<string, StatusMeta> = {
  pending: { text: '待审批', theme: 'blue' },
  approved: { text: '已通过', theme: 'green' },
  rejected: { text: '已拒绝', theme: 'red' },
  cancelled: { text: '已取消', theme: 'gray' },
}

export const importStatus: Record<string, StatusMeta> = {
  pending: { text: '待处理', theme: 'blue' },
  processing: { text: '处理中', theme: 'orange' },
  preview: { text: '待确认', theme: 'purple' },
  succeeded: { text: '已完成', theme: 'green' },
  failed: { text: '失败', theme: 'red' },
  cancelled: { text: '已取消', theme: 'gray' },
}

export const classroomType: Record<string, string> = {
  standard: '普通教室',
  multimedia: '多媒体教室',
  computer: '机房',
  lab: '实验室',
  lecture_hall: '报告厅',
  stadium: '体育场',
  drawing: '制图教室',
  language: '听力教室',
  studio: '画室',
  special: '专用教室',
}

export const classroomStatus: Record<string, StatusMeta> = {
  available: { text: '可用', theme: 'green' },
  maintenance: { text: '维修中', theme: 'blue' },
  disabled: { text: '停用', theme: 'red' },
}

export const userType: Record<string, string> = {
  student: '学生',
  teacher: '教师',
  staff: '职员',
}

/** 作息制度的生效描述:「每年 M 月 D 日起生效」。 */
export function effectiveLabel(r: { effectiveMonth: number; effectiveDay: number }): string {
  return `每年 ${r.effectiveMonth} 月 ${r.effectiveDay} 日起生效`
}
