import { Tag } from '@carbon/react'

// Shared attendance UI helpers: status labels/colors and the record-status
// Tag used by the list, detail and report pages.

export const STATUS_LABEL = {
  present: '出勤',
  late: '迟到',
  absent: '缺勤',
  leave: '请假',
}

// Carbon Tag types per record status; absent is the only one worth a warning
// color, the rest stay neutral/positive.
const STATUS_COLOR = {
  present: 'green',
  late: 'yellow',
  absent: 'red',
  leave: 'gray',
}

export function StatusTag({ status }) {
  return (
    <Tag type={STATUS_COLOR[status] || 'gray'} size="sm">
      {STATUS_LABEL[status] || status}
    </Tag>
  )
}

export function CheckinStatusTag({ status }) {
  return (
    <Tag type={status === 'active' ? 'blue' : 'gray'} size="sm">
      {status === 'active' ? '进行中' : '已结束'}
    </Tag>
  )
}

export function formatDateTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
