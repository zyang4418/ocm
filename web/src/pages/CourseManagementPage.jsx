import { useCallback, useEffect, useState } from 'react'
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
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'

// ---- Offerings (课程/开课) ----
const offeringHeaders = [
  { key: 'id', header: 'ID' },
  { key: 'catalogName', header: '课程' },
  { key: 'catalogCode', header: '课程代码' },
  { key: 'teachingClassName', header: '教学班' },
  { key: 'classNames', header: '行政班' },
  { key: 'teacher', header: '教师' },
  { key: 'semester', header: '学期' },
]

// ---- Catalog (课程库) ----
const catalogHeaders = [
  { key: 'id', header: 'ID' },
  { key: 'name', header: '课程名称' },
  { key: 'code', header: '课程代码' },
  { key: 'description', header: '描述' },
]

const emptyOffering = { catalogId: '', teachingClassId: '', teacher: '', semester: '', note: '' }
const emptyCatalog = { name: '', code: '', description: '' }

export default function CourseManagementPage() {
  const { token, user: currentUser } = useAuth()
  const navigate = useNavigate()
  const canManage = currentUser?.role === 'admin'

  const [offerings, setOfferings] = useState([])
  const [catalog, setCatalog] = useState([])
  const [teachingClasses, setTeachingClasses] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

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

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true)
      setError('')
      const [offs, cats, tcs] = await Promise.all([
        apiFetch('/api/offerings', { token }),
        apiFetch('/api/courses', { token }),
        apiFetch('/api/teaching-classes', { token }),
      ])
      setOfferings(Array.isArray(offs) ? offs : [])
      setCatalog(Array.isArray(cats) ? cats : [])
      setTeachingClasses(Array.isArray(tcs) ? tcs : [])
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  // ---- offering handlers ----
  const submitOffering = async () => {
    if (!offForm.catalogId || !offForm.teachingClassId || !offForm.teacher.trim() || !offForm.semester.trim()) {
      setOffError('课程、教学班、教师、学期为必填项')
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
      await fetchAll()
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
      setCatError('课程名称为必填项')
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
      await fetchAll()
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
      await fetchAll()
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
            iconDescription="编辑"
            onClick={() => (kind === 'offering' ? openEditOffering(row) : openEditCatalog(row))}
          />
          <Button
            kind="ghost"
            size="sm"
            hasIconOnly
            renderIcon={TrashCan}
            iconDescription="删除"
            onClick={() => setDelTarget({ kind, row })}
          />
        </div>
      </TableCell>
    )

  const renderTable = (rows, headers, kind) => (
    <DataTable rows={rows} headers={headers}>
      {({ rows, headers: th, getTableProps, getHeaderProps, getRowProps, getToolbarProps, onInputChange }) => (
        <TableContainer title={kind === 'offering' ? '课程列表' : '课程库'} description={`共 ${rows.length} 项`}>
          <TableToolbar {...getToolbarProps()}>
            <TableToolbarContent>
              <TableToolbarSearch onChange={onInputChange} placeholder="搜索" />
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
                  {kind === 'offering' ? '添加课程' : '添加课程库'}
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
                {canManage && <TableHeader>操作</TableHeader>}
              </TableRow>
            </TableHead>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>加载中…</TableCell>
                </TableRow>
              ) : rows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={headers.length + (canManage ? 1 : 0)}>暂无数据</TableCell>
                </TableRow>
              ) : (
                rows.map((row) => {
                  const src = kind === 'offering' ? offerings : catalog
                  const item = src.find((x) => String(x.id) === String(row.id))
                  return (
                    <TableRow key={row.id} {...getRowProps({ row })}>
                      {row.cells.map((cell) => {
                        if (cell.info.header === 'classNames') {
                          return (
                            <TableCell key={cell.id}>
                              {Array.isArray(cell.value) && cell.value.length ? cell.value.join('、') : '-'}
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
          <BreadcrumbItem isCurrentPage>课程管理</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="courses-page__heading">课程管理</h1>
        <p className="courses-page__subtitle">维护课程库与各班级开课信息，为排课与课表提供数据基础。</p>
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
        <Tabs>
          <TabList aria-label="课程管理">
            <Tab>课程列表</Tab>
            <Tab>课程库</Tab>
          </TabList>
          <TabPanels>
            <TabPanel>{renderTable(offerings, offeringHeaders, 'offering')}</TabPanel>
            <TabPanel>{renderTable(catalog, catalogHeaders, 'catalog')}</TabPanel>
          </TabPanels>
        </Tabs>
      </Column>

      {/* Offering create/edit modal */}
      <Modal
        open={offCreateOpen}
        modalHeading={offEditTarget ? `编辑课程：${offEditTarget.catalogName}` : '添加课程'}
        primaryButtonText="保存"
        secondaryButtonText="取消"
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
            labelText="课程（课程库）"
            value={offForm.catalogId}
            onChange={(e) => setOffForm({ ...offForm, catalogId: e.target.value })}
          >
            <SelectItem value="" text="请选择课程" />
            {catalog.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={`${c.name}${c.code ? `（${c.code}）` : ''}`} />
            ))}
          </Select>
          <Select
            id="off-teaching-class"
            labelText="教学班"
            value={offForm.teachingClassId}
            onChange={(e) => setOffForm({ ...offForm, teachingClassId: e.target.value })}
          >
            <SelectItem value="" text="请选择教学班" />
            {teachingClasses.map((t) => (
              <SelectItem key={t.id} value={String(t.id)} text={t.name} />
            ))}
          </Select>
          <TextInput
            id="off-teacher"
            labelText="教师"
            value={offForm.teacher}
            onChange={(e) => setOffForm({ ...offForm, teacher: e.target.value })}
          />
          <TextInput
            id="off-semester"
            labelText="学期"
            placeholder="如 2026秋"
            value={offForm.semester}
            onChange={(e) => setOffForm({ ...offForm, semester: e.target.value })}
          />
          <TextInput
            id="off-note"
            labelText="备注"
            value={offForm.note}
            onChange={(e) => setOffForm({ ...offForm, note: e.target.value })}
          />
          {offError && (
            <InlineNotification kind="error" title="保存失败" subtitle={offError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Catalog create/edit modal */}
      <Modal
        open={catCreateOpen}
        modalHeading={catEditTarget ? `编辑课程：${catEditTarget.name}` : '添加课程库'}
        primaryButtonText="保存"
        secondaryButtonText="取消"
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
            labelText="课程名称"
            value={catForm.name}
            onChange={(e) => setCatForm({ ...catForm, name: e.target.value })}
          />
          <TextInput
            id="cat-code"
            labelText="课程代码"
            placeholder="如 MATH101"
            value={catForm.code}
            onChange={(e) => setCatForm({ ...catForm, code: e.target.value })}
          />
          <TextInput
            id="cat-desc"
            labelText="描述"
            value={catForm.description}
            onChange={(e) => setCatForm({ ...catForm, description: e.target.value })}
          />
          {catError && (
            <InlineNotification kind="error" title="保存失败" subtitle={catError} lowContrast hideCloseButton />
          )}
        </div>
      </Modal>

      {/* Delete modal */}
      <Modal
        danger
        open={Boolean(delTarget)}
        modalHeading="删除"
        primaryButtonText="删除"
        secondaryButtonText="取消"
        onRequestClose={() => setDelTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="courses-page__confirm-text">
          确定要删除{delTarget?.kind === 'offering' ? '课程' : '课程库'}「
          {delTarget?.kind === 'offering' ? delTarget?.row?.catalogName : delTarget?.row?.name}」吗？此操作不可撤销。
        </p>
        {delError && (
          <InlineNotification kind="error" title="删除失败" subtitle={delError} lowContrast hideCloseButton />
        )}
      </Modal>
    </Grid>
  )
}
