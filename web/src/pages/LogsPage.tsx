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
  type DataTableHeader,
  type TagProps,
} from '@carbon/react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDate } from '../i18n/formatters'
import type { LogRetentionSettings, LogView } from '../types/api'

// statusKind groups HTTP status codes by class: the audit log records the
// outcome of every mutating request, so a 4xx shows an attempted (rejected)
// change, a 5xx a server-side failure.
function statusClass(code: number) {
  return Math.floor(code / 100)
}

// Carbon tags have no 'yellow'; the old JSX value silently rendered the
// default gray. Keep that exact look via a cast until a redesign picks a
// real color for "attempted" rows.
const STATUS_KIND: Record<number, TagProps<'div'>['type']> = {
  2: 'green',
  3: 'blue',
  4: 'yellow' as TagProps<'div'>['type'],
  5: 'red',
}

function pad(n: number) {
  return String(n).padStart(2, '0')
}

function fmt(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const DEFAULT_SETTINGS: LogRetentionSettings = { retentionEnabled: true, retentionDays: 180 }

export default function LogsPage() {
  const { t } = useTranslation('logs')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('log:manage')

  const headers: DataTableHeader[] = [
    { key: 'createdAt', header: t('field.createdAt') },
    { key: 'actorName', header: t('field.actorName') },
    { key: 'summary', header: t('field.summary') },
    { key: 'request', header: t('field.request') },
    { key: 'statusCode', header: t('field.statusCode') },
    { key: 'clientIp', header: t('field.clientIp') },
  ]

  // Date filters default to the last 30 days, the same window convention as
  // the bookings page.
  const today = new Date()
  const [from, setFrom] = useState(fmt(new Date(today.getFullYear(), today.getMonth(), today.getDate() - 30)))
  const [to, setTo] = useState(fmt(today))

  const list = usePagedList<LogView>({
    path: '/api/logs',
    token,
    extraParams: { from, to },
  })
  // Settings fetch/save errors are separate from the list fetch (the hook owns its error).
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError

  const [settings, setSettings] = useState<LogRetentionSettings>(DEFAULT_SETTINGS)
  const [settingsLoaded, setSettingsLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveNotice, setSaveNotice] = useState('')

  useEffect(() => {
    apiFetch<LogRetentionSettings>('/api/logs/settings', { token })
      .then((data) => {
        setSettings({
          retentionEnabled: Boolean(data?.retentionEnabled),
          retentionDays: Number.isFinite(Number(data?.retentionDays)) ? Number(data.retentionDays) : 180,
        })
        setSettingsLoaded(true)
      })
      .catch((err: Error) => setActionError(err.message))
  }, [token])

  const saveSettings = () => {
    setSaving(true)
    setSaveNotice('')
    const body: LogRetentionSettings = {
      retentionEnabled: settings.retentionEnabled,
      retentionDays: settings.retentionDays,
    }
    apiFetch('/api/logs/settings', { method: 'PUT', token, body })
      .then(() => {
        setSaveNotice(t('settings.savedNotice'))
        list.reload()
      })
      .catch((err: Error) => setActionError(err.message))
      .finally(() => setSaving(false))
  }

  // Carbon DataTable keys rows by a string id.
  const tableRows = list.items.map((l) => ({ ...l, id: String(l.id) }))

  return (
    <div className="logs-page">
      <Grid fullWidth>
        <Column sm={4} md={8} lg={16}>
          <Breadcrumb aria-label={t('aria.breadcrumb', { ns: 'common' })}>
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
          <h1 className="logs-page__heading">{t('title')}</h1>
          <p className="logs-page__subtitle">{t('subtitle')}</p>

          {error && (
            <InlineNotification kind="error" lowContrast title={t('error.load')} subtitle={error} />
          )}
          {saveNotice && (
            <InlineNotification kind="success" lowContrast title={saveNotice} />
          )}

          <div className="logs-page__filters">
            <TextInput
              id="logs-from"
              type="date"
              labelText={t('filter.from')}
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
            <TextInput
              id="logs-to"
              type="date"
              labelText={t('filter.to')}
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
              {t('filter.clear')}
            </Button>
          </div>

          <DataTable rows={tableRows} headers={headers}>
            {({ rows, headers: renderedHeaders, getTableProps, getHeaderProps, getRowProps }) => (
              <TableContainer className="logs-page__table">
                <TableToolbar>
                  <TableToolbarContent>
                    <TableToolbarSearch
                      placeholder={t('searchPlaceholder')}
                      value={list.q}
                      onChange={(e, v) => list.setQ(v ?? '')}
                    />
                  </TableToolbarContent>
                </TableToolbar>
                <Table {...getTableProps()}>
                  <TableHead>
                    <TableRow>
                      {renderedHeaders.map((h) => (
                        <TableHeader {...getHeaderProps({ header: h })}>
                          {h.header}
                        </TableHeader>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {list.loading ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>{t('empty.loading')}</TableCell>
                      </TableRow>
                    ) : rows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>
                          {list.q || from || to ? t('empty.search') : t('empty.none')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      rows.map((row) => {
                        const log = list.items.find((i) => String(i.id) === String(row.id))
                        if (!log) return null
                        const cls = statusClass(log.statusCode)
                        return (
                          <TableRow {...getRowProps({ row })}>
                            <TableCell>{formatDate(log.createdAt)}</TableCell>
                            <TableCell>{log.actorName || '-'}</TableCell>
                            <TableCell>
                              {log.summary || `${log.method ?? ''} ${log.path ?? ''}`.trim() || '-'}
                            </TableCell>
                            <TableCell>{`${log.method ?? ''} ${log.path ?? ''}`.trim() || '-'}</TableCell>
                            <TableCell>
                              <Tag type={STATUS_KIND[cls] || 'gray'} size="sm">
                                {t('result.' + cls, { defaultValue: '' })} {log.statusCode ?? ''}
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
            page={list.page}
            pageSize={list.pageSize}
            totalItems={list.total}
            onPageChange={list.setPage}
            onPageSizeChange={list.setPageSize}
          />

          <section className="logs-page__settings">
            <h2 className="logs-page__settings-heading">{t('settings.heading')}</h2>
            <p className="logs-page__settings-hint">
              {t('settings.hint')}{canManage ? '' : t('settings.adminOnlySuffix')}
            </p>
            <Toggle
              id="retentionEnabled"
              labelText={t('settings.retentionEnabledLabel')}
              toggled={settings.retentionEnabled}
              disabled={!canManage || !settingsLoaded}
              onToggle={(checked) => setSettings({ ...settings, retentionEnabled: checked })}
            />
            <NumberInput
              id="retentionDays"
              label={t('settings.retentionDaysLabel')}
              min={1}
              max={3650}
              value={settings.retentionDays}
              disabled={!canManage || !settingsLoaded || !settings.retentionEnabled}
              invalidText={t('settings.retentionDaysInvalid')}
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
                {t('settings.saveButton')}
              </Button>
            )}
          </section>
        </Column>
      </Grid>
    </div>
  )
}
