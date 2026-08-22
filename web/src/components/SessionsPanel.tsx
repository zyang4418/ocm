import { useEffect, useState } from 'react'
import {
  Button,
  DataTable,
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
  type DataTableHeader,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ExportButton from './ExportButton'
import ListPagination from './ListPagination'
import SessionFormModal from './SessionFormModal'
import usePagedList from '../hooks/usePagedList'
import type { Classroom, OfferingView, SessionView } from '../types/api'

export interface SessionsPanelProps {
  /** Whether the sessions tab is currently shown; drives lazy first render. */
  active: boolean
  /** Cross-tab prefill: offering id to filter by, consumed once written to state. */
  filterOfferingId?: string | null
  onFilterConsumed?: () => void
  /** Session mutations report the affected offering(s) so the owner refreshes its L2 caches. */
  onMutated?: (offeringId?: number, prevOfferingId?: number) => void
  offerings: OfferingView[]
  classrooms: Classroom[]
}

// SessionsPanel is the third tab of the course management page: a flat,
// full-featured view of all course sessions with combined filters (offering /
// classroom / date range), fuzzy search, pagination, xlsx export and CRUD via
// the shared SessionFormModal. Carbon keeps inactive TabPanels mounted but
// hidden, so the content is gated on first activation to avoid firing the
// sessions request on every page visit.
export default function SessionsPanel(props: SessionsPanelProps) {
  const { active } = props
  const [activated, setActivated] = useState(false)
  useEffect(() => {
    if (active) setActivated(true)
  }, [active])
  if (!activated) return null
  return <SessionsPanelContent {...props} />
}

function SessionsPanelContent({ filterOfferingId, onFilterConsumed, onMutated, offerings, classrooms }: SessionsPanelProps) {
  const { t, i18n } = useTranslation('courses')
  const { token, can } = useAuth()
  const canManage = can('course:manage')

  const headers: DataTableHeader[] = [
    { key: 'id', header: t('sessionField.id') },
    { key: 'date', header: t('sessionField.date') },
    { key: 'period', header: t('sessionField.period') },
    { key: 'courseName', header: t('sessionField.course') },
    { key: 'teachingClassName', header: t('sessionField.teachingClass') },
    { key: 'classNames', header: t('sessionField.classNames') },
    { key: 'teacher', header: t('sessionField.teacher') },
    { key: 'semester', header: t('sessionField.semester') },
    { key: 'classroomName', header: t('sessionField.classroom') },
    { key: 'note', header: t('sessionField.note') },
  ]

  const [offeringId, setOfferingId] = useState(filterOfferingId ?? '')
  const [classroomId, setClassroomId] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  const list = usePagedList<SessionView>({
    path: '/api/sessions',
    token,
    extraParams: { offering_id: offeringId, classroom_id: classroomId, from, to },
  })
  // Action/delete errors are separate from the list fetch (the hook owns its error).
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError

  // Cross-tab prefill: write the incoming offering filter once, then let the
  // owner clear the pending value so a later reload doesn't re-apply it.
  useEffect(() => {
    if (filterOfferingId) {
      setOfferingId(filterOfferingId)
      onFilterConsumed?.()
    }
  }, [filterOfferingId, onFilterConsumed])

  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<SessionView | null>(null)
  const [delTarget, setDelTarget] = useState<SessionView | null>(null)
  const [delError, setDelError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const periodLabel = (s: Pick<SessionView, 'periodStart' | 'periodEnd'>) =>
    s.periodStart === s.periodEnd
      ? t('sessionPeriod.single', { period: s.periodStart })
      : t('sessionPeriod.range', { start: s.periodStart, end: s.periodEnd })

  const openAdd = () => {
    setEditTarget(null)
    setModalOpen(true)
  }

  const openEdit = (s: SessionView) => {
    setEditTarget(s)
    setModalOpen(true)
  }

  // The export endpoint honors the same filters as the list (but not q), so the
  // downloaded file covers the full filtered range - mirroring bookings export.
  const exportParams = new URLSearchParams()
  if (offeringId) exportParams.set('offering_id', offeringId)
  if (classroomId) exportParams.set('classroom_id', classroomId)
  if (from) exportParams.set('from', from)
  if (to) exportParams.set('to', to)

  const handleDelete = async () => {
    if (!delTarget) return
    const { id, offeringId: oid } = delTarget
    try {
      setDeleting(true)
      setDelError('')
      await apiFetch(`/api/sessions/${id}`, { method: 'DELETE', token })
      setDelTarget(null)
      list.reload()
      onMutated?.(oid)
    } catch (err) {
      setDelError((err as Error).message)
    } finally {
      setDeleting(false)
    }
  }

  const rows = list.items.map((s) => ({
    id: String(s.id),
    date: s.date,
    period: periodLabel(s),
    courseName: s.courseName,
    teachingClassName: s.teachingClassName,
    classNames: s.classNames,
    teacher: s.teacher,
    semester: s.semester,
    classroomName: s.classroomName,
    note: s.note,
  }))

  const colSpan = headers.length + (canManage ? 1 : 0)

  // classNames column joins admin-class names with a locale-appropriate
  // separator via Intl.ListFormat narrow style (same as the offerings table).
  const listFmt = new Intl.ListFormat(i18n.language, { style: 'narrow' })

  return (
    <div className="sessions-panel">
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

      <div className="sessions-panel__filters">
        <Select
          id="sessions-filter-offering"
          labelText={t('sessionFilter.offering')}
          value={offeringId}
          onChange={(e) => setOfferingId(e.target.value)}
          className="sessions-panel__filter"
        >
          <SelectItem value="" text={t('sessionFilter.allOfferings')} />
          {offerings.map((o) => (
            <SelectItem
              key={o.id}
              value={String(o.id)}
              text={[o.catalogName, o.teachingClassName, o.teacher].filter(Boolean).join(' / ')}
            />
          ))}
        </Select>
        <Select
          id="sessions-filter-classroom"
          labelText={t('sessionFilter.classroom')}
          value={classroomId}
          onChange={(e) => setClassroomId(e.target.value)}
          className="sessions-panel__filter"
        >
          <SelectItem value="" text={t('sessionFilter.allClassrooms')} />
          {classrooms.map((c) => (
            <SelectItem key={c.id} value={String(c.id)} text={c.name} />
          ))}
        </Select>
        <TextInput
          id="sessions-filter-from"
          type="date"
          labelText={t('sessionFilter.from')}
          value={from}
          onChange={(e) => setFrom(e.target.value)}
          className="sessions-panel__filter"
        />
        <TextInput
          id="sessions-filter-to"
          type="date"
          labelText={t('sessionFilter.to')}
          value={to}
          onChange={(e) => setTo(e.target.value)}
          className="sessions-panel__filter"
        />
      </div>

      <DataTable rows={rows} headers={headers}>
        {({ rows: tableRows, headers: th, getTableProps, getHeaderProps, getToolbarProps }) => (
          <TableContainer title={t('table.sessions.title')} description={t('table.sessions.description', { count: list.total })}>
            <TableToolbar {...getToolbarProps()}>
              <TableToolbarContent>
                <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('searchPlaceholder')} />
                <ExportButton
                  path={`/api/sessions/export?${exportParams.toString()}`}
                  fallbackName="sessions.xlsx"
                  onError={setActionError}
                />
                {canManage && (
                  <Button renderIcon={Add} size="sm" onClick={openAdd}>
                    {t('sessionsExpanded.add')}
                  </Button>
                )}
              </TableToolbarContent>
            </TableToolbar>
            <Table {...getTableProps()}>
              <TableHead>
                <TableRow>
                  {th.map((header) => (
                    <TableHeader {...getHeaderProps({ header })}>{header.header}</TableHeader>
                  ))}
                  {canManage && <TableHeader>{t('field.actions')}</TableHeader>}
                </TableRow>
              </TableHead>
              <TableBody>
                {list.loading ? (
                  <TableRow>
                    <TableCell colSpan={colSpan}>{t('empty.loading')}</TableCell>
                  </TableRow>
                ) : tableRows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={colSpan}>
                      {list.q ? t('empty.noResults', { ns: 'common' }) : t('empty.noData', { ns: 'common' })}
                    </TableCell>
                  </TableRow>
                ) : (
                  tableRows.map((row) => {
                    const s = list.items.find((x) => String(x.id) === String(row.id))
                    if (!s) return null
                    return (
                      <TableRow key={row.id}>
                        {row.cells.map((cell) => {
                          if (cell.info.header === 'classNames') {
                            const value = cell.value as string[]
                            return (
                              <TableCell key={cell.id}>
                                {Array.isArray(value) && value.length ? listFmt.format(value) : '-'}
                              </TableCell>
                            )
                          }
                          return <TableCell key={cell.id}>{(cell.value as string) || '-'}</TableCell>
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
                                onClick={() => openEdit(s)}
                              />
                              <Button
                                kind="ghost"
                                size="sm"
                                hasIconOnly
                                renderIcon={TrashCan}
                                iconDescription={t('action.delete', { ns: 'common' })}
                                onClick={() => {
                                  setDelError('')
                                  setDelTarget(s)
                                }}
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

      <SessionFormModal
        open={modalOpen}
        target={editTarget}
        offerings={offerings}
        classrooms={classrooms}
        onSuccess={(offeringId_, prevOfferingId_) => {
          list.reload()
          onMutated?.(offeringId_, prevOfferingId_)
        }}
        onClose={() => setModalOpen(false)}
      />

      <Modal
        danger
        open={Boolean(delTarget)}
        modalHeading={t('modal.delete')}
        primaryButtonText={t('modal.deleteSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setDelTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="courses-page__confirm-text">
          {t('sessionModal.deleteConfirm', {
            date: delTarget?.date ?? '',
            period: delTarget ? periodLabel(delTarget) : '',
            classroom: delTarget?.classroomName ?? '',
          })}
        </p>
        {delError && <InlineNotification kind="error" title={t('error.delete')} subtitle={delError} lowContrast hideCloseButton />}
      </Modal>
    </div>
  )
}
