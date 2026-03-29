import { useState, useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { fetchActivities } from '@/api/activities'
import type { Activity } from '@/api/types'
import { useToast } from '@/lib/toast'
import { formatDuration, formatDistance } from '@/lib/utils'
import { useAccentColor } from '@/lib/accentColor'
import { fmtDateCompact } from '@/lib/format'
import { MergeComparisonTable } from '@/components/MergeComparisonTable'
import { ModalShell } from '@/components/ui/ModalShell'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'

interface MergeDialogProps {
  activity: Activity
  onClose: () => void
}

type FieldID = 'hr' | 'power' | 'cadence' | 'temperature' | 'gps' | 'altitude' | 'speed' | 'title' | 'tags' | 'gear'

const FIELD_MAP: Record<FieldID, keyof Activity> = {
  hr: 'avg_heart_rate',
  power: 'avg_power_w',
  cadence: 'avg_cadence',
  temperature: 'avg_temperature_c',
  gps: 'distance_m',
  altitude: 'elevation_gain_m',
  speed: 'avg_speed_ms',
  title: 'title',
  tags: 'tags',
  gear: 'gear_id',
}

const ALL_FIELD_KEYS: FieldID[] = ['hr', 'power', 'cadence', 'temperature', 'gps', 'altitude', 'speed', 'title', 'tags', 'gear']

function calculateOverlap(target: Activity, source: Activity): string {
  if (!target.started_at || !target.ended_at || !source.started_at || !source.ended_at) {
    return ''
  }
  const tStart = new Date(target.started_at).getTime()
  const tEnd = new Date(target.ended_at).getTime()
  const sStart = new Date(source.started_at).getTime()
  const sEnd = new Date(source.ended_at).getTime()

  const overlapStart = Math.max(tStart, sStart)
  const overlapEnd = Math.min(tEnd, sEnd)

  if (overlapStart >= overlapEnd) return 'Keine Zeitüberlappung'

  const overlapMs = overlapEnd - overlapStart
  const targetDuration = tEnd - tStart
  const pct = ((overlapMs / targetDuration) * 100).toFixed(1)
  const mins = Math.round(overlapMs / 60000)
  return `Zeitüberlappung: ${pct}% (${mins} min)`
}

export default function MergeDialog({ activity, onClose }: MergeDialogProps) {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { accent } = useAccentColor()

  const [search, setSearch] = useState('')
  const [selectedSourceId, setSelectedSourceId] = useState<string | null>(null)
  const [selectedFields, setSelectedFields] = useState<FieldID[]>(['hr', 'power', 'cadence', 'temperature'])

  // Fetch recent activities as candidates
  const { data: activitiesData } = useQuery({
    queryKey: ['activities-merge-candidates', search],
    queryFn: () => fetchActivities(undefined, 50, 'started_at', 'desc', search ? { search } : undefined),
    staleTime: 30_000,
  })

  // Exclude the current activity from candidates
  const candidates = (activitiesData?.activities ?? []).filter(a => a.id !== activity.id)

  const sourceActivity = selectedSourceId ? candidates.find(a => a.id === selectedSourceId) ?? null : null

  // Smart-defaults: auto-select fields where source has data and target does not
  useEffect(() => {
    if (!sourceActivity) return
    const autoSelect = ALL_FIELD_KEYS.filter(key => {
      const field = FIELD_MAP[key]
      const targetVal = activity[field]
      const sourceVal = sourceActivity[field]
      const hasTarget = targetVal !== null && targetVal !== undefined && targetVal !== '' && !(Array.isArray(targetVal) && targetVal.length === 0)
      const hasSource = sourceVal !== null && sourceVal !== undefined && sourceVal !== '' && !(Array.isArray(sourceVal) && sourceVal.length === 0)
      return !hasTarget && hasSource
    })
    setSelectedFields(autoSelect)
  }, [sourceActivity?.id])

  const toggleField = (field: string) => {
    setSelectedFields(prev => {
      const fid = field as FieldID
      if (prev.includes(fid)) {
        return prev.filter(f => f !== fid)
      } else {
        return [...prev, fid]
      }
    })
  }

  const mutation = useMutation({
    mutationFn: () =>
      apiClient<Activity>(`/activities/${activity.id}/merge`, {
        method: 'POST',
        body: {
          source_id: selectedSourceId,
          fields: selectedFields,
        },
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activity', activity.id] })
      queryClient.invalidateQueries({ queryKey: ['activity-streams', activity.id] })
      addToast({ type: 'success', title: 'Aktivitäten fusioniert', message: 'Ausgewählte Felder wurden übernommen.' })
      onClose()
    },
    onError: (err) => {
      addToast({ type: 'error', title: 'Fusion fehlgeschlagen', message: String(err), duration: 0 })
    },
  })

  const overlapInfo = sourceActivity ? calculateOverlap(activity, sourceActivity) : ''
  const isValid = selectedSourceId !== null && selectedFields.length > 0

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-4xl" className="bg-white dark:bg-neutral-950 rounded-lg shadow-lg w-[90vw] max-h-[90vh] overflow-auto">
        <h2 id="merge-dialog-title" className="text-lg font-semibold text-neutral-900 dark:text-neutral-50">
          Aktivitäten fusionieren
        </h2>
        <p className="mt-1 text-xs text-neutral-400 dark:text-neutral-500">
          Felder aus einer Quell-Aktivität in diese Aktivität übernehmen.
        </p>

        <div className={`mt-4 ${sourceActivity ? 'grid grid-cols-2 gap-6' : ''}`}>
          {/* Left column: source search */}
          <div>
            <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
              Quell-Aktivität suchen
            </label>
            <input
              type="text"
              placeholder="Titel suchen…"
              value={search}
              onChange={e => {
                setSearch(e.target.value)
                setSelectedSourceId(null)
              }}
              className={AUTH_INPUT_CLASS}
            />

            {/* Activity list */}
            <div className="mt-2 max-h-64 overflow-y-auto rounded-md border border-neutral-200 dark:border-neutral-700 divide-y divide-neutral-100 dark:divide-neutral-800">
              {candidates.length === 0 && (
                <p className="p-3 text-sm text-neutral-400 dark:text-neutral-500 text-center">Keine Aktivitäten gefunden</p>
              )}
              {candidates.map(a => (
                <button
                  key={a.id}
                  onClick={() => setSelectedSourceId(a.id)}
                  className={`cursor-pointer w-full text-left px-3 py-2 text-sm transition-colors ${
                    selectedSourceId === a.id
                      ? 'text-white'
                      : 'text-neutral-900 dark:text-neutral-50 hover:bg-neutral-50 dark:hover:bg-neutral-800'
                  }`}
                  style={selectedSourceId === a.id ? { backgroundColor: accent.shade500 } : undefined}
                >
                  <div className="font-medium truncate">{a.title ?? a.sport_type}</div>
                  <div className={`text-xs mt-0.5 ${selectedSourceId === a.id ? 'text-white/70' : 'text-neutral-400 dark:text-neutral-500'}`}>
                    {fmtDateCompact(a.started_at, a.timezone)}
                    {a.elapsed_time_s ? ` · ${formatDuration(a.elapsed_time_s)}` : ''}
                    {a.distance_m ? ` · ${formatDistance(a.distance_m)}` : ''}
                  </div>
                </button>
              ))}
            </div>

            {/* Overlap info */}
            {overlapInfo && (
              <p className={`mt-2 text-xs font-medium ${
                overlapInfo.startsWith('Keine')
                  ? 'text-amber-600 dark:text-amber-400'
                  : 'text-emerald-600 dark:text-emerald-400'
              }`}>
                {overlapInfo}
              </p>
            )}

            {/* Warning */}
            {selectedSourceId && (
              <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">
                Die Quell-Aktivität wird als Fusionsquelle markiert und aus der normalen Liste ausgeblendet.
              </p>
            )}
            {selectedFields.includes('gps') && (
              <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                GPS-Merge überschreibt die Koordinaten im Überlappungszeitraum.
                Track und Heatmap werden automatisch neu berechnet.
              </p>
            )}
          </div>

          {/* Right column: comparison table (only when a source is selected) */}
          {sourceActivity && (
            <div>
              <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
                Felder vergleichen &amp; auswählen
              </p>
              <MergeComparisonTable
                target={activity}
                source={sourceActivity}
                selectedFields={selectedFields}
                onToggleField={toggleField}
              />
            </div>
          )}
        </div>

        <div className="mt-6 flex gap-2 justify-end">
          <button
            onClick={onClose}
            className="cursor-pointer px-4 py-2 rounded-md text-sm font-medium bg-neutral-200 hover:bg-neutral-300 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-900 dark:text-neutral-50"
          >
            Abbrechen
          </button>
          <button
            onClick={() => mutation.mutate()}
            disabled={!isValid || mutation.isPending}
            className="cursor-pointer px-4 py-2 rounded-md text-sm font-medium text-white disabled:opacity-50 disabled:cursor-not-allowed hover:opacity-90"
            style={{ backgroundColor: accent.shade500 }}
          >
            {mutation.isPending ? 'Wird fusioniert…' : 'Fusionieren'}
          </button>
        </div>
    </ModalShell>
  )
}
