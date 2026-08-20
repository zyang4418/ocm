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
} from '@carbon/react'
import { QRCodeSVG } from 'qrcode.react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'
import { CheckinStatusTag, STATUS_KEYS, StatusTag, formatDateTime } from './attendanceUi.jsx'

export default function AttendanceDetailPage() {
  const { t } = useTranslation('attendance')
  const { id } = useParams()
  const navigate = useNavigate()
  const { token, can } = useAuth()
  const canManage = can('attendance:manage')

  const headers = [
    { key: 'displayName', header: t('detail.field.name') },
    { key: 'studentNo', header: t('detail.field.studentNo') },
    { key: 'adminClass', header: t('detail.field.adminClass') },
    { key: 'status', header: t('detail.field.status') },
    { key: 'checkedAt', header: t('detail.field.checkedAt') },
    { key: 'inRoster', header: t('detail.field.inRoster') },
  ]

  const [checkin, setCheckin] = useState(null)
  const [detailError, setDetailError] = useState('')
  const [now, setNow] = useState(() => new Date())

  const [statusFilter, setStatusFilter] = useState('')
  const list = usePagedList({ path: `/api/checkins/${id}/records`, token, extraParams: { status: statusFilter } })
  const { loading } = list

  // Carbon DataTable requires a unique `id` on each row; checkin records are
  // keyed by userId (no `id` field from the API), so derive it here. Without
  // this, row.id is undefined, the find() below misses, and r.displayName
  // throws — white-screening the page once any record renders.
  const tableRows = list.items.map((r) => ({ ...r, id: r.userId }))

  const [editTarget, setEditTarget] = useState(null)
  const [editStatus, setEditStatus] = useState('')
  const [editError, setEditError] = useState('')
  const [saving, setSaving] = useState(false)

  const [actionError, setActionError] = useState('')

  const load = async () => {
    try {
      const v = await apiFetch(`/api/checkins/${id}`, { token })
      setCheckin(v)
      setDetailError('')
      if (v.status === 'closed') list.reload()
    } catch (err) {
      setDetailError(err.message)
    }
  }

  useEffect(() => {
    load()
    const poll = setInterval(load, 5000)
    return () => clearInterval(poll)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, token])

  // 1s ticker drives the countdown; cleared once the checkin closes or has no
  // expiry.
  useEffect(() => {
    if (!checkin || checkin.status !== 'active' || !checkin.expiresAt) return
    const tick = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(tick)
  }, [checkin])

  const remainingSeconds = () => {
    if (!checkin || !checkin.expiresAt) return null
    const s = Math.floor((new Date(checkin.expiresAt).getTime() - now.getTime()) / 1000)
    return Math.max(0, s)
  }

  const openEdit = (r) => {
    setEditTarget(r)
    setEditStatus(r.status)
    setEditError('')
  }

  const handleSave = async () => {
    try {
      setSaving(true)
      setEditError('')
      await apiFetch(`/api/checkins/${id}/records/${editTarget.userId}`, {
        method: 'PUT',
        token,
        body: { status: editStatus },
      })
      setEditTarget(null)
      list.reload()
      load()
    } catch (err) {
      setEditError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleClose = async () => {
    try {
      setActionError('')
      await apiFetch(`/api/checkins/${id}/close`, { method: 'POST', token })
      load()
    } catch (err) {
      setActionError(err.message)
    }
  }

  const counts = checkin?.counts
  const statItems = [
    { label: t('stat.expected'), value: counts?.expected ?? '-' },
    { label: t('status.present', { ns: 'common' }), value: counts?.present ?? '-' },
    { label: t('status.late', { ns: 'common' }), value: counts?.late ?? '-' },
    { label: t('status.absent', { ns: 'common' }), value: counts?.absent ?? '-' },
    { label: t('status.leave', { ns: 'common' }), value: counts?.leave ?? '-' },
  ]
  const expired = checkin?.status === 'active' && remainingSeconds() === 0 && checkin.expiresAt

  const remainingText = () => {
    const s = remainingSeconds() ?? 0
    const mm = String(Math.floor(s / 60)).padStart(2, '0')
    const ss = String(s % 60).padStart(2, '0')
    return `${mm}:${ss}`
  }

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
          <BreadcrumbItem
            href="/attendance"
            onClick={(e) => {
              e.preventDefault()
              navigate('/attendance')
            }}
          >
            {t('breadcrumb.attendance')}
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>{checkin?.title ?? t('detail.titleFallback')}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">{checkin?.title ?? t('detail.titleFallback')}</h1>
        <p className="courses-page__subtitle">
          {checkin && (
            <>
              {t('detail.codePrefix')} {checkin.code} ·{' '}
              {checkin.courseName
                ? t('detail.courseAndClass', { course: checkin.courseName, teachingClass: checkin.teachingClassName })
                : t('detail.standalone')}
              {checkin.sessionText ? ` · ${checkin.sessionText}` : ''} ·{' '}
              {t('detail.startedAt', { time: formatDateTime(checkin.startsAt) })}
            </>
          )}
        </p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {detailError && (
          <InlineNotification
            kind="error"
            title={t('detail.error.load')}
            subtitle={detailError}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
        {actionError && (
          <InlineNotification
            kind="error"
            title={t('detail.error.action')}
            subtitle={actionError}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        {checkin && (
          <div className="attendance-detail">
            {/* QR + status panel */}
            <div className="attendance-detail__panel">
              <div className="attendance-detail__qr">
                <QRCodeSVG value={checkin.code} size={280} />
                <div className="attendance-detail__code">{checkin.code}</div>
              </div>
              <div className="attendance-detail__side">
                <CheckinStatusTag status={checkin.status} />
                {checkin.status === 'active' && (
                  <p className="attendance-detail__countdown">
                    {expired
                      ? t('detail.elapsed')
                      : checkin.expiresAt
                        ? t('detail.remaining', { time: remainingText() })
                        : t('detail.manualMode')}
                  </p>
                )}
                {checkin.lateMinutes > 0 && <p>{t('detail.lateThreshold', { minutes: checkin.lateMinutes })}</p>}
                {canManage && checkin.status === 'active' && (
                  <Button kind="danger--ghost" size="sm" onClick={handleClose}>
                    {t('detail.closeBtn')}
                  </Button>
                )}
                <p className="attendance-detail__hint">{t('detail.hint')}</p>
              </div>
            </div>

            {/* Live counts */}
            <div className="attendance-detail__stats">
              {statItems.map((s) => (
                <div key={s.label} className="attendance-detail__stat">
                  <div className="attendance-detail__stat-value">{s.value}</div>
                  <div className="attendance-detail__stat-label">{s.label}</div>
                </div>
              ))}
            </div>
            {checkin.counts.expected === 0 && checkin.offeringId > 0 && (
              <InlineNotification
                kind="warning"
                title={t('detail.warning.zeroExpectedTitle')}
                subtitle={t('detail.warning.noRosterSubtitle')}
                lowContrast
                hideCloseButton
                className="courses-page__notice"
              />
            )}
          </div>
        )}

        {list.error && (
          <InlineNotification
            kind="error"
            title={t('detail.error.recordLoad')}
            subtitle={list.error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <DataTable rows={tableRows} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title={t('detail.table.title')} description={t('detail.table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('detail.searchPlaceholder')} />
                  <Select
                    id="rec-status"
                    labelText={t('detail.field.status')}
                    hideLabel
                    value={statusFilter}
                    onChange={(e) => setStatusFilter(e.target.value)}
                  >
                    <SelectItem value="" text={t('list.filter.allStatuses')} />
                    <SelectItem value="present" text={t('status.present', { ns: 'common' })} />
                    <SelectItem value="late" text={t('status.late', { ns: 'common' })} />
                    <SelectItem value="absent" text={t('status.absent', { ns: 'common' })} />
                    <SelectItem value="leave" text={t('status.leave', { ns: 'common' })} />
                  </Select>
                  <ExportButton path={`/api/checkins/${id}/export`} fallbackName={`checkin-${id}.xlsx`} onError={setActionError} />
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
                    {canManage && <TableHeader>{t('detail.field.actions')}</TableHeader>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>{t('detail.empty.loading')}</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>
                        {statusFilter ? t('detail.empty.filtered') : t('detail.empty.none')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const r = list.items.find((x) => String(x.userId) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          <TableCell>{r.displayName}</TableCell>
                          <TableCell>{r.studentNo || '-'}</TableCell>
                          <TableCell>{r.adminClass || '-'}</TableCell>
                          <TableCell>
                            <StatusTag status={r.status} />
                          </TableCell>
                          <TableCell>{r.checkedAt ? formatDateTime(r.checkedAt) : '-'}</TableCell>
                          <TableCell>{r.inRoster ? t('detail.inRosterYes') : t('detail.inRosterNo')}</TableCell>
                          {canManage && (
                            <TableCell>
                              <Button kind="ghost" size="sm" onClick={() => openEdit(r)}>
                                {t('detail.action.modify')}
                              </Button>
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

      {/* Modify record */}
      <Modal
        open={Boolean(editTarget)}
        modalHeading={t('detail.modal.edit', { name: editTarget?.displayName ?? '' })}
        primaryButtonText={t('detail.modal.editSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleSave}
        primaryButtonDisabled={saving}
      >
        <div className="courses-page__form">
          <Select
            id="edit-status"
            labelText={t('detail.modal.statusLabel')}
            value={editStatus}
            onChange={(e) => setEditStatus(e.target.value)}
          >
            {STATUS_KEYS.map((value) => (
              <SelectItem key={value} value={value} text={t('status.' + value, { ns: 'common' })} />
            ))}
          </Select>
          <p className="courses-page__subtitle">{t('detail.modal.hint')}</p>
          {editError && (
            <InlineNotification kind="error" title={t('detail.error.save')} subtitle={editError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>
    </Grid>
  )
}
