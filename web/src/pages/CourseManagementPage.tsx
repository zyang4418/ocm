import { Fragment, useCallback, useEffect, useState } from 'react'
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
  TableExpandHeader,
  TableExpandedRow,
  TableExpandRow,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Tabs,
  TabList,
  Tab,
  TabPanels,
  TabPanel,
  TextInput,
  type DataTableHeader,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import ExportButton from '../components/ExportButton'
import ListPagination from '../components/ListPagination'
import SessionsMiniTable, { type SessionsCacheEntry } from '../components/SessionsMiniTable'
import SessionsPanel from '../components/SessionsPanel'
import usePagedList, { type PagedList } from '../hooks/usePagedList'
import type {
  CatalogCourse,
  CatalogInput,
  Classroom,
  OfferingInput,
  OfferingView,
  Paged,
  SessionView,
  TeachingClassView,
} from '../types/api'

// Form state keeps ids/numbers as strings while editing; converted on submit.
interface OfferingForm {
  catalogId: string
  teachingClassId: string
  teacher: string
  semester: string
  note: string
}

const emptyOffering: OfferingForm = { catalogId: '', teachingClassId: '', teacher: '', semester: '', note: '' }
const emptyCatalog = { name: '', code: '', description: '' }

// Delete target: which tab's row (discriminated for URL + confirm text).
type DeleteTarget =
  | { kind: 'offering'; row: OfferingView }
  | { kind: 'catalog'; row: CatalogCourse }

export default function CourseManagementPage() {
  const { t, i18n } = useTranslation('courses')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('course:manage')

  const offeringHeaders: DataTableHeader[] = [
    { key: 'id', header: t('offeringField.id') },
    { key: 'catalogName', header: t('offeringField.catalogName') },
    { key: 'catalogCode', header: t('offeringField.catalogCode') },
    { key: 'teachingClassName', header: t('offeringField.teachingClassName') },
    { key: 'classNames', header: t('offeringField.classNames') },
    { key: 'teacher', header: t('offeringField.teacher') },
    { key: 'semester', header: t('offeringField.semester') },
  ]
  const catalogHeaders: DataTableHeader[] = [
    { key: 'id', header: t('catalogField.id') },
    { key: 'name', header: t('catalogField.name') },
    { key: 'code', header: t('catalogField.code') },
    { key: 'description', header: t('catalogField.description') },
  ]

  const offerings = usePagedList<OfferingView>({ path: '/api/offerings', token })
  const catalogList = usePagedList<CatalogCourse>({ path: '/api/courses', token })
  // Dropdown options for the offering modal need (near-)full lists; pull the
  // maximum page. optionsKey re-triggers the fetch after catalog mutations.
  const [catalogOptions, setCatalogOptions] = useState<CatalogCourse[]>([])
  const [teachingClasses, setTeachingClasses] = useState<TeachingClassView[]>([])
  // Dropdown options for the session modal and the sessions tab (near-full lists).
  const [offeringOptions, setOfferingOptions] = useState<OfferingView[]>([])
  const [classroomOptions, setClassroomOptions] = useState<Classroom[]>([])
  const [optionsKey, setOptionsKey] = useState(0)
  // L3 sessions: per-offering cache backing the offerings table's expanded rows.
  const [sessionsCache, setSessionsCache] = useState<Record<string, SessionsCacheEntry>>({})
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())
  // Controlled tabs so an expanded row's "view all" link can jump to the
  // sessions tab with the offering filter pre-applied.
  const [tabIndex, setTabIndex] = useState(0)
  const [pendingSessionsFilter, setPendingSessionsFilter] = useState<string | null>(null)
  // Export errors are separate from the list fetches (the hooks own theirs).
  const [exportError, setExportError] = useState('')
  const error = offerings.error || catalogList.error || exportError

  // offering modals
  const [offCreateOpen, setOffCreateOpen] = useState(false)
  const [offForm, setOffForm] = useState<OfferingForm>(emptyOffering)
  const [offError, setOffError] = useState('')
  const [offSaving, setOffSaving] = useState(false)
  const [offEditTarget, setOffEditTarget] = useState<OfferingView | null>(null)

  // catalog modals
  const [catCreateOpen, setCatCreateOpen] = useState(false)
  const [catForm, setCatForm] = useState(emptyCatalog)
  const [catError, setCatError] = useState('')
  const [catSaving, setCatSaving] = useState(false)
  const [catEditTarget, setCatEditTarget] = useState<CatalogCourse | null>(null)

  // delete (shared)
  const [delTarget, setDelTarget] = useState<DeleteTarget | null>(null)
  const [delError, setDelError] = useState('')
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      apiFetch<Paged<CatalogCourse>>('/api/courses?page_size=500', { token }),
      apiFetch<Paged<TeachingClassView>>('/api/teaching-classes?page_size=500', { token }),
      apiFetch<Paged<OfferingView>>('/api/offerings?page_size=500', { token }),
      apiFetch<Paged<Classroom>>('/api/classrooms?page_size=500', { token }),
    ])
      .then(([cats, tcs, offs, cls]) => {
        if (cancelled) return
        setCatalogOptions(Array.isArray(cats?.items) ? cats.items : [])
        setTeachingClasses(Array.isArray(tcs?.items) ? tcs.items : [])
        setOfferingOptions(Array.isArray(offs?.items) ? offs.items : [])
        setClassroomOptions(Array.isArray(cls?.items) ? cls.items : [])
      })
      .catch(() => {
        if (!cancelled) {
          setCatalogOptions([])
          setTeachingClasses([])
          setOfferingOptions([])
          setClassroomOptions([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [token, optionsKey])

  // ---- offering handlers ----
  const submitOffering = async () => {
    if (!offForm.catalogId || !offForm.teachingClassId || !offForm.teacher.trim() || !offForm.semester.trim()) {
      setOffError(t('validation.offeringRequired'))
      return
    }
    // OfferingInput is a whole-object replace: keep the untouched fields of
    // the edit target (or zeros on create) so they are not reset.
    const base: OfferingInput = offEditTarget
      ? {
          catalogId: offEditTarget.catalogId,
          college: offEditTarget.college,
          courseSeq: offEditTarget.courseSeq,
          maxStudents: offEditTarget.maxStudents,
          note: offEditTarget.note,
          requirement: offEditTarget.requirement,
          semester: offEditTarget.semester,
          teacher: offEditTarget.teacher,
          teacherId: offEditTarget.teacherId,
          teacherTitle: offEditTarget.teacherTitle,
          teachingClassId: offEditTarget.teachingClassId,
          weeklyHours: offEditTarget.weeklyHours,
        }
      : {
          catalogId: 0,
          college: '',
          courseSeq: '',
          maxStudents: 0,
          note: '',
          requirement: '',
          semester: '',
          teacher: '',
          teacherId: '',
          teacherTitle: '',
          teachingClassId: 0,
          weeklyHours: 0,
        }
    const body: OfferingInput = {
      ...base,
      catalogId: Number(offForm.catalogId),
      teachingClassId: Number(offForm.teachingClassId),
      teacher: offForm.teacher.trim(),
      semester: offForm.semester.trim(),
      note: offForm.note.trim(),
    }
    try {
      setOffSaving(true)
      setOffError('')
      if (offEditTarget) {
        await apiFetch(`/api/offerings/${offEditTarget.id}`, { method: 'PUT', token, body })
      } else {
        await apiFetch('/api/offerings', { method: 'POST', token, body })
      }
      setOffCreateOpen(false)
      setOffEditTarget(null)
      offerings.reload()
    } catch (err) {
      setOffError((err as Error).message)
    } finally {
      setOffSaving(false)
    }
  }

  const openEditOffering = (o: OfferingView) => {
    setOffEditTarget(o)
    setOffForm({
      catalogId: String(o.catalogId),
      teachingClassId: String(o.teachingClassId),
      teacher: o.teacher,
      semester: o.semester,
      note: o.note,
    })
    setOffError('')
    setOffCreateOpen(true)
  }

  // ---- catalog handlers ----
  const submitCatalog = async () => {
    if (!catForm.name.trim()) {
      setCatError(t('validation.catalogNameRequired'))
      return
    }
    // CatalogInput is a whole-object replace: keep the edit target's other
    // fields (category/credits/examType/totalHours) intact.
    const body: CatalogInput = catEditTarget
      ? {
          category: catEditTarget.category,
          code: catEditTarget.code,
          credits: catEditTarget.credits,
          description: catEditTarget.description,
          examType: catEditTarget.examType,
          name: catEditTarget.name,
          totalHours: catEditTarget.totalHours,
        }
      : { category: '', code: '', credits: 0, description: '', examType: '', name: '', totalHours: 0 }
    body.name = catForm.name.trim()
    body.code = catForm.code.trim()
    body.description = catForm.description.trim()
    try {
      setCatSaving(true)
      setCatError('')
      if (catEditTarget) {
        await apiFetch(`/api/courses/${catEditTarget.id}`, { method: 'PUT', token, body })
      } else {
        await apiFetch('/api/courses', { method: 'POST', token, body })
      }
      setCatCreateOpen(false)
      setCatEditTarget(null)
      catalogList.reload()
      setOptionsKey((k) => k + 1) // keep the modal's course dropdown in sync
    } catch (err) {
      setCatError((err as Error).message)
    } finally {
      setCatSaving(false)
    }
  }

  const openEditCatalog = (c: CatalogCourse) => {
    setCatEditTarget(c)
    setCatForm({ name: c.name, code: c.code, description: c.description })
    setCatError('')
    setCatCreateOpen(true)
  }

  // ---- delete ----
  const handleDelete = async () => {
    if (!delTarget) return
    const url = delTarget.kind === 'offering' ? `/api/offerings/${delTarget.row.id}` : `/api/courses/${delTarget.row.id}`
    try {
      setDeleting(true)
      setDelError('')
      await apiFetch(url, { method: 'DELETE', token })
      const kind = delTarget.kind
      setDelTarget(null)
      if (kind === 'offering') {
        offerings.reload()
      } else {
        catalogList.reload()
        setOptionsKey((k) => k + 1)
      }
    } catch (err) {
      setDelError((err as Error).message)
    } finally {
      setDeleting(false)
    }
  }

  // ---- L3 sessions: expanded rows + cache ----

  // Fetch one offering's sessions into the cache (page_size=500 covers a
  // semester's ~60-90 sessions). Existing data stays visible while reloading.
  const loadSessions = useCallback(
    async (offeringId: string) => {
      setSessionsCache((prev) => ({
        ...prev,
        [offeringId]: { data: prev[offeringId]?.data ?? [], loading: true, error: '' },
      }))
      try {
        const data = await apiFetch<Paged<SessionView>>(`/api/sessions?offering_id=${offeringId}&page_size=500`, { token })
        setSessionsCache((prev) => ({
          ...prev,
          [offeringId]: { data: Array.isArray(data?.items) ? data.items : [], loading: false, error: '' },
        }))
      } catch (err) {
        setSessionsCache((prev) => ({
          ...prev,
          [offeringId]: { data: [], loading: false, error: (err as Error).message },
        }))
      }
    },
    [token],
  )

  // Toggling an un-cached row kicks off its lazy fetch; cached rows re-expand
  // without another request. Multiple rows may be expanded at once.
  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
    if (!sessionsCache[id]) void loadSessions(id)
  }

  // Session mutations (from the expanded rows or the sessions tab) refresh the
  // cache of every affected offering, keeping the two views in sync.
  const refreshSessionsCache = (offeringId?: number, prevOfferingId?: number) => {
    for (const id of [offeringId, prevOfferingId]) {
      if (id && sessionsCache[String(id)]) void loadSessions(String(id))
    }
  }

  // A new page/search/page-size means new rows: drop expansions but keep the
  // cache so paging back re-expands without another request.
  useEffect(() => {
    setExpandedIds(new Set())
  }, [offerings.page, offerings.q, offerings.pageSize])

  // "View all" from an expanded row: switch to the sessions tab pre-filtered.
  const viewAllSessions = (offeringId: number) => {
    setPendingSessionsFilter(String(offeringId))
    setTabIndex(2)
  }

  const renderActions = (kind: DeleteTarget['kind'], row: OfferingView | CatalogCourse) =>
    canManage && (
      <TableCell>
        <div className="courses-page__actions">
          <Button
            kind="ghost"
            size="sm"
            hasIconOnly
            renderIcon={Edit}
            iconDescription={t('action.edit', { ns: 'common' })}
            onClick={() => (kind === 'offering' ? openEditOffering(row as OfferingView) : openEditCatalog(row as CatalogCourse))}
          />
          <Button
            kind="ghost"
            size="sm"
            hasIconOnly
            renderIcon={TrashCan}
            iconDescription={t('action.delete', { ns: 'common' })}
            onClick={() => setDelTarget(kind === 'offering' ? { kind, row: row as OfferingView } : { kind, row: row as CatalogCourse })}
          />
        </div>
      </TableCell>
    )

  // classNames column joins admin-class names with a locale-appropriate
  // separator (zh: "、", en: ", ") via Intl.ListFormat narrow style.
  const listFmt = new Intl.ListFormat(i18n.language, { style: 'narrow' })

  const renderTable = (list: PagedList<OfferingView> | PagedList<CatalogCourse>, headers: DataTableHeader[], kind: 'offering' | 'catalog') => (
    <DataTable rows={list.items.map((item) => ({ ...item, id: String(item.id) }))} headers={headers}>
      {({ rows, headers: th, getTableProps, getHeaderProps, getRowProps, getToolbarProps }) => (
        <TableContainer
          title={kind === 'offering' ? t('table.offering.title') : t('table.catalog.title')}
          description={t(`table.${kind}.description`, { count: list.total })}
        >
          <TableToolbar {...getToolbarProps()}>
            <TableToolbarContent>
              <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('searchPlaceholder')} />
              <ExportButton
                path={kind === 'offering' ? '/api/offerings/export' : '/api/courses/export'}
                fallbackName={kind === 'offering' ? 'offerings.xlsx' : 'catalog.xlsx'}
                onError={setExportError}
              />
              {canManage && (
                <Button
                  renderIcon={Add}
                  size="sm"
                  onClick={() => {
                    if (kind === 'offering') {
                      setOffForm(emptyOffering)
                      setOffEditTarget(null)
                      setOffError('')
                      setOffCreateOpen(true)
                    } else {
                      setCatForm(emptyCatalog)
                      setCatEditTarget(null)
                      setCatError('')
                      setCatCreateOpen(true)
                    }
                  }}
                >
                  {kind === 'offering' ? t('addButton.offering') : t('addButton.catalog')}
                </Button>
              )}
            </TableToolbarContent>
          </TableToolbar>
          <Table {...getTableProps()}>
            <TableHead>
              <TableRow>
                {kind === 'offering' && <TableExpandHeader />}
                {th.map((h) => (
                  <TableHeader {...getHeaderProps({ header: h })}>
                    {h.header}
                  </TableHeader>
                ))}
                {canManage && <TableHeader>{t('field.actions')}</TableHeader>}
              </TableRow>
            </TableHead>
            <TableBody>
              {(() => {
                // Expand column (offerings) + data columns + optional actions.
                const colSpan = headers.length + (kind === 'offering' ? 1 : 0) + (canManage ? 1 : 0)
                if (list.loading) {
                  return (
                    <TableRow>
                      <TableCell colSpan={colSpan}>{t('empty.loading')}</TableCell>
                    </TableRow>
                  )
                }
                if (rows.length === 0) {
                  return (
                    <TableRow>
                      <TableCell colSpan={colSpan}>
                        {list.q ? t('empty.noResults', { ns: 'common' }) : t('empty.noData', { ns: 'common' })}
                      </TableCell>
                    </TableRow>
                  )
                }
                return rows.map((row) => {
                  const item = list.items.find((x) => String(x.id) === String(row.id))
                  if (!item) return null
                  if (kind !== 'offering') {
                    return (
                      <TableRow {...getRowProps({ row })}>
                        {row.cells.map((cell) => {
                          if (cell.info.header === 'classNames') {
                            const value = cell.value as string[]
                            return (
                              <TableCell key={cell.id}>
                                {Array.isArray(value) && value.length ? listFmt.format(value) : '-'}
                              </TableCell>
                            )
                          }
                          return <TableCell key={cell.id}>{(cell.value as string) || '-'}</TableCell>
                        })}
                        {renderActions(kind, item)}
                      </TableRow>
                    )
                  }
                  // Offering rows expand into their session list (controlled:
                  // our expandedIds set drives TableExpandRow/TableExpandedRow).
                  const offering = item as OfferingView
                  const idStr = String(offering.id)
                  const expanded = expandedIds.has(idStr)
                  return (
                    <Fragment key={row.id}>
                      <TableExpandRow
                        aria-label={t('sessionsExpanded.aria', { name: offering.catalogName })}
                        isExpanded={expanded}
                        onExpand={() => toggleExpand(idStr)}
                      >
                        {row.cells.map((cell) => {
                          if (cell.info.header === 'classNames') {
                            const value = cell.value as string[]
                            return (
                              <TableCell key={cell.id}>
                                {Array.isArray(value) && value.length ? listFmt.format(value) : '-'}
                              </TableCell>
                            )
                          }
                          return <TableCell key={cell.id}>{(cell.value as string) || '-'}</TableCell>
                        })}
                        {renderActions('offering', offering)}
                      </TableExpandRow>
                      {expanded && (
                        <TableExpandedRow colSpan={colSpan}>
                          <SessionsMiniTable
                            offering={offering}
                            entry={sessionsCache[idStr] ?? { data: [], loading: false, error: '' }}
                            canManage={canManage}
                            offerings={offeringOptions}
                            classrooms={classroomOptions}
                            onMutated={refreshSessionsCache}
                            onReload={(oid) => void loadSessions(String(oid))}
                            onViewAll={viewAllSessions}
                          />
                        </TableExpandedRow>
                      )}
                    </Fragment>
                  )
                })
              })()}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </DataTable>
  )

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
        {error && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={error}
            lowContrast
            hideCloseButton
            className="courses-page__notice"
          />
        )}
      </Column>

      <Column sm={4} md={8} lg={16}>
        <Tabs selectedIndex={tabIndex} onChange={({ selectedIndex }) => setTabIndex(selectedIndex)}>
          <TabList aria-label={t('tabs.ariaLabel')}>
            <Tab>{t('tabs.offerings')}</Tab>
            <Tab>{t('tabs.catalog')}</Tab>
            <Tab>{t('tabs.sessions')}</Tab>
          </TabList>
          <TabPanels>
            <TabPanel>
              {renderTable(offerings, offeringHeaders, 'offering')}
              <ListPagination
                page={offerings.page}
                pageSize={offerings.pageSize}
                totalItems={offerings.total}
                onPageChange={offerings.setPage}
                onPageSizeChange={offerings.setPageSize}
              />
            </TabPanel>
            <TabPanel>
              {renderTable(catalogList, catalogHeaders, 'catalog')}
              <ListPagination
                page={catalogList.page}
                pageSize={catalogList.pageSize}
                totalItems={catalogList.total}
                onPageChange={catalogList.setPage}
                onPageSizeChange={catalogList.setPageSize}
              />
            </TabPanel>
            <TabPanel>
              <SessionsPanel
                active={tabIndex === 2}
                filterOfferingId={pendingSessionsFilter}
                onFilterConsumed={() => setPendingSessionsFilter(null)}
                onMutated={refreshSessionsCache}
                offerings={offeringOptions}
                classrooms={classroomOptions}
              />
            </TabPanel>
          </TabPanels>
        </Tabs>
      </Column>

      {/* Offering create/edit modal */}
      <Modal
        open={offCreateOpen}
        modalHeading={offEditTarget ? t('modal.edit', { name: offEditTarget.catalogName }) : t('modal.offeringCreate')}
        primaryButtonText={t('modal.editSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => {
          setOffCreateOpen(false)
          setOffEditTarget(null)
        }}
        onRequestSubmit={submitOffering}
        primaryButtonDisabled={offSaving}
      >
        <div className="courses-page__form">
          <Select
            id="off-catalog"
            labelText={t('offeringForm.catalog')}
            value={offForm.catalogId}
            onChange={(e) => setOffForm({ ...offForm, catalogId: e.target.value })}
          >
            <SelectItem value="" text={t('offeringForm.selectCatalog')} />
            {catalogOptions.map((c) => (
              <SelectItem
                key={c.id}
                value={String(c.id)}
                text={c.code ? t('offeringForm.courseOption', { name: c.name, code: c.code }) : c.name}
              />
            ))}
          </Select>
          <Select
            id="off-teaching-class"
            labelText={t('offeringForm.teachingClass')}
            value={offForm.teachingClassId}
            onChange={(e) => setOffForm({ ...offForm, teachingClassId: e.target.value })}
          >
            <SelectItem value="" text={t('offeringForm.selectTeachingClass')} />
            {teachingClasses.map((tc) => (
              <SelectItem key={tc.id} value={String(tc.id)} text={tc.name} />
            ))}
          </Select>
          <TextInput
            id="off-teacher"
            labelText={t('offeringForm.teacher')}
            value={offForm.teacher}
            onChange={(e) => setOffForm({ ...offForm, teacher: e.target.value })}
          />
          <TextInput
            id="off-semester"
            labelText={t('offeringForm.semester')}
            placeholder={t('offeringForm.semesterPlaceholder')}
            value={offForm.semester}
            onChange={(e) => setOffForm({ ...offForm, semester: e.target.value })}
          />
          <TextInput
            id="off-note"
            labelText={t('offeringForm.note')}
            value={offForm.note}
            onChange={(e) => setOffForm({ ...offForm, note: e.target.value })}
          />
          {offError && (
            <InlineNotification kind="error" title={t('error.save')} subtitle={offError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Catalog create/edit modal */}
      <Modal
        open={catCreateOpen}
        modalHeading={catEditTarget ? t('modal.edit', { name: catEditTarget.name }) : t('modal.catalogCreate')}
        primaryButtonText={t('modal.editSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => {
          setCatCreateOpen(false)
          setCatEditTarget(null)
        }}
        onRequestSubmit={submitCatalog}
        primaryButtonDisabled={catSaving}
      >
        <div className="courses-page__form">
          <TextInput
            id="cat-name"
            labelText={t('catalogForm.name')}
            value={catForm.name}
            onChange={(e) => setCatForm({ ...catForm, name: e.target.value })}
          />
          <TextInput
            id="cat-code"
            labelText={t('catalogForm.code')}
            placeholder={t('catalogForm.codePlaceholder')}
            value={catForm.code}
            onChange={(e) => setCatForm({ ...catForm, code: e.target.value })}
          />
          <TextInput
            id="cat-desc"
            labelText={t('catalogForm.description')}
            value={catForm.description}
            onChange={(e) => setCatForm({ ...catForm, description: e.target.value })}
          />
          {catError && (
            <InlineNotification kind="error" title={t('error.save')} subtitle={catError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Delete modal */}
      <Modal
        danger
        open={Boolean(delTarget)}
        modalHeading={t('modal.delete')}
        primaryButtonText={t('modal.deleteSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setDelTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="courses-page__confirm-text">
          {t(`deleteConfirm.${delTarget?.kind ?? 'offering'}`, {
            name: delTarget?.kind === 'offering' ? delTarget.row.catalogName : delTarget?.kind === 'catalog' ? delTarget.row.name : '',
          })}
        </p>
        {delError && (
          <InlineNotification kind="error" title={t('error.delete')} subtitle={delError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
