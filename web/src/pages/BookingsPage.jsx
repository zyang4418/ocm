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
import { Add } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'

const statusLabel = {
  pending: '待审批',
  approved: '已通过',
  rejected: '已拒绝',
  cancelled: '已取消',
}

const statusKind = {
  pending: 'blue',
  approved: 'green',
  rejected: 'red',
  cancelled: 'gray',
}

const headers = [
  { key: 'classroomName', header: '教室' },
  { key: 'date', header: '日期' },
  { key: 'period', header: '节次' },
  { key: 'purpose', header: '用途' },
  { key: 'status', header: '状态' },
  { key: 'displayName', header: '预约人' },
  { key: 'createdAt', header: '创建时间' },
]

function pad(n) {
  return String(n).padStart(2, '0')
}

function fmt(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
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

function periodLabel(b) {
  if (!b) return ''
  if (b.periodStart === b.periodEnd) return `第 ${b.periodStart} 节`
  return `第 ${b.periodStart}–${b.periodEnd} 节`
}

const emptyForm = { classroomId: '', date: '', periodStart: '', periodEnd: '', purpose: '' }

export default function BookingsPage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const isAdmin = currentUser?.role === 'admin'

  const [classrooms, setClassrooms] = useState([])
  const [bookings, setBookings] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [filterClassroom, setFilterClassroom] = useState('')
  const [filterStatus, setFilterStatus] = useState('')
  const today = new Date()
  const [from, setFrom] = useState(fmt(today))
  const [to, setTo] = useState(fmt(new Date(today.getFullYear(), today.getMonth(), today.getDate() + 30)))

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState(emptyForm)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)
  const [periods, setPeriods] = useState([])
  const [busy, setBusy] = useState([])

  const [cancelTarget, setCancelTarget] = useState(null)
  const [actingId, setActingId] = useState(null)

  const fetchBookings = useCallback(async () => {
    try {
      setLoading(true)
      setError('')
      const params = new URLSearchParams()
      if (filterClassroom) params.set('classroom_id', filterClassroom)
      if (filterStatus) params.set('status', filterStatus)
      if (from) params.set('from', from)
      if (to) params.set('to', to)
      const data = await apiFetch(`/api/bookings?${params.toString()}`, { token })
      setBookings(Array.isArray(data) ? data : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [token, filterClassroom, filterStatus, from, to])

  useEffect(() => {
    apiFetch('/api/classrooms', { token })
      .then((cls) => setClassrooms(Array.isArray(cls) ? cls : []))
      .catch((err) => setError(err.message))
  }, [token])

  useEffect(() => {
    fetchBookings()
  }, [fetchBookings])

  // When the create modal's classroom or date changes, load the active regime
  // (for period options) and the day's sessions + bookings (for a busy hint).
  useEffect(() => {
    const cid = createForm.classroomId
    const d = createForm.date
    if (!cid || !d) {
      setPeriods([])
      setBusy([])
      return
    }
    let cancelled = false
    Promise.all([
      apiFetch(`/api/schedule/active?date=${d}`, { token }),
      apiFetch(`/api/sessions?classroom_id=${cid}&from=${d}&to=${d}`, { token }),
      apiFetch(`/api/bookings?classroom_id=${cid}&from=${d}&to=${d}`, { token }),
    ])
      .then(([regime, sess, books]) => {
        if (cancelled) return
        const ps = (regime?.periods || []).slice().sort((a, b) => a.periodIndex - b.periodIndex)
        setPeriods(ps)
        const busySet = new Set()
        ;(Array.isArray(sess) ? sess : []).forEach((s) => {
          for (let i = s.periodStart; i <= s.periodEnd; i++) busySet.add(i)
        })
        ;(Array.isArray(books) ? books : []).forEach((b) => {
          if (b.status === 'pending' || b.status === 'approved') {
            for (let i = b.periodStart; i <= b.periodEnd; i++) busySet.add(i)
          }
        })
        setBusy([...busySet].sort((a, b) => a - b))
        setCreateError('')
        setCreateForm((f) => {
          const first = ps[0]?.periodIndex
          const keepStart = f.periodStart && ps.some((p) => p.periodIndex === Number(f.periodStart))
          const keepEnd = f.periodEnd && ps.some((p) => p.periodIndex === Number(f.periodEnd))
          return {
            ...f,
            periodStart: keepStart ? f.periodStart : first ? String(first) : '',
            periodEnd: keepEnd ? f.periodEnd : first ? String(first) : '',
          }
        })
      })
      .catch((err) => {
        if (cancelled) return
        setPeriods([])
        setBusy([])
        setCreateError(err.status === 404 ? '该日期未配置作息制度，无法预约' : err.message)
      })
    return () => {
      cancelled = true
    }
  }, [createForm.classroomId, createForm.date, token])

  const openCreate = () => {
    setCreateForm(emptyForm)
    setCreateError('')
    setPeriods([])
    setBusy([])
    setCreateOpen(true)
  }

  const handleCreate = async () => {
    if (!createForm.classroomId) return setCreateError('请选择教室')
    if (!createForm.date) return setCreateError('请选择日期')
    if (!createForm.periodStart || !createForm.periodEnd) return setCreateError('请选择节次')
    if (Number(createForm.periodStart) > Number(createForm.periodEnd)) return setCreateError('起始节次不能大于结束节次')
    if (!createForm.purpose.trim()) return setCreateError('用途为必填项')
    try {
      setCreating(true)
      setCreateError('')
      await apiFetch('/api/bookings', {
        method: 'POST',
        token,
        body: {
          classroomId: Number(createForm.classroomId),
          date: createForm.date,
          periodStart: Number(createForm.periodStart),
          periodEnd: Number(createForm.periodEnd),
          purpose: createForm.purpose.trim(),
        },
      })
      setCreateOpen(false)
      await fetchBookings()
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const cancelBooking = async () => {
    const id = cancelTarget?.id
    if (!id) return
    try {
      setActingId(id)
      setError('')
      await apiFetch(`/api/bookings/${id}/cancel`, { method: 'POST', token })
      setCancelTarget(null)
      await fetchBookings()
    } catch (err) {
      setError(err.message)
    } finally {
      setActingId(null)
    }
  }

  const reviewBooking = async (id, decision) => {
    try {
      setActingId(id)
      setError('')
      await apiFetch(`/api/bookings/${id}/review`, { method: 'POST', token, body: { decision } })
      await fetchBookings()
    } catch (err) {
      setError(err.message)
    } finally {
      setActingId(null)
    }
  }

  const rows = bookings.map((b) => ({
    id: String(b.id),
    classroomName: b.classroomName,
    date: b.date,
    period: periodLabel(b),
    purpose: b.purpose,
    status: b.status,
    displayName: b.displayName,
    createdAt: b.createdAt,
  }))

  const colSpan = headers.length + 1

  // The export endpoint mirrors the list filters, so the downloaded file
  // reflects the currently-viewed (filtered) set of bookings.
  const exportParams = new URLSearchParams()
  if (filterClassroom) exportParams.set('classroom_id', filterClassroom)
  if (filterStatus) exportParams.set('status', filterStatus)
  if (from) exportParams.set('from', from)
  if (to) exportParams.set('to', to)

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
          <BreadcrumbItem isCurrentPage>教室预约</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">教室预约</h1>
        <p className="courses-page__subtitle">按节次预约教室，提交后由管理员审批；与课表共享冲突检测，避免重复占用。</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title="操作失败"
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <div className="bookings-page__filters">
          <Select
            id="f-classroom"
            labelText="教室"
            value={filterClassroom}
            onChange={(e) => setFilterClassroom(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text="全部教室" />
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={c.name} />
            ))}
          </Select>
          <Select
            id="f-status"
            labelText="状态"
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text="全部状态" />
            <SelectItem value="pending" text="待审批" />
            <SelectItem value="approved" text="已通过" />
            <SelectItem value="rejected" text="已拒绝" />
            <SelectItem value="cancelled" text="已取消" />
          </Select>
          <TextInput
            id="f-from"
            type="date"
            labelText="开始日期"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            className="bookings-page__filter"
          />
          <TextInput
            id="f-to"
            type="date"
            labelText="结束日期"
            value={to}
            onChange={(e) => setTo(e.target.value)}
            className="bookings-page__filter"
          />
        </div>
      </Column>

      <Column sm={4} md={8} lg={16}>
        <DataTable rows={rows} headers={headers}>
          {({
            rows,
            headers: tableHeaders,
            getTableProps,
            getHeaderProps,
            getRowProps,
            getToolbarProps,
            onInputChange,
          }) => (
            <TableContainer title="预约列表" description={`共 ${bookings.length} 条预约`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch onChange={onInputChange} placeholder="搜索预约" />
                  <ExportButton
                    path={`/api/bookings/export?${exportParams.toString()}`}
                    fallbackName="bookings.xlsx"
                    onError={setError}
                  />
                  <Button renderIcon={Add} size="sm" onClick={openCreate}>
                    新建预约
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
                      <TableCell colSpan={colSpan}>加载中…</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>
                        {bookings.length === 0 ? '暂无预约' : '未找到匹配的预约'}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const b = bookings.find((x) => String(x.id) === String(row.id))
                      const canCancel =
                        b &&
                        (b.status === 'pending' || b.status === 'approved') &&
                        (isAdmin || Number(currentUser?.id) === b.userId)
                      const canReview = isAdmin && b && b.status === 'pending'
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
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
                          <TableCell>
                            <div className="courses-page__actions">
                              {canCancel && (
                                <Button
                                  kind="ghost"
                                  size="sm"
                                  onClick={() => setCancelTarget(b)}
                                  disabled={actingId === b.id}
                                >
                                  取消
                                </Button>
                              )}
                              {canReview && (
                                <>
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    onClick={() => reviewBooking(b.id, 'approve')}
                                    disabled={actingId === b.id}
                                  >
                                    通过
                                  </Button>
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    onClick={() => reviewBooking(b.id, 'reject')}
                                    disabled={actingId === b.id}
                                  >
                                    拒绝
                                  </Button>
                                </>
                              )}
                              {!canCancel && !canReview && <span style={{ color: 'var(--cds-text-secondary)' }}>—</span>}
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
        modalHeading="新建预约"
        primaryButtonText="提交"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="courses-page__form">
          <Select
            id="c-classroom"
            labelText="教室"
            value={createForm.classroomId}
            onChange={(e) => setCreateForm({ ...createForm, classroomId: e.target.value })}
          >
            <SelectItem value="" text="请选择教室" />
            {classrooms
              .filter((c) => c.status === 'available')
              .map((c) => (
                <SelectItem key={c.id} value={String(c.id)} text={c.name} />
              ))}
          </Select>
          <TextInput
            id="c-date"
            type="date"
            labelText="日期"
            value={createForm.date}
            onChange={(e) => setCreateForm({ ...createForm, date: e.target.value })}
          />
          {periods.length > 0 && (
            <>
              <Select
                id="c-start"
                labelText="起始节次"
                value={createForm.periodStart}
                onChange={(e) => {
                  const start = e.target.value
                  setCreateForm((f) => ({
                    ...f,
                    periodStart: start,
                    periodEnd: Number(start) > Number(f.periodEnd) ? start : f.periodEnd,
                  }))
                }}
              >
                {periods.map((p) => (
                  <SelectItem key={p.periodIndex} value={String(p.periodIndex)} text={`第 ${p.periodIndex} 节（${p.startTime}-${p.endTime}）`} />
                ))}
              </Select>
              <Select
                id="c-end"
                labelText="结束节次"
                value={createForm.periodEnd}
                onChange={(e) => setCreateForm({ ...createForm, periodEnd: e.target.value })}
              >
                {periods.map((p) => (
                  <SelectItem key={p.periodIndex} value={String(p.periodIndex)} text={`第 ${p.periodIndex} 节（${p.startTime}-${p.endTime}）`} />
                ))}
              </Select>
              <p className="bookings-page__busy">
                {busy.length > 0 ? `当日已占用节次：${busy.join('、')}` : '当日暂无占用节次'}
              </p>
            </>
          )}
          <TextInput
            id="c-purpose"
            labelText="用途"
            placeholder="如 社团活动、会议"
            value={createForm.purpose}
            onChange={(e) => setCreateForm({ ...createForm, purpose: e.target.value })}
          />
          {createError && (
            <InlineNotification
              kind="error"
              title="提交失败"
              subtitle={createError}
              lowContrast
              hideCloseButton
            />
          )}
        </div>
      </Modal>

      {/* Cancel confirm */}
      <Modal
        danger
        open={Boolean(cancelTarget)}
        modalHeading="取消预约"
        primaryButtonText="确认取消"
        secondaryButtonText="返回"
        onRequestClose={() => setCancelTarget(null)}
        onRequestSubmit={cancelBooking}
        primaryButtonDisabled={actingId === cancelTarget?.id}
      >
        <p className="courses-page__confirm-text">
          确定要取消「{cancelTarget?.classroomName} · {cancelTarget?.date} · {periodLabel(cancelTarget)}」的预约吗？
        </p>
      </Modal>
    </Grid>
  )
}
