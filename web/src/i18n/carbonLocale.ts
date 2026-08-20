import i18n from './index'

// Carbon React does not ship a global locale context. This module provides
// helpers that map the active language to Carbon props and translateWithId
// callbacks.

type CarbonMessage = string | ((count: number) => string)

const CARBON_MESSAGE_IDS: Record<string, { 'zh-CN': CarbonMessage; en: CarbonMessage }> = {
  'carbon.table.toolbar.search.label': {
    'zh-CN': '筛选表',
    en: 'Filter table',
  },
  'carbon.table.toolbar.search.placeholder': {
    'zh-CN': '筛选表',
    en: 'Filter table',
  },
  'carbon.table.batch.cancel': {
    'zh-CN': '取消',
    en: 'Cancel',
  },
  'carbon.table.batch.item.selected': {
    'zh-CN': '已选择 1 项',
    en: '1 item selected',
  },
  'carbon.table.batch.items.selected': {
    'zh-CN': (n) => `已选择 ${n} 项`,
    en: (n) => `${n} items selected`,
  },
  'carbon.table.batch.selectAll': {
    'zh-CN': '全选所有行',
    en: 'Select all rows',
  },
  'carbon.table.header.icon.description': {
    'zh-CN': '排序',
    en: 'Sort rows by this header',
  },
  'carbon.pagination-nav.next': {
    'zh-CN': '下一页',
    en: 'Next',
  },
  'carbon.pagination-nav.previous': {
    'zh-CN': '上一页',
    en: 'Previous',
  },
  'carbon.list-box.input.clear.selection': {
    'zh-CN': '清除所选内容',
    en: 'Clear selection',
  },
}

export function carbonTranslateWithId(messageId: string, args?: { count?: number }): string {
  const entry = CARBON_MESSAGE_IDS[messageId]
  if (!entry) return messageId
  const value = entry[i18n.language as 'zh-CN' | 'en'] ?? entry['zh-CN']
  return typeof value === 'function' ? value(args?.count ?? 0) : value
}

export function paginationProps() {
  const lng = i18n.language || 'zh-CN'
  if (lng === 'en') {
    return {
      itemRangeText: (min: number, max: number, total: number) => `${min}–${max} of ${total} items`,
      itemsPerPageText: 'Items per page',
      pageRangeText: (current: number, total: number) => `of ${total} pages`,
      pageText: (page: number) => `page ${page}`,
      pageNumberText: 'Page Number',
      pageSizeText: 'Items per page',
      backwardText: 'Previous page',
      forwardText: 'Next page',
    }
  }
  return {
    itemRangeText: (min: number, max: number, total: number) => `${min}–${max} / 共 ${total} 条`,
    itemsPerPageText: '每页条数',
    pageRangeText: (current: number, total: number) => `共 ${total} 页`,
    pageText: (page: number) => `第 ${page} 页`,
    pageNumberText: '页码',
    pageSizeText: '每页条数',
    backwardText: '上一页',
    forwardText: '下一页',
  }
}

export function datePickerLocale(): string {
  const lng = i18n.language || 'zh-CN'
  // Carbon DatePicker wraps flatpickr. Flatpickr uses 'zh' for Simplified
  // Chinese and defaults to English when no locale is passed.
  if (lng.startsWith('zh')) return 'zh'
  return 'en'
}
