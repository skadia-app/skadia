import { useEffect, useRef, useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getToken } from '@/lib/auth'
import { updateModule, clearCompleted } from '@/lib/useEnrichmentProgress'

/**
 * SSE-Stream für Job-Fortschritt.
 *
 * Nutzt fetch() statt EventSource, damit der Authorization-Header
 * mitgeschickt werden kann (EventSource unterstützt keine custom Header).
 */
export function useJobStream() {
  const qc = useQueryClient()
  const abortRef = useRef<AbortController | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Abgelaufene Enrichment-Einträge alle 15 Sekunden aufräumen
  useEffect(() => {
    const timer = setInterval(() => clearCompleted(), 15_000)
    return () => clearInterval(timer)
  }, [])

  const connect = useCallback(() => {
    if (abortRef.current) return

    const token = getToken()
    if (!token) return

    const ctrl = new AbortController()
    abortRef.current = ctrl

    fetch('/api/v1/jobs/stream', {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then(async (res) => {
        if (!res.ok || !res.body) {
          throw new Error(`SSE failed: ${res.status}`)
        }

        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        let currentEvent = ''

        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() ?? ''

          for (const line of lines) {
            if (line.startsWith('event: ')) {
              currentEvent = line.slice(7).trim()
            } else if (line.startsWith('data: ')) {
              const dataStr = line.slice(6)
              try {
                const data = JSON.parse(dataStr)

                if (currentEvent === 'init') {
                  qc.setQueryData(['active-jobs'], { jobs: data, pending_enrichments: 0 })
                  // Also invalidate to ensure polling picks up any changes that
                  // happened between SSE reconnects (staleTime could prevent refetch)
                  qc.invalidateQueries({ queryKey: ['active-jobs'] })
                } else if (currentEvent === 'progress') {
                  qc.invalidateQueries({ queryKey: ['active-jobs'] })
                  if (data.status === 'done') {
                    qc.invalidateQueries({ queryKey: ['activity', data.activity_id] })
                  }
                  if (data.stage && data.activity_id) {
                    updateModule(data.activity_id, data.stage, data.status)
                  }
                }
              } catch {
                // Nicht-JSON Zeilen ignorieren (z.B. Keepalives)
              }
              currentEvent = ''
            } else if (line === '' || line.startsWith(':')) {
              // Leerzeile (Event-Grenze) oder Kommentar/Keepalive — ignorieren
              currentEvent = ''
            }
          }
        }
      })
      .catch(() => {
        // Fehler oder Abbruch — wird unten reconnected
      })
      .finally(() => {
        abortRef.current = null
        // Reconnect nach 5 Sekunden
        reconnectTimer.current = setTimeout(() => {
          reconnectTimer.current = null
          connect()
        }, 5000)
      })
  }, [qc])

  useEffect(() => {
    connect()
    return () => {
      abortRef.current?.abort()
      abortRef.current = null
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current)
        reconnectTimer.current = null
      }
    }
  }, [connect])
}
