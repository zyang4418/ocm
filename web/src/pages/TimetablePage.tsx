import { useCallback, useEffect, useState } from 'react'
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
} from '@carbon/react'
import { ChevronLeft, ChevronRight, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ExportButton from '../components/ExportButton'
import TimetableGrid from '../components/TimetableGrid'
import { addDays, fmt, mondayOf } from '../utils/date'
import type { Classroom, OfferingView, Paged, SessionInput, SessionView, TimetableDay } from '../types/api'

// Cell modal state: which grid cell is open, with its existing session (if
// the cell is occupied).
interface CellModal {
  date: string
  periodIndex: number
  session?: SessionView
}

export default function TimetablePage() {
  const { t } = useTranslation('timetable')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('course:manage')

  const [classrooms, setClassrooms] = useState<Classroom[]>([])
  const [offerings, setOfferings] = useState<OfferingView[]>([])
  const [classroomId, setClassroomId] = useState('')
  const [weekStart, setWeekStart] = useState(() => mondayOf(new Date()))
  const [days, setDays] = useState<TimetableDay[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [modal, setModal] = useState<CellModal | null>(null)
  const [form, setForm] = useState({ offeringId: '', classroomId: '', date: '', periodStart: '', periodEnd: '', note: '' })
  const [modalError, setModalError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    // Dropdowns need (near-)full lists; pull the maximum page and unwrap the
    // pagination envelope.
    Promise.all([
      apiFetch<Paged<Classroom>>('/api/classrooms?page_size=500', { token }),
      apiFetch<Paged<OfferingView>>('/api/offerings?page_size=500', { token }),
    ])
      .then(([cls, offs]) => {
        setClassrooms(Array.isArray(cls?.items) ? cls.items : [])
        setOfferings(Array.isArray(offs?.items) ? offs.items : [])
      })
      .catch((err: Error) => setError(err.message))
  }, [token])

  const from = fmt(weekStart)
  const to = fmt(addDays(weekStart, 6))

  const fetchTimetable = useCallback(async () => {
    if (!classroomId) return
    try {
      setLoading(true)
      setError('')
      const data = await apiFetch<TimetableDay[]>(`/api/timetable?classroom_id=${classroomId}&from=${from}&to=${to}`, { token })
      setDays(Array.isArray(data) ? data : [])
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [classroomId, from, to, token])

  useEffect(() => {
    fetchTimetable()
  }, [fetchTimetable])

  const openCell = (date: string, periodIndex: number, session?: SessionView) => {
    setModal({ date, periodIndex, session })
    setForm({
      offeringId: session ? String(session.offeringId) : '',
      classroomId: String(classroomId),
      date,
      periodStart: String(session ? session.periodStart : periodIndex),
      periodEnd: String(session ? session.periodEnd : periodIndex),
      note: session ? session.note : '',
    })
    setModalError('')
  }

  const submit = async () => {
    if (!form.offeringId) {
      setModalError(t('validation.selectOffering'))
      return
    }
    const periodStart = Number(form.periodStart)
    const periodEnd = form.periodEnd ? Number(form.periodEnd) : periodStart
    if (!periodStart || periodStart < 1 || periodEnd < periodStart) {
      setModalError(t('validation.periodRange'))
      return
    }
    const body: SessionInput = {
      offeringId: Number(form.offeringId),
      classroomId: Number(form.classroomId),
      date: form.date,
      periodStart,
      periodEnd,
      note: form.note.trim(),
    }
    try {
      setSaving(true)
      setModalError('')
      if (modal?.session) {
        await apiFetch(`/api/sessions/${modal.session.id}`, { method: 'PUT', token, body })
      } else {
        await apiFetch('/api/sessions', { method: 'POST', token, body })
      }
      setModal(null)
      await fetchTimetable()
    } catch (err) {
      setModalError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    if (!modal?.session) return
    try {
      setSaving(true)
      setModalError('')
      await apiFetch(`/api/sessions/${modal.session.id}`, { method: 'DELETE', token })
      setModal(null)
      await fetchTimetable()
    } catch (err) {
      setModalError((err as Error).message)
    } finally {
      setSaving(false)
    }
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
      </Column>

      <Column sm={4} md={8} lg={16}>
        <div className="timetable__controls">
          <Select
            id="tt-classroom"
            labelText={t('filter.classroom')}
            value={classroomId}
            onChange={(e) => setClassroomId(e.target.value)}
            className="timetable__select"
          >
            <SelectItem value="" text={t('filter.selectClassroom')} />
            {classrooms.map((c) => (
              <SelectItem
                key={c.id}
                value={String(c.id)}
                text={c.building ? t('filter.classroomOption', { name: c.name, building: c.building }) : c.name}
              />
            ))}
          </Select>
          <div className="timetable__week">
            <Button kind="ghost" size="sm" hasIconOnly renderIcon={ChevronLeft} iconDescription={t('weekNav.prev')} onClick={() => setWeekStart(addDays(weekStart, -7))} />
            <span className="timetable__week-label">
              {from} ~ {to}
            </span>
            <Button kind="ghost" size="sm" hasIconOnly renderIcon={ChevronRight} iconDescription={t('weekNav.next')} onClick={() => setWeekStart(addDays(weekStart, 7))} />
          </div>
          <ExportButton
            path={`/api/timetable/export?classroom_id=${classroomId}&from=${from}&to=${to}`}
            fallbackName={t('export.filename')}
            label={t('export.label')}
            onError={setError}
            disabled={!classroomId}
          />
        </div>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {!classroomId ? (
          <p>{t('empty.selectClassroom')}</p>
        ) : loading ? (
          <p>{t('empty.loading')}</p>
        ) : days.length === 0 ? (
          <p>{t('empty.none')}</p>
        ) : (
          <TimetableGrid days={days} interactive={canManage} onCellClick={openCell} />
        )}
      </Column>

      {/* Session add/edit modal */}
      <Modal
        open={Boolean(modal)}
        modalHeading={modal?.session ? t('modal.edit') : t('modal.create')}
        primaryButtonText={t('modal.submit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setModal(null)}
        onRequestSubmit={submit}
        primaryButtonDisabled={saving}
      >
        <div className="courses-page__form">
          <Select id="s-offering" labelText={t('form.offering')} value={form.offeringId} onChange={(e) => setForm({ ...form, offeringId: e.target.value })}>
            <SelectItem value="" text={t('form.selectOffering')} />
            {offerings.map((o) => (
              <SelectItem
                key={o.id}
                value={String(o.id)}
                text={t('form.offeringOption', { catalogName: o.catalogName, teachingClassName: o.teachingClassName, semester: o.semester })}
              />
            ))}
          </Select>
          <Select id="s-classroom" labelText={t('form.classroom')} value={form.classroomId} onChange={(e) => setForm({ ...form, classroomId: e.target.value })}>
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={c.name} />
            ))}
          </Select>
          <TextInput id="s-date" type="date" labelText={t('form.date')} value={form.date} onChange={(e) => setForm({ ...form, date: e.target.value })} />
          <TextInput id="s-period-start" type="number" labelText={t('form.periodStart')} min="1" value={form.periodStart} onChange={(e) => setForm({ ...form, periodStart: e.target.value })} />
          <TextInput id="s-period-end" type="number" labelText={t('form.periodEnd')} min="1" value={form.periodEnd} onChange={(e) => setForm({ ...form, periodEnd: e.target.value })} />
          <TextInput id="s-note" labelText={t('form.note')} value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} />
          {modalError && (
            <InlineNotification kind="error" title={t('error.save')} subtitle={modalError} lowContrast hideCloseButton />
          )}
        </div>
        {modal?.session && canManage && (
          <Button kind="danger" size="sm" renderIcon={TrashCan} onClick={remove} disabled={saving} className="timetable__delete">
            {t('modal.remove')}
          </Button>
        )}
      </Modal>
    </Grid>
  )
}
