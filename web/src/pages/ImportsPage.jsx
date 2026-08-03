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

// External docs site; when unset the docs link is hidden.
const docsUrl = import.meta.env.VITE_DOCS_URL || ''
const importGuideUrl = docsUrl ? `${docsUrl.replace(/\/$/, '')}/guide/import` : ''

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

export default function ImportsPage() {
  const { token } = useAuth()
  const navigate = useNavigate()
  const fileRef = useRef(null)

  const [jobs, setJobs] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

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
      await apiUpload('/api/imports/sessions', { file: selectedFile, token })
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

  const rows = jobs.map((j) => ({
    id: String(j.id),
    filename: j.filename || '(未命名)',
    status: j.status,
    totalRows: j.totalRows,
    succeededRows: j.succeededRows,
    failedRows: j.failedRows,
    createdAt: j.createdAt,
  }))

  const colSpan = headers.length + 1

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
          <BreadcrumbItem isCurrentPage>课表导入</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">课表导入</h1>
        <p className="courses-page__subtitle">
          上传 CSV 课表文件异步导入。教室与开课需预先在系统中建立，CSV 按名称引用。
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
          <input
            ref={fileRef}
            type="file"
            accept=".csv,text/csv"
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
          CSV 表头（按列名识别，顺序无关）：<code>date, period_index, classroom, course, class, semester, note</code>
        </p>
        {importGuideUrl && (
          <p className="imports-page__skill">
            课表是 Excel？先
            <a href={importGuideUrl} target="_blank" rel="noreferrer">
              阅读导入文档
            </a>
            ，按其中的契约用 AI 转换并上传。
          </p>
        )}
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
                            ) : j && j.failedRows > 0 ? (
                              <Button kind="ghost" size="sm" onClick={() => openDetail(j)}>
                                查看错误
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
        modalHeading={detailJob?.status === 'preview' ? '预览：确认导入内容' : '导入明细'}
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
                      <th>日期</th>
                      <th>节次</th>
                      <th>教室</th>
                      <th>课程</th>
                      <th>班级</th>
                      <th>学期</th>
                      <th>备注</th>
                    </tr>
                  </thead>
                  <tbody>
                    {shownRows.map((r, i) => (
                      <tr key={i}>
                        <td>{r.date}</td>
                        <td>{r.periodIndex}</td>
                        <td>{r.classroom}</td>
                        <td>{r.course}</td>
                        <td>{r.class}</td>
                        <td>{r.semester}</td>
                        <td>{r.note}</td>
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
