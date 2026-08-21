import { useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Column,
  ComboBox,
  DataTable,
  Grid,
  InlineNotification,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  type DataTableHeader,
} from '@carbon/react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ExportButton from '../components/ExportButton'
import type { OfferingSummary, OfferingView, Paged, SummaryRow } from '../types/api'
import { STATUS_KEYS, StatusTag, formatDateTime, type PickerOption } from './attendanceUi'

// L2 整学期考勤报表: pick one offering → per-student × per-checkin matrix
// with per-status subtotals, downloadable as a two-sheet xlsx.
const TOTAL_KEYS = ['present', 'late', 'absent', 'leave']

// One row of the matrix: fixed identity columns plus one dynamic cell per
// checkin (key `c<checkinId>`, value the record status) and the totals text.
type ReportRow = {
  id: string
  displayName: string
  studentNo: string
  adminClass: string
  totals: string
} & Record<string, string>

export default function AttendanceReportPage() {
  const { t } = useTranslation('attendance')
  const { token } = useAuth()
  const navigate = useNavigate()

  const [offerings, setOfferings] = useState<PickerOption[]>([])
  const [pickOffering, setPickOffering] = useState<PickerOption | null>(null)
  const [summary, setSummary] = useState<OfferingSummary | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const loadOfferings = async () => {
    try {
      const data = await apiFetch<Paged<OfferingView>>('/api/offerings?page_size=500', { token })
      setOfferings(
        (Array.isArray(data?.items) ? data.items : []).map((o) => ({
          id: String(o.id),
          text: t('list.offeringOption', { catalogName: o.catalogName, teachingClassName: o.teachingClassName, semester: o.semester }),
        })),
      )
    } catch {
      setOfferings([])
    }
  }

  const handlePickOffering = (e: { selectedItem?: PickerOption | null }) => {
    setPickOffering(e.selectedItem ?? null)
    setSummary(null)
    setError('')
    if (e.selectedItem) loadSummary(e.selectedItem.id)
  }

  const loadSummary = async (offeringId: string) => {
    try {
      setLoading(true)
      setError('')
      const data = await apiFetch<OfferingSummary>(`/api/checkins/summary?offering_id=${offeringId}`, { token })
      setSummary(data)
    } catch (err) {
      setError((err as Error).message)
      setSummary(null)
    } finally {
      setLoading(false)
    }
  }

  const baseHeaders: DataTableHeader[] = [
    { key: 'displayName', header: t('report.field.name') },
    { key: 'studentNo', header: t('report.field.studentNo') },
    { key: 'adminClass', header: t('report.field.adminClass') },
  ]
  const checkins = summary?.checkins ?? []
  const headers: DataTableHeader[] = [
    ...baseHeaders,
    ...checkins.map((c) => ({ key: `c${c.id}`, header: c.startsAt.slice(0, 10) })),
    { key: 'totals', header: t('report.field.totals') },
  ]

  const buildRow = (r: SummaryRow): ReportRow => {
    const row: ReportRow = { id: String(r.userId), displayName: r.displayName, studentNo: r.studentNo, adminClass: r.adminClass, totals: '' }
    for (const c of checkins) {
      row[`c${c.id}`] = r.records[c.id] ?? ''
    }
    row.totals = TOTAL_KEYS.map((k) => r.totals[k] ?? 0).join(' / ')
    return row
  }
  const rows = (summary?.rows ?? []).map(buildRow)

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
            href="/attendance"
            onClick={(e) => {
              e.preventDefault()
              navigate('/attendance')
            }}
          >
            {t('breadcrumb.attendance')}
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>{t('breadcrumb.report')}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">{t('report.title')}</h1>
        <p className="courses-page__subtitle">{t('report.subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title={t('report.error.load')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
        <ComboBox
          id="report-offering"
          titleText={t('report.offering')}
          placeholder={t('report.offeringPlaceholder')}
          items={offerings}
          itemToString={(item) => (item ? item.text : '')}
          selectedItem={pickOffering}
          onChange={handlePickOffering}
          shouldFilterItem={() => true}
          onFocus={() => {
            if (offerings.length === 0) loadOfferings()
          }}
        />

        {summary && (
          <>
            <p className="courses-page__subtitle">
              {summary.courseName} · {summary.teachingClassName} · {summary.semester} ·{' '}
              {t('report.summaryTeacher', { teacher: summary.teacher || '-' })} ·{' '}
              {t('report.summaryCheckins', { count: checkins.length })} ·{' '}
              {t('report.summaryStudents', { count: summary.rows.length })}
            </p>
            {checkins.length === 0 && (
              <InlineNotification
                kind="warning"
                title={t('report.warning.noCheckinsTitle')}
                subtitle={t('report.warning.noCheckinsSubtitle')}
                lowContrast
                hideCloseButton
                className="courses-page__notice"
              />
            )}
            {summary.rows.every((r) => !r.inRoster) && (
              <InlineNotification
                kind="warning"
                title={t('report.warning.emptyRosterTitle')}
                subtitle={t('report.warning.emptyRosterSubtitle')}
                lowContrast
                hideCloseButton
                className="courses-page__notice"
              />
            )}
          </>
        )}

        {summary && checkins.length > 0 && (
          <DataTable rows={rows} headers={headers}>
            {({ rows: tableRows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
              <TableContainer title={t('report.table.title')} description={t('report.table.description')}>
                <TableToolbar {...getToolbarProps()}>
                  <TableToolbarContent>
                    <ExportButton
                      path={`/api/checkins/export?offering_id=${pickOffering?.id}`}
                      fallbackName="attendance-report.xlsx"
                      onError={setError}
                    />
                  </TableToolbarContent>
                </TableToolbar>
                <Table {...getTableProps()} size="sm">
                  <TableHead>
                    <TableRow>
                      {tableHeaders.map((header) => (
                        <TableHeader {...getHeaderProps({ header })}>
                          {header.header}
                        </TableHeader>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {loading ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>{t('report.empty.loading')}</TableCell>
                      </TableRow>
                    ) : tableRows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={headers.length}>{t('report.empty.none')}</TableCell>
                      </TableRow>
                    ) : (
                      tableRows.map((row) => (
                        <TableRow {...getRowProps({ row })}>
                            {row.cells.map((cell) => {
                              if (cell.info.header === 'displayName' || cell.info.header === 'studentNo' || cell.info.header === 'adminClass') {
                                return <TableCell key={cell.id}>{cell.value || '-'}</TableCell>
                              }
                              if (cell.info.header === 'totals') {
                                return <TableCell key={cell.id}>{cell.value}</TableCell>
                              }
                              const status = cell.value as string
                              return (
                                <TableCell key={cell.id}>
                                  {status ? <StatusTag status={status} /> : <span className="attendance-report__empty">—</span>}
                                </TableCell>
                              )
                            })}
                          </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
          </DataTable>
        )}
        {loading && <p>{t('report.empty.loading')}</p>}
        {!summary && !loading && !error && (
          <p className="courses-page__subtitle">{t('report.noSelection')}</p>
        )}
        {/* Legend for the matrix */}
        {summary && checkins.length > 0 && (
          <p className="courses-page__subtitle">
            {t('report.legend')}{' '}
            {STATUS_KEYS.map((value) => (
              <span key={value} className="attendance-report__legend">
                <StatusTag status={value} /> {t('status.' + value, { ns: 'common' })}
              </span>
            ))}{' '}
            {t('report.lastStarted', { time: formatDateTime(checkins[0]?.startsAt) })}
          </p>
        )}
      </Column>
    </Grid>
  )
}
