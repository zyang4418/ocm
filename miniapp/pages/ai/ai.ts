import { getNavInfo } from '../../utils/nav'
import { ensureAuth } from '../../utils/auth'
import { can } from '../../utils/perms'
import { streamChat, StreamHandle } from '../../utils/sse'
import { request } from '../../utils/request'

// 首帧渲染即需正确高度（避免 100vh 在部分机型初始计算偏差），故在模块级同步取值。
const nav = getNavInfo()

interface ToolState {
  name: string
  status: string // running | ok | error
}

interface Proposal {
  payload: {
    classroomId: number
    classroomName: string
    date: string
    periodStart: number
    periodEnd: number
    periodLabel: string
    purpose: string
    conflicts: any[]
  }
  state: 'proposed' | 'submitting' | 'confirmed' | 'failed' | 'dismissed'
  error?: string
}

interface Message {
  id: string
  role: 'user' | 'assistant'
  content: string
  streaming?: boolean
  tools: ToolState[]
  toolTags: { text: string; theme: string }[]
  proposal: Proposal | null
}

// 工具中文名（与 web 端 AiPage 一致）。
const toolLabels: Record<string, string> = {
  list_classrooms: '查询教室',
  query_availability: '查询空闲教室',
  query_timetable: '查询课表',
  propose_booking: '生成预约方案'
}

const suggestions = [
  '周一 5-7 节有哪些可容纳 50 人的空闲教室？',
  '查询 A 栋所有教室',
  '明天 3-4 节 302 教室有没有课？',
  '帮我预约一间周五下午的多媒体教室'
]

Page({
  data: {
    statusBarHeight: nav.statusBarHeight,
    safeAreaBottom: nav.safeAreaBottom,
    pageHeight: nav.pageHeight,
    canUse: false,
    messages: [] as Message[],
    inputValue: '',
    busy: false,
    error: '',
    scrollInto: '',
    suggestions
  },

  _stream: null as StreamHandle | null,

  async onLoad() {
    const ok = await ensureAuth()
    if (!ok) return
    this.setData({ canUse: can('ai:chat') })
  },

  onTapSuggest(e: WechatMiniprogram.TouchEvent) {
    const { text } = e.currentTarget.dataset
    this.setData({ inputValue: text })
  },

  onInput(e: WechatMiniprogram.Input) {
    this.setData({ inputValue: e.detail.value })
  },

  /** Path-based patch onto one message (e.g. 'proposal.state'). */
  updateMessage(id: string, patch: Record<string, any>) {
    const idx = this.data.messages.findIndex((m) => m.id === id)
    if (idx < 0) return
    const upd: Record<string, any> = {}
    for (const key of Object.keys(patch)) {
      upd[`messages[${idx}].${key}`] = patch[key]
    }
    this.setData(upd)
  },

  setTools(id: string, tools: ToolState[]) {
    this.updateMessage(id, {
      tools,
      toolTags: tools.map((t) => {
        const label = toolLabels[t.name] || t.name
        if (t.status === 'running') return { text: `${label}中…`, theme: 'blue' }
        if (t.status === 'ok') return { text: `已${label}`, theme: 'green' }
        return { text: `${label}失败`, theme: 'red' }
      })
    })
  },

  onSend() {
    const text = this.data.inputValue.trim()
    if (!text || this.data.busy) return
    const now = Date.now()
    const history = this.data.messages
      .filter((m) => m.content)
      .map((m) => ({ role: m.role, content: m.content }))
    const userMsg: Message = { id: `u${now}`, role: 'user', content: text, tools: [], toolTags: [], proposal: null }
    const assistantMsg: Message = {
      id: `a${now}`,
      role: 'assistant',
      content: '',
      streaming: true,
      tools: [],
      toolTags: [],
      proposal: null
    }
    this.setData({
      messages: this.data.messages.concat([userMsg, assistantMsg]),
      inputValue: '',
      busy: true,
      error: '',
      scrollInto: `a${now}`
    })

    const assistantId = `a${now}`
    this._stream = streamChat({
      path: '/api/ai/chat',
      body: { messages: history.concat([{ role: 'user', content: text }]) },
      onEvent: (name, data) => this.onAiEvent(assistantId, name, data),
      onDone: () => {
        this.updateMessage(assistantId, { streaming: false })
        this.setData({ busy: false, scrollInto: assistantId })
      },
      onError: (message) => {
        this.updateMessage(assistantId, { streaming: false })
        // '已停止' 是用户主动打断,不算错误。
        this.setData({ busy: false, error: message === '已停止' ? '' : message })
      }
    })
  },

  onAiEvent(id: string, name: string, data: any) {
    const idx = this.data.messages.findIndex((m) => m.id === id)
    if (idx < 0) return
    const m = this.data.messages[idx]
    if (name === 'delta') {
      this.updateMessage(id, { content: m.content + (data.content || '') })
      this.setData({ scrollInto: id })
    } else if (name === 'tool') {
      if (data.status === 'running') {
        this.setTools(id, m.tools.concat([{ name: data.name, status: 'running' }]))
      } else {
        this.setTools(
          id,
          m.tools.map((t) => (t.name === data.name && t.status === 'running' ? { ...t, status: data.status } : t))
        )
      }
    } else if (name === 'proposal') {
      const payload = data.payload || {}
      const conflicts = (payload.conflicts || []).slice(0, 5).map((c: any) => {
        const kind = c.kind === 'session' ? '课程' : '预约'
        return (
          `${kind}：第${c.periodStart}-${c.periodEnd}节` +
          `${c.courseName ? ` ${c.courseName}` : ''}${c.displayName ? `（${c.displayName}）` : ''}`
        )
      })
      this.updateMessage(id, {
        proposal: {
          payload,
          state: 'proposed',
          error: '',
          conflictCount: (payload.conflicts || []).length,
          conflictsText: conflicts.join('；')
        }
      })
    } else if (name === 'done') {
      this.updateMessage(id, { streaming: false })
    } else if (name === 'error') {
      this.setData({ error: data.message || '出错了' })
      this.updateMessage(id, { streaming: false })
    }
  },

  onStop() {
    if (this._stream) this._stream.abort()
  },

  /** 确认 AI 生成的预约方案 → 走真实预约接口（后端二次校验冲突）。 */
  async onConfirmProposal(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    const idx = this.data.messages.findIndex((m) => m.id === id)
    if (idx < 0) return
    const proposal = this.data.messages[idx].proposal
    if (!proposal || proposal.state !== 'proposed') return
    this.updateMessage(id, { 'proposal.state': 'submitting' })
    try {
      await request({
        path: '/api/bookings',
        method: 'POST',
        data: {
          classroomId: proposal.payload.classroomId,
          date: proposal.payload.date,
          periodStart: proposal.payload.periodStart,
          periodEnd: proposal.payload.periodEnd,
          purpose: proposal.payload.purpose
        }
      })
      this.updateMessage(id, { 'proposal.state': 'confirmed' })
    } catch (err: any) {
      this.updateMessage(id, {
        'proposal.state': 'failed',
        'proposal.error': err && err.statusCode === 409 ? '该教室该时段已被占用，请重新选择' : (err && err.message) || '提交失败'
      })
    }
  },

  onDismissProposal(e: WechatMiniprogram.TouchEvent) {
    const { id } = e.currentTarget.dataset
    this.updateMessage(id, { 'proposal.state': 'dismissed' })
  }
})
