import { useEffect, useRef, useState, useCallback } from 'react'

const HEARTBEAT_MS = 1000
const TIMEOUT_MS = 3000

interface ChannelMessage {
  type: 'heartbeat' | 'event'
  tabId: string
  ts: number
  data?: unknown
}

/**
 * BroadcastChannel-based leader election for cross-tab coordination.
 *
 * Exactly one tab is the "leader" — it holds shared connections (SSE)
 * and broadcasts events to all follower tabs via the channel.
 *
 * @param channelName - Unique channel name (should include userId for multi-user isolation)
 */
export function useTabLeader(channelName: string) {
  const [isLeader, setIsLeader] = useState(false)
  const tabId = useRef(crypto.randomUUID()).current
  const channelRef = useRef<BroadcastChannel | null>(null)
  const heartbeatTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const lastHeartbeat = useRef(0)
  const leaderRef = useRef(false)
  const handlersRef = useRef(new Set<(data: unknown) => void>())

  // Keep ref in sync with state (for use in non-React callbacks)
  useEffect(() => {
    leaderRef.current = isLeader
  }, [isLeader])

  const broadcast = useCallback((data: unknown) => {
    channelRef.current?.postMessage({
      type: 'event',
      tabId,
      ts: Date.now(),
      data,
    } satisfies ChannelMessage)
  }, [tabId])

  const onMessage = useCallback((handler: (data: unknown) => void) => {
    handlersRef.current.add(handler)
    return () => { handlersRef.current.delete(handler) }
  }, [])

  useEffect(() => {
    // Graceful degradation: no BroadcastChannel → act as standalone leader
    if (!channelName || typeof BroadcastChannel === 'undefined') {
      leaderRef.current = true
      setIsLeader(true)
      return
    }

    const ch = new BroadcastChannel(channelName)
    channelRef.current = ch

    function becomeLeader() {
      if (leaderRef.current) return
      leaderRef.current = true
      setIsLeader(true)
      heartbeatTimer.current = setInterval(() => {
        ch.postMessage({
          type: 'heartbeat',
          tabId,
          ts: Date.now(),
        } satisfies ChannelMessage)
      }, HEARTBEAT_MS)
    }

    function yieldLeadership() {
      if (!leaderRef.current) return
      leaderRef.current = false
      setIsLeader(false)
      if (heartbeatTimer.current) {
        clearInterval(heartbeatTimer.current)
        heartbeatTimer.current = null
      }
    }

    ch.onmessage = (e: MessageEvent<ChannelMessage>) => {
      const msg = e.data
      // Validate message shape to reject corrupted or injected messages
      if (!msg || typeof msg !== 'object' || typeof msg.tabId !== 'string' || typeof msg.type !== 'string') return
      if (msg.tabId === tabId) return // Ignore own messages

      if (msg.type === 'heartbeat') {
        lastHeartbeat.current = msg.ts
        // Tiebreaker: lower UUID wins leadership
        if (leaderRef.current && msg.tabId < tabId) {
          yieldLeadership()
        }
      } else if (msg.type === 'event' && !leaderRef.current) {
        handlersRef.current.forEach(fn => fn(msg.data))
      }
    }

    // Claim leadership after random delay if no heartbeat received
    const claimDelay = Math.random() * 500 + 500 // 500-1000ms
    const claimTimer = setTimeout(() => {
      if (Date.now() - lastHeartbeat.current > TIMEOUT_MS) {
        becomeLeader()
      }
    }, claimDelay)

    // Watchdog: claim leadership if leader heartbeat stops
    const watchdog = setInterval(() => {
      if (!leaderRef.current && Date.now() - lastHeartbeat.current > TIMEOUT_MS) {
        becomeLeader()
      }
    }, TIMEOUT_MS)

    return () => {
      clearTimeout(claimTimer)
      clearInterval(watchdog)
      yieldLeadership()
      ch.close()
      channelRef.current = null
    }
  }, [channelName, tabId])

  return { isLeader, broadcast, onMessage }
}
