/**
 * Backend transport configuration.
 *
 * The same plain HTTP/JSON backend is reached two ways:
 *
 * transport = 'callContainer': route requests through wx.cloud.callContainer
 *   (requires the backend deployed as a WeChat Cloud Run service in the same
 *   cloud environment). cloudEnv + serviceName come from the Cloud Run console.
 *   Used in production.
 *
 * transport = 'http': call baseUrl directly via wx.request, for local dev /
 *   self-hosted / non-cloud deployments. Add the domain to the WeChat
 *   "request 合法域名" whitelist in that case (or, for localhost in the
 *   Developer Tools, enable 详情 -> 本地设置 -> 不校验合法域名).
 *
 * Selection is automatic by envVersion:
 *   develop (微信开发者工具/本地)      -> http   + baseUrl (localhost)
 *   trial   (体验版, real device)      -> callContainer
 *   release (正式版, published)        -> callContainer
 *
 * trial/release both use callContainer because they run on real devices, where
 * localhost is unreachable and a public domain would need extra whitelisting;
 * the cloud path is what you ship, so test it in 体验版.
 *
 * Switching hosting models is config-only: identity always goes through
 * wx.login + code2Session (never the cloud-gateway header), and request.ts
 * reads only `apiConfig`. No code changes required to flip transports.
 */

export type Transport = 'callContainer' | 'http'

export interface ApiConfig {
  transport: Transport
  /** Cloud Run environment id (callContainer only). */
  cloudEnv: string
  /** Cloud Run service name, sent as X-WX-SERVICE (callContainer only). */
  serviceName: string
  /** Base URL of the backend (http only). No trailing slash. */
  baseUrl: string
  /**
   * Base URL for streaming endpoints (http only). wx.cloud.callContainer
   * cannot stream SSE responses, so AI chat always uses wx.request against a
   * direct HTTP(S) URL. Empty in http mode (falls back to baseUrl). For
   * trial/release with the callContainer transport, set this to the
   * backend's public HTTPS origin (the backend is public anyway for the web
   * console) and add it to the request 合法域名 whitelist.
   */
  streamBaseUrl: string
  /** Request timeout in ms. callContainer caps this at 15000. */
  timeout: number
}

// ---- Production: WeChat Cloud Run (callContainer) ----
const prodConfig: ApiConfig = {
  transport: 'callContainer',
  cloudEnv: 'prod-xxxxxxxxxxxx', // TODO: replace with your Cloud Run env id
  serviceName: 'ocm', // TODO: replace with your Cloud Run service name
  baseUrl: '', // unused in callContainer mode
  streamBaseUrl: '', // TODO: public HTTPS origin for AI streaming (e.g. https://api.example.com)
  timeout: 15000,
}

// ---- Local dev: direct HTTP to the backend on your machine ----
// In the Developer Tools this is localhost. To debug on a real phone over Wi-Fi,
// swap localhost for your dev machine's LAN IP (e.g. http://192.168.1.10:8080)
// and keep the phone on the same network.
const devConfig: ApiConfig = {
  transport: 'http',
  cloudEnv: '', // unused
  serviceName: '', // unused
  baseUrl: 'http://localhost:8080', // match backend PORT (default 8080)
  streamBaseUrl: '', // empty = use baseUrl for streaming too
  timeout: 15000,
}

/**
 * Force a specific transport regardless of envVersion.
 * - undefined: auto-select by envVersion (normal use).
 * - 'http' / 'callContainer': override everywhere (e.g. test the cloud path
 *   from the Developer Tools without publishing a trial build).
 * Flip this and recompile; no other code needs to change.
 */
const FORCE_TRANSPORT: Transport | undefined = undefined

function resolveConfig(): ApiConfig {
  if (FORCE_TRANSPORT === 'http') return devConfig
  if (FORCE_TRANSPORT === 'callContainer') return prodConfig

  let envVersion = 'release'
  try {
    envVersion = wx.getAccountInfoSync().miniProgram.envVersion
  } catch {
    // Rare: very old base library without getAccountInfoSync. Fall back to the
    // production config so a published build never accidentally hits localhost.
  }
  return envVersion === 'develop' ? devConfig : prodConfig
}

export const apiConfig: ApiConfig = resolveConfig()

/**
 * Base URL for streaming requests (SSE). callContainer buffers responses and
 * cannot stream, so streaming always goes through wx.request directly.
 * Throws when no streaming URL is configured in callContainer mode.
 */
export function streamBaseUrl(): string {
  if (apiConfig.transport === 'http') return apiConfig.baseUrl
  if (apiConfig.streamBaseUrl) return apiConfig.streamBaseUrl
  throw new Error('AI 对话需配置流式服务地址')
}
