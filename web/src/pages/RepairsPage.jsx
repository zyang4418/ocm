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
} from '@carbon/react'
import { Add, CheckmarkOutline } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'

const statusLabel = {
  open: '待处理',
  processing: '处理中',
  completed: '待确认',
  confirmed: '已确认',
}
const statusKind = {
  open: 'red',
  processing: 'blue',
  completed: 'purple',
  confirmed: 'green',
}

const headers = [
  { key: 'classroom', header: '教室' },
  { key: 'description', header: '故障描述' },
  { key: 'creatorName', header: '报修人' },
  { key: 'assigneeName', header: '处理人' },
  { key: 'status', header: '状态' },
  { key: 'createdAt', header: '提交时间' },
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

function classroomLabel(r) {
  return r.building ? `${r.building} ${r.classroomName}` : r.classroomName
}

export default function RepairsPage() {
  const { token, user: currentUser, can } = useAuth()
  const navigate = useNavigate()
  const canSubmit = can('repair:create')
  const canAssign = can('repair:assign')

  const [filterStatus, setFilterStatus] = useState('')
  const list = usePagedList({
    path: '/api/repairs',
    token,
    extraParams: { status: filterStatus },
  })
  const { loading } = list
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError

  const [classrooms, setClassrooms] = useState([])

  // Submit form.
  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState({ classroomId: '', description: '' })
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)

  // Process (start / complete) dialog.
  const [processTarget, setProcessTarget] = useState(null)
  const [processRemark, setProcessRemark] = useState('')
  const [processError, setProcessError] = useState('')
  const [processing, setProcessing] = useState(false)

  const [actingId, setActingId] = useState(null)

  useEffect(() => {
    if (!canSubmit) return
    apiFetch('/api/classrooms?page_size=500', { token })
      .then((data) => setClassrooms(Array.isArray(data?.items) ? data.items : []))
      .catch(() => setClassrooms([]))
  }, [token, canSubmit])

  const openCreate = () => {
    setForm({ classroomId: '', description: '' })
    setFormError('')
    setFormOpen(true)
  }

  const handleCreate = async () => {
    if (!form.classroomId) return setFormError('请选择教室')
    if (!form.description.trim()) return setFormError('请填写故障描述')
    try {
      setSaving(true)
      setFormError('')
      await apiFetch('/api/repairs', {
        method: 'POST',
        token,
        body: { classroomId: Number(form.classroomId), description: form.description.trim() },
      })
      setFormOpen(false)
      list.reload()
    } catch (err) {
      setFormError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const openProcess = (row, nextStatus) => {
    setProcessTarget({ ...row, nextStatus })
    setProcessRemark('')
    setProcessError('')
  }

  const handleProcess = async () => {
    if (!processTarget) return
    try {
      setProcessing(true)
      setProcessError('')
      await apiFetch(`/api/repairs/${processTarget.id}`, {
        method: 'PUT',
        token,
        body: { status: processTarget.nextStatus, remark: processRemark.trim() },
      })
      setProcessTarget(null)
      list.reload()
    } catch (err) {
      setProcessError(err.message)
    } finally {
      setProcessing(false)
    }
  }

  const handleConfirm = async (row) => {
    try {
      setActingId(row.id)
      setActionError('')
      await apiFetch(`/api/repairs/${row.id}/confirm`, { method: 'POST', token })
      list.reload()
    } catch (err) {
      setActionError(err.message)
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
          <BreadcrumbItem isCurrentPage>教室报修</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="classrooms-page__heading">教室报修</h1>
        <p className="classrooms-page__subtitle">
          提交与跟踪教室设备设施报修，维修端指派处理，报修人确认完成。
        </p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title="操作失败"
            subtitle={error}
            lowContrast
            hideCloseButton
            className="classrooms-page__notice"
          />
        )}

        <div className="bookings-page__filters">
          <Select
            id="f-status"
            labelText="状态"
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text="全部状态" />
            <SelectItem value="open" text="待处理" />
            <SelectItem value="processing" text="处理中" />
            <SelectItem value="completed" text="待确认" />
            <SelectItem value="confirmed" text="已确认" />
          </Select>
        </div>

        <DataTable rows={rows} headers={headers}>
          {({ rows: tableRows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title="报修工单" description={`共 ${list.total} 条工单`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder="搜索报修" />
                  {canSubmit && (
                    <Button renderIcon={Add} size="sm" onClick={openCreate}>
                      提交报修
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
                      <TableCell colSpan={colSpan}>{list.q ? '未找到匹配的报修' : '暂无报修工单'}</TableCell>
                    </TableRow>
                  ) : (
                    tableRows.map((row) => {
                      const r = list.items.find((x) => String(x.id) === String(row.id))
                      const isCreator = Number(currentUser?.id) === r?.creatorId
                      const canStart = canAssign && r?.status === 'open'
                      const canFinish = canAssign && r?.status === 'processing'
                      const canConfirm = isCreator && r?.status === 'completed'
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
                            return <TableCell key={cell.id}>{cell.value}</TableCell>
                          })}
                          <TableCell>
                            <div className="classrooms-page__actions">
                              {canStart && (
                                <Button kind="ghost" size="sm" onClick={() => openProcess(r, 'processing')} disabled={actingId === r.id}>
                                  开始处理
                                </Button>
                              )}
                              {canFinish && (
                                <Button kind="ghost" size="sm" onClick={() => openProcess(r, 'completed')} disabled={actingId === r.id}>
                                  完成
                                </Button>
                              )}
                              {canConfirm && (
                                <Button kind="ghost" size="sm" renderIcon={CheckmarkOutline} onClick={() => handleConfirm(r)} disabled={actingId === r.id}>
                                  确认完成
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
        modalHeading="提交报修"
        primaryButtonText="提交"
        secondaryButtonText="取消"
        onRequestClose={() => setFormOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={saving}
      >
        <div className="classrooms-page__form">
          <Select
            id="r-classroom"
            labelText="教室"
            value={form.classroomId}
            onChange={(e) => setForm({ ...form, classroomId: e.target.value })}
          >
            <SelectItem value="" text="请选择教室" />
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={classroomLabel({ building: c.building, classroomName: c.name })} />
            ))}
          </Select>
          <TextArea
            id="r-description"
            labelText="故障描述"
            placeholder="请描述故障现象，如投影仪无法开机、空调不制冷等"
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
            rows={4}
          />
          {formError && (
            <InlineNotification kind="error" title="提交失败" subtitle={formError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Process dialog */}
      <Modal
        open={Boolean(processTarget)}
        modalHeading={processTarget?.nextStatus === 'processing' ? '开始处理' : '完成报修'}
        primaryButtonText="确定"
        secondaryButtonText="取消"
        onRequestClose={() => setProcessTarget(null)}
        onRequestSubmit={handleProcess}
        primaryButtonDisabled={processing}
      >
        <div className="classrooms-page__form">
          <p className="classrooms-page__confirm-text">
            教室：{processTarget ? classroomLabel(processTarget) : ''}
          </p>
          <TextArea
            id="r-remark"
            labelText="处理备注（选填）"
            placeholder="填写处理说明，将展示给报修人"
            value={processRemark}
            onChange={(e) => setProcessRemark(e.target.value)}
            rows={3}
          />
          {processError && (
            <InlineNotification kind="error" title="操作失败" subtitle={processError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>
    </Grid>
  )
}
