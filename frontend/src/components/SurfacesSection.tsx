import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchSurfaces } from '@/api/enrichment'
import type { ActivitySurface } from '@/api/types'

interface Props {
  activityId: string
}

const SURFACE_COLORS: Record<string, string> = {
  asphalt: '#374151',
  paved: '#6b7280',
  unpaved: '#92400e',
  gravel: '#b45309',
  fine_gravel: '#d97706',
  dirt: '#78350f',
  ground: '#7c5533',
  grass: '#166534',
  cobblestone: '#4b5563',
  pebblestone: '#4b5563',
  paving_stones: '#4b5563',
  sett: '#6b7280',
  sand: '#d97706',
  compacted: '#a16207',
  concrete: '#9ca3af',
  rock: '#6b7280',
  mud: '#78350f',
  wood: '#92400e',
}

const SURFACE_LABELS: Record<string, string> = {
  asphalt: 'Asphalt',
  gravel: 'Schotter',
  dirt: 'Feldweg',
  pebblestone: 'Kopfsteinpflaster',
  compacted: 'Befestigt',
  fine_gravel: 'Feinkies',
  ground: 'Naturweg',
  grass: 'Gras',
  sand: 'Sand',
  paving_stones: 'Pflaster',
  sett: 'Naturstein',
  concrete: 'Beton',
  unpaved: 'Unbefestigt',
  paved: 'Befestigt',
  rock: 'Fels',
  mud: 'Schlamm',
  wood: 'Holz',
  cobblestone: 'Kopfsteinpflaster',
}

interface AggregatedSurface {
  surface_type: string
  length_m: number
  percentage: number
}

// Always show at least MIN_SHOWN surface types before collapsing to "Sonstige",
// regardless of their percentage. This prevents routes that are 98% asphalt
// from hiding all other surfaces (gravel, compacted, etc.) in a single
// "Sonstige 2%" bucket.
const MAX_DISPLAYED = 7
const MIN_SHOWN = 5

function aggregateSurfaces(raw: ActivitySurface[]): AggregatedSurface[] {
  // Group by surface_type, summing length_m
  const map = new Map<string, number>()
  for (const s of raw) {
    map.set(s.surface_type, (map.get(s.surface_type) ?? 0) + s.length_m)
  }

  const totalLength = Array.from(map.values()).reduce((a, b) => a + b, 0)
  if (totalLength === 0) return []

  // Build sorted list (descending by length)
  const sorted: AggregatedSurface[] = Array.from(map.entries())
    .map(([surface_type, length_m]) => ({
      surface_type,
      length_m,
      percentage: (length_m / totalLength) * 100,
    }))
    .sort((a, b) => b.length_m - a.length_m)

  // Show at least MIN_SHOWN entries, up to MAX_DISPLAYED — remainder → "Sonstige"
  const showCount = Math.min(Math.max(sorted.length, MIN_SHOWN), MAX_DISPLAYED)
  let displayed = sorted.slice(0, showCount)
  const overflow = sorted.slice(showCount)

  const otherLength = overflow.reduce((a, s) => a + s.length_m, 0)
  if (otherLength > 0) {
    displayed = [
      ...displayed,
      {
        surface_type: '__other__',
        length_m: otherLength,
        percentage: (otherLength / totalLength) * 100,
      },
    ]
  }

  return displayed
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

  const aggregated = aggregateSurfaces(surfaces)
  if (aggregated.length === 0) return null

  function label(surface_type: string): string {
    if (surface_type === '__other__') return 'Sonstige'
    return SURFACE_LABELS[surface_type] ?? surface_type
  }

  function color(surface_type: string): string {
    if (surface_type === '__other__') return '#6b7280'
    return SURFACE_COLORS[surface_type] ?? '#9ca3af'
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
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
          Untergrund
        </h2>
      </button>

      {!collapsed && (
        <div className="space-y-3">
          {/* Stacked bar */}
          <div className="flex h-4 rounded-full overflow-hidden gap-0.5">
            {aggregated.map(s => (
              <div
                key={s.surface_type}
                style={{
                  width: `${s.percentage}%`,
                  backgroundColor: color(s.surface_type),
                }}
                title={`${label(s.surface_type)}: ${s.percentage.toFixed(0)}%`}
              />
            ))}
          </div>

          {/* Legend */}
          <div className="flex flex-wrap gap-x-4 gap-y-1.5 text-xs text-neutral-600 dark:text-neutral-400">
            {aggregated.map(s => (
              <span key={s.surface_type} className="flex items-center gap-1.5">
                <span
                  className="w-2.5 h-2.5 rounded-full inline-block shrink-0"
                  style={{ backgroundColor: color(s.surface_type) }}
                />
                <span className="text-neutral-800 dark:text-neutral-200">{label(s.surface_type)}</span>
                <span className="tabular-nums">{s.percentage.toFixed(0)}%</span>
                {s.length_m >= 1000
                  ? <span className="text-neutral-400 dark:text-neutral-500">({(s.length_m / 1000).toFixed(1)} km)</span>
                  : <span className="text-neutral-400 dark:text-neutral-500">({Math.round(s.length_m)} m)</span>
                }
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
