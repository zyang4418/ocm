import { useEffect, useRef, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  DataTable,
  Grid,
  InlineNotification,
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
  type DataTableHeader,
  type TagProps,
} from '@carbon/react'
import { Upload } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiUpload } from '../auth/api'
import { IMPORT_TYPES } from '../components/ImportPreviewTable'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import { formatDate } from '../i18n/formatters'
import type { ImportJob } from '../types/api'

const STATUS_KIND: Record<string, TagProps<'div'>['type']> = {
  pending: 'blue',
  processing: 'blue',
  preview: 'purple',
  succeeded: 'green',
  failed: 'red',
  cancelled: 'gray',
}

// ImportsPage (/imports) is the regular single-file import: pick a type, upload
// an xlsx, then track the resulting jobs in the list. The 教务处 split flow
// lives on its own page (/imports/split), and each job's full preview lives on
// its own page (/imports/:id) — so the large preview tables are never crammed
// into a modal. This page only owns the upload form + the job list.
export default function ImportsPage() {
  const { t } = useTranslation('imports')
  const { token } = useAuth()
  const navigate = useNavigate()
  const fileRef = useRef<HTMLInputElement>(null)

  const headers: DataTableHeader[] = [
    { key: 'type', header: t('field.type') },
    { key: 'filename', header: t('field.filename') },
    { key: 'status', header: t('field.status') },
    { key: 'totalRows', header: t('field.totalRows') },
    { key: 'succeededRows', header: t('field.succeededRows') },
    { key: 'failedRows', header: t('field.failedRows') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

  const list = usePagedList<ImportJob>({ path: '/api/imports', token })
  const { loading } = list

  const [importType, setImportType] = useState('sessions')
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [uploading, setUploading] = useState(false)
  const [uploadError, setUploadError] = useState('')

  // Poll while any job is still pending or processing. The poll refetches the
  // current page (a fresh job always lands on page 1, newest first).
  useEffect(() => {
    const hasActive = list.items.some((j) => j.status === 'pending' || j.status === 'processing')
    if (!hasActive) return undefined
    const timer = setInterval(list.reload, 3000)
    return () => clearInterval(timer)
  }, [list.items, list.reload])

  const typeLabel = (type: string) => (type ? t('types.' + type + '.label', { defaultValue: type }) : '-')

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
      setUploadError((err as Error).message)
    } finally {
      setUploading(false)
    }
  }

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

  const colSpan = headers.length + 1
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
            {Object.keys(IMPORT_TYPES).map((value) => (
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
          <strong>{typeLabel(importType)}</strong>
          {t('schemaIntroTail')}
          <code>{typeCfg?.schema}</code>
        </p>
        <p className="imports-page__note">{t('types.' + importType + '.note')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        <DataTable rows={rows} headers={headers}>
          {({ rows: tableRows, headers: renderedHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
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
                      <TableHeader {...getHeaderProps({ header })}>
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
                  ) : tableRows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>{list.q ? t('empty.search') : t('empty.none')}</TableCell>
                    </TableRow>
                  ) : (
                    tableRows.map((row) => {
                      const j = list.items.find((x) => String(x.id) === String(row.id))
                      const viewable = j && j.status !== 'pending' && j.status !== 'processing'
                      return (
                        <TableRow {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'type') {
                              return <TableCell key={cell.id}>{typeLabel(cell.value as string)}</TableCell>
                            }
                            if (cell.info.header === 'status') {
                              const value = cell.value as string
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={STATUS_KIND[value] ?? 'gray'} size="sm">
                                    {t('status.' + value, { defaultValue: value })}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value as string)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value as string | number}</TableCell>
                          })}
                          <TableCell>
                            {j && viewable ? (
                              <Button kind="ghost" size="sm" onClick={() => navigate(`/imports/${j.id}`)}>
                                {j.status === 'preview' ? t('action.viewPreview') : t('action.viewDetail')}
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
    </Grid>
  )
}
