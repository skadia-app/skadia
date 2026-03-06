import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'

interface SegmentActivityListProps {
  segmentId: number
}

function formatTime(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  return h > 0
    ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
    : `${m}:${String(sec).padStart(2, '0')}`
}

export function SegmentActivityList({ segmentId }: SegmentActivityListProps) {
  const navigate = useNavigate()
  const [sortMode, setSortMode] = useState<'standard' | 'reverse' | 'all'>('standard')
  const [selectedSports, setSelectedSports] = useState<string[]>([])

  const { data, isLoading } = useQuery({
    queryKey: ['segment-performances', segmentId],
    queryFn: () =>
      fetch(`/api/v1/segments/${segmentId}/performances`).then(r => r.json()),
  })

  const all: any[] = data?.performances ?? []

  const sorted =
    sortMode === 'reverse'
      ? [...all].reverse()
      : all // 'standard' and 'all' use default order (newest first from API)

  const filtered =
    selectedSports.length > 0
      ? sorted.filter((p: any) => selectedSports.includes(p.sport_type))
      : sorted

  const sportTypes = [...new Set(all.map((p: any) => p.sport_type as string))]

  const toggleSport = (s: string) =>
    setSelectedSports(prev =>
      prev.includes(s) ? prev.filter(x => x !== s) : [...prev, s]
    )

  return (
    <div className="flex flex-col gap-3">
      <div className="flex gap-2">
        {(['standard', 'reverse', 'all'] as const).map(mode => (
          <button
            key={mode}
            onClick={() => setSortMode(mode)}
            className={`px-3 py-1 rounded text-sm transition-colors ${
              sortMode === mode
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {mode === 'standard' ? 'Standard' : mode === 'reverse' ? 'Reverse' : 'Alle'}
          </button>
        ))}
      </div>

      {sportTypes.length > 1 && (
        <div className="flex flex-wrap gap-2 text-sm">
          {sportTypes.map(sport => (
            <label key={sport} className="flex items-center gap-1 cursor-pointer text-neutral-700 dark:text-neutral-300">
              <input
                type="checkbox"
                checked={selectedSports.includes(sport)}
                onChange={() => toggleSport(sport)}
                className="rounded"
              />
              {sport}
            </label>
          ))}
        </div>
      )}

      <div className="flex flex-col gap-2 max-h-72 overflow-y-auto">
        {isLoading && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">Lädt...</div>
        )}
        {!isLoading && filtered.length === 0 && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">
            Keine Aktivitäten auf diesem Segment.
          </div>
        )}
        {filtered.map((perf: any) => (
          <button
            key={perf.id}
            onClick={() => navigate(`/activities/${perf.activity_id}`)}
            className="text-left p-3 border border-neutral-200 dark:border-neutral-700 rounded-lg hover:border-neutral-300 dark:hover:border-neutral-600 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors"
          >
            <div className="font-medium text-sm truncate text-neutral-900 dark:text-neutral-100">
              {perf.title}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 flex gap-2 mt-1">
              <span>{new Date(perf.started_at).toLocaleDateString('de-DE')}</span>
              <span>·</span>
              <span>{formatTime(perf.elapsed_time_s)}</span>
              <span>·</span>
              <span>{perf.sport_type}</span>
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}
