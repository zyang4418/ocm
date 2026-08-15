import { useEffect, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  DataTable,
  Grid,
  InlineNotification,
  Modal,
  MultiSelect,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  TextArea,
  TextInput,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'

const headers = [
  { key: 'id', header: 'ID' },
  { key: 'name', header: '组名' },
  { key: 'description', header: '描述' },
  { key: 'memberCount', header: '成员数' },
  { key: 'createdAt', header: '创建时间' },
]

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

const emptyForm = { name: '', description: '', members: [], roles: [] }

export default function GroupsPage() {
  const { token } = useAuth()
  const navigate = useNavigate()

  const [groups, setGroups] = useState([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState(emptyForm)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)

  const [editTarget, setEditTarget] = useState(null)
  const [editForm, setEditForm] = useState(emptyForm)
  const [editError, setEditError] = useState('')
  const [editing, setEditing] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState(false)

  // Picker sources: all users (up to 500) and all roles.
  const [userOptions, setUserOptions] = useState([])
  const [roleOptions, setRoleOptions] = useState([])

  const load = async () => {
    try {
      setLoading(true)
      setLoadError('')
      const groupsData = await apiFetch('/api/groups', { token })
      setGroups(groupsData)
    } catch (err) {
      setLoadError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token])

  const loadPickerSources = async () => {
    const [usersPage, roles] = await Promise.all([
      apiFetch('/api/users?page_size=500', { token }),
      apiFetch('/api/roles', { token }),
    ])
    setUserOptions(
      usersPage.items.map((u) => ({
        id: String(u.id),
        text: `${u.displayName}（@${u.username}）`,
      })),
    )
    setRoleOptions(
      roles.map((r) => ({
        id: String(r.id),
        text: `${r.name}（${r.code}）`,
      })),
    )
  }

  const openCreate = async () => {
    setCreateForm(emptyForm)
    setCreateError('')
    setCreateOpen(true)
    try {
      await loadPickerSources()
    } catch {
      // Picker failure is non-fatal: name/description still work.
    }
  }

  const handleCreate = async () => {
    if (!createForm.name.trim()) {
      setCreateError('组名为必填项')
      return
    }
    try {
      setCreating(true)
      setCreateError('')
      await apiFetch('/api/groups', {
        method: 'POST',
        token,
        body: {
          name: createForm.name.trim(),
          description: createForm.description.trim(),
          members: createForm.members.map(Number),
          roles: createForm.roles.map(Number),
        },
      })
      setCreateOpen(false)
      load()
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = async (group) => {
    setEditTarget(group)
    setEditError('')
    try {
      const [detail] = await Promise.all([
        apiFetch(`/api/groups/${group.id}`, { token }),
        loadPickerSources(),
      ])
      setEditForm({
        name: detail.name,
        description: detail.description,
        members: detail.members.map((m) => String(m.id)),
        roles: detail.roles.map((r) => String(r.id)),
      })
    } catch (err) {
      setEditError(err.message)
    }
  }

  const handleEdit = async () => {
    if (!editForm.name.trim()) {
      setEditError('组名为必填项')
      return
    }
    try {
      setEditing(true)
      setEditError('')
      await apiFetch(`/api/groups/${editTarget.id}`, {
        method: 'PUT',
        token,
        body: {
          name: editForm.name.trim(),
          description: editForm.description.trim(),
          members: editForm.members.map(Number),
          roles: editForm.roles.map(Number),
        },
      })
      setEditTarget(null)
      load()
    } catch (err) {
      setEditError(err.message)
    } finally {
      setEditing(false)
    }
  }

  const handleDelete = async () => {
    try {
      setDeleting(true)
      setDeleteError('')
      await apiFetch(`/api/groups/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      load()
    } catch (err) {
      setDeleteError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <Grid fullWidth className="groups-page">
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
          <BreadcrumbItem isCurrentPage>用户组管理</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="groups-page__heading">用户组管理</h1>
        <p className="groups-page__subtitle">
          将用户组织成组并整体授予角色，成员权限 = 个人授权 ∪ 所在组授权。
        </p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loadError && (
          <InlineNotification
            kind="error"
            title="加载失败"
            subtitle={loadError}
            lowContrast
            hideCloseButton
            className="groups-page__notice"
          />
        )}

        <DataTable rows={groups} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps }) => (
            <TableContainer title="用户组列表" description={`共 ${groups.length} 个用户组`}>
              <TableToolbar>
                <TableToolbarContent>
                  <Button renderIcon={Add} size="sm" onClick={openCreate}>
                    新建用户组
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
                      <TableCell colSpan={headers.length + 1}>暂无用户组</TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const group = groups.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value}</TableCell>
                          })}
                          <TableCell>
                            <div className="groups-page__actions">
                              <Button
                                kind="ghost"
                                size="sm"
                                hasIconOnly
                                renderIcon={Edit}
                                iconDescription="编辑"
                                onClick={() => openEdit(group)}
                              />
                              <Button
                                kind="ghost"
                                size="sm"
                                hasIconOnly
                                renderIcon={TrashCan}
                                iconDescription="删除"
                                onClick={() => {
                                  setDeleteTarget(group)
                                  setDeleteError('')
                                }}
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
      </Column>

      {/* Create */}
      <Modal
        open={createOpen}
        modalHeading="新建用户组"
        primaryButtonText="创建"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="groups-page__form">
          <TextInput
            id="create-group-name"
            labelText="组名"
            placeholder="如 办公室助理组"
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextArea
            id="create-group-description"
            labelText="描述"
            value={createForm.description}
            onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
          />
          <MultiSelect
            id="create-group-members"
            titleText="成员"
            label="选择成员…"
            items={userOptions}
            selectedItems={userOptions.filter((o) => createForm.members.includes(o.id))}
            onChange={({ selectedItems }) =>
              setCreateForm({ ...createForm, members: selectedItems.map((o) => o.id) })
            }
          />
          <MultiSelect
            id="create-group-roles"
            titleText="角色"
            label="选择角色…"
            items={roleOptions}
            selectedItems={roleOptions.filter((o) => createForm.roles.includes(o.id))}
            onChange={({ selectedItems }) =>
              setCreateForm({ ...createForm, roles: selectedItems.map((o) => o.id) })
            }
          />
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
        modalHeading={`编辑用户组：${editTarget?.name ?? ''}`}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
      >
        <div className="groups-page__form">
          <TextInput
            id="edit-group-name"
            labelText="组名"
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextArea
            id="edit-group-description"
            labelText="描述"
            value={editForm.description}
            onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
          />
          <MultiSelect
            id="edit-group-members"
            titleText="成员"
            label="选择成员…"
            items={userOptions}
            selectedItems={userOptions.filter((o) => editForm.members.includes(o.id))}
            onChange={({ selectedItems }) =>
              setEditForm({ ...editForm, members: selectedItems.map((o) => o.id) })
            }
          />
          <MultiSelect
            id="edit-group-roles"
            titleText="角色"
            label="选择角色…"
            items={roleOptions}
            selectedItems={roleOptions.filter((o) => editForm.roles.includes(o.id))}
            onChange={({ selectedItems }) =>
              setEditForm({ ...editForm, roles: selectedItems.map((o) => o.id) })
            }
          />
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

      {/* Delete */}
      <Modal
        danger
        open={Boolean(deleteTarget)}
        modalHeading="删除用户组"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="groups-page__confirm-text">
          确定要删除用户组「{deleteTarget?.name}」吗？组内成员的组级授权将一并撤销。
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
