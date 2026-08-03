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
// set Content-Type so the browser can attach the multipart boundary.
export async function apiUpload(path, { file, token } = {}) {
  const form = new FormData()
  form.append('file', file)
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
