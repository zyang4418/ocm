import { useEffect, useRef, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  DataTable,
  Grid,
  InlineNotification,
  Modal,
  Select,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Tag,
  TextInput,
} from '@carbon/react'
import { Upload } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch, apiUpload } from '../auth/api.js'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'
import i18n from '../i18n/index.js'
import { formatDate } from '../i18n/formatters.js'

// IMPORT_TYPES describes each business-table import: the xlsx header contract
// (order-independent, matched by name — kept literal as a backend contract),
// and the preview column keys used to render the dry-run rows. The column keys
// mirror the toPreviewMap() keys produced by each backend importer and double
// as i18n keys under imports.columns.<type>.<key>.
const IMPORT_TYPES = {
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

const statusKind = {
  pending: 'blue',
  processing: 'blue',
  preview: 'purple',
  succeeded: 'green',
  failed: 'red',
  cancelled: 'gray',
}

const headers = (t) => [
  { key: 'type', header: t('field.type') },
  { key: 'filename', header: t('field.filename') },
  { key: 'status', header: t('field.status') },
  { key: 'totalRows', header: t('field.totalRows') },
  { key: 'succeededRows', header: t('field.succeededRows') },
  { key: 'failedRows', header: t('field.failedRows') },
  { key: 'createdAt', header: t('field.createdAt') },
]

// typeLabel maps a job's type string to a localized label; unknown types fall
// back to the raw value so the list stays readable even for stale/unregistered
// types.
// formatCell renders a preview cell value. Arrays (teaching-class members,
// regime periods) are joined into a readable string; other values pass through.
function formatCell(value) {
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

export default function ImportsPage() {
  const { t } = useTranslation('imports')
  const { token } = useAuth()
  const navigate = useNavigate()
  const fileRef = useRef(null)

  const list = usePagedList({ path: '/api/imports', token })
  const { loading } = list

  const [importType, setImportType] = useState('sessions')
  const [selectedFile, setSelectedFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [uploadError, setUploadError] = useState('')

  const [detailJob, setDetailJob] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [actionPending, setActionPending] = useState(false)
  const [actionError, setActionError] = useState('')

  // 教务处课表拆分表单：上传聚合表 + 学期 + 第一周周一，后端拆为 6 个导入任务。
  const [splitFile, setSplitFile] = useState(null)
  const [splitSemester, setSplitSemester] = useState('')
  const [splitWeek1, setSplitWeek1] = useState('')
  const [splitting, setSplitting] = useState(false)
  const [splitError, setSplitError] = useState('')
  const [splitResult, setSplitResult] = useState(null)
  const splitFileRef = useRef(null)

  // Poll while any job is still pending or processing. The poll refetches the
  // current page (a fresh job always lands on page 1, newest first).
  useEffect(() => {
    const hasActive = list.items.some((j) => j.status === 'pending' || j.status === 'processing')
    if (!hasActive) return undefined
    const timer = setInterval(list.reload, 3000)
    return () => clearInterval(timer)
  }, [list.items, list.reload])

  const typeLabel = (type) => (type ? t('types.' + type + '.label', { defaultValue: type }) : '-')

  const handleUpload = async () => {
    if (!selectedFile) return
    try {
      setUploading(true)
      setUploadError('')
      await apiUpload(`/api/imports/${importType}`, { file: selectedFile, token })
      setSelectedFile(null)
      if (fileRef.current) fileRef.current.value = ''
      list.reload()
    } catch (err) {
      setUploadError(err.message)
    } finally {
      setUploading(false)
    }
  }

  // handleSplit posts the 教务处 aggregated schedule to /api/imports/jwc_split
  // with semester + week1_monday. The backend splits it into 6 canonical xlsx
  // and creates one import job per output; we just refresh the job list and
  // surface the split stats/warnings. The 6 jobs then follow the normal
  // preview→commit flow (commit in dependency order: classrooms → catalog →
  // admin_classes → teaching_classes → offerings → sessions).
  const handleSplit = async () => {
    if (!splitFile || !splitSemester || !splitWeek1) return
    try {
      setSplitting(true)
      setSplitError('')
      setSplitResult(null)
      const data = await apiUpload('/api/imports/jwc_split', {
        file: splitFile,
        token,
        fields: { semester: splitSemester, week1_monday: splitWeek1 },
      })
      setSplitResult(data)
      setSplitFile(null)
      if (splitFileRef.current) splitFileRef.current.value = ''
      list.reload()
    } catch (err) {
      setSplitError(err.message)
    } finally {
      setSplitting(false)
    }
  }

  const openDetail = async (job) => {
    setActionError('')
    setDetailJob(job) // show list data immediately, fill rows when loaded
    setDetailLoading(true)
    try {
      const full = await apiFetch(`/api/imports/${job.id}`, { token })
      setDetailJob(full)
    } catch (err) {
      setActionError(err.message)
    } finally {
      setDetailLoading(false)
    }
  }

  const closeDetail = () => {
    setDetailJob(null)
    setActionError('')
  }

  const handleCommit = async () => {
    if (!detailJob) return
    setActionPending(true)
    setActionError('')
    try {
      await apiFetch(`/api/imports/${detailJob.id}/commit`, { method: 'POST', token })
      setDetailJob(null)
      list.reload()
    } catch (err) {
      setActionError(err.message)
    } finally {
      setActionPending(false)
    }
  }

  const handleCancel = async () => {
    if (!detailJob) return
    setActionPending(true)
    setActionError('')
    try {
      await apiFetch(`/api/imports/${detailJob.id}/cancel`, { method: 'POST', token })
      setDetailJob(null)
      list.reload()
    } catch (err) {
      setActionError(err.message)
    } finally {
      setActionPending(false)
    }
  }

  const errorList = (() => {
    try {
      const list = JSON.parse(detailJob?.errorReport || '[]')
      return Array.isArray(list) ? list : []
    } catch {
      return []
    }
  })()

  const previewRows = Array.isArray(detailJob?.rows) ? detailJob.rows : []
  const PREVIEW_CAP = 1000
  const shownRows = previewRows.slice(0, PREVIEW_CAP)
  const rowsTruncated = previewRows.length - shownRows.length
  // Preview columns are driven by the job's type so a preview reflects what was
  // uploaded, not the currently-selected type selector.
  const previewColumns = (detailJob ? IMPORT_TYPES[detailJob.type]?.columns || [] : [])
    .map((key) => ({ key, header: t('columns.' + detailJob.type + '.' + key, { defaultValue: key }) }))

  const tableHeaders = headers(t)
  const rows = list.items.map((j) => ({
    id: String(j.id),
    type: j.type,
    filename: j.filename || t('unnamed'),
    status: j.status,
    totalRows: j.totalRows,
    succeededRows: j.succeededRows,
    failedRows: j.failedRows,
    createdAt: j.createdAt,
  }))

  const colSpan = tableHeaders.length + 1
  const typeCfg = IMPORT_TYPES[importType]

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
        {list.error && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={list.error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
      </Column>

      <Column sm={4} md={8} lg={16}>
        <div className="imports-page__upload">
          <Select
            id="import-type"
            className="imports-page__type-select"
            labelText={t('upload.typeLabel')}
            value={importType}
            onChange={(e) => setImportType(e.target.value)}
            size="sm"
          >
            {Object.entries(IMPORT_TYPES).map(([value]) => (
              <SelectItem key={value} value={value} text={t('types.' + value + '.label')} />
            ))}
          </Select>
          <input
            ref={fileRef}
            type="file"
            accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            onChange={(e) => setSelectedFile(e.target.files?.[0] || null)}
          />
          <Button renderIcon={Upload} size="sm" onClick={handleUpload} disabled={!selectedFile || uploading}>
            {uploading ? t('upload.uploading') : t('upload.button')}
          </Button>
          {uploadError && (
            <InlineNotification
              kind="error"
              title={t('upload.error')}
              subtitle={uploadError}
              lowContrast
              hideCloseButton
              className="imports-page__upload-err"
            />
          )}
        </div>
        <p className="imports-page__schema">
          <strong>{typeLabel(importType)}</strong>{t('schemaIntroTail')}
          <code>{typeCfg.schema}</code>
        </p>
        <p className="imports-page__note">{t('types.' + importType + '.note')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        <div className="imports-page__split-head">
          <h2 className="imports-page__subheading">{t('split.heading')}</h2>
          <p className="courses-page__subtitle">{t('split.subtitle')}</p>
        </div>
        <div className="imports-page__upload">
          <input
            ref={splitFileRef}
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
        {splitResult && (
          <div className="imports-page__split-result">
            <InlineNotification
              kind="success"
              title={t('split.successTitle', { count: splitResult.jobs?.length ?? 6 })}
              subtitle={t('split.successSubtitle', {
                classrooms: splitResult.stats?.classrooms ?? 0,
                catalogCourses: splitResult.stats?.catalogCourses ?? 0,
                adminClasses: splitResult.stats?.adminClasses ?? 0,
                teachingClasses: splitResult.stats?.teachingClasses ?? 0,
                offerings: splitResult.stats?.offerings ?? 0,
                sessions: splitResult.stats?.sessions ?? 0,
                skippedEmptyAdmin: splitResult.stats?.skippedEmptyAdmin ?? 0,
                skippedParallel: splitResult.stats?.skippedParallel ?? 0,
                noTeacherFilled: splitResult.stats?.noTeacherFilled ?? 0,
              })}
              lowContrast
              hideCloseButton
              className="imports-page__upload-err"
            />
            {splitResult.warnings?.length > 0 && (
              <details className="imports-page__warnings">
                <summary>{t('split.warningsSummary', { count: splitResult.warnings.length })}</summary>
                <ul>
                  {splitResult.warnings.slice(0, 50).map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                  {splitResult.warnings.length > 50 && (
                    <li>{t('split.warningsMore', { count: splitResult.warnings.length - 50 })}</li>
                  )}
                </ul>
              </details>
            )}
          </div>
        )}
      </Column>

      <Column sm={4} md={8} lg={16}>
        <DataTable rows={rows} headers={tableHeaders}>
          {({ rows, headers: renderedHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('table.searchPlaceholder')} />
                </TableToolbarContent>
              </TableToolbar>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {renderedHeaders.map((header) => (
                      <TableHeader key={header.key} {...getHeaderProps({ header })}>
                        {header.header}
                      </TableHeader>
                    ))}
                    <TableHeader>{t('field.actions')}</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>{t('empty.loading')}</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>{list.q ? t('empty.search') : t('empty.none')}</TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const j = list.items.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'type') {
                              return <TableCell key={cell.id}>{typeLabel(cell.value)}</TableCell>
                            }
                            if (cell.info.header === 'status') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={statusKind[cell.value] ?? 'gray'} size="sm">
                                    {t('status.' + cell.value, { defaultValue: cell.value })}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value}</TableCell>
                          })}
                          <TableCell>
                            {j && j.status === 'preview' ? (
                              <Button kind="ghost" size="sm" onClick={() => openDetail(j)}>
                                {t('action.viewPreview')}
                              </Button>
                            ) : j && (j.failedRows > 0 || j.status === 'failed') ? (
                              <Button kind="ghost" size="sm" onClick={() => openDetail(j)}>
                                {t('action.viewDetail')}
                              </Button>
                            ) : (
                              <span style={{ color: 'var(--cds-text-secondary)' }}>-</span>
                            )}
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </DataTable>
        <ListPagination
          page={list.page}
          pageSize={list.pageSize}
          totalItems={list.total}
          onPageChange={list.setPage}
          onPageSizeChange={list.setPageSize}
        />
      </Column>

      <Modal
        open={Boolean(detailJob) || detailLoading}
        modalHeading={
          detailJob?.status === 'preview'
            ? t('modal.previewHeading', { label: typeLabel(detailJob?.type) })
            : t('modal.detailHeading', { label: typeLabel(detailJob?.type) })
        }
        primaryButtonText={t('modal.close')}
        onRequestClose={closeDetail}
        onRequestSubmit={closeDetail}
      >
        {detailJob ? (
          <>
            <p className="imports-page__summary">
              {detailJob.status === 'preview'
                ? t('modal.previewSummary', { succeeded: detailJob.succeededRows, failed: detailJob.failedRows, total: detailJob.totalRows })
                : t('modal.detailSummary', { succeeded: detailJob.succeededRows, failed: detailJob.failedRows, total: detailJob.totalRows })}
            </p>

            {detailJob.status === 'preview' && (
              <div className="imports-page__actions">
                <Button
                  size="sm"
                  onClick={handleCommit}
                  disabled={actionPending || detailJob.succeededRows === 0}
                >
                  {t('modal.commit')}
                </Button>
                <Button kind="ghost" size="sm" onClick={handleCancel} disabled={actionPending}>
                  {t('modal.cancel')}
                </Button>
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

            {detailLoading ? (
              <p className="imports-page__summary">{t('modal.loadingPreview')}</p>
            ) : shownRows.length > 0 ? (
              <div className="imports-page__rows">
                <table>
                  <thead>
                    <tr>
                      {previewColumns.map((c) => (
                        <th key={c.key}>{c.header}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {shownRows.map((r, i) => (
                      <tr key={i}>
                        {previewColumns.map((c) => (
                          <td key={c.key}>{formatCell(r[c.key])}</td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
                {rowsTruncated > 0 && (
                  <p className="imports-page__rows-cap">
                    {t('modal.rowsCap', { shown: shownRows.length, total: previewRows.length })}
                  </p>
                )}
              </div>
            ) : null}

            <ul className="imports-page__errors">
              {errorList.map((e, i) => (
                <li key={i}>
                  {e.row > 0 ? t('modal.errorRow', { row: e.row }) : ''}
                  {e.error}
                </li>
              ))}
              {errorList.length === 0 && <li>{t('modal.noErrors')}</li>}
            </ul>
          </>
        ) : (
          <p className="imports-page__summary">{t('modal.loading')}</p>
        )}
      </Modal>
    </Grid>
  )
}
