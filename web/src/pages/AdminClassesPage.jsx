import { useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  ComboBox,
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
  TableToolbarSearch,
  TextInput,
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

const emptyForm = { grade: '', name: '', note: '' }

export default function AdminClassesPage() {
  const { t } = useTranslation('adminClasses')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('admin_class:manage')
  // Roster (学生档案) is gated by the attendance permissions: teachers may
  // curate members even without admin-class manage rights.
  const canRoster = can('attendance:read')
  const canRosterManage = can('attendance:manage')

  const headers = [
    { key: 'id', header: t('field.id') },
    { key: 'grade', header: t('field.grade') },
    { key: 'name', header: t('field.name') },
    { key: 'note', header: t('field.note') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

  const list = usePagedList({ path: '/api/admin-classes', token })
  const { loading } = list
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

  // Roster modal (成员) state.
  const [membersTarget, setMembersTarget] = useState(null)
  const [members, setMembers] = useState([])
  const [membersLoading, setMembersLoading] = useState(false)
  const [membersError, setMembersError] = useState('')
  const [studentOptions, setStudentOptions] = useState([])
  const [pickStudent, setPickStudent] = useState(null) // { id, text } of the ComboBox selection
  const [pickNo, setPickNo] = useState('')
  const [addError, setAddError] = useState('')
  const [adding, setAdding] = useState(false)
  const [editMember, setEditMember] = useState(null) // { userId, studentNo, note }
  const [editMemberError, setEditMemberError] = useState('')
  const [savingMember, setSavingMember] = useState(false)
  const [removeMember, setRemoveMember] = useState(null)
  const [removeError, setRemoveError] = useState('')
  const [removing, setRemoving] = useState(false)

  const loadMembers = async (id) => {
    try {
      setMembersLoading(true)
      setMembersError('')
      const data = await apiFetch(`/api/admin-classes/${id}/students`, { token })
      setMembers(data || [])
    } catch (err) {
      setMembersError(err.message)
    } finally {
      setMembersLoading(false)
    }
  }

  const openMembers = (c) => {
    setMembersTarget(c)
    setMembers([])
    setMembersError('')
    setPickStudent(null)
    setPickNo('')
    setAddError('')
    setStudentOptions([])
    loadMembers(c.id)
  }

  // ComboBox search: query users and keep only student accounts. The backend
  // validates the type again on add.
  const searchStudents = async (q) => {
    try {
      const data = await apiFetch(`/api/users?q=${encodeURIComponent(q)}&page_size=50`, { token })
      setStudentOptions(
        ((data && data.items) || [])
          .filter((u) => u.type === 'student')
          .map((u) => ({ id: String(u.id), text: t('picker.userOption', { name: u.displayName, username: u.username }) }))
      )
    } catch {
      setStudentOptions([])
    }
  }

  const handleAddMember = async () => {
    if (!pickStudent) {
      setAddError(t('validation.selectStudent'))
      return
    }
    try {
      setAdding(true)
      setAddError('')
      await apiFetch(`/api/admin-classes/${membersTarget.id}/students`, {
        method: 'POST',
        token,
        body: { userId: Number(pickStudent.id), studentNo: pickNo.trim(), note: '' },
      })
      setPickStudent(null)
      setPickNo('')
      setStudentOptions([])
      await loadMembers(membersTarget.id)
    } catch (err) {
      setAddError(err.message)
    } finally {
      setAdding(false)
    }
  }

  const openEditMember = (m) => {
    setEditMember({ userId: m.userId, studentNo: m.studentNo, note: m.note })
    setEditMemberError('')
  }

  const handleSaveMember = async () => {
    try {
      setSavingMember(true)
      setEditMemberError('')
      await apiFetch(`/api/admin-classes/${membersTarget.id}/students/${editMember.userId}`, {
        method: 'PUT',
        token,
        body: { studentNo: editMember.studentNo.trim(), note: editMember.note.trim() },
      })
      setEditMember(null)
      await loadMembers(membersTarget.id)
    } catch (err) {
      setEditMemberError(err.message)
    } finally {
      setSavingMember(false)
    }
  }

  const openRemoveMember = (m) => {
    setRemoveMember(m)
    setRemoveError('')
  }

  const handleRemoveMember = async () => {
    try {
      setRemoving(true)
      setRemoveError('')
      await apiFetch(`/api/admin-classes/${membersTarget.id}/students/${removeMember.userId}`, {
        method: 'DELETE',
        token,
      })
      setRemoveMember(null)
      await loadMembers(membersTarget.id)
    } catch (err) {
      setRemoveError(err.message)
    } finally {
      setRemoving(false)
    }
  }

  const validate = (form) => {
    if (!form.name.trim()) return t('validation.nameRequired')
    return ''
  }

  const buildBody = (form) => ({
    grade: form.grade.trim(),
    name: form.name.trim(),
    note: form.note.trim(),
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
      await apiFetch('/api/admin-classes', { method: 'POST', token, body: buildBody(createForm) })
      setCreateOpen(false)
      setCreateForm(emptyForm)
      list.reload()
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (c) => {
    setEditTarget(c)
    setEditForm({ grade: c.grade, name: c.name, note: c.note })
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
      await apiFetch(`/api/admin-classes/${editTarget.id}`, { method: 'PUT', token, body: buildBody(editForm) })
      setEditTarget(null)
      list.reload()
    } catch (err) {
      setEditError(err.message)
    } finally {
      setEditing(false)
    }
  }

  const openDelete = (c) => {
    setDeleteTarget(c)
    setDeleteError('')
  }

  const handleDelete = async () => {
    try {
      setDeleting(true)
      setDeleteError('')
      await apiFetch(`/api/admin-classes/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      list.reload()
    } catch (err) {
      setDeleteError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  const canAdmin = canManage || canRoster
  const colSpan = headers.length + (canAdmin ? 1 : 0)

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

        <DataTable rows={list.items} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('searchPlaceholder')} />
                  <ExportButton
                    path="/api/admin-classes/export"
                    fallbackName="admin-classes.xlsx"
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
                    {canAdmin && <TableHeader>{t('field.actions')}</TableHeader>}
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
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value || '-'}</TableCell>
                          })}
                          {canAdmin && (
                            <TableCell>
                              <div className="courses-page__actions">
                                {canManage && (
                                  <>
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
                                  </>
                                )}
                                {canRoster && (
                                  <Button kind="ghost" size="sm" onClick={() => openMembers(c)}>
                                    {t('field.members')}
                                  </Button>
                                )}
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
            id="ac-grade"
            labelText={t('form.grade')}
            placeholder={t('placeholder.grade')}
            value={createForm.grade}
            onChange={(e) => setCreateForm({ ...createForm, grade: e.target.value })}
          />
          <TextInput
            id="ac-name"
            labelText={t('form.name')}
            placeholder={t('placeholder.name')}
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextInput
            id="ac-note"
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
            id="ac-edit-grade"
            labelText={t('form.grade')}
            value={editForm.grade}
            onChange={(e) => setEditForm({ ...editForm, grade: e.target.value })}
          />
          <TextInput
            id="ac-edit-name"
            labelText={t('form.name')}
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextInput
            id="ac-edit-note"
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

      {/* Members (学生档案): attendance-gated roster of one admin class. */}
      <Modal
        open={Boolean(membersTarget)}
        modalHeading={t('modal.members', { name: membersTarget?.name ?? '' })}
        primaryButtonText={t('modal.membersSubmit')}
        onRequestClose={() => {
          setMembersTarget(null)
          setEditMember(null)
          setRemoveMember(null)
        }}
        onRequestSubmit={() => setMembersTarget(null)}
        size="lg"
      >
        <div className="courses-page__form">
          <p className="courses-page__subtitle">{t('membersSubtitle')}</p>
          {canRosterManage && (
            <div className="courses-page__member-add">
              <ComboBox
                id="member-pick"
                titleText={t('memberAdd.title')}
                placeholder={t('memberAdd.placeholder')}
                items={studentOptions}
                itemToString={(item) => (item ? item.text : '')}
                selectedItem={pickStudent}
                onChange={(e) => {
                  if (e.selectedItem) {
                    setPickStudent(e.selectedItem)
                  } else {
                    setPickStudent(null)
                    searchStudents(e.inputValue ?? '')
                  }
                }}
                shouldFilterItem={() => true}
              />
              <TextInput
                id="member-no"
                labelText={t('memberAdd.studentNoLabel')}
                placeholder={t('placeholder.studentNo')}
                value={pickNo}
                onChange={(e) => setPickNo(e.target.value)}
              />
              <Button size="sm" onClick={handleAddMember} disabled={adding || !pickStudent}>
                {adding ? t('memberAdd.buttonLoading') : t('memberAdd.button')}
              </Button>
              {addError && (
                <InlineNotification kind="error" title={t('error.addMember')} subtitle={addError} lowContrast hideCloseButton />
              )}
            </div>
          )}
          {membersError && (
            <InlineNotification kind="error" title={t('error.load')} subtitle={membersError} lowContrast hideCloseButton />
          )}
          {membersLoading ? (
            <p>{t('empty.loading')}</p>
          ) : members.length === 0 ? (
            <p>
              {t('empty.members')}
              {canRosterManage ? t('empty.membersHint') : ''}
            </p>
          ) : (
            <TableContainer title={t('memberTable.description', { count: members.length })}>
              <Table size="sm">
                <TableHead>
                  <TableRow>
                    <TableHeader>{t('memberTable.name')}</TableHeader>
                    <TableHeader>{t('memberTable.username')}</TableHeader>
                    <TableHeader>{t('memberTable.studentNo')}</TableHeader>
                    {canRosterManage && <TableHeader>{t('memberTable.actions')}</TableHeader>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {members.map((m) => (
                    <TableRow key={m.userId}>
                      <TableCell>{m.displayName}</TableCell>
                      <TableCell>{m.username}</TableCell>
                      <TableCell>{m.studentNo || '-'}</TableCell>
                      {canRosterManage && (
                        <TableCell>
                          <div className="courses-page__actions">
                            <Button kind="ghost" size="sm" onClick={() => openEditMember(m)}>
                              {t('memberTable.edit')}
                            </Button>
                            <Button kind="ghost" size="sm" onClick={() => openRemoveMember(m)}>
                              {t('memberTable.remove')}
                            </Button>
                          </div>
                        </TableCell>
                      )}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </div>
      </Modal>

      {/* Edit member metadata */}
      <Modal
        open={Boolean(editMember)}
        modalHeading={t('modal.editMember')}
        primaryButtonText={t('modal.editMemberSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setEditMember(null)}
        onRequestSubmit={handleSaveMember}
        primaryButtonDisabled={savingMember}
      >
        <div className="courses-page__form">
          <TextInput
            id="member-edit-no"
            labelText={t('memberEditForm.studentNo')}
            value={editMember?.studentNo ?? ''}
            onChange={(e) => setEditMember({ ...editMember, studentNo: e.target.value })}
          />
          <TextInput
            id="member-edit-note"
            labelText={t('memberEditForm.note')}
            value={editMember?.note ?? ''}
            onChange={(e) => setEditMember({ ...editMember, note: e.target.value })}
          />
          {editMemberError && (
            <InlineNotification kind="error" title={t('error.saveMember')} subtitle={editMemberError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Remove member */}
      <Modal
        danger
        open={Boolean(removeMember)}
        modalHeading={t('modal.removeMember')}
        primaryButtonText={t('modal.removeMemberSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setRemoveMember(null)}
        onRequestSubmit={handleRemoveMember}
        primaryButtonDisabled={removing}
      >
        <p className="courses-page__confirm-text">
          {t('removeMemberConfirm', { name: removeMember?.displayName })}
        </p>
        {removeError && (
          <InlineNotification kind="error" title={t('error.removeMember')} subtitle={removeError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
