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
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'
import { CheckinStatusTag, STATUS_LABEL, StatusTag, formatDateTime } from './attendanceUi.jsx'

// 签到详情: QR code + countdown + live counts (5s poll) + record list with
// per-student corrections. This is the page the teacher projects.
const headers = [
  { key: 'displayName', header: '姓名' },
  { key: 'studentNo', header: '学号' },
  { key: 'adminClass', header: '行政班' },
  { key: 'status', header: '状态' },
  { key: 'checkedAt', header: '签到时间' },
  { key: 'inRoster', header: '名单' },
]

export default function AttendanceDetailPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { token, can } = useAuth()
  const canManage = can('attendance:manage')

  const [checkin, setCheckin] = useState(null)
  const [detailError, setDetailError] = useState('')
  const [now, setNow] = useState(() => new Date())

  const [statusFilter, setStatusFilter] = useState('')
  const list = usePagedList({ path: `/api/checkins/${id}/records`, token, extraParams: { status: statusFilter } })
  const { loading } = list

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
    { label: '应到', value: counts?.expected ?? '-' },
    { label: '出勤', value: counts?.present ?? '-' },
    { label: '迟到', value: counts?.late ?? '-' },
    { label: '缺勤', value: counts?.absent ?? '-' },
    { label: '请假', value: counts?.leave ?? '-' },
  ]
  const expired = checkin?.status === 'active' && remainingSeconds() === 0 && checkin.expiresAt

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
          <BreadcrumbItem
            href="/attendance"
            onClick={(e) => {
              e.preventDefault()
              navigate('/attendance')
            }}
          >
            课堂签到
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>{checkin?.title ?? '签到详情'}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">{checkin?.title ?? '签到详情'}</h1>
        <p className="courses-page__subtitle">
          {checkin && (
            <>
              签到码 {checkin.code} · {checkin.courseName ? `${checkin.courseName} / ${checkin.teachingClassName}` : '独立签到'}
              {checkin.sessionText ? ` · ${checkin.sessionText}` : ''} · 开始于 {formatDateTime(checkin.startsAt)}
            </>
          )}
        </p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {detailError && (
          <InlineNotification
            kind="error"
            title="加载失败"
            subtitle={detailError}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
        {actionError && (
          <InlineNotification
            kind="error"
            title="操作失败"
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
                      ? '已过签到时长，扫码将被拒绝'
                      : checkin.expiresAt
                        ? `剩余 ${String(Math.floor(remainingSeconds() / 60)).padStart(2, '0')}:${String(remainingSeconds() % 60).padStart(2, '0')}`
                        : '手动结束模式'}
                  </p>
                )}
                {checkin.lateMinutes > 0 && <p>迟到阈值：{checkin.lateMinutes} 分钟</p>}
                {canManage && checkin.status === 'active' && (
                  <Button kind="danger--ghost" size="sm" onClick={handleClose}>
                    结束签到
                  </Button>
                )}
                <p className="attendance-detail__hint">
                  学生打开小程序「签到中心」扫码，或输入上方 6 位签到码。
                </p>
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
                title="应到人数为 0"
                subtitle="该开课的教学班尚未配置学生档案（行政班成员），无法统计缺勤。"
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
            title="记录加载失败"
            subtitle={list.error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <DataTable rows={list.items} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title="签到记录" description={`共 ${list.total} 人`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder="搜索姓名/学号" />
                  <Select
                    id="rec-status"
                    labelText="状态"
                    hideLabel
                    value={statusFilter}
                    onChange={(e) => setStatusFilter(e.target.value)}
                  >
                    <SelectItem value="" text="全部状态" />
                    <SelectItem value="present" text="出勤" />
                    <SelectItem value="late" text="迟到" />
                    <SelectItem value="absent" text="缺勤" />
                    <SelectItem value="leave" text="请假" />
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
                    {canManage && <TableHeader>操作</TableHeader>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>加载中…</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>
                        {statusFilter ? '该状态下暂无记录' : '暂无记录'}
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
                          <TableCell>{r.inRoster ? '名单内' : '名单外'}</TableCell>
                          {canManage && (
                            <TableCell>
                              <Button kind="ghost" size="sm" onClick={() => openEdit(r)}>
                                修改
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
        modalHeading={`修改签到：${editTarget?.displayName ?? ''}`}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleSave}
        primaryButtonDisabled={saving}
      >
        <div className="courses-page__form">
          <Select
            id="edit-status"
            labelText="签到状态"
            value={editStatus}
            onChange={(e) => setEditStatus(e.target.value)}
          >
            {Object.entries(STATUS_LABEL).map(([value, label]) => (
              <SelectItem key={value} value={value} text={label} />
            ))}
          </Select>
          <p className="courses-page__subtitle">
            学生之后重复扫码不会覆盖本次修正（扫码只对首次生效）。
          </p>
          {editError && (
            <InlineNotification kind="error" title="保存失败" subtitle={editError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>
    </Grid>
  )
}
