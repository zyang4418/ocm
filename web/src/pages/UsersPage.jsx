import { useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Checkbox,
  CheckboxGroup,
  Column,
  DataTable,
  DatePicker,
  DatePickerInput,
  Grid,
  InlineNotification,
  Modal,
  PasswordInput,
  Select,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Tag,
  TextInput,
} from '@carbon/react'
import { Add, Edit, Password as PasswordIcon, TrashCan, UserSettings } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import usePagedList from '../hooks/usePagedList.js'
import ListPagination from '../components/ListPagination.jsx'

const headers = [
  { key: 'id', header: 'ID' },
  { key: 'username', header: '用户名' },
  { key: 'displayName', header: '显示名称' },
  { key: 'type', header: '类型' },
  { key: 'roles', header: '角色' },
  { key: 'groups', header: '用户组' },
  { key: 'createdAt', header: '创建时间' },
]

const typeLabel = (type) => ({ student: '学生', teacher: '教师', staff: '职员' }[type] ?? type)
const typeKind = (type) => ({ student: 'teal', teacher: 'blue', staff: 'gray' }[type] ?? 'gray')

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const emptyCreate = { username: '', password: '', displayName: '', type: 'staff' }

// Grants modal state: one entry per role (with optional expiry) and one per
// catalog permission. Groups are display-only.
const emptyGrantForm = { roles: {}, permissions: {}, groups: [] }

export default function UsersPage() {
  const { token, user: currentUser, can } = useAuth()
  const canManage = can('user:manage')
  const navigate = useNavigate()
  const list = usePagedList({ path: '/api/users', token })
  const { loading } = list

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState(emptyCreate)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)

  const [editTarget, setEditTarget] = useState(null)
  const [editForm, setEditForm] = useState({ displayName: '', type: 'staff' })
  const [editError, setEditError] = useState('')
  const [editing, setEditing] = useState(false)

  const [pwdTarget, setPwdTarget] = useState(null)
  const [pwdForm, setPwdForm] = useState({ password: '', confirm: '' })
  const [pwdError, setPwdError] = useState('')
  const [pwdSaving, setPwdSaving] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const [grantTarget, setGrantTarget] = useState(null)
  const [grantRoles, setGrantRoles] = useState([])
  const [grantCatalog, setGrantCatalog] = useState([])
  const [grantForm, setGrantForm] = useState(emptyGrantForm)
  const [grantError, setGrantError] = useState('')
  const [grantLoading, setGrantLoading] = useState(false)
  const [grantSaving, setGrantSaving] = useState(false)

  const handleCreate = async () => {
    const { username, password, displayName, type } = createForm
    if (!username.trim() || !password || !displayName.trim()) {
      setCreateError('用户名、密码和显示名称均为必填项')
      return
    }
    try {
      setCreating(true)
      setCreateError('')
      await apiFetch('/api/users', {
        method: 'POST',
        token,
        body: {
          username: username.trim(),
          password,
          displayName: displayName.trim(),
          type,
        },
      })
      setCreateOpen(false)
      list.reload()
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (u) => {
    setEditTarget(u)
    setEditForm({ displayName: u.displayName, type: u.type })
    setEditError('')
  }

  const handleEdit = async () => {
    if (!editForm.displayName.trim()) {
      setEditError('显示名称为必填项')
      return
    }
    try {
      setEditing(true)
      setEditError('')
      await apiFetch(`/api/users/${editTarget.id}`, {
        method: 'PUT',
        token,
        body: {
          displayName: editForm.displayName.trim(),
          type: editForm.type,
        },
      })
      setEditTarget(null)
      list.reload()
    } catch (err) {
      setEditError(err.message)
    } finally {
      setEditing(false)
    }
  }

  const openPassword = (u) => {
    setPwdTarget(u)
    setPwdForm({ password: '', confirm: '' })
    setPwdError('')
  }

  const handlePassword = async () => {
    if (!pwdForm.password) {
      setPwdError('请输入新密码')
      return
    }
    if (pwdForm.password !== pwdForm.confirm) {
      setPwdError('两次输入的密码不一致')
      return
    }
    try {
      setPwdSaving(true)
      setPwdError('')
      await apiFetch(`/api/users/${pwdTarget.id}/password`, {
        method: 'PATCH',
        token,
        body: { password: pwdForm.password },
      })
      setPwdTarget(null)
    } catch (err) {
      setPwdError(err.message)
    } finally {
      setPwdSaving(false)
    }
  }

  const openDelete = (u) => {
    setDeleteTarget(u)
    setDeleteError('')
  }

  const handleDelete = async () => {
    try {
      setDeleting(true)
      setDeleteError('')
      await apiFetch(`/api/users/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      list.reload()
    } catch (err) {
      setDeleteError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  // ---- Grants modal ----

  const openGrants = async (u) => {
    setGrantTarget(u)
    setGrantError('')
    setGrantLoading(true)
    try {
      const [grants, roles, catalog] = await Promise.all([
        apiFetch(`/api/users/${u.id}/grants`, { token }),
        apiFetch('/api/roles', { token }),
        apiFetch('/api/permissions', { token }),
      ])
      setGrantRoles(roles)
      setGrantCatalog(catalog)
      const form = { roles: {}, permissions: {}, groups: grants.groups }
      for (const role of roles) {
        const existing = grants.roles.find((g) => g.code === role.code)
        form.roles[role.code] = existing ? { checked: true, expiresAt: toDateInput(existing.expiresAt) } : { checked: false, expiresAt: '' }
      }
      for (const perm of catalog) {
        form.permissions[perm.code] = grants.permissions.some((g) => g.permission === perm.code)
      }
      setGrantForm(form)
    } catch (err) {
      setGrantError(err.message)
    } finally {
      setGrantLoading(false)
    }
  }

  const handleGrantSave = async () => {
    try {
      setGrantSaving(true)
      setGrantError('')
      const roles = Object.entries(grantForm.roles)
        .filter(([, v]) => v.checked)
        .map(([code, v]) => ({
          roleCode: code,
          expiresAt: v.expiresAt ? localMidnightUTC(v.expiresAt) : null,
        }))
      const permissions = Object.entries(grantForm.permissions)
        .filter(([, checked]) => checked)
        .map(([permission]) => ({ permission, expiresAt: null }))
      await apiFetch(`/api/users/${grantTarget.id}/roles`, {
        method: 'PUT',
        token,
        body: { roles },
      })
      await apiFetch(`/api/users/${grantTarget.id}/permissions`, {
        method: 'PUT',
        token,
        body: { permissions },
      })
      setGrantTarget(null)
      list.reload()
    } catch (err) {
      setGrantError(err.message)
    } finally {
      setGrantSaving(false)
    }
  }

  const isSelf = (u) => Boolean(u) && String(u.id) === String(currentUser?.id)

  const catalogGroups = groupCatalog(grantCatalog)

  return (
    <Grid fullWidth className="users-page">
      <Column sm={4} md={8} lg={16}>
        <Breadcrumb noTrailingSlash aria-label="面包屑导航">
          <BreadcrumbItem
            href="/"
            onClick={(e) => {
              e.preventDefault()
              navigate('/')
            }}
          >
            首页
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>用户管理</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="users-page__heading">用户管理</h1>
        <p className="users-page__subtitle">维护系统用户账号、类型、角色授权与密码。</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {list.error && (
          <InlineNotification
            kind="error"
            title="加载失败"
            subtitle={list.error}
            lowContrast
            hideCloseButton
            className="users-page__notice"
          />
        )}

        <DataTable rows={list.items} headers={headers}>
          {({
            rows,
            headers: tableHeaders,
            getTableProps,
            getHeaderProps,
            getRowProps,
            getToolbarProps,
          }) => (
            <TableContainer title="用户列表" description={`共 ${list.total} 个账号`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder="搜索用户" />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={() => setCreateOpen(true)}>
                      添加用户
                    </Button>
                  )}
                </TableToolbarContent>
              </TableToolbar>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {tableHeaders.map((header) => (
                      <TableHeader key={header.key} {...getHeaderProps({ header })}>
                        {header.header}
                      </TableHeader>
                    ))}
                    <TableHeader>操作</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>加载中…</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>
                        {list.q ? '未找到匹配的用户' : '暂无用户'}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const u = list.items.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'type') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={typeKind(cell.value)} size="sm">
                                    {typeLabel(cell.value)}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'roles' || cell.info.header === 'groups') {
                              const items = cell.value ?? []
                              return (
                                <TableCell key={cell.id}>
                                  {items.length === 0 ? (
                                    '-'
                                  ) : (
                                    <div className="users-page__tags">
                                      {items.map((item) => (
                                        <Tag key={`${cell.info.header}-${item.id}`} type="cool-gray" size="sm">
                                          {item.name}
                                        </Tag>
                                      ))}
                                    </div>
                                  )}
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value}</TableCell>
                          })}
                          <TableCell>
                            <div className="users-page__actions">
                              {canManage && (
                                <>
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={Edit}
                                    iconDescription="编辑"
                                    onClick={() => openEdit(u)}
                                  />
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={PasswordIcon}
                                    iconDescription="重置密码"
                                    onClick={() => openPassword(u)}
                                  />
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={UserSettings}
                                    iconDescription="角色与权限"
                                    onClick={() => openGrants(u)}
                                  />
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={TrashCan}
                                    iconDescription="删除"
                                    disabled={isSelf(u)}
                                    onClick={() => openDelete(u)}
                                  />
                                </>
                              )}
                              {!canManage && <span className="users-page__readonly">只读</span>}
                            </div>
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </DataTable>
        <ListPagination
          page={list.page}
          pageSize={list.pageSize}
          totalItems={list.total}
          onPageChange={list.setPage}
          onPageSizeChange={list.setPageSize}
        />
      </Column>

      {/* Create */}
      <Modal
        open={createOpen}
        modalHeading="添加用户"
        primaryButtonText="创建"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="users-page__form">
          <TextInput
            id="create-username"
            labelText="用户名"
            value={createForm.username}
            onChange={(e) => setCreateForm({ ...createForm, username: e.target.value })}
          />
          <PasswordInput
            id="create-password"
            labelText="密码"
            placeholder="设置初始密码"
            value={createForm.password}
            onChange={(e) => setCreateForm({ ...createForm, password: e.target.value })}
            showPasswordLabel="显示密码"
            hidePasswordLabel="隐藏密码"
          />
          <TextInput
            id="create-displayName"
            labelText="显示名称"
            value={createForm.displayName}
            onChange={(e) => setCreateForm({ ...createForm, displayName: e.target.value })}
          />
          <Select
            id="create-type"
            labelText="类型"
            value={createForm.type}
            onChange={(e) => setCreateForm({ ...createForm, type: e.target.value })}
          >
            <SelectItem value="student" text="学生" />
            <SelectItem value="teacher" text="教师" />
            <SelectItem value="staff" text="职员" />
          </Select>
          {createError && (
            <InlineNotification
              kind="error"
              title="创建失败"
              subtitle={createError}
              lowContrast
              hideCloseButton
            />
          )}
        </div>
      </Modal>

      {/* Edit */}
      <Modal
        open={Boolean(editTarget)}
        modalHeading={`编辑用户：${editTarget?.username ?? ''}`}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
      >
        <div className="users-page__form">
          <TextInput
            id="edit-displayName"
            labelText="显示名称"
            value={editForm.displayName}
            onChange={(e) => setEditForm({ ...editForm, displayName: e.target.value })}
          />
          <Select
            id="edit-type"
            labelText="类型"
            value={editForm.type}
            onChange={(e) => setEditForm({ ...editForm, type: e.target.value })}
          >
            <SelectItem value="student" text="学生" />
            <SelectItem value="teacher" text="教师" />
            <SelectItem value="staff" text="职员" />
          </Select>
          {editError && (
            <InlineNotification
              kind="error"
              title="保存失败"
              subtitle={editError}
              lowContrast
              hideCloseButton
            />
          )}
        </div>
      </Modal>

      {/* Password */}
      <Modal
        open={Boolean(pwdTarget)}
        modalHeading={`重置密码：${pwdTarget?.username ?? ''}`}
        primaryButtonText="重置密码"
        secondaryButtonText="取消"
        onRequestClose={() => setPwdTarget(null)}
        onRequestSubmit={handlePassword}
        primaryButtonDisabled={pwdSaving}
      >
        <div className="users-page__form">
          <PasswordInput
            id="pwd-password"
            labelText="新密码"
            value={pwdForm.password}
            onChange={(e) => setPwdForm({ ...pwdForm, password: e.target.value })}
            showPasswordLabel="显示密码"
            hidePasswordLabel="隐藏密码"
          />
          <PasswordInput
            id="pwd-confirm"
            labelText="确认新密码"
            value={pwdForm.confirm}
            onChange={(e) => setPwdForm({ ...pwdForm, confirm: e.target.value })}
            showPasswordLabel="显示密码"
            hidePasswordLabel="隐藏密码"
          />
          {pwdError && (
            <InlineNotification
              kind="error"
              title="重置失败"
              subtitle={pwdError}
              lowContrast
              hideCloseButton
            />
          )}
        </div>
      </Modal>

      {/* Grants */}
      <Modal
        open={Boolean(grantTarget)}
        modalHeading={`角色与权限：${grantTarget?.displayName ?? ''}`}
        primaryButtonText="保存授权"
        secondaryButtonText="取消"
        onRequestClose={() => setGrantTarget(null)}
        onRequestSubmit={handleGrantSave}
        primaryButtonDisabled={grantSaving}
        size="lg"
      >
        <div className="users-page__grants">
          {grantLoading && <p>加载中…</p>}
          {grantError && (
            <InlineNotification
              kind="error"
              title="授权失败"
              subtitle={grantError}
              lowContrast
              hideCloseButton
            />
          )}
          {!grantLoading && (
            <>
              <section className="users-page__grants-section">
                <h3>角色</h3>
                <p className="users-page__grants-hint">勾选后可为该角色设置有效期，留空表示长期有效。</p>
                {grantRoles.map((role) => {
                  const entry = grantForm.roles[role.code] ?? { checked: false, expiresAt: '' }
                  const expired = entry.checked && entry.expiresAt && new Date(entry.expiresAt) < startOfToday()
                  return (
                    <div key={role.id} className="users-page__grant-row">
                      <Checkbox
                        id={`grant-role-${role.id}`}
                        labelText={role.name}
                        checked={entry.checked}
                        onChange={(_, { checked }) =>
                          setGrantForm({
                            ...grantForm,
                            roles: { ...grantForm.roles, [role.code]: { ...entry, checked } },
                          })
                        }
                      />
                      {role.isSystem && <Tag type="purple" size="sm">内置</Tag>}
                      {entry.checked && (
                        <DatePicker
                          datePickerType="single"
                          dateFormat="Y-m-d"
                          onChange={(dates) =>
                            setGrantForm({
                              ...grantForm,
                              roles: {
                                ...grantForm.roles,
                                [role.code]: { ...entry, expiresAt: dates[0] ?? '' },
                              },
                            })
                          }
                        >
                          <DatePickerInput
                            id={`grant-role-${role.id}-expiry`}
                            placeholder="长期有效"
                            labelText="有效期至"
                            value={entry.expiresAt}
                            size="sm"
                          />
                        </DatePicker>
                      )}
                      {expired && <Tag type="red" size="sm">已过期</Tag>}
                    </div>
                  )
                })}
              </section>
              <section className="users-page__grants-section">
                <h3>直接授权权限</h3>
                <p className="users-page__grants-hint">在角色之外额外授予的单项权限，适合临时授权场景。</p>
                {catalogGroups.map((group) => (
                  <CheckboxGroup
                    key={group.name}
                    legendText={group.name}
                    className="users-page__perm-group"
                  >
                    {group.items.map((perm) => (
                      <Checkbox
                        key={perm.code}
                        id={`grant-perm-${perm.code}`}
                        labelText={`${perm.name}（${perm.code}）`}
                        checked={Boolean(grantForm.permissions[perm.code])}
                        onChange={(_, { checked }) =>
                          setGrantForm({
                            ...grantForm,
                            permissions: { ...grantForm.permissions, [perm.code]: checked },
                          })
                        }
                      />
                    ))}
                  </CheckboxGroup>
                ))}
              </section>
              <section className="users-page__grants-section">
                <h3>所属用户组</h3>
                <p className="users-page__grants-hint">组成员身份在「用户组管理」中维护，此处只读。</p>
                {grantForm.groups.length === 0 ? (
                  <p className="users-page__grants-empty">未加入任何用户组</p>
                ) : (
                  <div className="users-page__tags">
                    {grantForm.groups.map((g) => (
                      <Tag key={g.id} type="cool-gray" size="sm">
                        {g.name}
                      </Tag>
                    ))}
                  </div>
                )}
              </section>
            </>
          )}
        </div>
      </Modal>

      {/* Delete */}
      <Modal
        danger
        open={Boolean(deleteTarget)}
        modalHeading="删除用户"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="users-page__confirm-text">
          确定要删除用户「{deleteTarget?.displayName}」（@{deleteTarget?.username}）吗？此操作不可撤销。
        </p>
        {deleteError && (
          <InlineNotification
            kind="error"
            title="删除失败"
            subtitle={deleteError}
            lowContrast
            hideCloseButton
          />
        )}
      </Modal>
    </Grid>
  )
}

// toDateInput renders a nullable expiry as a Y-M-D string for the picker.
function toDateInput(value) {
  if (!value) return ''
  const d = new Date(value)
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// localMidnightUTC converts a Y-M-D picker value to a UTC RFC3339 instant of
// local midnight, matching the backend's server-side comparison.
function localMidnightUTC(value) {
  return new Date(`${value}T00:00:00`).toISOString()
}

function startOfToday() {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth(), now.getDate())
}

// groupCatalog buckets the permission catalog by categoryName, preserving
// the catalog's sorted order within each bucket.
function groupCatalog(catalog) {
  const groups = new Map()
  for (const perm of catalog) {
    if (!groups.has(perm.categoryName)) groups.set(perm.categoryName, [])
    groups.get(perm.categoryName).push(perm)
  }
  return Array.from(groups, ([name, items]) => ({ name, items }))
}
