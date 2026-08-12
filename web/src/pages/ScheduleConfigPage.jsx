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
  Tile,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'

const months = ['一月', '二月', '三月', '四月', '五月', '六月', '七月', '八月', '九月', '十月', '十一月', '十二月']

const emptyRegime = { name: '', effectiveMonth: 5, effectiveDay: 1 }

export default function ScheduleConfigPage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const canManage = currentUser?.role === 'admin'

  const [regimes, setRegimes] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [regimeOpen, setRegimeOpen] = useState(false)
  const [regimeForm, setRegimeForm] = useState(emptyRegime)
  const [regimeEditId, setRegimeEditId] = useState(null)
  const [regimeError, setRegimeError] = useState('')
  const [regimeSaving, setRegimeSaving] = useState(false)

  const [periodsTarget, setPeriodsTarget] = useState(null) // regime being edited
  const [periodRows, setPeriodRows] = useState([])
  const [periodsError, setPeriodsError] = useState('')
  const [periodsSaving, setPeriodsSaving] = useState(false)

  const [delTarget, setDelTarget] = useState(null)
  const [delError, setDelError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const fetchRegimes = useCallback(async () => {
    try {
      setLoading(true)
      setError('')
      const data = await apiFetch('/api/schedule/regimes', { token })
      setRegimes(Array.isArray(data) ? data : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchRegimes()
  }, [fetchRegimes])

  const openCreateRegime = () => {
    setRegimeForm(emptyRegime)
    setRegimeEditId(null)
    setRegimeError('')
    setRegimeOpen(true)
  }

  const openEditRegime = (r) => {
    setRegimeForm({ name: r.name, effectiveMonth: r.effectiveMonth, effectiveDay: r.effectiveDay })
    setRegimeEditId(r.id)
    setRegimeError('')
    setRegimeOpen(true)
  }

  const submitRegime = async () => {
    if (!regimeForm.name.trim()) {
      setRegimeError('名称为必填项')
      return
    }
    const body = {
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
      await fetchRegimes()
    } catch (err) {
      setRegimeError(err.message)
    } finally {
      setRegimeSaving(false)
    }
  }

  const openEditPeriods = (r) => {
    setPeriodsTarget(r)
    setPeriodRows(
      r.periods && r.periods.length
        ? r.periods.map((p) => ({ periodIndex: p.periodIndex, startTime: p.startTime, endTime: p.endTime }))
        : [{ periodIndex: 1, startTime: '08:00', endTime: '08:45' }],
    )
    setPeriodsError('')
  }

  const submitPeriods = async () => {
    try {
      setPeriodsSaving(true)
      setPeriodsError('')
      await apiFetch(`/api/schedule/regimes/${periodsTarget.id}/periods`, {
        method: 'PUT',
        token,
        body: { periods: periodRows.map((p) => ({ ...p, periodIndex: Number(p.periodIndex) })) },
      })
      setPeriodsTarget(null)
      await fetchRegimes()
    } catch (err) {
      setPeriodsError(err.message)
    } finally {
      setPeriodsSaving(false)
    }
  }

  const handleDelete = async () => {
    try {
      setDeleting(true)
      setDelError('')
      await apiFetch(`/api/schedule/regimes/${delTarget.id}`, { method: 'DELETE', token })
      setDelTarget(null)
      await fetchRegimes()
    } catch (err) {
      setDelError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  const updateRow = (i, field, value) => {
    const next = [...periodRows]
    next[i] = { ...next[i], [field]: value }
    setPeriodRows(next)
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
          <BreadcrumbItem isCurrentPage>作息设置</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">作息设置</h1>
        <p className="courses-page__subtitle">
          配置冬/夏令时作息及其切换日期，自定义每天节次数与各节次时间。
        </p>
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
        {canManage && (
          <Button renderIcon={Add} size="sm" onClick={openCreateRegime} className="courses-page__add">
            添加作息
          </Button>
        )}
        <ExportButton
          path="/api/schedule/regimes/export"
          fallbackName="regimes.xlsx"
          onError={setError}
          className="courses-page__add"
        />
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loading ? (
          <p>加载中…</p>
        ) : regimes.length === 0 ? (
          <p>暂无作息配置，请先添加。</p>
        ) : (
          regimes.map((r) => (
            <Tile key={r.id} className="schedule-regime">
              <div className="schedule-regime__head">
                <div>
                  <strong>{r.name}</strong>
                  <span className="schedule-regime__date">
                    {' '}
                    · 每年 {r.effectiveMonth} 月 {r.effectiveDay} 日起生效
                  </span>
                </div>
                {canManage && (
                  <div className="schedule-regime__actions">
                    <Button size="sm" kind="ghost" renderIcon={Edit} onClick={() => openEditRegime(r)}>
                      编辑
                    </Button>
                    <Button size="sm" kind="ghost" onClick={() => openEditPeriods(r)}>
                      编辑节次
                    </Button>
                    <Button
                      size="sm"
                      kind="ghost"
                      hasIconOnly
                      renderIcon={TrashCan}
                      iconDescription="删除"
                      onClick={() => setDelTarget(r)}
                    />
                  </div>
                )}
              </div>
              {r.periods && r.periods.length > 0 ? (
                <table className="schedule-regime__periods">
                  <thead>
                    <tr>
                      <th>节次</th>
                      <th>开始</th>
                      <th>结束</th>
                    </tr>
                  </thead>
                  <tbody>
                    {r.periods.map((p) => (
                      <tr key={p.id || p.periodIndex}>
                        <td>第 {p.periodIndex} 节</td>
                        <td>{p.startTime}</td>
                        <td>{p.endTime}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="schedule-regime__empty">尚未配置节次</p>
              )}
            </Tile>
          ))
        )}
      </Column>

      {/* Regime create/edit modal */}
      <Modal
        open={regimeOpen}
        modalHeading={regimeEditId ? '编辑作息' : '添加作息'}
        primaryButtonText="保存"
        secondaryButtonText="取消"
        onRequestClose={() => setRegimeOpen(false)}
        onRequestSubmit={submitRegime}
        primaryButtonDisabled={regimeSaving}
      >
        <div className="courses-page__form">
          <TextInput
            id="regime-name"
            labelText="名称"
            placeholder="如 冬令时 / 夏令时"
            value={regimeForm.name}
            onChange={(e) => setRegimeForm({ ...regimeForm, name: e.target.value })}
          />
          <Select
            id="regime-month"
            labelText="生效月份"
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
            labelText="生效日"
            min="1"
            max="31"
            value={regimeForm.effectiveDay}
            onChange={(e) => setRegimeForm({ ...regimeForm, effectiveDay: e.target.value })}
          />
          {regimeError && (
            <InlineNotification kind="error" title="保存失败" subtitle={regimeError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Periods edit modal */}
      <Modal
        open={Boolean(periodsTarget)}
        modalHeading={`编辑节次：${periodsTarget?.name ?? ''}`}
        primaryButtonText="保存节次"
        secondaryButtonText="取消"
        onRequestClose={() => setPeriodsTarget(null)}
        onRequestSubmit={submitPeriods}
        primaryButtonDisabled={periodsSaving}
        size="lg"
      >
        <div className="courses-page__form">
          <p className="schedule-periods__hint">每天节次数 = 下方行数，可增删行调整。</p>
          {periodRows.map((row, i) => (
            <div key={i} className="schedule-periods__row">
              <TextInput
                id={`pi-${i}`}
                type="number"
                labelText="节次"
                min="1"
                value={row.periodIndex}
                onChange={(e) => updateRow(i, 'periodIndex', e.target.value)}
              />
              <TextInput
                id={`st-${i}`}
                type="time"
                labelText="开始"
                value={row.startTime}
                onChange={(e) => updateRow(i, 'startTime', e.target.value)}
              />
              <TextInput
                id={`et-${i}`}
                type="time"
                labelText="结束"
                value={row.endTime}
                onChange={(e) => updateRow(i, 'endTime', e.target.value)}
              />
              <Button
                kind="ghost"
                size="sm"
                hasIconOnly
                renderIcon={TrashCan}
                iconDescription="删除该节"
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
                { periodIndex: periodRows.length + 1, startTime: '08:00', endTime: '08:45' },
              ])
            }
          >
            添加节次
          </Button>
          {periodsError && (
            <InlineNotification kind="error" title="保存失败" subtitle={periodsError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Delete modal */}
      <Modal
        danger
        open={Boolean(delTarget)}
        modalHeading="删除作息"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDelTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="courses-page__confirm-text">
          确定要删除作息「{delTarget?.name}」及其所有节次吗？此操作不可撤销。
        </p>
        {delError && (
          <InlineNotification kind="error" title="删除失败" subtitle={delError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
