import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchActivitySegments } from '@/api/segments'
import type { StreamPoint } from '@/api/types'
import SegmentEditorModal from '@/components/SegmentEditorModal'
import DifficultyBadge from '@/components/DifficultyBadge'

interface Props {
  activityId: string
  /** GPS stream data — enables the "Create segment from activity" button when present. */
  streams?: StreamPoint[]
  /** Sport type of the activity — pre-fills the sport group in the creation panel. */
  activitySportType?: string
}

function fmtDuration(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}`
  return `${m}:${sec.toString().padStart(2, '0')}`
}

/** Map a Garmin sport_type to the nearest segment sport_group. */
function sportTypeToGroup(sportType?: string): string {
  if (!sportType) return 'generic'
  const t = sportType.toLowerCase()
  if (t.includes('cycling') || t.includes('bike') || t.includes('ride') || t === 'e_bike_fitness' || t === 'gravel_cycling') return 'bike'
  if (t.includes('run') || t.includes('walk') || t.includes('hike') || t.includes('trail')) return 'foot'
  if (t.includes('swim')) return 'swim'
  if (t.includes('ski') || t.includes('snowboard') || t.includes('snow')) return 'ski'
  if (t.includes('kayak') || t.includes('row') || t.includes('paddle') || t.includes('canoe') || t.includes('stand_up_paddleboarding')) return 'paddle'
  return 'generic'
}

export default function ActivitySegments({ activityId, streams, activitySportType }: Props) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [collapsed, setCollapsed] = useState(false)
  const [showEditor, setShowEditor] = useState(false)

  const { data: efforts = [], isLoading } = useQuery({
    queryKey: ['activity-segments', activityId],
    queryFn: () => fetchActivitySegments(activityId),
  })

  const hasGps = (streams ?? []).filter(p => p.lat != null && p.lng != null).length >= 2

  if (isLoading) return null

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="flex items-center gap-2 group"
        >
          <svg
            className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
            fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
            Segmente
          </h2>
          {efforts.length > 0 && (
            <span className="text-sm text-neutral-400">({efforts.length})</span>
          )}
        </button>

        {hasGps && (
          <button
            onClick={() => setShowEditor(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg
                       border border-neutral-200 dark:border-neutral-700
                       text-neutral-700 dark:text-neutral-300
                       hover:bg-orange-50 dark:hover:bg-orange-900/20 hover:border-orange-300 dark:hover:border-orange-700
                       hover:text-accent-600 dark:hover:text-orange-400
                       transition-colors"
            title="Segment aus dem Track dieser Aktivitaet erstellen"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            Segment erstellen
          </button>
        )}
      </div>

      {/* Full-screen editor modal */}
      {showEditor && hasGps && (
        <SegmentEditorModal
          mode="track"
          trackPoints={streams
            ?.filter(p => p.lat != null && p.lng != null)
            .map(p => ({ lat: p.lat!, lng: p.lng! }))}
          sourceActivityId={activityId}
          defaultSportGroup={sportTypeToGroup(activitySportType)}
          onSuccess={() => {
            setShowEditor(false)
            queryClient.invalidateQueries({ queryKey: ['activity-segments', activityId] })
            queryClient.invalidateQueries({ queryKey: ['segments'] })
          }}
          onClose={() => setShowEditor(false)}
        />
      )}

      {!collapsed && efforts.length === 0 && !showEditor && (
        <p className="text-sm text-neutral-400 dark:text-neutral-500">Keine Segment-Treffer</p>
      )}

      {!collapsed && efforts.length > 0 && (
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
                    <div className="flex items-center gap-1.5">
                      <button
                        onClick={() => navigate('/segments')}
                        className="text-neutral-900 dark:text-neutral-100 hover:text-accent-DEFAULT transition-colors text-left"
                      >
                        {effort.segment_name ?? 'Unbenannt'}
                      </button>
                      {effort.segment_type_meta && (
                        <DifficultyBadge meta={effort.segment_type_meta} />
                      )}
                    </div>
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
                  <td className="px-3 py-2 text-center">
                    <span
                      title={effort.is_reverse ? 'Umgekehrte Richtung' : 'Referenzrichtung'}
                      className={`text-xs font-medium ${effort.is_reverse ? 'text-amber-500 dark:text-amber-400' : 'text-emerald-600 dark:text-emerald-400'}`}
                    >
                      {effort.is_reverse ? '← Umgekehrt' : '→ Normal'}
                    </span>
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
