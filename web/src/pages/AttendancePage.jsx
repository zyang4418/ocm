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
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'
import { CheckinStatusTag, formatDateTime } from './attendanceUi.jsx'

// 课堂签到: list of attendance events with a create flow (pick offering →
// optional session → duration/late threshold). Creating navigates straight to
// the detail page, where the QR code is projected.
const headers = [
  { key: 'title', header: '标题' },
  { key: 'offering', header: '课程 / 教学班' },
  { key: 'sessionText', header: '课次' },
  { key: 'status', header: '状态' },
  { key: 'counts', header: '应到 / 出勤 / 迟到 / 缺勤 / 请假' },
  { key: 'startsAt', header: '开始时间' },
]

export default function AttendancePage() {
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('attendance:manage')

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
          text: `${o.catalogName} · ${o.teachingClassName} · ${o.semester}`,
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
          text: `${s.date} 第${s.periodStart}-${s.periodEnd}节 · ${s.classroomName}`,
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
      setCreateError('迟到阈值为非负整数（分钟）')
      return
    }
    if (Number.isNaN(duration) || duration < 0) {
      setCreateError('签到时长为非负整数（分钟），0 表示手动结束')
      return
    }
    if (!pickOffering && !form.title.trim()) {
      setCreateError('未选择开课时需填写签到标题')
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
          <BreadcrumbItem isCurrentPage>课堂签到</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">课堂签到</h1>
        <p className="courses-page__subtitle">
          发起签到后在大屏展示二维码，学生用小程序扫码或输入 6 位签到码完成签到。
        </p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {list.error && (
          <InlineNotification
            kind="error"
            title="加载失败"
            subtitle={list.error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
        {closeError && (
          <InlineNotification
            kind="error"
            title="结束签到失败"
            subtitle={closeError}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <DataTable rows={list.items} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title="签到列表" description={`共 ${list.total} 次签到`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder="搜索标题/课程" />
                  <Select
                    id="att-status"
                    labelText="状态"
                    hideLabel
                    value={statusFilter}
                    onChange={(e) => setStatusFilter(e.target.value)}
                  >
                    <SelectItem value="" text="全部状态" />
                    <SelectItem value="active" text="进行中" />
                    <SelectItem value="closed" text="已结束" />
                  </Select>
                  <TextInput id="att-from" labelText="开始日期起" hideLabel type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
                  <TextInput id="att-to" labelText="开始日期止" hideLabel type="date" value={to} onChange={(e) => setTo(e.target.value)} />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={openCreate}>
                      发起签到
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
                      <TableCell colSpan={headers.length + 1}>加载中…</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>
                        {list.q || statusFilter || from || to ? '未找到匹配的签到' : '暂无签到，点击右上角发起'}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const c = list.items.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          <TableCell>{c.title}</TableCell>
                          <TableCell>
                            {c.courseName ? `${c.courseName} / ${c.teachingClassName}` : '未关联开课'}
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
                                详情
                              </Button>
                              {canManage && c.status === 'active' && (
                                <Button kind="ghost" size="sm" onClick={() => handleClose(c)}>
                                  结束
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
        modalHeading="发起签到"
        primaryButtonText="发起并展示二维码"
        secondaryButtonText="取消"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
        size="lg"
      >
        <div className="courses-page__form">
          <ComboBox
            id="att-offering"
            titleText="开课（可选）"
            placeholder="选择开课后可联动课次与整学期报表"
            items={offerings}
            itemToString={(item) => (item ? item.text : '')}
            selectedItem={pickOffering}
            onChange={handlePickOffering}
            shouldFilterItem={() => true}
            helperText="不选择开课则为独立签到（如班会），仅统计扫码实到"
          />
          {pickOffering && (
            <ComboBox
              id="att-session"
              titleText="上课实例（可选）"
              placeholder="选择本次上课的课次"
              items={sessions}
              itemToString={(item) => (item ? item.text : '')}
              selectedItem={pickSession}
              onChange={(e) => setPickSession(e.selectedItem ?? null)}
              shouldFilterItem={() => true}
              helperText="选择课次后，本次签到计入该开课的整学期考勤"
            />
          )}
          <TextInput
            id="att-title"
            labelText="标题"
            placeholder="未选择开课时必填；选择开课/课次可留空自动生成"
            value={form.title}
            onChange={(e) => setForm({ ...form, title: e.target.value })}
          />
          <TextInput
            id="att-late"
            labelText="迟到阈值（分钟，0 表示不判迟到）"
            type="number"
            value={form.lateMinutes}
            onChange={(e) => setForm({ ...form, lateMinutes: e.target.value })}
          />
          <TextInput
            id="att-duration"
            labelText="签到时长（分钟，留空则直到手动结束）"
            type="number"
            placeholder="如 15"
            value={form.durationMinute}
            onChange={(e) => setForm({ ...form, durationMinute: e.target.value })}
          />
          {createError && (
            <InlineNotification kind="error" title="发起失败" subtitle={createError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>
    </Grid>
  )
}
