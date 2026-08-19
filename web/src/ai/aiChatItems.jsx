import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Button, InlineNotification, Loading, Tag } from '@carbon/react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiFetch } from '../auth/api.js'

// These components render inside Carbon AI Chat's user_defined slots. The chat
// renders them through a React portal from the app tree, so contexts
// (useAuth, useTranslation) and the global Carbon styles apply normally.

function ToolIndicator({ tools }) {
  const { t } = useTranslation('aiChat')
  if (!tools || tools.length === 0) return null
  return (
    <div className="ai-chat-item__tools">
      {tools.map((tool, i) => {
        const label = t('tools.' + tool.name, { defaultValue: tool.name })
        let type = 'cyan'
        let text = t('tool.running', { label })
        let spinning = true
        if (tool.status === 'ok') {
          type = 'green'
          text = t('tool.ok', { label })
          spinning = false
        } else if (tool.status === 'error') {
          type = 'red'
          text = t('tool.error', { label })
          spinning = false
        }
        return (
          <Tag key={`${tool.name}-${i}`} type={type} size="sm">
            {spinning && <Loading small withOverlay={false} className="ai-chat-item__tool-spinner" />}
            {text}
          </Tag>
        )
      })}
    </div>
  )
}

function ConflictNote({ conflicts }) {
  const { t } = useTranslation('aiChat')
  if (!conflicts || conflicts.length === 0) return null
  const items = conflicts.slice(0, 5).map((c) => {
    const kind = c.kind === 'session' ? t('conflict.sessionKind') : t('conflict.bookingKind')
    const course = c.courseName ? ' ' + c.courseName : ''
    const name = c.displayName ? t('conflict.nameWrap', { name: c.displayName }) : ''
    return t('conflict.item', { kind, start: c.periodStart, end: c.periodEnd, course, name })
  })
  return (
    <InlineNotification
      kind="warning"
      lowContrast
      hideCloseButton
      title={t('conflict.title', { count: conflicts.length })}
      subtitle={items.join(t('conflict.join'))}
    />
  )
}

// ProposalCard renders one AI-generated booking preview with its own
// confirmation state machine: proposed -> submitting -> confirmed / failed, or
// dismissed. The confirm button submits through the existing booking API,
// which re-validates permissions and conflicts server-side.
function ProposalCard({ proposal }) {
  const { t } = useTranslation('aiChat')
  const { token } = useAuth()
  const [state, setState] = useState('proposed') // proposed | submitting | confirmed | failed | dismissed
  const [error, setError] = useState('')

  if (state === 'dismissed') {
    return <p className="ai-chat-item__proposal-dismissed">{t('proposal.dismissed')}</p>
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
      setError(err.status === 409 ? t('error.bookingConflict') : err.message)
      setState('failed')
    }
  }

  return (
    <div className="ai-chat-item__proposal">
      <h3 className="ai-chat-item__proposal-title">{t('proposal.title')}</h3>
      <div className="ai-chat-item__proposal-fields">
        <span>{t('proposal.field.classroom', { value: proposal.payload.classroomName })}</span>
        <span>{t('proposal.field.date', { value: proposal.payload.date })}</span>
        <span>{t('proposal.field.period', { value: proposal.payload.periodLabel })}</span>
        <span>{t('proposal.field.purpose', { value: proposal.payload.purpose })}</span>
      </div>
      {state === 'proposed' && <ConflictNote conflicts={proposal.payload.conflicts} />}
      {state === 'confirmed' && (
        <InlineNotification kind="success" lowContrast hideCloseButton title={t('proposal.successTitle')}>
          <Link to="/bookings">{t('proposal.successLink')}</Link>
        </InlineNotification>
      )}
      {state === 'failed' && (
        <InlineNotification kind="error" lowContrast hideCloseButton title={t('proposal.failedTitle')} subtitle={error} />
      )}
      {(state === 'proposed' || state === 'submitting') && (
        <div className="ai-chat-item__proposal-actions">
          <Button size="sm" kind="primary" disabled={state === 'submitting'} onClick={confirm}>
            {state === 'submitting' ? t('proposal.confirming') : t('proposal.confirm')}
          </Button>
          <Button size="sm" kind="ghost" disabled={state === 'submitting'} onClick={() => setState('dismissed')}>
            {t('action.cancel', { ns: 'common' })}
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
