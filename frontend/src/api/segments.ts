import { apiClient, apiDownload } from './client'
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
  is_favorite: boolean
  visibility: 'public' | 'private'
  created_by: string
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
  id: string
  segment_id: string
  activity_id: string
  user_id: string
  elapsed_time_s: number
  is_reverse: boolean
  sport_type: string
  rank_overall?: number
  is_personal_best: boolean
  is_segment_record: boolean
  started_at: string
  display_name: string
  gear_name?: string
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
  visibility?: 'public' | 'private'
}

export interface SegmentsResponse {
  segments: Segment[]
  next_cursor?: string
  has_more: boolean
}

export async function fetchSegments(params: {
  sport_group?: string
  segment_type?: string
  segment_types?: string
  search?: string
  bbox?: string
  only_ridden?: boolean
  include_geojson?: boolean
  cursor?: string
  limit?: number
} = {}): Promise<SegmentsResponse> {
  const qs = new URLSearchParams()
  if (params.sport_group) qs.set('sport_group', params.sport_group)
  if (params.segment_type) qs.set('segment_type', params.segment_type)
  if (params.segment_types) qs.set('segment_types', params.segment_types)
  if (params.search) qs.set('search', params.search)
  if (params.bbox) qs.set('bbox', params.bbox)
  if (params.only_ridden) qs.set('only_ridden', 'true')
  if (params.include_geojson) qs.set('include_geojson', 'true')
  if (params.cursor) qs.set('cursor', params.cursor)
  if (params.limit !== undefined) qs.set('limit', String(params.limit))
  const q = qs.toString()
  return apiClient<SegmentsResponse>(`/segments${q ? `?${q}` : ''}`)
}

export interface UpdateSegmentPayload {
  name?: string
  description?: string
  segment_type_meta?: Record<string, unknown>
  new_version?: boolean
  terrain_type?: string
  climb_type?: string
  visibility?: 'public' | 'private'
}

export async function fetchSegmentLeaderboard(
  segmentId: string,
  isReverse?: boolean,
  onlyMine?: boolean,
): Promise<LeaderboardEntry[]> {
  const qs = new URLSearchParams()
  if (isReverse !== undefined) qs.set('is_reverse', String(isReverse))
  if (onlyMine) qs.set('only_mine', 'true')
  const q = qs.toString()
  return apiClient<LeaderboardEntry[]>(`/segments/${segmentId}/leaderboard${q ? `?${q}` : ''}`)
}

export async function fetchSegment(id: string): Promise<Segment> {
  return apiClient<Segment>(`/segments/${id}`)
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

export async function publishSegment(id: string): Promise<{ status: string; job_id: string }> {
  return apiClient<{ status: string; job_id: string }>(`/segments/${id}/publish`, { method: 'POST' })
}

export async function exportSegment(id: string, format: 'gpx' | 'geojson'): Promise<{ blob: Blob; filename?: string }> {
  return apiDownload(`/segments/${id}/export?format=${format}`)
}
