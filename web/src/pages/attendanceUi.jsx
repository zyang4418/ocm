import { Tag } from '@carbon/react'
import { useTranslation } from 'react-i18next'
import i18n from '../i18n/index.js'

// Shared attendance UI helpers: status labels/colors and the record-status
// Tag used by the list, detail and report pages.

// Record-status enum values, in display order. Labels come from common.status
// so every page reads attendance statuses the same way.
export const STATUS_KEYS = ['present', 'late', 'absent', 'leave']

// Carbon Tag types per record status; absent is the only one worth a warning
// color, the rest stay neutral/positive.
const STATUS_COLOR = {
  present: 'green',
  late: 'yellow',
  absent: 'red',
  leave: 'gray',
}

export function StatusTag({ status }) {
  const { t } = useTranslation('common')
  return (
    <Tag type={STATUS_COLOR[status] || 'gray'} size="sm">
      {t('status.' + status, { defaultValue: status })}
    </Tag>
  )
}

export function CheckinStatusTag({ status }) {
  const { t } = useTranslation('attendance')
  return (
    <Tag type={status === 'active' ? 'blue' : 'gray'} size="sm">
      {t('checkinStatus.' + status, { defaultValue: status })}
    </Tag>
  )
}

// formatDateTime keeps the year (attendance records span time, so the year
// matters) and reads the active i18next language instead of hard-coding zh-CN.
export function formatDateTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString(i18n.language || 'zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
