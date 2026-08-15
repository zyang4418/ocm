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
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch, apiUpload } from '../auth/api.js'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'

// IMPORT_TYPES describes each business-table import: the upload label, the
// xlsx header contract (order-independent, matched by name), and the preview
// columns used to render the dry-run rows. The column keys mirror the
// toPreviewMap() keys produced by each backend importer.
const IMPORT_TYPES = {
  sessions: {
    label: '课表（课次）',
    schema: 'date, period_start, period_end, classroom, course, teaching_class, semester, note',
    note: '教室与开课需预先建立，按名称引用；按教室+日期+节次区间去重，冲突行跳过。period_end 可省略，默认为 period_start（连上多节如 3-4 为一条课次）。',
    columns: [
      { key: 'date', header: '日期' },
      { key: 'periodStart', header: '起始节次' },
      { key: 'periodEnd', header: '结束节次' },
      { key: 'classroom', header: '教室' },
      { key: 'course', header: '课程' },
      { key: 'teachingClass', header: '教学班' },
      { key: 'semester', header: '学期' },
      { key: 'note', header: '备注' },
    ],
  },
  classrooms: {
    label: '教室',
    schema: 'name, building, capacity, type, floor, campus, status, description',
    note: '按教室名称 upsert：已存在则更新，否则新增。floor/campus 为可选的楼层与校区。',
    columns: [
      { key: 'name', header: '教室编号' },
      { key: 'building', header: '楼栋' },
      { key: 'capacity', header: '座位数' },
      { key: 'type', header: '类型' },
      { key: 'floor', header: '楼层' },
      { key: 'campus', header: '校区' },
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
    schema: 'name, code, credits, total_hours, category, exam_type, description',
    note: '按课程名称 upsert。code 加唯一索引（留空存 NULL，互不冲突）；credits/total_hours/category/exam_type 为可选的教务处属性。',
    columns: [
      { key: 'name', header: '课程' },
      { key: 'code', header: '代码' },
      { key: 'credits', header: '学分' },
      { key: 'totalHours', header: '总学时' },
      { key: 'category', header: '课程类别' },
      { key: 'examType', header: '考核方式' },
      { key: 'description', header: '说明' },
    ],
  },
  offerings: {
    label: '开课',
    schema: 'course, teaching_class, semester, teacher, course_seq, teacher_id, teacher_title, college, max_students, requirement, weekly_hours, note',
    note: '按课程+教学班+学期 upsert；课程与教学班按名称引用，需预先建立。course_seq..weekly_hours 为可选的教务处开课元数据。',
    columns: [
      { key: 'course', header: '课程' },
      { key: 'teachingClass', header: '教学班' },
      { key: 'semester', header: '学期' },
      { key: 'teacher', header: '教师' },
      { key: 'courseSeq', header: '课程序号' },
      { key: 'teacherId', header: '教师工号' },
      { key: 'teacherTitle', header: '教师职称' },
      { key: 'college', header: '开课学院' },
      { key: 'maxStudents', header: '人数上限' },
      { key: 'requirement', header: '课程类别一' },
      { key: 'weeklyHours', header: '周学时' },
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
    const t = setInterval(list.reload, 3000)
    return () => clearInterval(t)
  }, [list.items, list.reload])

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
  const previewColumns = (detailJob && IMPORT_TYPES[detailJob.type]?.columns) || []

  const rows = list.items.map((j) => ({
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
        {list.error && (
          <InlineNotification
            kind="error"
            title="加载失败"
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
        <div className="imports-page__split-head">
          <h2 className="imports-page__subheading">教务处课表拆分</h2>
          <p className="courses-page__subtitle">
            上传教务处导出的聚合课表（含教室 / 课程 / 行政班 / 教师 / 周次），填学期与第一周
            周一，系统自动拆为 6 个导入任务（教室 / 课程库 / 行政班 / 教学班 / 开课 / 课次），
            各自进入预览。请按依赖顺序确认：教室 → 课程库 → 行政班 → 教学班 → 开课 → 课次
            （开课与课次引用课程库与教学班，未先确认时预览会报「不存在」，确认后重校通过）。
          </p>
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
            labelText="学期"
            placeholder="2024-2025-2"
            value={splitSemester}
            onChange={(e) => setSplitSemester(e.target.value)}
            size="sm"
          />
          <TextInput
            id="jwc-week1"
            className="imports-page__split-input"
            type="date"
            labelText="第一周周一（须为周一）"
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
            {splitting ? '拆分中…' : '拆分并建任务'}
          </Button>
        </div>
        {splitError && (
          <InlineNotification
            kind="error"
            title="拆分失败"
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
              title={`拆分完成，已建 ${splitResult.jobs?.length ?? 6} 个导入任务`}
              subtitle={`教室 ${splitResult.stats?.classrooms ?? 0} · 课程 ${splitResult.stats?.catalogCourses ?? 0} · 行政班 ${splitResult.stats?.adminClasses ?? 0} · 教学班 ${splitResult.stats?.teachingClasses ?? 0} · 开课 ${splitResult.stats?.offerings ?? 0} · 课次 ${splitResult.stats?.sessions ?? 0}（跳过 空行政班 ${splitResult.stats?.skippedEmptyAdmin ?? 0} / 平行 ${splitResult.stats?.skippedParallel ?? 0} / 无教师填未安排 ${splitResult.stats?.noTeacherFilled ?? 0}）`}
              lowContrast
              hideCloseButton
              className="imports-page__upload-err"
            />
            {splitResult.warnings?.length > 0 && (
              <details className="imports-page__warnings">
                <summary>告警 {splitResult.warnings.length} 条（点击展开）</summary>
                <ul>
                  {splitResult.warnings.slice(0, 50).map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                  {splitResult.warnings.length > 50 && (
                    <li>…其余 {splitResult.warnings.length - 50} 条省略</li>
                  )}
                </ul>
              </details>
            )}
          </div>
        )}
      </Column>

      <Column sm={4} md={8} lg={16}>
        <DataTable rows={rows} headers={headers}>
          {({ rows, headers: tableHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title="导入任务" description={`共 ${list.total} 个任务`}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder="搜索导入任务" />
                </TableToolbarContent>
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
                      <TableCell colSpan={colSpan}>{list.q ? '未找到匹配的导入任务' : '暂无导入任务'}</TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const j = list.items.find((x) => String(x.id) === String(row.id))
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
