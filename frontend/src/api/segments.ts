import { apiClient } from './client'
import type { DetectedLap } from './types'

export interface Segment {
  id: string
  name: string
  description?: string
  sport_group: string
  segment_type: string
  segment_type_meta: Record<string, unknown>
  distance_m?: number
  elevation_gain_m?: number
  effort_count: number
  athlete_count: number
  ridden_count: number
  visibility: string
  created_at: string
  track_geojson?: string
  terrain_type?: string
  climb_type?: string
}

export interface SegmentEffort {
  id: string
  segment_id: string
  activity_id: string
  elapsed_time_s: number
  is_reverse: boolean
  sport_type: string
  rank_overall?: number
  is_personal_best: boolean
  is_segment_record: boolean
  segment_name?: string
  segment_type_meta?: Record<string, unknown>
}

export interface LeaderboardEntry {
  rank: number
  athlete_name: string
  elapsed_time_s: number
  activity_id: string
  started_at: string
  gear_name?: string
  is_personal_best: boolean
  is_segment_record: boolean
}

export interface CreateSegmentPayload {
  name: string
  sport_group: string
  segment_type: string
  segment_type_meta?: Record<string, unknown>
  geojson_track: string
  source_activity_id?: string
  description?: string
  terrain_type?: string
  climb_type?: string
}

export async function fetchSegments(params?: {
  sport_group?: string
  segment_type?: string
  search?: string
  bbox?: string
}): Promise<Segment[]> {
  const qs = new URLSearchParams()
  if (params?.sport_group) qs.set('sport_group', params.sport_group)
  if (params?.segment_type) qs.set('segment_type', params.segment_type)
  if (params?.search) qs.set('search', params.search)
  if (params?.bbox) qs.set('bbox', params.bbox)
  const q = qs.toString()
  return apiClient<Segment[]>(`/segments${q ? `?${q}` : ''}`)
}

export interface UpdateSegmentPayload {
  name?: string
  description?: string
  sport_group?: string
  segment_type_meta?: Record<string, unknown>
  new_version?: boolean
  terrain_type?: string
  climb_type?: string
}

export async function fetchSegmentLeaderboard(
  segmentId: string,
  isReverse?: boolean,
): Promise<LeaderboardEntry[]> {
  const qs = isReverse !== undefined ? `?is_reverse=${isReverse}` : ''
  return apiClient<LeaderboardEntry[]>(`/segments/${segmentId}/leaderboard${qs}`)
}

export async function updateSegment(id: string, payload: UpdateSegmentPayload): Promise<Segment> {
  return apiClient<Segment>(`/segments/${id}`, { method: 'PUT', body: payload })
}

export async function createSegment(payload: CreateSegmentPayload): Promise<Segment> {
  return apiClient<Segment>('/segments', { method: 'POST', body: payload })
}

export async function deleteSegment(id: string): Promise<void> {
  return apiClient<void>(`/segments/${id}`, { method: 'DELETE' })
}

export async function fetchActivitySegments(activityId: string): Promise<SegmentEffort[]> {
  return apiClient<SegmentEffort[]>(`/activities/${activityId}/segments`)
}

export async function fetchDetectedLaps(activityId: string): Promise<DetectedLap[]> {
  return apiClient<DetectedLap[]>(`/activities/${activityId}/detected-laps`)
}

export async function recomputeSegment(segmentId: string): Promise<void> {
  return apiClient<void>(`/segments/${segmentId}/recompute`, { method: 'POST' })
}
