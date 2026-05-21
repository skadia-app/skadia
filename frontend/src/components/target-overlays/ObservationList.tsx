import { Activity } from 'lucide-react'
import type { TargetActivityObservation } from '@/api/targetOverlays'

const OBSERVATION_LABELS: Record<string, string> = {
  matched_traversal: 'Durchfahrt',
  visit: 'Besuch',
  dwell: 'Aufenthalt',
  check_in: 'Check-in',
  completion: 'Abgeschlossen',
}

export default function ObservationList({ observations, loading }: { observations: TargetActivityObservation[]; loading?: boolean }) {
  if (loading) {
    return <p className="py-4 text-sm text-neutral-500 dark:text-neutral-400">Aktivität wird geladen...</p>
  }

  if (observations.length === 0) {
    return <p className="py-4 text-sm text-neutral-500 dark:text-neutral-400">Noch keine sichtbaren Aktivitäten.</p>
  }

  return (
    <div className="divide-y divide-neutral-200 dark:divide-neutral-800">
      {observations.map(observation => (
        <div key={observation.id} className="flex items-start gap-3 py-3">
          <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300">
            <Activity size={16} />
          </span>
          <div className="min-w-0">
            <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              {OBSERVATION_LABELS[observation.observation_type] ?? observation.observation_type}
            </p>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">{formatDateTime(observation.occurred_at)}</p>
          </div>
        </div>
      ))}
    </div>
  )
}

function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('de-DE', { dateStyle: 'medium', timeStyle: 'short' })
}
