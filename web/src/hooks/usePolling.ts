import { useCallback, useEffect, useRef, useState } from 'react'

// Polls `fetcher` immediately and then every `intervalMs`, exposing a
// manual `refresh` for after a mutation. Used by every list page so the
// dashboard stays live without a bespoke WebSocket layer for Phase 0.
export function usePolling<T>(fetcher: () => Promise<T>, intervalMs = 5000) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher

  const refresh = useCallback(() => {
    fetcherRef.current()
      .then((res) => {
        setData(res)
        setError(null)
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load'))
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, intervalMs)
    return () => clearInterval(id)
  }, [refresh, intervalMs])

  return { data, error, refresh }
}
