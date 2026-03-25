import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchActivityIntervals } from '@/api/enrichment'
import type { ActivityInterval } from '@/api/types'
import { fmtDuration } from '@/lib/format'

interface Props {
  activityId: string
}

const intensityStyles: Record<string, { label: string; bg: string; text: string; dot: string }> = {
  hard:     { label: 'Hart',     bg: 'bg-red-100 dark:bg-red-900/30',    text: 'text-red-700 dark:text-red-300',       dot: 'bg-red-500' },
  moderate: { label: 'Mittel',   bg: 'bg-orange-100 dark:bg-orange-900/30', text: 'text-orange-700 dark:text-orange-300', dot: 'bg-orange-500' },
  easy:     { label: 'Locker',   bg: 'bg-green-100 dark:bg-green-900/30', text: 'text-green-700 dark:text-green-300',   dot: 'bg-green-500' },
  recovery: { label: 'Erholung', bg: 'bg-blue-100 dark:bg-blue-900/30',  text: 'text-blue-700 dark:text-blue-300',     dot: 'bg-blue-400' },
}

function IntensityBadge({ intensity }: { intensity: ActivityInterval['intensity'] }) {
  const style = intensityStyles[intensity] ?? intensityStyles.recovery
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium ${style.bg} ${style.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${style.dot}`} />
      {style.label}
    </span>
  )
}


function formatSpeed(ms: number): string {
  const kmh = ms * 3.6
  return `${kmh.toFixed(1)} km/h`
}

export default function IntervalSection({ activityId }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: intervals = [], isLoading, isError } = useQuery({
    queryKey: ['activity-intervals', activityId],
    queryFn: () => fetchActivityIntervals(activityId),
  })

  if (isLoading) return null
  if (isError) return null
  if (intervals.length === 0) return null

  const hardCount = intervals.filter(iv => iv.intensity === 'hard').length

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="cursor-pointer flex items-center gap-2 mb-3 group"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
          Intervalle
        </h2>
        <span className="text-sm text-neutral-400">
          ({intervals.length} Segmente, {hardCount} hart)
        </span>
      </button>

      {!collapsed && (
        <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
                <th className="text-center px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">#</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Dauer</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Leistung</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">HF</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Tempo</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Kadenz</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Intensität</th>
              </tr>
            </thead>
            <tbody>
              {intervals.map((iv, i) => (
                <tr
                  key={iv.id}
                  className={`border-b border-neutral-100 dark:border-neutral-800 transition-colors ${
                    i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''
                  }`}
                >
                  <td className="px-3 py-2 text-center text-xs text-neutral-600 dark:text-neutral-400">
                    {iv.segment_num}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-900 dark:text-neutral-100 font-medium">
                    {fmtDuration(iv.duration_s, 'abbr')}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                    {iv.avg_power_w != null ? `${Math.round(iv.avg_power_w)} W` : '--'}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                    {iv.avg_heart_rate != null ? `${iv.avg_heart_rate} bpm` : '--'}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                    {iv.avg_speed_ms != null ? formatSpeed(iv.avg_speed_ms) : '--'}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                    {iv.avg_cadence != null ? `${Math.round(iv.avg_cadence)} rpm` : '--'}
                  </td>
                  <td className="px-3 py-2">
                    <IntensityBadge intensity={iv.intensity} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
