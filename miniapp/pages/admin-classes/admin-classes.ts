import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { formatDateTime } from '../../utils/format'

interface AdminClass {
  id: number
  grade: string
  name: string
  note: string
  createdAt: string
}

const emptyForm = { grade: '', name: '', note: '' }

/** 校验文案与 web 端 AdminClassesPage 一致。 */
function validate(form: { name: string }): string {
  if (!form.name.trim()) return '班级名称为必填项'
  return ''
}

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    canManage: false,
    formOpen: false,
    formLoading: false,
    formError: '',
    formTitle: '添加行政班',
    form: { ...emptyForm },
    editId: 0,
    actingId: 0
  },

  _list: null as any,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('admin_class:manage') })
    this._list = createPagedList({
      path: '/api/admin-classes',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((c: AdminClass) => ({
          ...c,
          gradeText: c.grade ? `${c.grade}年级` : '—',
          createdAtText: formatDateTime(c.createdAt)
        }))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
  },

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 表单 ----

  openCreate() {
    this.setData({ formOpen: true, formTitle: '添加行政班', form: { ...emptyForm }, formError: '', editId: 0 })
  },

  openEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      formOpen: true,
      formTitle: '编辑行政班',
      form: { grade: row.grade || '', name: row.name, note: row.note || '' },
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

  async submitForm() {
    const msg = validate(this.data.form)
    if (msg) {
      this.setData({ formError: msg })
      return
    }
    this.setData({ formLoading: true, formError: '' })
    const body = {
      grade: this.data.form.grade.trim(),
      name: this.data.form.name.trim(),
      note: this.data.form.note.trim()
    }
    try {
      if (this.data.editId) {
        await request({ path: `/api/admin-classes/${this.data.editId}`, method: 'PUT', data: body })
      } else {
        await request({ path: '/api/admin-classes', method: 'POST', data: body })
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
      title: '删除行政班',
      content: `确定要删除行政班「${name}」吗？若该班已被教学班引用，需先移除引用。此操作不可撤销。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(Number(id))
      }
    })
  },

  async handleDelete(id: number) {
    this.setData({ actingId: id })
    try {
      await request({ path: `/api/admin-classes/${id}`, method: 'DELETE' })
      this._list.reload()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
