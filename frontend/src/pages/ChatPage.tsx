import { useState, useEffect, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
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
import { getToken, refreshToken, clearToken, redirectToLogin } from '@/lib/auth'
import { API_BASE } from '@/api/client'
import ChatMessageList, { type StreamMessage } from '@/components/chat/ChatMessageList'
import ChatInput from '@/components/chat/ChatInput'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { localeForLanguage } from '@/i18n/languages'

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
  const { t, i18n } = useTranslation()
  const locale = localeForLanguage(supportedLanguageFrom(i18n.language))
  const { addToast } = useToast()

  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const [messages, setMessages] = useState<StreamMessage[]>([])
  const [thinking, setThinking] = useState<string | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)
  const [sessionsOpen, setSessionsOpen] = useState(false)
  const sessionDropdownRef = useRef<HTMLDivElement>(null)
  const activeSessionIdRef = useRef<string | null>(null)
  const streamAbortRef = useRef<AbortController | null>(null)
  const skipNextSessionLoadRef = useRef<string | null>(null)

  useEffect(() => {
    activeSessionIdRef.current = activeSessionId
  }, [activeSessionId])

  useEffect(() => {
    return () => {
      streamAbortRef.current?.abort()
    }
  }, [])

  // Load sessions on mount
  useEffect(() => {
    listSessions()
      .then(data => {
        const currentActiveSessionId = activeSessionIdRef.current
        setSessions(prev => {
          if (!currentActiveSessionId || data.some(session => session.id === currentActiveSessionId)) {
            return data
          }
          const activeSession = prev.find(session => session.id === currentActiveSessionId)
          return activeSession
            ? [activeSession, ...data.filter(session => session.id !== currentActiveSessionId)]
            : data
        })
        if (data.length > 0 && !currentActiveSessionId) {
          activeSessionIdRef.current = data[0].id
          setActiveSessionId(data[0].id)
        }
      })
      .catch(() => {
        addToast({ type: 'error', title: t('chatPage.toasts.sessionsLoadFailed'), duration: 0 })
      })
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Load messages when session changes
  useEffect(() => {
    if (!activeSessionId) {
      setMessages([])
      return
    }
    let cancelled = false
    const sessionId = activeSessionId
    if (skipNextSessionLoadRef.current === sessionId) {
      skipNextSessionLoadRef.current = null
      return
    }
    setMessages([])
    getMessages(activeSessionId)
      .then(data => {
        if (cancelled || activeSessionIdRef.current !== sessionId) return
        setMessages(
          data
            .filter(m => m.role === 'user' || m.role === 'assistant')
            .map(chatMessageToStream),
        )
      })
      .catch(() => {
        if (cancelled || activeSessionIdRef.current !== sessionId) return
        addToast({ type: 'error', title: t('chatPage.toasts.messagesLoadFailed'), duration: 0 })
      })
    return () => {
      cancelled = true
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId])

  // Close session dropdown on outside click
  useClickOutside(sessionDropdownRef, () => setSessionsOpen(false), sessionsOpen)

  const abortActiveStream = useCallback(() => {
    const controller = streamAbortRef.current
    if (!controller) return
    controller.abort()
    streamAbortRef.current = null
    setThinking(null)
    setIsStreaming(false)
    setMessages(prev =>
      prev.map(m =>
        m.isStreaming
          ? { ...m, isStreaming: false, content: m.content || t('chatPage.inline.abortedBracket') }
          : m.confirmRequest?.status === 'pending'
            ? {
                ...m,
                content: t('chatPage.inline.aborted'),
                confirmRequest: { ...m.confirmRequest, status: 'declined', reason: 'cancelled' },
              }
          : m,
      ),
    )
  }, [t])

  const handleNewSession = useCallback(async () => {
    abortActiveStream()
    try {
      const session = await createSession()
      setSessions(prev => [session, ...prev])
      activeSessionIdRef.current = session.id
      setActiveSessionId(session.id)
      setMessages([])
    } catch {
      addToast({ type: 'error', title: t('chatPage.toasts.newSessionFailed'), duration: 0 })
    }
  }, [abortActiveStream, addToast, t])

  const handleDeleteSession = useCallback(async (sessionId: string) => {
    if (sessionId === activeSessionIdRef.current) {
      abortActiveStream()
    }
    try {
      await deleteSession(sessionId)
      setSessions(prev => {
        const next = prev.filter(s => s.id !== sessionId)
        if (activeSessionId === sessionId) {
          const nextActiveSessionId = next.length > 0 ? next[0].id : null
          activeSessionIdRef.current = nextActiveSessionId
          setActiveSessionId(nextActiveSessionId)
          setMessages([])
        }
        return next
      })
    } catch {
      addToast({ type: 'error', title: t('chatPage.toasts.deleteSessionFailed'), duration: 0 })
    }
  }, [abortActiveStream, activeSessionId, addToast, t])

  const handleSend = useCallback(async (text: string) => {
    if (isStreaming) return

    let sessionId = activeSessionId

    // Auto-create session if none exists
    if (!sessionId) {
      try {
        const session = await createSession()
        setSessions(prev => [session, ...prev])
        activeSessionIdRef.current = session.id
        skipNextSessionLoadRef.current = session.id
        setActiveSessionId(session.id)
        sessionId = session.id
      } catch {
        addToast({ type: 'error', title: t('chatPage.toasts.createSessionFailed'), duration: 0 })
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
    const removeOptimisticUserMessage = () => {
      updateStreamMessages(prev => prev.filter(m => m.id !== userMsg.id))
    }

    // Placeholder for streaming assistant message
    const assistantId = generateId()
    const streamSessionId = sessionId
    const controller = new AbortController()
    streamAbortRef.current = controller
    let responseAccepted = false
    const updateStreamMessages = (updater: (messages: StreamMessage[]) => StreamMessage[]) => {
      if (activeSessionIdRef.current !== streamSessionId) return
      setMessages(prev => (activeSessionIdRef.current === streamSessionId ? updater(prev) : prev))
    }

    try {
      const token = getToken()
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (token) headers['Authorization'] = `Bearer ${token}`

      let response = await fetch(`${API_BASE}/chat/sessions/${sessionId}/send`, {
        method: 'POST',
        headers,
        credentials: 'include',
        signal: controller.signal,
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
            signal: controller.signal,
            body: JSON.stringify({ message_id: messageId, message: text }),
          })
        } else {
          removeOptimisticUserMessage()
          setIsStreaming(false)
          setThinking(null)
          clearToken()
          redirectToLogin()
          return
        }
      }

      if (!response.ok) {
        const errText = await response.text()
        if (response.status === 429) {
          addToast({ type: 'warning', title: t('chatPage.toasts.rateLimitTitle'), message: t('chatPage.toasts.rateLimitMessage'), duration: 0 })
        } else {
          addToast({ type: 'error', title: t('chatPage.toasts.sendFailed'), message: errText, duration: 0 })
        }
        removeOptimisticUserMessage()
        setIsStreaming(false)
        return
      }
      responseAccepted = true

      const reader = response.body?.getReader()
      if (!reader) {
        addToast({ type: 'error', title: t('chatPage.toasts.streamingUnavailable'), duration: 0 })
        removeOptimisticUserMessage()
        setIsStreaming(false)
        return
      }

      // Initialize streaming assistant message
      updateStreamMessages(prev => [
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

      // Tool call tracking: map tool_call_id → index in toolCalls array
      const toolCallIDSet = new Set<string>()
      const confirmMessageIDMap = new Map<string, string>()

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
            tools?: string[]
            tool_call_id?: string
            args?: unknown
            summary?: string
            preview?: unknown
            expires_at_iso?: string
            sources?: string[]
            error?: string
            question?: string
            reason?: string
            intent?: string
            omitted_tools?: Array<string | { name: string; reason?: string }>
            expansion_level?: number
            usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
            hops?: number
            context_window?: number
          }
          try {
            evt = JSON.parse(data)
          } catch {
            continue
          }
          if (activeSessionIdRef.current !== streamSessionId) {
            await reader.cancel()
            return
          }

          switch (evt.type) {
            case 'chat.thinking':
              setThinking(evt.step ?? t('chatPage.thinking.default'))
              break

            case 'chat.progress':
              setThinking(evt.delta ?? evt.step ?? t('chatPage.thinking.progress'))
              break

            case 'chat.tools_loaded':
              updateStreamMessages(prev =>
                prev.map(m =>
                  m.id === assistantId
                    ? {
                        ...m,
                        loadedTools: {
                          names: evt.tools ?? [],
                          reason: evt.reason,
                          intent: evt.intent,
                          expansionLevel: evt.expansion_level,
                          omittedTools: normalizeOmittedTools(evt.omitted_tools),
                        },
                      }
                    : m,
                ),
              )
              break

            case 'chat.chunk':
              setThinking(null)
              updateStreamMessages(prev =>
                prev.map(m =>
                  m.id === assistantId
                    ? { ...m, content: m.content + (evt.delta ?? '') }
                    : m,
                ),
              )
              break

            case 'chat.tool_call': {
              const toolName = evt.tool ?? 'unknown'
              const toolCallID = evt.tool_call_id ?? toolName
              const argsStr = evt.args != null ? JSON.stringify(evt.args, null, 2) : undefined
              updateStreamMessages(prev =>
                prev.map(m => {
                  if (m.id !== assistantId) return m
                  const existingCalls = m.toolCalls ?? []
                  if (toolCallIDSet.has(toolCallID)) {
                    return {
                      ...m,
                      toolCalls: existingCalls.map(tc =>
                        tc.id === toolCallID ? { ...tc, tool: toolName, args: argsStr ?? tc.args } : tc,
                      ),
                    }
                  }
                  toolCallIDSet.add(toolCallID)
                  return {
                    ...m,
                    toolCalls: [...existingCalls, { id: toolCallID, tool: toolName, args: argsStr }],
                  }
                }),
              )
              break
            }

            case 'chat.tool_result': {
              const toolName = evt.tool ?? 'unknown'
              const toolCallID = evt.tool_call_id ?? toolName
              const resultStr = evt.summary ?? (evt.args != null ? JSON.stringify(evt.args, null, 2) : undefined)
              updateStreamMessages(prev =>
                prev.map(m => {
                  if (m.id !== assistantId) return m
                  const calls = (m.toolCalls ?? []).map(tc =>
                    tc.id === toolCallID ? { ...tc, tool: toolName, result: resultStr } : tc,
                  )
                  return { ...m, toolCalls: calls }
                }),
              )
              break
            }

            case 'chat.tool_confirm_request': {
              const toolCallID = evt.tool_call_id ?? generateId()
              const confirmMessageID = generateId()
              updateStreamMessages(prev => {
                confirmMessageIDMap.set(toolCallID, confirmMessageID)
                return [
                  ...prev,
                  {
                    id: confirmMessageID,
                    role: 'assistant',
                    content: '',
                    confirmRequest: {
                      toolCallID,
                      tool: evt.tool ?? 'unknown',
                      args: evt.args,
                      preview: evt.preview,
                      summary: evt.summary ?? t('chatPage.inline.confirmAction'),
                      expiresAtISO: evt.expires_at_iso ?? new Date(Date.now() + 5 * 60_000).toISOString(),
                      sessionID: streamSessionId,
                      status: 'pending',
                    },
                  },
                ]
              })
              break
            }

            case 'chat.tool_confirmed':
            case 'chat.tool_declined': {
              const confirmMessageID = evt.tool_call_id ? confirmMessageIDMap.get(evt.tool_call_id) : undefined
              if (confirmMessageID !== undefined) {
                updateStreamMessages(prev => {
                  return prev.map(msg => {
                    if (msg.id !== confirmMessageID || !msg.confirmRequest) return msg
                    return {
                      ...msg,
                      content: evt.type === 'chat.tool_confirmed'
                        ? t('chatPage.inline.confirmedRunning')
                        : t('chatPage.inline.declinedWithReason', { reason: evt.reason ?? 'user' }),
                      confirmRequest: {
                        ...msg.confirmRequest,
                        status: evt.type === 'chat.tool_confirmed' ? 'confirmed' : 'declined',
                        reason: evt.reason,
                      },
                    }
                  })
                })
              }
              break
            }

            case 'chat.user_question':
              setThinking(null)
              updateStreamMessages(prev =>
                prev.map(m =>
                  m.id === assistantId
                    ? {
                        ...m,
                        content: evt.question ?? t('chatPage.inline.needsClarification'),
                        isStreaming: false,
                      }
                    : m,
                ),
              )
              setIsStreaming(false)
              await reader.cancel()
              return

            case 'chat.done':
              setThinking(null)
              updateStreamMessages(prev =>
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
              {
                const errorMessage = evt.error ?? t('chatPage.inline.unknownError')
                const inlineError = `${t('chatPage.inline.errorHeading')}\n\n${errorMessage}`
                setThinking(null)
                addToast({
                  type: 'error',
                  title: t('chatPage.toasts.assistantError'),
                  message: errorMessage,
                  duration: 0,
                })
                updateStreamMessages(prev =>
                  prev.map(m => {
                    if (m.id !== assistantId) return m
                    const content = m.content.trim()
                      ? `${m.content}\n\n${inlineError}`
                      : inlineError
                    return { ...m, isStreaming: false, content }
                  }),
                )
                setIsStreaming(false)
              }
              break
          }
        }
      }
      updateStreamMessages(prev =>
        prev.map(m =>
          m.id === assistantId && m.isStreaming
            ? { ...m, isStreaming: false, content: m.content || t('chatPage.inline.noAnswer') }
            : m,
        ),
      )
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return
      if (!responseAccepted) {
        removeOptimisticUserMessage()
      }
      addToast({
        type: 'error',
        title: t('chatPage.toasts.connectionError'),
        message: err instanceof Error ? err.message : t('chatPage.inline.unknownError'),
        duration: 0,
      })
      updateStreamMessages(prev =>
        prev.map(m =>
          m.id === assistantId
            ? { ...m, isStreaming: false, content: m.content || t('chatPage.inline.connectionError') }
            : m,
        ),
      )
    } finally {
      setThinking(null)
      setIsStreaming(false)
      if (streamAbortRef.current === controller) {
        streamAbortRef.current = null
      }
    }
  }, [activeSessionId, isStreaming, addToast, t])

  const activeSession = sessions.find(s => s.id === activeSessionId)
  const formatSessionTitle = (session: ChatSession) =>
    session.title ?? t('chatPage.session.fallback', {
      date: new Date(session.created_at).toLocaleDateString(locale),
    })

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
                ? formatSessionTitle(activeSession)
                : t('chatPage.session.none')}
            </span>
            <ChevronDown className="w-3.5 h-3.5 text-neutral-400 flex-shrink-0" />
          </button>

          {sessionsOpen && (
            <div className="absolute left-0 top-full mt-1 w-72 rounded-xl border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-50 max-h-64 overflow-y-auto">
              {sessions.length === 0 ? (
                <p className="px-3 py-2 text-sm text-neutral-400">{t('chatPage.session.empty')}</p>
              ) : (
                sessions.map(session => (
                  <div
                    key={session.id}
                    className={cn(
                      'flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors',
                      session.id === activeSessionId && 'bg-neutral-100 dark:bg-neutral-800',
                    )}
                    onClick={() => {
                      if (session.id !== activeSessionIdRef.current) {
                        abortActiveStream()
                      }
                      activeSessionIdRef.current = session.id
                      setActiveSessionId(session.id)
                      setSessionsOpen(false)
                    }}
                  >
                    <span className="flex-1 truncate text-sm text-neutral-700 dark:text-neutral-300">
                      {formatSessionTitle(session)}
                    </span>
                    <button
                      onClick={e => {
                        e.stopPropagation()
                        handleDeleteSession(session.id)
                      }}
                      className="p-1 rounded text-neutral-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-950/20 cursor-pointer transition-colors flex-shrink-0"
                      title={t('chatPage.session.delete')}
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
          {t('chatPage.session.new')}
        </button>
      </div>

      {/* Message list */}
      <ChatMessageList messages={messages} thinking={thinking} />

      {/* Input */}
      <ChatInput
        onSend={handleSend}
        onCancel={abortActiveStream}
        disabled={isStreaming}
        placeholder={isStreaming ? t('chatPage.input.assistantResponding') : t('chatPage.input.placeholder')}
      />
    </div>
  )
}

function normalizeOmittedTools(
  omittedTools: Array<string | { name: string; reason?: string }> | undefined,
): Array<{ name: string; reason?: string }> | undefined {
  if (!omittedTools || omittedTools.length === 0) return undefined
  return omittedTools
    .map(tool => (typeof tool === 'string' ? { name: tool } : tool))
    .filter(tool => tool.name.trim() !== '')
}
