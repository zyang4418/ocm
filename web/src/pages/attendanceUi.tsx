import { Tag, type TagProps } from '@carbon/react'
import { useTranslation } from 'react-i18next'
import { formatDate } from '../i18n/formatters'

// Shared attendance UI helpers: status labels/colors and the record-status
// Tag used by the list, detail and report pages.

// Record-status enum values, in display order. Labels come from common.status
// so every page reads attendance statuses the same way.
export const STATUS_KEYS = ['present', 'late', 'absent', 'leave'] as const
export type RecordStatus = (typeof STATUS_KEYS)[number]

// Carbon Tag types per record status: present positive, absent warning-red,
// leave neutral. Carbon Tag has no 'yellow' kind (bx--tag--yellow has no CSS),
// so 'late' uses purple to stay distinguishable from the gray leave tag.
const STATUS_COLOR: Record<RecordStatus, TagProps<'div'>['type']> = {
  present: 'green',
  late: 'purple',
  absent: 'red',
  leave: 'gray',
}

export function StatusTag({ status }: { status: string }) {
  const { t } = useTranslation('common')
  return (
    <Tag type={STATUS_COLOR[status as RecordStatus] ?? 'gray'} size="sm">
      {t(`status.${status}`, { defaultValue: status })}
    </Tag>
  )
}

export function CheckinStatusTag({ status }: { status: string }) {
  const { t } = useTranslation('attendance')
  return (
    <Tag type={status === 'active' ? 'blue' : 'gray'} size="sm">
      {t(`checkinStatus.${status}`, { defaultValue: status })}
    </Tag>
  )
}

// formatDateTime keeps the year (attendance records span time, so the year
// matters); it is i18n/formatDate with the year always on.
export function formatDateTime(value: string | number | Date | null | undefined): string {
  return formatDate(value)
}
