import { useEffect, useMemo, useRef, useState } from 'react'
import { Check, X } from 'lucide-react'
import { confirmChatToolCall } from '@/api/chat'
import { cn } from '@/lib/utils'

export interface ToolConfirmRequest {
  toolCallID: string
  tool: string
  args?: unknown
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
  const confirmButtonRef = useRef<HTMLButtonElement>(null)
  const [now, setNow] = useState(() => Date.now())
  const [busy, setBusy] = useState(false)
  const [announcement, setAnnouncement] = useState('')
  const [localStatus, setLocalStatus] = useState<ToolConfirmRequest['status']>()
  const expiresAt = useMemo(() => new Date(request.expiresAtISO).getTime(), [request.expiresAtISO])
  const effectiveStatus = request.status && request.status !== 'pending' ? request.status : (localStatus ?? request.status)
  const remainingMs = Math.max(0, expiresAt - now)
  const remainingSeconds = Math.ceil(remainingMs / 1000)
  const expired = remainingMs <= 0 || effectiveStatus === 'expired'
  const disabled = busy || expired || effectiveStatus === 'confirmed' || effectiveStatus === 'declined'

  useEffect(() => {
    setLocalStatus(undefined)
  }, [request.toolCallID])

  useEffect(() => {
    confirmButtonRef.current?.focus()
  }, [])

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 250)
    return () => window.clearInterval(id)
  }, [])

  useEffect(() => {
    if (remainingSeconds === 60 || remainingSeconds === 30 || remainingSeconds === 10) {
      setAnnouncement(`Noch ${remainingSeconds} Sekunden`)
    }
  }, [remainingSeconds])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (disabled) return
      if (event.key === 'Enter') {
        event.preventDefault()
        void submit(true)
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        void submit(false)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  async function submit(confirmed: boolean) {
    if (disabled) return
    setBusy(true)
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
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      role="alertdialog"
      aria-labelledby={`confirm-${request.toolCallID}`}
      className={cn(
        'my-2 rounded-lg border p-3 text-sm',
        disabled ? 'border-neutral-300 bg-neutral-50 text-neutral-500 dark:border-neutral-700 dark:bg-neutral-900' : 'border-amber-300 bg-amber-50 text-neutral-900 dark:border-amber-700 dark:bg-amber-950/30 dark:text-neutral-100',
      )}
    >
      <div id={`confirm-${request.toolCallID}`} className="font-medium">
        {request.summary}
      </div>
      <div className="mt-1 font-mono text-[11px] text-neutral-500">{request.tool}</div>
      {effectiveStatus === 'confirmed' && <div className="mt-2 text-xs text-emerald-700 dark:text-emerald-300">Bestätigt - wird ausgeführt...</div>}
      {effectiveStatus === 'declined' && <div className="mt-2 text-xs text-neutral-500">Abgelehnt</div>}
      <div className="mt-3 flex items-center gap-2">
        <button
          ref={confirmButtonRef}
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
