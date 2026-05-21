import { useState } from 'react'
import { Send } from 'lucide-react'
import { createTargetReport, type CreateTargetReportRequest, type OverlaySubjectKind, type TargetReportType } from '@/api/targetOverlays'

const REPORT_TYPES: Array<{ value: TargetReportType; label: string }> = [
  { value: 'condition', label: 'Zustand' },
  { value: 'issue', label: 'Problem' },
  { value: 'warning', label: 'Warnung' },
  { value: 'trail_work', label: 'Arbeit' },
  { value: 'closure', label: 'Sperrung' },
  { value: 'metadata', label: 'Daten' },
]

type Props = {
  subjectKind: OverlaySubjectKind
  subjectId: string
  targetVersionId?: string
  disabled?: boolean
  onCreated?: () => void
}

export default function ReportComposer({ subjectKind, subjectId, targetVersionId, disabled, onCreated }: Props) {
  const [reportType, setReportType] = useState<TargetReportType>('condition')
  const [description, setDescription] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async () => {
    const text = description.trim()
    if (!text || busy || disabled) return
    setBusy(true)
    setError(null)
    try {
      const request: CreateTargetReportRequest = {
        subject_kind: subjectKind,
        subject_id: subjectId,
        target_version_id: targetVersionId,
        report_type: reportType,
        visibility: 'public',
        attribution_mode: 'named',
        description: text,
        observed_at: new Date().toISOString(),
      }
      await createTargetReport(request)
      setDescription('')
      onCreated?.()
    } catch {
      setError('Meldung konnte nicht gespeichert werden.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-2 rounded-md border border-neutral-200 p-3 dark:border-neutral-800">
      <div className="flex flex-col gap-2 sm:flex-row">
        <select
          value={reportType}
          onChange={event => setReportType(event.target.value as TargetReportType)}
          disabled={disabled || busy}
          className="rounded-md border border-neutral-300 bg-white px-2 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-950"
        >
          {REPORT_TYPES.map(type => <option key={type.value} value={type.value}>{type.label}</option>)}
        </select>
        <textarea
          value={description}
          onChange={event => setDescription(event.target.value)}
          disabled={disabled || busy}
          rows={2}
          className="min-h-10 flex-1 rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-950"
          placeholder="Meldung eingeben"
        />
        <button
          type="button"
          onClick={submit}
          disabled={disabled || busy || !description.trim()}
          className="inline-flex items-center justify-center gap-1.5 rounded-md border border-neutral-300 px-3 py-2 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50 dark:border-neutral-700"
        >
          <Send size={14} />
          Senden
        </button>
      </div>
      {error && <p className="text-xs text-red-500">{error}</p>}
    </div>
  )
}
