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
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
} from '@carbon/react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'
import { STATUS_LABEL, StatusTag, formatDateTime } from './attendanceUi.jsx'

// L2 整学期考勤报表: pick one offering → per-student × per-checkin matrix
// with per-status subtotals, downloadable as a two-sheet xlsx.
const TOTAL_KEYS = ['present', 'late', 'absent', 'leave']

export default function AttendanceReportPage() {
  const { token } = useAuth()
  const navigate = useNavigate()

  const [offerings, setOfferings] = useState([])
  const [pickOffering, setPickOffering] = useState(null)
  const [summary, setSummary] = useState(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

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

  const handlePickOffering = (e) => {
    setPickOffering(e.selectedItem ?? null)
    setSummary(null)
    setError('')
    if (e.selectedItem) loadSummary(e.selectedItem.id)
  }

  const loadSummary = async (offeringId) => {
    try {
      setLoading(true)
      setError('')
      const data = await apiFetch(`/api/checkins/summary?offering_id=${offeringId}`, { token })
      setSummary(data)
    } catch (err) {
      setError(err.message)
      setSummary(null)
    } finally {
      setLoading(false)
    }
  }

  const baseHeaders = [
    { key: 'displayName', header: '姓名' },
    { key: 'studentNo', header: '学号' },
    { key: 'adminClass', header: '行政班' },
  ]
  const checkins = summary?.checkins ?? []
  const headers = [
    ...baseHeaders,
    ...checkins.map((c) => ({ key: `c${c.id}`, header: c.startsAt.slice(0, 10) })),
    { key: 'totals', header: '出勤 / 迟到 / 缺勤 / 请假' },
  ]

  const buildRow = (r) => {
    const row = { id: r.userId, displayName: r.displayName, studentNo: r.studentNo, adminClass: r.adminClass }
    for (const c of checkins) {
      row[`c${c.id}`] = r.records[c.id] ?? ''
    }
    row.totals = TOTAL_KEYS.map((k) => r.totals[k] ?? 0).join(' / ')
    return row
  }
  const rows = (summary?.rows ?? []).map(buildRow)

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
          <BreadcrumbItem isCurrentPage>考勤报表</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">考勤报表</h1>
        <p className="courses-page__subtitle">
          选择一门开课，一次性查看整学期所有签到情况（仅统计关联了该开课/课次的签到）。
        </p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title="加载失败"
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
        <ComboBox
          id="report-offering"
          titleText="开课"
          placeholder="搜索并选择开课"
          items={offerings}
          itemToString={(item) => (item ? item.text : '')}
          selectedItem={pickOffering}
          onChange={handlePickOffering}
          shouldFilterItem={() => true}
          onFocus={() => {
            if (offerings.length === 0) loadOfferings()
          }}
        />

        {summary && (
          <>
            <p className="courses-page__subtitle">
              {summary.courseName} · {summary.teachingClassName} · {summary.semester} · 教师 {summary.teacher || '-'} ·
              共 {checkins.length} 次签到 · {summary.rows.length} 名学生
            </p>
            {checkins.length === 0 && (
              <InlineNotification
                kind="warning"
                title="该开课暂无签到"
                subtitle="发起签到时选择本开课（或其课次）后才会出现在报表中。"
                lowContrast
                hideCloseButton
                className="courses-page__notice"
              />
            )}
            {summary.rows.every((r) => !r.inRoster) && (
              <InlineNotification
                kind="warning"
                title="应到名单为空"
                subtitle="该开课的教学班尚未配置学生档案（行政班成员），无法统计缺勤。"
                lowContrast
                hideCloseButton
                className="courses-page__notice"
              />
            )}
          </>
        )}

        {summary && checkins.length > 0 && (
          <DataTable rows={rows} headers={headers}>
            {({ rows: tableRows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
              <TableContainer title="整学期签到明细" description="行为学生、列为签到，末列为四态小计">
                <TableToolbar {...getToolbarProps()}>
                  <TableToolbarContent>
                    <ExportButton
                      path={`/api/checkins/export?offering_id=${pickOffering?.id}`}
                      fallbackName="attendance-report.xlsx"
                      onError={setError}
                    />
                  </TableToolbarContent>
                </TableToolbar>
                <Table {...getTableProps()} size="sm">
                  <TableHead>
                    <TableRow>
                      {tableHeaders.map((header) => (
                        <TableHeader key={header.key} {...getHeaderProps({ header })}>
                          {header.header}
                        </TableHeader>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {loading ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>加载中…</TableCell>
                      </TableRow>
                    ) : tableRows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>暂无学生数据</TableCell>
                      </TableRow>
                    ) : (
                      tableRows.map((row) => {
                        const r = summary.rows.find((x) => String(x.userId) === String(row.id))
                        return (
                          <TableRow key={row.id} {...getRowProps({ row })}>
                            {row.cells.map((cell) => {
                              if (cell.info.header === 'displayName' || cell.info.header === 'studentNo' || cell.info.header === 'adminClass') {
                                return <TableCell key={cell.id}>{cell.value || '-'}</TableCell>
                              }
                              if (cell.info.header === 'totals') {
                                return <TableCell key={cell.id}>{cell.value}</TableCell>
                              }
                              const status = cell.value
                              return (
                                <TableCell key={cell.id}>
                                  {status ? <StatusTag status={status} /> : <span className="attendance-report__empty">—</span>}
                                </TableCell>
                              )
                            })}
                          </TableRow>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
          </DataTable>
        )}
        {loading && <p>加载中…</p>}
        {!summary && !loading && !error && (
          <p className="courses-page__subtitle">请选择开课查看整学期考勤。</p>
        )}
        {/* Legend for the matrix */}
        {summary && checkins.length > 0 && (
          <p className="courses-page__subtitle">
            图例:{' '}
            {Object.entries(STATUS_LABEL).map(([value, label]) => (
              <span key={value} className="attendance-report__legend">
                <StatusTag status={value} /> {label}
              </span>
            ))}{' '}
            · 最近一次签到开始于 {formatDateTime(checkins[0].startsAt)}
          </p>
        )}
      </Column>
    </Grid>
  )
}
