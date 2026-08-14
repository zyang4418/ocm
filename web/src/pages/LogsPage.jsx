import { useEffect, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  DataTable,
  Grid,
  InlineNotification,
  NumberInput,
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
  Toggle,
} from '@carbon/react'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'

// statusLabel/statusKind group HTTP status codes by class: the audit log
// records the outcome of every mutating request, so a 4xx shows an attempted
// (rejected) change, a 5xx a server-side failure.
function statusClass(code) {
  return Math.floor(code / 100)
}

const statusLabel = {
  2: '成功',
  3: '重定向',
  4: '拒绝',
  5: '错误',
}

const statusKind = {
  2: 'green',
  3: 'blue',
  4: 'yellow',
  5: 'red',
}

const headers = [
  { key: 'createdAt', header: '时间' },
  { key: 'actorName', header: '操作人' },
  { key: 'summary', header: '操作' },
  { key: 'request', header: '请求' },
  { key: 'statusCode', header: '结果' },
  { key: 'clientIp', header: 'IP' },
]

function pad(n) {
  return String(n).padStart(2, '0')
}

function fmt(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

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

const defaultSettings = { retentionEnabled: true, retentionDays: 180 }

export default function LogsPage() {
  const { token, can } = useAuth()
  const canManage = can('log:manage')

  // Date filters default to the last 30 days, the same window convention as
  // the bookings page.
  const today = new Date()
  const [from, setFrom] = useState(fmt(new Date(today.getFullYear(), today.getMonth(), today.getDate() - 30)))
  const [to, setTo] = useState(fmt(today))

  const list = usePagedList({
    path: '/api/logs',
    token,
    extraParams: { from, to },
  })
  // Settings fetch/save errors are separate from the list fetch (the hook owns its error).
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError

  const [settings, setSettings] = useState(defaultSettings)
  const [settingsLoaded, setSettingsLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveNotice, setSaveNotice] = useState('')

  useEffect(() => {
    apiFetch('/api/logs/settings', { token })
      .then((data) => {
        setSettings({
          retentionEnabled: Boolean(data?.retentionEnabled),
          retentionDays: Number.isFinite(Number(data?.retentionDays)) ? Number(data.retentionDays) : 180,
        })
        setSettingsLoaded(true)
      })
      .catch((err) => setActionError(err.message))
  }, [token])

  const saveSettings = () => {
    setSaving(true)
    setSaveNotice('')
    apiFetch('/api/logs/settings', {
      method: 'PUT',
      body: {
        retentionEnabled: settings.retentionEnabled,
        retentionDays: settings.retentionDays,
      },
      token,
    })
      .then(() => {
        setSaveNotice('日志保留设置已保存')
        list.reload()
      })
      .catch((err) => setActionError(err.message))
      .finally(() => setSaving(false))
  }

  return (
    <div className="logs-page">
      <Grid fullWidth>
        <Column sm={4} md={8} lg={16}>
          <Breadcrumb>
            <BreadcrumbItem href="/">首页</BreadcrumbItem>
            <BreadcrumbItem isCurrentPage>审计日志</BreadcrumbItem>
          </Breadcrumb>
          <h1 className="logs-page__heading">审计日志</h1>
          <p className="logs-page__subtitle">
            记录系统中的所有写操作（增删改、审批、导入等），仅管理员可见。日志按保留策略自动清理。
          </p>

          {error && (
            <InlineNotification kind="error" lowContrast title="加载失败" subtitle={error} />
          )}
          {saveNotice && (
            <InlineNotification kind="success" lowContrast title={saveNotice} />
          )}

          <div className="logs-page__filters">
            <TextInput
              id="logs-from"
              type="date"
              labelText="开始日期"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
            <TextInput
              id="logs-to"
              type="date"
              labelText="结束日期"
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
            <Button
              kind="ghost"
              size="md"
              onClick={() => {
                setFrom(fmt(new Date(today.getFullYear(), today.getMonth(), today.getDate() - 30)))
                setTo(fmt(today))
              }}
            >
              清空筛选
            </Button>
          </div>

          <DataTable rows={list.items} headers={headers}>
            {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps }) => (
              <TableContainer className="logs-page__table">
                <TableToolbar>
                  <TableToolbarContent>
                    <TableToolbarSearch
                      placeholder="搜索操作人 / 操作内容 / 请求路径"
                      value={list.q}
                      onChange={(e, v) => list.setQ(v ?? '')}
                    />
                  </TableToolbarContent>
                </TableToolbar>
                <Table {...getTableProps()}>
                  <TableHead>
                    <TableRow>
                      {tableHeaders.map((h) => (
                        <TableHeader key={h.key} {...getHeaderProps({ header: h })}>
                          {h.header}
                        </TableHeader>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {list.loading ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>加载中…</TableCell>
                      </TableRow>
                    ) : rows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>
                          {list.q || from || to ? '未找到匹配的日志' : '暂无日志'}
                        </TableCell>
                      </TableRow>
                    ) : (
                      rows.map((row) => {
                        const log = list.items.find((i) => String(i.id) === String(row.id)) ?? {}
                        const cls = statusClass(log.statusCode)
                        return (
                          <TableRow key={row.id} {...getRowProps({ row })}>
                            <TableCell>{formatDate(log.createdAt)}</TableCell>
                            <TableCell>{log.actorName || '-'}</TableCell>
                            <TableCell>
                              {log.summary || `${log.method ?? ''} ${log.path ?? ''}`.trim() || '-'}
                            </TableCell>
                            <TableCell>{`${log.method ?? ''} ${log.path ?? ''}`.trim() || '-'}</TableCell>
                            <TableCell>
                              <Tag type={statusKind[cls] || 'gray'} size="sm">
                                {statusLabel[cls] || ''} {log.statusCode ?? ''}
                              </Tag>
                            </TableCell>
                            <TableCell>{log.clientIp || '-'}</TableCell>
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
            total={list.total}
            page={list.page}
            pageSize={list.pageSize}
            onChange={({ page, pageSize }) => {
              if (pageSize !== list.pageSize) {
                list.setPageSize(pageSize)
              } else {
                list.setPage(page)
              }
            }}
          />

          <section className="logs-page__settings">
            <h2 className="logs-page__settings-heading">日志保留策略</h2>
            <p className="logs-page__settings-hint">
              超过保留天数的日志将被自动清理{canManage ? '' : '（仅管理员可修改）'}。
            </p>
            <Toggle
              id="retentionEnabled"
              labelText="启用日志保留"
              toggled={settings.retentionEnabled}
              disabled={!canManage || !settingsLoaded}
              onToggle={(checked) => setSettings({ ...settings, retentionEnabled: checked })}
            />
            <NumberInput
              id="retentionDays"
              label="保留天数"
              min={1}
              max={3650}
              value={settings.retentionDays}
              disabled={!canManage || !settingsLoaded || !settings.retentionEnabled}
              invalidText={`保留天数需在 1–3650 之间`}
              onChange={(e, { value }) =>
                setSettings({ ...settings, retentionDays: Number(value) })
              }
            />
            {canManage && (
              <Button
                size="md"
                disabled={saving || settings.retentionDays < 1 || settings.retentionDays > 3650}
                onClick={saveSettings}
              >
                保存设置
              </Button>
            )}
          </section>
        </Column>
      </Grid>
    </div>
  )
}
