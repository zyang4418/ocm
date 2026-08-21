import { useRef, useState } from 'react'
import { Breadcrumb, BreadcrumbItem, Button, Column, Grid, InlineNotification, TextInput } from '@carbon/react'
import { Upload } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiUpload } from '../auth/api'
import SplitWizard from '../components/SplitWizard'
import type { SplitResult } from '../types/api'

// SplitPage (/imports/split) is the 教务处课表拆分 entry: upload the aggregated
// schedule + semester + week-1 Monday, the backend splits it into 6 import jobs,
// and the inline SplitWizard guides the operator through them in dependency
// order (full page width — no modal — so the large preview tables are readable).
export default function SplitPage() {
  const { t } = useTranslation('imports')
  const { token } = useAuth()
  const navigate = useNavigate()
  const fileRef = useRef<HTMLInputElement>(null)

  const [splitFile, setSplitFile] = useState<File | null>(null)
  const [splitSemester, setSplitSemester] = useState('')
  const [splitWeek1, setSplitWeek1] = useState('')
  const [splitting, setSplitting] = useState(false)
  const [splitError, setSplitError] = useState('')
  // SplitWizard session: holds the 6 jobs (dependency order) + stats/warnings
  // returned by jwc_split. Null = show the split form; set = show the wizard.
  const [wizard, setWizard] = useState<SplitResult | null>(null)

  const handleSplit = async () => {
    if (!splitFile || !splitSemester || !splitWeek1) return
    try {
      setSplitting(true)
      setSplitError('')
      setWizard(null)
      const data = await apiUpload<SplitResult>('/api/imports/jwc_split', {
        file: splitFile,
        token,
        fields: { semester: splitSemester, week1_monday: splitWeek1 },
      })
      setSplitFile(null)
      if (fileRef.current) fileRef.current.value = ''
      setWizard({ jobs: data.jobs, stats: data.stats, warnings: data.warnings })
    } catch (err) {
      setSplitError((err as Error).message)
    } finally {
      setSplitting(false)
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
          <BreadcrumbItem
            href="/imports"
            onClick={(e) => {
              e.preventDefault()
              navigate('/imports')
            }}
          >
            {t('breadcrumb.current')}
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>{t('split.heading')}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">{t('split.heading')}</h1>
        <p className="courses-page__subtitle">{t('split.subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {wizard ? (
          <SplitWizard
            jobs={wizard.jobs}
            stats={wizard.stats}
            warnings={wizard.warnings}
            token={token}
            onExit={() => setWizard(null)}
            onViewJobs={() => navigate('/imports')}
          />
        ) : (
          <>
            <div className="imports-page__upload">
              <input
                ref={fileRef}
                type="file"
                accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                onChange={(e) => setSplitFile(e.target.files?.[0] || null)}
              />
              <TextInput
                id="jwc-semester"
                className="imports-page__split-input"
                labelText={t('split.semester')}
                placeholder="2024-2025-2"
                value={splitSemester}
                onChange={(e) => setSplitSemester(e.target.value)}
                size="sm"
              />
              <TextInput
                id="jwc-week1"
                className="imports-page__split-input"
                type="date"
                labelText={t('split.week1')}
                value={splitWeek1}
                onChange={(e) => setSplitWeek1(e.target.value)}
                size="sm"
              />
              <Button
                renderIcon={Upload}
                size="sm"
                onClick={handleSplit}
                disabled={!splitFile || !splitSemester || !splitWeek1 || splitting}
              >
                {splitting ? t('split.splitting') : t('split.button')}
              </Button>
            </div>
            {splitError && (
              <InlineNotification
                kind="error"
                title={t('split.error')}
                subtitle={splitError}
                lowContrast
                hideCloseButton
                className="imports-page__upload-err"
              />
            )}
          </>
        )}
      </Column>
    </Grid>
  )
}
