import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Button, InlineNotification, Loading, Tag } from '@carbon/react'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'

// Labels for the assistant's tools, shown in the per-message tool indicators.
const toolLabels = {
  list_classrooms: '查询教室',
  query_availability: '查询空闲教室',
  query_timetable: '查询课表',
  propose_booking: '生成预约方案',
}

// These components render inside Carbon AI Chat's user_defined slots. The chat
// renders them through a React portal from the app tree, so contexts
// (useAuth) and the global Carbon styles apply normally.

function ToolIndicator({ tools }) {
  if (!tools || tools.length === 0) return null
  return (
    <div className="ai-chat-item__tools">
      {tools.map((t, i) => {
        const label = toolLabels[t.name] ?? t.name
        let type = 'cyan'
        let text = `${label}中…`
        let spinning = true
        if (t.status === 'ok') {
          type = 'green'
          text = `已${label}`
          spinning = false
        } else if (t.status === 'error') {
          type = 'red'
          text = `${label}失败`
          spinning = false
        }
        return (
          <Tag key={`${t.name}-${i}`} type={type} size="sm">
            {spinning && <Loading small withOverlay={false} className="ai-chat-item__tool-spinner" />}
            {text}
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
// confirmation state machine: proposed -> submitting -> confirmed / failed, or
// dismissed. The confirm button submits through the existing booking API,
// which re-validates permissions and conflicts server-side.
function ProposalCard({ proposal }) {
  const { token } = useAuth()
  const [state, setState] = useState('proposed') // proposed | submitting | confirmed | failed | dismissed
  const [error, setError] = useState('')

  if (state === 'dismissed') {
    return <p className="ai-chat-item__proposal-dismissed">已取消此预约方案。</p>
  }

  const confirm = async () => {
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
      setError(err.status === 409 ? '该教室该时段已被占用，请重新选择' : err.message)
      setState('failed')
    }
  }

  return (
    <div className="ai-chat-item__proposal">
      <h3 className="ai-chat-item__proposal-title">预约预览</h3>
      <div className="ai-chat-item__proposal-fields">
        <span>教室：{proposal.payload.classroomName}</span>
        <span>日期：{proposal.payload.date}</span>
        <span>节次：{proposal.payload.periodLabel}</span>
        <span>用途：{proposal.payload.purpose}</span>
      </div>
      {state === 'proposed' && <ConflictNote conflicts={proposal.payload.conflicts} />}
      {state === 'confirmed' && (
        <InlineNotification kind="success" lowContrast hideCloseButton title="已提交预约，等待管理员审批">
          <Link to="/bookings">查看预约</Link>
        </InlineNotification>
      )}
      {state === 'failed' && (
        <InlineNotification kind="error" lowContrast hideCloseButton title="预约提交失败" subtitle={error} />
      )}
      {(state === 'proposed' || state === 'submitting') && (
        <div className="ai-chat-item__proposal-actions">
          <Button size="sm" kind="primary" disabled={state === 'submitting'} onClick={confirm}>
            {state === 'submitting' ? '提交中…' : '确认预约'}
          </Button>
          <Button size="sm" kind="ghost" disabled={state === 'submitting'} onClick={() => setState('dismissed')}>
            取消
          </Button>
        </div>
      )}
    </div>
  )
}

// renderAiCustomItem maps the assistant's user_defined message items to React
// components. Called by Carbon AI Chat on every rerender: keep it side-effect
// free. During streaming messageItem is not set yet - fall back to the newest
// partial chunk.
export default function renderAiCustomItem(renderState) {
  const item = renderState?.messageItem ?? renderState?.partialItems?.at(-1)
  const userDefined = item?.user_defined
  if (userDefined?.user_defined_type === 'ai_tools') {
    return <ToolIndicator tools={userDefined.tools} />
  }
  if (userDefined?.user_defined_type === 'ai_proposal') {
    return <ProposalCard proposal={userDefined.proposal} />
  }
  return null
}
