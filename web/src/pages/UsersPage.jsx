import { useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  DataTable,
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
import { Add, Edit, Password as PasswordIcon, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import usePagedList from '../hooks/usePagedList.js'
import ListPagination from '../components/ListPagination.jsx'

const headers = [
  { key: 'id', header: 'ID' },
  { key: 'username', header: '用户名' },
  { key: 'displayName', header: '显示名称' },
  { key: 'role', header: '角色' },
  { key: 'createdAt', header: '创建时间' },
]

const roleLabel = (role) => (role === 'admin' ? '管理员' : '普通用户')
const roleKind = (role) => (role === 'admin' ? 'purple' : 'gray')

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

const emptyCreate = { username: '', password: '', displayName: '', role: 'user' }

export default function UsersPage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const list = usePagedList({ path: '/api/users', token })
  const { loading } = list

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState(emptyCreate)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)

  const [editTarget, setEditTarget] = useState(null)
  const [editForm, setEditForm] = useState({ displayName: '', role: 'user' })
  const [editError, setEditError] = useState('')
  const [editing, setEditing] = useState(false)

  const [pwdTarget, setPwdTarget] = useState(null)
  const [pwdForm, setPwdForm] = useState({ password: '', confirm: '' })
  const [pwdError, setPwdError] = useState('')
  const [pwdSaving, setPwdSaving] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const handleCreate = async () => {
    const { username, password, displayName, role } = createForm
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
          role,
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
    setEditForm({ displayName: u.displayName, role: u.role })
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
          role: editForm.role,
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

  const isSelf = (u) => Boolean(u) && String(u.id) === String(currentUser?.id)

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
        <p className="users-page__subtitle">维护系统用户账号、角色与密码。</p>
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
                  <Button renderIcon={Add} size="sm" onClick={() => setCreateOpen(true)}>
                    添加用户
                  </Button>
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
                            if (cell.info.header === 'role') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={roleKind(cell.value)} size="sm">
                                    {roleLabel(cell.value)}
                                  </Tag>
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
                                renderIcon={TrashCan}
                                iconDescription="删除"
                                disabled={isSelf(u)}
                                onClick={() => openDelete(u)}
                              />
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
            id="create-role"
            labelText="角色"
            value={createForm.role}
            onChange={(e) => setCreateForm({ ...createForm, role: e.target.value })}
          >
            <SelectItem value="user" text="普通用户" />
            <SelectItem value="admin" text="管理员" />
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
            id="edit-role"
            labelText="角色"
            value={editForm.role}
            onChange={(e) => setEditForm({ ...editForm, role: e.target.value })}
          >
            <SelectItem value="user" text="普通用户" />
            <SelectItem value="admin" text="管理员" />
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
