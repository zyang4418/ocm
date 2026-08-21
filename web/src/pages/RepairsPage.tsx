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
  TextArea,
  type DataTableHeader,
  type TagProps,
} from '@carbon/react'
import { Add, CheckmarkOutline } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDate } from '../i18n/formatters'
import type { Classroom, Paged, RepairInput, RepairUpdateInput, RepairView } from '../types/api'

const STATUS_KIND: Record<string, TagProps<'div'>['type']> = {
  open: 'red',
  processing: 'blue',
  completed: 'purple',
  confirmed: 'green',
}

function classroomLabel(r: Pick<RepairView, 'building' | 'classroomName'>) {
  return r.building ? `${r.building} ${r.classroomName}` : r.classroomName
}

// The create body intentionally omits `images`: the original JS version did
// too, and the backend stores an absent json.RawMessage as NULL (vs '[]'), so
// keeping the key absent preserves the wire behavior.
type RepairCreateInput = Omit<RepairInput, 'images'>

// Process dialog state: the ticket under transition and its next status.
interface ProcessTarget extends RepairView {
  nextStatus: 'processing' | 'completed'
}

export default function RepairsPage() {
  const { t } = useTranslation('repairs')
  const { token, user: currentUser, can } = useAuth()
  const navigate = useNavigate()
  const canSubmit = can('repair:create')
  const canAssign = can('repair:assign')

  const headers: DataTableHeader[] = [
    { key: 'classroom', header: t('field.classroom') },
    { key: 'description', header: t('field.description') },
    { key: 'creatorName', header: t('field.creatorName') },
    { key: 'assigneeName', header: t('field.assigneeName') },
    { key: 'status', header: t('field.status') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

  const [filterStatus, setFilterStatus] = useState('')
  const list = usePagedList<RepairView>({
    path: '/api/repairs',
    token,
    extraParams: { status: filterStatus },
  })
  const { loading } = list
  // Action errors are separate from the list fetch (the hook owns its error).
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError

  const [classrooms, setClassrooms] = useState<Classroom[]>([])

  // Submit form.
  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState({ classroomId: '', description: '' })
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  // Process (start / complete) dialog.
  const [processTarget, setProcessTarget] = useState<ProcessTarget | null>(null)
  const [processRemark, setProcessRemark] = useState('')
  const [processError, setProcessError] = useState('')
  const [processing, setProcessing] = useState(false)

  const [actingId, setActingId] = useState<number | null>(null)

  useEffect(() => {
    if (!canSubmit) return
    apiFetch<Paged<Classroom>>('/api/classrooms?page_size=500', { token })
      .then((data) => setClassrooms(Array.isArray(data?.items) ? data.items : []))
      .catch(() => setClassrooms([]))
  }, [token, canSubmit])

  const openCreate = () => {
    setForm({ classroomId: '', description: '' })
    setFormError('')
    setFormOpen(true)
  }

  const handleCreate = async () => {
    if (!form.classroomId) return setFormError(t('validation.classroomRequired'))
    if (!form.description.trim()) return setFormError(t('validation.descriptionRequired'))
    try {
      setSaving(true)
      setFormError('')
      const body: RepairCreateInput = {
        classroomId: Number(form.classroomId),
        description: form.description.trim(),
      }
      await apiFetch('/api/repairs', { method: 'POST', token, body })
      setFormOpen(false)
      list.reload()
    } catch (err) {
      setFormError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const openProcess = (row: RepairView, nextStatus: ProcessTarget['nextStatus']) => {
    setProcessTarget({ ...row, nextStatus })
    setProcessRemark('')
    setProcessError('')
  }

  const handleProcess = async () => {
    if (!processTarget) return
    try {
      setProcessing(true)
      setProcessError('')
      const body: RepairUpdateInput = { status: processTarget.nextStatus, remark: processRemark.trim() }
      await apiFetch(`/api/repairs/${processTarget.id}`, { method: 'PUT', token, body })
      setProcessTarget(null)
      list.reload()
    } catch (err) {
      setProcessError((err as Error).message)
    } finally {
      setProcessing(false)
    }
  }

  const handleConfirm = async (row: RepairView) => {
    try {
      setActingId(row.id)
      setActionError('')
      await apiFetch(`/api/repairs/${row.id}/confirm`, { method: 'POST', token })
      list.reload()
    } catch (err) {
      setActionError((err as Error).message)
    } finally {
      setActingId(null)
    }
  }

  const rows = list.items.map((r) => ({
    id: String(r.id),
    classroom: classroomLabel(r),
    description: r.description,
    creatorName: r.creatorName,
    assigneeName: r.assigneeName || '-',
    status: r.status,
    createdAt: formatDate(r.createdAt),
  }))

  const colSpan = headers.length + 1

  return (
    <Grid fullWidth className="classrooms-page">
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
        <h1 className="classrooms-page__heading">{t('title')}</h1>
        <p className="classrooms-page__subtitle">{t('subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title={t('error.action')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="classrooms-page__notice"
          />
        )}

        <div className="bookings-page__filters">
          <Select
            id="f-status"
            labelText={t('filter.status')}
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text={t('filter.allStatuses')} />
            <SelectItem value="open" text={t('status.open')} />
            <SelectItem value="processing" text={t('status.processing')} />
            <SelectItem value="completed" text={t('status.completed')} />
            <SelectItem value="confirmed" text={t('status.confirmed')} />
          </Select>
        </div>
      </Column>

      <Column sm={4} md={8} lg={16}>
        <DataTable rows={rows} headers={headers}>
          {({ rows: tableRows, headers: renderedHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('table.searchPlaceholder')} />
                  {canSubmit && (
                    <Button renderIcon={Add} size="sm" onClick={openCreate}>
                      {t('table.addButton')}
                    </Button>
                  )}
                </TableToolbarContent>
              </TableToolbar>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {renderedHeaders.map((header) => (
                      <TableHeader {...getHeaderProps({ header })}>
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
                  ) : tableRows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>{list.q ? t('empty.search') : t('empty.none')}</TableCell>
                    </TableRow>
                  ) : (
                    tableRows.map((row) => {
                      const r = list.items.find((x) => String(x.id) === String(row.id))
                      const isCreator = r && Number(currentUser?.id) === r.creatorId
                      const canStart = canAssign && r?.status === 'open'
                      const canFinish = canAssign && r?.status === 'processing'
                      const canConfirm = isCreator && r?.status === 'completed'
                      return (
                        <TableRow {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'status') {
                              const value = cell.value as string
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={STATUS_KIND[value] ?? 'gray'} size="sm">
                                    {t('status.' + value, { defaultValue: value })}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            return <TableCell key={cell.id}>{cell.value as string}</TableCell>
                          })}
                          <TableCell>
                            <div className="classrooms-page__actions">
                              {r && canStart && (
                                <Button kind="ghost" size="sm" onClick={() => openProcess(r, 'processing')} disabled={actingId === r.id}>
                                  {t('action.start')}
                                </Button>
                              )}
                              {r && canFinish && (
                                <Button kind="ghost" size="sm" onClick={() => openProcess(r, 'completed')} disabled={actingId === r.id}>
                                  {t('action.finish')}
                                </Button>
                              )}
                              {r && canConfirm && (
                                <Button kind="ghost" size="sm" renderIcon={CheckmarkOutline} onClick={() => handleConfirm(r)} disabled={actingId === r.id}>
                                  {t('action.confirm')}
                                </Button>
                              )}
                              {!canStart && !canFinish && !canConfirm && (
                                <span style={{ color: 'var(--cds-text-secondary)' }}>—</span>
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

      {/* Submit form */}
      <Modal
        open={formOpen}
        modalHeading={t('modal.create')}
        primaryButtonText={t('modal.createSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setFormOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={saving}
      >
        <div className="classrooms-page__form">
          <Select
            id="r-classroom"
            labelText={t('form.classroom')}
            value={form.classroomId}
            onChange={(e) => setForm({ ...form, classroomId: e.target.value })}
          >
            <SelectItem value="" text={t('form.classroomPlaceholder')} />
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={classroomLabel({ building: c.building, classroomName: c.name })} />
            ))}
          </Select>
          <TextArea
            id="r-description"
            labelText={t('form.description')}
            placeholder={t('form.descriptionPlaceholder')}
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
            rows={4}
          />
          {formError && (
            <InlineNotification kind="error" title={t('error.submit')} subtitle={formError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Process dialog */}
      <Modal
        open={Boolean(processTarget)}
        modalHeading={processTarget?.nextStatus === 'processing' ? t('modal.processStart') : t('modal.processFinish')}
        primaryButtonText={t('modal.processSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setProcessTarget(null)}
        onRequestSubmit={handleProcess}
        primaryButtonDisabled={processing}
      >
        <div className="classrooms-page__form">
          <p className="classrooms-page__confirm-text">
            {t('modal.classroomLine', { classroom: processTarget ? classroomLabel(processTarget) : '' })}
          </p>
          <TextArea
            id="r-remark"
            labelText={t('modal.remark')}
            placeholder={t('modal.remarkPlaceholder')}
            value={processRemark}
            onChange={(e) => setProcessRemark(e.target.value)}
            rows={3}
          />
          {processError && (
            <InlineNotification kind="error" title={t('error.action')} subtitle={processError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>
    </Grid>
  )
}
