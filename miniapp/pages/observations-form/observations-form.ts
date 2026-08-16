import { ensureAuth } from '../../utils/auth'
import { request } from '../../utils/request'

interface RatingOption { label: string; value: string }
interface Indicator { key: string; title: string; lines: string[]; value: string }
interface CommentField { key: string; label: string; placeholder: string; value: string }
interface Extra { key: string; label: string; options: string[]; value: string; detailKey: string; detailRequiredWhen: string; detailPlaceholder: string; detailValue: string }
interface StudentQuestion { key: string; title: string; optionValues: string[]; optionLabels: string[]; answers: string[] }

function indicatorScoreGroups(ind: any): { key: string; lines: string[] }[] {
  if (ind.scoreGroups && ind.scoreGroups.length) {
    return ind.scoreGroups.map((g: any) => ({
      key: g.key,
      lines: (g.lineIndexes || []).map((i: number) => ind.lines?.[i]).filter(Boolean)
    }))
  }
  return [{ key: ind.key, lines: ind.lines || [] }]
}

Page({
  data: {
    mode: 'create' as 'create' | 'edit',
    id: 0,
    loading: true,
    saving: false,
    error: '',
    // 基础信息
    templateOptions: [] as string[],
    templateValues: [] as string[],
    templateIndex: 0,
    offeringOptions: [] as string[],
    offeringValues: [] as string[],
    offeringIndex: 0,
    observeDate: '',
    periodLabels: [] as string[],
    periodValues: [] as number[],
    sections: [] as number[],
    isAnonymous: false,
    // 模板渲染
    ratingOptions: [] as RatingOption[],
    indicatorGroups: [] as Indicator[],
    commentFields: [] as CommentField[],
    postCommentFields: [] as CommentField[],
    postCommentTitle: '',
    headerExtras: [] as Extra[],
    extras: [] as Extra[],
    studentColumns: [] as string[],
    studentQuestions: [] as StudentQuestion[],
    scoreLabel: '总评成绩',
    contentLabel: '授课内容',
    contentLimit: 0,
    totalScore: '',
    contentOutline: ''
  },

  _templates: [] as any[],
  _offerings: [] as any[],
  _periods: [] as any[],
  _raw: null as any,

  async onLoad(options: Record<string, string | undefined>) {
    const ok = await ensureAuth()
    if (!ok) return
    const id = options && options.id ? Number(options.id) : 0
    const mode: 'create' | 'edit' = id ? 'edit' : 'create'
    this.setData({ mode, id })
    try {
      const [sch, off] = await Promise.all([
        request<{ rating_options: RatingOption[]; templates: any[] }>({ path: '/api/observations/templates' }),
        request<{ items: any[] }>({ path: '/api/offerings', params: { page_size: 500 } })
      ])
      this._templates = (sch && sch.templates) || []
      this._offerings = (off && off.items) || []
      this.setData({
        ratingOptions: (sch && sch.rating_options) || [],
        templateOptions: ['请选择模板'].concat(this._templates.map((t) => t.label)),
        templateValues: [''].concat(this._templates.map((t) => t.value)),
        offeringOptions: ['请选择课程'].concat(this._offerings.map((o) => `${o.catalogName}（${o.teachingClassName} · ${o.teacher}）`)),
        offeringValues: [''].concat(this._offerings.map((o: any) => String(o.id)))
      })
      if (mode === 'edit') {
        await this.loadDetail(id)
      }
      this.setData({ loading: false })
    } catch (err: any) {
      this.setData({ loading: false, error: (err && err.message) || '加载失败' })
    }
  },

  async loadDetail(id: number) {
    const v = await request<any>({ path: `/api/observations/${id}` })
    this._raw = v
    const fd = v.formData && typeof v.formData === 'object' ? v.formData : {}
    const tIdx = this._templates.findIndex((t) => t.value === v.templateType)
    const oIdx = this._offerings.findIndex((o: any) => o.id === v.courseId)
    this.setData({
      templateIndex: tIdx >= 0 ? tIdx + 1 : 0,
      offeringIndex: oIdx >= 0 ? oIdx + 1 : 0,
      observeDate: v.observeDate || '',
      sections: Array.isArray(v.sections) ? v.sections : [],
      isAnonymous: Boolean(v.isAnonymous),
      totalScore: v.totalScore != null ? String(v.totalScore) : '',
      contentOutline: fd.contentOutline || v.content || ''
    })
    this.applyTemplateView(this._templates[tIdx], {
      indicatorScores: fd.indicatorScores || {},
      comments: fd.comments || {},
      extraValues: fd.extraValues || {},
      extraDetails: fd.extraDetails || {},
      studentFeedback: fd.studentFeedback || {}
    })
    if (v.observeDate) this.loadPeriods(v.observeDate)
  },

  // ---- 模板渲染结构 ----
  applyTemplateView(t: any, values: any) {
    if (!t) {
      this.setData({
        indicatorGroups: [], commentFields: [], postCommentFields: [], postCommentTitle: '',
        headerExtras: [], extras: [], studentColumns: [], studentQuestions: [],
        scoreLabel: '总评成绩', contentLabel: '授课内容', contentLimit: 0
      })
      return
    }
    const indScores = values.indicatorScores || {}
    const comments = values.comments || {}
    const extraValues = values.extraValues || {}
    const extraDetails = values.extraDetails || {}
    const sf = values.studentFeedback || {}

    const indicatorGroups: Indicator[] = []
    ;(t.indicators || []).forEach((ind: any) => {
      indicatorScoreGroups(ind).forEach((g) => {
        indicatorGroups.push({ key: g.key, title: ind.title, lines: g.lines, value: indScores[g.key] || '' })
      })
    })

    const mkComment = (cf: any): CommentField => ({
      key: cf.key, label: cf.label, placeholder: cf.placeholder || '', value: comments[cf.key] || ''
    })

    const mkExtra = (ex: any): Extra => ({
      key: ex.key,
      label: ex.label,
      options: ex.options || [],
      value: extraValues[ex.key] || '',
      detailKey: ex.detail_key || '',
      detailRequiredWhen: ex.detail_required_when,
      detailPlaceholder: ex.detail_placeholder || '',
      detailValue: ex.detail_key ? (extraDetails[ex.detail_key] || '') : ''
    })

    const columns: string[] = (t.student_feedback && t.student_feedback.columns) || []
    const studentQuestions: StudentQuestion[] = ((t.student_feedback && t.student_feedback.questions) || []).map((q: any) => {
      // answers[i] is a picker index: 0 = unset, 1..n = option index+1.
      const answers = columns.map((col) => {
        const cur = (sf[q.key] && sf[q.key][col]) || ''
        const vi = (q.options || []).findIndex((o: any) => o.value === cur)
        return vi >= 0 ? vi + 1 : 0
      })
      return {
        key: q.key,
        title: q.title,
        optionValues: [''].concat((q.options || []).map((o: any) => o.value)),
        optionLabels: ['—'].concat((q.options || []).map((o: any) => o.label)),
        answers
      }
    })

    this.setData({
      indicatorGroups,
      commentFields: (t.comment_fields || []).map(mkComment),
      postCommentFields: (t.post_content_comment_fields || []).map(mkComment),
      postCommentTitle: t.post_content_comment_title || '其它评价',
      headerExtras: (t.header_extras || []).map(mkExtra),
      extras: (t.extras || []).map(mkExtra),
      studentColumns: columns,
      studentQuestions,
      scoreLabel: t.score_label || '总评成绩',
      contentLabel: t.content_label || '授课内容',
      contentLimit: (t.content_limit && t.content_limit.max_length) || 0
    })
  },

  // ---- 基础信息 ----
  onTemplateChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    this.setData({ templateIndex: idx })
    this.applyTemplateView(this._templates[idx - 1], {})
  },

  onOfferingChange(e: WechatMiniprogram.PickerChange) {
    this.setData({ offeringIndex: Number(e.detail.value) })
  },

  async onDateChange(e: WechatMiniprogram.PickerChange) {
    const d = String(e.detail.value)
    this.setData({ observeDate: d })
    this.loadPeriods(d)
  },

  async loadPeriods(date: string) {
    try {
      const regime = await request<{ periods: any[] }>({ path: '/api/schedule/active', params: { date } })
      const periods = ((regime && regime.periods) || []).slice().sort((a, b) => a.periodIndex - b.periodIndex)
      this._periods = periods
      this.setData({
        periodLabels: periods.map((p) => `第 ${p.periodIndex} 节（${p.startTime}-${p.endTime}）`),
        periodValues: periods.map((p) => p.periodIndex)
      })
    } catch {
      this._periods = []
      this.setData({ periodLabels: [], periodValues: [] })
    }
  },

  onToggleSection(e: WechatMiniprogram.TouchEvent) {
    const idx = Number(e.currentTarget.dataset.idx)
    const val = this.data.periodValues[idx]
    let sections = this.data.sections.slice()
    if (sections.includes(val)) sections = sections.filter((s) => s !== val)
    else sections = sections.concat(val).sort((a, b) => a - b)
    this.setData({ sections })
  },

  onAnonymousChange(e: WechatMiniprogram.SwitchChange) {
    this.setData({ isAnonymous: e.detail.value })
  },

  // ---- 评分 / 评语 / extras ----
  onRateTap(e: WechatMiniprogram.TouchEvent) {
    const { idx, value } = e.currentTarget.dataset
    this.setData({ [`indicatorGroups[${idx}].value`]: value })
  },

  onHeaderExtraTap(e: WechatMiniprogram.TouchEvent) {
    const { idx, value } = e.currentTarget.dataset
    this.setData({ [`headerExtras[${idx}].value`]: value })
  },

  onExtraTap(e: WechatMiniprogram.TouchEvent) {
    const { idx, value } = e.currentTarget.dataset
    this.setData({ [`extras[${idx}].value`]: value })
  },

  onExtraDetailInput(e: WechatMiniprogram.Input) {
    const idx = Number(e.currentTarget.dataset.idx)
    this.setData({ [`extras[${idx}].detailValue`]: e.detail.value })
  },

  onCommentInput(e: WechatMiniprogram.Input) {
    const idx = Number(e.currentTarget.dataset.idx)
    this.setData({ [`commentFields[${idx}].value`]: e.detail.value })
  },

  onPostCommentInput(e: WechatMiniprogram.Input) {
    const idx = Number(e.currentTarget.dataset.idx)
    this.setData({ [`postCommentFields[${idx}].value`]: e.detail.value })
  },

  onStudentAnswer(e: WechatMiniprogram.PickerChange) {
    const { qidx, cidx } = e.currentTarget.dataset
    const val = Number(e.detail.value)
    this.setData({ [`studentQuestions[${qidx}].answers[${cidx}]`]: val })
  },

  onTotalScoreInput(e: WechatMiniprogram.Input) {
    this.setData({ totalScore: e.detail.value })
  },

  onContentInput(e: WechatMiniprogram.Input) {
    this.setData({ contentOutline: e.detail.value })
  },

  // ---- 保存 ----
  async onSave() {
    const d = this.data
    if (!d.templateValues[d.templateIndex]) return this.toast('请选择模板类型')
    if (!d.offeringValues[d.offeringIndex]) return this.toast('请选择课程')
    if (!d.observeDate) return this.toast('请选择听课日期')

    const indicatorScores: Record<string, string> = {}
    d.indicatorGroups.forEach((g) => { if (g.value) indicatorScores[g.key] = g.value })
    const comments: Record<string, string> = {}
    d.commentFields.forEach((cf) => { if (cf.value) comments[cf.key] = cf.value })
    d.postCommentFields.forEach((cf) => { if (cf.value) comments[cf.key] = cf.value })
    const extraValues: Record<string, string> = {}
    const extraDetails: Record<string, string> = {}
    d.headerExtras.forEach((ex) => { if (ex.value) extraValues[ex.key] = ex.value })
    d.extras.forEach((ex) => {
      if (ex.value) extraValues[ex.key] = ex.value
      if (ex.detailKey && ex.detailValue) extraDetails[ex.detailKey] = ex.detailValue
    })
    const studentFeedback: Record<string, Record<string, string>> = {}
    d.studentQuestions.forEach((q) => {
      const answers: Record<string, string> = {}
      d.studentColumns.forEach((col, ci) => {
        const vi = q.answers[ci]
        if (vi > 0 && q.optionValues[vi]) answers[col] = q.optionValues[vi]
      })
      studentFeedback[q.key] = answers
    })

    const payload = {
      templateType: d.templateValues[d.templateIndex],
      courseId: Number(d.offeringValues[d.offeringIndex]),
      observeDate: d.observeDate,
      sections: d.sections.slice().sort((a, b) => a - b),
      isAnonymous: d.isAnonymous,
      formData: {
        indicatorScores,
        totalScore: d.totalScore === '' ? null : Number(d.totalScore),
        contentOutline: d.contentOutline,
        comments,
        extraValues,
        extraDetails,
        studentFeedback
      }
    }

    this.setData({ saving: true, error: '' })
    try {
      if (d.mode === 'edit') {
        await request({ path: `/api/observations/${d.id}`, method: 'PUT', data: payload })
      } else {
        await request({ path: '/api/observations', method: 'POST', data: payload })
      }
      wx.showToast({ title: '已保存', icon: 'success' })
      setTimeout(() => wx.navigateBack(), 600)
    } catch (err: any) {
      this.setData({ error: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ saving: false })
    }
  },

  toast(title: string) {
    wx.showToast({ title, icon: 'none' })
  }
})
