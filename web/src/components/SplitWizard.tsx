import { useEffect, useState } from 'react'
import { Button, InlineNotification, Tag, type TagProps } from '@carbon/react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '../auth/api'
import ImportPreviewTable from './ImportPreviewTable'
import type { ImportJob, SplitStats } from '../types/api'

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

export interface SplitWizardProps {
  jobs: Array<{ id: number; type: string; status: string }>
  stats?: SplitStats | null
  warnings?: string[]
  token?: string | null
  onExit: () => void
  onViewJobs: () => void
}

// SplitWizard guides an operator through the 6 jobs produced by
// /api/imports/jwc_split in dependency order (classrooms -> catalog ->
// admin_classes -> teaching_classes -> offerings -> sessions). It renders INLINE
// (no modal) so the large preview tables get the full page width.
//
// Per step it shows a *fresh* preview: on entering a step it waits for the
// initial preview, then reanalyzes against the current DB so dependent tables
// no longer show the stale "课程不存在" errors produced at split time. The
// operator then 确定 (commit) -> 下一步.
//
// 下一步 is gated on the step being committed or explicitly skipped, which
// makes out-of-order commits impossible from this flow. 跳过当前步 leaves the
// job in preview for later handling via /imports/:id; a genuinely failed commit
// can be retried (the backend accepts failed->commit) or skipped. 退出 returns
// to the split form (onExit) without touching job state; the jobs persist and
// are visible in /imports.
export default function SplitWizard({ jobs, stats, warnings, token, onExit, onViewJobs }: SplitWizardProps) {
  const { t } = useTranslation('imports')
  const steps = jobs // [{id,type,status}], already in dependency order
  const [current, setCurrent] = useState(0)
  const [results, setResults] = useState<Record<number, ImportJob>>({}) // jobId -> full job object
  const [resolved, setResolved] = useState<Record<number, 'succeeded' | 'skipped' | 'failed'>>({}) // jobId -> outcome
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [finished, setFinished] = useState(false)

  const typeLabel = (type: string) => (type ? t(`types.${type}.label`, { defaultValue: type }) : '-')

  async function fetchJob(id: number): Promise<ImportJob> {
    return apiFetch<ImportJob>(`/api/imports/${id}`, { token })
  }

  // pollUntil GETs a job until pred(status) is true or ~90s elapse; returns the
  // last-fetched job regardless (caller inspects job.status).
  async function pollUntil(id: number, pred: (status: string) => boolean, { timeoutMs = 90000, interval = 1200 } = {}) {
    const deadline = Date.now() + timeoutMs
    let job = await fetchJob(id)
    while (!pred(job.status) && Date.now() < deadline) {
      await sleep(interval)
      job = await fetchJob(id)
    }
    return job
  }

  // On entering a step: wait for the initial preview, then reanalyze so the
  // preview reflects the current DB (a prerequisite may have just been
  // committed in the previous step).
  useEffect(() => {
    let cancelled = false
    setError('')
    setBusy(true)
    ;(async () => {
      try {
        const id = steps[current]!.id
        let job = await pollUntil(id, (s) => s === 'preview' || s === 'failed')
        if (cancelled) return
        if (job.status === 'preview') {
          try {
            await apiFetch(`/api/imports/${id}/reanalyze`, { method: 'POST', token })
            job = await pollUntil(id, (s) => s === 'preview' || s === 'failed')
          } catch {
            // reanalyze rejected (e.g. lost a race) - fall back to the preview
            job = await fetchJob(id)
          }
        }
        if (cancelled) return
        setResults((p) => ({ ...p, [id]: job }))
        if (job.status === 'failed') setResolved((p) => ({ ...p, [id]: 'failed' }))
      } catch (e) {
        if (!cancelled) setError((e as Error).message)
      } finally {
        if (!cancelled) setBusy(false)
      }
    })()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current])

  async function handleReanalyze() {
    const id = steps[current]!.id
    setError('')
    setBusy(true)
    try {
      await apiFetch(`/api/imports/${id}/reanalyze`, { method: 'POST', token })
      const job = await pollUntil(id, (s) => s === 'preview' || s === 'failed')
      setResults((p) => ({ ...p, [id]: job }))
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function handleCommit() {
    const id = steps[current]!.id
    setError('')
    setBusy(true)
    try {
      await apiFetch(`/api/imports/${id}/commit`, { method: 'POST', token })
      const job = await pollUntil(id, (s) => s === 'succeeded' || s === 'failed')
      setResults((p) => ({ ...p, [id]: job }))
      if (job.status === 'succeeded' || job.status === 'failed') {
        setResolved((p) => ({ ...p, [id]: job.status as 'succeeded' | 'failed' }))
      } else {
        setError(t('wizard.timeout'))
      }
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  function handleSkip() {
    const id = steps[current]!.id
    setResolved((p) => ({ ...p, [id]: 'skipped' }))
  }

  function handleNext() {
    if (current < steps.length - 1) setCurrent((c) => c + 1)
    else setFinished(true)
  }

  if (!steps?.length) return null

  if (finished) {
    return (
      <div className="imports-page__split-result">
        <InlineNotification
          kind="success"
          title={t('wizard.doneTitle')}
          subtitle={t('wizard.doneSubtitle')}
          lowContrast
          hideCloseButton
          className="imports-page__upload-err"
        />
        <div style={{ display: 'flex', gap: '.5rem', marginTop: '.5rem' }}>
          <Button size="sm" onClick={onViewJobs}>{t('wizard.viewJobs')}</Button>
          <Button kind="ghost" size="sm" onClick={onExit}>{t('wizard.splitAgain')}</Button>
        </div>
      </div>
    )
  }

  const step = steps[current]!
  const job = results[step.id]
  const r = resolved[step.id]
  const isLast = current === steps.length - 1
  const notResolved = r !== 'succeeded' && r !== 'skipped'
  const warnList = warnings ?? []

  const stepState = (k: number): 'done' | 'active' | 'todo' | 'skipped' | 'failed' => {
    const rs = resolved[steps[k]!.id]
    if (k < current) return rs === 'skipped' ? 'skipped' : 'done'
    if (k === current) {
      if (rs === 'skipped') return 'skipped'
      if (rs === 'failed') return 'failed'
      if (rs === 'succeeded') return 'done'
      return 'active'
    }
    return 'todo'
  }

  const tagType: Record<'done' | 'active' | 'todo' | 'skipped' | 'failed', TagProps<'div'>['type']> = {
    done: 'green', active: 'blue', todo: 'gray', skipped: 'gray', failed: 'red',
  }

  const summary =
    r === 'succeeded' || r === 'failed'
      ? t('modal.detailSummary', { succeeded: job?.succeededRows ?? 0, failed: job?.failedRows ?? 0, total: job?.totalRows ?? 0 })
      : t('modal.previewSummary', { succeeded: job?.succeededRows ?? 0, failed: job?.failedRows ?? 0, total: job?.totalRows ?? 0 })

  return (
    <section className="imports-page__wizard">
      <div style={{ marginBottom: '.5rem' }}>
        <h2 className="imports-page__subheading">{t('wizard.heading')}</h2>
        <p className="courses-page__subtitle">
          {t('wizard.stepLabel', { n: current + 1, total: steps.length, label: typeLabel(step.type) })}
        </p>
      </div>

      <div className="imports-page__split-result" style={{ marginBottom: '1rem' }}>
        <InlineNotification
          kind="success"
          title={t('split.successTitle', { count: steps.length })}
          subtitle={t('split.successSubtitle', {
            classrooms: stats?.classrooms ?? 0,
            catalogCourses: stats?.catalogCourses ?? 0,
            adminClasses: stats?.adminClasses ?? 0,
            teachingClasses: stats?.teachingClasses ?? 0,
            offerings: stats?.offerings ?? 0,
            sessions: stats?.sessions ?? 0,
            skippedEmptyAdmin: stats?.skippedEmptyAdmin ?? 0,
            skippedParallel: stats?.skippedParallel ?? 0,
            noTeacherFilled: stats?.noTeacherFilled ?? 0,
          })}
          lowContrast
          hideCloseButton
          className="imports-page__upload-err"
        />
        {warnList.length > 0 && (
          <details className="imports-page__warnings">
            <summary>{t('split.warningsSummary', { count: warnList.length })}</summary>
            <ul>
              {warnList.slice(0, 50).map((w, i) => (
                <li key={i}>{w}</li>
              ))}
              {warnList.length > 50 && (
                <li>{t('split.warningsMore', { count: warnList.length - 50 })}</li>
              )}
            </ul>
          </details>
        )}
      </div>

      <div style={{ display: 'flex', gap: '.25rem', flexWrap: 'wrap', margin: '.5rem 0 1rem' }}>
        {steps.map((s, k) => {
          const st = stepState(k)
          return (
            <Tag key={s.id} type={tagType[st]} size="sm">
              {k + 1}. {typeLabel(s.type)}
              {st === 'skipped' ? `（${t('wizard.skippedTag')}）` : ''}
            </Tag>
          )
        })}
      </div>

      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '.5rem', flexWrap: 'wrap', margin: '.5rem 0' }}>
        <div style={{ display: 'flex', gap: '.5rem' }}>
          <Button kind="ghost" size="sm" onClick={handleReanalyze} disabled={busy || !job || job.status !== 'preview'}>
            {t('wizard.refresh')}
          </Button>
          <Button kind="ghost" size="sm" onClick={() => { if (!busy) onExit() }} disabled={busy}>
            {t('wizard.exit')}
          </Button>
        </div>
        <div style={{ display: 'flex', gap: '.5rem' }}>
          {notResolved && (
            <Button kind="ghost" size="sm" onClick={handleSkip} disabled={busy}>
              {t('wizard.skip')}
            </Button>
          )}
          {notResolved ? (
            <Button size="sm" onClick={handleCommit} disabled={busy || (r !== 'failed' && (job?.succeededRows ?? 0) === 0)}>
              {r === 'failed' ? t('wizard.retry') : t('modal.commit')}
            </Button>
          ) : (
            <Button size="sm" onClick={handleNext} disabled={busy}>
              {isLast ? t('wizard.finish') : t('wizard.next')}
            </Button>
          )}
        </div>
      </div>

      {r === 'skipped' && (
        <InlineNotification
          kind="info"
          title={t('wizard.skippedNote')}
          lowContrast
          hideCloseButton
          className="imports-page__upload-err"
        />
      )}
      {r === 'failed' && (
        <InlineNotification
          kind="error"
          title={t('wizard.failedNote')}
          lowContrast
          hideCloseButton
          className="imports-page__upload-err"
        />
      )}
      {error && (
        <InlineNotification
          kind="error"
          title={t('error.action')}
          subtitle={error}
          lowContrast
          hideCloseButton
          className="imports-page__upload-err"
        />
      )}

      {busy && !job ? (
        <p className="imports-page__summary">{t('wizard.loading')}</p>
      ) : job ? (
        <>
          <p className="imports-page__summary">{summary}</p>
          <ImportPreviewTable job={job} token={token} t={t} />
        </>
      ) : (
        <p className="imports-page__summary">{t('wizard.loading')}</p>
      )}
    </section>
  )
}
