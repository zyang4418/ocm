import { ensureAuth, getUser } from '../../utils/auth'
import { can } from '../../utils/perms'
import { createPagedList } from '../../utils/paged'
import { request } from '../../utils/request'
import { apiConfig } from '../../config/api'
import { getToken } from '../../utils/storage'
import { observationStatus, observationTemplateLabel, formatDateTime } from '../../utils/format'

interface Observation {
  id: number
  templateType: string
  observeDate: string
  sections: number[]
  status: string
  courseName: string
  teacher: string
  teachingClassName: string
  totalScore: number | null
  observerId: number
  createdAt: string
}

function sectionsText(sections: number[] | undefined): string {
  if (!sections || !sections.length) return '—'
  return `第 ${sections.slice().sort((a, b) => a - b).join('、')} 节`
}

Page({
  data: {
    list: { items: [] as any[], total: 0, page: 1, q: '', loading: true, error: '', hasMore: false },
    actionError: '',
    canWrite: false,
    canManage: false,
    meId: 0,
    // 状态筛选 chips
    statusChips: [
      { label: '全部', value: '' },
      { label: '草稿', value: 'draft' },
      { label: '已提交', value: 'submitted' }
    ],
    activeStatus: '',
    // 模板筛选
    templateOptions: [] as string[],
    templateValues: [] as string[],
    filterTemplateIndex: 0,
    // 行内操作防连点
    actingId: 0
  },

  _list: null as any,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    const me = getUser()
    this.setData({
      canWrite: can('observation:write'),
      canManage: can('observation:manage'),
      meId: me ? me.id : 0
    })
    this._list = createPagedList({
      path: '/api/observations',
      pageSize: 20,
      extraParams: () => {
        const { activeStatus } = this.data
        const tv = this.data.templateValues[this.data.filterTemplateIndex]
        return { status: activeStatus, template_type: tv || '' }
      },
      setData: (p) => {
        const view = (p.items || []).map((o: Observation) => this.rowView(o))
        this.setData({ list: { ...p, items: view } })
      }
    })
    this._list.load()
    this.loadTemplates()
  },

  onShow() {
    // 从表单页返回时刷新（新建/编辑后）
    if (this._list) this._list.reload()
  },

  rowView(o: Observation) {
    const st = observationStatus[o.status] || { text: o.status, theme: 'gray' }
    const isDraft = o.status === 'draft'
    const own = this.data.meId === o.observerId
    return {
      ...o,
      templateLabel: observationTemplateLabel[o.templateType] || o.templateType,
      sectionsText: sectionsText(o.sections),
      totalScoreText: o.totalScore != null ? String(o.totalScore) : '—',
      createdAtText: formatDateTime(o.createdAt),
      statusText: st.text,
      statusTheme: st.theme,
      canEdit: isDraft && (this.data.canManage || own),
      canSubmit: isDraft && (this.data.canManage || own),
      canExport: !isDraft && (this.data.canManage || own),
      canDelete: isDraft && (this.data.canManage || own)
    }
  },

  async loadTemplates() {
    try {
      const sch = await request<{ templates: { value: string; label: string }[] }>({ path: '/api/observations/templates' })
      const ts = (sch && sch.templates) || []
      this.setData({
        templateOptions: ['全部模板'].concat(ts.map((t) => t.label)),
        templateValues: [''].concat(ts.map((t) => t.value))
      })
    } catch (err: any) {
      this.setData({ actionError: (err && err.message) || '加载模板失败' })
    }
  },

  // ---- 列表筛选 ----
  onTapStatus(e: WechatMiniprogram.TouchEvent) {
    const { value } = e.currentTarget.dataset
    this.setData({ activeStatus: value })
    this._list.load()
  },

  onFilterTemplateChange(e: WechatMiniprogram.PickerChange) {
    this.setData({ filterTemplateIndex: Number(e.detail.value) })
    this._list.load()
  },

  onSearchInput(e: WechatMiniprogram.Input) {
    this._list.setQ(e.detail.value)
  },

  onReachBottom() {
    this._list.loadMore()
  },

  // ---- 新建 / 编辑 ----
  onCreate() {
    wx.navigateTo({ url: '/pages/observations-form/observations-form' })
  },

  onEdit(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    wx.navigateTo({ url: `/pages/observations-form/observations-form?id=${id}` })
  },

  // ---- 行内操作 ----
  async onSubmit(e: WechatMiniprogram.TouchEvent) {
    const id = Number(e.currentTarget.dataset.id)
    this.setData({ actingId: id, actionError: '' })
    try {
      await request({ path: `/api/observations/${id}/submit`, method: 'POST' })
      wx.showToast({ title: '已提交', icon: 'success' })
      this._list.reload()
    } catch (err: any) {
      this.setData({ actionError: (err && err.message) || '提交失败' })
    } finally {
      this.setData({ actingId: 0 })
    }
  },

  async onDelete(e: WechatMiniprogram.TouchEvent) {
    const id = Number(e.currentTarget.dataset.id)
    const row = (this.data.list.items as any[]).find((x) => x.id === id)
    const name = row ? `${row.courseName} · ${row.observeDate}` : '该记录'
    const res = await new Promise<boolean>((resolve) => {
      wx.showModal({
        title: '删除评课',
        content: `确定要删除「${name}」的评课记录吗？此操作不可恢复。`,
        confirmColor: '#D54941',
        success: (r) => resolve(r.confirm)
      })
    })
    if (!res) return
    this.setData({ actingId: id, actionError: '' })
    try {
      await request({ path: `/api/observations/${id}`, method: 'DELETE' })
      wx.showToast({ title: '已删除', icon: 'success' })
      this._list.reload()
    } catch (err: any) {
      this.setData({ actionError: (err && err.message) || '删除失败' })
    } finally {
      this.setData({ actingId: 0 })
    }
  },

  // ---- 导出 docx ----
  onExport(e: WechatMiniprogram.TouchEvent) {
    const id = Number(e.currentTarget.dataset.id)
    this.exportDocx(id)
  },

  exportDocx(id: number) {
    if (apiConfig.transport === 'callContainer') {
      wx.showModal({
        title: '导出提示',
        content: '当前为云端部署，请在 Web 管理端导出 Word 听课记录表。',
        showCancel: false
      })
      return
    }
    this.setData({ actingId: id, actionError: '' })
    const token = getToken()
    wx.request({
      url: `${apiConfig.baseUrl}/api/observations/${id}/export`,
      method: 'POST',
      header: token ? { Authorization: `Bearer ${token}` } : {},
      responseType: 'arraybuffer',
      success: (res) => {
        this.setData({ actingId: 0 })
        if (res.statusCode !== 200) {
          const msg = (res.data && (res.data as any).error) || '导出失败'
          this.setData({ actionError: msg })
          return
        }
        const fs = wx.getFileSystemManager()
        const path = `${wx.env.USER_DATA_PATH}/observation-${id}.docx`
        fs.writeFile({
          filePath: path,
          data: res.data as ArrayBuffer,
          success: () => {
            wx.openDocument({
              filePath: path,
              fileType: 'docx',
              showMenu: true,
              fail: () => wx.showToast({ title: '已导出，请到文件管理查看', icon: 'none' })
            })
          },
          fail: () => this.setData({ actionError: '保存文件失败' })
        })
      },
      fail: () => {
        this.setData({ actingId: 0, actionError: '导出失败，请检查网络' })
      }
    })
  }
})
