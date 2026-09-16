import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  Grid,
  InlineNotification,
  Modal,
  Slider,
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

// The command types the console renders as dedicated buttons; they are part
// of the backend command vocabulary (internal/iot model.go KnownCommandType).
const COMMAND_BUTTONS = ['door_open', 'door_close', 'scene_start_class', 'scene_end_class'] as const

// Classroom-node devices (spec §11) report one entity per room with a
// functions namespace: peripherals are addressed via payload.function, so
// function commands live on the function cards and only node-level commands
// (scenes) stay as plain buttons.
const NODE_COMMAND_BUTTONS = ['scene_start_class', 'scene_end_class'] as const

type NodeFunction = {
  kind?: string
  state?: string
  locked?: boolean
  power_w?: number
  electric?: number
  run_seconds?: number
}

const isNodeDevice = (attrs: Record<string, unknown>): boolean =>
  Boolean(attrs.functions) && typeof attrs.functions === 'object'

const nodeFunctions = (attrs: Record<string, unknown>): [string, NodeFunction][] =>
  Object.entries((attrs.functions ?? {}) as Record<string, unknown>).map(([key, value]) => [
    key,
    value && typeof value === 'object' ? (value as NodeFunction) : { state: String(value) },
  ])

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

  // Node-model volume, optimistic like a native control panel: the slider
  // echoes the operator's value locally and keeps it until the DEVICE
  // confirms — sendCommand's success only confirms broker delivery, not that
  // the controller applied and reported the value. volumePending drives the
  // badge; volumeDraftRef backs the reconcile effect; volumeTimerRef arms a
  // 30 s fallback (the command TTL) that reverts to the last device-reported
  // value when no confirmation arrives.
  const [volumeDraft, setVolumeDraft] = useState<number | null>(null)
  const [volumePending, setVolumePending] = useState(false)
  const volumeDraftRef = useRef<number | null>(null)
  const volumeTimerRef = useRef<number | null>(null)

  // Same-route navigation (param change) does not remount this page — the
  // optimistic volume state must not leak from one classroom's node into the
  // next. The cleanup also runs on unmount, dropping the confirmation timer.
  useEffect(() => {
    return () => {
      if (volumeTimerRef.current !== null) window.clearTimeout(volumeTimerRef.current)
      volumeTimerRef.current = null
      volumeDraftRef.current = null
      setVolumeDraft(null)
      setVolumePending(false)
    }
  }, [id])

  // Reconcile the optimistic volume: the only trustworthy confirmation is a
  // state report (the panel channel has no sync ack — spec §11 notes the
  // controller confirms via readback). A report matching the draft resolves
  // the pending state; a differing report (another operator's write) is left
  // to the timeout, which falls back to the device-reported value.
  useEffect(() => {
    if (!volumePending) return
    const reported = (device?.state ?? null) as Record<string, unknown> | null
    const confirmed = typeof reported?.volume === 'number' ? reported.volume : null
    if (confirmed !== null && volumeDraftRef.current !== null && confirmed === volumeDraftRef.current) {
      if (volumeTimerRef.current !== null) {
        window.clearTimeout(volumeTimerRef.current)
        volumeTimerRef.current = null
      }
      volumeDraftRef.current = null
      setVolumeDraft(null)
      setVolumePending(false)
    }
  }, [device, volumePending])

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

  // Known-command confirm dialog. Node-model function cards pass a payload
  // (payload.function) alongside the type.
  const [commandTarget, setCommandTarget] = useState<{ type: string; payload?: Record<string, unknown> } | null>(null)
  const [commandSending, setCommandSending] = useState(false)
  const [commandError, setCommandError] = useState('')
  const [commandSent, setCommandSent] = useState('')

  // Custom (JSON) command dialog.
  const [customOpen, setCustomOpen] = useState(false)
  const [customForm, setCustomForm] = useState({ type: '', payload: '' })
  const [customError, setCustomError] = useState('')
  const [customSending, setCustomSending] = useState(false)

  const [showRaw, setShowRaw] = useState(false)

  const sendCommand = async (type: string, payload?: Record<string, unknown>): Promise<boolean> => {
    if (!device) return false
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
      return true
    } catch (err) {
      setCommandError((err as Error).message)
      return false
    } finally {
      setCommandSending(false)
    }
  }

  // Optimistic volume: the draft shows immediately (local echo, like the
  // native panel) and stays until the device confirms via a state report
  // (reconcile effect) or the 30 s TTL fallback reverts it. A REST-level
  // failure (not queued / not delivered) keeps the draft for retry.
  const sendVolume = async (value: number) => {
    volumeDraftRef.current = value
    setVolumeDraft(value)
    setVolumePending(true)
    const ok = await sendCommand('volume_set', { value })
    if (!ok) {
      setVolumePending(false)
      return
    }
    if (volumeTimerRef.current !== null) window.clearTimeout(volumeTimerRef.current)
    volumeTimerRef.current = window.setTimeout(() => {
      volumeTimerRef.current = null
      volumeDraftRef.current = null
      setVolumeDraft(null)
      setVolumePending(false)
      setCommandError(t('node.volumeTimeout'))
    }, 30_000)
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

  // Function-card state (spec §11): door/relay states render as human badges,
  // everything else keeps the device-reported raw value — the console never
  // invents semantics the node did not declare.
  const renderFunctionState = (f: NodeFunction) => {
    if (f.kind === 'door' && f.state === '1') return <Tag type="green" size="sm">{t('node.doorClosed')}</Tag>
    if (f.kind === 'door' && f.state === '0') return <Tag type="red" size="sm">{t('node.doorOpened')}</Tag>
    if (f.kind === 'relay') {
      return <Tag type={f.state === '1' ? 'green' : 'gray'} size="sm">{f.state === '1' ? t('node.on') : t('node.off')}</Tag>
    }
    if (f.state !== undefined) return <Tag size="sm">{f.state}</Tag>
    return <Tag type="gray" size="sm">—</Tag>
  }

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
          {/* Node model (spec §11): the controller link is a node attr, distinct
              from registry presence (which is the gateway's uplink). */}
          {isNodeDevice(attrs) && typeof attrs.online === 'boolean' && (
            <Tag type={attrs.online ? 'green' : 'red'} size="sm">
              {t('node.online')}: {attrs.online ? t('node.onlineOn') : t('node.onlineOff')}
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
          iot:control on the server; manage alone never issues any. Node-model
          devices only expose node-level buttons here (scenes); function
          commands live on the function cards below. */}
      {device && device.status !== 'pending' && canControl && (
        <Column sm={4} md={8} lg={16}>
          <h3>{t('detail.commands')}</h3>
          <div className="classrooms-page__actions">
            {(isNodeDevice(attrs) ? NODE_COMMAND_BUTTONS : COMMAND_BUTTONS).map((type) => (
              <Button
                key={type}
                kind={type === 'door_open' ? 'danger--ghost' : 'ghost'}
                size="sm"
                onClick={() => {
                  setCommandError('')
                  setCommandTarget({ type })
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

      {/* Latest attributes / classroom function cards (spec §11) */}
      <Column sm={4} md={8} lg={16}>
        <h3>{isNodeDevice(attrs) ? t('node.functions') : t('detail.attrs')}</h3>
        {isNodeDevice(attrs) ? (
          <div className="iot-function-grid">
            {typeof attrs.volume === 'number' && (
              <div className="iot-function-card">
                <div className="iot-function-card__head">
                  <span className="iot-function-card__title">{t('node.volume')}</span>
                  <Tag size="sm" type={volumePending ? 'blue' : undefined}>
                    {volumePending
                      ? `${volumeDraft ?? String(attrs.volume)} · ${t('node.volumePending')}`
                      : String(attrs.volume)}
                  </Tag>
                </div>
                {canControl && (
                  <div className="iot-function-card__actions">
                    <Slider
                      id="iot-node-volume"
                      labelText={t('node.volume')}
                      min={0}
                      max={100}
                      step={1}
                      value={volumeDraft ?? (attrs.volume as number)}
                      onChange={(data: { value: number }) => setVolumeDraft(data.value)}
                      hideTextInput
                    />
                    <Button
                      kind="ghost"
                      size="sm"
                      onClick={() => void sendVolume(volumeDraft ?? (attrs.volume as number))}
                    >
                      {t('command.volume_set')}
                    </Button>
                  </div>
                )}
              </div>
            )}
            {nodeFunctions(attrs).map(([key, f]) => (
              <div className="iot-function-card" key={key}>
                <div className="iot-function-card__head">
                  <span className="iot-function-card__title">{key}</span>
                  <Tag size="sm" type="cool-gray">
                    {t('node.kind.' + (f.kind ?? 'generic'), { defaultValue: f.kind ?? 'generic' })}
                  </Tag>
                </div>
                <div className="iot-function-card__body">
                  {renderFunctionState(f)}
                  {typeof f.power_w === 'number' && (
                    <span className="iot-function-card__meter">{t('node.powerWatts', { watts: Math.round(f.power_w * 10) / 10 })}</span>
                  )}
                  {typeof f.electric === 'number' && (
                    <span className="iot-function-card__meter">{t('node.powerWatts', { watts: Math.round(f.electric * 10) / 10 })}</span>
                  )}
                  {typeof f.run_seconds === 'number' && (
                    <span className="iot-function-card__meter">{t('node.runtimeHours', { hours: Math.round(f.run_seconds / 360) / 10 })}</span>
                  )}
                </div>
                {canControl && f.kind === 'door' && (
                  <div className="iot-function-card__actions">
                    <Button
                      kind="danger--ghost"
                      size="sm"
                      onClick={() => {
                        setCommandError('')
                        setCommandTarget({ type: 'door_open', payload: { function: key } })
                      }}
                    >
                      {t('command.door_open')}
                    </Button>
                    <Button
                      kind="ghost"
                      size="sm"
                      onClick={() => {
                        setCommandError('')
                        setCommandTarget({ type: 'door_close', payload: { function: key } })
                      }}
                    >
                      {t('command.door_close')}
                    </Button>
                  </div>
                )}
                {canControl && f.kind === 'relay' && (
                  <div className="iot-function-card__actions">
                    <Button
                      kind="ghost"
                      size="sm"
                      onClick={() => {
                        setCommandError('')
                        setCommandTarget({ type: 'device_on', payload: { function: key } })
                      }}
                    >
                      {t('node.on')}
                    </Button>
                    <Button
                      kind="ghost"
                      size="sm"
                      onClick={() => {
                        setCommandError('')
                        setCommandTarget({ type: 'device_off', payload: { function: key } })
                      }}
                    >
                      {t('node.off')}
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        ) : (
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
        )}
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
                  <TableCell>
                    {e.type}
                    {typeof e.payload?.function === 'string' ? ` · ${e.payload.function}` : ''}
                  </TableCell>
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
        onRequestSubmit={() => commandTarget && void sendCommand(commandTarget.type, commandTarget.payload)}
        primaryButtonDisabled={commandSending}
      >
        <p className="classrooms-page__confirm-text">
          {t('modal.commandLine', {
            device: deviceName,
            command: commandTarget ? t('command.' + commandTarget.type) : '',
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
