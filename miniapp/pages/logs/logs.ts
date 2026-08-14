import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { addDays, fmtDate, formatDateTime } from '../../utils/format'

interface LogRow {
  id: number
  createdAt: string
  actorName: string
  summary: string
  method: string
  path: string
  statusCode: number
  clientIp: string
}

/** 状态码分段配色:2 成功 / 3 重定向 / 4 拒绝 / 5 错误。 */
function statusView(code: number) {
  const cls = Math.floor(code / 100)
  if (cls === 2) return { text: `成功 ${code}`, theme: 'green' }
  if (cls === 3) return { text: `重定向 ${code}`, theme: 'blue' }
  if (cls === 4) return { text: `拒绝 ${code}`, theme: 'orange' }
  return { text: `错误 ${code}`, theme: 'red' }
}

const RETENTION_MIN = 1
const RETENTION_MAX = 3650

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    from: fmtDate(addDays(new Date(), -30)),
    to: fmtDate(new Date()),
    canManage: false,
    retentionLoaded: false,
    retentionEnabled: false,
    retentionDays: 30,
    retentionSaving: false,
    retentionError: '',
    retentionValid: true
  },

  _list: null as any,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('log:manage') })
    this._list = createPagedList({
      path: '/api/logs',
      pageSize: 20,
      extraParams: () => ({ from: this.data.from, to: this.data.to }),
      setData: (p) => {
        const view = (p.items || []).map((l: LogRow) => this.logView(l))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
    this.loadRetention()
  },

  logView(l: LogRow) {
    const st = statusView(l.statusCode)
    return {
      ...l,
      statusText: st.text,
      statusTheme: st.theme,
      createdAtText: formatDateTime(l.createdAt),
      requestLine: `${l.method} ${l.path}`
    }
  },

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onFromChange(e: WechatMiniprogram.PickerChange) {
    const from = String(e.detail.value)
    if (from > this.data.to) {
      wx.showToast({ title: '开始日期不能晚于结束日期', icon: 'none' })
      return
    }
    this.setData({ from })
    this._list.load()
  },

  onToChange(e: WechatMiniprogram.PickerChange) {
    const to = String(e.detail.value)
    if (this.data.from > to) {
      wx.showToast({ title: '结束日期不能早于开始日期', icon: 'none' })
      return
    }
    this.setData({ to })
    this._list.load()
  },

  onClearFilters() {
    this.setData({ from: fmtDate(addDays(new Date(), -30)), to: fmtDate(new Date()) })
    this._list.load()
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 日志保留设置 ----

  async loadRetention() {
    if (!this.data.canManage) return
    try {
      const s = await request<{ retentionEnabled: boolean; retentionDays: number }>({ path: '/api/logs/settings' })
      this.setData({
        retentionLoaded: true,
        retentionEnabled: Boolean(s.retentionEnabled),
        retentionDays: Number(s.retentionDays) || 30
      })
    } catch {
      this.setData({ retentionLoaded: true })
    }
  },

  onRetentionToggle(e: WechatMiniprogram.CustomEvent) {
    this.setData({ retentionEnabled: Boolean(e.detail.value) })
  },

  onRetentionDaysInput(e: WechatMiniprogram.Input) {
    const days = Number(e.detail.value)
    this.setData({ retentionDays: Number(e.detail.value) || 0, retentionValid: days >= RETENTION_MIN && days <= RETENTION_MAX })
  },

  async saveRetention() {
    const days = Number(this.data.retentionDays)
    if (days < RETENTION_MIN || days > RETENTION_MAX) {
      this.setData({ retentionError: `保留天数需在 ${RETENTION_MIN}–${RETENTION_MAX} 之间` })
      return
    }
    this.setData({ retentionSaving: true, retentionError: '' })
    try {
      await request({
        path: '/api/logs/settings',
        method: 'PUT',
        data: { retentionEnabled: this.data.retentionEnabled, retentionDays: days }
      })
      wx.showToast({ title: '日志保留设置已保存', icon: 'success' })
    } catch (err: any) {
      this.setData({ retentionError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ retentionSaving: false })
    }
  }
})
