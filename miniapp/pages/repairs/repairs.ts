import { ensureAuth, getUser } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { repairStatus, formatDateTime } from '../../utils/format'

interface Repair {
  id: number
  classroomId: number
  creatorId: number
  assigneeId: number | null
  description: string
  images: string[]
  status: string
  remark: string
  classroomName: string
  building: string
  creatorName: string
  assigneeName: string
  createdAt: string
  updatedAt: string
}

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    actionError: '',
    canSubmit: false,
    canAssign: false,
    meId: 0,
    // 状态筛选 chips
    statusChips: [
      { label: '全部', value: '' },
      { label: '待处理', value: 'open' },
      { label: '处理中', value: 'processing' },
      { label: '待确认', value: 'completed' },
      { label: '已确认', value: 'confirmed' }
    ],
    activeStatus: '',
    // 提交报修弹层
    submitOpen: false,
    submitLoading: false,
    submitError: '',
    classroomOptions: [] as string[],
    classroomValues: [] as number[],
    classroomIndex: 0,
    description: '',
    // 处理弹层（开始处理 / 完成）
    processOpen: false,
    processLoading: false,
    processError: '',
    processTitle: '',
    processAction: '' as 'start' | 'finish',
    processId: 0,
    processRemark: '',
    // 行内操作防连点
    actingId: 0
  },

  _list: null as any,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    const me = getUser()
    this.setData({
      canSubmit: can('repair:create'),
      canAssign: can('repair:assign'),
      meId: me ? me.id : 0
    })
    this._list = createPagedList({
      path: '/api/repairs',
      pageSize: 20,
      extraParams: () => ({ status: this.data.activeStatus }),
      setData: (p) => {
        const view = (p.items || []).map((r: Repair) => this.rowView(r))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
    if (this.data.canSubmit) this.loadClassrooms()
  },

  async loadClassrooms() {
    try {
      const data = await request<{ items: any[] }>({ path: '/api/classrooms', params: { page_size: 500 } })
      const rooms = (data && data.items) || []
      this.setData({
        classroomOptions: ['请选择教室'].concat(rooms.map((c) => (c.building ? `${c.building} ${c.name}` : c.name))),
        classroomValues: [0].concat(rooms.map((c) => Number(c.id)))
      })
    } catch (err: any) {
      this.setData({ actionError: (err && err.message) || '加载教室列表失败' })
    }
  },

  rowView(r: Repair) {
    const st = repairStatus[r.status] || { text: r.status, theme: 'gray' }
    const own = this.data.meId === r.creatorId
    return {
      ...r,
      classroomLabel: r.building ? `${r.building} ${r.classroomName}` : r.classroomName,
      statusText: st.text,
      statusTheme: st.theme,
      createdAtText: formatDateTime(r.createdAt),
      canStart: this.data.canAssign && r.status === 'open',
      canFinish: this.data.canAssign && r.status === 'processing',
      canConfirm: own && r.status === 'completed'
    }
  },

  // ---- 列表筛选 ----
  onTapStatus(e: WechatMiniprogram.TouchEvent) {
    const { value } = e.currentTarget.dataset
    this.setData({ activeStatus: value })
    this._list.load()
  },

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 提交报修 ----
  openSubmit() {
    this.setData({
      submitOpen: true,
      submitError: '',
      classroomIndex: 0,
      description: ''
    })
  },

  closeSubmit() {
    if (this.data.submitLoading) return
    this.setData({ submitOpen: false })
  },

  onClassroomChange(e: WechatMiniprogram.PickerChange) {
    this.setData({ classroomIndex: Number(e.detail.value) })
  },

  onDescriptionInput(e: WechatMiniprogram.Input) {
    this.setData({ description: e.detail.value })
  },

  async submitRepair() {
    const classroomId = this.data.classroomValues[this.data.classroomIndex]
    const description = this.data.description.trim()
    if (!classroomId) {
      this.setData({ submitError: '请选择教室' })
      return
    }
    if (!description) {
      this.setData({ submitError: '请填写故障描述' })
      return
    }
    this.setData({ submitLoading: true, submitError: '' })
    try {
      await request({ path: '/api/repairs', method: 'POST', data: { classroomId, description } })
      wx.showToast({ title: '已提交', icon: 'success' })
      this.setData({ submitOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ submitError: (err && err.message) || '提交失败' })
    } finally {
      this.setData({ submitLoading: false })
    }
  },

  // ---- 处理报修 ----
  openProcess(e: WechatMiniprogram.TouchEvent) {
    const id = Number(e.currentTarget.dataset.id)
    const action = e.currentTarget.dataset.action as 'start' | 'finish'
    this.setData({
      processOpen: true,
      processError: '',
      processId: id,
      processAction: action,
      processTitle: action === 'start' ? '开始处理' : '完成报修',
      processRemark: ''
    })
  },

  closeProcess() {
    if (this.data.processLoading) return
    this.setData({ processOpen: false })
  },

  onProcessRemarkInput(e: WechatMiniprogram.Input) {
    this.setData({ processRemark: e.detail.value })
  },

  async submitProcess() {
    const status = this.data.processAction === 'start' ? 'processing' : 'completed'
    this.setData({ processLoading: true, processError: '' })
    try {
      await request({
        path: `/api/repairs/${this.data.processId}`,
        method: 'PUT',
        data: { status, remark: this.data.processRemark.trim() }
      })
      wx.showToast({ title: '已更新', icon: 'success' })
      this.setData({ processOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ processError: (err && err.message) || '操作失败' })
    } finally {
      this.setData({ processLoading: false })
    }
  },

  // ---- 确认完成 ----
  onConfirm(e: WechatMiniprogram.TouchEvent) {
    const id = Number(e.currentTarget.dataset.id)
    const row = (this.data.list.items as any[]).find((x) => x.id === id)
    const name = row ? row.classroomLabel : '该报修'
    wx.showModal({
      title: '确认完成',
      content: `确认「${name}」的维修已完成？`,
      confirmColor: '#2B5FF6',
      success: async (res) => {
        if (!res.confirm) return
        this.setData({ actingId: id, actionError: '' })
        try {
          await request({ path: `/api/repairs/${id}/confirm`, method: 'POST' })
          wx.showToast({ title: '已确认', icon: 'success' })
          this._list.reload()
        } catch (err: any) {
          this.setData({ actionError: (err && err.message) || '确认失败' })
        } finally {
          this.setData({ actingId: 0 })
        }
      }
    })
  }
})
