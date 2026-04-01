import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchSurfaces } from '@/api/enrichment'
import {
  SURFACE_COLORS,
  HIGHWAY_COLORS,
  HIGHWAY_LABELS,
  aggregateSurfaces,
  aggregateHighways,
} from '@/lib/surfaceConstants'
import StackedBar from '@/components/shared/StackedBar'

interface Props {
  activityId: string
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

  // Split by category
  const surfaceRows = surfaces.filter(s => s.category === 'surface')
  const highwayRows = surfaces.filter(s => s.category === 'highway')

  // Legacy fallback: old data without category → show everything as a single surface bar
  const isLegacy = surfaceRows.length === 0 && highwayRows.length === 0

  const aggregatedSurfaces = isLegacy
    ? aggregateSurfaces(surfaces)
    : aggregateSurfaces(surfaceRows)
  const aggregatedHighways = isLegacy ? [] : aggregateHighways(highwayRows)

  const hasAnything = aggregatedSurfaces.length > 0 || aggregatedHighways.length > 0
  if (!hasAnything) return null

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 group cursor-pointer"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-accent-DEFAULT transition-colors">
          Wege und Oberflächen
        </h2>
      </button>

      {!collapsed && (
        <div className="space-y-5">
          {aggregatedHighways.length > 0 && (
            <StackedBar
              aggregated={aggregatedHighways}
              colorMap={HIGHWAY_COLORS}
              labelMap={HIGHWAY_LABELS}
              title="Wegearten"
            />
          )}
          {aggregatedSurfaces.length > 0 && (
            <StackedBar
              aggregated={aggregatedSurfaces}
              colorMap={SURFACE_COLORS}
              title={isLegacy ? 'Untergrund' : 'Oberflächen'}
            />
          )}
        </div>
      )}
    </div>
  )
}
