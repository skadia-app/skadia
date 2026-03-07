import { useQuery } from '@tanstack/react-query'
import { useState, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { Wrench } from 'lucide-react'
import { sportLabel } from '@/lib/sportTaxonomy'
import { apiClient } from '@/api/client'
import type { Segment } from '@/api/segments'

interface Performance {
  id: string
  activity_id: string
  elapsed_time_s: number
  moving_time_s: number
  started_at: string
  sport_type: string
  title: string
  is_reverse: boolean
  gear_id?: string | null
  gear_name?: string | null
  gear_type?: string | null
}

function getDirectionLabels(segment?: Segment) {
  if (segment?.segment_type === 'lift') return { standard: 'Bergfahrt', reverse: 'Talfahrt', all: 'Alle' }
  if (segment?.climb_type === 'bergauf' || segment?.climb_type === 'bergab') return { standard: 'Bergauf', reverse: 'Bergab', all: 'Alle' }
  return { standard: 'Standard', reverse: 'Reverse', all: 'Alle' }
}

interface SegmentActivityListProps {
  segmentId: string
  segment?: Segment
}

function formatTime(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  return h > 0
    ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
    : `${m}:${String(sec).padStart(2, '0')}`
}

function toggleItem<T>(arr: T[], val: T): T[] {
  return arr.includes(val) ? arr.filter(x => x !== val) : [...arr, val]
}

export function SegmentActivityList({ segmentId, segment }: SegmentActivityListProps) {
  const navigate = useNavigate()
  const [directionMode, setDirectionMode] = useState<'standard' | 'reverse' | 'all'>('standard')
  const [selectedSports, setSelectedSports] = useState<string[]>([])
  const [selectedGearIds, setSelectedGearIds] = useState<string[]>([])

  const isReverse = directionMode === 'all' ? undefined : directionMode === 'reverse'

  const { data, isLoading } = useQuery({
    queryKey: ['segment-performances', segmentId, directionMode],
    queryFn: () => {
      const qs = isReverse !== undefined ? `?is_reverse=${isReverse}` : ''
      return apiClient<{ performances: Performance[] }>(`/segments/${segmentId}/performances${qs}`)
    },
  })

  const all: Performance[] = data?.performances ?? []

  // Unique sport types found in data
  const availableSportTypes = useMemo(
    () => [...new Set(all.map(p => p.sport_type))].filter(Boolean),
    [all],
  )

  // Unique gear options found in data
  const availableGear = useMemo(() => {
    const map = new Map<string, string>()
    all.forEach(p => {
      if (p.gear_id && p.gear_name) map.set(p.gear_id, p.gear_name)
    })
    return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
  }, [all])

  const filtered = useMemo(() => {
    return all.filter(p => {
      if (selectedSports.length > 0 && !selectedSports.includes(p.sport_type)) return false
      if (selectedGearIds.length > 0 && (!p.gear_id || !selectedGearIds.includes(p.gear_id)))
        return false
      return true
    })
  }, [all, selectedSports, selectedGearIds])

  const activeFilterCount =
    (selectedSports.length > 0 ? 1 : 0) + (selectedGearIds.length > 0 ? 1 : 0)

  return (
    <div className="flex flex-col gap-3">
      {/* Sort controls */}
      <div className="flex gap-2">
        {(['standard', 'reverse', 'all'] as const).map(mode => (
          <button
            key={mode}
            onClick={() => setDirectionMode(mode)}
            className={`px-3 py-1 rounded text-sm transition-colors ${
              directionMode === mode
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {getDirectionLabels(segment)[mode as 'standard' | 'reverse' | 'all']}
          </button>
        ))}
      </div>

      {/* Sport-type filter — only shown when more than one sport is present */}
      {availableSportTypes.length > 1 && (
        <div className="flex flex-wrap gap-1.5">
          {availableSportTypes.map(sport => {
            const active = selectedSports.includes(sport)
            return (
              <button
                key={sport}
                onClick={() => setSelectedSports(prev => toggleItem(prev, sport))}
                style={
                  active
                    ? {
                        backgroundColor: 'var(--color-accent-DEFAULT, #FF6B35)',
                        borderColor: 'var(--color-accent-DEFAULT, #FF6B35)',
                        color: '#fff',
                      }
                    : undefined
                }
                className={`px-2 py-0.5 rounded-full text-xs border transition-colors ${
                  active
                    ? ''
                    : 'border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {sportLabel(sport)}
              </button>
            )
          })}
        </div>
      )}

      {/* Gear filter — only shown when activities have gear data */}
      {availableGear.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-neutral-500 dark:text-neutral-400 flex items-center gap-1">
            <Wrench className="w-3 h-3" />
            Gear:
          </span>
          {availableGear.map(g => {
            const active = selectedGearIds.includes(g.id)
            return (
              <button
                key={g.id}
                onClick={() => setSelectedGearIds(prev => toggleItem(prev, g.id))}
                style={
                  active
                    ? {
                        backgroundColor: 'var(--color-accent-DEFAULT, #FF6B35)',
                        borderColor: 'var(--color-accent-DEFAULT, #FF6B35)',
                        color: '#fff',
                      }
                    : undefined
                }
                className={`px-2 py-0.5 rounded-full text-xs border transition-colors ${
                  active
                    ? ''
                    : 'border-neutral-300 dark:border-neutral-600 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                }`}
              >
                {g.name}
              </button>
            )
          })}
        </div>
      )}

      {/* Activity list */}
      <div className="flex flex-col gap-2 max-h-72 overflow-y-auto">
        {isLoading && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">Lädt...</div>
        )}
        {!isLoading && filtered.length === 0 && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">
            {activeFilterCount > 0
              ? 'Keine Aktivitäten mit diesen Filtern.'
              : 'Keine Aktivitäten auf diesem Segment.'}
          </div>
        )}
        {filtered.map(perf => (
          <button
            key={perf.id}
            onClick={() => navigate(`/activities/${perf.activity_id}`)}
            className="text-left p-3 border border-neutral-200 dark:border-neutral-700 rounded-lg hover:border-neutral-300 dark:hover:border-neutral-600 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer"
          >
            <div className="font-medium text-sm truncate text-neutral-900 dark:text-neutral-100">
              {perf.title}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 flex flex-wrap gap-2 mt-1">
              <span>{new Date(perf.started_at).toLocaleDateString('de-DE')}</span>
              <span>·</span>
              <span>{formatTime(perf.elapsed_time_s)}</span>
              <span>·</span>
              <span>{sportLabel(perf.sport_type)}</span>
              {perf.gear_name && (
                <>
                  <span>·</span>
                  <span className="inline-flex items-center gap-1 text-neutral-600 dark:text-neutral-400">
                    <Wrench className="w-3 h-3" />
                    {perf.gear_name}
                  </span>
                </>
              )}
            </div>
          </button>
        ))}
      </div>

      {/* Result count */}
      {!isLoading && all.length > 0 && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {filtered.length === all.length
            ? `${all.length} Aktivität${all.length !== 1 ? 'en' : ''}`
            : `${filtered.length} von ${all.length} Aktivitäten`}
        </div>
      )}
    </div>
  )
}
