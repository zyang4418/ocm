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
  TextInput,
} from '@carbon/react'
import { Add } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { CheckinStatusTag, formatDateTime } from './attendanceUi'

export default function AttendancePage() {
  const { t } = useTranslation('attendance')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('attendance:manage')

  const headers = [
    { key: 'title', header: t('list.field.title') },
    { key: 'offering', header: t('list.field.offering') },
    { key: 'sessionText', header: t('list.field.sessionText') },
    { key: 'status', header: t('list.field.status') },
    { key: 'counts', header: t('list.field.counts') },
    { key: 'startsAt', header: t('list.field.startsAt') },
  ]

  const [statusFilter, setStatusFilter] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const list = usePagedList({
    path: '/api/checkins',
    token,
    extraParams: { status: statusFilter, from, to },
  })
  const { loading } = list

  const [createOpen, setCreateOpen] = useState(false)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)
  const [offerings, setOfferings] = useState([])
  const [pickOffering, setPickOffering] = useState(null)
  const [sessions, setSessions] = useState([])
  const [pickSession, setPickSession] = useState(null)
  const [form, setForm] = useState({ title: '', lateMinutes: '0', durationMinute: '' })

  const [closeError, setCloseError] = useState('')

  const loadOfferings = async () => {
    try {
      const data = await apiFetch('/api/offerings?page_size=500', { token })
      setOfferings(
        ((data && data.items) || []).map((o) => ({
          id: String(o.id),
          text: t('list.offeringOption', { catalogName: o.catalogName, teachingClassName: o.teachingClassName, semester: o.semester }),
        }))
      )
    } catch {
      setOfferings([])
    }
  }

  const openCreate = () => {
    setCreateOpen(true)
    setCreateError('')
    setForm({ title: '', lateMinutes: '0', durationMinute: '' })
    setPickOffering(null)
    setPickSession(null)
    setSessions([])
    loadOfferings()
  }

  const handlePickOffering = (e) => {
    setPickOffering(e.selectedItem ?? null)
    setPickSession(null)
    setSessions([])
    if (e.selectedItem) loadSessions(e.selectedItem.id)
  }

  const loadSessions = async (offeringId) => {
    try {
      const data = await apiFetch(`/api/sessions?offering_id=${offeringId}&page_size=500`, { token })
      setSessions(
        ((data && data.items) || []).map((s) => ({
          id: String(s.id),
          text: t('list.sessionOption', { date: s.date, start: s.periodStart, end: s.periodEnd, classroom: s.classroomName }),
        }))
      )
    } catch {
      setSessions([])
    }
  }

  const handleCreate = async () => {
    const late = parseInt(form.lateMinutes || '0', 10)
    const duration = parseInt(form.durationMinute || '0', 10)
    if (Number.isNaN(late) || late < 0) {
      setCreateError(t('list.validation.lateNonNeg'))
      return
    }
    if (Number.isNaN(duration) || duration < 0) {
      setCreateError(t('list.validation.durationNonNeg'))
      return
    }
    if (!pickOffering && !form.title.trim()) {
      setCreateError(t('list.validation.titleRequired'))
      return
    }
    try {
      setCreating(true)
      setCreateError('')
      const v = await apiFetch('/api/checkins', {
        method: 'POST',
        token,
        body: {
          offeringId: pickOffering ? Number(pickOffering.id) : 0,
          sessionId: pickSession ? Number(pickSession.id) : 0,
          title: form.title.trim(),
          lateMinutes: late,
          durationMinute: duration,
        },
      })
      setCreateOpen(false)
      navigate(`/attendance/${v.id}`)
    } catch (err) {
      setCreateError(err.message)
    } finally {
      setCreating(false)
    }
  }

  const handleClose = async (c) => {
    try {
      setCloseError('')
      await apiFetch(`/api/checkins/${c.id}/close`, { method: 'POST', token })
      list.reload()
    } catch (err) {
      setCloseError(err.message)
    }
  }

  const countsText = (c) =>
    `${c.counts.expected} / ${c.counts.present} / ${c.counts.late} / ${c.counts.absent} / ${c.counts.leave}`

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
          <BreadcrumbItem isCurrentPage>{t('breadcrumb.attendance')}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">{t('list.title')}</h1>
        <p className="courses-page__subtitle">{t('list.subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {list.error && (
          <InlineNotification
            kind="error"
            title={t('list.error.load')}
            subtitle={list.error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
        {closeError && (
          <InlineNotification
            kind="error"
            title={t('list.error.close')}
            subtitle={closeError}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <DataTable rows={list.items} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title={t('list.table.title')} description={t('list.table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('list.searchPlaceholder')} />
                  <Select
                    id="att-status"
                    labelText={t('list.filter.status')}
                    hideLabel
                    value={statusFilter}
                    onChange={(e) => setStatusFilter(e.target.value)}
                  >
                    <SelectItem value="" text={t('list.filter.allStatuses')} />
                    <SelectItem value="active" text={t('list.filter.active')} />
                    <SelectItem value="closed" text={t('list.filter.closed')} />
                  </Select>
                  <TextInput id="att-from" labelText={t('list.filter.from')} hideLabel type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
                  <TextInput id="att-to" labelText={t('list.filter.to')} hideLabel type="date" value={to} onChange={(e) => setTo(e.target.value)} />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={openCreate}>
                      {t('list.addButton')}
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
                    <TableHeader>{t('list.field.actions')}</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>{t('list.empty.loading')}</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>
                        {list.q || statusFilter || from || to ? t('list.empty.search') : t('list.empty.none')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const c = list.items.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          <TableCell>{c.title}</TableCell>
                          <TableCell>
                            {c.courseName ? `${c.courseName} / ${c.teachingClassName}` : t('list.noOffering')}
                          </TableCell>
                          <TableCell>{c.sessionText || '-'}</TableCell>
                          <TableCell>
                            <CheckinStatusTag status={c.status} />
                          </TableCell>
                          <TableCell>{countsText(c)}</TableCell>
                          <TableCell>{formatDateTime(c.startsAt)}</TableCell>
                          <TableCell>
                            <div className="courses-page__actions">
                              <Button kind="ghost" size="sm" onClick={() => navigate(`/attendance/${c.id}`)}>
                                {t('list.action.detail')}
                              </Button>
                              {canManage && c.status === 'active' && (
                                <Button kind="ghost" size="sm" onClick={() => handleClose(c)}>
                                  {t('list.action.close')}
                                </Button>
                              )}
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
        modalHeading={t('list.modal.create')}
        primaryButtonText={t('list.modal.createSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
        size="lg"
      >
        <div className="courses-page__form">
          <ComboBox
            id="att-offering"
            titleText={t('list.form.offering')}
            placeholder={t('list.form.offeringPlaceholder')}
            items={offerings}
            itemToString={(item) => (item ? item.text : '')}
            selectedItem={pickOffering}
            onChange={handlePickOffering}
            shouldFilterItem={() => true}
            helperText={t('list.form.offeringHelper')}
          />
          {pickOffering && (
            <ComboBox
              id="att-session"
              titleText={t('list.form.session')}
              placeholder={t('list.form.sessionPlaceholder')}
              items={sessions}
              itemToString={(item) => (item ? item.text : '')}
              selectedItem={pickSession}
              onChange={(e) => setPickSession(e.selectedItem ?? null)}
              shouldFilterItem={() => true}
              helperText={t('list.form.sessionHelper')}
            />
          )}
          <TextInput
            id="att-title"
            labelText={t('list.form.title')}
            placeholder={t('list.form.titlePlaceholder')}
            value={form.title}
            onChange={(e) => setForm({ ...form, title: e.target.value })}
          />
          <TextInput
            id="att-late"
            labelText={t('list.form.lateMinutes')}
            type="number"
            value={form.lateMinutes}
            onChange={(e) => setForm({ ...form, lateMinutes: e.target.value })}
          />
          <TextInput
            id="att-duration"
            labelText={t('list.form.duration')}
            type="number"
            placeholder={t('list.form.durationPlaceholder')}
            value={form.durationMinute}
            onChange={(e) => setForm({ ...form, durationMinute: e.target.value })}
          />
          {createError && (
            <InlineNotification kind="error" title={t('list.error.create')} subtitle={createError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>
    </Grid>
  )
}
