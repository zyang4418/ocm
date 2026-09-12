import { useCallback, useEffect, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  Grid,
  InlineNotification,
  Modal,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Tag,
  TextArea,
  TextInput,
  type TagProps,
} from '@carbon/react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch, apiStream } from '../auth/api'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDateTime } from '../i18n/formatters'
import type { IotDevice, IotDeviceEvent } from '../types/api'

const STATUS_KIND: Record<string, TagProps<'div'>['type']> = {
  pending: 'blue',
  online: 'green',
  offline: 'gray',
}

// The command types the console renders as dedicated buttons; they match the
// backend whitelist (internal/iot model.go CommandTypes).
const COMMAND_BUTTONS = ['door_open', 'door_close', 'scene_start_class', 'scene_end_class'] as const

export default function IotDeviceDetailPage() {
  const { t } = useTranslation('iot')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const id = useParams<{ id: string }>().id
  const canControl = can('iot:control')

  const [device, setDevice] = useState<IotDevice | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [loadError, setLoadError] = useState('')

  const load = useCallback(async () => {
    try {
      const d = await apiFetch<IotDevice>(`/api/iot/devices/${id}`, { token })
      setDevice(d)
      setNotFound(false)
      setLoadError('')
    } catch (err) {
      if ((err as { status?: number }).status === 404) setNotFound(true)
      else setLoadError((err as Error).message)
    }
  }, [id, token])

  useEffect(() => {
    void load()
  }, [load])

  // Live status: the backend broadcasts device.updated on every state report.
  // The frame is a trimmed view, so refresh the full record for this device.
  useEffect(() => {
    if (!token) return
    const { promise, controller } = apiStream('/api/iot/stream', {
      method: 'GET',
      token,
      onEvent: (name, data) => {
        if (name === 'device.updated' && data && Number(data.id) === Number(id)) void load()
      },
    })
    promise.catch(() => {
      // Stream drops are non-fatal: the record reloads on the next visit.
    })
    return () => controller.abort()
  }, [token, id, load])

  const events = usePagedList<IotDeviceEvent>({
    path: `/api/iot/devices/${id}/events`,
    token,
  })

  // Known-command confirm dialog.
  const [commandTarget, setCommandTarget] = useState<string | null>(null)
  const [commandSending, setCommandSending] = useState(false)
  const [commandError, setCommandError] = useState('')
  const [commandSent, setCommandSent] = useState('')

  // Custom (JSON) command dialog.
  const [customOpen, setCustomOpen] = useState(false)
  const [customForm, setCustomForm] = useState({ type: '', payload: '' })
  const [customError, setCustomError] = useState('')
  const [customSending, setCustomSending] = useState(false)

  const [showRaw, setShowRaw] = useState(false)

  const sendCommand = async (type: string, payload?: Record<string, unknown>) => {
    if (!device) return
    try {
      setCommandSending(true)
      setCommandError('')
      const body: { type: string; payload?: Record<string, unknown> } = { type }
      if (payload !== undefined) body.payload = payload
      const cmd = await apiFetch<{ status: string }>(`/api/iot/devices/${device.id}/commands`, {
        method: 'POST',
        token,
        body,
      })
      setCommandSent(t('command.sent', { status: t('status.' + cmd.status, { defaultValue: cmd.status }) }))
      setCommandTarget(null)
      setCustomOpen(false)
    } catch (err) {
      setCommandError((err as Error).message)
    } finally {
      setCommandSending(false)
    }
  }

  const handleCustomSend = async () => {
    if (!customForm.type.trim()) return setCustomError(t('command.typeRequired'))
    let payload: Record<string, unknown> | undefined
    if (customForm.payload.trim()) {
      try {
        const parsed: unknown = JSON.parse(customForm.payload)
        if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
          return setCustomError(t('command.invalidPayload'))
        }
        payload = parsed as Record<string, unknown>
      } catch {
        return setCustomError(t('command.invalidPayload'))
      }
    }
    await sendCommand(customForm.type.trim(), payload)
  }

  if (notFound) {
    return (
      <Grid fullWidth className="classrooms-page">
        <Column sm={4} md={8} lg={16}>
          <InlineNotification kind="error" title={t('error.load')} subtitle={t('detail.notFound')} lowContrast hideCloseButton />
          <Button kind="ghost" onClick={() => navigate('/iot')}>
            {t('breadcrumb.list')}
          </Button>
        </Column>
      </Grid>
    )
  }

  const attrs: Record<string, unknown> =
    device && device.state ? (device.state as Record<string, unknown>) : {}
  const attrEntries = Object.entries(attrs)

  const deviceName = device?.name || device?.externalId || `#${id}`

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
          <BreadcrumbItem
            href="/iot"
            onClick={(e) => {
              e.preventDefault()
              navigate('/iot')
            }}
          >
            {t('breadcrumb.list')}
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>{deviceName}</BreadcrumbItem>
        </Breadcrumb>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <h1 className="classrooms-page__heading">{deviceName}</h1>
          {device && (
            <Tag type={STATUS_KIND[device.status] ?? 'gray'} size="sm">
              {t('status.' + device.status, { defaultValue: device.status })}
            </Tag>
          )}
        </div>
        <p className="classrooms-page__subtitle">
          {device
            ? `${device.externalId} · ${device.category} · ${t('field.sourceId')}: ${device.sourceId}`
            : ''}
        </p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loadError && (
          <InlineNotification kind="error" title={t('error.load')} subtitle={loadError} lowContrast hideCloseButton />
        )}
        {device?.status === 'pending' && (
          <InlineNotification
            kind="info"
            title={t('status.pending')}
            subtitle={t('detail.pendingNotice')}
            lowContrast
            hideCloseButton
            className="classrooms-page__notice"
          />
        )}
        {commandSent && (
          <InlineNotification
            kind="success"
            title={t('detail.commands')}
            subtitle={commandSent}
            lowContrast
            hideCloseButton
            className="classrooms-page__notice"
          />
        )}
        {commandError && (
          <InlineNotification kind="error" title={t('error.action')} subtitle={commandError} lowContrast hideCloseButton className="classrooms-page__notice" />
        )}
      </Column>

      {/* Commands — issuing one is a physical-world action gated by
          iot:control on the server; manage alone never issues any. */}
      {device && device.status !== 'pending' && canControl && (
        <Column sm={4} md={8} lg={16}>
          <h3>{t('detail.commands')}</h3>
          <div className="classrooms-page__actions">
            {COMMAND_BUTTONS.map((type) => (
              <Button
                key={type}
                kind={type === 'door_open' ? 'danger--ghost' : 'ghost'}
                size="sm"
                onClick={() => {
                  setCommandError('')
                  setCommandTarget(type)
                }}
              >
                {t('command.' + type)}
              </Button>
            ))}
            <Button
              kind="ghost"
              size="sm"
              onClick={() => {
                setCustomError('')
                setCustomForm({ type: '', payload: '' })
                setCustomOpen(true)
              }}
            >
              {t('command.genericTitle')}
            </Button>
          </div>
        </Column>
      )}

      {/* Latest attributes */}
      <Column sm={4} md={8} lg={16}>
        <h3>{t('detail.attrs')}</h3>
        <Table size="sm">
          <TableHead>
            <TableRow>
              <TableHeader>#</TableHeader>
              <TableHeader>{t('detail.attrs')}</TableHeader>
            </TableRow>
          </TableHead>
          <TableBody>
            {attrEntries.length === 0 ? (
              <TableRow>
                <TableCell colSpan={2}>{t('detail.attrsEmpty')}</TableCell>
              </TableRow>
            ) : (
              attrEntries.map(([k, v]) => (
                <TableRow key={k}>
                  <TableCell>{k}</TableCell>
                  <TableCell>{typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        {attrEntries.length > 0 && (
          <>
            <Button kind="ghost" size="sm" onClick={() => setShowRaw((v) => !v)}>
              {showRaw ? t('detail.hideRaw') : t('detail.showRaw')}
            </Button>
            {showRaw && <pre style={{ overflowX: 'auto' }}>{JSON.stringify(device?.state, null, 2)}</pre>}
          </>
        )}
      </Column>

      {/* Events */}
      <Column sm={4} md={8} lg={16}>
        <h3>{t('detail.events')}</h3>
        <Table size="sm">
          <TableHead>
            <TableRow>
              <TableHeader>{t('field.type')}</TableHeader>
              <TableHeader>{t('field.payload')}</TableHeader>
              <TableHeader>{t('field.occurredAt')}</TableHeader>
              <TableHeader>{t('field.receivedAt')}</TableHeader>
            </TableRow>
          </TableHead>
          <TableBody>
            {events.loading ? (
              <TableRow>
                <TableCell colSpan={4}>{t('empty.loading')}</TableCell>
              </TableRow>
            ) : events.items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={4}>{t('empty.events')}</TableCell>
              </TableRow>
            ) : (
              events.items.map((e) => (
                <TableRow key={e.id}>
                  <TableCell>{e.type}</TableCell>
                  <TableCell>{e.payload ? JSON.stringify(e.payload) : '—'}</TableCell>
                  <TableCell>{formatDateTime(e.occurredAt)}</TableCell>
                  <TableCell>{formatDateTime(e.receivedAt)}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        <ListPagination
          page={events.page}
          pageSize={events.pageSize}
          totalItems={events.total}
          onPageChange={events.setPage}
          onPageSizeChange={events.setPageSize}
        />
      </Column>

      {/* Known-command confirm dialog */}
      <Modal
        open={Boolean(commandTarget)}
        modalHeading={t('modal.command')}
        primaryButtonText={t('modal.commandSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCommandTarget(null)}
        onRequestSubmit={() => commandTarget && void sendCommand(commandTarget)}
        primaryButtonDisabled={commandSending}
      >
        <p className="classrooms-page__confirm-text">
          {t('modal.commandLine', {
            device: deviceName,
            command: commandTarget ? t('command.' + commandTarget) : '',
          })}
        </p>
        {commandError && <InlineNotification kind="error" title={t('error.action')} subtitle={commandError} lowContrast hideCloseButton />}
      </Modal>

      {/* Custom-command dialog */}
      <Modal
        open={customOpen}
        modalHeading={t('command.genericTitle')}
        primaryButtonText={t('command.send')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCustomOpen(false)}
        onRequestSubmit={handleCustomSend}
        primaryButtonDisabled={customSending}
      >
        <div className="classrooms-page__form">
          <TextInput
            id="iot-cmd-type"
            labelText={t('command.typeLabel')}
            placeholder={t('command.typePlaceholder')}
            value={customForm.type}
            onChange={(e) => setCustomForm({ ...customForm, type: e.target.value })}
          />
          <TextArea
            id="iot-cmd-payload"
            labelText={t('command.payloadLabel')}
            placeholder={t('command.payloadPlaceholder')}
            value={customForm.payload}
            onChange={(e) => setCustomForm({ ...customForm, payload: e.target.value })}
            rows={4}
          />
          {customError && <InlineNotification kind="error" title={t('error.action')} subtitle={customError} lowContrast hideCloseButton />}
        </div>
      </Modal>
    </Grid>
  )
}
