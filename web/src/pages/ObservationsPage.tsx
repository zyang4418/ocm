import { useEffect, useMemo, useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Checkbox,
  Column,
  DataTable,
  Grid,
  InlineNotification,
  Modal,
  NumberInput,
  RadioButton,
  RadioButtonGroup,
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
  TextArea,
  TextInput,
  type DataTableHeader,
  type TagProps,
} from '@carbon/react'
import { Add, Download, Edit, TrashCan, CheckmarkOutline } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch, apiDownload } from '../auth/api'
import ListPagination from '../components/ListPagination'
import usePagedList from '../hooks/usePagedList'
import type { Classroom, ObservationInput, ObservationView, OfferingView, Paged, Period, Regime } from '../types/api'

const STATUS_KIND: Record<string, TagProps<'div'>['type']> = { draft: 'gray', submitted: 'green' }

// ---------------------------------------------------------------------------
// Local types for the Python-derived template schema (schema.json) served by
// GET /api/observations/templates. The docx renderer is not wired yet, so Go
// is not the source of truth — these stay hand-mirrored here. NOTE: backend
// schema uses snake_case keys (score_groups / line_indexes), NOT camelCase.
// ---------------------------------------------------------------------------

interface RatingOption {
  value: string
  label: string
}

interface ScoreGroup {
  key: string
  line_indexes?: number[]
}

interface Indicator {
  key: string
  title: string
  lines?: string[]
  score_groups?: ScoreGroup[]
}

interface HeaderExtra {
  key: string
  label: string
  options?: string[]
}

interface CommentField {
  key: string
  label: string
  placeholder?: string
  max_length?: number
}

interface ExtraField {
  key: string
  label: string
  options?: string[]
  detail_key?: string
  detail_placeholder?: string
  detail_limit?: { max_length?: number }
  detail_required_when?: string
}

interface StudentFeedbackSchema {
  columns?: string[]
  questions?: Array<{ key: string; title: string; options?: RatingOption[] }>
}

interface TemplateEntry {
  value: string
  label: string
  indicators?: Indicator[]
  header_extras?: HeaderExtra[]
  header_extra_section_title?: string
  score_label?: string
  content_label?: string
  content_limit?: { max_length?: number }
  comment_fields?: CommentField[]
  extras?: ExtraField[]
  extra_section_title?: string
  post_content_comment_fields?: CommentField[]
  post_content_comment_title?: string
  student_feedback?: StudentFeedbackSchema
}

interface TemplateSchema {
  templates?: TemplateEntry[]
  rating_options?: RatingOption[]
}

// indicatorScoreGroups mirrors the backend: an explicit score_groups list wins,
// otherwise a single group keyed by the indicator key.
function indicatorScoreGroups(ind: Indicator): Array<{ key: string; lines: string[] }> {
  if (ind.score_groups && ind.score_groups.length) {
    return ind.score_groups.map((g) => ({
      key: g.key,
      lines: (g.line_indexes || []).map((i) => ind.lines?.[i]).filter((l): l is string => Boolean(l)),
    }))
  }
  return [{ key: ind.key, lines: ind.lines || [] }]
}

function fmtScore(v: number | null | undefined): string {
  if (v === null || v === undefined || (v as unknown) === '') return '—'
  return String(v)
}

// Form meta: ids/dates stay strings while editing (converted on submit).
interface MetaState {
  templateType: string
  courseId: string
  classroomId: string
  observeDate: string
  sections: number[]
  isAnonymous: boolean
}

const emptyMeta: MetaState = { templateType: '', courseId: '', classroomId: '', observeDate: '', sections: [], isAnonymous: false }

// Dynamic form data written into the observation's opaque formData JSON. The
// shape is driven by the selected template's schema (see TemplateEntry).
interface FormDataState {
  indicatorScores: Record<string, string>
  totalScore: string
  contentOutline: string
  comments: Record<string, string>
  extraValues: Record<string, string>
  extraDetails: Record<string, string>
  studentFeedback: Record<string, Record<string, string>>
}

const emptyFormData: FormDataState = {
  indicatorScores: {},
  totalScore: '',
  contentOutline: '',
  comments: {},
  extraValues: {},
  extraDetails: {},
  studentFeedback: {},
}

// The wire payload: the generated ObservationInput keeps formData opaque
// (Record<string, never>), so refine it with the page's dynamic shape.
type ObservationPayload = Omit<ObservationInput, 'formData'> & {
  formData: {
    indicatorScores: Record<string, string>
    totalScore: number | null
    contentOutline: string
    comments: Record<string, string>
    extraValues: Record<string, string>
    extraDetails: Record<string, string>
    studentFeedback: Record<string, Record<string, string>>
  }
}

export default function ObservationsPage() {
  const { t, i18n } = useTranslation('observations')
  const { token, user: currentUser, can } = useAuth()
  const navigate = useNavigate()
  const canWrite = can('observation:write')
  const canManage = can('observation:manage')

  const headers: DataTableHeader[] = [
    { key: 'courseName', header: t('field.courseName') },
    { key: 'teacher', header: t('field.teacher') },
    { key: 'teachingClassName', header: t('field.teachingClassName') },
    { key: 'observeDate', header: t('field.observeDate') },
    { key: 'sections', header: t('field.sections') },
    { key: 'templateType', header: t('field.templateType') },
    { key: 'totalScore', header: t('field.totalScore') },
    { key: 'status', header: t('field.status') },
  ]

  // Static reference data: form schema, course offerings, classrooms, periods.
  const [schema, setSchema] = useState<TemplateSchema | null>(null)
  const [offerings, setOfferings] = useState<OfferingView[]>([])
  const [classrooms, setClassrooms] = useState<Classroom[]>([])
  const [periods, setPeriods] = useState<Period[]>([])
  const [schemaError, setSchemaError] = useState('')

  const [filterStatus, setFilterStatus] = useState('')
  const [filterTemplate, setFilterTemplate] = useState('')

  const list = usePagedList<ObservationView>({
    path: '/api/observations',
    token,
    extraParams: { status: filterStatus, template_type: filterTemplate },
  })
  const { loading } = list
  const [actionError, setActionError] = useState('')
  const error = list.error || actionError || schemaError

  // Form modal state.
  const [formOpen, setFormOpen] = useState(false)
  const [editId, setEditId] = useState<number | null>(null)
  const [meta, setMeta] = useState<MetaState>(emptyMeta)
  const [formData, setFormData] = useState<FormDataState>(emptyFormData)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [actingId, setActingId] = useState<number | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ObservationView | null>(null)

  useEffect(() => {
    Promise.all([
      apiFetch<TemplateSchema>('/api/observations/templates', { token }),
      apiFetch<Paged<OfferingView>>('/api/offerings?page_size=500', { token }),
      apiFetch<Paged<Classroom>>('/api/classrooms?page_size=500', { token }),
    ])
      .then(([sch, off, clr]) => {
        setSchema(sch)
        setOfferings(Array.isArray(off?.items) ? off.items : [])
        setClassrooms(Array.isArray(clr?.items) ? clr.items : [])
      })
      .catch((err: Error) => setSchemaError(err.message))
  }, [token])

  useEffect(() => {
    if (!meta.observeDate) {
      setPeriods([])
      return
    }
    let cancelled = false
    apiFetch<Regime>(`/api/schedule/active?date=${meta.observeDate}`, { token })
      .then((regime) => {
        if (cancelled) return
        setPeriods((regime?.periods || []).slice().sort((a, b) => a.periodIndex - b.periodIndex))
      })
      .catch(() => {
        if (!cancelled) setPeriods([])
      })
    return () => {
      cancelled = true
    }
  }, [meta.observeDate, token])

  const selectedTemplate = useMemo(() => {
    if (!schema || !meta.templateType) return null
    return schema.templates?.find((tpl) => tpl.value === meta.templateType) || null
  }, [schema, meta.templateType])

  const templateLabel = (value: string) => t('templateType.' + value, { defaultValue: value || '' })

  const sectionsLabel = (sections: number[]) => {
    if (!sections || !sections.length) return '—'
    const sorted = sections.slice().sort((a, b) => a - b)
    const listFmt = new Intl.ListFormat(i18n.language || 'zh-CN', { style: 'narrow' })
    return t('sectionsLabel', { sections: listFmt.format(sorted.map(String)) })
  }

  const openCreate = () => {
    setEditId(null)
    setMeta(emptyMeta)
    setFormData(emptyFormData)
    setFormError('')
    setFormOpen(true)
  }

  const openEdit = async (row: ObservationView) => {
    try {
      setFormError('')
      setActionError('')
      const v = await apiFetch<ObservationView>(`/api/observations/${row.id}`, { token })
      const fd = (v.formData ?? {}) as Partial<FormDataState>
      setEditId(row.id)
      setMeta({
        templateType: v.templateType || '',
        courseId: v.courseId ? String(v.courseId) : '',
        classroomId: v.classroomId ? String(v.classroomId) : '',
        observeDate: v.observeDate || '',
        sections: Array.isArray(v.sections) ? v.sections : [],
        isAnonymous: Boolean(v.isAnonymous),
      })
      setFormData({
        indicatorScores: fd.indicatorScores || {},
        totalScore: v.totalScore != null ? String(v.totalScore) : '',
        contentOutline: fd.contentOutline || v.content || '',
        comments: fd.comments || {},
        extraValues: fd.extraValues || {},
        extraDetails: fd.extraDetails || {},
        studentFeedback: fd.studentFeedback || {},
      })
      setFormOpen(true)
    } catch (err) {
      setActionError((err as Error).message)
    }
  }

  const buildPayload = (): ObservationPayload => ({
    templateType: meta.templateType,
    courseId: Number(meta.courseId),
    // Undefined drops the key on JSON.stringify — the Go side decodes the
    // missing/null key as a nil pointer either way.
    classroomId: meta.classroomId ? Number(meta.classroomId) : undefined,
    observeDate: meta.observeDate,
    sections: meta.sections.map(Number).sort((a, b) => a - b),
    isAnonymous: meta.isAnonymous,
    formData: {
      indicatorScores: formData.indicatorScores,
      totalScore: formData.totalScore === '' || formData.totalScore === null ? null : Number(formData.totalScore),
      contentOutline: formData.contentOutline,
      comments: formData.comments,
      extraValues: formData.extraValues,
      extraDetails: formData.extraDetails,
      studentFeedback: formData.studentFeedback,
    },
  })

  const handleSave = async () => {
    if (!meta.templateType) return setFormError(t('validation.templateRequired'))
    if (!meta.courseId) return setFormError(t('validation.courseRequired'))
    if (!meta.classroomId) return setFormError(t('validation.classroomRequired'))
    if (!meta.observeDate) return setFormError(t('validation.dateRequired'))
    try {
      setSaving(true)
      setFormError('')
      const payload = buildPayload()
      if (editId) {
        await apiFetch(`/api/observations/${editId}`, { method: 'PUT', token, body: payload })
      } else {
        await apiFetch('/api/observations', { method: 'POST', token, body: payload })
      }
      setFormOpen(false)
      list.reload()
    } catch (err) {
      setFormError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const submitObservation = async (row: ObservationView) => {
    try {
      setActingId(row.id)
      setActionError('')
      await apiFetch(`/api/observations/${row.id}/submit`, { method: 'POST', token })
      list.reload()
    } catch (err) {
      setActionError((err as Error).message)
    } finally {
      setActingId(null)
    }
  }

  const exportObservation = async (row: ObservationView) => {
    try {
      setActingId(row.id)
      setActionError('')
      await apiDownload(`/api/observations/${row.id}/export`, {
        token,
        method: 'POST',
        fallbackName: `observation-${row.id}.docx`,
      })
    } catch (err) {
      setActionError((err as Error).message)
    } finally {
      setActingId(null)
    }
  }

  const deleteObservation = async () => {
    const id = deleteTarget?.id
    if (!id) return
    try {
      setActingId(id)
      setActionError('')
      await apiFetch(`/api/observations/${id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      list.reload()
    } catch (err) {
      setActionError((err as Error).message)
    } finally {
      setActingId(null)
    }
  }

  const setIndicatorScore = (key: string, val: string) =>
    setFormData((f) => ({ ...f, indicatorScores: { ...f.indicatorScores, [key]: val } }))
  const setComment = (key: string, val: string) =>
    setFormData((f) => ({ ...f, comments: { ...f.comments, [key]: val } }))
  const setExtraValue = (key: string, val: string) =>
    setFormData((f) => ({ ...f, extraValues: { ...f.extraValues, [key]: val } }))
  const setExtraDetail = (key: string, val: string) =>
    setFormData((f) => ({ ...f, extraDetails: { ...f.extraDetails, [key]: val } }))
  const setStudentAnswer = (qKey: string, student: string, val: string) =>
    setFormData((f) => ({
      ...f,
      studentFeedback: { ...f.studentFeedback, [qKey]: { ...(f.studentFeedback[qKey] || {}), [student]: val } },
    }))

  const toggleSection = (idx: number, checked: boolean) =>
    setMeta((m) => ({
      ...m,
      sections: checked ? [...m.sections, idx].sort((a, b) => a - b) : m.sections.filter((s) => s !== idx),
    }))

  const ratingOptions = schema?.rating_options || []

  const rows = list.items.map((o) => ({
    id: String(o.id),
    courseName: o.courseName,
    teacher: o.teacher,
    teachingClassName: o.teachingClassName,
    observeDate: o.observeDate,
    sections: sectionsLabel(o.sections),
    templateType: templateLabel(o.templateType),
    totalScore: fmtScore(o.totalScore),
    status: o.status,
  }))

  const colSpan = headers.length + 1

  return (
    <Grid fullWidth className="courses-page observations-page">
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
      </Column>

      <Column sm={4} md={8} lg={16}>
        {error && (
          <InlineNotification
            kind="error"
            title={t('error.action')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}

        <div className="bookings-page__filters">
          <Select
            id="f-status"
            labelText={t('filter.status')}
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text={t('filter.allStatuses')} />
            <SelectItem value="draft" text={t('status.draft', { ns: 'common' })} />
            <SelectItem value="submitted" text={t('status.submitted', { ns: 'common' })} />
          </Select>
          <Select
            id="f-template"
            labelText={t('filter.template')}
            value={filterTemplate}
            onChange={(e) => setFilterTemplate(e.target.value)}
            className="bookings-page__filter"
          >
            <SelectItem value="" text={t('filter.allTemplates')} />
            {(schema?.templates || []).map((tpl) => (
              <SelectItem key={tpl.value} value={tpl.value} text={tpl.label} />
            ))}
          </Select>
        </div>
      </Column>

      <Column sm={4} md={8} lg={16}>
        <DataTable rows={rows} headers={headers}>
          {({ rows: tableRows, headers: renderedHeaders, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('table.searchPlaceholder')} />
                  {canWrite && (
                    <Button renderIcon={Add} size="sm" onClick={openCreate}>
                      {t('table.addButton')}
                    </Button>
                  )}
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
                      const o = list.items.find((x) => String(x.id) === String(row.id))
                      const isDraft = o?.status === 'draft'
                      const own = Boolean(o) && Number(currentUser?.id) === o!.observerId
                      const canEdit = isDraft && (canManage || own)
                      const canSubmit = isDraft && (canManage || own)
                      const canDelete = isDraft && (canManage || own)
                      const canExport = o?.status === 'submitted' && (canManage || own)
                      return (
                        <TableRow {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'status') {
                              const value = cell.value as string
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={STATUS_KIND[value] ?? 'gray'} size="sm">
                                    {t('status.' + value, { ns: 'common', defaultValue: value })}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            return <TableCell key={cell.id}>{cell.value as string}</TableCell>
                          })}
                          <TableCell>
                            <div className="courses-page__actions">
                              {o && canEdit && (
                                <Button kind="ghost" size="sm" renderIcon={Edit} onClick={() => openEdit(o)} disabled={actingId === o.id}>
                                  {t('action.edit', { ns: 'common' })}
                                </Button>
                              )}
                              {o && canSubmit && (
                                <Button kind="ghost" size="sm" renderIcon={CheckmarkOutline} onClick={() => submitObservation(o)} disabled={actingId === o.id}>
                                  {t('action.submit', { ns: 'common' })}
                                </Button>
                              )}
                              {o && canExport && (
                                <Button kind="ghost" size="sm" renderIcon={Download} onClick={() => exportObservation(o)} disabled={actingId === o.id}>
                                  {t('action.export', { ns: 'common' })}
                                </Button>
                              )}
                              {o && canDelete && (
                                <Button kind="ghost" size="sm" renderIcon={TrashCan} onClick={() => setDeleteTarget(o)} disabled={actingId === o.id}>
                                  {t('action.delete', { ns: 'common' })}
                                </Button>
                              )}
                              {!canEdit && !canSubmit && !canExport && !canDelete && (
                                <span style={{ color: 'var(--cds-text-secondary)' }}>—</span>
                              )}
                            </div>
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

      {/* Create / edit form */}
      <Modal
        open={formOpen}
        size="lg"
        modalHeading={editId ? t('modal.editHeading') : t('modal.createHeading')}
        primaryButtonText={t('action.save', { ns: 'common' })}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setFormOpen(false)}
        onRequestSubmit={handleSave}
        primaryButtonDisabled={saving}
        className="observations-page__form-modal"
      >
        <div className="courses-page__form observations-page__form">
          {/* 基础信息 */}
          <div className="observations-page__section">
            <h3 className="observations-page__section-title">{t('section.basic')}</h3>
            <Select
              id="o-template"
              labelText={t('form.templateType')}
              value={meta.templateType}
              onChange={(e) => {
                setMeta({ ...meta, templateType: e.target.value })
                setFormData(emptyFormData)
              }}
            >
              <SelectItem value="" text={t('form.templatePlaceholder')} />
              {(schema?.templates || []).map((tpl) => (
                <SelectItem key={tpl.value} value={tpl.value} text={tpl.label} />
              ))}
            </Select>
            <Select
              id="o-course"
              labelText={t('form.course')}
              value={meta.courseId}
              onChange={(e) => setMeta({ ...meta, courseId: e.target.value })}
            >
              <SelectItem value="" text={t('form.coursePlaceholder')} />
              {offerings.map((o) => (
                <SelectItem
                  key={o.id}
                  value={String(o.id)}
                  text={t('courseOption', { catalogName: o.catalogName, teachingClass: o.teachingClassName, teacher: o.teacher })}
                />
              ))}
            </Select>
            <Select
              id="o-classroom"
              labelText={t('form.classroom')}
              value={meta.classroomId}
              onChange={(e) => setMeta({ ...meta, classroomId: e.target.value })}
            >
              <SelectItem value="" text={t('form.classroomPlaceholder')} />
              {classrooms.map((c) => (
                <SelectItem
                  key={c.id}
                  value={String(c.id)}
                  text={c.building ? `${c.building} ${c.name}` : c.name}
                />
              ))}
            </Select>
            <TextInput
              id="o-date"
              type="date"
              labelText={t('form.observeDate')}
              value={meta.observeDate}
              onChange={(e) => setMeta({ ...meta, observeDate: e.target.value })}
            />
            {periods.length > 0 && (
              <fieldset className="observations-page__sections">
                <legend className="observations-page__legend">{t('form.sectionsLegend')}</legend>
                {periods.map((p) => (
                  <Checkbox
                    key={p.periodIndex}
                    id={`o-sec-${p.periodIndex}`}
                    labelText={t('periodOption', { index: p.periodIndex, start: p.startTime, end: p.endTime })}
                    checked={meta.sections.includes(p.periodIndex)}
                    onChange={(_, { checked }) => toggleSection(p.periodIndex, checked)}
                  />
                ))}
              </fieldset>
            )}
            <Checkbox
              id="o-anon"
              labelText={t('form.anonymous')}
              checked={meta.isAnonymous}
              onChange={(_, { checked }) => setMeta({ ...meta, isAnonymous: checked })}
            />
          </div>

          {selectedTemplate && (
            <>
              {/* 评分指标 */}
              {selectedTemplate.indicators && selectedTemplate.indicators.length > 0 && (
                <div className="observations-page__section">
                  <h3 className="observations-page__section-title">{t('section.indicators')}</h3>
                  {selectedTemplate.indicators.map((ind) =>
                    indicatorScoreGroups(ind).map((g) => (
                      <div key={g.key} className="observations-page__indicator">
                        <div className="observations-page__indicator-head">
                          <span className="observations-page__indicator-title">{ind.title}</span>
                        </div>
                        {g.lines.map((line, i) => (
                          <p key={i} className="observations-page__indicator-line">
                            {line}
                          </p>
                        ))}
                        <RadioButtonGroup
                          name={`ind-${g.key}`}
                          legendText={t('form.ratingLegend')}
                          orientation="horizontal"
                          valueSelected={formData.indicatorScores[g.key] || ''}
                          onChange={(v) => setIndicatorScore(g.key, String(v))}
                        >
                          {ratingOptions.map((r) => (
                            <RadioButton key={r.value} labelText={r.label} value={r.value} />
                          ))}
                        </RadioButtonGroup>
                      </div>
                    )),
                  )}
                </div>
              )}

              {/* header extras（radio，如年龄层次/班级规模） */}
              {selectedTemplate.header_extras && selectedTemplate.header_extras.length > 0 && (
                <div className="observations-page__section">
                  <h3 className="observations-page__section-title">{selectedTemplate.header_extra_section_title || t('sectionFallback.headerExtra')}</h3>
                  {selectedTemplate.header_extras.map((ex) => (
                    <RadioButtonGroup
                      key={ex.key}
                      name={`he-${ex.key}`}
                      legendText={ex.label}
                      orientation="horizontal"
                      valueSelected={formData.extraValues[ex.key] || ''}
                      onChange={(v) => setExtraValue(ex.key, String(v))}
                    >
                      {(ex.options || []).map((opt) => (
                        <RadioButton key={opt} labelText={opt} value={opt} />
                      ))}
                    </RadioButtonGroup>
                  ))}
                </div>
              )}

              {/* 总分 + 授课内容 */}
              <div className="observations-page__section">
                <h3 className="observations-page__section-title">{t('section.conclusion')}</h3>
                <NumberInput
                  id="o-score"
                  label={selectedTemplate.score_label || t('form.scoreLabelFallback')}
                  min={0}
                  max={100}
                  value={formData.totalScore}
                  onChange={(e, { value }) => setFormData((f) => ({ ...f, totalScore: value == null ? '' : String(value) }))}
                  allowEmpty
                />
                <TextArea
                  id="o-content"
                  labelText={selectedTemplate.content_label || t('form.contentLabelFallback')}
                  placeholder={t('form.contentPlaceholder', { label: selectedTemplate.content_label || t('form.contentLabelFallback'), max: selectedTemplate.content_limit?.max_length || '' })}
                  value={formData.contentOutline}
                  onChange={(e) => setFormData((f) => ({ ...f, contentOutline: e.target.value }))}
                  maxLength={selectedTemplate.content_limit?.max_length || undefined}
                  rows={4}
                />
              </div>

              {/* 评语 */}
              {selectedTemplate.comment_fields && selectedTemplate.comment_fields.length > 0 && (
                <div className="observations-page__section">
                  <h3 className="observations-page__section-title">{t('section.comments')}</h3>
                  {selectedTemplate.comment_fields.map((cf) => (
                    <TextArea
                      key={cf.key}
                      id={`c-${cf.key}`}
                      labelText={cf.label}
                      placeholder={cf.placeholder}
                      value={formData.comments[cf.key] || ''}
                      onChange={(e) => setComment(cf.key, e.target.value)}
                      maxLength={cf.max_length || undefined}
                      rows={3}
                    />
                  ))}
                </div>
              )}

              {/* extras（radio_with_detail） */}
              {selectedTemplate.extras && selectedTemplate.extras.length > 0 && (
                <div className="observations-page__section">
                  <h3 className="observations-page__section-title">{selectedTemplate.extra_section_title || t('sectionFallback.extra')}</h3>
                  {selectedTemplate.extras.map((ex) => {
                    const detailVisible =
                      ex.detail_required_when === undefined ||
                      formData.extraValues[ex.key] === ex.detail_required_when ||
                      formData.extraValues[ex.key] === ''
                    return (
                      <div key={ex.key} className="observations-page__extra">
                        <RadioButtonGroup
                          name={`ex-${ex.key}`}
                          legendText={ex.label}
                          orientation="horizontal"
                          valueSelected={formData.extraValues[ex.key] || ''}
                          onChange={(v) => setExtraValue(ex.key, String(v))}
                        >
                          {(ex.options || []).map((opt) => (
                            <RadioButton key={opt} labelText={opt} value={opt} />
                          ))}
                        </RadioButtonGroup>
                        {ex.detail_key && detailVisible && (
                          <TextArea
                            id={`exd-${ex.detail_key}`}
                            labelText={t('form.extraDetailLabel', { label: ex.label })}
                            placeholder={ex.detail_placeholder}
                            value={formData.extraDetails[ex.detail_key] || ''}
                            onChange={(e) => setExtraDetail(ex.detail_key ?? '', e.target.value)}
                            maxLength={ex.detail_limit?.max_length || undefined}
                            rows={3}
                          />
                        )}
                      </div>
                    )
                  })}
                </div>
              )}

              {/* post_content_comment_fields（教室条件/听课笔记） */}
              {selectedTemplate.post_content_comment_fields && selectedTemplate.post_content_comment_fields.length > 0 && (
                <div className="observations-page__section">
                  <h3 className="observations-page__section-title">
                    {selectedTemplate.post_content_comment_title || t('sectionFallback.postContent')}
                  </h3>
                  {selectedTemplate.post_content_comment_fields.map((cf) => (
                    <TextArea
                      key={cf.key}
                      id={`pc-${cf.key}`}
                      labelText={cf.label}
                      placeholder={cf.placeholder}
                      value={formData.comments[cf.key] || ''}
                      onChange={(e) => setComment(cf.key, e.target.value)}
                      maxLength={cf.max_length || undefined}
                      rows={3}
                    />
                  ))}
                </div>
              )}

              {/* 学生反馈矩阵 */}
              {selectedTemplate.student_feedback && (
                <div className="observations-page__section">
                  <h3 className="observations-page__section-title">{t('section.studentFeedback')}</h3>
                  <div className="observations-page__matrix">
                    <table className="observations-page__matrix-table">
                      <thead>
                        <tr>
                          <th>{t('form.studentFeedbackHeader')}</th>
                          {(selectedTemplate.student_feedback.columns || []).map((col) => (
                            <th key={col}>{col}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {(selectedTemplate.student_feedback.questions || []).map((q) => (
                          <tr key={q.key}>
                            <td className="observations-page__matrix-question">{q.title}</td>
                            {(selectedTemplate.student_feedback?.columns || []).map((col) => (
                              <td key={col}>
                                <Select
                                  id={`sf-${q.key}-${col}`}
                                  labelText=""
                                  hideLabel
                                  value={formData.studentFeedback[q.key]?.[col] || ''}
                                  onChange={(e) => setStudentAnswer(q.key, col, e.target.value)}
                                >
                                  <SelectItem value="" text={t('form.dash')} />
                                  {(q.options || []).map((opt) => (
                                    <SelectItem key={opt.value} value={opt.value} text={opt.label} />
                                  ))}
                                </Select>
                              </td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </>
          )}

          {formError && (
            <InlineNotification kind="error" title={t('error.save')} subtitle={formError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Delete confirm */}
      <Modal
        danger
        open={Boolean(deleteTarget)}
        modalHeading={t('modal.deleteHeading')}
        primaryButtonText={t('modal.deleteSubmit')}
        secondaryButtonText={t('action.back', { ns: 'common' })}
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={deleteObservation}
        primaryButtonDisabled={actingId === deleteTarget?.id}
      >
        <p className="courses-page__confirm-text">
          {t('modal.deleteConfirm', { course: deleteTarget?.courseName, date: deleteTarget?.observeDate })}
        </p>
      </Modal>
    </Grid>
  )
}
