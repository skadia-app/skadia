import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchStops } from '@/api/enrichment'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'

interface Props {
  activityId: string
}

function formatDurationShort(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const min = Math.floor(seconds / 60)
  const sec = seconds % 60
  if (min < 60) return sec > 0 ? `${min}m ${sec}s` : `${min} min`
  const h = Math.floor(min / 60)
  const rm = min % 60
  return rm > 0 ? `${h}h ${rm}m` : `${h}h`
}

export default function StopsSection({ activityId }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: stops = [], isLoading, isError } = useQuery({
    queryKey: ['activity-stops', activityId],
    queryFn: () => fetchStops(activityId),
  })

  if (isLoading) return null
  if (isError) return null
  if (stops.length === 0) return null

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 group"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-orange-500 transition-colors">
          Stopps
        </h2>
        <span className="text-sm text-neutral-400">({stops.length})</span>
      </button>

      {!collapsed && (
        <div className="space-y-1.5">
          {stops.map(stop => (
            <div
              key={stop.id}
              className="flex items-center justify-between text-sm px-3 py-2 rounded-lg bg-neutral-50 dark:bg-neutral-800/40"
            >
              <div className="flex items-center gap-3">
                <span className="text-xs tabular-nums text-neutral-500 dark:text-neutral-400">
                  {format(new Date(stop.started_at), 'HH:mm', { locale: de })}
                </span>
                {stop.name ? (
                  <span className="font-medium text-neutral-800 dark:text-neutral-200">{stop.name}</span>
                ) : (
                  <span className="text-neutral-500 dark:text-neutral-400">Stopp</span>
                )}
              </div>
              <span className="tabular-nums text-neutral-600 dark:text-neutral-400">
                {formatDurationShort(stop.duration_s)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
