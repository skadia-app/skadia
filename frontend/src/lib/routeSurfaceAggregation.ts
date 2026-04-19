// ─── Types ────────────────────────────────────────────────────────────────────

export interface RouteSubSegment {
  from_idx: number
  to_idx: number
  highway: string
  surface: string
  sac_scale?: string
  mtb_scale?: string
  tracktype?: string
  osm_way_id?: number
  // F26
  mtb_scale_uphill?: string
  via_ferrata_scale?: string
  trail_visibility?: string
}

export interface RouteSurfaceData {
  subSegments: RouteSubSegment[]
  totalDistanceM: number
}

// ─── Internal: Haversine ──────────────────────────────────────────────────────

/** Haversine distance in meters between two GeoJSON [lon, lat] points. */
function haversineM(a: number[], b: number[]): number {
  const R = 6371000
  const dLat = (b[1] - a[1]) * Math.PI / 180
  const dLon = (b[0] - a[0]) * Math.PI / 180
  const sinDLat = Math.sin(dLat / 2)
  const sinDLon = Math.sin(dLon / 2)
  const h =
    sinDLat * sinDLat +
    Math.cos(a[1] * Math.PI / 180) * Math.cos(b[1] * Math.PI / 180) * sinDLon * sinDLon
  return 2 * R * Math.asin(Math.sqrt(h))
}

// ─── Public API ───────────────────────────────────────────────────────────────

export interface SubSegmentWithDistance {
  highway: string
  surface: string
  distanceM: number
}

/**
 * Compute per-sub-segment distances from GeoJSON coordinates.
 *
 * Sums Haversine distances between consecutive coordinates within
 * each sub-segment's [from_idx, to_idx] range (inclusive).
 *
 * @param coordinates - GeoJSON coordinate array: [lon, lat] or [lon, lat, ele]
 * @param subSegments - Sub-segment descriptors from BadRouter properties.segments
 */
export function computeSubSegmentDistances(
  coordinates: number[][],
  subSegments: RouteSubSegment[],
): SubSegmentWithDistance[] {
  return subSegments.map(seg => {
    const start = Math.max(0, seg.from_idx)
    const end = Math.min(coordinates.length - 1, seg.to_idx)
    let distanceM = 0
    for (let i = start; i < end; i++) {
      distanceM += haversineM(coordinates[i], coordinates[i + 1])
    }
    return {
      highway: seg.highway,
      surface: seg.surface,
      distanceM,
    }
  })
}

export interface AggregatedSegment {
  key: string
  distanceM: number
  percentage: number
}

/**
 * Aggregate sub-segments by highway type or surface.
 * Returns entries sorted by distance descending, with percentage of total.
 *
 * @param segments - Output of computeSubSegmentDistances
 * @param field    - 'highway' or 'surface'
 */
export function aggregateByField(
  segments: SubSegmentWithDistance[],
  field: 'highway' | 'surface',
): AggregatedSegment[] {
  const totals = new Map<string, number>()
  for (const seg of segments) {
    const key = seg[field] || 'unknown'
    totals.set(key, (totals.get(key) ?? 0) + seg.distanceM)
  }

  const totalDistanceM = [...totals.values()].reduce((a, b) => a + b, 0)

  return [...totals.entries()]
    .map(([key, distanceM]) => ({
      key,
      distanceM,
      percentage: totalDistanceM > 0 ? (distanceM / totalDistanceM) * 100 : 0,
    }))
    .sort((a, b) => b.distanceM - a.distanceM)
}
