import { useEffect, useState } from 'react'
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
  Tabs,
  TabList,
  Tab,
  TabPanels,
  TabPanel,
  TextInput,
} from '@carbon/react'
import { Add, Edit, TrashCan } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'
import ExportButton from '../components/ExportButton.jsx'
import ListPagination from '../components/ListPagination.jsx'
import usePagedList from '../hooks/usePagedList.js'

const emptyOffering = { catalogId: '', teachingClassId: '', teacher: '', semester: '', note: '' }
const emptyCatalog = { name: '', code: '', description: '' }

export default function CourseManagementPage() {
  const { t, i18n } = useTranslation('courses')
  const { token, can } = useAuth()
  const navigate = useNavigate()
  const canManage = can('course:manage')

  const offeringHeaders = [
    { key: 'id', header: t('offeringField.id') },
    { key: 'catalogName', header: t('offeringField.catalogName') },
    { key: 'catalogCode', header: t('offeringField.catalogCode') },
    { key: 'teachingClassName', header: t('offeringField.teachingClassName') },
    { key: 'classNames', header: t('offeringField.classNames') },
    { key: 'teacher', header: t('offeringField.teacher') },
    { key: 'semester', header: t('offeringField.semester') },
  ]
  const catalogHeaders = [
    { key: 'id', header: t('catalogField.id') },
    { key: 'name', header: t('catalogField.name') },
    { key: 'code', header: t('catalogField.code') },
    { key: 'description', header: t('catalogField.description') },
  ]

  const offerings = usePagedList({ path: '/api/offerings', token })
  const catalogList = usePagedList({ path: '/api/courses', token })
  // Dropdown options for the offering modal need (near-)full lists; pull the
  // maximum page. optionsKey re-triggers the fetch after catalog mutations.
  const [catalogOptions, setCatalogOptions] = useState([])
  const [teachingClasses, setTeachingClasses] = useState([])
  const [optionsKey, setOptionsKey] = useState(0)
  // Export errors are separate from the list fetches (the hooks own theirs).
  const [exportError, setExportError] = useState('')
  const error = offerings.error || catalogList.error || exportError

  // offering modals
  const [offCreateOpen, setOffCreateOpen] = useState(false)
  const [offForm, setOffForm] = useState(emptyOffering)
  const [offError, setOffError] = useState('')
  const [offSaving, setOffSaving] = useState(false)
  const [offEditTarget, setOffEditTarget] = useState(null)

  // catalog modals
  const [catCreateOpen, setCatCreateOpen] = useState(false)
  const [catForm, setCatForm] = useState(emptyCatalog)
  const [catError, setCatError] = useState('')
  const [catSaving, setCatSaving] = useState(false)
  const [catEditTarget, setCatEditTarget] = useState(null)

  // delete (shared)
  const [delTarget, setDelTarget] = useState(null) // {kind:'offering'|'catalog', row}
  const [delError, setDelError] = useState('')
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      apiFetch('/api/courses?page_size=500', { token }),
      apiFetch('/api/teaching-classes?page_size=500', { token }),
    ])
      .then(([cats, tcs]) => {
        if (cancelled) return
        setCatalogOptions(Array.isArray(cats?.items) ? cats.items : [])
        setTeachingClasses(Array.isArray(tcs?.items) ? tcs.items : [])
      })
      .catch(() => {
        if (!cancelled) {
          setCatalogOptions([])
          setTeachingClasses([])
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
    const body = {
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
      setOffError(err.message)
    } finally {
      setOffSaving(false)
    }
  }

  const openEditOffering = (o) => {
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
    const body = {
      name: catForm.name.trim(),
      code: catForm.code.trim(),
      description: catForm.description.trim(),
    }
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
      setCatError(err.message)
    } finally {
      setCatSaving(false)
    }
  }

  const openEditCatalog = (c) => {
    setCatEditTarget(c)
    setCatForm({ name: c.name, code: c.code, description: c.description })
    setCatError('')
    setCatCreateOpen(true)
  }

  // ---- delete ----
  const handleDelete = async () => {
    const { kind, row } = delTarget
    const url = kind === 'offering' ? `/api/offerings/${row.id}` : `/api/courses/${row.id}`
    try {
      setDeleting(true)
      setDelError('')
      await apiFetch(url, { method: 'DELETE', token })
      setDelTarget(null)
      if (kind === 'offering') {
        offerings.reload()
      } else {
        catalogList.reload()
        setOptionsKey((k) => k + 1)
      }
    } catch (err) {
      setDelError(err.message)
    } finally {
      setDeleting(false)
    }
  }

  const renderActions = (kind, row) =>
    canManage && (
      <TableCell>
        <div className="courses-page__actions">
          <Button
            kind="ghost"
            size="sm"
            hasIconOnly
            renderIcon={Edit}
            iconDescription={t('action.edit', { ns: 'common' })}
            onClick={() => (kind === 'offering' ? openEditOffering(row) : openEditCatalog(row))}
          />
          <Button
            kind="ghost"
            size="sm"
            hasIconOnly
            renderIcon={TrashCan}
            iconDescription={t('action.delete', { ns: 'common' })}
            onClick={() => setDelTarget({ kind, row })}
          />
        </div>
      </TableCell>
    )

  // classNames column joins admin-class names with a locale-appropriate
  // separator (zh: "、", en: ", ") via Intl.ListFormat narrow style.
  const listFmt = new Intl.ListFormat(i18n.language, { style: 'narrow' })

  const renderTable = (list, headers, kind) => (
    <DataTable rows={list.items} headers={headers}>
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
                {th.map((h) => (
                  <TableHeader key={h.key} {...getHeaderProps({ header: h })}>
                    {h.header}
                  </TableHeader>
                ))}
                {canManage && <TableHeader>{t('field.actions')}</TableHeader>}
              </TableRow>
            </TableHead>
            <TableBody>
              {list.loading ? (
                <TableRow>
                  <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>{t('empty.loading')}</TableCell>
                </TableRow>
              ) : rows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>
                    {list.q ? t('empty.noResults', { ns: 'common' }) : t('empty.noData', { ns: 'common' })}
                  </TableCell>
                </TableRow>
              ) : (
                rows.map((row) => {
                  const item = list.items.find((x) => String(x.id) === String(row.id))
                  return (
                    <TableRow key={row.id} {...getRowProps({ row })}>
                      {row.cells.map((cell) => {
                        if (cell.info.header === 'classNames') {
                          return (
                            <TableCell key={cell.id}>
                              {Array.isArray(cell.value) && cell.value.length
                                ? listFmt.format(cell.value)
                                : '-'}
                            </TableCell>
                          )
                        }
                        return <TableCell key={cell.id}>{cell.value || '-'}</TableCell>
                      })}
                      {renderActions(kind, item)}
                    </TableRow>
                  )
                })
              )}
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
        <Tabs>
          <TabList aria-label={t('tabs.ariaLabel')}>
            <Tab>{t('tabs.offerings')}</Tab>
            <Tab>{t('tabs.catalog')}</Tab>
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
            name: delTarget?.kind === 'offering' ? delTarget?.row?.catalogName : delTarget?.row?.name,
          })}
        </p>
        {delError && (
          <InlineNotification kind="error" title={t('error.delete')} subtitle={delError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
