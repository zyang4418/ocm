import { apiConfig, streamBaseUrl } from '../config/api'
import { getToken } from './storage'
import { notifyUnauthorized } from './request'

/**
 * Minimal SSE client over wx.request, used only for POST /api/ai/chat. The
 * backend streams "event: <name>\ndata: <json>\n\n" frames and keeps the
 * response open until done/error.
 *
 * Transport strategy:
 * - Prefer wx.request + enableChunked (base library >= 2.20.1), the only
 *   streaming path available to mini-programs. Works on real devices.
 * - Some environments (notably the DevTools simulator) buffer chunked
 *   responses: onChunkReceived never fires and the body is empty. When a
 *   chunked attempt delivers nothing, retry the same request once without
 *   chunked mode (full body arrives at completion) and remember the
 *   capability for subsequent calls. Streaming turns into one-shot text in
 *   such environments, but the answer still arrives.
 *
 * wx.cloud.callContainer cannot stream, so this always uses wx.request
 * against a direct HTTP(S) base URL (baseUrl in http mode, streamBaseUrl
 * otherwise — see config/api.ts).
 */

const STREAM_TIMEOUT = 120000 // AI turns with tool calls can run long

export interface StreamChatOptions {
  path: string
  body: any
  onEvent: (name: string, data: any) => void
  onDone: () => void
  onError: (message: string) => void
}

export interface StreamHandle {
  abort: () => void
}

// true = chunked delivery proven working, false = buffered mode required,
// null = unknown (try chunked).
let chunkedOk: boolean | null = null

export function streamChat(opts: StreamChatOptions): StreamHandle {
  let buffer = ''
  // Chunk boundaries may split a multi-byte UTF-8 char; carry the trailing
  // incomplete bytes across chunks.
  let pendingBytes: number[] = []
  let settled = false
  let task: WechatMiniprogram.RequestTask | null = null

  const finish = (fn: () => void) => {
    if (settled) return
    settled = true
    fn()
  }

  function decodeAppend(buf: ArrayBuffer): string {
    const all = pendingBytes.concat(Array.from(new Uint8Array(buf)))
    pendingBytes = []
    let out = ''
    let i = 0
    while (i < all.length) {
      const b = all[i]
      let len: number
      if (b < 0x80) len = 1
      else if ((b & 0xe0) === 0xc0) len = 2
      else if ((b & 0xf0) === 0xe0) len = 3
      else if ((b & 0xf8) === 0xf0) len = 4
      else len = 1 // stray continuation byte: emit as-is
      if (i + len > all.length) {
        pendingBytes = all.slice(i)
        break
      }
      let code: number
      if (len === 1) code = b
      else if (len === 2) code = ((b & 0x1f) << 6) | (all[i + 1] & 0x3f)
      else if (len === 3) code = ((b & 0x0f) << 12) | ((all[i + 1] & 0x3f) << 6) | (all[i + 2] & 0x3f)
      else code = ((b & 0x07) << 18) | ((all[i + 1] & 0x3f) << 12) | ((all[i + 2] & 0x3f) << 6) | (all[i + 3] & 0x3f)
      out +=
        code > 0xffff
          ? String.fromCharCode(0xd800 + ((code - 0x10000) >> 10), 0xdc00 + ((code - 0x10000) & 0x3ff))
          : String.fromCharCode(code)
      i += len
    }
    return out
  }

  function handleFrame(frame: string) {
    let name = ''
    const dataLines: string[] = []
    for (const line of frame.split('\n')) {
      if (line.indexOf('event:') === 0) name = line.slice(6).trim()
      else if (line.indexOf('data:') === 0) dataLines.push(line.slice(5).trimStart())
    }
    if (!name || !dataLines.length) return
    try {
      opts.onEvent(name, JSON.parse(dataLines.join('\n')))
    } catch {
      // Non-JSON frame: skip it rather than killing the stream.
    }
  }

  function parseChunk(text: string) {
    buffer += text.replace(/\r\n/g, '\n')
    let idx: number
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      const frame = buffer.slice(0, idx)
      buffer = buffer.slice(idx + 2)
      handleFrame(frame)
    }
  }

  function flushResidual() {
    if (buffer.trim()) {
      handleFrame(buffer)
      buffer = ''
    }
  }

  let base: string
  try {
    base = streamBaseUrl()
  } catch (err: any) {
    opts.onError((err && err.message) || 'AI 对话需配置流式服务地址')
    return { abort: () => {} }
  }

  const header: Record<string, string> = {
    'content-type': 'application/json',
    Accept: 'text/event-stream'
  }
  const token = getToken()
  if (token) header['Authorization'] = `Bearer ${token}`

  function launch(useChunked: boolean, isRetry: boolean) {
    let gotChunks = 0
    let respData: any = null
    task = wx.request({
      url: base + opts.path,
      method: 'POST',
      data: opts.body,
      header,
      timeout: STREAM_TIMEOUT,
      enableChunked: useChunked || undefined,
      success: (res: any) => {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          if (typeof res.onChunkReceived === 'function') {
            res.onChunkReceived((chunk: any) => {
              if (settled) return
              gotChunks++
              parseChunk(decodeAppend(chunk.data))
            })
          }
          respData = res.data
        } else {
          // Non-2xx: the backend answered with a plain JSON error. In chunked
          // mode res.data may arrive as a raw string, so parse it too.
          let message = '请求失败'
          let d = res.data
          if (typeof d === 'string' && d) {
            try {
              d = JSON.parse(d)
            } catch {
              message = d
            }
          }
          if (d && typeof d === 'object' && typeof d.error === 'string') message = d.error
          if (res.statusCode === 401) notifyUnauthorized()
          finish(() => opts.onError(message))
        }
      },
      fail: (err: any) => {
        const msg = (err && err.errMsg) || ''
        if (msg.indexOf('abort') >= 0) {
          finish(() => opts.onError('已停止'))
        } else {
          finish(() => opts.onError('网络异常，请检查网络后重试'))
        }
      },
      complete: () => {
        if (settled) return
        if (gotChunks > 0) {
          flushResidual()
          if (chunkedOk === null) chunkedOk = true
          finish(opts.onDone)
          return
        }
        // 无分片到达:若完整响应体可用,整体解析兜底(部分环境把流式响应
        // 缓冲进 res.data)。
        if (typeof respData === 'string' && respData) {
          parseChunk(respData)
          flushResidual()
          if (chunkedOk === null) chunkedOk = true
          finish(opts.onDone)
          return
        }
        // chunked 模式一无所获(DevTools 模拟器已知缺陷):自动以缓冲模式
        // 重试一次,并把能力记忆下来。
        if (useChunked && !isRetry && chunkedOk !== false) {
          chunkedOk = false
          launch(false, true)
          return
        }
        flushResidual()
        finish(opts.onDone)
      }
    })
  }

  launch(chunkedOk !== false, false)

  return {
    abort() {
      if (task) task.abort()
    }
  }
}
