import {
  Breadcrumb,
  BreadcrumbItem,
  ClickableTile,
  Column,
  Grid,
  Tag,
  Tile,
} from '@carbon/react'
import {
  ArrowRight,
  ChartLine,
  CheckboxChecked,
  UserMultiple,
  WarningAlt,
} from '@carbon/icons-react'
import AppShell from '../components/AppShell.jsx'
import { useAuth } from '../auth/AuthContext.jsx'

const metrics = [
  { icon: UserMultiple, label: '活跃用户', value: '128', trend: '较上周 +12%', kind: 'green' },
  { icon: CheckboxChecked, label: '待办事项', value: '16', trend: '4 项即将到期', kind: 'blue' },
  { icon: WarningAlt, label: '未处理告警', value: '3', trend: '需要关注', kind: 'red' },
  { icon: ChartLine, label: '本月访问量', value: '8,432', trend: '较上月 +5.4%', kind: 'teal' },
]

const quickLinks = [
  { title: '用户管理', description: '维护组织成员与账号状态' },
  { title: '角色管理', description: '配置角色与访问权限' },
  { title: '参数配置', description: '调整系统级运行参数' },
  { title: '审计日志', description: '查看关键操作记录' },
]

export default function DashboardPage() {
  const { user } = useAuth()

  return (
    <AppShell>
      <Grid fullWidth className="dashboard">
        <Column sm={4} md={8} lg={16}>
          <Breadcrumb noTrailingSlash aria-label="面包屑导航">
            <BreadcrumbItem href="/">首页</BreadcrumbItem>
            <BreadcrumbItem isCurrentPage>概览</BreadcrumbItem>
          </Breadcrumb>
          <h1 className="dashboard__heading">概览</h1>
          <p className="dashboard__greeting">
            {user?.displayName}，欢迎回来。这里是系统的整体运行情况。
          </p>
        </Column>

        {metrics.map(({ icon: Icon, label, value, trend, kind }) => (
          <Column key={label} sm={4} md={4} lg={4}>
            <Tile className="dashboard__metric">
              <div className="dashboard__metric-header">
                <span className="dashboard__metric-label">{label}</span>
                <Icon size={20} aria-hidden />
              </div>
              <p className="dashboard__metric-value">{value}</p>
              <Tag type={kind} size="sm">
                {trend}
              </Tag>
            </Tile>
          </Column>
        ))}

        <Column sm={4} md={8} lg={16}>
          <h2 className="dashboard__section">快速开始</h2>
        </Column>
        {quickLinks.map(({ title, description }) => (
          <Column key={title} sm={4} md={4} lg={4}>
            <ClickableTile href="#" className="dashboard__quicklink">
              <div className="dashboard__quicklink-title">
                {title}
                <ArrowRight size={16} aria-hidden />
              </div>
              <p className="dashboard__quicklink-desc">{description}</p>
            </ClickableTile>
          </Column>
        ))}
      </Grid>
    </AppShell>
  )
}
