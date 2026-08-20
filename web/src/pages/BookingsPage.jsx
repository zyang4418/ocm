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
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ExportButton from '../components/ExportButton'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDate } from '../i18n/formatters'

const statusKind = {
  pending: 'blue',
  approved: 'green',
  rejected: 'red',
  cancelled: 'gray',
}

// Booking status enum values shown in the status filter dropdown.
const STATUS_FILTER_KEYS = ['pending', 'approved', 'rejected', 'cancelled']

function pad(n) {
  return String(n).padStart(2, '0')
}

// fmt produces an ISO YYYY-MM-DD string for <input type="date"> values, which
// are locale-independent (the display formatting is handled separately).
function fmt(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const emptyForm = { classroomId: '', date: '', periodStart: '', periodEnd: '', purpose: '' }

export default function BookingsPage() {
  const { t, i18n } = useTranslation('bookings')
  const { token, user: currentUser, can } = useAuth()
  const navigate = useNavigate()
  const canApprove = can('booking:approve')

  const headers = [
    { key: 'classroomName', header: t('field.classroom') },
    { key: 'date', header: t('field.date') },
    { key: 'period', header: t('field.period') },
    { key: 'purpose', header: t('field.purpose') },
    { key: 'status', header: t('field.status') },
    { key: 'displayName', header: t('field.bookedBy') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

  const periodLabel = (b) => {
    if (!b) return ''
    if (b.periodStart === b.periodEnd) return t('periodLabel.single', { period: b.periodStart })
    return t('periodLabel.range', { start: b.periodStart, end: b.periodEnd })
  }

  const [classrooms, setClassrooms] = useState([])

  const [filterClassroom, setFilterClassroom] = useState('')
  const [filterStatus, setFilterStatus] = useState('')
  const today = new Date()
  const [from, setFrom] = useState(fmt(today))
  const [to, setTo] = useState(fmt(new Date(today.getFullYear(), today.getMonth(), today.getDate() + 30)))

  const list = usePagedList({
    path: '/api/bookings',
    token,
    extraParams: { classroom_id: filterClassroom, status: filterStatus, from, to },
  })
  const { loading } = list
  // Action/export errors are separate from the list fetch (the hook owns its error).
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState(emptyForm)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)
  const [periods, setPeriods] = useState([])
  const [busy, setBusy] = useState([])

  const [cancelTarget, setCancelTarget] = useState(null)
  const [actingId, setActingId] = useState(null)

  useEffect(() => {
    apiFetch('/api/classrooms?page_size=500', { token })
      .then((data) => setClassrooms(Array.isArray(data?.items) ? data.items : []))
      .catch((err) => setActionError(err.message))
  }, [token])

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
      apiFetch(`/api/sessions?classroom_id=${cid}&from=${d}&to=${d}&page_size=500`, { token }),
      apiFetch(`/api/bookings?classroom_id=${cid}&from=${d}&to=${d}&page_size=500`, { token }),
    ])
      .then(([regime, sessData, booksData]) => {
        if (cancelled) return
        const ps = (regime?.periods || []).slice().sort((a, b) => a.periodIndex - b.periodIndex)
        setPeriods(ps)
        const sess = Array.isArray(sessData?.items) ? sessData.items : []
        const books = Array.isArray(booksData?.items) ? booksData.items : []
        const busySet = new Set()
        sess.forEach((s) => {
          for (let i = s.periodStart; i <= s.periodEnd; i++) busySet.add(i)
        })
        books.forEach((b) => {
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
        setCreateError(err.status === 404 ? t('validation.noSchedule') : err.message)
      })
    return () => {
      cancelled = true
    }
  }, [createForm.classroomId, createForm.date, token, t])

  const openCreate = () => {
    setCreateForm(emptyForm)
    setCreateError('')
    setPeriods([])
    setBusy([])
    setCreateOpen(true)
  }

  const handleCreate = async () => {
    if (!createForm.classroomId) return setCreateError(t('validation.selectClassroom'))
    if (!createForm.date) return setCreateError(t('validation.selectDate'))
    if (!createForm.periodStart || !createForm.periodEnd) return setCreateError(t('validation.selectPeriod'))
    if (Number(createForm.periodStart) > Number(createForm.periodEnd)) return setCreateError(t('validation.periodOrder'))
    if (!createForm.purpose.trim()) return setCreateError(t('validation.purposeRequired'))
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
      list.reload()
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
      setActionError('')
      await apiFetch(`/api/bookings/${id}/cancel`, { method: 'POST', token })
      setCancelTarget(null)
      list.reload()
    } catch (err) {
      setActionError(err.message)
    } finally {
      setActingId(null)
    }
  }

  const reviewBooking = async (id, decision) => {
    try {
      setActingId(id)
      setActionError('')
      await apiFetch(`/api/bookings/${id}/review`, { method: 'POST', token, body: { decision } })
      list.reload()
    } catch (err) {
      setActionError(err.message)
    } finally {
      setActingId(null)
    }
  }

  const rows = list.items.map((b) => ({
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

  // The busy-periods hint joins numbers with a locale-appropriate separator
  // (zh: "1、2、3", en: "1, 2, 3") via Intl.ListFormat narrow style.
  const busyText =
    busy.length > 0
      ? t('form.busyPrefix') + new Intl.ListFormat(i18n.language, { style: 'narrow' }).format(busy.map(String))
      : t('form.busyNone')

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
            title={t('error.action')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <div className="bookings-page__filters">
          <Select
            id="f-classroom"
            labelText={t('filter.classroom')}
            value={filterClassroom}
            onChange={(e) => setFilterClassroom(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text={t('filter.allClassrooms')} />
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={c.name} />
            ))}
          </Select>
          <Select
            id="f-status"
            labelText={t('filter.status')}
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text={t('filter.allStatuses')} />
            {STATUS_FILTER_KEYS.map((k) => (
              <SelectItem key={k} value={k} text={t('status.' + k, { ns: 'common' })} />
            ))}
          </Select>
          <TextInput
            id="f-from"
            type="date"
            labelText={t('filter.from')}
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            className="bookings-page__filter"
          />
          <TextInput
            id="f-to"
            type="date"
            labelText={t('filter.to')}
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
          }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('searchPlaceholder')} />
                  <ExportButton
                    path={`/api/bookings/export?${exportParams.toString()}`}
                    fallbackName="bookings.xlsx"
                    onError={setActionError}
                  />
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
                      const b = list.items.find((x) => String(x.id) === String(row.id))
                      const canCancel =
                        b &&
                        (b.status === 'pending' || b.status === 'approved') &&
                        (canApprove || Number(currentUser?.id) === b.userId)
                      const canReview = canApprove && b && b.status === 'pending'
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'status') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={statusKind[cell.value] ?? 'gray'} size="sm">
                                    {t('status.' + cell.value, { ns: 'common', defaultValue: cell.value })}
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
                                  {t('action.cancel')}
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
                                    {t('action.approve')}
                                  </Button>
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    onClick={() => reviewBooking(b.id, 'reject')}
                                    disabled={actingId === b.id}
                                  >
                                    {t('action.reject')}
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
        modalHeading={t('form.create')}
        primaryButtonText={t('form.createSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="courses-page__form">
          <Select
            id="c-classroom"
            labelText={t('form.classroom')}
            value={createForm.classroomId}
            onChange={(e) => setCreateForm({ ...createForm, classroomId: e.target.value })}
          >
            <SelectItem value="" text={t('form.selectClassroom')} />
            {classrooms
              .filter((c) => c.status === 'available')
              .map((c) => (
                <SelectItem key={c.id} value={String(c.id)} text={c.name} />
              ))}
          </Select>
          <TextInput
            id="c-date"
            type="date"
            labelText={t('form.date')}
            value={createForm.date}
            onChange={(e) => setCreateForm({ ...createForm, date: e.target.value })}
          />
          {periods.length > 0 && (
            <>
              <Select
                id="c-start"
                labelText={t('form.periodStart')}
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
                  <SelectItem
                    key={p.periodIndex}
                    value={String(p.periodIndex)}
                    text={t('periodOption', { period: p.periodIndex, start: p.startTime, end: p.endTime })}
                  />
                ))}
              </Select>
              <Select
                id="c-end"
                labelText={t('form.periodEnd')}
                value={createForm.periodEnd}
                onChange={(e) => setCreateForm((f) => ({ ...f, periodEnd: e.target.value }))}
              >
                {periods.map((p) => (
                  <SelectItem
                    key={p.periodIndex}
                    value={String(p.periodIndex)}
                    text={t('periodOption', { period: p.periodIndex, start: p.startTime, end: p.endTime })}
                  />
                ))}
              </Select>
              <p className="bookings-page__busy">{busyText}</p>
            </>
          )}
          <TextInput
            id="c-purpose"
            labelText={t('form.purpose')}
            placeholder={t('form.purposePlaceholder')}
            value={createForm.purpose}
            onChange={(e) => setCreateForm({ ...createForm, purpose: e.target.value })}
          />
          {createError && (
            <InlineNotification
              kind="error"
              title={t('error.submit')}
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
        modalHeading={t('cancelModal.title')}
        primaryButtonText={t('cancelModal.submit')}
        secondaryButtonText={t('cancelModal.secondary')}
        onRequestClose={() => setCancelTarget(null)}
        onRequestSubmit={cancelBooking}
        primaryButtonDisabled={actingId === cancelTarget?.id}
      >
        <p className="courses-page__confirm-text">
          {t('cancelModal.confirm', {
            classroom: cancelTarget?.classroomName,
            date: cancelTarget?.date,
            period: periodLabel(cancelTarget),
          })}
        </p>
      </Modal>
    </Grid>
  )
}
