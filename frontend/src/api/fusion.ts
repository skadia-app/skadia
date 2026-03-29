import { apiClient } from './client'

export interface FusionCandidate {
  id: string
  title: string | null
  started_at: string
  distance_m: number
  sport_type: string
  has_hr: boolean
  has_power: boolean
  has_gps: boolean
  has_temperature: boolean
  has_cadence: boolean
  has_altitude: boolean
  overlap_pct: number
  timezone?: string | null
}

export interface GPSFusionConfig {
  primary_source: string
  extend_track: boolean
}

export interface FusionMergeRequest {
  sources: string[]
  field_map: Record<string, string>  // field → source UUID or "fuse"
  gps_fusion_config?: GPSFusionConfig
  title_source?: string
  tags_merge: boolean
  gear_source?: string
}

export async function fetchFusionCandidates(activityId: string): Promise<FusionCandidate[]> {
  return apiClient<FusionCandidate[]>(`/activities/${activityId}/fusion-candidates`)
}

export interface FusionMergeResponse {
  id: string  // ID der neuen merged Aktivität
}

export async function executeFusionMerge(
  targetId: string,
  req: FusionMergeRequest,
): Promise<FusionMergeResponse> {
  return apiClient<FusionMergeResponse>(`/activities/${targetId}/fusion-merge`, {
    method: 'POST',
    body: req,
  })
}

export async function revertFusion(activityId: string): Promise<void> {
  return apiClient<void>(`/activities/${activityId}/revert-fusion`, { method: 'POST' })
}
