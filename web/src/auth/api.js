export async function apiFetch(path, { method = 'GET', body, token } = {}) {
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (token) headers.Authorization = `Bearer ${token}`

  let res
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new Error('无法连接到服务器，请稍后重试')
  }

  const data = await res.json().catch(() => null)
  if (!res.ok) {
    const err = new Error(data?.error || `请求失败（${res.status}）`)
    err.status = res.status
    throw err
  }
  return data
}

// apiUpload posts a file as multipart/form-data. Unlike apiFetch it must NOT
// set Content-Type so the browser can attach the multipart boundary. The
// optional `fields` object adds extra text form parts alongside the file (used
// by the jwc_split endpoint, which sends semester + week1_monday with the file).
export async function apiUpload(path, { file, token, fields = {} } = {}) {
  const form = new FormData()
  form.append('file', file)
  for (const [k, v] of Object.entries(fields)) {
    form.append(k, v)
  }
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`

  let res
  try {
    res = await fetch(path, { method: 'POST', headers, body: form })
  } catch {
    throw new Error('无法连接到服务器，请稍后重试')
  }

  const data = await res.json().catch(() => null)
  if (!res.ok) {
    const err = new Error(data?.error || `上传失败（${res.status}）`)
    err.status = res.status
    throw err
  }
  return data
}

// apiDownload fetches a binary file (xlsx export) as a blob and triggers a
// browser download. The filename is read from the Content-Disposition header
// (decoded for UTF-8 / quoted forms); fallbackName is used when the header is
// absent or unparseable. Export endpoints return the file directly (not JSON),
// so this does not parse the body as JSON.
export async function apiDownload(path, { token, fallbackName = 'export.xlsx' } = {}) {
  const headers = {}
  if (token) headers.Authorization = `Bearer ${token}`

  let res
  try {
    res = await fetch(path, { method: 'GET', headers })
  } catch {
    throw new Error('无法连接到服务器，请稍后重试')
  }

  if (!res.ok) {
    // Export errors come back as JSON {error}, so try to read it.
    const data = await res.json().catch(() => null)
    const err = new Error(data?.error || `导出失败（${res.status}）`)
    err.status = res.status
    throw err
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
// frame. Non-2xx responses are read as JSON and thrown like apiFetch. Returns
// { promise, controller } — abort the controller to stop the stream mid-way.
export function apiStream(path, { body, token, onEvent }) {
  const controller = new AbortController()
  const promise = (async () => {
    const headers = { Accept: 'text/event-stream' }
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    if (token) headers.Authorization = `Bearer ${token}`

    let res
    try {
      res = await fetch(path, {
        method: 'POST',
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      })
    } catch (err) {
      if (err.name === 'AbortError') throw err
      throw new Error('无法连接到服务器，请稍后重试')
    }
    if (!res.ok) {
      const data = await res.json().catch(() => null)
      const err = new Error(data?.error || `请求失败（${res.status}）`)
      err.status = res.status
      throw err
    }

    const reader = res.body.getReader()
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
        let parsed = null
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
function parseFilename(disposition) {
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
