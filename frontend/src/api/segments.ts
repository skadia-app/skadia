import { apiClient, apiDownload } from './client'
import type { DetectedLap } from './types'
import type { SportPredicate } from '@/lib/sportPredicates'

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
  /** null = legacy match-all inside the stored sport group; [] = none; entries = tuple predicates */
  allowed_predicates?: SportPredicate[] | null
  /** true = segment can be ridden in both directions */
  bidirectional: boolean
}

export interface SegmentEffort {
  id: string
  segment_id: string
  activity_id: string
  started_at: string
  elapsed_time_s: number
  is_reverse: boolean
  sport_type: string
  sub_type?: string
  rank_overall?: number
  is_personal_best: boolean
  personal_rank?: number
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
  /** null = legacy match-all inside the stored sport group; [] = none; entries = tuple predicates */
  allowed_predicates?: SportPredicate[] | null
  /** nil on wire = use sport-group default (true for most groups) */
  bidirectional?: boolean
}

export interface SegmentsResponse {
  segments: Segment[]
  next_cursor?: string
  has_more: boolean
}

export interface FetchSegmentsOpts {
  sport_group?: string
  segment_type?: string
  segment_types?: string
  search?: string
  bbox?: string
  lifts_only_ridden?: boolean
  completed_only?: boolean
  include_geojson?: boolean
  cursor?: string
  limit?: number
  sac_scale?: string
  mtb_scale?: string
  surface?: string
  /** comma-separated: "public", "private", or "public,private" */
  visibility?: string
  /** "any" = OR between SAC and MTB; "all" (default) = AND */
  difficulty_match?: 'any' | 'all'
}

export async function fetchSegments(params: FetchSegmentsOpts = {}): Promise<SegmentsResponse> {
  const qs = new URLSearchParams()
  if (params.sport_group) qs.set('sport_group', params.sport_group)
  if (params.segment_type) qs.set('segment_type', params.segment_type)
  if (params.segment_types) qs.set('segment_types', params.segment_types)
  if (params.search) qs.set('search', params.search)
  if (params.bbox) qs.set('bbox', params.bbox)
  if (params.lifts_only_ridden) qs.set('lifts_only_ridden', 'true')
  if (params.completed_only) qs.set('completed_only', 'true')
  if (params.include_geojson) qs.set('include_geojson', 'true')
  if (params.cursor) qs.set('cursor', params.cursor)
  if (params.limit !== undefined) qs.set('limit', String(params.limit))
  if (params.sac_scale) qs.set('sac_scale', params.sac_scale)
  if (params.mtb_scale) qs.set('mtb_scale', params.mtb_scale)
  if (params.surface) qs.set('surface', params.surface)
  if (params.visibility) qs.set('visibility', params.visibility)
  if (params.difficulty_match) qs.set('difficulty_match', params.difficulty_match)
  
  const q = qs.toString()
  return apiClient<SegmentsResponse>(`/segments${q ? `?${q}` : ''}`)
}

export interface UpdateSegmentPayload {
  name?: string
  description?: string
  segment_type?: string
  segment_type_meta?: Record<string, unknown>
  new_version?: boolean
  terrain_type?: string
  climb_type?: string
  visibility?: 'public' | 'private'
  /** absent = unchanged; null = legacy match-all inside the stored sport group; [] = no sports; entries = tuple predicates */
  allowed_predicates?: SportPredicate[] | null
  /** absent = unchanged */
  bidirectional?: boolean
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

export async function fetchSegment(id: string, opts: { includeGeoJSON?: boolean } = {}): Promise<Segment> {
  const qs = opts.includeGeoJSON ? '?include_geojson=true' : ''
  return apiClient<Segment>(`/segments/${id}${qs}`)
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

export async function deleteSegments(ids: string[]): Promise<{ deleted: number }> {
  return apiClient<{ deleted: number }>('/segments', {
    method: 'DELETE',
    body: { ids },
  })
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

export async function exportSegments(ids: string[], format: 'gpx' | 'geojson'): Promise<{ blob: Blob; filename?: string }> {
  return apiDownload('/segments/export', { method: 'POST', body: { ids, format } })
}

export interface SegmentPRsResponse {
  efforts: SegmentEffort[]
  count: number
}

export async function fetchSegmentPRs(): Promise<SegmentPRsResponse> {
  return apiClient<SegmentPRsResponse>('/segment-prs')
}

export interface GlobalTopSegmentEffort extends SegmentEffort {
  athlete_count: number
}

export interface GlobalTopSegmentEffortsResponse {
  efforts: GlobalTopSegmentEffort[]
  count: number
}

export async function fetchGlobalTopSegmentPRs(): Promise<GlobalTopSegmentEffortsResponse> {
  return apiClient<GlobalTopSegmentEffortsResponse>('/segment-prs/global-top')
}

// ---- Enrichment (spec §4.7) --------------------------------------------------

export interface SegmentEnrichmentRequest {
  coordinates: Array<[number, number]>  // [lng, lat]
  seg_type: 'standard' | 'trail' | 'lap' | 'wintersport' | 'wassersport' | 'lift'
  dominant_sport?: string
}

export interface SurfaceSuggestion {
  dominant: string
  distribution: Record<string, number>
  mixed: boolean
}

export interface ScaleSuggestion {
  dominant: string                      // e.g. "T3" or "S2"
  max: string                           // highest scale present
  distribution: Record<string, number>
}

export interface SegmentEnrichmentResponse {
  surface:   SurfaceSuggestion | null
  sac_scale: ScaleSuggestion   | null
  mtb_scale: ScaleSuggestion   | null
  coverage_pct: number                  // 0..1
  reason?: 'wintersport' | 'wassersport' | 'low_coverage' | 'no_graph' | 'no_data'
}

export async function suggestSegmentEnrichment(
  req: SegmentEnrichmentRequest,
): Promise<SegmentEnrichmentResponse> {
  return apiClient<SegmentEnrichmentResponse>('/segments/enrichment-suggest', {
    method: 'POST',
    body: req,
  })
}

// ---- Reverse (spec §4.11) ----------------------------------------------------

export interface ReverseSegmentResponse {
  efforts_updated: number
}

export async function reverseSegment(segmentID: string): Promise<ReverseSegmentResponse> {
  return apiClient<ReverseSegmentResponse>(`/segments/${encodeURIComponent(segmentID)}/reverse`, {
    method: 'POST',
  })
}
