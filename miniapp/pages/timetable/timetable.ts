import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { request } from '../../utils/request'
import { addDays, fmtDate, mondayOf, today, weekdayLabel, pad } from '../../utils/format'

interface Classroom {
  id: number
  name: string
}

interface Offering {
  id: number
  catalogName: string
  teachingClassName: string
  semester: string
}

interface SessionView {
  id: number
  offeringId: number
  courseName: string
  teachingClassName: string
  teacher: string
  periodStart: number
  periodEnd: number
  note: string
}

interface Slot {
  periodIndex: number
  startTime: string
  endTime: string
  session: SessionView | null
}

interface Day {
  date: string
  dayOfWeek: number
  regimeName: string
  slots: Slot[]
}

interface Cell {
  date: string
  session: SessionView | null
  isStart: boolean
  span: number
}

interface GridRow {
  periodIndex: number
  startTime: string
  endTime: string
  cells: Cell[]
}

const emptyForm = { offeringId: '', classroomId: '', date: '', periodStart: '', periodEnd: '', note: '' }

Page({
  data: {
    canManage: false,
    loading: false,
    error: '',
    classroomOptions: [] as string[],
    classroomIndex: 0,
    weekLabel: '',
    days: [] as Day[],
    gridRows: [] as GridRow[],
    cellHeight: 96,
    // 课次弹层
    modalOpen: false,
    modalLoading: false,
    modalError: '',
    modalTitle: '添加课次',
    form: { ...emptyForm },
    offeringOptions: [] as string[],
    offeringIndex: 0,
    editSessionId: 0,
    fixedInfo: ''
  },

  _classrooms: [] as Classroom[],
  _offerings: [] as Offering[],
  _weekStart: mondayOf(new Date()),
  _modalDate: '',
  _modalPeriodIndex: 0,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('course:manage') })
    try {
      const [cls, offs] = await Promise.all([
        request<{ items: Classroom[] }>({ path: '/api/classrooms', params: { page_size: 500 } }),
        request<{ items: Offering[] }>({ path: '/api/offerings', params: { page_size: 500 } })
      ])
      this._classrooms = (cls && cls.items) || []
      this._offerings = (offs && offs.items) || []
      this.setData({
        classroomOptions: this._classrooms.map((c) => c.name),
        offeringOptions: this._offerings.map((o) => `${o.catalogName} · ${o.teachingClassName} · ${o.semester}`)
      })
      if (this._classrooms.length) this.fetchTimetable()
    } catch (err: any) {
      this.setData({ error: (err && err.message) || '加载失败' })
    }
  },

  buildWeekLabel() {
    const ws = this._weekStart
    const we = addDays(ws, 6)
    this.setData({ weekLabel: `${fmtDate(ws)} ~ ${fmtDate(we)}` })
  },

  async fetchTimetable() {
    const c = this._classrooms[this.data.classroomIndex]
    if (!c) return
    this.buildWeekLabel()
    this.setData({ loading: true, error: '' })
    try {
      const from = fmtDate(this._weekStart)
      const to = fmtDate(addDays(this._weekStart, 6))
      const data = await request<Day[]>({
        path: '/api/timetable',
        params: { classroom_id: c.id, from, to }
      })
      const days = Array.isArray(data) ? data : []
      const rows = this.buildGrid(days)
      this.setData({ days: this.dayViews(days), gridRows: rows })
    } catch (err: any) {
      this.setData({ error: (err && err.message) || '加载失败' })
    } finally {
      this.setData({ loading: false })
    }
  },

  dayViews(days: Day[]) {
    return days.map((d) => {
      const dt = new Date(d.date + 'T00:00:00')
      return {
        ...d,
        dayName: weekdayLabel(dt),
        dateShort: `${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}`
      }
    })
  },

  /** 预计算网格:节次并集为行,cell 按天对齐;多节课次在起始行渲染并向下覆盖。 */
  buildGrid(days: Day[]): GridRow[] {
    const map: Record<number, { periodIndex: number; startTime: string; endTime: string }> = {}
    days.forEach((d) =>
      d.slots.forEach((s) => {
        if (!map[s.periodIndex]) map[s.periodIndex] = { periodIndex: s.periodIndex, startTime: s.startTime, endTime: s.endTime }
      })
    )
    const periods = Object.values(map).sort((a, b) => a.periodIndex - b.periodIndex)
    return periods.map((p) => ({
      ...p,
      cells: days.map((d) => {
        const slot = d.slots.find((s) => s.periodIndex === p.periodIndex)
        const session = slot && slot.session ? slot.session : null
        if (session && session.periodStart !== p.periodIndex) {
          // 延续行:由上方课次块覆盖,不渲染内容
          return { date: d.date, session, isStart: false, span: 0 }
        }
        return {
          date: d.date,
          session,
          isStart: Boolean(session),
          span: session ? session.periodEnd - session.periodStart + 1 : 1
        }
      })
    }))
  },

  onClassroomChange(e: WechatMiniprogram.PickerChange) {
    this.setData({ classroomIndex: Number(e.detail.value) })
    this.fetchTimetable()
  },

  onPrevWeek() {
    this._weekStart = addDays(this._weekStart, -7)
    this.fetchTimetable()
  },

  onNextWeek() {
    this._weekStart = addDays(this._weekStart, 7)
    this.fetchTimetable()
  },

  // ---- 课次编辑 ----

  onCellTap(e: WechatMiniprogram.CustomEvent) {
    if (!this.data.canManage) return
    const { date, periodIndex, session } = e.detail
    this._modalDate = date
    this._modalPeriodIndex = periodIndex
    this.setData({
      modalOpen: true,
      modalTitle: session ? '编辑课次' : '添加课次',
      fixedInfo: `${this._classrooms[this.data.classroomIndex].name} · ${date} · 第 ${periodIndex} 节`,
      form: {
        offeringId: session ? String(session.offeringId) : '',
        classroomId: String(this._classrooms[this.data.classroomIndex].id),
        date,
        periodStart: String(session ? session.periodStart : periodIndex),
        periodEnd: String(session ? session.periodEnd : periodIndex),
        note: session ? session.note || '' : ''
      },
      offeringIndex: session ? Math.max(0, this._offerings.findIndex((o) => o.id === session.offeringId)) : 0,
      editSessionId: session ? session.id : 0,
      modalError: ''
    })
  },

  closeModal() {
    if (this.data.modalLoading) return
    this.setData({ modalOpen: false })
  },

  onOfferingChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    const o = this._offerings[idx]
    this.setData({ offeringIndex: idx, 'form.offeringId': o ? String(o.id) : '' })
  },

  onPeriodStartInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.periodStart': e.detail.value })
  },

  onPeriodEndInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.periodEnd': e.detail.value })
  },

  onNoteInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.note': e.detail.value })
  },

  async submitSession() {
    const f = this.data.form
    if (!f.offeringId) {
      this.setData({ modalError: '请选择课程' })
      return
    }
    const periodStart = Number(f.periodStart)
    const periodEnd = f.periodEnd ? Number(f.periodEnd) : periodStart
    if (!periodStart || periodStart < 1 || periodEnd < periodStart) {
      this.setData({ modalError: '节次范围不合法：起始节次须 ≥1 且不大于结束节次' })
      return
    }
    this.setData({ modalLoading: true, modalError: '' })
    const body = {
      offeringId: Number(f.offeringId),
      classroomId: Number(f.classroomId),
      date: f.date,
      periodStart,
      periodEnd,
      note: f.note.trim()
    }
    try {
      if (this.data.editSessionId) {
        await request({ path: `/api/sessions/${this.data.editSessionId}`, method: 'PUT', data: body })
      } else {
        await request({ path: '/api/sessions', method: 'POST', data: body })
      }
      this.setData({ modalOpen: false })
      await this.fetchTimetable()
    } catch (err: any) {
      this.setData({ modalError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ modalLoading: false })
    }
  },

  async removeSession() {
    if (!this.data.editSessionId) return
    this.setData({ modalLoading: true, modalError: '' })
    try {
      await request({ path: `/api/sessions/${this.data.editSessionId}`, method: 'DELETE' })
      this.setData({ modalOpen: false })
      await this.fetchTimetable()
    } catch (err: any) {
      this.setData({ modalError: (err && err.message) || '删除失败' })
    } finally {
      this.setData({ modalLoading: false })
    }
  }
})
