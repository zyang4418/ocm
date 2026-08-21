import { useCallback, useEffect, useState } from 'react'
import { apiFetch } from '../auth/api'
import type { DashboardSummary } from '../types/api'

// useDashboardSummary fetches the console homepage payload
// (GET /api/dashboard/summary?date=<local today>). The backend prunes sections
// the caller lacks permission for, so `data` field presence drives module
// visibility on the page. reload() refetches (retry button after an error).
export default function useDashboardSummary(token: string | null | undefined) {
  const today = new Date()
  const date = [
    today.getFullYear(),
    String(today.getMonth() + 1).padStart(2, '0'),
    String(today.getDate()).padStart(2, '0'),
  ].join('-')

  const [data, setData] = useState<DashboardSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    apiFetch<DashboardSummary>(`/api/dashboard/summary?date=${date}`, { token })
      .then((d) => {
        if (!cancelled) setData(d)
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, date, reloadKey])

  const reload = useCallback(() => setReloadKey((k) => k + 1), [])

  return { date, data, loading, error, reload }
}
