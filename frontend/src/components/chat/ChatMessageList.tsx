import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'
import ToolCallCard from './ToolCallCard'
import ChatToolConfirm, { type ToolConfirmRequest } from './ChatToolConfirm'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { localeForLanguage } from '@/i18n/languages'

export interface LoadedToolsDiagnostics {
  names: string[]
  reason?: string
  intent?: string
  expansionLevel?: number
  omittedTools?: Array<{ name: string; reason?: string }>
}

export interface StreamMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  loadedTools?: LoadedToolsDiagnostics
  toolCalls?: Array<{ id?: string; tool: string; args?: string; result?: string }>
  confirmRequest?: ToolConfirmRequest
  isStreaming?: boolean
  usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
  hops?: number
  contextWindow?: number
}

interface ChatMessageListProps {
  messages: StreamMessage[]
  thinking?: string | null
}

export default function ChatMessageList({ messages, thinking }: ChatMessageListProps) {
  const { t, i18n } = useTranslation()
  const locale = localeForLanguage(supportedLanguageFrom(i18n.language))
  const bottomRef = useRef<HTMLDivElement>(null)
  const [expandedToolDetails, setExpandedToolDetails] = useState<Set<string>>(() => new Set())

  useEffect(() => {
    if (typeof bottomRef.current?.scrollIntoView === 'function') {
      bottomRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages, thinking])

  if (messages.length === 0 && !thinking) {
    return (
      <div className="flex-1 flex items-center justify-center text-neutral-400 text-sm">
        {t('chatPage.messages.empty')}
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto p-4 space-y-4">
      {messages.map(msg => (
        <div key={msg.id} className={cn('flex', msg.role === 'user' ? 'justify-end' : 'justify-start')}>
          <div
            className={cn(
              'max-w-[80%] rounded-xl px-4 py-2 text-sm',
              msg.role === 'user'
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100',
            )}
          >
            {msg.loadedTools && msg.loadedTools.names.length > 0 && (() => {
              const detailsId = `tool-details-${msg.id}`
              const expanded = expandedToolDetails.has(msg.id)
              return (
                <div className="mb-1 rounded border border-neutral-200 dark:border-neutral-700 text-xs text-neutral-500 dark:text-neutral-400">
                  <button
                    type="button"
                    aria-expanded={expanded}
                    aria-controls={detailsId}
                    onClick={() => {
                      setExpandedToolDetails(prev => {
                        const next = new Set(prev)
                        if (next.has(msg.id)) {
                          next.delete(msg.id)
                        } else {
                          next.add(msg.id)
                        }
                        return next
                      })
                    }}
                    className="flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left font-medium hover:bg-neutral-50 dark:hover:bg-neutral-700/40"
                  >
                    <span>{t('chatPage.messages.toolsLoaded', { count: msg.loadedTools.names.length })}</span>
                    <span aria-hidden="true" className="text-neutral-400">{expanded ? '-' : '+'}</span>
                  </button>
                  {expanded && (
                    <div id={detailsId} className="space-y-1 border-t border-neutral-200 dark:border-neutral-700 px-3 py-2">
                      <div className="font-mono text-[11px] leading-relaxed break-words">
                        {msg.loadedTools.names.join(', ')}
                      </div>
                      {(msg.loadedTools.intent || msg.loadedTools.reason || msg.loadedTools.expansionLevel != null) && (
                        <dl className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-[11px]">
                          {msg.loadedTools.intent && (
                            <>
                              <dt className="text-neutral-400">{t('chatPage.messages.intent')}</dt>
                              <dd className="font-mono break-words">{msg.loadedTools.intent}</dd>
                            </>
                          )}
                          {msg.loadedTools.reason && (
                            <>
                              <dt className="text-neutral-400">{t('chatPage.messages.reason')}</dt>
                              <dd className="font-mono break-words">{msg.loadedTools.reason}</dd>
                            </>
                          )}
                          {msg.loadedTools.expansionLevel != null && (
                            <>
                              <dt className="text-neutral-400">{t('chatPage.messages.level')}</dt>
                              <dd className="font-mono">{msg.loadedTools.expansionLevel}</dd>
                            </>
                          )}
                        </dl>
                      )}
                      {msg.loadedTools.omittedTools && msg.loadedTools.omittedTools.length > 0 && (
                        <div className="space-y-0.5 text-[11px]">
                          <div className="text-neutral-400">{t('chatPage.messages.omitted')}</div>
                          {msg.loadedTools.omittedTools.map(tool => (
                            <div key={`${tool.name}:${tool.reason ?? ''}`} className="font-mono break-words">
                              {tool.name}{tool.reason ? ` - ${tool.reason}` : ''}
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )
            })()}
            {msg.toolCalls?.map((tc, i) => (
              <ToolCallCard key={tc.id ?? i} toolName={tc.tool} args={tc.args} result={tc.result} />
            ))}
            {msg.confirmRequest && (
              <ChatToolConfirm
                request={msg.confirmRequest}
                onConfirmed={() => undefined}
                onDeclined={() => undefined}
              />
            )}
            {msg.role === 'assistant' ? (
              <>
                <div className="prose prose-sm dark:prose-invert max-w-none">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{msg.content}</ReactMarkdown>
                  {msg.isStreaming && <span className="inline-block w-1 h-4 bg-current ml-0.5 animate-pulse align-bottom" />}
                </div>
                {!msg.isStreaming && msg.usage && (() => {
                  const ctx = msg.contextWindow ?? 32768
                  const used = msg.usage.prompt_tokens + msg.usage.completion_tokens
                  const pct = Math.min(100, Math.round((used / ctx) * 100))
                  return (
                    <div className="mt-1 pt-1 border-t border-neutral-200 dark:border-neutral-700 text-[10px] text-neutral-400 dark:text-neutral-500 font-mono tabular-nums flex items-center gap-2">
                      <span>{t('chatPage.messages.tokenUsage', { used: used.toLocaleString(locale), total: ctx.toLocaleString(locale), percent: pct })}</span>
                      {msg.hops != null && msg.hops > 0 && (
                        <span>&middot; {t('chatPage.messages.hops', { count: msg.hops })}</span>
                      )}
                      <div className="flex-1 h-1 rounded-full bg-neutral-200 dark:bg-neutral-700 max-w-[80px]">
                        <div
                          className={cn('h-1 rounded-full', pct > 80 ? 'bg-red-400' : pct > 50 ? 'bg-yellow-400' : 'bg-accent-DEFAULT')}
                          style={{ width: `${pct}%` }}
                        />
                      </div>
                    </div>
                  )
                })()}
              </>
            ) : (
              <div className="whitespace-pre-wrap">
                {msg.content}
              </div>
            )}
          </div>
        </div>
      ))}
      {thinking && (
        <div className="flex justify-start">
          <div className="bg-neutral-100 dark:bg-neutral-800 rounded-xl px-4 py-2 text-sm text-neutral-500 dark:text-neutral-400 italic">
            {thinking}…
          </div>
        </div>
      )}
      <div ref={bottomRef} />
    </div>
  )
}
