import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchActivitySegments } from '@/api/segments'

interface Props {
  activityId: string
}

function fmtDuration(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}`
  return `${m}:${sec.toString().padStart(2, '0')}`
}

export default function ActivitySegments({ activityId }: Props) {
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(false)

  const { data: efforts = [], isLoading } = useQuery({
    queryKey: ['activity-segments', activityId],
    queryFn: () => fetchActivitySegments(activityId),
  })

  if (isLoading) return null
  if (efforts.length === 0) {
    return (
      <div>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Segmente</h2>
        <p className="text-sm text-neutral-400 dark:text-neutral-500">Keine Segment-Treffer</p>
      </div>
    )
  }

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
          Segmente
        </h2>
        <span className="text-sm text-neutral-400">({efforts.length})</span>
      </button>

      {!collapsed && (
        <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
                <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Segment</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Zeit</th>
                <th className="text-center px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Rang</th>
                <th className="text-center px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">PB</th>
                <th className="text-center px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Richtung</th>
              </tr>
            </thead>
            <tbody>
              {efforts.map((effort, i) => (
                <tr
                  key={effort.id}
                  className={`border-b border-neutral-100 dark:border-neutral-800 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
                >
                  <td className="px-3 py-2">
                    <button
                      onClick={() => navigate('/segments')}
                      className="text-neutral-900 dark:text-neutral-100 hover:text-orange-500 transition-colors text-left"
                    >
                      {effort.segment_name ?? 'Unbenannt'}
                    </button>
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-900 dark:text-neutral-100">
                    {fmtDuration(effort.elapsed_time_s)}
                  </td>
                  <td className="px-3 py-2 text-center">
                    {effort.rank_overall != null ? (
                      effort.rank_overall === 1 ? (
                        <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-bold text-xs">
                          1
                        </span>
                      ) : (
                        <span className="text-neutral-600 dark:text-neutral-400 text-xs">#{effort.rank_overall}</span>
                      )
                    ) : (
                      <span className="text-neutral-400 text-xs">--</span>
                    )}
                  </td>
                  <td className="px-3 py-2 text-center">
                    {effort.is_personal_best && (
                      <span className="text-xs font-bold text-amber-600 dark:text-amber-400">PB</span>
                    )}
                  </td>
                  <td className="px-3 py-2 text-center text-neutral-500 dark:text-neutral-400">
                    {effort.is_reverse ? (
                      <span title="Umgekehrte Richtung">&larr;</span>
                    ) : (
                      <span title="Normale Richtung">&rarr;</span>
                    )}
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
