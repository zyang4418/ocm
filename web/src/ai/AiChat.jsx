import { useCallback, useMemo, useRef } from 'react'
import { ChatContainer } from '@carbon/ai-chat'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiStream } from '../auth/api.js'
import { aiChatState } from './chatInstance.js'
import renderAiCustomItem from './aiChatItems.jsx'

// Carbon AI Chat only ships an English UI language pack (dayjs has zh locales,
// the UI strings do not), so override the visible strings here.
const zhStrings = {
  input_placeholder: '例如：周一 5-7 节有哪些可容纳 50 人的空闲教室？',
  input_ariaLabel: '向 AI 助手提问',
  input_buttonLabel: '发送',
  window_title: 'AI 助手',
  window_ariaChatRegion: 'AI 助手对话',
  launcher_isOpen: '打开 AI 助手',
  launcher_isClosed: '收起 AI 助手',
  launcher_desktopGreeting: '有什么可以帮您？',
  launcher_mobileGreeting: '有什么可以帮您？',
  messages_youSaid: '您',
  messages_assistantSaid: 'AI 助手',
  errors_communicating: '无法连接 AI 助手，请稍后重试',
  errors_somethingWrong: '出错了，请稍后重试',
  homeScreen_returnToAssistant: '返回对话',
  homeScreen_returnToHome: '返回首页',
  buttons_restart: '重新开始',
  buttons_cancel: '取消',
  buttons_retry: '重试',
}

const homescreen = {
  isOn: true,
  greeting: '按您的权限查询教室、空闲时段与课表；预约操作需在预览中点击确认后才会提交。',
  starters: {
    isOn: true,
    buttons: [
      { label: '周一 5-7 节有哪些可容纳 50 人的空闲教室？' },
      { label: '查询 A 栋所有教室' },
      { label: '明天 3-4 节 302 教室有没有课？' },
      { label: '帮我预约一间周五下午的多媒体教室' },
    ],
  },
}

// AiChat mounts Carbon AI Chat's float layout (bottom-right launcher + pop-over
// window). It lives in AppShell so the conversation survives page navigation.
// Streaming still goes through the existing SSE endpoint (apiStream); the
// events are mapped onto the chat's chunk protocol below.
export default function AiChat() {
  const { token } = useAuth()
  const tokenRef = useRef(token)
  tokenRef.current = token
  // Conversation history sent to the backend ({role, content} pairs).
  const historyRef = useRef([])

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
          tools = tools.map((t) => (t.name === data.name && t.status === 'running' ? { ...t, status: data.status } : t))
        }
        chunk({
          response_type: 'user_defined',
          user_defined: { user_defined_type: 'ai_tools', tools: tools.map((t) => ({ ...t })) },
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
        failed = data?.message || 'AI 助手暂时无法回答，请稍后重试'
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
      if (aborted) tools = tools.map((t) => (t.status === 'running' ? { ...t, status: 'error' } : t))
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
      assistantName="AI 助手"
      header={{ title: 'OCM', name: 'AI 助手', showRestartButton: true }}
      homescreen={homescreen}
      strings={zhStrings}
    />
  )
}
