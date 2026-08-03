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
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'

const headers = [
  { key: 'id', header: 'ID' },
  { key: 'name', header: '教室编号' },
  { key: 'building', header: '楼栋' },
  { key: 'capacity', header: '座位数' },
  { key: 'type', header: '类型' },
  { key: 'status', header: '状态' },
  { key: 'createdAt', header: '创建时间' },
]

const typeLabel = {
  standard: '普通教室',
  multimedia: '多媒体教室',
  computer: '机房',
  lab: '实验室',
  lecture_hall: '报告厅',
}

const statusLabel = {
  available: '可用',
  maintenance: '维修中',
  disabled: '停用',
}

const statusKind = {
  available: 'green',
  maintenance: 'blue',
  disabled: 'red',
}

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

const emptyForm = {
  name: '',
  building: '',
  capacity: '',
  type: 'standard',
  status: 'available',
  description: '',
}

export default function ClassroomsPage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const canManage = currentUser?.role === 'admin'

  const [classrooms, setClassrooms] = useState([])
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

  const fetchClassrooms = useCallback(async () => {
    try {
      setLoading(true)
      setError('')
      const data = await apiFetch('/api/classrooms', { token })
      setClassrooms(Array.isArray(data) ? data : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchClassrooms()
  }, [fetchClassrooms])

  const validate = (form) => {
    if (!form.name.trim()) return '教室编号为必填项'
    if (!form.capacity || Number(form.capacity) <= 0) return '座位数必须大于 0'
    return ''
  }

  const buildBody = (form) => ({
    name: form.name.trim(),
    building: form.building.trim(),
    capacity: Number(form.capacity),
    type: form.type,
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
      await fetchClassrooms()
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (c) => {
    setEditTarget(c)
    setEditForm({
      name: c.name,
      building: c.building,
      capacity: String(c.capacity),
      type: c.type,
      status: c.status,
      description: c.description,
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
      await apiFetch(`/api/classrooms/${editTarget.id}`, {
        method: 'PUT',
        token,
        body: buildBody(editForm),
      })
      setEditTarget(null)
      await fetchClassrooms()
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
      await apiFetch(`/api/classrooms/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      await fetchClassrooms()
    } catch (err) {
      setDeleteError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  const colSpan = headers.length + (canManage ? 1 : 0)

  return (
    <Grid fullWidth className="classrooms-page">
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
          <BreadcrumbItem isCurrentPage>教室管理</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="classrooms-page__heading">教室管理</h1>
        <p className="classrooms-page__subtitle">
          维护教室基础信息，为后续预约与报修提供数据基础。
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
            className="classrooms-page__notice"
          />
        )}

        <DataTable rows={classrooms} headers={headers}>
          {({
            rows,
            headers: tableHeaders,
            getTableProps,
            getHeaderProps,
            getRowProps,
            getToolbarProps,
            onInputChange,
          }) => (
            <TableContainer title="教室列表" description={`共 ${classrooms.length} 间教室`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch onChange={onInputChange} placeholder="搜索教室" />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={() => setCreateOpen(true)}>
                      添加教室
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
                        {classrooms.length === 0 ? '暂无教室' : '未找到匹配的教室'}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const c = classrooms.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'type') {
                              return (
                                <TableCell key={cell.id}>
                                  {typeLabel[cell.value] ?? cell.value}
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'status') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={statusKind[cell.value] ?? 'gray'} size="sm">
                                    {statusLabel[cell.value] ?? cell.value}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value}</TableCell>
                          })}
                          {canManage && (
                            <TableCell>
                              <div className="classrooms-page__actions">
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
        modalHeading="添加教室"
        primaryButtonText="创建"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="classrooms-page__form">
          <TextInput
            id="create-name"
            labelText="教室编号"
            placeholder="如 A301"
            value={createForm.name}
            onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
          />
          <TextInput
            id="create-building"
            labelText="楼栋"
            placeholder="如 第一教学楼"
            value={createForm.building}
            onChange={(e) => setCreateForm({ ...createForm, building: e.target.value })}
          />
          <TextInput
            id="create-capacity"
            type="number"
            labelText="座位数"
            min="1"
            value={createForm.capacity}
            onChange={(e) => setCreateForm({ ...createForm, capacity: e.target.value })}
          />
          <Select
            id="create-type"
            labelText="类型"
            value={createForm.type}
            onChange={(e) => setCreateForm({ ...createForm, type: e.target.value })}
          >
            <SelectItem value="standard" text="普通教室" />
            <SelectItem value="multimedia" text="多媒体教室" />
            <SelectItem value="computer" text="机房" />
            <SelectItem value="lab" text="实验室" />
            <SelectItem value="lecture_hall" text="报告厅" />
          </Select>
          <Select
            id="create-status"
            labelText="状态"
            value={createForm.status}
            onChange={(e) => setCreateForm({ ...createForm, status: e.target.value })}
          >
            <SelectItem value="available" text="可用" />
            <SelectItem value="maintenance" text="维修中" />
            <SelectItem value="disabled" text="停用" />
          </Select>
          <TextInput
            id="create-description"
            labelText="备注"
            value={createForm.description}
            onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
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
        modalHeading={`编辑教室：${editTarget?.name ?? ''}`}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
      >
        <div className="classrooms-page__form">
          <TextInput
            id="edit-name"
            labelText="教室编号"
            value={editForm.name}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
          />
          <TextInput
            id="edit-building"
            labelText="楼栋"
            value={editForm.building}
            onChange={(e) => setEditForm({ ...editForm, building: e.target.value })}
          />
          <TextInput
            id="edit-capacity"
            type="number"
            labelText="座位数"
            min="1"
            value={editForm.capacity}
            onChange={(e) => setEditForm({ ...editForm, capacity: e.target.value })}
          />
          <Select
            id="edit-type"
            labelText="类型"
            value={editForm.type}
            onChange={(e) => setEditForm({ ...editForm, type: e.target.value })}
          >
            <SelectItem value="standard" text="普通教室" />
            <SelectItem value="multimedia" text="多媒体教室" />
            <SelectItem value="computer" text="机房" />
            <SelectItem value="lab" text="实验室" />
            <SelectItem value="lecture_hall" text="报告厅" />
          </Select>
          <Select
            id="edit-status"
            labelText="状态"
            value={editForm.status}
            onChange={(e) => setEditForm({ ...editForm, status: e.target.value })}
          >
            <SelectItem value="available" text="可用" />
            <SelectItem value="maintenance" text="维修中" />
            <SelectItem value="disabled" text="停用" />
          </Select>
          <TextInput
            id="edit-description"
            labelText="备注"
            value={editForm.description}
            onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
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
        modalHeading="删除教室"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="classrooms-page__confirm-text">
          确定要删除教室「{deleteTarget?.name}」吗？此操作不可撤销。
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
