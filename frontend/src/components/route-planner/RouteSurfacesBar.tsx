import { useMemo, useState, useCallback } from 'react'
import StackedBar from '@/components/shared/StackedBar'
import {
  SURFACE_GROUPS,
  SURFACE_COLORS,
  HIGHWAY_COLORS,
  HIGHWAY_LABELS,
  type AggregatedSurface,
  MAX_DISPLAYED,
  MIN_SHOWN,
} from '@/lib/surfaceConstants'
import {
  computeSubSegmentDistances,
  aggregateByField,
  type RouteSubSegment,
} from '@/lib/routeSurfaceAggregation'
import type { RouteSegment } from '@/api/routes'

interface Props {
  segments: RouteSegment[]
  onHighlight?: (type: { kind: 'highway' | 'surface'; key: string } | null) => void
}

function buildAggregated(
  items: { key: string; distanceM: number; percentage: number }[],
  groupMap?: Record<string, string>,
): AggregatedSurface[] {
  const merged = new Map<string, number>()
  for (const item of items) {
    const group = groupMap ? (groupMap[item.key] ?? item.key) : item.key
    merged.set(group, (merged.get(group) ?? 0) + item.distanceM)
  }

  const total = [...merged.values()].reduce((a, b) => a + b, 0)
  if (total === 0) return []

  const sorted = [...merged.entries()]
    .map(([surface_type, length_m]) => ({
      surface_type,
      length_m,
      percentage: (length_m / total) * 100,
    }))
    .sort((a, b) => b.length_m - a.length_m)

  const showCount = Math.min(Math.max(sorted.length, MIN_SHOWN), MAX_DISPLAYED)
  const displayed = sorted.slice(0, showCount)
  const overflow = sorted.slice(showCount)
  const otherLength = overflow.reduce((a, s) => a + s.length_m, 0)
  if (otherLength > 0) {
    displayed.push({
      surface_type: '__other__',
      length_m: otherLength,
      percentage: (otherLength / total) * 100,
    })
  }
  return displayed
}

export default function RouteSurfacesBar({ segments, onHighlight }: Props) {
  const [hoveredType, setHoveredType] = useState<{ kind: 'highway' | 'surface'; key: string } | null>(null)
  const [pinnedType, setPinnedType] = useState<{ kind: 'highway' | 'surface'; key: string } | null>(null)

  const { highways, surfaces } = useMemo(() => {
    const allWithDistance: { highway: string; surface: string; distanceM: number }[] = []

    for (const seg of segments) {
      if (!seg.sub_segments?.length || !seg.geometry?.coordinates?.length) continue
      const coords = seg.geometry.coordinates
      const computed = computeSubSegmentDistances(coords, seg.sub_segments as RouteSubSegment[])
      allWithDistance.push(...computed)
    }

    if (allWithDistance.length === 0) return { highways: [], surfaces: [] }

    const hwRaw = aggregateByField(allWithDistance, 'highway')
    const sfRaw = aggregateByField(allWithDistance, 'surface')

    return {
      highways: buildAggregated(hwRaw),
      surfaces: buildAggregated(sfRaw, SURFACE_GROUPS),
    }
  }, [segments])

  const activeType = pinnedType ?? hoveredType

  const handleHighlightChange = useCallback((
    type: { kind: 'highway' | 'surface'; key: string } | null,
  ) => {
    onHighlight?.(type)
  }, [onHighlight])

  const handleHwHover = useCallback((key: string | null) => {
    const next = key ? { kind: 'highway' as const, key } : null
    setHoveredType(next)
    if (!pinnedType) handleHighlightChange(next)
  }, [pinnedType, handleHighlightChange])

  const handleHwPin = useCallback((key: string) => {
    setPinnedType(prev => {
      const next = prev?.key === key && prev.kind === 'highway' ? null : { kind: 'highway' as const, key }
      handleHighlightChange(next)
      return next
    })
  }, [handleHighlightChange])

  const handleSfHover = useCallback((key: string | null) => {
    const next = key ? { kind: 'surface' as const, key } : null
    setHoveredType(next)
    if (!pinnedType) handleHighlightChange(next)
  }, [pinnedType, handleHighlightChange])

  const handleSfPin = useCallback((key: string) => {
    setPinnedType(prev => {
      const next = prev?.key === key && prev.kind === 'surface' ? null : { kind: 'surface' as const, key }
      handleHighlightChange(next)
      return next
    })
  }, [handleHighlightChange])

  if (highways.length === 0 && surfaces.length === 0) return null

  const hwActiveKey = activeType?.kind === 'highway' ? activeType.key : null
  const sfActiveKey = activeType?.kind === 'surface' ? activeType.key : null

  return (
    <div className="space-y-1.5 mt-2">
      {highways.length > 0 && (
        <StackedBar
          aggregated={highways}
          colorMap={HIGHWAY_COLORS}
          labelMap={HIGHWAY_LABELS}
          title="Wegearten"
          compact
          onHover={handleHwHover}
          onPin={handleHwPin}
          activeKey={hwActiveKey}
        />
      )}
      {surfaces.length > 0 && (
        <StackedBar
          aggregated={surfaces}
          colorMap={SURFACE_COLORS}
          title="Oberflächen"
          compact
          onHover={handleSfHover}
          onPin={handleSfPin}
          activeKey={sfActiveKey}
        />
      )}
    </div>
  )
}
