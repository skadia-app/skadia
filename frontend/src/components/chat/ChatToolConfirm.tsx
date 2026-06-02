import { type KeyboardEvent, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Check, X } from 'lucide-react'
import { confirmChatToolCall } from '@/api/chat'
import { cn } from '@/lib/utils'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

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
  const { t } = useTranslation()
  const { formatDistance } = useUnitFormatters()
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
  const previewText = useMemo(() => formatPreview(request.preview, t, formatDistance), [formatDistance, request.preview, t])

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
      setAnnouncement(t('chatPage.confirm.remainingAnnouncement', { count: remainingSeconds }))
    }
  }, [remainingSeconds, t])

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
      setError(errorMessage(err, t))
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
      {effectiveStatus === 'confirmed' && <div className="mt-2 text-xs text-emerald-700 dark:text-emerald-300">{t('chatPage.confirm.confirmedRunning')}</div>}
      {effectiveStatus === 'declined' && <div className="mt-2 text-xs text-neutral-500">{t('chatPage.confirm.declined')}</div>}
      <div className="mt-3 flex items-center gap-2">
        <button
          type="button"
          disabled={disabled}
          onClick={() => void submit(true)}
          className="inline-flex h-8 items-center gap-1 rounded-md bg-emerald-600 px-3 text-xs font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Check className="h-3.5 w-3.5" />
          {t('chatPage.confirm.confirm')}
        </button>
        <button
          type="button"
          disabled={disabled}
          onClick={() => void submit(false)}
          className="inline-flex h-8 items-center gap-1 rounded-md border border-neutral-300 px-3 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
        >
          <X className="h-3.5 w-3.5" />
          {t('chatPage.confirm.cancel')}
        </button>
        <span className="ml-auto text-xs text-neutral-500">
          {expired ? t('chatPage.confirm.expired') : t('chatPage.confirm.remainingShort', { count: remainingSeconds })}
        </span>
      </div>
      <div aria-live="polite" className="sr-only">{announcement}</div>
    </div>
  )
}

function formatPreview(
  preview: unknown,
  t: TFunction,
  formatDistance: (meters: number | null | undefined) => string,
): string {
  if (preview == null) return ''
  if (typeof preview === 'string') return preview
  if (isRecord(preview)) {
    const lines: string[] = []
    const kind = textValue(preview.kind)
    const count = preview.activity_count ?? preview.count ?? preview.affected_count
    if (kind) lines.push(`${t('chatPage.confirm.preview.type')}: ${kind}`)
    if (typeof count === 'number') lines.push(`${t('chatPage.confirm.preview.affectedActivities')}: ${count}`)
    const tagName = textValue(preview.tag_name)
    if (tagName) lines.push(`${t('chatPage.confirm.preview.tag')}: ${tagName}`)
    const mode = textValue(preview.mode)
    if (mode) lines.push(`${t('chatPage.confirm.preview.action')}: ${formatMode(mode, t)}`)
    const gearName = textValue(preview.gear_name)
    if (gearName) lines.push(`${t('chatPage.confirm.preview.gear')}: ${gearName}`)
    const routeName = textValue(preview.name)
    if (routeName) lines.push(`${t('chatPage.confirm.preview.name')}: ${routeName}`)
    const profile = textValue(preview.profile)
    if (profile) lines.push(`${t('chatPage.confirm.preview.profile')}: ${profile}`)
    if (typeof preview.total_distance_m === 'number') {
      lines.push(`${t('chatPage.confirm.preview.distance')}: ${formatDistance(preview.total_distance_m)}`)
    }
    if (isRecord(preview.criteria)) {
      lines.push(`${t('chatPage.confirm.preview.selection')}:`)
      lines.push(formatObjectLines(preview.criteria, '  '))
    }
    if (Array.isArray(preview.sample_activities) && preview.sample_activities.length > 0) {
      lines.push(`${t('chatPage.confirm.preview.examples')}:`)
      lines.push(...preview.sample_activities.slice(0, 5).map(item => `  - ${formatActivitySample(item, t, formatDistance)}`))
    }
    if (Array.isArray(preview.sample_ids) && preview.sample_ids.length > 0) {
      lines.push(`${t('chatPage.confirm.preview.sampleIds')}: ${preview.sample_ids.slice(0, 5).join(', ')}`)
    }
    if (isRecord(preview.cap)) {
      lines.push(`${t('chatPage.confirm.preview.limits')}:`)
      lines.push(formatObjectLines(preview.cap, '  '))
    }
    if (lines.length > 0) return lines.join('\n')
  }
  try {
    return JSON.stringify(preview, null, 2)
  } catch {
    return ''
  }
}

function formatMode(mode: string, t: TFunction): string {
  if (mode === 'remove') return t('chatPage.confirm.preview.mode.remove')
  if (mode === 'add') return t('chatPage.confirm.preview.mode.add')
  return mode
}

function errorMessage(err: unknown, t: TFunction): string {
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
  return t('chatPage.confirm.sendFailed')
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function textValue(value: unknown): string {
  return typeof value === 'string' && value.trim() ? value : ''
}

function formatObjectLines(record: Record<string, unknown>, prefix: string): string {
  return Object.entries(record)
    .map(([key, value]) => `${prefix}${key}: ${formatPreviewValue(value)}`)
    .join('\n')
}

function formatPreviewValue(value: unknown): string {
  if (Array.isArray(value)) return value.join(', ')
  if (isRecord(value)) return JSON.stringify(value)
  if (value == null) return '-'
  return String(value)
}

function formatActivitySample(
  value: unknown,
  t: TFunction,
  formatDistance: (meters: number | null | undefined) => string,
): string {
  if (!isRecord(value)) return String(value)
  const title = textValue(value.title) || textValue(value.name) || textValue(value.id) || t('chatPage.confirm.preview.activityFallback')
  const startedAt = textValue(value.started_at) || textValue(value.date)
  const sportType = textValue(value.sport_type) || textValue(value.sport)
  const distance = typeof value.distance_km === 'number'
    ? formatDistance(value.distance_km * 1000)
    : typeof value.km === 'number'
      ? formatDistance(value.km * 1000)
    : typeof value.distance_m === 'number'
      ? formatDistance(value.distance_m)
      : ''
  return [title, startedAt, sportType, distance].filter(Boolean).join(' · ')
}
