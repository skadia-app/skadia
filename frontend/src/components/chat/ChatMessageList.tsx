import { useEffect, useRef } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'
import ToolCallCard from './ToolCallCard'

export interface StreamMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  toolCalls?: Array<{ tool: string; args?: string; result?: string }>
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
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, thinking])

  if (messages.length === 0 && !thinking) {
    return (
      <div className="flex-1 flex items-center justify-center text-neutral-400 text-sm">
        Stell mir eine Frage zu deinen Trainingsdaten
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
            {msg.toolCalls?.map((tc, i) => (
              <ToolCallCard key={i} toolName={tc.tool} args={tc.args} result={tc.result} />
            ))}
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
                      <span>{used.toLocaleString('de-DE')}/{ctx.toLocaleString('de-DE')} tok ({pct}%)</span>
                      {msg.hops != null && msg.hops > 0 && <span>&middot; {msg.hops} {msg.hops === 1 ? 'hop' : 'hops'}</span>}
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
