import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { SessionView, TimetableDay, TimetableSlot } from '../types/api'

// One row of the period index (union of period indices across the week).
interface PeriodRow {
  periodIndex: number
  startTime: string
  endTime: string
}

/** The picked period range highlighted in the grid (picker selection mode). */
export interface TimetableSelection {
  date: string
  periodStart: number
  periodEnd: number
}

export interface TimetableGridProps {
  days: TimetableDay[]
  /** Clickable cells: the page's edit mode or the picker's selection mode. */
  interactive: boolean
  /**
   * Fired on free-cell and session-cell clicks. Booking cells never fire:
   * their slot cannot host a session (the backend rejects it with 409), so the
   * grid keeps them inert and only shows their tooltip.
   */
  onCellClick?: (date: string, periodIndex: number, session?: SessionView) => void
  /** Current picked range to highlight (picker mode). */
  selection?: TimetableSelection | null
  /**
   * The session being edited or moved (picker mode): its cells stay selectable
   * - the moved session does not conflict with itself - and render highlighted
   * so the user can see where it sits.
   */
  highlightSessionId?: number
}

// TimetableGrid renders the weekly classroom grid shared by the timetable page
// (edit mode) and the session-form picker (selection mode): days as columns,
// periods as rows, and multi-period occupants (course sessions and active
// bookings) each merged into one row-spanning cell starting at their start
// period. A slot carries at most one of the two (the backend conflict model
// guarantees it); sessions win defensively if data ever shows both.
export default function TimetableGrid({
  days,
  interactive,
  onCellClick,
  selection,
  highlightSessionId,
}: TimetableGridProps) {
  const { t } = useTranslation('timetable')
  const dayNames = t('dayNames', { returnObjects: true }) as string[]

  // union of period indices across the week (rows of the grid)
  const periods = useMemo<PeriodRow[]>(() => {
    const map = new Map<number, PeriodRow>()
    days.forEach((d) =>
      d.slots.forEach((s) => {
        if (!map.has(s.periodIndex)) map.set(s.periodIndex, { periodIndex: s.periodIndex, startTime: s.startTime, endTime: s.endTime })
      }),
    )
    return Array.from(map.values()).sort((a, b) => a.periodIndex - b.periodIndex)
  }, [days])

  const slotFor = (day: TimetableDay, periodIndex: number): TimetableSlot | undefined =>
    day.slots.find((s) => s.periodIndex === periodIndex)

  return (
    <div className="timetable__scroll">
      <table className="timetable__grid">
        <thead>
          <tr>
            <th className="timetable__corner">{t('corner')}</th>
            {days.map((d) => (
              <th key={d.date}>
                {dayNames[d.dayOfWeek - 1]}
                <span className="timetable__date">{d.date.slice(5)}</span>
                {d.regimeName && <span className="timetable__regime">{d.regimeName}</span>}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {periods.map((p) => (
            <tr key={p.periodIndex}>
              <td className="timetable__period">
                <span>{t('periodLabel.single', { period: p.periodIndex })}</span>
                <span className="timetable__time">{p.startTime}-{p.endTime}</span>
              </td>
              {days.map((d) => {
                const slot = slotFor(d, p.periodIndex)
                const session = slot?.session
                const booking = slot?.booking
                // 连上多节的占用（课次或预约）从起始节起合并为一个单元格
                //（rowSpan），被覆盖的后续节次不再渲染。课次优先于预约。
                const occupant = session ?? booking
                if (occupant && occupant.periodStart !== p.periodIndex) return null
                const span = occupant ? occupant.periodEnd - occupant.periodStart + 1 : 1
                const isCurrent = session != null && session.id === highlightSessionId
                // Picked-range highlight: free cells match when their own
                // period is inside the range; merged occupant cells (only the
                // moved session can be in a selection) when they overlap it.
                const inSelection =
                  selection != null &&
                  selection.date === d.date &&
                  (occupant
                    ? occupant.periodEnd >= selection.periodStart && occupant.periodStart <= selection.periodEnd
                    : p.periodIndex >= selection.periodStart && p.periodIndex <= selection.periodEnd)
                const classes = ['timetable__cell']
                if (session) classes.push('timetable__cell--filled')
                if (booking) classes.push('timetable__cell--booking')
                if (isCurrent) classes.push('timetable__cell--current')
                if (inSelection) classes.push('timetable__cell--selected')
                const fire = () => {
                  if (!interactive || booking) return
                  onCellClick?.(d.date, p.periodIndex, session)
                }
                return (
                  <td
                    key={d.date + '-' + p.periodIndex}
                    rowSpan={span}
                    className={classes.join(' ')}
                    onClick={fire}
                  >
                    {session ? (
                      <div
                        className="timetable__session"
                        title={[session.courseName, session.teachingClassName, session.teacher]
                          .filter(Boolean)
                          .join('\n')}
                      >
                        <strong>{session.courseName}</strong>
                        {session.teachingClassName && <span>{session.teachingClassName}</span>}
                        {session.teacher && <span>{session.teacher}</span>}
                      </div>
                    ) : booking ? (
                      <div
                        className="timetable__booking"
                        title={[t('booking.label'), booking.purpose, booking.displayName || booking.username]
                          .filter(Boolean)
                          .join('\n')}
                      >
                        <strong>{t('booking.label')}</strong>
                        {booking.purpose && <span>{booking.purpose}</span>}
                        {(booking.displayName || booking.username) && (
                          <span>{booking.displayName || booking.username}</span>
                        )}
                      </div>
                    ) : interactive ? (
                      // Free cell affordance: ＋ invites a click, ✓ marks a
                      // checked cell (picker selection mode).
                      <span className={inSelection ? 'timetable__pick-check' : 'timetable__add'}>
                        {inSelection ? '✓' : '＋'}
                      </span>
                    ) : null}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
