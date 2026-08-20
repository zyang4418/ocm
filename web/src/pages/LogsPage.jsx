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
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDate } from '../i18n/formatters'

// statusLabel/statusKind group HTTP status codes by class: the audit log
// records the outcome of every mutating request, so a 4xx shows an attempted
// (rejected) change, a 5xx a server-side failure.
function statusClass(code) {
  return Math.floor(code / 100)
}

const statusKind = {
  2: 'green',
  3: 'blue',
  4: 'yellow',
  5: 'red',
}

const headers = (t) => [
  { key: 'createdAt', header: t('field.createdAt') },
  { key: 'actorName', header: t('field.actorName') },
  { key: 'summary', header: t('field.summary') },
  { key: 'request', header: t('field.request') },
  { key: 'statusCode', header: t('field.statusCode') },
  { key: 'clientIp', header: t('field.clientIp') },
]

function pad(n) {
  return String(n).padStart(2, '0')
}

function fmt(d) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const defaultSettings = { retentionEnabled: true, retentionDays: 180 }

export default function LogsPage() {
  const { t } = useTranslation('logs')
  const { token, can } = useAuth()
  const navigate = useNavigate()
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
        setSaveNotice(t('settings.savedNotice'))
        list.reload()
      })
      .catch((err) => setActionError(err.message))
      .finally(() => setSaving(false))
  }

  const tableHeaders = headers(t)

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

          <DataTable rows={list.items} headers={tableHeaders}>
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
                        <TableHeader key={h.key} {...getHeaderProps({ header: h })}>
                          {h.header}
                        </TableHeader>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {list.loading ? (
                      <TableRow>
                        <TableCell colSpan={tableHeaders.length}>{t('empty.loading')}</TableCell>
                      </TableRow>
                    ) : rows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={tableHeaders.length}>
                          {list.q || from || to ? t('empty.search') : t('empty.none')}
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
