import { useCallback, useMemo, useRef } from 'react'
import { ChatContainer } from '@carbon/ai-chat'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiStream } from '../auth/api.js'
import i18n from '../i18n/index.js'
import { aiChatState } from './chatInstance.js'
import renderAiCustomItem from './aiChatItems.jsx'

// Carbon AI Chat ships its own built-in English UI strings; we only override
// them for zh-CN (see AiChat.strings below). English therefore falls back to
// Carbon's defaults, per the i18n plan.

// AiChat mounts Carbon AI Chat's float layout (bottom-right launcher + pop-over
// window). It lives in AppShell so the conversation survives page navigation.
// Streaming still goes through the existing SSE endpoint (apiStream); the
// events are mapped onto the chat's chunk protocol below.
export default function AiChat() {
  const { t } = useTranslation('aiChat')
  const { token } = useAuth()
  const tokenRef = useRef(token)
  tokenRef.current = token
  // Conversation history sent to the backend ({role, content} pairs).
  const historyRef = useRef([])

  // Carbon AI Chat UI strings. English (and any non-zh language) returns
  // undefined so Carbon's built-in English strings are used; zh-CN overrides
  // every visible string from the aiChat namespace.
  const strings = useMemo(() => {
    if (!i18n.language?.startsWith('zh')) return undefined
    return t('strings', { returnObjects: true })
  }, [i18n.language, t])

  const homescreen = useMemo(
    () => ({
      isOn: true,
      greeting: t('homescreen.greeting'),
      starters: {
        isOn: true,
        buttons: t('homescreen.starters', { returnObjects: true }).map((label) => ({ label })),
      },
    }),
    [t],
  )

  // customSendMessage is invoked by the chat for every user message. It never
  // returns the response - chunks are pushed through instance.messaging. Text
  // partial chunks are appended by the client, so only the NEW delta is sent;
  // user_defined chunks carry the full payload each time.
  const customSendMessage = useCallback(async (request, { signal }, instance) => {
    const text = (request.input?.text ?? '').trim()
    if (!text || request.history?.is_welcome_request) return

    const responseId = `ai-${Date.now()}`
    let textAcc = ''
    let tools = []
    let proposal = null
    let failed = ''
    let aborted = false

    const chunk = (partialItem) =>
      instance.messaging.addMessageChunk({
        partial_item: partialItem,
        streaming_metadata: { response_id: responseId },
      })

    // Always deliver a final state, even on cancel/error, so the message stops
    // streaming. Streamed items keep their ids to avoid remounts.
    const finalize = async () => {
      const generic = []
      if (tools.length > 0) {
        generic.push({
          response_type: 'user_defined',
          user_defined: { user_defined_type: 'ai_tools', tools },
          streaming_metadata: { id: 'tools' },
        })
      }
      if (textAcc) {
        generic.push({
          response_type: 'text',
          text: textAcc,
          streaming_metadata: { id: 'text', stream_stopped: aborted || undefined },
        })
      }
      if (proposal) {
        generic.push({
          response_type: 'user_defined',
          user_defined: { user_defined_type: 'ai_proposal', proposal },
          streaming_metadata: { id: 'proposal' },
        })
      }
      if (failed) {
        generic.push({ response_type: 'inline_error', text: failed, streaming_metadata: { id: 'error' } })
      }
      await instance.messaging.addMessageChunk({
        final_response: { id: responseId, output: { generic } },
      })
    }

    const onEvent = (name, data) => {
      if (name === 'delta' && data?.content) {
        textAcc += data.content
        chunk({
          response_type: 'text',
          text: data.content,
          streaming_metadata: { id: 'text', cancellable: true },
        })
      } else if (name === 'tool') {
        if (data.status === 'running') {
          tools = [...tools, { name: data.name, status: 'running' }]
        } else {
          tools = tools.map((tool) => (tool.name === data.name && tool.status === 'running' ? { ...tool, status: data.status } : tool))
        }
        chunk({
          response_type: 'user_defined',
          user_defined: { user_defined_type: 'ai_tools', tools: tools.map((tool) => ({ ...tool })) },
          streaming_metadata: { id: 'tools' },
        })
      } else if (name === 'proposal') {
        proposal = { ...data, state: 'proposed', error: '' }
        chunk({
          response_type: 'user_defined',
          user_defined: { user_defined_type: 'ai_proposal', proposal },
          streaming_metadata: { id: 'proposal' },
        })
      } else if (name === 'error') {
        failed = data?.message || i18n.t('error.fallback', { ns: 'aiChat' })
      }
      // 'done' needs no handling: the SSE stream ending triggers finalize.
    }

    const onAbort = () => controller.abort()
    signal?.addEventListener('abort', onAbort, { once: true })
    const { promise, controller } = apiStream('/api/ai/chat', {
      token: tokenRef.current,
      body: { messages: [...historyRef.current, { role: 'user', content: text }] },
      onEvent,
    })
    try {
      await promise
    } catch (err) {
      if (err.name === 'AbortError') aborted = true
      else failed = err.message
    } finally {
      signal?.removeEventListener('abort', onAbort)
      historyRef.current = [...historyRef.current, { role: 'user', content: text }]
      if (textAcc) historyRef.current.push({ role: 'assistant', content: textAcc })
      if (aborted) tools = tools.map((tool) => (tool.status === 'running' ? { ...tool, status: 'error' } : tool))
      await finalize()
    }
  }, [])

  const messaging = useMemo(
    () => ({
      customSendMessage,
      skipWelcome: true,
      showStopButtonImmediately: true,
      messageTimeoutSecs: 300,
    }),
    [customSendMessage],
  )

  const onBeforeRender = useCallback((instance) => {
    aiChatState.instance = instance
    // An open request may have fired before the lazy widget finished loading.
    if (aiChatState.openRequested) {
      aiChatState.openRequested = false
      instance.changeView('mainWindow')
    }
    instance.on({ type: 'restartConversation', handler: () => {
      historyRef.current = []
    } })
  }, [])

  const renderUserDefinedResponse = useCallback((state) => renderAiCustomItem(state), [])

  return (
    <ChatContainer
      messaging={messaging}
      onBeforeRender={onBeforeRender}
      renderUserDefinedResponse={renderUserDefinedResponse}
      injectCarbonTheme="g10"
      assistantName={t('assistantName')}
      header={{ title: 'OCM', name: t('assistantName'), showRestartButton: true }}
      homescreen={homescreen}
      strings={strings}
    />
  )
}
