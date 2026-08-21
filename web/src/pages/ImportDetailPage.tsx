import { useEffect, useState } from 'react'
import { Breadcrumb, BreadcrumbItem, Button, Column, Grid, InlineNotification, Tag, type TagProps } from '@carbon/react'
import { useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ImportPreviewTable from '../components/ImportPreviewTable'
import type { ImportJob } from '../types/api'

const STATUS_KIND: Record<string, TagProps<'div'>['type']> = {
  pending: 'blue',
  processing: 'blue',
  preview: 'purple',
  succeeded: 'green',
  failed: 'red',
  cancelled: 'gray',
}

// ImportDetailPage (/imports/:id) is the full-page preview for one import job,
// replacing the old detail modal so the (potentially very large) preview table
// gets the full content width. Loads job metadata, polls while pending/
// processing, and offers commit/cancel inline. A failed commit can be retried
// here too (the backend accepts failed→commit).
export default function ImportDetailPage() {
  const { t } = useTranslation('imports')
  const { id } = useParams()
  const navigate = useNavigate()
  const { token } = useAuth()

  const [job, setJob] = useState<ImportJob | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [actionPending, setActionPending] = useState(false)
  const [actionError, setActionError] = useState('')

  const load = async () => {
    try {
      const v = await apiFetch<ImportJob>(`/api/imports/${id}`, { token })
      setJob(v)
      setLoadError('')
      setLoading(false)
    } catch (e) {
      setLoadError((e as Error).message)
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  // Poll while the job is still pending or processing (initial preview, or a
  // commit/reanalyze in flight). Stops once it reaches a terminal state.
  useEffect(() => {
    if (!job || (job.status !== 'pending' && job.status !== 'processing')) return undefined
    const timer = setInterval(async () => {
      try {
        const v = await apiFetch<ImportJob>(`/api/imports/${id}`, { token })
        setJob(v)
      } catch {
        // keep the last known job on a transient poll failure
      }
    }, 1500)
    return () => clearInterval(timer)
  }, [job?.status, id, token])

  const typeLabel = (type?: string) => (type ? t('types.' + type + '.label', { defaultValue: type }) : '-')

  const handleCommit = async () => {
    setActionPending(true)
    setActionError('')
    try {
      await apiFetch(`/api/imports/${id}/commit`, { method: 'POST', token })
      await load() // flip to processing; the poll effect takes over
    } catch (e) {
      setActionError((e as Error).message)
    } finally {
      setActionPending(false)
    }
  }

  const handleCancel = async () => {
    setActionPending(true)
    setActionError('')
    try {
      await apiFetch(`/api/imports/${id}/cancel`, { method: 'POST', token })
      navigate('/imports')
    } catch (e) {
      setActionError((e as Error).message)
    } finally {
      setActionPending(false)
    }
  }

  const canCommit = job && (job.status === 'preview' || job.status === 'failed')
  const commitDisabled = actionPending || (job?.status === 'preview' && (job?.succeededRows ?? 0) === 0)

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
          <BreadcrumbItem
            href="/imports"
            onClick={(e) => {
              e.preventDefault()
              navigate('/imports')
            }}
          >
            {t('breadcrumb.current')}
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>
            {t('detail.heading', { label: typeLabel(job?.type) })}
          </BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">
          {job ? t('detail.heading', { label: typeLabel(job.type) }) : t('modal.loading')}
        </h1>
        {loadError && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={loadError}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
      </Column>

      <Column sm={4} md={8} lg={16}>
        {loading ? (
          <p className="imports-page__summary">{t('modal.loading')}</p>
        ) : job ? (
          <>
            <div
              className="imports-page__detail-head"
              style={{ display: 'flex', alignItems: 'center', gap: '.5rem', flexWrap: 'wrap', marginBottom: '.5rem' }}
            >
              <Tag type={STATUS_KIND[job.status] ?? 'gray'} size="sm">
                {t('status.' + job.status, { defaultValue: job.status })}
              </Tag>
              <span style={{ color: 'var(--cds-text-secondary)' }}>{job.filename || t('unnamed')}</span>
            </div>

            <p className="imports-page__summary">
              {job.status === 'preview'
                ? t('modal.previewSummary', { succeeded: job.succeededRows, failed: job.failedRows, total: job.totalRows })
                : t('modal.detailSummary', { succeeded: job.succeededRows, failed: job.failedRows, total: job.totalRows })}
            </p>

            {canCommit && (
              <div className="imports-page__actions" style={{ display: 'flex', gap: '.5rem', margin: '.5rem 0' }}>
                <Button size="sm" onClick={handleCommit} disabled={commitDisabled}>
                  {job.status === 'failed' ? t('wizard.retry') : t('modal.commit')}
                </Button>
                {job.status === 'preview' && (
                  <Button kind="ghost" size="sm" onClick={handleCancel} disabled={actionPending}>
                    {t('modal.cancel')}
                  </Button>
                )}
              </div>
            )}

            {actionError && (
              <InlineNotification
                kind="error"
                title={t('error.action')}
                subtitle={actionError}
                lowContrast
                hideCloseButton
                className="imports-page__upload-err"
              />
            )}

            <ImportPreviewTable job={job} token={token} t={t} />
          </>
        ) : null}
      </Column>
    </Grid>
  )
}
