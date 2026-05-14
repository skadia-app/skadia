import { useQuery } from '@tanstack/react-query'
import { apiClient } from './client'
import type { CoordRange } from './routes'
import type { CapabilityFeatureState } from './routes'

export interface TrackSummarySurfaceItem {
  type: string
  percentage: number
  length_m: number
}

export interface TrackSummaryGeoTagItem {
  id: string
  category: string
  name: string
  osm_id: number | null
  lat: number | null
  lng: number | null
  ele_m: number | null
  dist_m: number | null
  track_pos: number | null
}

export interface TrackSummaryWaterItem extends TrackSummaryGeoTagItem {
  kind: 'still' | 'flowing'
}

export interface ActivityTrackSummary {
  forest_pct: number | null
  protected_pct: number | null
  forest_ranges: CoordRange[] | null
  protected_ranges: CoordRange[] | null
  sac_ranges: Record<string, CoordRange[]> | null
  mtb_ranges: Record<string, CoordRange[]> | null
  vf_ranges: Record<string, CoordRange[]> | null
  vf_unknown_ranges: CoordRange[] | null
  surfaces: TrackSummarySurfaceItem[] | null
  highways: TrackSummarySurfaceItem[] | null
  // Map raw OSM type → coordinate-index ranges (over the resampled track at
  // ~10 m spacing among GPS-valid stream points). Null when no per-segment
  // ranges were stored (older data, before the ranges-backfill).
  surface_ranges: Record<string, CoordRange[]> | null
  highway_ranges: Record<string, CoordRange[]> | null
  summits: TrackSummaryGeoTagItem[] | null
  waters: TrackSummaryWaterItem[] | null
  settlements: TrackSummaryGeoTagItem[] | null
  // Whether the enrichment used the graph-snap pipeline (OSM tile coverage
  // required). False or absent means the SQL-fallback path was used and
  // surface_ranges / highway_ranges will be null — map highlight is unavailable.
  // Added by backend R1-B wave; optional so older responses remain valid.
  graph_snap_used?: boolean
  // Resampling interval in metres used when building surface_ranges / highway_ranges.
  // Absent in older responses; fall back to the frontend constant RESAMPLE_SPACING_M.
  // Added by backend R1-B wave.
  resample_spacing_m?: number
  feature_capabilities?: Record<string, CapabilityFeatureState | undefined>
}

export function fetchActivityTrackSummary(activityId: string): Promise<ActivityTrackSummary> {
  return apiClient<ActivityTrackSummary>(`/activities/${activityId}/track-summary`)
}

export function useActivityTrackSummary(activityId: string | null | undefined, enabled = true) {
  return useQuery({
    queryKey: ['activity-track-summary', activityId],
    queryFn: () => fetchActivityTrackSummary(activityId!),
    enabled: enabled && !!activityId,
    staleTime: 24 * 60 * 60 * 1000,
  })
}
