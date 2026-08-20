import i18n from './index'

// formatDate and formatDateTime centralise locale-aware formatting so every
// page stops hard-coding 'zh-CN'. They read the active i18next language.
// Pass Intl options to extend (e.g. { year: 'numeric' } on formatDateTime).

type DateLike = string | number | Date

export function formatDate(value: DateLike | null | undefined, opts: Intl.DateTimeFormatOptions = {}): string {
  if (!value) return '-'
  return new Date(value).toLocaleString(i18n.language || 'zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    ...opts,
  })
}

export function formatDateTime(value: DateLike | null | undefined, opts: Intl.DateTimeFormatOptions = {}): string {
  if (!value) return '-'
  return new Date(value).toLocaleString(i18n.language || 'zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    ...opts,
  })
}

export function formatNumber(value: string | number | null | undefined, opts: Intl.NumberFormatOptions = {}): string {
  if (value === null || value === undefined || value === '') return '-'
  return Number(value).toLocaleString(i18n.language || 'zh-CN', opts)
}
