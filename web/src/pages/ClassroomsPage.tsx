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
  type DataTableHeader,
  type TagProps,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ExportButton from '../components/ExportButton'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDate } from '../i18n/formatters'
import type { Classroom, ClassroomInput } from '../types/api'

// Enum value lists (not translatable text — the labels come from i18n).
const TYPE_KEYS = [
  'standard',
  'multimedia',
  'computer',
  'lab',
  'lecture_hall',
  'stadium',
  'drawing',
  'language',
  'studio',
  'special',
] as const
const STATUS_KEYS = ['available', 'maintenance', 'disabled'] as const

const statusKind: Record<string, TagProps<'div'>['type']> = {
  available: 'green',
  maintenance: 'blue',
  disabled: 'red',
}

// Form fields are all strings while editing; capacity converts on submit.
interface ClassroomForm {
  name: string
  building: string
  capacity: string
  type: string
  floor: string
  campus: string
  status: string
  description: string
}

const emptyForm: ClassroomForm = {
  name: '',
  building: '',
  capacity: '',
  type: 'standard',
  floor: '',
  campus: '',
  status: 'available',
  description: '',
}

export default function ClassroomsPage() {
  const { t } = useTranslation('classrooms')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('classroom:manage')

  const headers: DataTableHeader[] = [
    { key: 'id', header: t('field.id') },
    { key: 'name', header: t('field.name') },
    { key: 'building', header: t('field.building') },
    { key: 'capacity', header: t('field.capacity') },
    { key: 'type', header: t('field.type') },
    { key: 'floor', header: t('field.floor') },
    { key: 'campus', header: t('field.campus') },
    { key: 'status', header: t('field.status') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

  const list = usePagedList<Classroom>({ path: '/api/classrooms', token })
  const { loading } = list
  // Export errors are separate from the list fetch (the hook owns its error).
  const [exportError, setExportError] = useState('')
  const error = list.error || exportError

  // Carbon DataTable keys rows by a string id.
  const tableRows = list.items.map((c) => ({ ...c, id: String(c.id) }))

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState<ClassroomForm>(emptyForm)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)

  const [editTarget, setEditTarget] = useState<Classroom | null>(null)
  const [editForm, setEditForm] = useState<ClassroomForm>(emptyForm)
  const [editError, setEditError] = useState('')
  const [editing, setEditing] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState<Classroom | null>(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const validate = (form: ClassroomForm) => {
    if (!form.name.trim()) return t('validation.nameRequired')
    if (!form.capacity || Number(form.capacity) <= 0) return t('validation.capacityPositive')
    return ''
  }

  const buildBody = (form: ClassroomForm): ClassroomInput => ({
    name: form.name.trim(),
    building: form.building.trim(),
    capacity: Number(form.capacity),
    type: form.type,
    floor: form.floor.trim(),
    campus: form.campus.trim(),
    status: form.status,
    description: form.description.trim(),
  })

  const handleCreate = async () => {
    const msg = validate(createForm)
    if (msg) {
      setCreateError(msg)
      return
    }
    try {
      setCreating(true)
      setCreateError('')
      await apiFetch('/api/classrooms', {
        method: 'POST',
        token,
        body: buildBody(createForm),
      })
      setCreateOpen(false)
      setCreateForm(emptyForm)
      list.reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (c: Classroom) => {
    setEditTarget(c)
    setEditForm({
      name: c.name,
      building: c.building,
      capacity: String(c.capacity),
      type: c.type,
      floor: c.floor ?? '',
      campus: c.campus ?? '',
      status: c.status,
      description: c.description,
    })
    setEditError('')
  }

  const handleEdit = async () => {
    if (!editTarget) return
    const msg = validate(editForm)
    if (msg) {
      setEditError(msg)
      return
    }
    try {
      setEditing(true)
      setEditError('')
      await apiFetch(`/api/classrooms/${editTarget.id}`, {
        method: 'PUT',
        token,
        body: buildBody(editForm),
      })
      setEditTarget(null)
      list.reload()
    } catch (err) {
      setEditError((err as Error).message)
    } finally {
      setEditing(false)
    }
  }

  const openDelete = (c: Classroom) => {
    setDeleteTarget(c)
    setDeleteError('')
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      setDeleting(true)
      setDeleteError('')
      await apiFetch(`/api/classrooms/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      list.reload()
    } catch (err) {
      setDeleteError((err as Error).message)
    } finally {
      setDeleting(false)
    }
  }

  const colSpan = headers.length + (canManage ? 1 : 0)

  return (
    <Grid fullWidth className="classrooms-page">
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
        <h1 className="classrooms-page__heading">{t('title')}</h1>
        <p className="classrooms-page__subtitle">{t('subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="classrooms-page__notice"
          />
        )}

        <DataTable rows={tableRows} headers={headers}>
          {({
            rows,
            headers: tableHeaders,
            getTableProps,
            getHeaderProps,
            getRowProps,
            getToolbarProps,
          }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('searchPlaceholder')} />
                  <ExportButton
                    path="/api/classrooms/export"
                    fallbackName="classrooms.xlsx"
                    onError={setExportError}
                  />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={() => setCreateOpen(true)}>
                      {t('addButton')}
                    </Button>
                  )}
                </TableToolbarContent>
              </TableToolbar>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {tableHeaders.map((header) => (
                      <TableHeader {...getHeaderProps({ header })}>
                        {header.header}
                      </TableHeader>
                    ))}
                    {canManage && <TableHeader>{t('field.actions')}</TableHeader>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>{t('empty.loading')}</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>
                        {list.q ? t('empty.search') : t('empty.none')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const c = list.items.find((x) => String(x.id) === String(row.id))
                      if (!c) return null
                      return (
                        <TableRow {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            const value = cell.value as string
                            if (cell.info.header === 'type') {
                              return (
                                <TableCell key={cell.id}>
                                  {t('type.' + value, { defaultValue: value })}
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'status') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={statusKind[value] ?? 'gray'} size="sm">
                                    {t('status.' + value, { ns: 'common', defaultValue: value })}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value as string)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{value}</TableCell>
                          })}
                          {canManage && (
                            <TableCell>
                              <div className="classrooms-page__actions">
                                <Button
                                  kind="ghost"
                                  size="sm"
                                  hasIconOnly
                                  renderIcon={Edit}
                                  iconDescription={t('action.edit', { ns: 'common' })}
                                  onClick={() => openEdit(c)}
                                />
                                <Button
                                  kind="ghost"
                                  size="sm"
                                  hasIconOnly
                                  renderIcon={TrashCan}
                                  iconDescription={t('action.delete', { ns: 'common' })}
                                  onClick={() => openDelete(c)}
                                />
                              </div>
                            </TableCell>
                          )}
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
        modalHeading={t('modal.create')}
        primaryButtonText={t('modal.createSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="classrooms-page__form">
          <TextInput
            id="create-name"
            labelText={t('field.name')}
            placeholder={t('placeholder.name')}
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextInput
            id="create-building"
            labelText={t('field.building')}
            placeholder={t('placeholder.building')}
            value={createForm.building}
            onChange={(e) => setCreateForm({ ...createForm, building: e.target.value })}
          />
          <TextInput
            id="create-capacity"
            type="number"
            labelText={t('field.capacity')}
            min="1"
            value={createForm.capacity}
            onChange={(e) => setCreateForm({ ...createForm, capacity: e.target.value })}
          />
          <Select
            id="create-type"
            labelText={t('field.type')}
            value={createForm.type}
            onChange={(e) => setCreateForm({ ...createForm, type: e.target.value })}
          >
            {TYPE_KEYS.map((k) => (
              <SelectItem key={k} value={k} text={t('type.' + k)} />
            ))}
          </Select>
          <TextInput
            id="create-floor"
            labelText={t('field.floor')}
            placeholder={t('placeholder.floor')}
            value={createForm.floor}
            onChange={(e) => setCreateForm({ ...createForm, floor: e.target.value })}
          />
          <TextInput
            id="create-campus"
            labelText={t('field.campus')}
            placeholder={t('placeholder.campus')}
            value={createForm.campus}
            onChange={(e) => setCreateForm({ ...createForm, campus: e.target.value })}
          />
          <Select
            id="create-status"
            labelText={t('field.status')}
            value={createForm.status}
            onChange={(e) => setCreateForm({ ...createForm, status: e.target.value })}
          >
            {STATUS_KEYS.map((k) => (
              <SelectItem key={k} value={k} text={t('status.' + k, { ns: 'common' })} />
            ))}
          </Select>
          <TextInput
            id="create-description"
            labelText={t('field.description')}
            value={createForm.description}
            onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
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
        <div className="classrooms-page__form">
          <TextInput
            id="edit-name"
            labelText={t('field.name')}
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextInput
            id="edit-building"
            labelText={t('field.building')}
            value={editForm.building}
            onChange={(e) => setEditForm({ ...editForm, building: e.target.value })}
          />
          <TextInput
            id="edit-capacity"
            type="number"
            labelText={t('field.capacity')}
            min="1"
            value={editForm.capacity}
            onChange={(e) => setEditForm({ ...editForm, capacity: e.target.value })}
          />
          <Select
            id="edit-type"
            labelText={t('field.type')}
            value={editForm.type}
            onChange={(e) => setEditForm({ ...editForm, type: e.target.value })}
          >
            {TYPE_KEYS.map((k) => (
              <SelectItem key={k} value={k} text={t('type.' + k)} />
            ))}
          </Select>
          <TextInput
            id="edit-floor"
            labelText={t('field.floor')}
            value={editForm.floor}
            onChange={(e) => setEditForm({ ...editForm, floor: e.target.value })}
          />
          <TextInput
            id="edit-campus"
            labelText={t('field.campus')}
            value={editForm.campus}
            onChange={(e) => setEditForm({ ...editForm, campus: e.target.value })}
          />
          <Select
            id="edit-status"
            labelText={t('field.status')}
            value={editForm.status}
            onChange={(e) => setEditForm({ ...editForm, status: e.target.value })}
          >
            {STATUS_KEYS.map((k) => (
              <SelectItem key={k} value={k} text={t('status.' + k, { ns: 'common' })} />
            ))}
          </Select>
          <TextInput
            id="edit-description"
            labelText={t('field.description')}
            value={editForm.description}
            onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
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
        <p className="classrooms-page__confirm-text">
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
