import { ensureAuth } from '../../utils/auth'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { uploadFile } from '../../utils/upload'
import { importStatus, formatDateTime } from '../../utils/format'

// 8 类导入的契约,与 web 端 IMPORT_TYPES 一致(schema/note/columns)。
const IMPORT_TYPES: Record<string, { label: string; schema: string; note: string; columns: { key: string; header: string }[] }> = {
  sessions: {
    label: '课表（课次）',
    schema: 'date, period_start, period_end, classroom, course, teaching_class, semester, note',
    note: '教室与开课需预先建立，按名称引用；按教室+日期+节次区间去重，冲突行跳过。period_end 可省略，默认为 period_start。',
    columns: [
      { key: 'date', header: '日期' },
      { key: 'periodStart', header: '起始节次' },
      { key: 'periodEnd', header: '结束节次' },
      { key: 'classroom', header: '教室' },
      { key: 'course', header: '课程' },
      { key: 'teachingClass', header: '教学班' },
      { key: 'semester', header: '学期' },
      { key: 'note', header: '备注' }
    ]
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
      { key: 'description', header: '备注' }
    ]
  },
  admin_classes: {
    label: '行政班',
    schema: 'grade, name, note',
    note: '按年级+班级名称 upsert。',
    columns: [
      { key: 'grade', header: '年级' },
      { key: 'name', header: '班级' },
      { key: 'note', header: '备注' }
    ]
  },
  teaching_classes: {
    label: '教学班',
    schema: 'name, note, admin_grade, admin_name',
    note: '父子表扁平化：每个成员行政班一行，按 name 分组。被开课引用的教学班成员不可修改。',
    columns: [
      { key: 'name', header: '教学班' },
      { key: 'note', header: '备注' },
      { key: 'admin_classes', header: '成员行政班' }
    ]
  },
  catalog: {
    label: '课程库',
    schema: 'name, code, credits, total_hours, category, exam_type, description',
    note: '按课程名称 upsert。code 留空存 NULL；credits/total_hours/category/exam_type 为可选的教务处属性。',
    columns: [
      { key: 'name', header: '课程' },
      { key: 'code', header: '代码' },
      { key: 'credits', header: '学分' },
      { key: 'totalHours', header: '总学时' },
      { key: 'category', header: '课程类别' },
      { key: 'examType', header: '考核方式' },
      { key: 'description', header: '说明' }
    ]
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
      { key: 'note', header: '备注' }
    ]
  },
  regimes: {
    label: '作息制度',
    schema: 'regime_name, effective_month, effective_day, period_index, start_time, end_time',
    note: '父子表扁平化：每节次一行，按 regime_name 分组；提交时整套替换该制度的节次。',
    columns: [
      { key: 'name', header: '制度' },
      { key: 'effectiveMonth', header: '生效月' },
      { key: 'effectiveDay', header: '生效日' },
      { key: 'periods', header: '节次' }
    ]
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
      { key: 'purpose', header: '事由' }
    ]
  }
}

const TYPE_KEYS = Object.keys(IMPORT_TYPES)

interface Job {
  id: number
  type: string
  status: string
  filename: string
  totalRows: number
  succeededRows: number
  failedRows: number
  errorReport: string
  createdAt: string
}

interface JobDetail extends Job {
  rows: Record<string, any>[]
}

/** 预览单元格展示:数组/对象格式化成可读文本。 */
function fmtCell(v: any): string {
  if (v === null || v === undefined) return ''
  if (Array.isArray(v)) {
    return v
      .map((x) => {
        if (x && typeof x === 'object') {
          return [x.periodIndex, x.startTime, x.endTime].filter(Boolean).join('-') || JSON.stringify(x)
        }
        return String(x)
      })
      .join('，')
  }
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

const POLL_INTERVAL = 3000

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    typeKeys: TYPE_KEYS,
    typeLabels: TYPE_KEYS.map((k) => IMPORT_TYPES[k].label),
    typeIndex: 0,
    typeNote: IMPORT_TYPES[TYPE_KEYS[0]].note,
    uploading: false,
    actionError: '',
    // jwc_split
    jwcOpen: false,
    jwcSemester: '',
    jwcWeek1Monday: '',
    jwcUploading: false,
    jwcResult: null as any,
    jwcError: '',
    // 详情弹层
    detailOpen: false,
    detailLoading: false,
    detail: null as any,
    detailView: null as any,
    detailError: '',
    detailActing: false
  },

  _list: null as any,
  _pollTimer: 0,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this._list = createPagedList({
      path: '/api/imports',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((j: Job) => this.jobView(j))
        this.setData({ list: { ...p, items: view } })
        this.syncPolling()
      }
    })
    this._list.load()
  },

  onUnload() {
    this.stopPolling()
  },

  onHide() {
    this.stopPolling()
  },

  onShow() {
    if (this._list) {
      this._list.reload()
      this.syncPolling()
    }
  },

  jobView(j: Job) {
    const st = importStatus[j.status] || { text: j.status, theme: 'gray' }
    return {
      ...j,
      typeLabel: (IMPORT_TYPES[j.type] || { label: j.type }).label,
      statusText: st.text,
      statusTheme: st.theme,
      createdAtText: formatDateTime(j.createdAt),
      canPreview: j.status === 'preview',
      canInspect: j.failedRows > 0 || j.status === 'failed'
    }
  },

  /** 仅当存在 pending/processing 任务时轮询(镜像 web 端 3s 轮询)。 */
  syncPolling() {
    const hasActive = this.data.list.items.some((j: any) => j.status === 'pending' || j.status === 'processing')
    if (hasActive && !this._pollTimer) {
      this._pollTimer = setInterval(() => this._list.reload(), POLL_INTERVAL)
    } else if (!hasActive && this._pollTimer) {
      this.stopPolling()
    }
  },

  stopPolling() {
    if (this._pollTimer) {
      clearInterval(this._pollTimer)
      this._pollTimer = 0
    }
  },

  // ---- 上传 ----

  onTypeChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ typeIndex: idx, typeNote: IMPORT_TYPES[TYPE_KEYS[idx]].note })
  },

  async onChooseFile() {
    const type = TYPE_KEYS[this.data.typeIndex]
    wx.chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['xlsx'],
      success: async (res) => {
        const file = res.tempFiles && res.tempFiles[0]
        if (!file) return
        this.setData({ uploading: true, actionError: '' })
        try {
          await uploadFile({ path: `/api/imports/${type}`, filePath: file.path })
          wx.showToast({ title: '已提交导入', icon: 'success' })
          this._list.reload()
        } catch (err: any) {
          this.setData({ actionError: (err && err.message) || '上传失败' })
        } finally {
          this.setData({ uploading: false })
        }
      }
    })
  },

  // ---- jwc_split ----

  openJwc() {
    this.setData({ jwcOpen: true, jwcSemester: '', jwcWeek1Monday: '', jwcResult: null, jwcError: '' })
  },

  closeJwc() {
    if (this.data.jwcUploading) return
    this.setData({ jwcOpen: false })
  },

  onJwcSemesterInput(e: WechatMiniprogram.Input) {
    this.setData({ jwcSemester: e.detail.value })
  },

  onJwcDateChange(e: WechatMiniprogram.PickerChange) {
    this.setData({ jwcWeek1Monday: String(e.detail.value) })
  },

  async onJwcChooseFile() {
    if (!this.data.jwcSemester.trim()) {
      this.setData({ jwcError: '请填写学期' })
      return
    }
    if (!this.data.jwcWeek1Monday) {
      this.setData({ jwcError: '请选择第 1 周周一' })
      return
    }
    wx.chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['xlsx'],
      success: async (res) => {
        const file = res.tempFiles && res.tempFiles[0]
        if (!file) return
        this.setData({ jwcUploading: true, jwcError: '' })
        try {
          const result = await uploadFile<{ jobs: any[]; stats: any; warnings: string[] }>({
            path: '/api/imports/jwc_split',
            filePath: file.path,
            formData: { semester: this.data.jwcSemester.trim(), week1_monday: this.data.jwcWeek1Monday }
          })
          this.setData({ jwcResult: result })
          this._list.reload()
        } catch (err: any) {
          this.setData({ jwcError: (err && err.message) || '上传失败' })
        } finally {
          this.setData({ jwcUploading: false })
        }
      }
    })
  },

  // ---- 详情/预览 ----

  async openDetail(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    this.setData({ detailOpen: true, detailLoading: true, detail: null, detailView: null, detailError: '' })
    try {
      const job = await request<JobDetail>({ path: `/api/imports/${id}` })
      const cfg = IMPORT_TYPES[job.type] || { columns: [] }
      const errors: { row: number; error: string }[] = (() => {
        try {
          return JSON.parse(job.errorReport || '[]')
        } catch {
          return []
        }
      })()
      const maxPreview = 1000
      const rows = (job.rows || []).slice(0, maxPreview)
      this.setData({
        detail: job,
        detailView: {
          ...this.jobView(job),
          isPreview: job.status === 'preview',
          columns: cfg.columns,
          // 预览行:每行按 columns 顺序取单元格文本
          rows: rows.map((r) => cfg.columns.map((c) => fmtCell(r[c.key]))),
          truncated: (job.rows || []).length > maxPreview,
          rowCount: (job.rows || []).length,
          errors: errors.slice(0, 100),
          errorCount: errors.length
        }
      })
    } catch (err: any) {
      this.setData({ detailError: (err && err.message) || '加载失败' })
    } finally {
      this.setData({ detailLoading: false })
    }
  },

  closeDetail() {
    if (this.data.detailActing) return
    this.setData({ detailOpen: false })
  },

  /** 底部取消键:预览态=取消导入,明细态=仅关闭。 */
  onDetailCancel(e: WechatMiniprogram.TouchEvent) {
    if (!this.data.detailView || !this.data.detailView.isPreview) {
      this.closeDetail()
      return
    }
    const { id } = e.currentTarget.dataset
    // 复用取消任务逻辑(不带 dataset 时从 detail 取 id)
    const jobId = id || (this.data.detail && this.data.detail.id)
    if (jobId) this.onCancelJob({ currentTarget: { dataset: { id: jobId } } } as any)
  },

  async onCommit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    this.setData({ detailActing: true, detailError: '' })
    try {
      await request({ path: `/api/imports/${id}/commit`, method: 'POST' })
      this.setData({ detailOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ detailError: (err && err.message) || '操作失败' })
    } finally {
      this.setData({ detailActing: false })
    }
  },

  async onCancelJob(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    this.setData({ detailActing: true, detailError: '' })
    try {
      await request({ path: `/api/imports/${id}/cancel`, method: 'POST' })
      this.setData({ detailOpen: false })
      this._list.reload()
    } catch (err: any) {
      this.setData({ detailError: (err && err.message) || '操作失败' })
    } finally {
      this.setData({ detailActing: false })
    }
  },

  onReachBottom() {
    this._list.loadMore()
  }
})
