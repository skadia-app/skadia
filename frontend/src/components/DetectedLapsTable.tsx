import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchDetectedLaps } from '@/api/segments'
import { fmtDuration, fmtDist } from '@/lib/format'

interface Props {
  activityId: string
}

function fmtPace(speedMs: number | undefined): string {
  if (speedMs == null || speedMs <= 0) return '--'
  const minPerKm = (1000 / speedMs) / 60
  const min = Math.floor(minPerKm)
  const sec = Math.round((minPerKm - min) * 60)
  return `${min}:${sec.toString().padStart(2, '0')} /km`
}

export default function DetectedLapsTable({ activityId }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: laps = [], isLoading } = useQuery({
    queryKey: ['detected-laps', activityId],
    queryFn: () => fetchDetectedLaps(activityId),
  })

  if (isLoading) return null
  if (laps.length === 0) return null

  // Find best lap (fastest elapsed time, ignoring laps with missing/zero time)
  const validLaps = laps.filter(l => l.elapsed_time_s > 0)
  const bestIdx = validLaps.length > 0
    ? laps.indexOf(validLaps.reduce((best, lap) =>
        lap.elapsed_time_s < best.elapsed_time_s ? lap : best))
    : 0
  const bestTime = laps[bestIdx]?.elapsed_time_s ?? 0

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 cursor-pointer group"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
          Erkannte Runden
        </h2>
        <span className="text-sm text-neutral-400">({laps.length})</span>
      </button>

      {!collapsed && (
        <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
                <th className="text-center px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">#</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Zeit</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Delta</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Distanz</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Tempo</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">HR</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Power</th>
              </tr>
            </thead>
            <tbody>
              {laps.map((lap, i) => {
                const isBest = i === bestIdx
                const hasTime = lap.elapsed_time_s > 0 && bestTime > 0
                const delta = hasTime ? lap.elapsed_time_s - bestTime : null
                return (
                  <tr
                    key={lap.id}
                    className={`border-b border-neutral-100 dark:border-neutral-800 transition-colors ${
                      isBest
                        ? 'bg-amber-50/50 dark:bg-amber-900/10'
                        : i % 2 !== 0
                          ? 'bg-neutral-50/50 dark:bg-neutral-800/20'
                          : ''
                    }`}
                  >
                    <td className="px-3 py-2 text-center">
                      {isBest ? (
                        <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-bold text-xs">
                          {lap.lap_number}
                        </span>
                      ) : (
                        <span className="text-neutral-600 dark:text-neutral-400 text-xs">{lap.lap_number}</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-neutral-900 dark:text-neutral-100 font-medium">
                      {fmtDuration(lap.elapsed_time_s)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums">
                      {delta == null ? (
                        <span className="text-neutral-400 text-xs">--</span>
                      ) : delta === 0 ? (
                        <span className="text-amber-600 dark:text-amber-400 font-bold text-xs">BEST</span>
                      ) : (
                        <span className="text-neutral-500 dark:text-neutral-400 text-xs">+{fmtDuration(delta)}</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                      {fmtDist(lap.distance_m)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                      {fmtPace(lap.avg_speed_ms)}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                      {lap.avg_heart_rate != null ? `${Math.round(lap.avg_heart_rate)}` : '--'}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                      {lap.avg_power_w != null ? `${Math.round(lap.avg_power_w)} W` : '--'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
