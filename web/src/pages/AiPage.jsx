import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Column,
  Grid,
  InlineNotification,
  Loading,
  Tag,
  TextArea,
} from '@carbon/react'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch, apiStream } from '../auth/api.js'

// Labels for the assistant's tools, shown in the per-message tool indicators.
const toolLabels = {
  list_classrooms: '查询教室',
  query_availability: '查询空闲教室',
  query_timetable: '查询课表',
  propose_booking: '生成预约方案',
}

const suggestions = [
  '周一 5-7 节有哪些可容纳 50 人的空闲教室？',
  '查询 A 栋所有教室',
  '明天 3-4 节 302 教室有没有课？',
  '帮我预约一间周五下午的多媒体教室',
]

function ToolIndicator({ tools }) {
  if (!tools || tools.length === 0) return null
  return (
    <div className="ai-page__tools">
      {tools.map((t, i) => {
        const label = toolLabels[t.name] ?? t.name
        const tag = { type: 'cyan', text: `${label}中…`, icon: true }
        if (t.status === 'ok') {
          tag.type = 'green'
          tag.text = `已${label}`
          tag.icon = false
        } else if (t.status === 'error') {
          tag.type = 'red'
          tag.text = `${label}失败`
          tag.icon = false
        }
        return (
          <Tag key={`${t.name}-${i}`} type={tag.type} size="sm">
            {tag.icon && <Loading small withOverlay={false} className="ai-page__tool-spinner" />}
            {tag.text}
          </Tag>
        )
      })}
    </div>
  )
}

function ConflictNote({ conflicts }) {
  if (!conflicts || conflicts.length === 0) return null
  return (
    <InlineNotification
      kind="warning"
      lowContrast
      hideCloseButton
      title={`该时段存在 ${conflicts.length} 项冲突`}
      subtitle={conflicts
        .slice(0, 5)
        .map(
          (c) =>
            `${c.kind === 'session' ? '课程' : '预约'}：第${c.periodStart}-${c.periodEnd}节` +
            `${c.courseName ? ` ${c.courseName}` : ''}${c.displayName ? `（${c.displayName}）` : ''}`,
        )
        .join('；')}
    />
  )
}

// ProposalCard renders one AI-generated booking preview with its own
// confirmation state machine: proposed → submitting → confirmed / failed, or
// dismissed. The confirm button submits through the existing booking API,
// which re-validates permissions and conflicts server-side.
function ProposalCard({ proposal, onConfirm, onDismiss }) {
  const p = proposal.payload
  if (proposal.state === 'dismissed') {
    return <p className="ai-page__proposal-dismissed">已取消此预约方案。</p>
  }
  return (
    <div className="ai-page__proposal">
      <h3 className="ai-page__proposal-title">预约预览</h3>
      <div className="ai-page__proposal-fields">
        <span>教室：{p.classroomName}</span>
        <span>日期：{p.date}</span>
        <span>节次：{p.periodLabel}</span>
        <span>用途：{p.purpose}</span>
      </div>
      {proposal.state === 'proposed' && <ConflictNote conflicts={p.conflicts} />}
      {proposal.state === 'confirmed' && (
        <InlineNotification
          kind="success"
          lowContrast
          hideCloseButton
          title="已提交预约，等待管理员审批"
        >
          <Link to="/bookings">查看预约</Link>
        </InlineNotification>
      )}
      {proposal.state === 'failed' && (
        <InlineNotification kind="error" lowContrast hideCloseButton title="预约提交失败" subtitle={proposal.error} />
      )}
      {(proposal.state === 'proposed' || proposal.state === 'submitting') && (
        <div className="ai-page__proposal-actions">
          <Button
            size="sm"
            kind="primary"
            disabled={proposal.state === 'submitting'}
            onClick={() => onConfirm()}
          >
            {proposal.state === 'submitting' ? '提交中…' : '确认预约'}
          </Button>
          <Button size="sm" kind="ghost" disabled={proposal.state === 'submitting'} onClick={onDismiss}>
            取消
          </Button>
        </div>
      )}
    </div>
  )
}

function MessageBubble({ message, onConfirm, onDismiss }) {
  const isUser = message.role === 'user'
  return (
    <div className={`ai-page__row ${isUser ? 'ai-page__row--user' : 'ai-page__row--assistant'}`}>
      <div className={`ai-page__bubble ${isUser ? 'ai-page__bubble--user' : 'ai-page__bubble--assistant'}`}>
        {message.content && <p className="ai-page__text">{message.content}</p>}
        {message.streaming && !message.content && <Loading small withOverlay={false} />}
        {message.streaming && message.content && <span className="ai-page__cursor" aria-hidden />}
        <ToolIndicator tools={message.tools} />
        {message.proposal && <ProposalCard proposal={message.proposal} onConfirm={onConfirm} onDismiss={onDismiss} />}
      </div>
    </div>
  )
}

export default function AiPage() {
  const { token, can } = useAuth()
  const [messages, setMessages] = useState([])
  const [input, setInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const assistantIdRef = useRef(null)
  const streamRef = useRef(null)
  const bottomRef = useRef(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (!can('ai:chat')) {
    return (
      <div className="ai-page">
        <Grid fullWidth>
          <Column sm={4} md={8} lg={16}>
            <Breadcrumb>
              <BreadcrumbItem href="/">首页</BreadcrumbItem>
              <BreadcrumbItem isCurrentPage>AI 助手</BreadcrumbItem>
            </Breadcrumb>
            <h1 className="ai-page__heading">AI 助手</h1>
            <InlineNotification
              kind="info"
              lowContrast
              hideCloseButton
              title="没有使用权限"
              subtitle="你没有使用 AI 助手的权限，如需使用请联系管理员在组织与权限中授权。"
            />
          </Column>
        </Grid>
      </div>
    )
  }

  const updateAssistant = (updater) => {
    const id = assistantIdRef.current
    if (!id) return
    setMessages((prev) => prev.map((m) => (m.id === id ? updater(m) : m)))
  }

  const send = () => {
    const text = input.trim()
    if (!text || busy) return
    const now = Date.now()
    const history = [
      ...messages.filter((m) => m.content),
      { id: `u${now}`, role: 'user', content: text },
    ]
    setMessages(history)
    setInput('')
    setError('')
    setBusy(true)

    const assistantId = `a${now}`
    assistantIdRef.current = assistantId
    setMessages((prev) => [
      ...prev,
      { id: assistantId, role: 'assistant', content: '', streaming: true, tools: [], proposal: null },
    ])

    const onEvent = (name, data) => {
      if (name === 'delta') {
        updateAssistant((m) => ({ ...m, content: m.content + data.content }))
      } else if (name === 'tool') {
        if (data.status === 'running') {
          updateAssistant((m) => ({ ...m, tools: [...m.tools, { name: data.name, status: 'running' }] }))
        } else {
          updateAssistant((m) => ({
            ...m,
            tools: m.tools.map((t) => (t.name === data.name && t.status === 'running' ? { ...t, status: data.status } : t)),
          }))
        }
      } else if (name === 'proposal') {
        updateAssistant((m) => ({ ...m, proposal: { ...data, state: 'proposed', error: '' } }))
      } else if (name === 'done') {
        updateAssistant((m) => ({ ...m, streaming: false }))
      } else if (name === 'error') {
        setError(data.message)
        updateAssistant((m) => ({ ...m, streaming: false }))
      }
    }

    const apiHistory = history.map(({ role, content }) => ({ role, content }))
    const { promise, controller } = apiStream('/api/ai/chat', {
      token,
      body: { messages: apiHistory },
      onEvent,
    })
    streamRef.current = controller
    promise
      .catch((err) => {
        if (err.name === 'AbortError') {
          updateAssistant((m) => ({ ...m, streaming: false }))
        } else {
          setError(err.message)
          updateAssistant((m) => ({ ...m, streaming: false }))
        }
      })
      .finally(() => {
        setBusy(false)
        streamRef.current = null
      })
  }

  const stop = () => {
    streamRef.current?.abort()
  }

  const confirmProposal = async (messageId) => {
    const message = messages.find((m) => m.id === messageId)
    const proposal = message?.proposal
    if (!proposal || proposal.state !== 'proposed') return
    const setState = (state, extra = {}) => {
      setMessages((prev) =>
        prev.map((m) => (m.id === messageId ? { ...m, proposal: { ...m.proposal, state, ...extra } } : m)),
      )
    }
    setState('submitting')
    try {
      await apiFetch('/api/bookings', {
        method: 'POST',
        token,
        body: {
          classroomId: proposal.payload.classroomId,
          date: proposal.payload.date,
          periodStart: proposal.payload.periodStart,
          periodEnd: proposal.payload.periodEnd,
          purpose: proposal.payload.purpose,
        },
      })
      setState('confirmed')
    } catch (err) {
      setState('failed', { error: err.status === 409 ? '该教室该时段已被占用，请重新选择' : err.message })
    }
  }

  const dismissProposal = (messageId) => {
    setMessages((prev) =>
      prev.map((m) => (m.id === messageId ? { ...m, proposal: { ...m.proposal, state: 'dismissed' } } : m)),
    )
  }

  return (
    <div className="ai-page">
      <Grid fullWidth>
        <Column sm={4} md={8} lg={16}>
          <Breadcrumb>
            <BreadcrumbItem href="/">首页</BreadcrumbItem>
            <BreadcrumbItem isCurrentPage>AI 助手</BreadcrumbItem>
          </Breadcrumb>
          <h1 className="ai-page__heading">AI 助手</h1>
          <p className="ai-page__subtitle">
            按您的权限查询教室、空闲时段与课表；预约操作需在预览中点击确认后才会提交。
          </p>

          <div className="ai-page__chat">
            {error && (
              <InlineNotification
                kind="error"
                lowContrast
                title="出错了"
                subtitle={error}
                onClose={() => setError('')}
              />
            )}

            {messages.length === 0 && (
              <div className="ai-page__empty">
                <p className="ai-page__empty-title">有什么可以帮您？</p>
                <p className="ai-page__empty-hint">例如：</p>
                <div className="ai-page__chips">
                  {suggestions.map((s) => (
                    <button key={s} type="button" className="ai-page__chip" onClick={() => setInput(s)}>
                      {s}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {messages.map((m) => (
              <MessageBubble
                key={m.id}
                message={m}
                onConfirm={() => confirmProposal(m.id)}
                onDismiss={() => dismissProposal(m.id)}
              />
            ))}
            <div ref={bottomRef} />

            <div className="ai-page__input-row">
              <TextArea
                id="ai-input"
                labelText="向 AI 助手提问"
                hideLabel
                placeholder="例如：周一 5-7 节有哪些可容纳 50 人的空闲教室？"
                rows={2}
                value={input}
                disabled={busy}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    send()
                  }
                }}
              />
              {busy ? (
                <Button kind="ghost" onClick={stop}>
                  停止
                </Button>
              ) : (
                <Button onClick={send} disabled={input.trim() === ''}>
                  发送
                </Button>
              )}
            </div>
          </div>
        </Column>
      </Grid>
    </div>
  )
}
