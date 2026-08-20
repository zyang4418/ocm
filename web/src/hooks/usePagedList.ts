import { useCallback, useEffect, useRef, useState } from 'react'
import { apiFetch } from '../auth/api'
import type { Paged } from '../types/api'

export interface UsePagedListOptions {
  path: string
  token?: string | null
  extraParams?: Record<string, string | number | boolean | null | undefined>
  initialPageSize?: number
  debounceMs?: number
}

export interface PagedList<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
  q: string
  loading: boolean
  error: string
  setPage: (page: number) => void
  setPageSize: (pageSize: number) => void
  setQ: (q: string) => void
  reload: () => void
}

// usePagedList drives a paginated, server-searched list endpoint. It owns the
// page/pageSize/q state, debounces the search input, unwraps the backend's
// {items, total, page, pageSize} envelope and refetches when any of them
// change. Callers get a reload() for post-mutation refresh (create/update/
// delete, imports polling) - it refetches the current page.
//
// extraParams holds fixed filter params (e.g. classroom_id/status/from/to on
// the bookings page); empty-string/null values are omitted from the query.
export default function usePagedList<T = unknown>({
  path,
  token,
  extraParams = {},
  initialPageSize = 100,
  debounceMs = 300,
}: UsePagedListOptions): PagedList<T> {
  const [items, setItems] = useState<T[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(initialPageSize)
  const [q, setQ] = useState('')
  const [debouncedQ, setDebouncedQ] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reloadKey, setReloadKey] = useState(0)

  // extraParams arrives as a fresh object literal every render; only its
  // serialized content should trigger a refetch. The effect reads the latest
  // value through a ref and depends on the serialized key instead.
  const paramsRef = useRef(extraParams)
  const paramsKey = JSON.stringify(extraParams)
  useEffect(() => {
    paramsRef.current = extraParams
  })

  // Latest items, so the fetch effect can decide whether to show the loading
  // state without listing items in its deps.
  const itemsRef = useRef(items)
  useEffect(() => {
    itemsRef.current = items
  })

  // A new filter set starts a new result set: jump back to page 1. Declared
  // before the fetch effect so the reset is queued first (the transient fetch
  // with the stale page is discarded by the cancelled flag below).
  const prevKeyRef = useRef(paramsKey)
  useEffect(() => {
    if (prevKeyRef.current !== paramsKey) {
      prevKeyRef.current = paramsKey
      setPage(1)
    }
  }, [paramsKey])

  // Debounce the search input into debouncedQ (the value requests use).
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQ(q), debounceMs)
    return () => clearTimeout(t)
  }, [q, debounceMs])

  useEffect(() => {
    let cancelled = false
    const params = new URLSearchParams()
    params.set('page', String(page))
    params.set('page_size', String(pageSize))
    if (debouncedQ) params.set('q', debouncedQ)
    for (const [k, v] of Object.entries(paramsRef.current)) {
      if (v !== '' && v !== null && v !== undefined) params.set(k, String(v))
    }
    // Show the loading row only when there is nothing to display yet; refetches
    // (polling, post-mutation reloads) keep the previous page visible instead
    // of flashing a spinner.
    if (itemsRef.current.length === 0) setLoading(true)
    setError('')
    apiFetch<Paged<T>>(`${path}?${params.toString()}`, { token })
      .then((data) => {
        if (cancelled) return
        const rows = Array.isArray(data?.items) ? data.items : []
        const count = typeof data?.total === 'number' ? data.total : 0
        setItems(rows)
        setTotal(count)
        // The page fell past the last page (e.g. its last row was deleted):
        // step back one page instead of showing an empty one.
        if (page > 1 && rows.length === 0 && count > 0) {
          setPage(page - 1)
        }
      })
      .catch((err: Error) => {
        if (cancelled) return
        setError(err.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [path, token, page, pageSize, debouncedQ, paramsKey, reloadKey])

  // Changing the search term or page size starts a new result set: page 1.
  const changeQ = useCallback((v: string) => {
    setQ(v)
    setPage(1)
  }, [])

  const changePageSize = useCallback((size: number) => {
    setPageSize(size)
    setPage(1)
  }, [])

  const reload = useCallback(() => setReloadKey((k) => k + 1), [])

  return {
    items,
    total,
    page,
    pageSize,
    q,
    loading,
    error,
    setPage,
    setPageSize: changePageSize,
    setQ: changeQ,
    reload,
  }
}
