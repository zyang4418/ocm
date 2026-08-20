import i18n from '../i18n/index'
import { ApiError } from '../types/api'

// t is a thin common-namespace wrapper so error messages localize without
// pulling react-i18next into non-React modules.
const t = (key: string, options?: Record<string, unknown>) =>
  i18n.t(key, { ns: 'common', ...options })

export interface ApiFetchOptions {
  method?: string
  body?: unknown
  token?: string | null
}

export async function apiFetch<T = unknown>(
  path: string,
  { method = 'GET', body, token }: ApiFetchOptions = {},
): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (token) headers.Authorization = `Bearer ${token}`

  let res: Response
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new Error(t('error.network'))
  }

  const data: unknown = await res.json().catch(() => null)
  if (!res.ok) {
    const message =
      (data as { error?: string } | null)?.error ||
      t('error.requestFailed', { status: res.status })
    throw new ApiError(res.status, message)
  }
  return data as T
}

// apiUpload posts a file as multipart/form-data. Unlike apiFetch it must NOT
// set Content-Type so the browser can attach the multipart boundary. The
// optional `fields` object adds extra text form parts alongside the file (used
// by the jwc_split endpoint, which sends semester + week1_monday with the file).
export async function apiUpload<T = unknown>(
  path: string,
  { file, token, fields = {} }: { file: File; token?: string | null; fields?: Record<string, string> },
): Promise<T> {
  const form = new FormData()
  form.append('file', file)
  for (const [k, v] of Object.entries(fields)) {
    form.append(k, v)
  }
  const headers: Record<string, string> = {}
  if (token) headers.Authorization = `Bearer ${token}`

  let res: Response
  try {
    res = await fetch(path, { method: 'POST', headers, body: form })
  } catch {
    throw new Error(t('error.network'))
  }

  const data: unknown = await res.json().catch(() => null)
  if (!res.ok) {
    const message =
      (data as { error?: string } | null)?.error ||
      t('error.uploadFailed', { status: res.status })
    throw new ApiError(res.status, message)
  }
  return data as T
}

// apiDownload fetches a binary file (xlsx/docx export) as a blob and triggers
// a browser download. The filename is read from the Content-Disposition header
// (decoded for UTF-8 / quoted forms); fallbackName is used when the header is
// absent or unparseable. Export endpoints return the file directly (not JSON),
// so this does not parse the body as JSON. method/body default to GET (xlsx),
// but a POST body is supported for endpoints like the observation docx export.
export async function apiDownload(
  path: string,
  { token, fallbackName = 'export.xlsx', method = 'GET', body }: { token?: string | null; fallbackName?: string; method?: string; body?: unknown } = {},
): Promise<void> {
  const headers: Record<string, string> = {}
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  let res: Response
  try {
    res = await fetch(path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined })
  } catch {
    throw new Error(t('error.network'))
  }

  if (!res.ok) {
    // Export errors come back as JSON {error}, so try to read it.
    const data: unknown = await res.json().catch(() => null)
    const message =
      (data as { error?: string } | null)?.error ||
      t('error.exportFailed', { status: res.status })
    throw new ApiError(res.status, message)
  }

  const blob = await res.blob()
  const filename = parseFilename(res.headers.get('Content-Disposition')) || fallbackName

  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // Defer revocation so the click has time to start the download in all browsers.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// apiStream POSTs JSON and consumes a text/event-stream response (the AI
// assistant's SSE endpoint), invoking onEvent(eventName, dataObject) for every
// frame. `data` is the parsed JSON payload, or null when a frame is malformed -
// narrow it per event name at the call site. Non-2xx responses are read as JSON
// and thrown like apiFetch. Returns { promise, controller } - abort the
// controller to stop the stream mid-way.
export function apiStream(path: string, { body, token, onEvent }: {
  body?: unknown
  token?: string | null
  onEvent: (eventName: string, data: Record<string, any> | null) => void
}): { promise: Promise<void>; controller: AbortController } {
  const controller = new AbortController()
  const promise = (async () => {
    const headers: Record<string, string> = { Accept: 'text/event-stream' }
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    if (token) headers.Authorization = `Bearer ${token}`

    let res: Response
    try {
      res = await fetch(path, {
        method: 'POST',
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      })
    } catch (err) {
      if ((err as Error).name === 'AbortError') throw err
      throw new Error(t('error.network'))
    }
    if (!res.ok) {
      const data: unknown = await res.json().catch(() => null)
      const message =
        (data as { error?: string } | null)?.error ||
        t('error.requestFailed', { status: res.status })
      throw new ApiError(res.status, message)
    }

    const reader = res.body!.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      let idx
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const frame = buf.slice(0, idx)
        buf = buf.slice(idx + 2)
        let eventName = 'message'
        let dataLine = ''
        for (const line of frame.split('\n')) {
          if (line.startsWith('event:')) eventName = line.slice(6).trim()
          else if (line.startsWith('data:')) dataLine += line.slice(5).trim()
        }
        if (!dataLine) continue
        let parsed: Record<string, any> | null = null
        try {
          parsed = JSON.parse(dataLine)
        } catch {
          // Malformed frame: skip it rather than failing the whole turn.
        }
        onEvent(eventName, parsed)
      }
    }
  })()
  return { promise, controller }
}

// parseFilename extracts the filename from a Content-Disposition header,
// handling both the ASCII `filename="x.xlsx"` and the UTF-8
// `filename*=UTF-8''<percent-encoded>` forms. Returns '' when not found.
function parseFilename(disposition: string | null): string {
  if (!disposition) return ''
  // Prefer the UTF-8 form when present.
  const utf8 = /filename\*\s*=\s*[^']*''([^;]+)/i.exec(disposition)
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1])
    } catch {
      return utf8[1]
    }
  }
  const ascii = /filename\s*=\s*"?([^";]+)"?/i.exec(disposition)
  return ascii?.[1]?.trim() ?? ''
}
