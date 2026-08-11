import { useCallback, useEffect, useRef, useState } from 'react'
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
  Tag,
} from '@carbon/react'
import { Upload } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch, apiUpload } from '../auth/api.js'

// IMPORT_TYPES describes each business-table import: the upload label, the
// xlsx header contract (order-independent, matched by name), and the preview
// columns used to render the dry-run rows. The column keys mirror the
// toPreviewMap() keys produced by each backend importer.
const IMPORT_TYPES = {
  sessions: {
    label: '课表（课次）',
    schema: 'date, period_index, classroom, course, teaching_class, semester, note',
    note: '教室与开课需预先建立，按名称引用；按教室+日期+节次去重，冲突行跳过。',
    columns: [
      { key: 'date', header: '日期' },
      { key: 'periodIndex', header: '节次' },
      { key: 'classroom', header: '教室' },
      { key: 'course', header: '课程' },
      { key: 'teachingClass', header: '教学班' },
      { key: 'semester', header: '学期' },
      { key: 'note', header: '备注' },
    ],
  },
  classrooms: {
    label: '教室',
    schema: 'name, building, capacity, type, status, description',
    note: '按教室名称 upsert：已存在则更新，否则新增。',
    columns: [
      { key: 'name', header: '教室编号' },
      { key: 'building', header: '楼栋' },
      { key: 'capacity', header: '座位数' },
      { key: 'type', header: '类型' },
      { key: 'status', header: '状态' },
      { key: 'description', header: '备注' },
    ],
  },
  admin_classes: {
    label: '行政班',
    schema: 'grade, name, note',
    note: '按年级+班级名称 upsert。',
    columns: [
      { key: 'grade', header: '年级' },
      { key: 'name', header: '班级' },
      { key: 'note', header: '备注' },
    ],
  },
  teaching_classes: {
    label: '教学班',
    schema: 'name, note, admin_grade, admin_name',
    note: '父子表扁平化：每个成员行政班一行，按 name 分组。被开课引用的教学班成员不可修改。',
    columns: [
      { key: 'name', header: '教学班' },
      { key: 'note', header: '备注' },
      { key: 'admin_classes', header: '成员行政班' },
    ],
  },
  catalog: {
    label: '课程库',
    schema: 'name, code, description',
    note: '按课程名称 upsert。',
    columns: [
      { key: 'name', header: '课程' },
      { key: 'code', header: '代码' },
      { key: 'description', header: '说明' },
    ],
  },
  offerings: {
    label: '开课',
    schema: 'course, teaching_class, semester, teacher, note',
    note: '按课程+教学班+学期 upsert；课程与教学班按名称引用，需预先建立。',
    columns: [
      { key: 'course', header: '课程' },
      { key: 'teachingClass', header: '教学班' },
      { key: 'semester', header: '学期' },
      { key: 'teacher', header: '教师' },
      { key: 'note', header: '备注' },
    ],
  },
  regimes: {
    label: '作息制度',
    schema: 'regime_name, effective_month, effective_day, period_index, start_time, end_time',
    note: '父子表扁平化：每节次一行，按 regime_name 分组；提交时整套替换该制度的节次。',
    columns: [
      { key: 'name', header: '制度' },
      { key: 'effectiveMonth', header: '生效月' },
      { key: 'effectiveDay', header: '生效日' },
      { key: 'periods', header: '节次' },
    ],
  },
  bookings: {
    label: '教室预约',
    schema: 'classroom, username, date, period_start, period_end, status, purpose',
    note: '恢复模式：按文件中的 status 还原。pending/approved 行占用时段并做冲突校验。',
    columns: [
      { key: 'classroom', header: '教室' },
      { key: 'username', header: '预约人' },
      { key: 'date', header: '日期' },
      { key: 'periodStart', header: '起始节次' },
      { key: 'periodEnd', header: '结束节次' },
      { key: 'status', header: '状态' },
      { key: 'purpose', header: '事由' },
    ],
  },
}

// The ordered list shown in the type selector.
const TYPE_OPTIONS = Object.entries(IMPORT_TYPES).map(([value, cfg]) => ({
  value,
  label: cfg.label,
}))

const statusLabel = {
  pending: '待处理',
  processing: '处理中',
  preview: '待确认',
  succeeded: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

const statusKind = {
  pending: 'blue',
  processing: 'blue',
  preview: 'purple',
  succeeded: 'green',
  failed: 'red',
  cancelled: 'gray',
}

const headers = [
  { key: 'type', header: '类型' },
  { key: 'filename', header: '文件' },
  { key: 'status', header: '状态' },
  { key: 'totalRows', header: '总行数' },
  { key: 'succeededRows', header: '成功' },
  { key: 'failedRows', header: '失败' },
  { key: 'createdAt', header: '创建时间' },
]

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// typeLabelOf maps a job's type string to a Chinese label; unknown types fall
// back to the raw value so the list stays readable even for stale/unregistered
// types.
function typeLabelOf(type) {
  return IMPORT_TYPES[type]?.label ?? type ?? '-'
}

// formatCell renders a preview cell value. Arrays (teaching-class members,
// regime periods) are joined into a readable string; other values pass through.
function formatCell(value) {
  if (Array.isArray(value)) {
    if (value.length === 0) return ''
    // Regime periods are objects {periodIndex, startTime, endTime}; teaching
    // class members are plain label strings. Detect by element type.
    if (typeof value[0] === 'object' && value[0] !== null) {
      return value
        .map((p) => `${p.periodIndex}(${p.startTime}-${p.endTime})`)
        .join('，')
    }
    return value.join('，')
  }
  if (value === null || value === undefined) return ''
  return String(value)
}

export default function ImportsPage() {
  const { token } = useAuth()
  const navigate = useNavigate()
  const fileRef = useRef(null)

  const [jobs, setJobs] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [importType, setImportType] = useState('sessions')
  const [selectedFile, setSelectedFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [uploadError, setUploadError] = useState('')

  const [detailJob, setDetailJob] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [actionPending, setActionPending] = useState(false)
  const [actionError, setActionError] = useState('')

  const fetchJobs = useCallback(async () => {
    try {
      setError('')
      const data = await apiFetch('/api/imports', { token })
      setJobs(Array.isArray(data) ? data : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchJobs()
  }, [fetchJobs])

  // Poll while any job is still pending or processing.
  useEffect(() => {
    const hasActive = jobs.some((j) => j.status === 'pending' || j.status === 'processing')
    if (!hasActive) return undefined
    const t = setInterval(fetchJobs, 3000)
    return () => clearInterval(t)
  }, [jobs, fetchJobs])

  const handleUpload = async () => {
    if (!selectedFile) return
    try {
      setUploading(true)
      setUploadError('')
      await apiUpload(`/api/imports/${importType}`, { file: selectedFile, token })
      setSelectedFile(null)
      if (fileRef.current) fileRef.current.value = ''
      await fetchJobs()
    } catch (err) {
      setUploadError(err.message)
    } finally {
      setUploading(false)
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
      await fetchJobs()
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
      await fetchJobs()
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
  const previewColumns = (detailJob && IMPORT_TYPES[detailJob.type]?.columns) || []

  const rows = jobs.map((j) => ({
    id: String(j.id),
    type: j.type,
    filename: j.filename || '(未命名)',
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
          <BreadcrumbItem isCurrentPage>数据导入</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">数据导入</h1>
        <p className="courses-page__subtitle">
          上传 xlsx 文件异步导入业务数据。先选择导入类型，再上传文件；解析后可预览并确认。
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
      </Column>

      <Column sm={4} md={8} lg={16}>
        <div className="imports-page__upload">
          <Select
            id="import-type"
            className="imports-page__type-select"
            labelText="导入类型"
            value={importType}
            onChange={(e) => setImportType(e.target.value)}
            size="sm"
          >
            {TYPE_OPTIONS.map((t) => (
              <SelectItem key={t.value} value={t.value} text={t.label} />
            ))}
          </Select>
          <input
            ref={fileRef}
            type="file"
            accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            onChange={(e) => setSelectedFile(e.target.files?.[0] || null)}
          />
          <Button renderIcon={Upload} size="sm" onClick={handleUpload} disabled={!selectedFile || uploading}>
            {uploading ? '上传中…' : '上传导入'}
          </Button>
          {uploadError && (
            <InlineNotification
              kind="error"
              title="上传失败"
              subtitle={uploadError}
              lowContrast
              hideCloseButton
              className="imports-page__upload-err"
            />
          )}
        </div>
        <p className="imports-page__schema">
          <strong>{typeCfg.label}</strong> 表头（按列名识别，顺序无关）：
          <code>{typeCfg.schema}</code>
        </p>
        <p className="imports-page__note">{typeCfg.note}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        <DataTable rows={rows} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title="导入任务" description={`共 ${jobs.length} 个任务`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent />
              </TableToolbar>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {tableHeaders.map((header) => (
                      <TableHeader key={header.key} {...getHeaderProps({ header })}>
                        {header.header}
                      </TableHeader>
                    ))}
                    <TableHeader>操作</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>加载中…</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={colSpan}>暂无导入任务</TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const j = jobs.find((x) => String(x.id) === String(row.id))
                      return (
                        <TableRow key={row.id} {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'type') {
                              return <TableCell key={cell.id}>{typeLabelOf(cell.value)}</TableCell>
                            }
                            if (cell.info.header === 'status') {
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={statusKind[cell.value] ?? 'gray'} size="sm">
                                    {statusLabel[cell.value] ?? cell.value}
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
                                查看预览
                              </Button>
                            ) : j && (j.failedRows > 0 || j.status === 'failed') ? (
                              <Button kind="ghost" size="sm" onClick={() => openDetail(j)}>
                                查看明细
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
      </Column>

      <Modal
        open={Boolean(detailJob) || detailLoading}
        modalHeading={
          detailJob?.status === 'preview'
            ? `预览：确认导入「${typeLabelOf(detailJob?.type)}」`
            : `导入明细：${typeLabelOf(detailJob?.type)}`
        }
        primaryButtonText="关闭"
        onRequestClose={closeDetail}
        onRequestSubmit={closeDetail}
      >
        {detailJob ? (
          <>
            <p className="imports-page__summary">
              {detailJob.status === 'preview'
                ? `将导入 ${detailJob.succeededRows} / 失败 ${detailJob.failedRows} / 总 ${detailJob.totalRows} 行`
                : `成功 ${detailJob.succeededRows} / 失败 ${detailJob.failedRows} / 总 ${detailJob.totalRows} 行`}
            </p>

            {detailJob.status === 'preview' && (
              <div className="imports-page__actions">
                <Button
                  size="sm"
                  onClick={handleCommit}
                  disabled={actionPending || detailJob.succeededRows === 0}
                >
                  确认导入
                </Button>
                <Button kind="ghost" size="sm" onClick={handleCancel} disabled={actionPending}>
                  取消导入
                </Button>
              </div>
            )}
            {actionError && (
              <InlineNotification
                kind="error"
                title="操作失败"
                subtitle={actionError}
                lowContrast
                hideCloseButton
                className="imports-page__upload-err"
              />
            )}

            {detailLoading ? (
              <p className="imports-page__summary">加载预览数据…</p>
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
                    仅显示前 {shownRows.length} 行，共 {previewRows.length} 行
                  </p>
                )}
              </div>
            ) : null}

            <ul className="imports-page__errors">
              {errorList.map((e, i) => (
                <li key={i}>
                  {e.row > 0 ? `第 ${e.row} 行：` : ''}
                  {e.error}
                </li>
              ))}
              {errorList.length === 0 && <li>无错误</li>}
            </ul>
          </>
        ) : (
          <p className="imports-page__summary">加载中…</p>
        )}
      </Modal>
    </Grid>
  )
}
