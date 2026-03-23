import { useQuery } from '@tanstack/react-query'
import { useState, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { Wrench } from 'lucide-react'
import { sportLabel } from '@/lib/sportTaxonomy'
import { apiClient } from '@/api/client'
import { fmtDuration } from '@/lib/format'
import type { Segment } from '@/api/segments'
import type { ActivityFilterState } from '@/components/ActivityFilters'

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
  filters?: ActivityFilterState
}


export function SegmentActivityList({ segmentId, segment, filters }: SegmentActivityListProps) {
  const navigate = useNavigate()
  const [directionMode, setDirectionMode] = useState<'standard' | 'reverse' | 'all'>('standard')

  const isReverse = directionMode === 'all' ? undefined : directionMode === 'reverse'

  const { data, isLoading } = useQuery({
    queryKey: ['segment-performances', segmentId, directionMode],
    queryFn: () => {
      const qs = isReverse !== undefined ? `?is_reverse=${isReverse}` : ''
      return apiClient<{ performances: Performance[] }>(`/segments/${segmentId}/performances${qs}`)
    },
  })

  const all: Performance[] = data?.performances ?? []

  const filtered = useMemo(() => {
    return all.filter(p => {
      const date = new Date(p.started_at)

      if (filters?.dateFrom) {
        const from = new Date(filters.dateFrom)
        from.setHours(0, 0, 0, 0)
        if (date < from) return false
      }
      if (filters?.dateTo) {
        const to = new Date(filters.dateTo)
        to.setHours(23, 59, 59, 999)
        if (date > to) return false
      }

      if (filters?.sportTypes && filters.sportTypes.size > 0) {
        // sportTypes uses "sport" or "sport:subtype" keys — match against sport_type
        const matches = [...filters.sportTypes].some(key => {
          const [sport] = key.split(':')
          return sport === p.sport_type || key === p.sport_type
        })
        if (!matches) return false
      }

      if (filters?.gearFilters && filters.gearFilters.length > 0) {
        const includes = filters.gearFilters.filter(gf => gf.state === 'include')
        const excludes = filters.gearFilters.filter(gf => gf.state === 'exclude')
        if (includes.length > 0 && (!p.gear_id || !includes.some(gf => gf.id === p.gear_id))) return false
        if (excludes.length > 0 && p.gear_id && excludes.some(gf => gf.id === p.gear_id)) return false
      }

      return true
    })
  }, [all, filters])

  return (
    <div className="flex flex-col gap-3">
      {/* Direction controls */}
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

      {/* Activity list */}
      <div className="flex flex-col gap-2 max-h-72 overflow-y-auto">
        {isLoading && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">Lädt...</div>
        )}
        {!isLoading && filtered.length === 0 && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">
            Keine Aktivitäten mit diesen Filtern.
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
              <span>{fmtDuration(perf.elapsed_time_s)}</span>
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
