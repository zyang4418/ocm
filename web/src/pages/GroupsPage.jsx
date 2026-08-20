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
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import { formatDate } from '../i18n/formatters'

const emptyForm = { name: '', description: '', members: [], roles: [] }

export default function GroupsPage() {
  const { t } = useTranslation('groups')
  const { token } = useAuth()
  const navigate = useNavigate()

  const headers = [
    { key: 'id', header: t('field.id') },
    { key: 'name', header: t('field.name') },
    { key: 'description', header: t('field.description') },
    { key: 'memberCount', header: t('field.memberCount') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

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
        text: t('picker.userOption', { name: u.displayName, username: u.username }),
      })),
    )
    setRoleOptions(
      roles.map((r) => ({
        id: String(r.id),
        text: t('picker.roleOption', { name: r.name, code: r.code }),
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
      setCreateError(t('validation.nameRequired'))
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
      setEditError(t('validation.nameRequired'))
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
        <h1 className="groups-page__heading">{t('title')}</h1>
        <p className="groups-page__subtitle">{t('subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loadError && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={loadError}
            lowContrast
            hideCloseButton
            className="groups-page__notice"
          />
        )}

        <DataTable rows={groups} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: groups.length })}>
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
                                iconDescription={t('action.edit', { ns: 'common' })}
                                onClick={() => openEdit(group)}
                              />
                              <Button
                                kind="ghost"
                                size="sm"
                                hasIconOnly
                                renderIcon={TrashCan}
                                iconDescription={t('action.delete', { ns: 'common' })}
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
        modalHeading={t('modal.create')}
        primaryButtonText={t('modal.createSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="groups-page__form">
          <TextInput
            id="create-group-name"
            labelText={t('form.name')}
            placeholder={t('placeholder.name')}
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextArea
            id="create-group-description"
            labelText={t('form.description')}
            value={createForm.description}
            onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
          />
          <MultiSelect
            id="create-group-members"
            titleText={t('form.members')}
            label={t('picker.members')}
            items={userOptions}
            selectedItems={userOptions.filter((o) => createForm.members.includes(o.id))}
            onChange={({ selectedItems }) =>
              setCreateForm({ ...createForm, members: selectedItems.map((o) => o.id) })
            }
          />
          <MultiSelect
            id="create-group-roles"
            titleText={t('form.roles')}
            label={t('picker.roles')}
            items={roleOptions}
            selectedItems={roleOptions.filter((o) => createForm.roles.includes(o.id))}
            onChange={({ selectedItems }) =>
              setCreateForm({ ...createForm, roles: selectedItems.map((o) => o.id) })
            }
          />
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
      >
        <div className="groups-page__form">
          <TextInput
            id="edit-group-name"
            labelText={t('form.name')}
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextArea
            id="edit-group-description"
            labelText={t('form.description')}
            value={editForm.description}
            onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
          />
          <MultiSelect
            id="edit-group-members"
            titleText={t('form.members')}
            label={t('picker.members')}
            items={userOptions}
            selectedItems={userOptions.filter((o) => editForm.members.includes(o.id))}
            onChange={({ selectedItems }) =>
              setEditForm({ ...editForm, members: selectedItems.map((o) => o.id) })
            }
          />
          <MultiSelect
            id="edit-group-roles"
            titleText={t('form.roles')}
            label={t('picker.roles')}
            items={roleOptions}
            selectedItems={roleOptions.filter((o) => editForm.roles.includes(o.id))}
            onChange={({ selectedItems }) =>
              setEditForm({ ...editForm, roles: selectedItems.map((o) => o.id) })
            }
          />
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
        <p className="groups-page__confirm-text">
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
