import { useEffect, useState } from 'react'
import ListPagination from './ListPagination.jsx'
import { apiFetch } from '../auth/api.js'
import i18n from '../i18n/index.js'

// IMPORT_TYPES describes each business-table import: the xlsx header contract
// (order-independent, matched by name — kept literal as a backend contract),
// and the preview column keys used to render the dry-run rows. The column keys
// mirror the toPreviewMap() keys produced by each backend importer and double
// as i18n keys under imports.columns.<type>.<key>. Shared by ImportsPage and
// the SplitWizard so the contract lives in one place.
export const IMPORT_TYPES = {
  sessions: {
    schema: 'date, period_start, period_end, classroom, course, teaching_class, semester, note',
    columns: ['date', 'periodStart', 'periodEnd', 'classroom', 'course', 'teachingClass', 'semester', 'note'],
  },
  classrooms: {
    schema: 'name, building, capacity, type, floor, campus, status, description',
    columns: ['name', 'building', 'capacity', 'type', 'floor', 'campus', 'status', 'description'],
  },
  admin_classes: {
    schema: 'grade, name, note',
    columns: ['grade', 'name', 'note'],
  },
  teaching_classes: {
    schema: 'name, note, admin_grade, admin_name',
    columns: ['name', 'note', 'admin_classes'],
  },
  catalog: {
    schema: 'name, code, credits, total_hours, category, exam_type, description',
    columns: ['name', 'code', 'credits', 'totalHours', 'category', 'examType', 'description'],
  },
  offerings: {
    schema: 'course, teaching_class, semester, teacher, course_seq, teacher_id, teacher_title, college, max_students, requirement, weekly_hours, note',
    columns: ['course', 'teachingClass', 'semester', 'teacher', 'courseSeq', 'teacherId', 'teacherTitle', 'college', 'maxStudents', 'requirement', 'weeklyHours', 'note'],
  },
  regimes: {
    schema: 'regime_name, effective_month, effective_day, period_index, start_time, end_time',
    columns: ['name', 'effectiveMonth', 'effectiveDay', 'periods'],
  },
  bookings: {
    schema: 'classroom, username, date, period_start, period_end, status, purpose',
    columns: ['classroom', 'username', 'date', 'periodStart', 'periodEnd', 'status', 'purpose'],
  },
}

// formatCell renders a preview cell value. Arrays (teaching-class members,
// regime periods) are joined into a readable string; other values pass through.
export function formatCell(value) {
  if (Array.isArray(value)) {
    if (value.length === 0) return ''
    // Regime periods are objects {periodIndex, startTime, endTime}; teaching
    // class members are plain label strings. Detect by element type.
    const sep = (i18n.language || 'zh-CN').startsWith('zh') ? '，' : ', '
    if (typeof value[0] === 'object' && value[0] !== null) {
      return value
        .map((p) => `${p.periodIndex}(${p.startTime}-${p.endTime})`)
        .join(sep)
    }
    return value.join(sep)
  }
  if (value === null || value === undefined) return ''
  return String(value)
}

// ImportPreviewTable renders the dry-run rows for one job, fetching pages from
// the server (GET /api/imports/{id}/rows) so a multi-thousand-row preview is
// not shipped in full. `job` is the job metadata (status / counts) and carries
// neither rows nor the per-row error report; rows are fetched page-by-page
// below, and the error list is fetched on demand from
// GET /api/imports/{id}/errors (it can be several MB for a sessions job whose
// rows all fail, so it is not piggybacked on the polled job meta). `t` is the
// 'imports'-namespace translator from the host.
export default function ImportPreviewTable({ job, token, t }) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(100)
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [errors, setErrors] = useState([])
  const [errorsLoaded, setErrorsLoaded] = useState(false)

  const columns = (IMPORT_TYPES[job?.type]?.columns || [])
    .map((key) => ({ key, header: t('columns.' + job?.type + '.' + key, { defaultValue: key }) }))

  useEffect(() => {
    if (!job?.id) return undefined
    let cancelled = false
    setLoading(true)
    ;(async () => {
      try {
        const data = await apiFetch(`/api/imports/${job.id}/rows?page=${page}&pageSize=${pageSize}`, { token })
        if (cancelled) return
        const nextRows = Array.isArray(data?.rows) ? data.rows : []
        const nextTotal = data?.total ?? 0
        setRows(nextRows)
        setTotal(nextTotal)
        // If the current page is out of range (e.g. switched to a smaller job
        // while on page > 1, or a reanalyze shrank the preview), jump back to
        // page 1 so the operator isn't staring at an empty table.
        if (page > 1 && nextTotal > 0 && (page - 1) * pageSize >= nextTotal) {
          setPage(1)
        }
      } catch {
        if (!cancelled) {
          setRows([])
          setTotal(0)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => { cancelled = true }
    // Re-fetch when the job identity or its preview "version" changes (reanalyze
    // bumps startedAt/succeededRows; commit changes status), or on page change.
    // Polled meta during processing keeps these primitives stable, so polling
    // does not refetch rows.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [job?.id, job?.status, job?.succeededRows, job?.startedAt, page, pageSize])

  // Fetch the per-row error report on demand. error_report can be several MB
  // for a sessions job whose rows all fail, so the polled job meta no longer
  // carries it; fetch it here, refetched only when the preview "version"
  // changes (same primitives as the rows effect) — never on each meta poll.
  useEffect(() => {
    if (!job?.id) return undefined
    let cancelled = false
    ;(async () => {
      try {
        const data = await apiFetch(`/api/imports/${job.id}/errors`, { token })
        if (cancelled) return
        setErrors(Array.isArray(data?.errors) ? data.errors : [])
      } catch {
        if (!cancelled) setErrors([])
      } finally {
        if (!cancelled) setErrorsLoaded(true)
      }
    })()
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [job?.id, job?.status, job?.succeededRows, job?.startedAt])

  const lastPage = Math.max(1, Math.ceil(total / pageSize))
  const safePage = Math.min(page, lastPage)

  return (
    <>
      {loading && rows.length === 0 ? (
        <p className="imports-page__summary">{t('modal.loadingPreview')}</p>
      ) : rows.length > 0 ? (
        <div className="imports-page__rows">
          <table>
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c.key}>{c.header}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  {columns.map((c) => (
                    <td key={c.key}>{formatCell(r[c.key])}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {total > 0 && (
        <ListPagination
          page={safePage}
          pageSize={pageSize}
          totalItems={total}
          onPageChange={(p) => setPage(p)}
          onPageSizeChange={(s) => { setPageSize(s); setPage(1) }}
        />
      )}

      <ul className="imports-page__errors">
        {errors.map((e, i) => (
          <li key={i}>
            {e.row > 0 ? t('modal.errorRow', { row: e.row }) : ''}
            {e.error}
          </li>
        ))}
        {errorsLoaded && errors.length === 0 && <li>{t('modal.noErrors')}</li>}
      </ul>
    </>
  )
}
