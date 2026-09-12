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
  type DataTableHeader,
  type TagProps,
} from '@carbon/react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDateTime } from '../i18n/formatters'
import type { Classroom, IotDevice, Paged } from '../types/api'

const STATUS_KIND: Record<string, TagProps<'div'>['type']> = {
  pending: 'blue',
  online: 'green',
  offline: 'gray',
}

export default function IotDevicesPage() {
  const { t } = useTranslation('iot')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('iot:manage')

  const headers: DataTableHeader[] = [
    { key: 'name', header: t('field.name') },
    { key: 'externalId', header: t('field.externalId') },
    { key: 'category', header: t('field.category') },
    { key: 'sourceId', header: t('field.sourceId') },
    { key: 'classroom', header: t('field.classroom') },
    { key: 'status', header: t('field.status') },
    { key: 'lastSeenAt', header: t('field.lastSeenAt') },
  ]

  const [filterStatus, setFilterStatus] = useState('')
  const list = usePagedList<IotDevice>({
    path: '/api/iot/devices',
    token,
    extraParams: { status: filterStatus },
  })
  const { loading } = list
  // Action errors are separate from the list fetch (the hook owns its error).
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError

  const [classrooms, setClassrooms] = useState<Classroom[]>([])
  useEffect(() => {
    if (!canManage) return
    apiFetch<Paged<Classroom>>('/api/classrooms?page_size=500', { token })
      .then((data) => setClassrooms(Array.isArray(data?.items) ? data.items : []))
      .catch(() => setClassrooms([]))
  }, [token, canManage])

  // Approve (claim) dialog for a pending device.
  const [approveTarget, setApproveTarget] = useState<IotDevice | null>(null)
  const [approveForm, setApproveForm] = useState({ name: '', classroomId: '', category: '' })
  const [approveError, setApproveError] = useState('')
  const [approving, setApproving] = useState(false)

  // Delete dialog.
  const [deleteTarget, setDeleteTarget] = useState<IotDevice | null>(null)
  const [deleting, setDeleting] = useState(false)

  const classroomLabel = (id: number | null | undefined) => {
    if (!id) return '—'
    const c = classrooms.find((x) => x.id === id)
    return c ? (c.building ? `${c.building} ${c.name}` : c.name) : `#${id}`
  }

  const openApprove = (d: IotDevice) => {
    setApproveTarget(d)
    setApproveForm({ name: d.name || d.externalId, classroomId: '', category: d.category === 'generic' ? '' : d.category })
    setApproveError('')
  }

  const handleApprove = async () => {
    if (!approveTarget) return
    if (!approveForm.name.trim()) return setApproveError(t('validation.nameRequired'))
    if (!approveForm.classroomId) return setApproveError(t('validation.classroomRequired'))
    try {
      setApproving(true)
      setApproveError('')
      const body: { name: string; classroomId: number; category?: string } = {
        name: approveForm.name.trim(),
        classroomId: Number(approveForm.classroomId),
      }
      if (approveForm.category.trim()) body.category = approveForm.category.trim()
      await apiFetch(`/api/iot/devices/${approveTarget.id}/approve`, { method: 'POST', token, body })
      setApproveTarget(null)
      list.reload()
    } catch (err) {
      setApproveError((err as Error).message)
    } finally {
      setApproving(false)
    }
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      setDeleting(true)
      setActionError('')
      await apiFetch(`/api/iot/devices/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      list.reload()
    } catch (err) {
      setActionError((err as Error).message)
    } finally {
      setDeleting(false)
    }
  }

  const rows = list.items.map((d) => ({
    id: String(d.id),
    name: d.name || '—',
    externalId: d.externalId,
    category: d.category,
    sourceId: d.sourceId,
    classroom: classroomLabel(d.classroomId),
    status: d.status,
    lastSeenAt: d.lastSeenAt ? formatDateTime(d.lastSeenAt) : '—',
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
            id="iot-status"
            labelText={t('filter.status')}
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text={t('filter.allStatuses')} />
            <SelectItem value="pending" text={t('status.pending')} />
            <SelectItem value="online" text={t('status.online')} />
            <SelectItem value="offline" text={t('status.offline')} />
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
                      const d = list.items.find((x) => String(x.id) === String(row.id))
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
                              <Button kind="ghost" size="sm" onClick={() => navigate(`/iot/${d?.id}`)}>
                                {t('action.detail')}
                              </Button>
                              {d && d.status === 'pending' && canManage && (
                                <Button kind="ghost" size="sm" onClick={() => openApprove(d)}>
                                  {t('action.approve')}
                                </Button>
                              )}
                              {d && canManage && (
                                <Button kind="ghost" size="sm" onClick={() => setDeleteTarget(d)}>
                                  {t('action.delete', { ns: 'common' })}
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

      {/* Approve dialog */}
      <Modal
        open={Boolean(approveTarget)}
        modalHeading={t('modal.approve')}
        primaryButtonText={t('modal.approveSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setApproveTarget(null)}
        onRequestSubmit={handleApprove}
        primaryButtonDisabled={approving}
      >
        <div className="classrooms-page__form">
          <p className="classrooms-page__confirm-text">{t('modal.approveHint')}</p>
          <TextInput
            id="iot-approve-name"
            labelText={t('modal.name')}
            value={approveForm.name}
            onChange={(e) => setApproveForm({ ...approveForm, name: e.target.value })}
          />
          <Select
            id="iot-approve-classroom"
            labelText={t('modal.classroom')}
            value={approveForm.classroomId}
            onChange={(e) => setApproveForm({ ...approveForm, classroomId: e.target.value })}
          >
            <SelectItem value="" text={t('validation.classroomRequired')} />
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={c.building ? `${c.building} ${c.name}` : c.name} />
            ))}
          </Select>
          <TextInput
            id="iot-approve-category"
            labelText={t('modal.category')}
            value={approveForm.category}
            onChange={(e) => setApproveForm({ ...approveForm, category: e.target.value })}
          />
          {approveError && (
            <InlineNotification kind="error" title={t('error.action')} subtitle={approveError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Delete dialog */}
      <Modal
        open={Boolean(deleteTarget)}
        modalHeading={t('modal.delete')}
        primaryButtonText={t('modal.deleteSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
        danger
      >
        <p className="classrooms-page__confirm-text">
          {t('modal.deleteLine', { name: deleteTarget?.name || deleteTarget?.externalId })}
        </p>
      </Modal>
    </Grid>
  )
}
