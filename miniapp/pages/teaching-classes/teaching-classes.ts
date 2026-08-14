import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { formatDateTime } from '../../utils/format'

interface AdminClassOption {
  id: number
  grade: string
  name: string
}

interface TeachingClass {
  id: number
  name: string
  note: string
  classes: { id: number; grade: string; name: string }[]
  createdAt: string
}

const emptyForm = { name: '', note: '', classIds: [] as number[] }

/** 校验文案与 web 端 TeachingClassesPage 一致。 */
function validate(form: { name: string; classIds: number[] }): string {
  if (!form.name.trim()) return '教学班名称为必填项'
  if (!form.classIds.length) return '至少选择一个行政班'
  return ''
}

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    canManage: false,
    noAdminClasses: false,
    formOpen: false,
    formLoading: false,
    formError: '',
    formTitle: '添加教学班',
    form: { ...emptyForm },
    editId: 0,
    // 成员多选弹层
    pickerOpen: false,
    pickerItems: [] as { id: number; text: string; checked: boolean }[],
    actingId: 0
  },

  _list: null as any,
  _adminClasses: [] as AdminClassOption[],

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('teaching_class:manage') })
    this._list = createPagedList({
      path: '/api/teaching-classes',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((t: TeachingClass) => ({
          ...t,
          classesText: (t.classes || []).map((c) => `${c.grade || ''}${c.name}`.trim()).join('、') || '—',
          createdAtText: formatDateTime(t.createdAt)
        }))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
    await this.loadAdminClasses()
  },

  async loadAdminClasses() {
    try {
      const data = await request<{ items: AdminClassOption[] }>({ path: '/api/admin-classes', params: { page_size: 500 } })
      this._adminClasses = (data && data.items) || []
      this.setData({ noAdminClasses: this._adminClasses.length === 0 })
    } catch {
      this._adminClasses = []
      this.setData({ noAdminClasses: true })
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
    this.setData({ formOpen: true, formTitle: '添加教学班', form: { ...emptyForm }, formError: '', editId: 0 })
  },

  openEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      formOpen: true,
      formTitle: '编辑教学班',
      form: {
        name: row.name,
        note: row.note || '',
        classIds: (row.classes || []).map((c: any) => Number(c.id))
      },
      formError: '',
      editId: row.id
    })
  },

  closeForm() {
    if (this.data.formLoading) return
    this.setData({ formOpen: false })
  },

  onFormInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`form.${field}`]: e.detail.value })
  },

  // ---- 成员多选 ----

  openPicker() {
    this.setData({
      pickerOpen: true,
      pickerItems: this._adminClasses.map((c) => ({
        id: c.id,
        text: `${c.grade || ''}${c.name}`.trim(),
        checked: this.data.form.classIds.includes(c.id)
      }))
    })
  },

  closePicker() {
    this.setData({ pickerOpen: false })
  },

  onPickerItemTap(e: WechatMiniprogram.TouchEvent) {
    const idx = Number(e.currentTarget.dataset.idx)
    const item = this.data.pickerItems[idx]
    this.setData({ [`pickerItems[${idx}].checked`]: !item.checked })
  },

  confirmPicker() {
    this.setData({
      pickerOpen: false,
      'form.classIds': this.data.pickerItems.filter((i) => i.checked).map((i) => i.id)
    })
  },

  // ---- 保存 ----

  async submitForm() {
    const msg = validate(this.data.form)
    if (msg) {
      this.setData({ formError: msg })
      return
    }
    this.setData({ formLoading: true, formError: '' })
    const body = {
      name: this.data.form.name.trim(),
      note: this.data.form.note.trim(),
      classIds: this.data.form.classIds.map(Number)
    }
    try {
      if (this.data.editId) {
        await request({ path: `/api/teaching-classes/${this.data.editId}`, method: 'PUT', data: body })
      } else {
        await request({ path: '/api/teaching-classes', method: 'POST', data: body })
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
      title: '删除教学班',
      content: `确定要删除教学班「${name}」吗？若已有开课引用该教学班，需先移除引用。此操作不可撤销。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(Number(id))
      }
    })
  },

  async handleDelete(id: number) {
    this.setData({ actingId: id })
    try {
      await request({ path: `/api/teaching-classes/${id}`, method: 'DELETE' })
      this._list.reload()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
