import { ensureAuth, getUser } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { userType, formatDateTime } from '../../utils/format'

interface UserRow {
  id: number
  username: string
  displayName: string
  type: string
  roles: { id: number; name: string }[]
  groups: { id: number; name: string }[]
  createdAt: string
}

interface RoleOption {
  id: number
  code: string
  name: string
  isSystem: boolean
}

interface GrantRole extends RoleOption {
  checked: boolean
  expiresAt: string
  expired: boolean
}

interface PermItem {
  code: string
  label: string
  checked: boolean
}

interface PermGroup {
  name: string
  items: PermItem[]
}

/** ISO -> 'YYYY-MM-DD' 输入框值;空值留空。 */
function toDateInput(iso: string | null | undefined): string {
  return iso ? String(iso).slice(0, 10) : ''
}

/** 'YYYY-MM-DD' -> 本地零点 UTC 时间戳(与 web 端 localMidnightUTC 一致)。 */
function localMidnightUTC(dateStr: string): string {
  return new Date(dateStr + 'T00:00:00').toISOString()
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

const USER_TYPE_KEYS = Object.keys(userType)

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    canManage: false,
    meId: 0,
    typeOptions: USER_TYPE_KEYS.map((k) => userType[k]),
    // 新建
    createOpen: false,
    createLoading: false,
    createError: '',
    createForm: { username: '', password: '', displayName: '', type: 'staff' },
    createTypeIndex: USER_TYPE_KEYS.indexOf('staff'),
    // 编辑
    editOpen: false,
    editLoading: false,
    editError: '',
    editForm: { displayName: '', type: 'staff' },
    editTypeIndex: USER_TYPE_KEYS.indexOf('staff'),
    editTarget: null as UserRow | null,
    // 重置密码
    pwdOpen: false,
    pwdLoading: false,
    pwdError: '',
    pwdForm: { password: '', confirm: '' },
    pwdTarget: null as UserRow | null,
    // 授权
    grantOpen: false,
    grantLoading: false,
    grantSaving: false,
    grantError: '',
    grantTarget: null as UserRow | null,
    grantRoles: [] as GrantRole[],
    permGroups: [] as PermGroup[],
    grantGroups: [] as { id: number; name: string }[],
    actingId: 0
  },

  _list: null as any,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    const me = getUser()
    this.setData({ canManage: can('user:manage'), meId: me ? me.id : 0 })
    this._list = createPagedList({
      path: '/api/users',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((u: UserRow) => this.rowView(u))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
  },

  rowView(u: UserRow) {
    return {
      ...u,
      typeText: userType[u.type] || u.type,
      rolesText: (u.roles || []).map((r) => r.name).join('、') || '—',
      groupsText: (u.groups || []).map((g) => g.name).join('、') || '—',
      createdAtText: formatDateTime(u.createdAt),
      isSelf: Number(u.id) === this.data.meId
    }
  },

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 新建 ----

  openCreate() {
    this.setData({
      createOpen: true,
      createForm: { username: '', password: '', displayName: '', type: 'staff' },
      createTypeIndex: USER_TYPE_KEYS.indexOf('staff'),
      createError: ''
    })
  },

  closeCreate() {
    if (this.data.createLoading) return
    this.setData({ createOpen: false })
  },

  onCreateInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`createForm.${field}`]: e.detail.value })
  },

  onCreateTypeChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ createTypeIndex: idx, 'createForm.type': USER_TYPE_KEYS[idx] })
  },

  async submitCreate() {
    const f = this.data.createForm
    if (!f.username.trim() || !f.password || !f.displayName.trim()) {
      this.setData({ createError: '用户名、密码和显示名称均为必填项' })
      return
    }
    this.setData({ createLoading: true, createError: '' })
    try {
      await request({
        path: '/api/users',
        method: 'POST',
        data: {
          username: f.username.trim(),
          password: f.password,
          displayName: f.displayName.trim(),
          type: f.type
        }
      })
      this.setData({ createOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ createError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ createLoading: false })
    }
  },

  // ---- 编辑 ----

  openEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      editOpen: true,
      editForm: { displayName: row.displayName, type: row.type },
      editTypeIndex: Math.max(0, USER_TYPE_KEYS.indexOf(row.type)),
      editTarget: row,
      editError: ''
    })
  },

  closeEdit() {
    if (this.data.editLoading) return
    this.setData({ editOpen: false })
  },

  onEditInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`editForm.${field}`]: e.detail.value })
  },

  onEditTypeChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ editTypeIndex: idx, 'editForm.type': USER_TYPE_KEYS[idx] })
  },

  async submitEdit() {
    const f = this.data.editForm
    if (!f.displayName.trim()) {
      this.setData({ editError: '显示名称为必填项' })
      return
    }
    this.setData({ editLoading: true, editError: '' })
    try {
      await request({
        path: `/api/users/${(this.data.editTarget as any).id}`,
        method: 'PUT',
        data: { displayName: f.displayName.trim(), type: f.type }
      })
      this.setData({ editOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ editError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ editLoading: false })
    }
  },

  // ---- 重置密码 ----

  openPassword(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({ pwdOpen: true, pwdForm: { password: '', confirm: '' }, pwdTarget: row, pwdError: '' })
  },

  closePassword() {
    if (this.data.pwdLoading) return
    this.setData({ pwdOpen: false })
  },

  onPwdInput(e: WechatMiniprogram.Input) {
    const field = e.currentTarget.dataset.field
    this.setData({ [`pwdForm.${field}`]: e.detail.value })
  },

  async submitPassword() {
    const f = this.data.pwdForm
    if (!f.password) {
      this.setData({ pwdError: '请输入新密码' })
      return
    }
    if (f.password !== f.confirm) {
      this.setData({ pwdError: '两次输入的密码不一致' })
      return
    }
    this.setData({ pwdLoading: true, pwdError: '' })
    try {
      await request({
        path: `/api/users/${(this.data.pwdTarget as any).id}/password`,
        method: 'PATCH',
        data: { password: f.password }
      })
      this.setData({ pwdOpen: false })
    } catch (err: any) {
      this.setData({ pwdError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ pwdLoading: false })
    }
  },

  // ---- 授权 ----

  async openGrants(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.list.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({ grantOpen: true, grantLoading: true, grantError: '', grantTarget: row, grantRoles: [], permGroups: [], grantGroups: [] })
    try {
      const [grants, roles, catalog] = await Promise.all([
        request<{ roles: { code: string; expiresAt: string | null }[]; permissions: { permission: string; expiresAt: string | null }[]; groups: { id: number; name: string }[] }>({ path: `/api/users/${row.id}/grants` }),
        request<RoleOption[]>({ path: '/api/roles' }),
        request<{ code: string; name: string; categoryName: string }[]>({ path: '/api/permissions' })
      ])
      const startOfToday = new Date(new Date().toDateString())
      const grantRoles: GrantRole[] = (roles || []).map((r) => {
        const existing = (grants.roles || []).find((g) => g.code === r.code)
        const expiresAt = toDateInput(existing && existing.expiresAt)
        return {
          ...r,
          checked: Boolean(existing),
          expiresAt,
          expired: Boolean(existing && existing.expiresAt && new Date(existing.expiresAt) < startOfToday)
        }
      })
      const permGroups: PermGroup[] = groupCatalog(catalog || []).map((g) => ({
        name: g.name,
        items: g.items.map((p) => ({
          code: p.code,
          label: `${p.name}（${p.code}）`,
          checked: (grants.permissions || []).some((x) => x.permission === p.code)
        }))
      }))
      this.setData({ grantRoles, permGroups, grantGroups: (grants.groups || []) })
    } catch (err: any) {
      this.setData({ grantError: (err && err.message) || '加载失败' })
    } finally {
      this.setData({ grantLoading: false })
    }
  },

  closeGrants() {
    if (this.data.grantSaving) return
    this.setData({ grantOpen: false })
  },

  onGrantRoleTap(e: WechatMiniprogram.TouchEvent) {
    const idx = Number(e.currentTarget.dataset.idx)
    const checked = !this.data.grantRoles[idx].checked
    this.setData({ [`grantRoles[${idx}].checked`]: checked })
  },

  onGrantRoleExpiryChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.currentTarget.dataset.idx)
    this.setData({ [`grantRoles[${idx}].expiresAt`]: String(e.detail.value) })
  },

  onGrantPermTap(e: WechatMiniprogram.TouchEvent) {
    const gi = Number(e.currentTarget.dataset.gi)
    const pi = Number(e.currentTarget.dataset.pi)
    const checked = !this.data.permGroups[gi].items[pi].checked
    this.setData({ [`permGroups[${gi}].items[${pi}].checked`]: checked })
  },

  async submitGrants() {
    this.setData({ grantSaving: true, grantError: '' })
    try {
      const roles = this.data.grantRoles
        .filter((r) => r.checked)
        .map((r) => ({ roleCode: r.code, expiresAt: r.expiresAt ? localMidnightUTC(r.expiresAt) : null }))
      const permissions: { permission: string; expiresAt: null }[] = []
      for (const g of this.data.permGroups) {
        for (const p of g.items) {
          if (p.checked) permissions.push({ permission: p.code, expiresAt: null })
        }
      }
      await request({
        path: `/api/users/${(this.data.grantTarget as any).id}/roles`,
        method: 'PUT',
        data: { roles }
      })
      await request({
        path: `/api/users/${(this.data.grantTarget as any).id}/permissions`,
        method: 'PUT',
        data: { permissions }
      })
      this.setData({ grantOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ grantError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ grantSaving: false })
    }
  },

  // ---- 删除 ----

  onTapDelete(e: WechatMiniprogram.TouchEvent) {
    const { id, name, username } = e.currentTarget.dataset
    wx.showModal({
      title: '删除用户',
      content: `确定要删除用户「${name}」（@${username}）吗？此操作不可撤销。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(Number(id))
      }
    })
  },

  async handleDelete(id: number) {
    this.setData({ actingId: id })
    try {
      await request({ path: `/api/users/${id}`, method: 'DELETE' })
      this._list.reload()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
