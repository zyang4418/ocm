import { useEffect, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Checkbox,
  CheckboxGroup,
  Column,
  DataTable,
  Grid,
  InlineNotification,
  Modal,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  Tag,
  TextArea,
  TextInput,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'

const headers = [
  { key: 'id', header: 'ID' },
  { key: 'code', header: '角色代码' },
  { key: 'name', header: '角色名称' },
  { key: 'description', header: '描述' },
  { key: 'permissions', header: '权限数' },
  { key: 'isSystem', header: '类型' },
]

const emptyForm = { code: '', name: '', description: '', permissions: {} }

export default function RolesPage() {
  const { token } = useAuth()
  const navigate = useNavigate()

  const [roles, setRoles] = useState([])
  const [catalog, setCatalog] = useState([])
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

  const load = async () => {
    try {
      setLoading(true)
      setLoadError('')
      const [rolesData, catalogData] = await Promise.all([
        apiFetch('/api/roles', { token }),
        apiFetch('/api/permissions', { token }),
      ])
      setRoles(rolesData)
      setCatalog(catalogData)
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

  // permissionChecked reads a permission key from a form's permission map.
  const permissionChecked = (form, code) => Boolean(form.permissions[code])

  const togglePermission = (form, setForm, code, checked) =>
    setForm({ ...form, permissions: { ...form.permissions, [code]: checked } })

  const buildPermissions = (form) =>
    Object.entries(form.permissions)
      .filter(([, checked]) => checked)
      .map(([code]) => code)

  const openCreate = () => {
    setCreateForm(emptyForm)
    setCreateError('')
    setCreateOpen(true)
  }

  const handleCreate = async () => {
    if (!createForm.code.trim()) {
      setCreateError('角色代码为必填项')
      return
    }
    if (!createForm.name.trim()) {
      setCreateError('角色名称为必填项')
      return
    }
    try {
      setCreating(true)
      setCreateError('')
      await apiFetch('/api/roles', {
        method: 'POST',
        token,
        body: {
          code: createForm.code.trim(),
          name: createForm.name.trim(),
          description: createForm.description.trim(),
          permissions: buildPermissions(createForm),
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

  const openEdit = (role) => {
    setEditTarget(role)
    const permissions = {}
    for (const perm of role.permissions) permissions[perm] = true
    setEditForm({ code: role.code, name: role.name, description: role.description, permissions })
    setEditError('')
  }

  const handleEdit = async () => {
    if (!editForm.name.trim()) {
      setEditError('角色名称为必填项')
      return
    }
    try {
      setEditing(true)
      setEditError('')
      await apiFetch(`/api/roles/${editTarget.id}`, {
        method: 'PUT',
        token,
        body: {
          name: editForm.name.trim(),
          description: editForm.description.trim(),
          permissions: buildPermissions(editForm),
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
      await apiFetch(`/api/roles/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      load()
    } catch (err) {
      setDeleteError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  const catalogGroups = groupCatalog(catalog)
  const displayRows = roles.map((r) => ({ ...r, isSystem: r.isSystem }))

  return (
    <Grid fullWidth className="roles-page">
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
          <BreadcrumbItem isCurrentPage>角色管理</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="roles-page__heading">角色管理</h1>
        <p className="roles-page__subtitle">定义角色及其权限集合，并将角色授予用户或用户组。</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loadError && (
          <InlineNotification
            kind="error"
            title="加载失败"
            subtitle={loadError}
            lowContrast
            hideCloseButton
            className="roles-page__notice"
          />
        )}

        <DataTable rows={displayRows} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps }) => (
            <TableContainer title="角色列表" description={`共 ${roles.length} 个角色`}>
              <TableToolbar>
                <TableToolbarContent>
                  <Button renderIcon={Add} size="sm" onClick={openCreate}>
                    新建角色
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
                      <TableCell colSpan={headers.length + 1}>暂无角色</TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const role = roles.find((x) => String(x.id) === String(row.id))
                      const isSystem = Boolean(role?.isSystem)
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'permissions') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={cell.value.includes('*') ? 'purple' : 'blue'} size="sm">
                                    {cell.value.includes('*') ? '全部' : `${cell.value.length} 项`}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'isSystem') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={cell.value ? 'purple' : 'gray'} size="sm">
                                    {cell.value ? '系统内置' : '自定义'}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            return <TableCell key={cell.id}>{cell.value}</TableCell>
                          })}
                          <TableCell>
                            <div className="roles-page__actions">
                              <Button
                                kind="ghost"
                                size="sm"
                                hasIconOnly
                                renderIcon={Edit}
                                iconDescription="编辑"
                                disabled={isSystem}
                                onClick={() => openEdit(role)}
                              />
                              <Button
                                kind="ghost"
                                size="sm"
                                hasIconOnly
                                renderIcon={TrashCan}
                                iconDescription="删除"
                                disabled={isSystem}
                                onClick={() => {
                                  setDeleteTarget(role)
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
        modalHeading="新建角色"
        primaryButtonText="创建"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
        size="lg"
      >
        <div className="roles-page__form">
          <TextInput
            id="create-role-code"
            labelText="角色代码"
            helperText="小写字母开头，仅含小写字母、数字与下划线；创建后不可修改"
            placeholder="如 office-assistant"
            value={createForm.code}
            onChange={(e) => setCreateForm({ ...createForm, code: e.target.value })}
          />
          <TextInput
            id="create-role-name"
            labelText="角色名称"
            placeholder="如 办公室助理"
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextArea
            id="create-role-description"
            labelText="描述"
            value={createForm.description}
            onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
          />
          <div className="roles-page__perm-list">
            {catalogGroups.map((group) => (
              <CheckboxGroup key={group.name} legendText={group.name}>
                {group.items.map((perm) => (
                  <Checkbox
                    key={perm.code}
                    id={`create-role-perm-${perm.code}`}
                    labelText={`${perm.name}（${perm.code}）`}
                    checked={permissionChecked(createForm, perm.code)}
                    onChange={(_, { checked }) =>
                      togglePermission(createForm, setCreateForm, perm.code, checked)
                    }
                  />
                ))}
              </CheckboxGroup>
            ))}
          </div>
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
        modalHeading={`编辑角色：${editTarget?.name ?? ''}`}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
        size="lg"
      >
        <div className="roles-page__form">
          <TextInput
            id="edit-role-code"
            labelText="角色代码"
            value={editForm.code}
            readOnly
            helperText="角色代码创建后不可修改"
          />
          <TextInput
            id="edit-role-name"
            labelText="角色名称"
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextArea
            id="edit-role-description"
            labelText="描述"
            value={editForm.description}
            onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
          />
          <div className="roles-page__perm-list">
            {catalogGroups.map((group) => (
              <CheckboxGroup key={group.name} legendText={group.name}>
                {group.items.map((perm) => (
                  <Checkbox
                    key={perm.code}
                    id={`edit-role-perm-${perm.code}`}
                    labelText={`${perm.name}（${perm.code}）`}
                    checked={permissionChecked(editForm, perm.code)}
                    onChange={(_, { checked }) =>
                      togglePermission(editForm, setEditForm, perm.code, checked)
                    }
                  />
                ))}
              </CheckboxGroup>
            ))}
          </div>
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
        modalHeading="删除角色"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="roles-page__confirm-text">
          确定要删除角色「{deleteTarget?.name}」吗？仍在使用的角色无法删除。
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
