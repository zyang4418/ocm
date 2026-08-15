import { ensureAuth, getUser } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import {
  addDays, fmtDate, today, periodLabel, formatDateTime,
  bookingStatus, periodOptionLabel
} from '../../utils/format'

interface Classroom {
  id: number
  name: string
  status: string
}

interface Booking {
  id: number
  classroomName: string
  date: string
  periodStart: number
  periodEnd: number
  purpose: string
  status: string
  displayName: string
  userId: number
  createdAt: string
}

interface Period {
  periodIndex: number
  startTime: string
  endTime: string
}

const emptyCreate = { classroomId: '', date: '', periodStart: '', periodEnd: '', purpose: '' }

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    actionError: '',
    canApprove: false,
    canBook: false,
    meId: 0,
    // 状态筛选 chips
    statusChips: [
      { label: '全部', value: '' },
      { label: '待审批', value: 'pending' },
      { label: '已通过', value: 'approved' },
      { label: '已拒绝', value: 'rejected' },
      { label: '已取消', value: 'cancelled' }
    ],
    activeStatus: '',
    // 筛选
    classroomOptions: [] as string[],
    filterClassroomIndex: 0,
    from: today(),
    to: fmtDate(addDays(new Date(), 30)),
    // 新建预约弹层
    createOpen: false,
    createLoading: false,
    createError: '',
    createForm: { ...emptyCreate },
    createClassroomOptions: [] as string[],
    createClassroomIndex: 0,
    periodOptions: [] as string[],
    periodStartIndex: 0,
    periodEndIndex: 0,
    busyText: '',
    // 行内操作防连点
    actingId: 0,
    cancelTarget: null as Booking | null
  },

  _list: null as any,
  _classrooms: [] as Classroom[],
  _createIds: [] as number[],
  _periods: [] as Period[],
  _periodsSeq: 0,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    const me = getUser()
    this.setData({
      canApprove: can('booking:approve'),
      canBook: can('classroom:book'),
      meId: me ? me.id : 0
    })
    this._list = createPagedList({
      path: '/api/bookings',
      pageSize: 20,
      extraParams: () => {
        const { activeStatus, from, to } = this.data
        const cid = this._classrooms[this.data.filterClassroomIndex]
        return {
          classroom_id: cid ? cid.id : '',
          status: activeStatus,
          from,
          to
        }
      },
      setData: (p) => {
        const view = (p.items || []).map((b: Booking) => this.rowView(b))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
    this.loadClassrooms()
  },

  /** List row -> display fields + per-row action visibility. */
  rowView(b: Booking) {
    const st = bookingStatus[b.status] || { text: b.status, theme: 'gray' }
    return {
      ...b,
      periodText: periodLabel(b),
      createdAtText: formatDateTime(b.createdAt),
      statusText: st.text,
      statusTheme: st.theme,
      canCancel:
        (b.status === 'pending' || b.status === 'approved') &&
        (this.data.canApprove || this.data.meId === b.userId),
      canReview: this.data.canApprove && b.status === 'pending'
    }
  },

  async loadClassrooms() {
    try {
      const data = await request<{ items: Classroom[] }>({ path: '/api/classrooms', params: { page_size: 500 } })
      const items = (data && data.items) || []
      this._classrooms = items
      const avail = items.filter((c) => c.status === 'available')
      this._createIds = [0].concat(avail.map((c) => c.id))
      this.setData({
        classroomOptions: ['全部教室'].concat(items.map((c) => c.name)),
        createClassroomOptions: ['请选择教室'].concat(avail.map((c) => c.name))
      })
    } catch (err: any) {
      this.setData({ actionError: (err && err.message) || '加载失败' })
    }
  },

  // ---- 列表筛选 ----

  onTapStatus(e: WechatMiniprogram.TouchEvent) {
    const { value } = e.currentTarget.dataset
    this.setData({ activeStatus: value })
    this._list.load()
  },

  onFilterClassroomChange(e: WechatMiniprogram.PickerChange) {
    this.setData({ filterClassroomIndex: Number(e.detail.value) })
    this._list.load()
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

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 新建预约 ----

  openCreate() {
    this.setData({
      createOpen: true,
      createLoading: false,
      createError: '',
      createForm: { ...emptyCreate },
      createClassroomIndex: 0,
      periodOptions: [],
      periodStartIndex: 0,
      periodEndIndex: 0,
      busyText: ''
    })
    this._periods = []
  },

  closeCreate() {
    this.setData({ createOpen: false })
  },

  onCreateClassroomChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    const id = this._createIds[idx] || ''
    this.setData({ createClassroomIndex: idx, 'createForm.classroomId': String(id) })
    this.loadPeriods()
  },

  onCreateDateChange(e: WechatMiniprogram.PickerChange) {
    this.setData({ 'createForm.date': String(e.detail.value) })
    this.loadPeriods()
  },

  /** Classroom + date ready -> active regime periods + day's busy periods. */
  async loadPeriods() {
    const mySeq = ++this._periodsSeq
    const form = this.data.createForm
    const cid = form.classroomId
    const d = form.date
    if (!cid || !d) {
      this.setData({ periodOptions: [], busyText: '', createError: '' })
      return
    }
    try {
      const [regime, sessData, booksData] = await Promise.all([
        request<{ periods: Period[] }>({ path: '/api/schedule/active', params: { date: d } }),
        request<{ items: any[] }>({ path: '/api/sessions', params: { classroom_id: cid, from: d, to: d, page_size: 500 } }),
        request<{ items: any[] }>({ path: '/api/bookings', params: { classroom_id: cid, from: d, to: d, page_size: 500 } })
      ])
      if (mySeq !== this._periodsSeq) return
      const periods = ((regime && regime.periods) || []).slice().sort((a, b) => a.periodIndex - b.periodIndex)
      this._periods = periods
      const sess = (sessData && sessData.items) || []
      const books = (booksData && booksData.items) || []
      const busySet: Record<number, boolean> = {}
      sess.forEach((s) => {
        for (let i = s.periodStart; i <= s.periodEnd; i++) busySet[i] = true
      })
      books.forEach((b) => {
        if (b.status === 'pending' || b.status === 'approved') {
          for (let i = b.periodStart; i <= b.periodEnd; i++) busySet[i] = true
        }
      })
      const busy = Object.keys(busySet).map(Number).sort((a, b) => a - b)
      // 保持已选节次(仍存在时),否则默认第一节。
      const first = periods[0] ? periods[0].periodIndex : ''
      const keepStart = form.periodStart && periods.some((p) => p.periodIndex === Number(form.periodStart))
      const keepEnd = form.periodEnd && periods.some((p) => p.periodIndex === Number(form.periodEnd))
      const start = keepStart ? Number(form.periodStart) : Number(first)
      const end = keepEnd ? Number(form.periodEnd) : Number(first)
      this.setData({
        periodOptions: periods.map((p) => periodOptionLabel(p)),
        periodStartIndex: Math.max(0, periods.findIndex((p) => p.periodIndex === start)),
        periodEndIndex: Math.max(0, periods.findIndex((p) => p.periodIndex === end)),
        'createForm.periodStart': first ? String(start) : '',
        'createForm.periodEnd': first ? String(end) : '',
        busyText: busy.length ? `当日已占用节次：${busy.join('、')}` : '当日暂无占用节次',
        createError: ''
      })
    } catch (err: any) {
      if (mySeq !== this._periodsSeq) return
      this._periods = []
      this.setData({
        periodOptions: [],
        busyText: '',
        createError: err && err.statusCode === 404 ? '该日期未配置作息制度，无法预约' : (err && err.message) || '加载失败'
      })
    }
  },

  onCreatePeriodStartChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    const start = this._periods[idx] ? this._periods[idx].periodIndex : 0
    const end = this._periods[this.data.periodEndIndex]
    this.setData({
      periodStartIndex: idx,
      'createForm.periodStart': String(start),
      // 起始大于结束时,结束跟随起始(镜像 web)。
      ...(end && start > end.periodIndex
        ? { periodEndIndex: idx, 'createForm.periodEnd': String(start) }
        : {})
    })
  },

  onCreatePeriodEndChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    const end = this._periods[idx] ? this._periods[idx].periodIndex : 0
    this.setData({ periodEndIndex: idx, 'createForm.periodEnd': String(end) })
  },

  onCreatePurposeInput(e: WechatMiniprogram.Input) {
    this.setData({ 'createForm.purpose': e.detail.value })
  },

  async handleCreate() {
    const f = this.data.createForm
    if (!f.classroomId) return this.setData({ createError: '请选择教室' })
    if (!f.date) return this.setData({ createError: '请选择日期' })
    if (!f.periodStart || !f.periodEnd) return this.setData({ createError: '请选择节次' })
    if (Number(f.periodStart) > Number(f.periodEnd)) return this.setData({ createError: '起始节次不能大于结束节次' })
    if (!f.purpose.trim()) return this.setData({ createError: '用途为必填项' })
    this.setData({ createLoading: true, createError: '' })
    try {
      await request({
        path: '/api/bookings',
        method: 'POST',
        data: {
          classroomId: Number(f.classroomId),
          date: f.date,
          periodStart: Number(f.periodStart),
          periodEnd: Number(f.periodEnd),
          purpose: f.purpose.trim()
        }
      })
      this.setData({ createOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ createError: (err && err.message) || '提交失败' })
    } finally {
      this.setData({ createLoading: false })
    }
  },

  // ---- 行内操作 ----

  onTapCancelRow(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({ cancelTarget: row })
  },

  closeCancel() {
    this.setData({ cancelTarget: null })
  },

  async handleCancel() {
    const target = this.data.cancelTarget as any
    if (!target) return
    this.setData({ actingId: target.id, actionError: '' })
    try {
      await request({ path: `/api/bookings/${target.id}/cancel`, method: 'POST' })
      this.setData({ cancelTarget: null })
      this._list.reload()
    } catch (err: any) {
      this.setData({ actionError: (err && err.message) || '操作失败' })
    } finally {
      this.setData({ actingId: 0 })
    }
  },

  async onTapReview(e: WechatMiniprogram.TouchEvent) {
    const { id, decision } = e.currentTarget.dataset
    const bid = Number(id)
    this.setData({ actingId: bid, actionError: '' })
    try {
      await request({ path: `/api/bookings/${bid}/review`, method: 'POST', data: { decision } })
      this._list.reload()
    } catch (err: any) {
      this.setData({ actionError: (err && err.message) || '操作失败' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
