import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { classroomType, classroomStatus, formatDateTime } from '../../utils/format'

interface Classroom {
  id: number
  name: string
  building: string
  capacity: number
  type: string
  floor: string
  campus: string
  status: string
  description: string
  createdAt: string
}

interface ClassroomForm {
  name: string
  building: string
  capacity: string
  type: string
  floor: string
  campus: string
  status: string
  description: string
}

const emptyForm: ClassroomForm = {
  name: '',
  building: '',
  capacity: '',
  type: 'standard',
  floor: '',
  campus: '',
  status: 'available',
  description: ''
}

const TYPE_KEYS = Object.keys(classroomType)
const STATUS_KEYS = Object.keys(classroomStatus)

/** 校验文案与 web 端 ClassroomsPage 一致。 */
function validate(form: ClassroomForm): string {
  if (!form.name.trim()) return '教室编号为必填项'
  if (!form.capacity || Number(form.capacity) <= 0) return '座位数必须大于 0'
  return ''
}

function buildBody(form: ClassroomForm) {
  return {
    name: form.name.trim(),
    building: form.building.trim(),
    capacity: Number(form.capacity),
    type: form.type,
    floor: form.floor.trim(),
    campus: form.campus.trim(),
    status: form.status,
    description: form.description.trim()
  }
}

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    canManage: false,
    typeOptions: TYPE_KEYS.map((k) => classroomType[k]),
    statusOptions: STATUS_KEYS.map((k) => classroomStatus[k].text),
    // 新建/编辑共用弹层(editTarget 为空即新建)
    formOpen: false,
    formLoading: false,
    formError: '',
    formTitle: '添加教室',
    form: { ...emptyForm },
    typeIndex: 0,
    statusIndex: 0,
    editId: 0,
    actingId: 0
  },

  _list: null as any,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('classroom:manage') })
    this._list = createPagedList({
      path: '/api/classrooms',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((c: Classroom) => this.rowView(c))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
  },

  rowView(c: Classroom) {
    const st = classroomStatus[c.status] || { text: c.status, theme: 'gray' }
    return {
      ...c,
      typeText: classroomType[c.type] || c.type,
      statusText: st.text,
      statusTheme: st.theme,
      createdAtText: formatDateTime(c.createdAt)
    }
  },

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 表单 ----

  openCreate() {
    this.setData({
      formOpen: true,
      formTitle: '添加教室',
      form: { ...emptyForm },
      typeIndex: 0,
      statusIndex: 0,
      formError: '',
      editId: 0
    })
  },

  openEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      formOpen: true,
      formTitle: '编辑教室',
      form: {
        name: row.name,
        building: row.building || '',
        capacity: String(row.capacity),
        type: row.type,
        floor: row.floor || '',
        campus: row.campus || '',
        status: row.status,
        description: row.description || ''
      },
      typeIndex: Math.max(0, TYPE_KEYS.indexOf(row.type)),
      statusIndex: Math.max(0, STATUS_KEYS.indexOf(row.status)),
      formError: '',
      editId: row.id
    })
  },

  closeForm() {
    if (this.data.formLoading) return
    this.setData({ formOpen: false })
  },

  onNameInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.name': e.detail.value })
  },

  onBuildingInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.building': e.detail.value })
  },

  onCapacityInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.capacity': e.detail.value })
  },

  onTypeChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ typeIndex: idx, 'form.type': TYPE_KEYS[idx] })
  },

  onFloorInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.floor': e.detail.value })
  },

  onCampusInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.campus': e.detail.value })
  },

  onStatusChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ statusIndex: idx, 'form.status': STATUS_KEYS[idx] })
  },

  onDescriptionInput(e: WechatMiniprogram.Input) {
    this.setData({ 'form.description': e.detail.value })
  },

  async submitForm() {
    const msg = validate(this.data.form)
    if (msg) {
      this.setData({ formError: msg })
      return
    }
    this.setData({ formLoading: true, formError: '' })
    try {
      if (this.data.editId) {
        await request({ path: `/api/classrooms/${this.data.editId}`, method: 'PUT', data: buildBody(this.data.form) })
      } else {
        await request({ path: '/api/classrooms', method: 'POST', data: buildBody(this.data.form) })
      }
      this.setData({ formOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ formError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ formLoading: false })
    }
  },

  // ---- 删除 ----

  onTapDelete(e: WechatMiniprogram.TouchEvent) {
    const { id, name } = e.currentTarget.dataset
    wx.showModal({
      title: '删除教室',
      content: `确定要删除教室「${name}」吗？此操作不可撤销。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(Number(id))
      }
    })
  },

  async handleDelete(id: number) {
    this.setData({ actingId: id })
    try {
      await request({ path: `/api/classrooms/${id}`, method: 'DELETE' })
      this._list.reload()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
