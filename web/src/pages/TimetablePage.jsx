import { useCallback, useEffect, useMemo, useState } from 'react'
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
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'

const dayNames = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']

function fmt(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function mondayOf(d) {
  const r = new Date(d)
  r.setHours(0, 0, 0, 0)
  r.setDate(r.getDate() - ((r.getDay() + 6) % 7))
  return r
}

function addDays(d, n) {
  const r = new Date(d)
  r.setDate(r.getDate() + n)
  return r
}

export default function TimetablePage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const canManage = currentUser?.role === 'admin'

  const [classrooms, setClassrooms] = useState([])
  const [offerings, setOfferings] = useState([])
  const [classroomId, setClassroomId] = useState('')
  const [weekStart, setWeekStart] = useState(mondayOf(new Date()))
  const [days, setDays] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [modal, setModal] = useState(null) // {date, periodIndex, session?}
  const [form, setForm] = useState({ offeringId: '', classroomId: '', date: '', periodStart: '', periodEnd: '', note: '' })
  const [modalError, setModalError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    // Dropdowns need (near-)full lists; pull the maximum page and unwrap the
    // pagination envelope.
    Promise.all([
      apiFetch('/api/classrooms?page_size=500', { token }),
      apiFetch('/api/offerings?page_size=500', { token }),
    ])
      .then(([cls, offs]) => {
        setClassrooms(Array.isArray(cls?.items) ? cls.items : [])
        setOfferings(Array.isArray(offs?.items) ? offs.items : [])
      })
      .catch((err) => setError(err.message))
  }, [token])

  const from = fmt(weekStart)
  const to = fmt(addDays(weekStart, 6))

  const fetchTimetable = useCallback(async () => {
    if (!classroomId) return
    try {
      setLoading(true)
      setError('')
      const data = await apiFetch(`/api/timetable?classroom_id=${classroomId}&from=${from}&to=${to}`, { token })
      setDays(Array.isArray(data) ? data : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [classroomId, from, to, token])

  useEffect(() => {
    fetchTimetable()
  }, [fetchTimetable])

  // union of period indices across the week (rows of the grid)
  const periods = useMemo(() => {
    const map = new Map()
    days.forEach((d) =>
      d.slots.forEach((s) => {
        if (!map.has(s.periodIndex)) map.set(s.periodIndex, { startTime: s.startTime, endTime: s.endTime })
      }),
    )
    return [...map.entries()].sort((a, b) => a[0] - b[0]).map(([idx, t]) => ({ periodIndex: idx, ...t }))
  }, [days])

  const slotFor = (day, periodIndex) => day.slots.find((s) => s.periodIndex === periodIndex)

  const openCell = (date, periodIndex, session) => {
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
      setModalError('请选择课程')
      return
    }
    const periodStart = Number(form.periodStart)
    const periodEnd = form.periodEnd ? Number(form.periodEnd) : periodStart
    if (!periodStart || periodStart < 1 || periodEnd < periodStart) {
      setModalError('节次范围不合法：起始节次须 ≥1 且不大于结束节次')
      return
    }
    const body = {
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
      if (modal.session) {
        await apiFetch(`/api/sessions/${modal.session.id}`, { method: 'PUT', token, body })
      } else {
        await apiFetch('/api/sessions', { method: 'POST', token, body })
      }
      setModal(null)
      await fetchTimetable()
    } catch (err) {
      setModalError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    try {
      setSaving(true)
      setModalError('')
      await apiFetch(`/api/sessions/${modal.session.id}`, { method: 'DELETE', token })
      setModal(null)
      await fetchTimetable()
    } catch (err) {
      setModalError(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Grid fullWidth className="courses-page">
      <Column sm={4} md={8} lg={16}>
        <Breadcrumb noTrailingSlash aria-label="面包屑导航">
          <BreadcrumbItem
            href="/"
            onClick={(e) => {
              e.preventDefault()
              navigate('/')
            }}
          >
            首页
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>教室课表</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">教室课表</h1>
        <p className="courses-page__subtitle">查看各教室周课表，支持加课、调课、删课。</p>
        {error && (
          <InlineNotification
            kind="error"
            title="加载失败"
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
            labelText="教室"
            value={classroomId}
            onChange={(e) => setClassroomId(e.target.value)}
            className="timetable__select"
          >
            <SelectItem value="" text="请选择教室" />
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={`${c.name}${c.building ? `（${c.building}）` : ''}`} />
            ))}
          </Select>
          <div className="timetable__week">
            <Button kind="ghost" size="sm" hasIconOnly renderIcon={ChevronLeft} iconDescription="上一周" onClick={() => setWeekStart(addDays(weekStart, -7))} />
            <span className="timetable__week-label">
              {from} ~ {to}
            </span>
            <Button kind="ghost" size="sm" hasIconOnly renderIcon={ChevronRight} iconDescription="下一周" onClick={() => setWeekStart(addDays(weekStart, 7))} />
          </div>
          <ExportButton
            path={`/api/sessions/export?classroom_id=${classroomId}&from=${from}&to=${to}`}
            fallbackName="sessions.xlsx"
            label="导出课表"
            onError={setError}
            disabled={!classroomId}
          />
        </div>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {!classroomId ? (
          <p>请先选择教室。</p>
        ) : loading ? (
          <p>加载中…</p>
        ) : days.length === 0 ? (
          <p>暂无数据。</p>
        ) : (
          <div className="timetable__scroll">
            <table className="timetable__grid">
              <thead>
                <tr>
                  <th className="timetable__corner">节次</th>
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
                      <span>第 {p.periodIndex} 节</span>
                      <span className="timetable__time">{p.startTime}-{p.endTime}</span>
                    </td>
                    {days.map((d) => {
                      const slot = slotFor(d, p.periodIndex)
                      const session = slot?.session
                      // 连上多节的课次从起始节起合并为一个单元格（rowSpan），
                      // 被覆盖的后续节次不再渲染。
                      if (session && session.periodStart !== p.periodIndex) return null
                      const span = session ? session.periodEnd - session.periodStart + 1 : 1
                      return (
                        <td
                          key={d.date + '-' + p.periodIndex}
                          rowSpan={span}
                          className={session ? 'timetable__cell timetable__cell--filled' : 'timetable__cell'}
                          onClick={() => canManage && openCell(d.date, p.periodIndex, session)}
                        >
                          {session ? (
                            <div className="timetable__session">
                              <strong>{session.courseName}</strong>
                              {session.teachingClassName && <span>{session.teachingClassName}</span>}
                              {session.teacher && <span>{session.teacher}</span>}
                            </div>
                          ) : canManage ? (
                            <span className="timetable__add">＋</span>
                          ) : null}
                        </td>
                      )
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Column>

      {/* Session add/edit modal */}
      <Modal
        open={Boolean(modal)}
        modalHeading={modal?.session ? '调课' : '添加课次'}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setModal(null)}
        onRequestSubmit={submit}
        primaryButtonDisabled={saving}
      >
        <div className="courses-page__form">
          <Select id="s-offering" labelText="课程" value={form.offeringId} onChange={(e) => setForm({ ...form, offeringId: e.target.value })}>
            <SelectItem value="" text="请选择课程" />
            {offerings.map((o) => (
              <SelectItem key={o.id} value={String(o.id)} text={`${o.catalogName} · ${o.teachingClassName} · ${o.semester}`} />
            ))}
          </Select>
          <Select id="s-classroom" labelText="教室" value={form.classroomId} onChange={(e) => setForm({ ...form, classroomId: e.target.value })}>
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={c.name} />
            ))}
          </Select>
          <TextInput id="s-date" type="date" labelText="日期" value={form.date} onChange={(e) => setForm({ ...form, date: e.target.value })} />
          <TextInput id="s-period-start" type="number" labelText="起始节次" min="1" value={form.periodStart} onChange={(e) => setForm({ ...form, periodStart: e.target.value })} />
          <TextInput id="s-period-end" type="number" labelText="结束节次（连上多节填末节，单节留空）" min="1" value={form.periodEnd} onChange={(e) => setForm({ ...form, periodEnd: e.target.value })} />
          <TextInput id="s-note" labelText="备注" value={form.note} onChange={(e) => setForm({ ...form, note: e.target.value })} />
          {modalError && (
            <InlineNotification kind="error" title="保存失败" subtitle={modalError} lowContrast hideCloseButton />
          )}
        </div>
        {modal?.session && canManage && (
          <Button kind="danger" size="sm" renderIcon={TrashCan} onClick={remove} disabled={saving} className="timetable__delete">
            删除此课次
          </Button>
        )}
      </Modal>
    </Grid>
  )
}
