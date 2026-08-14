import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { request } from '../../utils/request'
import { formatDateTime } from '../../utils/format'

interface GroupRow {
  id: number
  name: string
  description: string
  memberCount: number
  createdAt: string
}

interface UserOption {
  id: number
  text: string
}

interface RoleOption {
  id: number
  text: string
}

const emptyForm = { name: '', description: '', members: [] as number[], roles: [] as number[] }

Page({
  data: {
    rows: [] as any[],
    loading: true,
    error: '',
    canManage: false,
    // 新建/编辑共用弹层
    formOpen: false,
    formLoading: false,
    formError: '',
    formTitle: '新建用户组',
    form: { ...emptyForm },
    editId: 0,
    // 成员/角色多选弹层(底部 popup)
    pickerOpen: false,
    pickerKind: '' as '' | 'members' | 'roles',
    pickerTitle: '',
    pickerItems: [] as { id: number; text: string; checked: boolean }[],
    actingId: 0
  },

  _userOptions: [] as UserOption[],
  _roleOptions: [] as RoleOption[],

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('group:manage') })
    await this.load()
  },

  async load() {
    this.setData({ loading: this.data.rows.length === 0, error: '' })
    try {
      const groups = await request<GroupRow[]>({ path: '/api/groups' })
      const rows = (groups || []).map((g) => ({
        ...g,
        createdAtText: formatDateTime(g.createdAt)
      }))
      this.setData({ rows })
    } catch (err: any) {
      this.setData({ error: (err && err.message) || '加载失败' })
    } finally {
      this.setData({ loading: false })
    }
  },

  onRetry() {
    this.load()
  },

  /** 多选弹层的选项源(成员/角色),懒加载一次。 */
  async loadPickerSources(): Promise<void> {
    if (this._userOptions.length || this._roleOptions.length) return
    try {
      const [usersPage, roles] = await Promise.all([
        request<{ items: any[] }>({ path: '/api/users', params: { page_size: 500 } }),
        request<RoleOption[]>({ path: '/api/roles' })
      ])
      this._userOptions = ((usersPage && usersPage.items) || []).map((u) => ({
        id: Number(u.id),
        text: `${u.displayName}（@${u.username}）`
      }))
      this._roleOptions = ((roles as any[]) || []).map((r) => ({
        id: Number(r.id),
        text: `${r.name}（${r.code}）`
      }))
    } catch {
      this._userOptions = []
      this._roleOptions = []
    }
  },

  // ---- 表单 ----

  openCreate() {
    this.setData({
      formOpen: true,
      formTitle: '新建用户组',
      form: { ...emptyForm },
      formError: '',
      editId: 0
    })
    this.loadPickerSources()
  },

  async openEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = this.data.rows.find((x) => x.id === Number(id))
    if (!row) return
    this.setData({ formOpen: true, formTitle: '编辑用户组', formError: '', editId: row.id })
    try {
      const [detail] = await Promise.all([
        request<{ name: string; description: string; members: { id: number }[]; roles: { id: number }[] }>({ path: `/api/groups/${row.id}` }),
        this.loadPickerSources()
      ])
      this.setData({
        form: {
          name: detail.name,
          description: detail.description || '',
          members: (detail.members || []).map((m) => Number(m.id)),
          roles: (detail.roles || []).map((r) => Number(r.id))
        }
      })
    } catch (err: any) {
      this.setData({ formError: (err && err.message) || '加载失败' })
    }
  },

  closeForm() {
    if (this.data.formLoading) return
    this.setData({ formOpen: false })
  },

  onFormInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`form.${field}`]: e.detail.value })
  },

  // ---- 多选弹层 ----

  openPicker(e: WechatMiniprogram.TouchEvent) {
    const kind = e.currentTarget.dataset.kind as 'members' | 'roles'
    const options = kind === 'members' ? this._userOptions : this._roleOptions
    const selected = kind === 'members' ? this.data.form.members : this.data.form.roles
    this.setData({
      pickerOpen: true,
      pickerKind: kind,
      pickerTitle: kind === 'members' ? '选择成员' : '选择角色',
      pickerItems: options.map((o) => ({ ...o, checked: selected.includes(o.id) }))
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
    const kind = this.data.pickerKind
    const ids = this.data.pickerItems.filter((i) => i.checked).map((i) => i.id)
    this.setData({
      pickerOpen: false,
      [`form.${kind}`]: ids
    })
  },

  // ---- 保存 ----

  async submitForm() {
    const f = this.data.form
    if (!f.name.trim()) {
      this.setData({ formError: '组名为必填项' })
      return
    }
    this.setData({ formLoading: true, formError: '' })
    const body = {
      name: f.name.trim(),
      description: f.description.trim(),
      members: f.members.map(Number),
      roles: f.roles.map(Number)
    }
    try {
      if (this.data.editId) {
        await request({ path: `/api/groups/${this.data.editId}`, method: 'PUT', data: body })
      } else {
        await request({ path: '/api/groups', method: 'POST', data: body })
      }
      this.setData({ formOpen: false })
      await this.load()
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
      title: '删除用户组',
      content: `确定要删除用户组「${name}」吗？组内成员的组级授权将一并撤销。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(Number(id))
      }
    })
  },

  async handleDelete(id: number) {
    this.setData({ actingId: id })
    try {
      await request({ path: `/api/groups/${id}`, method: 'DELETE' })
      await this.load()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
