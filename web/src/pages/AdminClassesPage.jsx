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

// 行政班 (admin class): a persistent student cohort identified by grade + name.
// Managed here because it is an organizational unit, not a course-delivery
// concept; teaching classes (合班) reference admin classes as members.
const headers = [
  { key: 'id', header: 'ID' },
  { key: 'grade', header: '年级' },
  { key: 'name', header: '班级名称' },
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

const emptyForm = { grade: '', name: '', note: '' }

export default function AdminClassesPage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const canManage = currentUser?.role === 'admin'

  const [classes, setClasses] = useState([])
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

  const fetchClasses = useCallback(async () => {
    try {
      setLoading(true)
      setError('')
      const data = await apiFetch('/api/admin-classes', { token })
      setClasses(Array.isArray(data) ? data : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchClasses()
  }, [fetchClasses])

  const validate = (form) => {
    if (!form.name.trim()) return '班级名称为必填项'
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
      await fetchClasses()
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
      await fetchClasses()
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
      await fetchClasses()
    } catch (err) {
      setDeleteError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  const colSpan = headers.length + (canManage ? 1 : 0)

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
          <BreadcrumbItem isCurrentPage>行政班管理</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">行政班管理</h1>
        <p className="courses-page__subtitle">
          维护行政班（年级 + 班级名称），作为教学班合班与排课的基础数据。
        </p>
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

        <DataTable rows={classes} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps, onInputChange }) => (
            <TableContainer title="行政班列表" description={`共 ${classes.length} 个行政班`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch onChange={onInputChange} placeholder="搜索行政班" />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={() => setCreateOpen(true)}>
                      添加行政班
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
                        {classes.length === 0 ? '暂无行政班' : '未找到匹配的行政班'}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const c = classes.find((x) => String(x.id) === String(row.id))
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
                                  onClick={() => openEdit(c)}
                                />
                                <Button
                                  kind="ghost"
                                  size="sm"
                                  hasIconOnly
                                  renderIcon={TrashCan}
                                  iconDescription="删除"
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
      </Column>

      {/* Create */}
      <Modal
        open={createOpen}
        modalHeading="添加行政班"
        primaryButtonText="创建"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="courses-page__form">
          <TextInput
            id="ac-grade"
            labelText="年级"
            placeholder="如 2024级"
            value={createForm.grade}
            onChange={(e) => setCreateForm({ ...createForm, grade: e.target.value })}
          />
          <TextInput
            id="ac-name"
            labelText="班级名称"
            placeholder="如 计算机244"
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextInput
            id="ac-note"
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
        modalHeading={`编辑行政班：${editTarget?.name ?? ''}`}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
      >
        <div className="courses-page__form">
          <TextInput
            id="ac-edit-grade"
            labelText="年级"
            value={editForm.grade}
            onChange={(e) => setEditForm({ ...editForm, grade: e.target.value })}
          />
          <TextInput
            id="ac-edit-name"
            labelText="班级名称"
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextInput
            id="ac-edit-note"
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
        modalHeading="删除行政班"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="courses-page__confirm-text">
          确定要删除行政班「{deleteTarget?.name}」吗？若该班已被教学班引用，需先移除引用。此操作不可撤销。
        </p>
        {deleteError && (
          <InlineNotification kind="error" title="删除失败" subtitle={deleteError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
