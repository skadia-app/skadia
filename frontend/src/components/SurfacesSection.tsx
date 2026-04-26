import { useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { Info, Loader2 } from 'lucide-react'
import type { ActivityTrackSummary, TrackSummarySurfaceItem } from '@/api/trackSummary'
import type { HighlightKind } from '@/hooks/useStickyHighlight'
import {
  SURFACE_COLORS,
  HIGHWAY_COLORS,
  HIGHWAY_LABELS,
  aggregateSurfaces,
  aggregateHighways,
} from '@/lib/surfaceConstants'
import StackedBar from '@/components/shared/StackedBar'
import { useActiveJobs } from '@/lib/useActiveJobs'

export type SurfaceHighlight = { kind: 'highway' | 'surface'; key: string } | null

interface Props {
  summary?: Pick<ActivityTrackSummary, 'surfaces' | 'highways' | 'graph_snap_used' | 'surface_ranges' | 'highway_ranges'> | null
  /**
   * Whether the parent query is still loading (undefined = query in flight).
   * When true, a skeleton/pulse state is shown instead of this section.
   */
  summaryLoading?: boolean
  /** Activity ID used to check for active enrichment jobs. */
  activityId?: string
  /** Hover changes (null = no hover). Parent owns the global highlight state. */
  onHighlight?: (h: HighlightKind) => void
  /** Controlled pin state (sticky after click). */
  pinnedType?: SurfaceHighlight
  /** Click toggles the pin; parent decides next state. */
  onPin?: (h: SurfaceHighlight) => void
  /** Whether track streams are still loading — suppresses hover interaction. */
  streamsLoading?: boolean
  /**
   * Optional pills row rendered directly after the section header (before bars).
   * Used by ActivityDetailPage to place StatsRow (forest/protected/POIs/scales)
   * inside this section instead of floating above it.
   */
  pills?: ReactNode
}

interface SurfaceDisplayRow {
  id: string
  category: 'surface' | 'highway'
  surface_type: string
  path_type: string
  length_m: number
  percentage: number
}

function summaryRows(
  summary: Pick<ActivityTrackSummary, 'surfaces' | 'highways'> | null
): SurfaceDisplayRow[] {
  if (!summary) return []
  const surfaces = (summary.surfaces ?? []).map(item => summarySurfaceRow(item, 'surface'))
  const highways = (summary.highways ?? []).map(item => summarySurfaceRow(item, 'highway'))
  return [...surfaces, ...highways]
}

function summarySurfaceRow(item: TrackSummarySurfaceItem, category: 'surface' | 'highway'): SurfaceDisplayRow {
  return {
    id: `summary-${category}-${item.type}`,
    category,
    surface_type: item.type,
    path_type: item.type,
    length_m: item.length_m,
    percentage: item.percentage,
  }
}

export default function SurfacesSection({ summary, summaryLoading, activityId, onHighlight, pinnedType = null, onPin, streamsLoading, pills }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { isJobActiveForEntity } = useActiveJobs()
  const isEnrichmentActive = activityId != null && isJobActiveForEntity(activityId)

  const rows = summaryRows(summary ?? null)

  // Split by category — done before the early return so hooks below stay stable.
  const surfaceRows = rows.filter(s => s.category === 'surface')
  const highwayRows = rows.filter(s => s.category === 'highway')

  const aggregatedSurfaces = aggregateSurfaces(surfaceRows)
  const aggregatedHighways = aggregateHighways(highwayRows)

  // Whether map highlight is available for this activity:
  // - graph_snap_used explicitly false → SQL fallback was used, ranges are null
  // - graph_snap_used absent AND all ranges null → treat as unavailable
  const rangesAvailable = summary != null && (
    summary.graph_snap_used === true ||
    (summary.graph_snap_used == null && (
      (summary.surface_ranges != null && Object.keys(summary.surface_ranges).length > 0) ||
      (summary.highway_ranges != null && Object.keys(summary.highway_ranges).length > 0)
    ))
  )

  // Hover/pin handlers — mirror RouteSurfacesBar so the route-planner pattern
  // is the single source of truth for hover-coordination semantics.
  // When ranges or streams are not available, suppress the highlight callback.
  const handleHwHover = useCallback((key: string | null) => {
    if (!rangesAvailable || streamsLoading) return
    if (!key) { onHighlight?.(null); return }
    // Pass overflow raw types so TrackMap can union ranges for the '__other__' bucket.
    const bucket = aggregatedHighways.find(a => a.surface_type === key)
    onHighlight?.({ kind: 'highway' as const, key, overflowRawTypes: bucket?.overflowRawTypes })
  }, [onHighlight, rangesAvailable, streamsLoading, aggregatedHighways])

  const handleHwPin = useCallback((key: string) => {
    const next: SurfaceHighlight =
      pinnedType?.kind === 'highway' && pinnedType.key === key
        ? null
        : { kind: 'highway', key }
    onPin?.(next)
  }, [pinnedType, onPin])

  const handleSfHover = useCallback((key: string | null) => {
    if (!rangesAvailable || streamsLoading) return
    if (!key) { onHighlight?.(null); return }
    // Pass overflow raw types (display-group names) so TrackMap can union ranges for '__other__'.
    const bucket = aggregatedSurfaces.find(a => a.surface_type === key)
    onHighlight?.({ kind: 'surface' as const, key, overflowRawTypes: bucket?.overflowRawTypes })
  }, [onHighlight, rangesAvailable, streamsLoading, aggregatedSurfaces])

  const handleSfPin = useCallback((key: string) => {
    const next: SurfaceHighlight =
      pinnedType?.kind === 'surface' && pinnedType.key === key
        ? null
        : { kind: 'surface', key }
    onPin?.(next)
  }, [pinnedType, onPin])

  // Query still in flight — let the parent's skeleton handle it.
  if (summaryLoading) return null

  const hasAnything = aggregatedSurfaces.length > 0 || aggregatedHighways.length > 0

  // No surface data yet: show a "computing" hint when an enrichment job is running,
  // so users know results are on the way rather than seeing a blank section.
  if (!hasAnything && isEnrichmentActive) {
    return (
      <div>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">
          Wege und Oberflächen
        </h2>
        <p className="flex items-center gap-1.5 text-xs text-neutral-400 dark:text-neutral-500">
          <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" aria-hidden="true" />
          Wird verarbeitet…
        </p>
      </div>
    )
  }

  if (rows.length === 0) return null
  if (!hasAnything) return null

  const hwActiveKey = pinnedType?.kind === 'highway' ? pinnedType.key : null
  const sfActiveKey = pinnedType?.kind === 'surface' ? pinnedType.key : null

  // Bars are interactive only when ranges are present and streams have loaded.
  const barsInteractive = rangesAvailable && !streamsLoading

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

      {/* Item 1: pills row (forest/protected/POIs/scales) placed below the header,
          not above. Passed in by ActivityDetailPage via the pills prop. */}
      {pills && <div className="mb-3">{pills}</div>}

      {!collapsed && (
        <div className="space-y-5">
          {aggregatedHighways.length > 0 && (
            <StackedBar
              aggregated={aggregatedHighways}
              colorMap={HIGHWAY_COLORS}
              labelMap={HIGHWAY_LABELS}
              title="Wegearten"
              onHover={barsInteractive ? handleHwHover : undefined}
              onPin={barsInteractive ? handleHwPin : undefined}
              activeKey={hwActiveKey}
              streamsLoading={streamsLoading}
            />
          )}
          {aggregatedSurfaces.length > 0 && (
            <StackedBar
              aggregated={aggregatedSurfaces}
              colorMap={SURFACE_COLORS}
              title="Oberflächen"
              onHover={barsInteractive ? handleSfHover : undefined}
              onPin={barsInteractive ? handleSfPin : undefined}
              activeKey={sfActiveKey}
              streamsLoading={streamsLoading}
            />
          )}

          {/* Availability hints — shown once per section, not per bar */}
          {streamsLoading && (
            <p className="flex items-center gap-1.5 text-xs text-neutral-400 dark:text-neutral-500">
              <Info className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              Kartendaten werden geladen…
            </p>
          )}
          {!streamsLoading && !rangesAvailable && (
            <p className="flex items-center gap-1.5 text-xs text-neutral-400 dark:text-neutral-500">
              <Info className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              Karten-Highlight für Wege/Oberflächen ist für diese Aktivität nicht verfügbar (eingeschränkte Tile-Coverage).
            </p>
          )}
        </div>
      )}
    </div>
  )
}
