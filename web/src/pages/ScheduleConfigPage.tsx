import { useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  Grid,
  InlineNotification,
  Modal,
  Select,
  SelectItem,
  TextInput,
  Tile,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ExportButton from '../components/ExportButton'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import type { PeriodInput, Regime, RegimeInput } from '../types/api'

// Regime form: the day field is a text input, so it holds a string while
// editing (converted on submit).
interface RegimeForm {
  name: string
  effectiveMonth: number
  effectiveDay: string
}

const emptyRegime: RegimeForm = { name: '', effectiveMonth: 5, effectiveDay: '1' }

// One editable period row in the periods modal: periodIndex stays a string
// while the number input is being edited (converted on submit).
interface PeriodRow {
  periodIndex: string
  startTime: string
  endTime: string
}

export default function ScheduleConfigPage() {
  const { t } = useTranslation('scheduleConfig')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('course:manage')

  const months = t('months', { returnObjects: true }) as string[]

  const list = usePagedList<Regime>({ path: '/api/schedule/regimes', token })
  const { loading } = list
  // Export errors are separate from the list fetch (the hook owns its error).
  const [exportError, setExportError] = useState('')
  const error = list.error || exportError

  const [regimeOpen, setRegimeOpen] = useState(false)
  const [regimeForm, setRegimeForm] = useState<RegimeForm>(emptyRegime)
  const [regimeEditId, setRegimeEditId] = useState<number | null>(null)
  const [regimeError, setRegimeError] = useState('')
  const [regimeSaving, setRegimeSaving] = useState(false)

  const [periodsTarget, setPeriodsTarget] = useState<Regime | null>(null) // regime being edited
  const [periodRows, setPeriodRows] = useState<PeriodRow[]>([])
  const [periodsError, setPeriodsError] = useState('')
  const [periodsSaving, setPeriodsSaving] = useState(false)

  const [delTarget, setDelTarget] = useState<Regime | null>(null)
  const [delError, setDelError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const openCreateRegime = () => {
    setRegimeForm(emptyRegime)
    setRegimeEditId(null)
    setRegimeError('')
    setRegimeOpen(true)
  }

  const openEditRegime = (r: Regime) => {
    setRegimeForm({ name: r.name, effectiveMonth: r.effectiveMonth, effectiveDay: String(r.effectiveDay) })
    setRegimeEditId(r.id)
    setRegimeError('')
    setRegimeOpen(true)
  }

  const submitRegime = async () => {
    if (!regimeForm.name.trim()) {
      setRegimeError(t('validation.nameRequired'))
      return
    }
    const body: RegimeInput = {
      name: regimeForm.name.trim(),
      effectiveMonth: Number(regimeForm.effectiveMonth),
      effectiveDay: Number(regimeForm.effectiveDay),
    }
    try {
      setRegimeSaving(true)
      setRegimeError('')
      if (regimeEditId) {
        await apiFetch(`/api/schedule/regimes/${regimeEditId}`, { method: 'PUT', token, body })
      } else {
        await apiFetch('/api/schedule/regimes', { method: 'POST', token, body })
      }
      setRegimeOpen(false)
      list.reload()
    } catch (err) {
      setRegimeError((err as Error).message)
    } finally {
      setRegimeSaving(false)
    }
  }

  const openEditPeriods = (r: Regime) => {
    setPeriodsTarget(r)
    setPeriodRows(
      r.periods && r.periods.length
        ? r.periods.map((p) => ({ periodIndex: String(p.periodIndex), startTime: p.startTime, endTime: p.endTime }))
        : [{ periodIndex: '1', startTime: '08:00', endTime: '08:45' }],
    )
    setPeriodsError('')
  }

  const submitPeriods = async () => {
    if (!periodsTarget) return
    try {
      setPeriodsSaving(true)
      setPeriodsError('')
      const periods: PeriodInput[] = periodRows.map((p) => ({
        periodIndex: Number(p.periodIndex),
        startTime: p.startTime,
        endTime: p.endTime,
      }))
      await apiFetch(`/api/schedule/regimes/${periodsTarget.id}/periods`, {
        method: 'PUT',
        token,
        body: { periods },
      })
      setPeriodsTarget(null)
      list.reload()
    } catch (err) {
      setPeriodsError((err as Error).message)
    } finally {
      setPeriodsSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!delTarget) return
    try {
      setDeleting(true)
      setDelError('')
      await apiFetch(`/api/schedule/regimes/${delTarget.id}`, { method: 'DELETE', token })
      setDelTarget(null)
      list.reload()
    } catch (err) {
      setDelError((err as Error).message)
    } finally {
      setDeleting(false)
    }
  }

  const updateRow = (i: number, field: keyof PeriodRow, value: string) => {
    // Explicit per-field mapping keeps the row type concrete (a computed
    // [field] key would widen the object to a Partial).
    setPeriodRows((rows) =>
      rows.map((row, j) => {
        if (j !== i) return row
        if (field === 'periodIndex') return { ...row, periodIndex: value }
        if (field === 'startTime') return { ...row, startTime: value }
        return { ...row, endTime: value }
      }),
    )
  }

  return (
    <Grid fullWidth className="courses-page">
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
          <BreadcrumbItem isCurrentPage>{t('breadcrumb.current')}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">{t('title')}</h1>
        <p className="courses-page__subtitle">{t('subtitle')}</p>
        {error && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
        {canManage && (
          <Button renderIcon={Add} size="sm" onClick={openCreateRegime} className="courses-page__add">
            {t('addButton')}
          </Button>
        )}
        <ExportButton
          path="/api/schedule/regimes/export"
          fallbackName="regimes.xlsx"
          onError={setExportError}
          className="courses-page__add"
        />
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loading ? (
          <p>{t('empty.loading')}</p>
        ) : list.items.length === 0 ? (
          <p>{t('empty.none')}</p>
        ) : (
          list.items.map((r) => (
            <Tile key={r.id} className="schedule-regime">
              <div className="schedule-regime__head">
                <div>
                  <strong>{r.name}</strong>
                  <span className="schedule-regime__date">
                    {' '}
                    {t('effective', { month: r.effectiveMonth, day: r.effectiveDay })}
                  </span>
                </div>
                {canManage && (
                  <div className="schedule-regime__actions">
                    <Button size="sm" kind="ghost" renderIcon={Edit} onClick={() => openEditRegime(r)}>
                      {t('action.edit')}
                    </Button>
                    <Button size="sm" kind="ghost" onClick={() => openEditPeriods(r)}>
                      {t('action.editPeriods')}
                    </Button>
                    <Button
                      size="sm"
                      kind="ghost"
                      hasIconOnly
                      renderIcon={TrashCan}
                      iconDescription={t('action.delete', { ns: 'common' })}
                      onClick={() => setDelTarget(r)}
                    />
                  </div>
                )}
              </div>
              {r.periods && r.periods.length > 0 ? (
                <table className="schedule-regime__periods">
                  <thead>
                    <tr>
                      <th>{t('periodForm.period')}</th>
                      <th>{t('periodForm.start')}</th>
                      <th>{t('periodForm.end')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {r.periods.map((p) => (
                      <tr key={p.id || p.periodIndex}>
                        <td>{t('periodLabel.single', { period: p.periodIndex })}</td>
                        <td>{p.startTime}</td>
                        <td>{p.endTime}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="schedule-regime__empty">{t('empty.noPeriods')}</p>
              )}
            </Tile>
          ))
        )}
        <ListPagination
          page={list.page}
          pageSize={list.pageSize}
          totalItems={list.total}
          onPageChange={list.setPage}
          onPageSizeChange={list.setPageSize}
        />
      </Column>

      {/* Regime create/edit modal */}
      <Modal
        open={regimeOpen}
        modalHeading={regimeEditId ? t('modal.regimeEdit') : t('modal.regimeCreate')}
        primaryButtonText={t('modal.regimeSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setRegimeOpen(false)}
        onRequestSubmit={submitRegime}
        primaryButtonDisabled={regimeSaving}
      >
        <div className="courses-page__form">
          <TextInput
            id="regime-name"
            labelText={t('regimeForm.name')}
            placeholder={t('regimeForm.namePlaceholder')}
            value={regimeForm.name}
            onChange={(e) => setRegimeForm({ ...regimeForm, name: e.target.value })}
          />
          <Select
            id="regime-month"
            labelText={t('regimeForm.month')}
            value={String(regimeForm.effectiveMonth)}
            onChange={(e) => setRegimeForm({ ...regimeForm, effectiveMonth: Number(e.target.value) })}
          >
            {months.map((m, i) => (
              <SelectItem key={i} value={String(i + 1)} text={m} />
            ))}
          </Select>
          <TextInput
            id="regime-day"
            type="number"
            labelText={t('regimeForm.day')}
            min="1"
            max="31"
            value={regimeForm.effectiveDay}
            onChange={(e) => setRegimeForm({ ...regimeForm, effectiveDay: e.target.value })}
          />
          {regimeError && (
            <InlineNotification kind="error" title={t('error.save')} subtitle={regimeError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Periods edit modal */}
      <Modal
        open={Boolean(periodsTarget)}
        modalHeading={t('modal.periodsTitle', { name: periodsTarget?.name ?? '' })}
        primaryButtonText={t('modal.periodsSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setPeriodsTarget(null)}
        onRequestSubmit={submitPeriods}
        primaryButtonDisabled={periodsSaving}
        size="lg"
      >
        <div className="courses-page__form">
          <p className="schedule-periods__hint">{t('periodsHint')}</p>
          {periodRows.map((row, i) => (
            <div key={i} className="schedule-periods__row">
              <TextInput
                id={`pi-${i}`}
                type="number"
                labelText={t('periodForm.period')}
                min="1"
                value={row.periodIndex}
                onChange={(e) => updateRow(i, 'periodIndex', e.target.value)}
              />
              <TextInput
                id={`st-${i}`}
                type="time"
                labelText={t('periodForm.start')}
                value={row.startTime}
                onChange={(e) => updateRow(i, 'startTime', e.target.value)}
              />
              <TextInput
                id={`et-${i}`}
                type="time"
                labelText={t('periodForm.end')}
                value={row.endTime}
                onChange={(e) => updateRow(i, 'endTime', e.target.value)}
              />
              <Button
                kind="ghost"
                size="sm"
                hasIconOnly
                renderIcon={TrashCan}
                iconDescription={t('periodForm.removePeriod')}
                onClick={() => setPeriodRows(periodRows.filter((_, j) => j !== i))}
              />
            </div>
          ))}
          <Button
            kind="ghost"
            size="sm"
            renderIcon={Add}
            onClick={() =>
              setPeriodRows([
                ...periodRows,
                { periodIndex: String(periodRows.length + 1), startTime: '08:00', endTime: '08:45' },
              ])
            }
          >
            {t('periodForm.addPeriod')}
          </Button>
          {periodsError && (
            <InlineNotification kind="error" title={t('error.save')} subtitle={periodsError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Delete modal */}
      <Modal
        danger
        open={Boolean(delTarget)}
        modalHeading={t('modal.delete')}
        primaryButtonText={t('modal.deleteSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setDelTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="courses-page__confirm-text">
          {t('deleteConfirm', { name: delTarget?.name })}
        </p>
        {delError && (
          <InlineNotification kind="error" title={t('error.delete')} subtitle={delError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
