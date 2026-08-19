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
  TableToolbarSearch,
  TextInput,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'
import { formatDate } from '../i18n/formatters.js'

// classLabel formats an admin class as "grade/name" (or just name when the
// grade is empty). grade/name are backend data, so the format is locale-neutral.
const classLabel = (c) => (c.grade ? `${c.grade}/${c.name}` : c.name)
const emptyForm = { name: '', note: '', classIds: [] }

export default function TeachingClassesPage() {
  const { t, i18n } = useTranslation('teachingClasses')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('teaching_class:manage')

  const headers = [
    { key: 'id', header: t('field.id') },
    { key: 'name', header: t('field.name') },
    { key: 'members', header: t('field.members') },
    { key: 'note', header: t('field.note') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

  const list = usePagedList({ path: '/api/teaching-classes', token })
  const { loading } = list
  const [adminClasses, setAdminClasses] = useState([])
  // Export errors are separate from the list fetch (the hook owns its error).
  const [exportError, setExportError] = useState('')
  const error = list.error || exportError

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

  // The admin-class dropdown needs the full option list; pull the maximum page.
  useEffect(() => {
    let cancelled = false
    apiFetch('/api/admin-classes?page_size=500', { token })
      .then((data) => {
        if (cancelled) return
        setAdminClasses(Array.isArray(data?.items) ? data.items : [])
      })
      .catch(() => {
        if (!cancelled) setAdminClasses([])
      })
    return () => {
      cancelled = true
    }
  }, [token])

  const validate = (form) => {
    if (!form.name.trim()) return t('validation.nameRequired')
    if (form.classIds.length === 0) return t('validation.selectClass')
    return ''
  }

  const buildBody = (form) => ({
    name: form.name.trim(),
    note: form.note.trim(),
    classIds: form.classIds,
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
      await apiFetch('/api/teaching-classes', { method: 'POST', token, body: buildBody(createForm) })
      setCreateOpen(false)
      setCreateForm(emptyForm)
      list.reload()
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (tc) => {
    setEditTarget(tc)
    setEditForm({
      name: tc.name,
      note: tc.note,
      classIds: (tc.classes || []).map((c) => c.id),
    })
    setEditError('')
  }

  const handleEdit = async () => {
    const msg = validate(editForm)
    if (msg) {
      setEditError(msg)
      return
    }
    try {
      setEditing(true)
      setEditError('')
      await apiFetch(`/api/teaching-classes/${editTarget.id}`, { method: 'PUT', token, body: buildBody(editForm) })
      setEditTarget(null)
      list.reload()
    } catch (err) {
      setEditError(err.message)
    } finally {
      setEditing(false)
    }
  }

  const openDelete = (tc) => {
    setDeleteTarget(tc)
    setDeleteError('')
  }

  const handleDelete = async () => {
    try {
      setDeleting(true)
      setDeleteError('')
      await apiFetch(`/api/teaching-classes/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      list.reload()
    } catch (err) {
      setDeleteError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  // Derive MultiSelect selection (item objects) from stored IDs so the dropdown
  // check state stays in sync. Items come from adminClasses state, so the
  // object references match what MultiSelect holds.
  const selectionFor = (ids) => adminClasses.filter((a) => ids.includes(a.id))

  // Members column joins admin-class labels with a locale-appropriate separator
  // (zh: "、", en: ", ") via Intl.ListFormat narrow style.
  const listFmt = new Intl.ListFormat(i18n.language, { style: 'narrow' })
  const formatMembers = (classes) => {
    const labels = (classes || []).map(classLabel)
    return labels.length ? listFmt.format(labels) : '-'
  }
  const rows = list.items.map((tc) => ({
    id: String(tc.id),
    name: tc.name,
    members: formatMembers(tc.classes),
    note: tc.note,
    createdAt: tc.createdAt,
  }))

  const colSpan = headers.length + (canManage ? 1 : 0)

  const renderMemberSelect = (form, setForm) => (
    <MultiSelect
      id="tc-classes"
      titleText={t('form.classes')}
      items={adminClasses}
      itemToString={classLabel}
      selection={selectionFor(form.classIds)}
      onChange={({ selectedItems }) => setForm({ ...form, classIds: selectedItems.map((i) => i.id) })}
      label={t('classesPicker.label')}
      disabled={adminClasses.length === 0}
    />
  )

  return (
    <Grid fullWidth className="courses-page">
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
        <h1 className="courses-page__heading">{t('title')}</h1>
        <p className="courses-page__subtitle">{t('subtitle')}</p>
        {adminClasses.length === 0 && (
          <InlineNotification
            kind="info"
            title={t('info.title')}
            subtitle={t('info.subtitle')}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <DataTable rows={rows} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('searchPlaceholder')} />
                  <ExportButton
                    path="/api/teaching-classes/export"
                    fallbackName="teaching-classes.xlsx"
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
                      <TableHeader key={header.key} {...getHeaderProps({ header })}>
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
                      const tc = list.items.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value || '-'}</TableCell>
                          })}
                          {canManage && (
                            <TableCell>
                              <div className="courses-page__actions">
                                <Button
                                  kind="ghost"
                                  size="sm"
                                  hasIconOnly
                                  renderIcon={Edit}
                                  iconDescription={t('action.edit', { ns: 'common' })}
                                  onClick={() => openEdit(tc)}
                                />
                                <Button
                                  kind="ghost"
                                  size="sm"
                                  hasIconOnly
                                  renderIcon={TrashCan}
                                  iconDescription={t('action.delete', { ns: 'common' })}
                                  onClick={() => openDelete(tc)}
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
        <div className="courses-page__form">
          <TextInput
            id="tc-name"
            labelText={t('form.name')}
            placeholder={t('placeholder.name')}
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          {renderMemberSelect(createForm, setCreateForm)}
          <TextInput
            id="tc-note"
            labelText={t('form.note')}
            value={createForm.note}
            onChange={(e) => setCreateForm({ ...createForm, note: e.target.value })}
          />
          {createError && (
            <InlineNotification kind="error" title={t('error.create')} subtitle={createError} lowContrast hideCloseButton />
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
        <div className="courses-page__form">
          <TextInput
            id="tc-edit-name"
            labelText={t('form.name')}
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          {renderMemberSelect(editForm, setEditForm)}
          <TextInput
            id="tc-edit-note"
            labelText={t('form.note')}
            value={editForm.note}
            onChange={(e) => setEditForm({ ...editForm, note: e.target.value })}
          />
          {editError && (
            <InlineNotification kind="error" title={t('error.save')} subtitle={editError} lowContrast hideCloseButton />
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
        <p className="courses-page__confirm-text">
          {t('deleteConfirm', { name: deleteTarget?.name })}
        </p>
        {deleteError && (
          <InlineNotification kind="error" title={t('error.delete')} subtitle={deleteError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
