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
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'

const emptyForm = { code: '', name: '', description: '', permissions: {} }

export default function RolesPage() {
  const { t } = useTranslation('roles')
  const { token } = useAuth()
  const navigate = useNavigate()

  const headers = [
    { key: 'id', header: t('field.id') },
    { key: 'code', header: t('field.code') },
    { key: 'name', header: t('field.name') },
    { key: 'description', header: t('field.description') },
    { key: 'permissions', header: t('field.permissions') },
    { key: 'isSystem', header: t('field.isSystem') },
  ]

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
      setCreateError(t('validation.codeRequired'))
      return
    }
    if (!createForm.name.trim()) {
      setCreateError(t('validation.nameRequired'))
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
      setEditError(t('validation.nameRequired'))
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
        <Breadcrumb noTrailingSlash aria-label={t('aria.breadcrumb', { ns: 'common' })}>
          <BreadcrumbItem
            href="/"
            onClick={(e) => {
              e.preventDefault()
              navigate('/')
            }}
          >
            {t('breadcrumb.home')}
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>{t('breadcrumb.current')}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="roles-page__heading">{t('title')}</h1>
        <p className="roles-page__subtitle">{t('subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loadError && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={loadError}
            lowContrast
            hideCloseButton
            className="roles-page__notice"
          />
        )}

        <DataTable rows={displayRows} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: roles.length })}>
              <TableToolbar>
                <TableToolbarContent>
                  <Button renderIcon={Add} size="sm" onClick={openCreate}>
                    {t('addButton')}
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
                    <TableHeader>{t('field.actions')}</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>{t('empty.loading')}</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>{t('empty.none')}</TableCell>
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
                                    {cell.value.includes('*')
                                      ? t('permissions.all')
                                      : t('permissions.count', { count: cell.value.length })}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'isSystem') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={cell.value ? 'purple' : 'gray'} size="sm">
                                    {cell.value ? t('system.builtin') : t('system.custom')}
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
                                iconDescription={t('action.edit', { ns: 'common' })}
                                disabled={isSystem}
                                onClick={() => openEdit(role)}
                              />
                              <Button
                                kind="ghost"
                                size="sm"
                                hasIconOnly
                                renderIcon={TrashCan}
                                iconDescription={t('action.delete', { ns: 'common' })}
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
        modalHeading={t('modal.create')}
        primaryButtonText={t('modal.createSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
        size="lg"
      >
        <div className="roles-page__form">
          <TextInput
            id="create-role-code"
            labelText={t('form.code')}
            helperText={t('codeHelperCreate')}
            placeholder={t('placeholder.code')}
            value={createForm.code}
            onChange={(e) => setCreateForm({ ...createForm, code: e.target.value })}
          />
          <TextInput
            id="create-role-name"
            labelText={t('form.name')}
            placeholder={t('placeholder.name')}
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextArea
            id="create-role-description"
            labelText={t('form.description')}
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
                    labelText={t('permOption', { name: perm.name, code: perm.code })}
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
              title={t('error.create')}
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
        modalHeading={t('modal.edit', { name: editTarget?.name ?? '' })}
        primaryButtonText={t('modal.editSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
        size="lg"
      >
        <div className="roles-page__form">
          <TextInput
            id="edit-role-code"
            labelText={t('form.code')}
            value={editForm.code}
            readOnly
            helperText={t('codeHelperEdit')}
          />
          <TextInput
            id="edit-role-name"
            labelText={t('form.name')}
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextArea
            id="edit-role-description"
            labelText={t('form.description')}
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
                    labelText={t('permOption', { name: perm.name, code: perm.code })}
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
              title={t('error.save')}
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
        modalHeading={t('modal.delete')}
        primaryButtonText={t('modal.deleteSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="roles-page__confirm-text">
          {t('deleteConfirm', { name: deleteTarget?.name })}
        </p>
        {deleteError && (
          <InlineNotification
            kind="error"
            title={t('error.delete')}
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
