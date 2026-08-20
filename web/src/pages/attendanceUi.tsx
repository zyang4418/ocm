import { Tag, type TagProps } from '@carbon/react'
import { useTranslation } from 'react-i18next'
import { formatDate } from '../i18n/formatters'

// Shared attendance UI helpers: status labels/colors and the record-status
// Tag used by the list, detail and report pages.

// Record-status enum values, in display order. Labels come from common.status
// so every page reads attendance statuses the same way.
export const STATUS_KEYS = ['present', 'late', 'absent', 'leave'] as const
export type RecordStatus = (typeof STATUS_KEYS)[number]

// Carbon Tag types per record status; absent is the only one worth a warning
// color, the rest stay neutral/positive. 'late' is 'yellow', which is NOT a
// real Carbon tag kind (bx--tag--yellow has no CSS) - kept to preserve the
// existing rendering until the status colors are revisited.
const STATUS_COLOR: Record<RecordStatus, string> = {
  present: 'green',
  late: 'yellow',
  absent: 'red',
  leave: 'gray',
}

export function StatusTag({ status }: { status: string }) {
  const { t } = useTranslation('common')
  return (
    <Tag type={(STATUS_COLOR[status as RecordStatus] ?? 'gray') as TagProps<'div'>['type']} size="sm">
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
