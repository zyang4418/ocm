import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { formatDateTime } from '../../utils/format'

interface Catalog {
  id: number
  name: string
  code: string
  description: string
}

interface Offering {
  id: number
  catalogId: number
  catalogName: string
  catalogCode: string
  teachingClassId: number
  teachingClassName: string
  classNames: string[]
  teacher: string
  semester: string
  note: string
  createdAt: string
}

interface TeachingClass {
  id: number
  name: string
}

const emptyOffForm = { catalogId: '', teachingClassId: '', teacher: '', semester: '', note: '' }
const emptyCatForm = { name: '', code: '', description: '' }

Page({
  data: {
    tab: 'offerings',
    tabs: [
      { label: '开课列表', value: 'offerings' },
      { label: '课程库', value: 'catalog' }
    ],
    canManage: false,
    offList: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    catList: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    // 开课弹层
    offOpen: false,
    offLoading: false,
    offError: '',
    offTitle: '添加开课',
    offForm: { ...emptyOffForm },
    offCatalogOptions: [] as string[],
    offCatalogIndex: 0,
    offTcOptions: [] as string[],
    offTcIndex: 0,
    offEditId: 0,
    // 课程库弹层
    catOpen: false,
    catLoading: false,
    catError: '',
    catTitle: '添加课程',
    catForm: { ...emptyCatForm },
    catEditId: 0,
    actingId: 0
  },

  _offList: null as any,
  _catList: null as any,
  _catalogs: [] as Catalog[],
  _teachingClasses: [] as TeachingClass[],

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canManage: can('course:manage') })
    this._offList = createPagedList({
      path: '/api/offerings',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((o: Offering) => this.offeringView(o))
        this.setData({ offList: { ...p, items: view } })
      }
    })
    this._catList = createPagedList({
      path: '/api/courses',
      pageSize: 20,
      setData: (p) => {
        const view = (p.items || []).map((c: Catalog) => ({
          ...c,
          createdAtText: formatDateTime((c as any).createdAt)
        }))
        this.setData({ catList: { ...p, items: view } })
      }
    })
    this._offList.load()
    this._catList.load()
    this.loadOptions()
  },

  offeringView(o: Offering) {
    return {
      ...o,
      classNamesText: (o.classNames || []).join('、'),
      createdAtText: formatDateTime(o.createdAt)
    }
  },

  /** 开课弹层的课程/教学班下拉(近全量)。 */
  async loadOptions() {
    try {
      const [cats, tcs] = await Promise.all([
        request<{ items: Catalog[] }>({ path: '/api/courses', params: { page_size: 500 } }),
        request<{ items: TeachingClass[] }>({ path: '/api/teaching-classes', params: { page_size: 500 } })
      ])
      this._catalogs = (cats && cats.items) || []
      this._teachingClasses = (tcs && tcs.items) || []
      this.setData({
        offCatalogOptions: this._catalogs.map((c) => `${c.name}（${c.code}）`),
        offTcOptions: this._teachingClasses.map((t) => t.name)
      })
    } catch {
      this._catalogs = []
      this._teachingClasses = []
    }
  },

  onTabChange(e: WechatMiniprogram.CustomEvent) {
    this.setData({ tab: e.detail.value })
  },

  onOffSearchInput(e: WechatMiniprogram.Input) {
    this._offList.setQ(e.detail.value)
  },

  onCatSearchInput(e: WechatMiniprogram.Input) {
    this._catList.setQ(e.detail.value)
  },

  onReachBottom() {
    if (this.data.tab === 'offerings') this._offList.loadMore()
    else this._catList.loadMore()
  },

  // ---- 开课 ----

  openOffCreate() {
    this.setData({
      offOpen: true,
      offTitle: '添加开课',
      offForm: { ...emptyOffForm },
      offCatalogIndex: 0,
      offTcIndex: 0,
      offError: '',
      offEditId: 0
    })
  },

  openOffEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.offList.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      offOpen: true,
      offTitle: '编辑开课',
      offForm: {
        catalogId: String(row.catalogId),
        teachingClassId: String(row.teachingClassId),
        teacher: row.teacher,
        semester: row.semester,
        note: row.note || ''
      },
      offCatalogIndex: Math.max(0, this._catalogs.findIndex((c) => c.id === row.catalogId)),
      offTcIndex: Math.max(0, this._teachingClasses.findIndex((t) => t.id === row.teachingClassId)),
      offError: '',
      offEditId: row.id
    })
  },

  closeOff() {
    if (this.data.offLoading) return
    this.setData({ offOpen: false })
  },

  onOffCatalogChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    const c = this._catalogs[idx]
    this.setData({ offCatalogIndex: idx, 'offForm.catalogId': c ? String(c.id) : '' })
  },

  onOffTcChange(e: WechatMiniprogram.PickerChange) {
    const idx = Number(e.detail.value)
    const t = this._teachingClasses[idx]
    this.setData({ offTcIndex: idx, 'offForm.teachingClassId': t ? String(t.id) : '' })
  },

  onOffTeacherInput(e: WechatMiniprogram.Input) {
    this.setData({ 'offForm.teacher': e.detail.value })
  },

  onOffSemesterInput(e: WechatMiniprogram.Input) {
    this.setData({ 'offForm.semester': e.detail.value })
  },

  onOffNoteInput(e: WechatMiniprogram.Input) {
    this.setData({ 'offForm.note': e.detail.value })
  },

  async submitOffering() {
    const f = this.data.offForm
    if (!f.catalogId || !f.teachingClassId || !f.teacher.trim() || !f.semester.trim()) {
      this.setData({ offError: '课程、教学班、教师、学期为必填项' })
      return
    }
    this.setData({ offLoading: true, offError: '' })
    const body = {
      catalogId: Number(f.catalogId),
      teachingClassId: Number(f.teachingClassId),
      teacher: f.teacher.trim(),
      semester: f.semester.trim(),
      note: f.note.trim()
    }
    try {
      if (this.data.offEditId) {
        await request({ path: `/api/offerings/${this.data.offEditId}`, method: 'PUT', data: body })
      } else {
        await request({ path: '/api/offerings', method: 'POST', data: body })
      }
      this.setData({ offOpen: false })
      this._offList.reload()
    } catch (err: any) {
      this.setData({ offError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ offLoading: false })
    }
  },

  // ---- 课程库 ----

  openCatCreate() {
    this.setData({
      catOpen: true,
      catTitle: '添加课程',
      catForm: { ...emptyCatForm },
      catError: '',
      catEditId: 0
    })
  },

  openCatEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const row = (this.data.catList.items as any[]).find((x) => x.id === Number(id))
    if (!row) return
    this.setData({
      catOpen: true,
      catTitle: '编辑课程',
      catForm: { name: row.name, code: row.code || '', description: row.description || '' },
      catError: '',
      catEditId: row.id
    })
  },

  closeCat() {
    if (this.data.catLoading) return
    this.setData({ catOpen: false })
  },

  onCatNameInput(e: WechatMiniprogram.Input) {
    this.setData({ 'catForm.name': e.detail.value })
  },

  onCatCodeInput(e: WechatMiniprogram.Input) {
    this.setData({ 'catForm.code': e.detail.value })
  },

  onCatDescInput(e: WechatMiniprogram.Input) {
    this.setData({ 'catForm.description': e.detail.value })
  },

  async submitCatalog() {
    const f = this.data.catForm
    if (!f.name.trim()) {
      this.setData({ catError: '课程名称为必填项' })
      return
    }
    this.setData({ catLoading: true, catError: '' })
    const body = { name: f.name.trim(), code: f.code.trim(), description: f.description.trim() }
    try {
      if (this.data.catEditId) {
        await request({ path: `/api/courses/${this.data.catEditId}`, method: 'PUT', data: body })
      } else {
        await request({ path: '/api/courses', method: 'POST', data: body })
      }
      this.setData({ catOpen: false })
      this._catList.reload()
      // 课程库变更后刷新开课弹层的课程下拉
      this.loadOptions()
    } catch (err: any) {
      this.setData({ catError: (err && err.message) || '保存失败' })
    } finally {
      this.setData({ catLoading: false })
    }
  },

  // ---- 删除 ----

  onTapDeleteOffering(e: WechatMiniprogram.TouchEvent) {
    const { id, name } = e.currentTarget.dataset
    this.confirmDelete('offering', Number(id), name)
  },

  onTapDeleteCatalog(e: WechatMiniprogram.TouchEvent) {
    const { id, name } = e.currentTarget.dataset
    this.confirmDelete('catalog', Number(id), name)
  },

  confirmDelete(kind: 'offering' | 'catalog', id: number, name: string) {
    const label = kind === 'offering' ? '课程' : '课程库'
    wx.showModal({
      title: `删除${label}`,
      content: `确定要删除${label}「${name}」吗？此操作不可撤销。`,
      confirmColor: '#D54941',
      success: (res) => {
        if (res.confirm) this.handleDelete(kind, id)
      }
    })
  },

  async handleDelete(kind: 'offering' | 'catalog', id: number) {
    this.setData({ actingId: id })
    const url = kind === 'offering' ? `/api/offerings/${id}` : `/api/courses/${id}`
    try {
      await request({ path: url, method: 'DELETE' })
      if (kind === 'offering') this._offList.reload()
      else this._catList.reload()
    } catch (err: any) {
      wx.showToast({ title: (err && err.message) || '删除失败', icon: 'none' })
    } finally {
      this.setData({ actingId: 0 })
    }
  }
})
