import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, InlineNotification, Modal, Select, SelectItem } from '@carbon/react'
import { ChevronLeft, ChevronRight } from '@carbon/icons-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import TimetableGrid, { type TimetableSelection } from './TimetableGrid'
import { addDays, fmt, mondayOf, parseDate } from '../utils/date'
import type { Classroom, SessionView, TimetableDay } from '../types/api'

/** What the picker reports back: the selected classroom and period range. */
export interface TimetablePick {
  classroomId: string
  date: string
  periodStart: number
  periodEnd: number
}

export interface TimetablePickerModalProps {
  open: boolean
  classrooms: Classroom[]
  /** Preselected classroom (the session form's current value); falls back to the first classroom. */
  initialClassroomId?: string
  /** Week anchor (the session form's current date); defaults to the current week. */
  initialDate?: string
  /**
   * The session being edited: its cells count as free for selection (the moved
   * session does not conflict with itself) and render highlighted in the grid.
   */
  targetSession?: SessionView | null
  onClose: () => void
  onPick: (pick: TimetablePick) => void
}

// Cell key in the picked-cells set: `${date}#${period}`.
const cellKey = (date: string, period: number) => `${date}#${period}`

/** The picked cells as (date, period) pairs, sorted for run grouping. */
const pickedCells = (picked: Set<string>) =>
  Array.from(picked, (k) => {
    const i = k.indexOf('#')
    return { date: k.slice(0, i), period: Number(k.slice(i + 1)) }
  }).sort((a, b) => (a.date === b.date ? a.period - b.period : a.date < b.date ? -1 : 1))

/** Whether the sorted cells form exactly one contiguous run on one day. */
const isOneRun = (cells: { date: string; period: number }[]) => {
  const first = cells.at(0)
  const last = cells.at(-1)
  return (
    first != null &&
    last != null &&
    cells.every((c) => c.date === first.date) &&
    last.period - first.period === cells.length - 1
  )
}

/** Keeps only the first contiguous run (earliest date, then lowest period). */
const firstRunOf = (picked: Set<string>) => {
  const run: { date: string; period: number }[] = []
  for (const c of pickedCells(picked)) {
    const prev = run[run.length - 1]
    if (prev && (prev.date !== c.date || c.period !== prev.period + 1)) break
    run.push(c)
  }
  return new Set(run.map((c) => cellKey(c.date, c.period)))
}

// TimetablePickerModal is the classroom-timetable browser opened from the
// session form: switch classrooms freely, page through weeks, and check the
// wanted periods cell by cell. The picked cells always form ONE contiguous run
// on one day - the one session the form will backfill. The 回填 button then
// backfills the selection into the form. It performs no writes - the form
// stays the single place a change is confirmed (and where the backend's
// conflict check still runs on submit).
export default function TimetablePickerModal({
  open,
  classrooms,
  initialClassroomId,
  initialDate,
  targetSession,
  onClose,
  onPick,
}: TimetablePickerModalProps) {
  const { t } = useTranslation('timetable')
  const { token } = useAuth()
  const [classroomId, setClassroomId] = useState('')
  const [weekStart, setWeekStart] = useState(() => mondayOf(new Date()))
  const [days, setDays] = useState<TimetableDay[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  // The checked cells (key `${date}#${period}`); always one contiguous run on
  // one day - handleCellClick enforces that while toggling.
  const [picked, setPicked] = useState<Set<string>>(new Set())

  // Re-anchor on every open: the form's classroom and date may have changed
  // since the last visit. Falls back to the first classroom when the form has
  // none yet. `anchored` gates the fetch below until this reset has landed, so
  // a reopen never flashes the previously browsed week before correcting it.
  const [anchored, setAnchored] = useState(false)
  useEffect(() => {
    if (!open) {
      setAnchored(false)
      return
    }
    setClassroomId(initialClassroomId || (classrooms[0] ? String(classrooms[0].id) : ''))
    const anchor = parseDate(initialDate ?? '')
    setWeekStart(anchor ? mondayOf(anchor) : mondayOf(new Date()))
    setPicked(new Set())
    setAnchored(true)
  }, [open, initialClassroomId, initialDate, classrooms])

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
    if (open && anchored) fetchTimetable()
  }, [open, anchored, fetchTimetable])

  // Picked cells refer to one classroom/week; browsing away invalidates them.
  useEffect(() => {
    setPicked(new Set())
  }, [classroomId, from, to])

  // A period is selectable when the slot exists, is not reserved by a booking,
  // and is either free or held by the session being moved (which cannot
  // conflict with itself). The per-period slot lookup works under merged cells
  // too: the backend fills every covered period of a multi-period occupant.
  const pickable = useCallback(
    (date: string, period: number) => {
      const slot = days.find((x) => x.date === date)?.slots.find((s) => s.periodIndex === period)
      if (!slot) return false
      if (slot.booking) return false
      if (slot.session && slot.session.id !== targetSession?.id) return false
      return true
    },
    [days, targetSession],
  )

  // The run reported to the grid/form, derived from the picked cells (which
  // are one contiguous run by construction; min/max defensively).
  const selection = useMemo<TimetableSelection | null>(() => {
    const first = pickedCells(picked).at(0)
    if (!first) return null
    const periods = pickedCells(picked)
      .filter((c) => c.date === first.date)
      .map((c) => c.period)
    return { date: first.date, periodStart: Math.min(...periods), periodEnd: Math.max(...periods) }
  }, [picked])

  // Click-to-check, one cell at a time. Clicking a free cell adjacent to the
  // run extends it; a non-adjacent cell restarts the run there; a picked cell
  // unchecks (when that splits the run, the earlier part survives). The moved
  // session's own cell is one rowSpan-merged td, so its click toggles the whole
  // [periodStart, periodEnd] range as a unit.
  const handleCellClick = (date: string, period: number, session?: SessionView) => {
    if (!pickable(date, period)) return
    const targets = session
      ? Array.from({ length: session.periodEnd - session.periodStart + 1 }, (_, i) => session.periodStart + i)
      : [period]
    const allPicked = targets.every((p) => picked.has(cellKey(date, p)))
    if (allPicked) {
      // Uncheck: removing a middle cell/segment splits the run - keep the head.
      const next = new Set(picked)
      targets.forEach((p) => next.delete(cellKey(date, p)))
      setPicked(firstRunOf(next))
      return
    }
    const next = new Set(picked)
    targets.forEach((p) => next.add(cellKey(date, p)))
    setPicked(isOneRun(pickedCells(next)) ? next : new Set(targets.map((p) => cellKey(date, p))))
  }

  const apply = () => {
    if (!selection) return
    onPick({ classroomId, date: selection.date, periodStart: selection.periodStart, periodEnd: selection.periodEnd })
  }

  return (
    <Modal
      open={open}
      modalHeading={t('picker.title')}
      primaryButtonText={t('picker.apply')}
      primaryButtonDisabled={!selection || loading}
      secondaryButtonText={t('action.cancel', { ns: 'common' })}
      onRequestClose={onClose}
      onRequestSubmit={apply}
      size="lg"
      hasScrollingContent
      className="timetable-picker"
    >
      <p className="timetable-picker__hint">{t('picker.hint')}</p>
      <div className="timetable__controls">
        <Select
          id="tt-picker-classroom"
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
            {loading && <span className="timetable-picker__loading">{t('empty.loading')}</span>}
          </span>
          <Button kind="ghost" size="sm" hasIconOnly renderIcon={ChevronRight} iconDescription={t('weekNav.next')} onClick={() => setWeekStart(addDays(weekStart, 7))} />
        </div>
      </div>
      {error && <InlineNotification kind="error" title={t('error.load')} subtitle={error} lowContrast hideCloseButton />}
      {!classroomId ? (
        <p>{t('empty.selectClassroom')}</p>
      ) : days.length === 0 && loading ? (
        <p>{t('empty.loading')}</p>
      ) : days.length === 0 ? (
        <p>{t('empty.none')}</p>
      ) : (
        // While refetching (classroom/week switch) the previous grid stays
        // mounted instead of swapping to a loading line, so the modal does not
        // flicker; only the first load of an empty classroom shows the loader.
        <div className="timetable-picker__grid" aria-busy={loading || undefined}>
          <TimetableGrid
            days={days}
            interactive
            onCellClick={handleCellClick}
            selection={selection}
            highlightSessionId={targetSession?.id}
          />
        </div>
      )}
    </Modal>
  )
}
