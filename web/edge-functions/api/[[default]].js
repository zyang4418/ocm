// EdgeOne Pages Edge Function: reverse-proxy /api/* to the backend API.
// The frontend calls the API with same-origin relative paths (e.g.
// /api/auth/login); this forwards them to the backend so no CORS is needed.
// The backend origin is read from the API_ORIGIN env var (EdgeOne project
// settings); if it is unset the function fails fast with 502.
export async function onRequest(context) {
  const request = context.request
  const origin = context.env?.API_ORIGIN
  if (!origin) {
    return new Response(JSON.stringify({ error: '后端地址未配置（API_ORIGIN 未设置）' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  const url = new URL(request.url)
  // Use new URL to normalize a trailing slash in API_ORIGIN; plain string
  // concat would produce a double slash that triggers ServeMux path-cleaning
  // 301s and, with redirect: "manual", an infinite redirect loop.
  const target = new URL(url.pathname + url.search, origin).href

  const headers = new Headers(request.headers)
  headers.delete('host') // let the upstream use the target URL's host

  const init = {
    method: request.method,
    headers,
    redirect: 'manual',
  }
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    init.body = request.body
  }

  try {
    return await fetch(target, init)
  } catch {
    return new Response(JSON.stringify({ error: '无法连接到后端服务' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json' },
    })
  }
}
