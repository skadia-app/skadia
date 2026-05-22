import { useState } from 'react'
import { MessageSquare, Send, Star } from 'lucide-react'
import { createTargetUserContent, type OverlaySubjectKind, type OverlayVisibility, type TargetUserContent } from '@/api/targetOverlays'
import VisibilityAccessField from '@/components/target-objects/VisibilityAccessField'

type Props = {
  subjectKind: OverlaySubjectKind
  subjectId: string
  targetVersionId?: string
  content: TargetUserContent[]
  loading?: boolean
  canWrite?: boolean
  onChanged?: () => void
}

export default function UserContentPanel({ subjectKind, subjectId, targetVersionId, content, loading, canWrite, onChanged }: Props) {
  const [text, setText] = useState('')
  const [visibility, setVisibility] = useState<OverlayVisibility>('private')
  const [accessScopeId, setAccessScopeId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const comments = content.filter(row => row.content_kind === 'comment')
  const ratings = content.filter(row => row.content_kind === 'rating')

  const submitComment = async () => {
    const value = text.trim()
    if (!value || busy || !canWrite) return
    setBusy(true)
    setError(null)
    try {
      await createTargetUserContent({
        subject_kind: subjectKind,
        subject_id: subjectId,
        target_version_id: targetVersionId,
        content_kind: 'comment',
        visibility,
        access_scope_id: visibility === 'shared' ? accessScopeId ?? undefined : undefined,
        payload: { text: value },
      })
      setText('')
      onChanged?.()
    } catch {
      setError('Kommentar konnte nicht gespeichert werden.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-4">
      {canWrite && (
        <div className="space-y-2 rounded-md border border-neutral-200 p-3 dark:border-neutral-800">
          <div className="flex gap-2">
            <input
              value={text}
              onChange={event => setText(event.target.value)}
              disabled={busy}
              className="min-w-0 flex-1 rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-950"
              placeholder="Kommentar"
            />
            <button
              type="button"
              onClick={submitComment}
              disabled={busy || !text.trim() || (visibility === 'shared' && !accessScopeId)}
              className="inline-flex items-center justify-center gap-1.5 rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
            >
              <Send size={14} />
              Senden
            </button>
          </div>
          <VisibilityAccessField
            visibility={visibility}
            accessScopeId={accessScopeId}
            onChange={(next) => {
              setVisibility(next.visibility as OverlayVisibility)
              setAccessScopeId(next.access_scope_id ?? null)
            }}
          />
          {error && <p className="text-xs text-red-500">{error}</p>}
        </div>
      )}

      {ratings.length > 0 && (
        <div className="flex flex-wrap gap-2 text-xs text-neutral-500 dark:text-neutral-400">
          <span className="inline-flex items-center gap-1 rounded-md bg-neutral-100 px-2 py-1 dark:bg-neutral-800">
            <Star size={13} />
            {ratings.length} Bewertungen
          </span>
        </div>
      )}

      {loading ? (
        <p className="py-4 text-sm text-neutral-500 dark:text-neutral-400">Kommentare werden geladen...</p>
      ) : comments.length === 0 ? (
        <p className="py-4 text-sm text-neutral-500 dark:text-neutral-400">Noch keine Kommentare.</p>
      ) : (
        <div className="divide-y divide-neutral-200 dark:divide-neutral-800">
          {comments.map(item => (
            <div key={item.id} className="flex items-start gap-3 py-3">
              <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
                <MessageSquare size={16} />
              </span>
              <div className="min-w-0">
                <p className="whitespace-pre-wrap break-words text-sm text-neutral-900 dark:text-neutral-100">
                  {String(item.payload?.text ?? '')}
                </p>
                <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">{formatDateTime(item.created_at)}</p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('de-DE', { dateStyle: 'medium', timeStyle: 'short' })
}
