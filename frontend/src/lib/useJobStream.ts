import { useEffect, useRef, useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getToken, refreshToken } from '@/lib/auth'
import { updateModule, clearCompleted, clearAll } from '@/lib/useEnrichmentProgress'
import { useTabLeader } from '@/lib/useTabLeader'

interface BroadcastEvent {
  event: string
  data: unknown
}

/** Max consecutive auth failures before giving up SSE reconnects. */
const MAX_AUTH_RETRIES = 3

/** Jittered reconnect delay to prevent thundering herd on server deploy. */
function reconnectDelay() {
  return 3000 + Math.random() * 4000 // 3-7 seconds
}

/**
 * Process a parsed SSE event — shared by leader (from SSE) and follower (from broadcast).
 *
 * Note: Both init AND progress events are broadcast to follower tabs.
 * Followers receive enrichment progress updates in real-time via this path.
 */
function handleEvent(
  eventType: string,
  data: any,
  qc: ReturnType<typeof useQueryClient>,
) {
  if (eventType === 'init') {
    // Backend omits pending_enrichments/unread_notifications on query failure —
    // in that case, keep the previous cached value instead of resetting to 0.
    const prev = qc.getQueryData<{ pending_enrichments?: number }>(['active-jobs'])
    qc.setQueryData(['active-jobs'], {
      jobs: data.jobs ?? [],
      pending_enrichments: data.pending_enrichments ?? prev?.pending_enrichments ?? 0,
    })
    if (data.unread_notifications !== undefined) {
      qc.setQueryData(['notifications', 'unread-count'], {
        unread_count: data.unread_notifications,
      })
    }
  } else if (eventType === 'progress') {
    if (data.status === 'done') {
      qc.invalidateQueries({ queryKey: ['activity', data.activity_id] })
    }
    if (data.stage && data.activity_id) {
      updateModule(data.activity_id, data.stage, data.status)
    }
  }
}

/**
 * SSE-Stream for job progress with cross-tab coordination.
 *
 * Only the leader tab holds the SSE connection and broadcasts ALL events
 * (both `init` and `progress`) to follower tabs via BroadcastChannel.
 *
 * @param userId - Current user ID (used to scope the BroadcastChannel)
 */
export function useJobStream(userId: string) {
  const qc = useQueryClient()
  const { isLeader, broadcast, onMessage } = useTabLeader(`skadia-events-${userId}`)
  const abortRef = useRef<AbortController | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const activeRef = useRef(false)
  const authRetriesRef = useRef(0)

  // Cleanup completed enrichment entries periodically
  useEffect(() => {
    const timer = setInterval(() => clearCompleted(), 15_000)
    return () => clearInterval(timer)
  }, [])

  // Clear enrichment progress on user change (logout → login as different user)
  useEffect(() => {
    clearAll()
  }, [userId])

  // Follower path: receive broadcast events from leader
  useEffect(() => {
    if (isLeader) return
    return onMessage((payload) => {
      const { event, data } = payload as BroadcastEvent
      handleEvent(event, data, qc)
    })
  }, [isLeader, onMessage, qc])

  // Leader path: SSE connection + broadcast
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
        // Handle auth failure: refresh token and retry (up to MAX_AUTH_RETRIES)
        if (res.status === 401) {
          authRetriesRef.current++
          if (authRetriesRef.current > MAX_AUTH_RETRIES) {
            activeRef.current = false // Stop reconnecting — user needs to re-login
            return
          }
          await refreshToken().catch(() => {})
          return // falls through to finally → reconnect with fresh token
        }

        // Server rejected: too many SSE connections for this user.
        // Another tab is likely the leader — stop reconnecting.
        if (res.status === 429) {
          activeRef.current = false
          return
        }

        if (!res.ok || !res.body) throw new Error(`SSE failed: ${res.status}`)

        // Successful connection — reset auth retry counter
        authRetriesRef.current = 0

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
              try {
                const data = JSON.parse(line.slice(6))
                handleEvent(currentEvent, data, qc)
                broadcast({ event: currentEvent, data })
              } catch {
                // Non-JSON lines (keepalives) — ignore
              }
              currentEvent = ''
            } else if (line === '' || line.startsWith(':')) {
              currentEvent = ''
            }
          }
        }
      })
      .catch(() => {
        // Error or abort — reconnected below
      })
      .finally(() => {
        abortRef.current = null
        if (!activeRef.current) return // Stopped by cleanup or auth failure — don't reconnect
        reconnectTimer.current = setTimeout(() => {
          reconnectTimer.current = null
          connect()
        }, reconnectDelay())
      })
  }, [qc, broadcast])

  useEffect(() => {
    if (!isLeader) return

    activeRef.current = true
    authRetriesRef.current = 0

    // Brief stabilization delay: if two tabs race to become leader, the UUID
    // tiebreaker resolves within ~1 heartbeat (1s). This 500ms delay ensures
    // the losing tab yields BEFORE we open an SSE connection, preventing a
    // brief window of dual SSE connections.
    const stabilize = setTimeout(() => {
      if (activeRef.current) connect()
    }, 500)

    return () => {
      activeRef.current = false
      clearTimeout(stabilize)
      abortRef.current?.abort()
      abortRef.current = null
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current)
        reconnectTimer.current = null
      }
    }
  }, [isLeader, connect])
}
