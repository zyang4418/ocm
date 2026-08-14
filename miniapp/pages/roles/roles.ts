import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { request } from '../../utils/request'
import { formatDateTime } from '../../utils/format'

interface Role {
  id: number
  code: string
  name: string
  description: string
  isSystem: boolean
  permissions: string[]
  createdAt: string
}

interface PermGroup {
  name: string
  items: { code: string; label: string; checked: boolean }[]
}

/** 权限目录按 categoryName 分组(与 web 端 groupCatalog 一致)。 */
function groupCatalog(catalog: { code: string; name: string; categoryName: string }[]) {
  const groups = new Map<string, { code: string; name: string }[]>()
  for (const perm of catalog) {
    if (!groups.has(perm.categoryName)) groups.set(perm.categoryName, [])
    groups.get(perm.categoryName)!.push({ code: perm.code, name: perm.name })
  }
  return Array.from(groups, ([name, items]) => ({ name, items }))
}

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
    formTitle: '新建角色',
    form: { code: '', name: '', description: '' },
    permGroups: [] as PermGroup[],
    editId: 0,
    editIsSystem: false,
    actingId: 0
  },

  _catalog: [] as { code: string; name: string; categoryName: string }[],

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('role:manage') })
    await this.load()
  },

  async load() {
    this.setData({ loading: this.data.rows.length === 0, error: '' })
    try {
      const [roles, catalog] = await Promise.all([
        request<Role[]>({ path: '/api/roles' }),
        request<{ code: string; name: string; categoryName: string }[]>({ path: '/api/permissions' })
      ])
      this._catalog = catalog || []
      const rows = (roles || []).map((r) => ({
        ...r,
        isSystem: Boolean(r.isSystem),
        permText: (r.permissions || []).includes('*') ? '全部' : String((r.permissions || []).length),
        createdAtText: formatDateTime(r.createdAt)
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

  /** 以勾选集构建权限分组(预选已有权限)。 */
  buildPermGroups(checkedCodes: string[]): PermGroup[] {
    return groupCatalog(this._catalog).map((g) => ({
      name: g.name,
      items: g.items.map((p) => ({ code: p.code, label: `${p.name}（${p.code}）`, checked: checkedCodes.includes(p.code) }))
    }))
  },

  // ---- 表单 ----

  openCreate() {
    this.setData({
      formOpen: true,
      formTitle: '新建角色',
      form: { code: '', name: '', description: '' },
      permGroups: this.buildPermGroups([]),
      formError: '',
      editId: 0,
      editIsSystem: false
    })
  },

  openEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = this.data.rows.find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      formOpen: true,
      formTitle: '编辑角色',
      form: { code: row.code, name: row.name, description: row.description || '' },
      permGroups: this.buildPermGroups(row.permissions || []),
      formError: '',
      editId: row.id,
      editIsSystem: Boolean(row.isSystem)
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

  onPermTap(e: WechatMiniprogram.TouchEvent) {
    const gi = Number(e.currentTarget.dataset.gi)
    const pi = Number(e.currentTarget.dataset.pi)
    const checked = !this.data.permGroups[gi].items[pi].checked
    this.setData({ [`permGroups[${gi}].items[${pi}].checked`]: checked })
  },

  async submitForm() {
    const f = this.data.form
    const isCreate = !this.data.editId
    if (isCreate && !f.code.trim()) {
      this.setData({ formError: '角色代码为必填项' })
      return
    }
    if (!f.name.trim()) {
      this.setData({ formError: '角色名称为必填项' })
      return
    }
    const permissions: string[] = []
    for (const g of this.data.permGroups) {
      for (const p of g.items) {
        if (p.checked) permissions.push(p.code)
      }
    }
    this.setData({ formLoading: true, formError: '' })
    const body: any = { name: f.name.trim(), description: f.description.trim(), permissions }
    if (isCreate) body.code = f.code.trim()
    try {
      if (isCreate) {
        await request({ path: '/api/roles', method: 'POST', data: body })
      } else {
        await request({ path: `/api/roles/${this.data.editId}`, method: 'PUT', data: body })
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
      title: '删除角色',
      content: `确定要删除角色「${name}」吗？仍在使用的角色无法删除。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(Number(id))
      }
    })
  },

  async handleDelete(id: number) {
    this.setData({ actingId: id })
    try {
      await request({ path: `/api/roles/${id}`, method: 'DELETE' })
      await this.load()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
