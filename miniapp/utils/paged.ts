import { request } from './request'

/**
 * Server-paginated list state, a mini-program mirror of the web console's
 * usePagedList hook. Owns {items,total,page,pageSize,q,loading,error,hasMore}
 * and writes setData patches to a page data subtree (callers use
 * `setData: (p) => this.setData({ list: p })`). Pages keep the instance on
 * `this._list` and bind scrolltolower / search input / post-mutation reloads
 * to it.
 *
 * Behavior mirrored from web/src/hooks/usePagedList.js:
 * - load()/setQ() reset to page 1; q is debounced 300ms.
 * - loadMore() appends the next page (guarded by hasMore).
 * - reload() refetches the current page, keeping old rows visible (no
 *   spinner flash) — used after mutations and for imports polling.
 * - If a page > 1 comes back empty while total > 0 (its last row was
 *   deleted), step back one page automatically.
 * - Stale responses are dropped via a request sequence counter.
 */

export interface PagedState {
  items: any[]
  total: number
  page: number
  pageSize: number
  q: string
  loading: boolean
  error: string
  hasMore: boolean
}

export interface PagedOptions {
  path: string
  pageSize?: number
  /** Fixed filter params; read fresh on each fetch. Empty/null values omitted. */
  extraParams?: () => Record<string, string | number | undefined | null>
  /** setData patch for the page's list subtree. */
  setData: (patch: Record<string, any>) => void
  debounceMs?: number
}

export interface PagedList {
  state: PagedState
  /** First page / after filter change (replaces rows). */
  load: () => void
  /** Append the next page (scrolltolower). */
  loadMore: () => void
  /** Update the search term (debounced, resets to page 1). */
  setQ: (v: string) => void
  /** Refetch the current page, keeping old rows visible. */
  reload: () => void
}

export function createPagedList(opts: PagedOptions): PagedList {
  const { path, extraParams, setData } = opts
  const debounceMs = opts.debounceMs != null ? opts.debounceMs : 300

  const state: PagedState = {
    items: [],
    total: 0,
    page: 1,
    pageSize: opts.pageSize || 20,
    q: '',
    loading: true,
    error: '',
    hasMore: false,
  }

  let seq = 0
  let debounceTimer: any = null
  let mode: 'replace' | 'append' = 'replace'

  function pushState() {
    setData({
      items: state.items,
      total: state.total,
      page: state.page,
      q: state.q,
      loading: state.loading,
      error: state.error,
      hasMore: state.hasMore,
    })
  }

  async function fetchPage(): Promise<void> {
    const mySeq = ++seq
    const params: Record<string, string | number> = {
      page: state.page,
      page_size: state.pageSize,
    }
    if (state.q) params.q = state.q
    const extra = extraParams ? extraParams() : {}
    for (const key of Object.keys(extra)) {
      const v = extra[key]
      if (v === '' || v === undefined || v === null) continue
      params[key] = v
    }
    // Show the loading state only when there is nothing to display yet;
    // refetches (reload, loadMore) keep the previous rows visible.
    if (state.items.length === 0) state.loading = true
    state.error = ''
    pushState()
    try {
      const data = await request<{ items: any[]; total: number }>({ path, params })
      if (mySeq !== seq) return // stale response
      const rows = Array.isArray(data && data.items) ? data.items : []
      const count = typeof data && typeof data.total === 'number' ? data.total : 0
      state.items = mode === 'append' ? state.items.concat(rows) : rows
      state.total = count
      // The page fell past the last page (e.g. its last row was deleted):
      // step back one page instead of showing an empty one.
      if (state.page > 1 && rows.length === 0 && count > 0) {
        state.page -= 1
        mode = 'replace'
        return fetchPage()
      }
      state.hasMore = state.items.length < count
      pushState()
    } catch (err: any) {
      if (mySeq !== seq) return
      state.error = (err && err.message) || '加载失败'
      pushState()
    } finally {
      if (mySeq === seq) {
        state.loading = false
        pushState()
      }
    }
  }

  function load() {
    if (debounceTimer) clearTimeout(debounceTimer)
    seq++ // discard any in-flight response
    state.page = 1
    mode = 'replace'
    fetchPage()
  }

  function loadMore() {
    if (state.loading || !state.hasMore) return
    state.page += 1
    mode = 'append'
    fetchPage()
  }

  function setQ(v: string) {
    state.q = v
    state.page = 1
    if (debounceTimer) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => {
      debounceTimer = null
      mode = 'replace'
      fetchPage()
    }, debounceMs)
  }

  function reload() {
    mode = 'replace'
    fetchPage()
  }

  return { state, load, loadMore, setQ, reload }
}
