import { useCallback, useEffect, useState } from 'react'
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
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'

// 教学班 (teaching class): a named group of admin classes taught together (合班).
// An offering is taught to exactly one teaching class; two offerings of the same
// course/teacher/semester taught to different groups are two teaching classes.
const headers = [
  { key: 'id', header: 'ID' },
  { key: 'name', header: '教学班名称' },
  { key: 'members', header: '包含行政班' },
  { key: 'note', header: '备注' },
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

const classLabel = (c) => (c.grade ? `${c.grade}/${c.name}` : c.name)
const emptyForm = { name: '', note: '', classIds: [] }

export default function TeachingClassesPage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const canManage = currentUser?.role === 'admin'

  const [teachingClasses, setTeachingClasses] = useState([])
  const [adminClasses, setAdminClasses] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

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

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true)
      setError('')
      const [tc, ac] = await Promise.all([
        apiFetch('/api/teaching-classes', { token }),
        apiFetch('/api/admin-classes', { token }),
      ])
      setTeachingClasses(Array.isArray(tc) ? tc : [])
      setAdminClasses(Array.isArray(ac) ? ac : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  const validate = (form) => {
    if (!form.name.trim()) return '教学班名称为必填项'
    if (form.classIds.length === 0) return '至少选择一个行政班'
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
      await fetchAll()
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (t) => {
    setEditTarget(t)
    setEditForm({
      name: t.name,
      note: t.note,
      classIds: (t.classes || []).map((c) => c.id),
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
      await fetchAll()
    } catch (err) {
      setEditError(err.message)
    } finally {
      setEditing(false)
    }
  }

  const openDelete = (t) => {
    setDeleteTarget(t)
    setDeleteError('')
  }

  const handleDelete = async () => {
    try {
      setDeleting(true)
      setDeleteError('')
      await apiFetch(`/api/teaching-classes/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      await fetchAll()
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

  const rows = teachingClasses.map((t) => ({
    id: String(t.id),
    name: t.name,
    members: (t.classes || []).map(classLabel).join('、') || '-',
    note: t.note,
    createdAt: t.createdAt,
  }))

  const colSpan = headers.length + (canManage ? 1 : 0)

  const renderMemberSelect = (form, setForm) => (
    <MultiSelect
      id="tc-classes"
      titleText="包含行政班"
      items={adminClasses}
      itemToString={classLabel}
      selection={selectionFor(form.classIds)}
      onChange={({ selectedItems }) => setForm({ ...form, classIds: selectedItems.map((i) => i.id) })}
      label="选择行政班"
      disabled={adminClasses.length === 0}
    />
  )

  return (
    <Grid fullWidth className="courses-page">
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
          <BreadcrumbItem isCurrentPage>教学班管理</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">教学班管理</h1>
        <p className="courses-page__subtitle">
          教学班由若干行政班合班而成，是开课的归属单位。同一课程、教师、学期面向不同群体开课时，通过不同教学班区分。
        </p>
        {adminClasses.length === 0 && (
          <InlineNotification
            kind="info"
            title="请先创建行政班"
            subtitle="教学班需引用行政班作为成员，请先在「行政班管理」中创建。"
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
            title="加载失败"
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <DataTable rows={rows} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps, onInputChange }) => (
            <TableContainer title="教学班列表" description={`共 ${teachingClasses.length} 个教学班`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch onChange={onInputChange} placeholder="搜索教学班" />
                  <ExportButton
                    path="/api/teaching-classes/export"
                    fallbackName="teaching-classes.xlsx"
                    onError={setError}
                  />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={() => setCreateOpen(true)}>
                      添加教学班
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
                    {canManage && <TableHeader>操作</TableHeader>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>加载中…</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>
                        {teachingClasses.length === 0 ? '暂无教学班' : '未找到匹配的教学班'}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const t = teachingClasses.find((x) => String(x.id) === String(row.id))
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
                                  iconDescription="编辑"
                                  onClick={() => openEdit(t)}
                                />
                                <Button
                                  kind="ghost"
                                  size="sm"
                                  hasIconOnly
                                  renderIcon={TrashCan}
                                  iconDescription="删除"
                                  onClick={() => openDelete(t)}
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
      </Column>

      {/* Create */}
      <Modal
        open={createOpen}
        modalHeading="添加教学班"
        primaryButtonText="创建"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="courses-page__form">
          <TextInput
            id="tc-name"
            labelText="教学班名称"
            placeholder="如 高数-A班（1+2）"
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          {renderMemberSelect(createForm, setCreateForm)}
          <TextInput
            id="tc-note"
            labelText="备注"
            value={createForm.note}
            onChange={(e) => setCreateForm({ ...createForm, note: e.target.value })}
          />
          {createError && (
            <InlineNotification kind="error" title="创建失败" subtitle={createError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Edit */}
      <Modal
        open={Boolean(editTarget)}
        modalHeading={`编辑教学班：${editTarget?.name ?? ''}`}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
      >
        <div className="courses-page__form">
          <TextInput
            id="tc-edit-name"
            labelText="教学班名称"
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          {renderMemberSelect(editForm, setEditForm)}
          <TextInput
            id="tc-edit-note"
            labelText="备注"
            value={editForm.note}
            onChange={(e) => setEditForm({ ...editForm, note: e.target.value })}
          />
          {editError && (
            <InlineNotification kind="error" title="保存失败" subtitle={editError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Delete */}
      <Modal
        danger
        open={Boolean(deleteTarget)}
        modalHeading="删除教学班"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="courses-page__confirm-text">
          确定要删除教学班「{deleteTarget?.name}」吗？若已有开课引用该教学班，需先移除引用。此操作不可撤销。
        </p>
        {deleteError && (
          <InlineNotification kind="error" title="删除失败" subtitle={deleteError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
