import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { formatDateTime, effectiveLabel } from '../../utils/format'

interface Period {
  periodIndex: number
  startTime: string
  endTime: string
}

interface Regime {
  id: number
  name: string
  effectiveMonth: number
  effectiveDay: number
  periods: Period[]
  createdAt: string
}

interface RegimeForm {
  name: string
  effectiveMonth: number
  effectiveDay: string
}

interface PeriodRow {
  periodIndex: number
  startTime: string
  endTime: string
}

const MONTHS = ['1 月', '2 月', '3 月', '4 月', '5 月', '6 月', '7 月', '8 月', '9 月', '10 月', '11 月', '12 月']

const emptyRegime: RegimeForm = { name: '', effectiveMonth: 5, effectiveDay: '1' }

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    canManage: false,
    months: MONTHS,
    // 制度弹层
    regimeOpen: false,
    regimeLoading: false,
    regimeError: '',
    regimeTitle: '添加作息',
    regimeForm: { ...emptyRegime },
    monthIndex: 4,
    regimeEditId: 0,
    // 节次弹层
    periodsOpen: false,
    periodsLoading: false,
    periodsError: '',
    periodsTitle: '编辑节次',
    periodRows: [] as PeriodRow[],
    periodsTargetId: 0,
    actingId: 0
  },

  _list: null as any,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('course:manage') })
    this._list = createPagedList({
      path: '/api/schedule/regimes',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((r: Regime) => this.regimeView(r))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
  },

  regimeView(r: Regime) {
    const periods = (r.periods || []).slice().sort((a, b) => a.periodIndex - b.periodIndex)
    return {
      ...r,
      effectiveText: effectiveLabel(r),
      createdAtText: formatDateTime(r.createdAt),
      periodRows: periods,
      hasPeriods: periods.length > 0
    }
  },

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 制度新建/编辑 ----

  openRegimeCreate() {
    this.setData({
      regimeOpen: true,
      regimeTitle: '添加作息',
      regimeForm: { ...emptyRegime },
      monthIndex: 4,
      regimeError: '',
      regimeEditId: 0
    })
  },

  openRegimeEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      regimeOpen: true,
      regimeTitle: '编辑作息',
      regimeForm: { name: row.name, effectiveMonth: row.effectiveMonth, effectiveDay: String(row.effectiveDay) },
      monthIndex: Math.max(0, row.effectiveMonth - 1),
      regimeError: '',
      regimeEditId: row.id
    })
  },

  closeRegime() {
    if (this.data.regimeLoading) return
    this.setData({ regimeOpen: false })
  },

  onRegimeNameInput(e: WechatMiniprogram.Input) {
    this.setData({ 'regimeForm.name': e.detail.value })
  },

  onMonthChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ monthIndex: idx, 'regimeForm.effectiveMonth': idx + 1 })
  },

  onDayInput(e: WechatMiniprogram.Input) {
    this.setData({ 'regimeForm.effectiveDay': e.detail.value })
  },

  async submitRegime() {
    const f = this.data.regimeForm
    if (!f.name.trim()) {
      this.setData({ regimeError: '名称为必填项' })
      return
    }
    this.setData({ regimeLoading: true, regimeError: '' })
    const body = {
      name: f.name.trim(),
      effectiveMonth: Number(f.effectiveMonth),
      effectiveDay: Number(f.effectiveDay)
    }
    try {
      if (this.data.regimeEditId) {
        await request({ path: `/api/schedule/regimes/${this.data.regimeEditId}`, method: 'PUT', data: body })
      } else {
        await request({ path: '/api/schedule/regimes', method: 'POST', data: body })
      }
      this.setData({ regimeOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ regimeError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ regimeLoading: false })
    }
  },

  // ---- 节次编辑 ----

  openPeriods(e: WechatMiniprogram.TouchEvent) {
    const { id, name } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    const periods = (row.periods || []).slice().sort((a, b) => a.periodIndex - b.periodIndex)
    this.setData({
      periodsOpen: true,
      periodsTitle: `编辑节次 · ${name}`,
      // 空制度给一行默认节次(与 web 一致)
      periodRows: periods.length
        ? periods.map((p: Period) => ({ periodIndex: p.periodIndex, startTime: p.startTime, endTime: p.endTime }))
        : [{ periodIndex: 1, startTime: '08:00', endTime: '08:45' }],
      periodsError: '',
      periodsTargetId: row.id
    })
  },

  closePeriods() {
    if (this.data.periodsLoading) return
    this.setData({ periodsOpen: false })
  },

  onPeriodIndexInput(e: WechatMiniprogram.Input) {
    const idx = Number(e.currentTarget.dataset.idx)
    this.setData({ [`periodRows[${idx}].periodIndex`]: Number(e.detail.value) || 0 })
  },

  onPeriodStartChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.currentTarget.dataset.idx)
    this.setData({ [`periodRows[${idx}].startTime`]: e.detail.value })
  },

  onPeriodEndChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.currentTarget.dataset.idx)
    this.setData({ [`periodRows[${idx}].endTime`]: e.detail.value })
  },

  onPeriodRemove(e: WechatMiniprogram.TouchEvent) {
    const idx = Number(e.currentTarget.dataset.idx)
    const rows = this.data.periodRows.slice()
    rows.splice(idx, 1)
    this.setData({ periodRows: rows })
  },

  onPeriodAdd() {
    const rows = this.data.periodRows
    const nextIndex = rows.length ? Math.max(...rows.map((r) => r.periodIndex)) + 1 : 1
    this.setData({ periodRows: rows.concat([{ periodIndex: nextIndex, startTime: '08:00', endTime: '08:45' }]) })
  },

  async submitPeriods() {
    this.setData({ periodsLoading: true, periodsError: '' })
    const body = {
      periods: this.data.periodRows.map((p) => ({
        periodIndex: Number(p.periodIndex),
        startTime: p.startTime,
        endTime: p.endTime
      }))
    }
    try {
      await request({
        path: `/api/schedule/regimes/${this.data.periodsTargetId}/periods`,
        method: 'PUT',
        data: body
      })
      this.setData({ periodsOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ periodsError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ periodsLoading: false })
    }
  },

  // ---- 删除 ----

  onTapDelete(e: WechatMiniprogram.TouchEvent) {
    const { id, name } = e.currentTarget.dataset
    wx.showModal({
      title: '删除作息',
      content: `确定要删除作息「${name}」及其所有节次吗？此操作不可撤销。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(Number(id))
      }
    })
  },

  async handleDelete(id: number) {
    this.setData({ actingId: id })
    try {
      await request({ path: `/api/schedule/regimes/${id}`, method: 'DELETE' })
      this._list.reload()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
