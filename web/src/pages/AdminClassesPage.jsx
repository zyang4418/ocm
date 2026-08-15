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
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'

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
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('admin_class:manage')
  // Roster (学生档案) is gated by the attendance permissions: teachers may
  // curate members even without admin-class manage rights.
  const canRoster = can('attendance:read')
  const canRosterManage = can('attendance:manage')

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
          .map((u) => ({ id: String(u.id), text: `${u.displayName}（${u.username}）` }))
      )
    } catch {
      setStudentOptions([])
    }
  }

  const handleAddMember = async () => {
    if (!pickStudent) {
      setAddError('请选择学生')
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

        <DataTable rows={list.items} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title="行政班列表" description={`共 ${list.total} 个行政班`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder="搜索行政班" />
                  <ExportButton
                    path="/api/admin-classes/export"
                    fallbackName="admin-classes.xlsx"
                    onError={setExportError}
                  />
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
                    {canAdmin && <TableHeader>操作</TableHeader>}
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
                        {list.q ? '未找到匹配的行政班' : '暂无行政班'}
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
                                  </>
                                )}
                                {canRoster && (
                                  <Button kind="ghost" size="sm" onClick={() => openMembers(c)}>
                                    成员
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

      {/* Members (学生档案): attendance-gated roster of one admin class. */}
      <Modal
        open={Boolean(membersTarget)}
        modalHeading={`班级成员：${membersTarget?.name ?? ''}`}
        primaryButtonText="关闭"
        onRequestClose={() => {
          setMembersTarget(null)
          setEditMember(null)
          setRemoveMember(null)
        }}
        onRequestSubmit={() => setMembersTarget(null)}
        size="lg"
      >
        <div className="courses-page__form">
          <p className="courses-page__subtitle">
            维护学生档案（账号 ↔ 行政班）。名单用于课堂签到的应到统计，学生可在此添加或移除。
          </p>
          {canRosterManage && (
            <div className="courses-page__member-add">
              <ComboBox
                id="member-pick"
                titleText="添加学生"
                placeholder="输入姓名/用户名搜索学生"
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
                labelText="学号（可选）"
                placeholder="如 2023001"
                value={pickNo}
                onChange={(e) => setPickNo(e.target.value)}
              />
              <Button size="sm" onClick={handleAddMember} disabled={adding || !pickStudent}>
                {adding ? '添加中…' : '添加'}
              </Button>
              {addError && (
                <InlineNotification kind="error" title="添加失败" subtitle={addError} lowContrast hideCloseButton />
              )}
            </div>
          )}
          {membersError && (
            <InlineNotification kind="error" title="加载失败" subtitle={membersError} lowContrast hideCloseButton />
          )}
          {membersLoading ? (
            <p>加载中…</p>
          ) : members.length === 0 ? (
            <p>暂无成员。{canRosterManage ? '请通过上方搜索添加学生。' : ''}</p>
          ) : (
            <TableContainer title={`共 ${members.length} 名学生`}>
              <Table size="sm">
                <TableHead>
                  <TableRow>
                    <TableHeader>姓名</TableHeader>
                    <TableHeader>用户名</TableHeader>
                    <TableHeader>学号</TableHeader>
                    {canRosterManage && <TableHeader>操作</TableHeader>}
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
                              编辑
                            </Button>
                            <Button kind="ghost" size="sm" onClick={() => openRemoveMember(m)}>
                              移除
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
        modalHeading="编辑学生档案"
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditMember(null)}
        onRequestSubmit={handleSaveMember}
        primaryButtonDisabled={savingMember}
      >
        <div className="courses-page__form">
          <TextInput
            id="member-edit-no"
            labelText="学号"
            value={editMember?.studentNo ?? ''}
            onChange={(e) => setEditMember({ ...editMember, studentNo: e.target.value })}
          />
          <TextInput
            id="member-edit-note"
            labelText="备注"
            value={editMember?.note ?? ''}
            onChange={(e) => setEditMember({ ...editMember, note: e.target.value })}
          />
          {editMemberError && (
            <InlineNotification kind="error" title="保存失败" subtitle={editMemberError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Remove member */}
      <Modal
        danger
        open={Boolean(removeMember)}
        modalHeading="移除学生"
        primaryButtonText="移除"
        secondaryButtonText="取消"
        onRequestClose={() => setRemoveMember(null)}
        onRequestSubmit={handleRemoveMember}
        primaryButtonDisabled={removing}
      >
        <p className="courses-page__confirm-text">
          确定要将「{removeMember?.displayName}」移出该行政班吗？历史签到记录不受影响。
        </p>
        {removeError && (
          <InlineNotification kind="error" title="移除失败" subtitle={removeError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
