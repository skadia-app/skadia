import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { fetchActivities } from '@/api/activities'
import type { Activity } from '@/api/types'
import { useToast } from '@/lib/toast'
import { formatDuration, formatDistance } from '@/lib/utils'
import { useAccentColor } from '@/lib/accentColor'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'

interface MergeDialogProps {
  activity: Activity
  onClose: () => void
}

const MERGEABLE_FIELDS = [
  { id: 'hr', label: 'Herzfrequenz' },
  { id: 'power', label: 'Leistung' },
  { id: 'cadence', label: 'Kadenz' },
  { id: 'temperature', label: 'Temperatur' },
  { id: 'title', label: 'Titel' },
  { id: 'tags', label: 'Tags' },
  { id: 'gear', label: 'Ausrustung' },
] as const

type FieldID = typeof MERGEABLE_FIELDS[number]['id']

export default function MergeDialog({ activity, onClose }: MergeDialogProps) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { accent } = useAccentColor()

  const [search, setSearch] = useState('')
  const [selectedSourceId, setSelectedSourceId] = useState<string | null>(null)
  const [selectedFields, setSelectedFields] = useState<Set<FieldID>>(new Set(['hr', 'power', 'cadence', 'temperature']))

  // Fetch recent activities as candidates
  const { data: activitiesData } = useQuery({
    queryKey: ['activities-merge-candidates', search],
    queryFn: () => fetchActivities(undefined, 50, 'started_at', 'desc', search ? { search } : undefined),
    staleTime: 30_000,
  })

  // Exclude the current activity from candidates
  const candidates = (activitiesData?.activities ?? []).filter(a => a.id !== activity.id)

  const toggleField = (field: FieldID) => {
    setSelectedFields(prev => {
      const next = new Set(prev)
      if (next.has(field)) {
        next.delete(field)
      } else {
        next.add(field)
      }
      return next
    })
  }

  const mutation = useMutation({
    mutationFn: () =>
      apiClient<Activity>(`/activities/${activity.id}/merge`, {
        method: 'POST',
        body: {
          source_id: selectedSourceId,
          fields: [...selectedFields],
        },
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
      addToast({ type: 'success', title: 'Aktivitaten fusioniert', message: 'Ausgewahlte Felder wurden ubernommen.' })
      onClose()
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Fusion fehlgeschlagen', message: String(err), duration: 0 })
    },
  })

  const isValid = selectedSourceId !== null && selectedFields.size > 0

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={onClose}
      role="presentation"
    >
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" />
      <div
        className="relative bg-white dark:bg-neutral-950 rounded-lg shadow-lg p-6 max-w-md w-[90vw] max-h-[90vh] overflow-auto"
        onClick={e => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="merge-dialog-title"
      >
        <h2 id="merge-dialog-title" className="text-lg font-semibold text-neutral-900 dark:text-neutral-50">
          Aktivitaten fusionieren
        </h2>
        <p className="mt-1 text-xs text-neutral-400 dark:text-neutral-500">
          Felder aus einer Quell-Aktivitat in diese Aktivitat ubernehmen.
        </p>

        {/* Source activity search */}
        <div className="mt-4">
          <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
            Quell-Aktivitat suchen
          </label>
          <input
            type="text"
            placeholder="Titel suchen…"
            value={search}
            onChange={e => {
              setSearch(e.target.value)
              setSelectedSourceId(null)
            }}
            className="w-full px-3 py-2 border border-neutral-300 rounded-md dark:bg-neutral-900 dark:border-neutral-700 dark:text-neutral-50 focus:outline-none focus:ring-2 focus:ring-offset-0 text-sm"
            style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
          />
        </div>

        {/* Activity list */}
        <div className="mt-2 max-h-44 overflow-y-auto rounded-md border border-neutral-200 dark:border-neutral-700 divide-y divide-neutral-100 dark:divide-neutral-800">
          {candidates.length === 0 && (
            <p className="p-3 text-sm text-neutral-400 dark:text-neutral-500 text-center">Keine Aktivitaten gefunden</p>
          )}
          {candidates.map(a => (
            <button
              key={a.id}
              onClick={() => setSelectedSourceId(a.id)}
              className={`w-full text-left px-3 py-2 text-sm transition-colors ${
                selectedSourceId === a.id
                  ? 'text-white'
                  : 'text-neutral-900 dark:text-neutral-50 hover:bg-neutral-50 dark:hover:bg-neutral-800'
              }`}
              style={selectedSourceId === a.id ? { backgroundColor: accent.shade500 } : undefined}
            >
              <div className="font-medium truncate">{a.title ?? a.sport_type}</div>
              <div className={`text-xs mt-0.5 ${selectedSourceId === a.id ? 'text-white/70' : 'text-neutral-400 dark:text-neutral-500'}`}>
                {format(new Date(a.started_at), 'dd.MM.yyyy', { locale: de })}
                {a.elapsed_time_s ? ` · ${formatDuration(a.elapsed_time_s)}` : ''}
                {a.distance_m ? ` · ${formatDistance(a.distance_m)}` : ''}
              </div>
            </button>
          ))}
        </div>

        {/* Fields to merge */}
        <div className="mt-4">
          <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
            Zu ubernehmende Felder
          </p>
          <div className="grid grid-cols-2 gap-2">
            {MERGEABLE_FIELDS.map(field => (
              <label key={field.id} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={selectedFields.has(field.id)}
                  onChange={() => toggleField(field.id)}
                  className="rounded"
                  style={{ accentColor: accent.shade500 }}
                />
                <span className="text-sm text-neutral-700 dark:text-neutral-300">{field.label}</span>
              </label>
            ))}
          </div>
        </div>

        {/* Warning */}
        {selectedSourceId && (
          <p className="mt-3 text-xs text-amber-600 dark:text-amber-400">
            Die Quell-Aktivitat wird als Fusionsquelle markiert und aus der normalen Liste ausgeblendet.
          </p>
        )}

        <div className="mt-6 flex gap-2 justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-md text-sm font-medium bg-neutral-200 hover:bg-neutral-300 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-900 dark:text-neutral-50"
          >
            Abbrechen
          </button>
          <button
            onClick={() => mutation.mutate()}
            disabled={!isValid || mutation.isPending}
            className="px-4 py-2 rounded-md text-sm font-medium text-white disabled:opacity-50 disabled:cursor-not-allowed hover:opacity-90"
            style={{ backgroundColor: accent.shade500 }}
          >
            {mutation.isPending ? 'Wird fusioniert…' : 'Fusionieren'}
          </button>
        </div>
      </div>
    </div>
  )
}
