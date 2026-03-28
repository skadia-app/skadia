import { useState, useEffect, useCallback, useRef } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { Plus, Trash2, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useToast } from '@/lib/toast'
import {
  listSessions,
  createSession,
  getMessages,
  deleteSession,
  type ChatSession,
  type ChatMessage,
} from '@/api/chat'
import { getToken, refreshToken, clearToken } from '@/lib/auth'
import { API_BASE } from '@/api/client'
import ChatMessageList, { type StreamMessage } from '@/components/chat/ChatMessageList'
import ChatInput from '@/components/chat/ChatInput'

function generateId(): string {
  return crypto.randomUUID()
}

function chatMessageToStream(msg: ChatMessage): StreamMessage {
  return {
    id: msg.id,
    role: msg.role === 'assistant' ? 'assistant' : 'user',
    content: msg.content,
  }
}

export default function ChatPage() {
  const { addToast } = useToast()

  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [messages, setMessages] = useState<StreamMessage[]>([])
  const [thinking, setThinking] = useState<string | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)
  const [sessionsOpen, setSessionsOpen] = useState(false)
  const sessionDropdownRef = useRef<HTMLDivElement>(null)

  // Load sessions on mount
  useEffect(() => {
    listSessions()
      .then(data => {
        setSessions(data)
        if (data.length > 0 && !activeSessionId) {
          setActiveSessionId(data[0].id)
        }
      })
      .catch(() => {
        addToast({ type: 'error', title: 'Sitzungen konnten nicht geladen werden', duration: 0 })
      })
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Load messages when session changes
  useEffect(() => {
    if (!activeSessionId) {
      setMessages([])
      return
    }
    getMessages(activeSessionId)
      .then(data => {
        setMessages(
          data
            .filter(m => m.role === 'user' || m.role === 'assistant')
            .map(chatMessageToStream),
        )
      })
      .catch(() => {
        addToast({ type: 'error', title: 'Nachrichten konnten nicht geladen werden', duration: 0 })
      })
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId])

  // Close session dropdown on outside click
  useClickOutside(sessionDropdownRef, () => setSessionsOpen(false), sessionsOpen)

  const handleNewSession = useCallback(async () => {
    try {
      const session = await createSession()
      setSessions(prev => [session, ...prev])
      setActiveSessionId(session.id)
      setMessages([])
    } catch {
      addToast({ type: 'error', title: 'Neue Sitzung konnte nicht erstellt werden', duration: 0 })
    }
  }, [addToast])

  const handleDeleteSession = useCallback(async (sessionId: string) => {
    try {
      await deleteSession(sessionId)
      setSessions(prev => {
        const next = prev.filter(s => s.id !== sessionId)
        if (activeSessionId === sessionId) {
          setActiveSessionId(next.length > 0 ? next[0].id : null)
          setMessages([])
        }
        return next
      })
    } catch {
      addToast({ type: 'error', title: 'Sitzung konnte nicht gelöscht werden', duration: 0 })
    }
  }, [activeSessionId, addToast])

  const handleSend = useCallback(async (text: string) => {
    if (isStreaming) return

    let sessionId = activeSessionId

    // Auto-create session if none exists
    if (!sessionId) {
      try {
        const session = await createSession()
        setSessions(prev => [session, ...prev])
        setActiveSessionId(session.id)
        sessionId = session.id
      } catch {
        addToast({ type: 'error', title: 'Sitzung konnte nicht erstellt werden', duration: 0 })
        return
      }
    }

    const messageId = generateId()
    const userMsg: StreamMessage = {
      id: generateId(),
      role: 'user',
      content: text,
    }
    setMessages(prev => [...prev, userMsg])
    setIsStreaming(true)
    setThinking(null)

    // Placeholder for streaming assistant message
    const assistantId = generateId()

    try {
      const token = getToken()
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (token) headers['Authorization'] = `Bearer ${token}`

      let response = await fetch(`${API_BASE}/chat/sessions/${sessionId}/send`, {
        method: 'POST',
        headers,
        credentials: 'include',
        body: JSON.stringify({ message_id: messageId, message: text }),
      })

      // Handle 401 with token refresh
      if (response.status === 401) {
        const refreshed = await refreshToken()
        if (refreshed) {
          const newToken = getToken()
          const retryHeaders: Record<string, string> = { 'Content-Type': 'application/json' }
          if (newToken) retryHeaders['Authorization'] = `Bearer ${newToken}`
          response = await fetch(`${API_BASE}/chat/sessions/${sessionId}/send`, {
            method: 'POST',
            headers: retryHeaders,
            credentials: 'include',
            body: JSON.stringify({ message_id: messageId, message: text }),
          })
        } else {
          clearToken()
          window.location.href = '/login'
          return
        }
      }

      if (!response.ok) {
        const errText = await response.text()
        if (response.status === 429) {
          addToast({ type: 'warning', title: 'Zu viele Anfragen', message: 'Bitte warte einen Moment (10/min Limit)', duration: 0 })
        } else {
          addToast({ type: 'error', title: 'Fehler beim Senden', message: errText, duration: 0 })
        }
        setIsStreaming(false)
        return
      }

      const reader = response.body?.getReader()
      if (!reader) {
        addToast({ type: 'error', title: 'Streaming nicht verfügbar', duration: 0 })
        setIsStreaming(false)
        return
      }

      // Initialize streaming assistant message
      setMessages(prev => [
        ...prev,
        {
          id: assistantId,
          role: 'assistant',
          content: '',
          toolCalls: [],
          isStreaming: true,
        },
      ])

      const decoder = new TextDecoder()
      let buffer = ''

      // Tool call tracking: map tool name → index in toolCalls array
      const toolCallIndexMap = new Map<string, number>()

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })

        // Process complete SSE lines
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6).trim()
          if (!data) continue

          let evt: {
            type: string
            id?: string
            delta?: string
            step?: string
            tool?: string
            args?: unknown
            summary?: string
            sources?: string[]
            error?: string
            usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
            hops?: number
            context_window?: number
          }
          try {
            evt = JSON.parse(data)
          } catch {
            continue
          }

          switch (evt.type) {
            case 'chat.thinking':
              setThinking(evt.step ?? 'Denke nach')
              break

            case 'chat.chunk':
              setThinking(null)
              setMessages(prev =>
                prev.map(m =>
                  m.id === assistantId
                    ? { ...m, content: m.content + (evt.delta ?? '') }
                    : m,
                ),
              )
              break

            case 'chat.tool_call': {
              const toolName = evt.tool ?? 'unknown'
              const argsStr = evt.args != null ? JSON.stringify(evt.args, null, 2) : undefined
              setMessages(prev =>
                prev.map(m => {
                  if (m.id !== assistantId) return m
                  const existingCalls = m.toolCalls ?? []
                  const idx = existingCalls.length
                  toolCallIndexMap.set(toolName, idx)
                  return {
                    ...m,
                    toolCalls: [...existingCalls, { tool: toolName, args: argsStr }],
                  }
                }),
              )
              break
            }

            case 'chat.tool_result': {
              const toolName = evt.tool ?? 'unknown'
              const resultStr = evt.summary ?? (evt.args != null ? JSON.stringify(evt.args, null, 2) : undefined)
              const idx = toolCallIndexMap.get(toolName)
              if (idx !== undefined) {
                setMessages(prev =>
                  prev.map(m => {
                    if (m.id !== assistantId) return m
                    const calls = [...(m.toolCalls ?? [])]
                    if (calls[idx]) {
                      calls[idx] = { ...calls[idx], result: resultStr }
                    }
                    return { ...m, toolCalls: calls }
                  }),
                )
              }
              break
            }

            case 'chat.done':
              setThinking(null)
              setMessages(prev =>
                prev.map(m =>
                  m.id === assistantId
                    ? {
                        ...m,
                        isStreaming: false,
                        ...(evt.usage ? { usage: evt.usage } : {}),
                        ...(evt.hops != null ? { hops: evt.hops } : {}),
                        ...(evt.context_window ? { contextWindow: evt.context_window } : {}),
                      }
                    : m,
                ),
              )
              setIsStreaming(false)
              break

            case 'chat.error':
              setThinking(null)
              addToast({
                type: 'error',
                title: 'Assistenten-Fehler',
                message: evt.error ?? 'Unbekannter Fehler',
                duration: 0,
              })
              setMessages(prev =>
                prev.map(m =>
                  m.id === assistantId
                    ? { ...m, isStreaming: false, content: m.content || '(Fehler)' }
                    : m,
                ),
              )
              setIsStreaming(false)
              break
          }
        }
      }
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Verbindungsfehler',
        message: err instanceof Error ? err.message : 'Unbekannter Fehler',
        duration: 0,
      })
      setMessages(prev =>
        prev.map(m =>
          m.id === assistantId
            ? { ...m, isStreaming: false, content: m.content || '(Verbindungsfehler)' }
            : m,
        ),
      )
    } finally {
      setThinking(null)
      setIsStreaming(false)
    }
  }, [activeSessionId, isStreaming, addToast])

  const activeSession = sessions.find(s => s.id === activeSessionId)

  return (
    <div className="flex flex-col h-[calc(100vh-3.5rem-3rem)] min-h-0">
      {/* Header: Session dropdown + New session button */}
      <div className="flex items-center gap-2 pb-3 border-b border-neutral-200 dark:border-neutral-800">
        <div ref={sessionDropdownRef} className="relative flex-1 min-w-0">
          <button
            onClick={() => setSessionsOpen(o => !o)}
            className="flex items-center gap-2 max-w-xs px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors truncate"
          >
            <span className="truncate text-neutral-700 dark:text-neutral-300">
              {activeSession
                ? activeSession.title ?? `Sitzung ${new Date(activeSession.created_at).toLocaleDateString('de-DE')}`
                : 'Keine Sitzung'}
            </span>
            <ChevronDown className="w-3.5 h-3.5 text-neutral-400 flex-shrink-0" />
          </button>

          {sessionsOpen && (
            <div className="absolute left-0 top-full mt-1 w-72 rounded-xl border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-50 max-h-64 overflow-y-auto">
              {sessions.length === 0 ? (
                <p className="px-3 py-2 text-sm text-neutral-400">Keine Sitzungen</p>
              ) : (
                sessions.map(session => (
                  <div
                    key={session.id}
                    className={cn(
                      'flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors',
                      session.id === activeSessionId && 'bg-neutral-100 dark:bg-neutral-800',
                    )}
                    onClick={() => {
                      setActiveSessionId(session.id)
                      setSessionsOpen(false)
                    }}
                  >
                    <span className="flex-1 truncate text-sm text-neutral-700 dark:text-neutral-300">
                      {session.title ?? `Sitzung ${new Date(session.created_at).toLocaleDateString('de-DE')}`}
                    </span>
                    <button
                      onClick={e => {
                        e.stopPropagation()
                        handleDeleteSession(session.id)
                      }}
                      className="p-1 rounded text-neutral-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-950/20 cursor-pointer transition-colors flex-shrink-0"
                      title="Sitzung löschen"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        <button
          onClick={handleNewSession}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors text-neutral-700 dark:text-neutral-300 flex-shrink-0"
        >
          <Plus className="w-3.5 h-3.5" />
          Neu
        </button>
      </div>

      {/* Message list */}
      <ChatMessageList messages={messages} thinking={thinking} />

      {/* Input */}
      <ChatInput
        onSend={handleSend}
        disabled={isStreaming}
        placeholder={isStreaming ? 'Assistent antwortet…' : 'Frag mich etwas…'}
      />
    </div>
  )
}
