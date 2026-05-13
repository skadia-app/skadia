import { type KeyboardEvent, useEffect, useMemo, useState } from 'react'
import { Check, X } from 'lucide-react'
import { confirmChatToolCall } from '@/api/chat'
import { cn } from '@/lib/utils'

export interface ToolConfirmRequest {
  toolCallID: string
  tool: string
  args?: unknown
  preview?: unknown
  summary: string
  expiresAtISO: string
  sessionID: string
  status?: 'pending' | 'confirmed' | 'declined' | 'expired'
  reason?: string
}

interface ChatToolConfirmProps {
  request: ToolConfirmRequest
  onConfirmed: () => void
  onDeclined: (reason: string) => void
}

export default function ChatToolConfirm({ request, onConfirmed, onDeclined }: ChatToolConfirmProps) {
  const [now, setNow] = useState(() => Date.now())
  const [busy, setBusy] = useState(false)
  const [announcement, setAnnouncement] = useState('')
  const [error, setError] = useState('')
  const [localStatus, setLocalStatus] = useState<ToolConfirmRequest['status']>()
  const expiresAt = useMemo(() => new Date(request.expiresAtISO).getTime(), [request.expiresAtISO])
  const effectiveStatus = request.status && request.status !== 'pending' ? request.status : (localStatus ?? request.status)
  const remainingMs = Math.max(0, expiresAt - now)
  const remainingSeconds = Math.ceil(remainingMs / 1000)
  const expired = remainingMs <= 0 || effectiveStatus === 'expired'
  const disabled = busy || expired || effectiveStatus === 'confirmed' || effectiveStatus === 'declined'
  const previewText = useMemo(() => formatPreview(request.preview), [request.preview])

  useEffect(() => {
    setLocalStatus(undefined)
    setError('')
  }, [request.toolCallID])

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 250)
    return () => window.clearInterval(id)
  }, [])

  useEffect(() => {
    if (remainingSeconds === 60 || remainingSeconds === 30 || remainingSeconds === 10) {
      setAnnouncement(`Noch ${remainingSeconds} Sekunden`)
    }
  }, [remainingSeconds])

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== 'Escape' || disabled) return
    event.preventDefault()
    event.stopPropagation()
    void submit(false)
  }

  async function submit(confirmed: boolean) {
    if (disabled) return
    setBusy(true)
    setError('')
    try {
      await confirmChatToolCall(request.sessionID, {
        tool_call_id: request.toolCallID,
        confirmed,
      })
      if (confirmed) {
        setLocalStatus('confirmed')
        onConfirmed()
      } else {
        setLocalStatus('declined')
        onDeclined('user')
      }
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      role="alertdialog"
      aria-labelledby={`confirm-${request.toolCallID}`}
      onKeyDown={handleKeyDown}
      className={cn(
        'my-2 rounded-lg border p-3 text-sm',
        disabled ? 'border-neutral-300 bg-neutral-50 text-neutral-500 dark:border-neutral-700 dark:bg-neutral-900' : 'border-amber-300 bg-amber-50 text-neutral-900 dark:border-amber-700 dark:bg-amber-950/30 dark:text-neutral-100',
      )}
    >
      <div id={`confirm-${request.toolCallID}`} className="font-medium">
        {request.summary}
      </div>
      <div className="mt-1 font-mono text-[11px] text-neutral-500">{request.tool}</div>
      {previewText && (
        <pre className="mt-2 max-h-48 overflow-auto rounded-md border border-amber-200 bg-white/80 p-2 text-[11px] leading-4 text-neutral-700 dark:border-amber-900/70 dark:bg-neutral-950/60 dark:text-neutral-200">
          {previewText}
        </pre>
      )}
      {error && (
        <div role="alert" className="mt-2 rounded-md border border-red-200 bg-red-50 px-2 py-1 text-xs text-red-700 dark:border-red-900/70 dark:bg-red-950/30 dark:text-red-200">
          {error}
        </div>
      )}
      {effectiveStatus === 'confirmed' && <div className="mt-2 text-xs text-emerald-700 dark:text-emerald-300">Bestätigt - wird ausgeführt...</div>}
      {effectiveStatus === 'declined' && <div className="mt-2 text-xs text-neutral-500">Abgelehnt</div>}
      <div className="mt-3 flex items-center gap-2">
        <button
          type="button"
          disabled={disabled}
          onClick={() => void submit(true)}
          className="inline-flex h-8 items-center gap-1 rounded-md bg-emerald-600 px-3 text-xs font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Check className="h-3.5 w-3.5" />
          Bestätigen
        </button>
        <button
          type="button"
          disabled={disabled}
          onClick={() => void submit(false)}
          className="inline-flex h-8 items-center gap-1 rounded-md border border-neutral-300 px-3 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
        >
          <X className="h-3.5 w-3.5" />
          Abbrechen
        </button>
        <span className="ml-auto text-xs text-neutral-500">
          {expired ? 'Bestätigung abgelaufen' : `${remainingSeconds}s`}
        </span>
      </div>
      <div aria-live="polite" className="sr-only">{announcement}</div>
    </div>
  )
}

function formatPreview(preview: unknown): string {
  if (preview == null) return ''
  if (typeof preview === 'string') return preview
  try {
    return JSON.stringify(preview, null, 2)
  } catch {
    return ''
  }
}

function errorMessage(err: unknown): string {
  if (typeof err === 'string' && err.trim()) return err
  const maybeStructured = err as { json?: Record<string, unknown>; body?: string } | null
  const jsonMessage = maybeStructured?.json?.message
  if (typeof jsonMessage === 'string' && jsonMessage.trim()) return jsonMessage
  const jsonError = maybeStructured?.json?.error
  if (typeof jsonError === 'string' && jsonError.trim()) return jsonError

  if (typeof maybeStructured?.body === 'string' && maybeStructured.body.trim()) {
    try {
      const parsed = JSON.parse(maybeStructured.body) as unknown
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const message = (parsed as Record<string, unknown>).message
        if (typeof message === 'string' && message.trim()) return message
      }
    } catch {
      return maybeStructured.body
    }
    return maybeStructured.body
  }

  if (err instanceof Error && err.message.trim()) return err.message
  return 'Bestätigung konnte nicht gesendet werden'
}
