import { useQuery } from '@tanstack/react-query'
import { apiClient } from './client'
import type { EnrichmentSettings } from './types'
import type { CoordRange } from './routes'

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
  summits: TrackSummaryGeoTagItem[] | null
  waters: TrackSummaryWaterItem[] | null
  settlements: TrackSummaryGeoTagItem[] | null
  settings: EnrichmentSettings
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
