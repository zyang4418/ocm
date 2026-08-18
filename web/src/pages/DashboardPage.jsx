import { lazy, Suspense } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  ClickableTile,
  Column,
  Grid,
  InlineNotification,
  Link,
  SkeletonText,
  Tag,
  Tile,
} from '@carbon/react'
import {
  Activity,
  ArrowRight,
  Building,
  Calendar,
  CheckmarkOutline,
  Event,
  WarningAlt,
} from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { openAiChat } from '../ai/chatInstance.js'
import useDashboardSummary from '../hooks/useDashboardSummary.js'

// The @carbon/charts band is code-split: the d3/charts bundle (~600KB) loads
// only when the homepage actually renders charts, keeping every other route's
// chunk lean.
const DashboardCharts = lazy(() => import('../dashboard/DashboardCharts.jsx'))

// Status tag colors match the bookings / repairs pages so every page reads the
// same semantics the same way.
const bookingStatusKind = {
  pending: 'blue',
  approved: 'green',
  rejected: 'red',
  cancelled: 'gray',
}

const repairStatusKind = {
  open: 'red',
  processing: 'yellow',
  completed: 'gray',
  confirmed: 'green',
}

const repairStatusLabel = {
  open: '待处理',
  processing: '维修中',
  completed: '已维修',
  confirmed: '已确认',
}

const WEEKDAYS = ['日', '一', '二', '三', '四', '五', '六']

function periodLabel(it) {
  if (!it) return ''
  if (it.periodStart === it.periodEnd) return `第 ${it.periodStart} 节`
  return `第 ${it.periodStart}–${it.periodEnd} 节`
}

function formatDateTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Panel is the shared frame for every dashboard section: a Tile with a
// heading, an optional trailing "view all" link (AppShell-style href +
// navigate) and a padded body.
function Panel({ title, viewAll, children }) {
  const navigate = useNavigate()
  return (
    <Tile className="dashboard__panel">
      <div className="dashboard__panel-head">
        <h2 className="dashboard__panel-title">{title}</h2>
        {viewAll && (
          <Link
            href={viewAll.to}
            className="dashboard__panel-link"
            onClick={(e) => {
              e.preventDefault()
              navigate(viewAll.to)
            }}
          >
            {viewAll.label}
          </Link>
        )}
      </div>
      {children}
    </Tile>
  )
}

function EmptyRow({ text }) {
  return <p className="dashboard__empty">{text}</p>
}

// Same frame as the real chart panels, shown while the split charts chunk
// downloads - the band reserves its height instead of popping in.
function ChartPanelSkeleton() {
  return (
    <Tile className="dashboard__panel dashboard__chart-panel">
      <SkeletonText heading width="30%" />
      <SkeletonText className="dashboard__chart-skeleton" />
    </Tile>
  )
}

const quickLinks = [
  { title: 'AI 助手', description: '问答查询教室与课表，生成预约方案', openChat: true, perm: 'ai:chat' },
  { title: '用户管理', description: '维护组织成员与账号状态', href: '/users', perm: 'user:read' },
  { title: '角色管理', description: '配置角色与访问权限', href: '/roles', perm: 'role:manage' },
  { title: '参数配置', description: '调整系统级运行参数', href: '/settings', adminOnly: true },
  { title: '审计日志', description: '查看关键操作记录', href: '/logs', perm: 'log:read' },
]

export default function DashboardPage() {
  const { user, token, can } = useAuth()
  const navigate = useNavigate()
  const { date, data, loading, error, reload } = useDashboardSummary(token)

  const go = (path) => (e) => {
    e.preventDefault()
    navigate(path)
  }

  const weekday = WEEKDAYS[new Date(`${date}T00:00:00`).getDay()]
  const dateLabel = `${Number(date.slice(5, 7))} 月 ${Number(date.slice(8, 10))} 日`

  // KPI row: built from the fields the backend actually returned - each entry
  // is permission-gated server side, so the row collapses naturally for
  // low-privilege users instead of showing forbidden modules.
  const kpis = []
  if (data?.todaySessions)
    kpis.push({ key: 'sessions', icon: Calendar, label: '今日课程', value: data.todaySessions.total, unit: '节', href: '/timetable' })
  if (data?.classroomTotal !== undefined)
    kpis.push({ key: 'classrooms', icon: Building, label: '教室总数', value: data.classroomTotal, unit: '间', href: '/classrooms' })
  if (data?.pendingBookings)
    kpis.push({ key: 'pending', icon: CheckmarkOutline, label: '待审批预约', value: data.pendingBookings.total, unit: '项', href: '/bookings', alert: data.pendingBookings.total > 0 })
  if (data?.openRepairs)
    kpis.push({ key: 'repairs', icon: WarningAlt, label: '未结报修', value: data.openRepairs.total, unit: '项', href: '/repairs', alert: data.openRepairs.total > 0 })
  if (data?.myBookings)
    kpis.push({ key: 'mine', icon: Event, label: '我的预约', value: data.myBookings.length, unit: '项', href: '/bookings' })

  const sessions = data?.todaySessions?.items ?? []
  const pending = data?.pendingBookings?.items ?? []
  const repairs = data?.openRepairs?.items ?? []
  const mine = data?.myBookings ?? []
  const logs = data?.recentLogs ?? []

  return (
    <Grid fullWidth className="dashboard">
      <Column sm={4} md={8} lg={16}>
        <Breadcrumb noTrailingSlash aria-label="面包屑导航">
          <BreadcrumbItem href="/">首页</BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>概览</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="dashboard__heading">概览</h1>
        <p className="dashboard__greeting">
          {user?.displayName}，欢迎回来。今天是 {dateLabel} 星期{weekday}。
        </p>
      </Column>

      {error && (
        <Column sm={4} md={8} lg={16}>
          <InlineNotification
            kind="error"
            title="概览数据加载失败"
            subtitle={error}
            actions={<Button size="sm" kind="tertiary" onClick={reload}>重试</Button>}
            className="dashboard__error"
          />
        </Column>
      )}

      {/* KPI row */}
      {(loading || kpis.length > 0) && (
        <Column sm={4} md={8} lg={16}>
          <div className="dashboard__kpis">
            {loading
              ? Array.from({ length: 4 }, (_, i) => (
                  <Tile key={i} className="dashboard__kpi">
                    <SkeletonText className="dashboard__kpi-skeleton" />
                  </Tile>
                ))
              : kpis.map(({ key, icon: Icon, label, value, unit, href, alert }) => (
                  <ClickableTile
                    key={key}
                    href={href}
                    onClick={go(href)}
                    className={`dashboard__kpi${alert ? ' dashboard__kpi--alert' : ''}`}
                  >
                    <div className="dashboard__kpi-head">
                      <span className="dashboard__kpi-label">{label}</span>
                      <Icon size={20} aria-hidden />
                    </div>
                    <p className="dashboard__kpi-value">
                      {value}
                      <span className="dashboard__kpi-unit">{unit}</span>
                    </p>
                  </ClickableTile>
                ))}
          </div>
        </Column>
      )}

      {/* Charts band: same omitempty semantics as the sections - a chart with
          no server data (permission or an empty day) collapses its column */}
      {!loading && (data?.sessionPeriods?.length > 0 || data?.bookingLoad?.length > 0) && (
        <Column sm={4} md={8} lg={16} className="dashboard__charts-col">
          <Suspense
            fallback={
              <Grid className="dashboard__charts">
                {data?.sessionPeriods?.length > 0 && (
                  <Column md={4} lg={8}>
                    <ChartPanelSkeleton />
                  </Column>
                )}
                {data?.bookingLoad?.length > 0 && (
                  <Column md={4} lg={8}>
                    <ChartPanelSkeleton />
                  </Column>
                )}
              </Grid>
            }
          >
            <DashboardCharts
              periods={data?.sessionPeriods ?? []}
              load={data?.bookingLoad ?? []}
              loadAll={can('booking:approve')}
            />
          </Suspense>
        </Column>
      )}

      {/* Main band: today's sessions + to-do center */}
      <Column sm={4} md={8} lg={10} className="dashboard__main-col">
        {loading ? (
          <Tile className="dashboard__panel">
            <SkeletonText heading width="30%" />
            <SkeletonText width="90%" />
            <SkeletonText width="80%" />
            <SkeletonText width="85%" />
          </Tile>
        ) : data?.todaySessions ? (
          <Panel title="今日课程" viewAll={{ to: '/timetable', label: '完整课表' }}>
            {sessions.length === 0 ? (
              <EmptyRow text="今日没有排课。" />
            ) : (
              <ul className="dashboard__sessions">
                {sessions.map((s) => (
                  <li key={s.id} className="dashboard__session">
                    <Tag size="sm" type="cool-gray">{periodLabel(s)}</Tag>
                    <div className="dashboard__session-main">
                      <span className="dashboard__session-course" title={s.courseName}>{s.courseName}</span>
                      <span className="dashboard__session-meta">
                        {s.classroomName}
                        {s.teacher ? ` · ${s.teacher}` : ''}
                      </span>
                    </div>
                  </li>
                ))}
              </ul>
            )}
            {data.todaySessions.total > sessions.length && (
              <p className="dashboard__more">共 {data.todaySessions.total} 节，仅显示前 {sessions.length} 节</p>
            )}
          </Panel>
        ) : null}
      </Column>

      <Column sm={4} md={8} lg={6} className="dashboard__main-col">
        {loading ? (
          <Tile className="dashboard__panel">
            <SkeletonText heading width="40%" />
            <SkeletonText width="85%" />
            <SkeletonText width="75%" />
          </Tile>
        ) : (
          <div className="dashboard__todos">
            {data?.pendingBookings && (
              <Panel title="待审批预约" viewAll={{ to: '/bookings', label: '全部预约' }}>
                {pending.length === 0 ? (
                  <EmptyRow text="没有待审批的预约。" />
                ) : (
                  <ul className="dashboard__list">
                    {pending.map((b) => (
                      <li key={b.id} className="dashboard__list-row">
                        <div className="dashboard__list-main">
                          <span className="dashboard__list-title">
                            {b.classroomName} · {b.date} {periodLabel(b)}
                          </span>
                          <span className="dashboard__list-meta">
                            {b.displayName || b.username}
                            {b.purpose ? ` · ${b.purpose}` : ''}
                          </span>
                        </div>
                        <Tag size="sm" type={bookingStatusKind[b.status] ?? 'gray'}>待审批</Tag>
                      </li>
                    ))}
                  </ul>
                )}
              </Panel>
            )}
            {data?.openRepairs && (
              <Panel title="未结报修" viewAll={{ to: '/repairs', label: '全部工单' }}>
                {repairs.length === 0 ? (
                  <EmptyRow text="没有未结的报修工单。" />
                ) : (
                  <ul className="dashboard__list">
                    {repairs.map((rp) => (
                      <li key={rp.id} className="dashboard__list-row">
                        <div className="dashboard__list-main">
                          <span className="dashboard__list-title" title={rp.description}>
                            {rp.classroomName} · {rp.description}
                          </span>
                          <span className="dashboard__list-meta">{rp.creatorName}</span>
                        </div>
                        <Tag size="sm" type={repairStatusKind[rp.status] ?? 'gray'}>
                          {repairStatusLabel[rp.status] ?? rp.status}
                        </Tag>
                      </li>
                    ))}
                  </ul>
                )}
              </Panel>
            )}
            {data?.myBookings && mine.length > 0 && (
              <Panel title="我的近期预约" viewAll={{ to: '/bookings', label: '我的预约' }}>
                <ul className="dashboard__list">
                  {mine.map((b) => (
                    <li key={b.id} className="dashboard__list-row">
                      <div className="dashboard__list-main">
                        <span className="dashboard__list-title">
                          {b.classroomName} · {b.date} {periodLabel(b)}
                        </span>
                        <span className="dashboard__list-meta">{b.purpose}</span>
                      </div>
                      <Tag size="sm" type={bookingStatusKind[b.status] ?? 'gray'}>已通过</Tag>
                    </li>
                  ))}
                </ul>
              </Panel>
            )}
          </div>
        )}
      </Column>

      {/* Bottom band: recent activity + quick start */}
      {!loading && data?.recentLogs && (
        <Column sm={4} md={8} lg={10}>
          <Panel title="最近动态" viewAll={{ to: '/logs', label: '全部日志' }}>
            {logs.length === 0 ? (
              <EmptyRow text="暂无操作记录。" />
            ) : (
              <ul className="dashboard__logs">
                {logs.map((l) => (
                  <li key={l.id} className="dashboard__log">
                    <span className="dashboard__log-time">{formatDateTime(l.createdAt)}</span>
                    <span className="dashboard__log-actor">{l.actorName || '系统'}</span>
                    <span className="dashboard__log-text">
                      {l.summary || `${l.method} ${l.path}`}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </Column>
      )}

      {!loading && (
        <Column sm={4} md={8} lg={6}>
          <div className="dashboard__quicklinks">
            <h2 className="dashboard__section">快速开始</h2>
            <div className="dashboard__quickgrid">
              {quickLinks
                .filter((l) => (!l.adminOnly || can('*')) && (!l.perm || can(l.perm)))
                .map(({ title, description, href, openChat }) => (
                  <ClickableTile
                    key={title}
                    href={href ?? '#'}
                    className="dashboard__quicklink"
                    onClick={
                      openChat
                        ? (e) => {
                            e.preventDefault()
                            openAiChat()
                          }
                        : href
                          ? go(href)
                          : undefined
                    }
                  >
                    <div className="dashboard__quicklink-title">
                      {title}
                      <ArrowRight size={16} aria-hidden />
                    </div>
                    <p className="dashboard__quicklink-desc">{description}</p>
                  </ClickableTile>
                ))}
            </div>
          </div>
        </Column>
      )}
    </Grid>
  )
}
