import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchSurfaces } from '@/api/enrichment'

interface Props {
  activityId: string
}

const SURFACE_COLORS: Record<string, string> = {
  asphalt: '#374151',
  paved: '#6b7280',
  unpaved: '#92400e',
  gravel: '#b45309',
  dirt: '#78350f',
  grass: '#166534',
  cobblestone: '#4b5563',
  sand: '#d97706',
}

const SURFACE_LABELS: Record<string, string> = {
  asphalt: 'Asphalt',
  paved: 'Befestigt',
  unpaved: 'Unbefestigt',
  gravel: 'Schotter',
  dirt: 'Feldweg',
  grass: 'Gras',
  cobblestone: 'Pflaster',
  sand: 'Sand',
}

export default function SurfacesSection({ activityId }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: surfaces = [], isLoading, isError } = useQuery({
    queryKey: ['activity-surfaces', activityId],
    queryFn: () => fetchSurfaces(activityId),
  })

  if (isLoading) return null
  if (isError) return null
  if (surfaces.length === 0) return null

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
          Untergrund
        </h2>
      </button>

      {!collapsed && (
        <div className="space-y-3">
          {/* Stacked bar */}
          <div className="flex h-4 rounded-full overflow-hidden gap-0.5">
            {surfaces.map(s => (
              <div
                key={s.id}
                style={{
                  width: `${s.percentage}%`,
                  backgroundColor: SURFACE_COLORS[s.surface_type] || '#9ca3af',
                }}
                title={`${SURFACE_LABELS[s.surface_type] || s.surface_type}: ${s.percentage.toFixed(0)}%`}
              />
            ))}
          </div>

          {/* Legend */}
          <div className="flex flex-wrap gap-3 text-xs text-neutral-600 dark:text-neutral-400">
            {surfaces.map(s => (
              <span key={s.id} className="flex items-center gap-1.5">
                <span
                  className="w-2.5 h-2.5 rounded-full inline-block"
                  style={{ backgroundColor: SURFACE_COLORS[s.surface_type] || '#9ca3af' }}
                />
                {SURFACE_LABELS[s.surface_type] || s.surface_type}
                <span className="tabular-nums">{s.percentage.toFixed(0)}%</span>
                {s.length_m >= 1000
                  ? <span className="text-neutral-400">({(s.length_m / 1000).toFixed(1)} km)</span>
                  : <span className="text-neutral-400">({Math.round(s.length_m)} m)</span>
                }
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
