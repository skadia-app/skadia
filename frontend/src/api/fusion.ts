import { apiClient } from './client'
import type { FusionMergeRequest } from './types'

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
  source_device?: string | null
  source?: string | null
  original_file_format?: string | null
}

export async function fetchFusionCandidates(activityId: string): Promise<FusionCandidate[]> {
  return apiClient<FusionCandidate[]>(`/activities/${activityId}/fusion-candidates`)
}

export async function revertFusion(activityId: string): Promise<void> {
  return apiClient<void>(`/activities/${activityId}/revert-fusion`, { method: 'POST' })
}

export interface FusionMergeResponse {
  id: string
  /** Non-fatal backend warnings (gear copy failed, title nil, etc.).
   *  Absent or empty means no warnings. Read defensively — older servers
   *  always omit this field. */
  warnings?: string[]
}

export async function executeFusionMerge(req: FusionMergeRequest): Promise<FusionMergeResponse> {
  return apiClient<FusionMergeResponse>('/fusion-merge', {
    method: 'POST',
    body: req,
  })
}
