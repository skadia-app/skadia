import { apiClient, apiDownload } from './client'
import type { DetectedLap } from './types'
import type { SportPredicate } from '@/lib/sportPredicates'

export type SegmentVisibility = 'public' | 'private' | 'shared'
export type CourseKind = 'line' | 'checkpoint_sequence'
export type CheckpointControlRole = 'start' | 'checkpoint' | 'finish'
export type CheckpointControlPrimitive = 'point' | 'area'
export type CheckpointControlGeometryShape = 'point' | 'circle' | 'polygon' | 'multipolygon'

export interface CheckpointControl {
  id?: string
  sequence_index: number
  role: CheckpointControlRole
  primitive: CheckpointControlPrimitive
  geometry_shape: CheckpointControlGeometryShape
  label?: string
  geometry: unknown
  hit_radius_m?: number
  area_radius_m?: number
  required: boolean
}

export interface CheckpointControlSummary {
  control_count: number
  required_control_count: number
  primitive_kinds: string[]
  labels?: string[]
}

export interface CheckpointControlMapFeature {
  sequence_index: number
  role: CheckpointControlRole
  primitive: CheckpointControlPrimitive
  geometry_shape: CheckpointControlGeometryShape
  label?: string
  centroid: unknown
  geometry?: unknown
  hit_radius_m?: number
  area_radius_m?: number
  required: boolean
}

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
  visibility: SegmentVisibility
  access_scope_id?: string | null
  created_by: string
  created_at: string
  track_geojson?: string
  terrain_type?: string
  climb_type?: string
  elevation_kind?: 'flat' | 'hilly' | 'uphill' | 'downhill'
  /** null = legacy match-all inside the stored sport group; [] = none; entries = tuple predicates */
  allowed_predicates?: SportPredicate[] | null
  /** true = segment can be ridden in both directions */
  bidirectional: boolean
  course_kind?: CourseKind
  control_summary?: CheckpointControlSummary
  control_map_features?: CheckpointControlMapFeature[]
  controls?: CheckpointControl[]
}

export interface SegmentEffort {
  id: string
  segment_id: string
  segment_version_id?: string
  version_number?: number
  leaderboard_equivalence_group_id?: string
  activity_id: string
  started_at: string
  elapsed_time_s: number
  is_reverse: boolean
  sport_type: string
  sub_type?: string
  is_indoor?: boolean
  is_virtual?: boolean
  is_electric?: boolean
  rank_overall?: number
  is_personal_best: boolean
  personal_rank?: number
  is_segment_record: boolean
  segment_name?: string
  segment_type_meta?: Record<string, unknown>
  course_kind?: CourseKind
  control_summary?: CheckpointControlSummary
}

export interface LeaderboardEntry {
  id: string
  segment_id: string
  segment_version_id?: string
  version_number?: number
  leaderboard_equivalence_group_id?: string
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

export type SegmentLeaderboardQuery = {
  sport_type?: string
  is_reverse?: boolean
  scope?: 'current_version' | 'version' | 'equivalence_group' | 'sport_overall'
  electric_mode?: 'all' | 'exclude_electric' | 'only_electric'
  gear_id?: string
  only_mine?: boolean
  version_id?: string
  leaderboard_equivalence_group_id?: string
}

export type SegmentVersionValidityUpdate = {
  operation_id: string
  valid_from_at: string | null
  valid_until_at: string | null
  reason?: string
  recalculate: boolean
  equivalence_decision?: 'continue_group' | 'new_group' | 'version_specific'
}

export type SegmentGeometryUpdate = {
  operation_id: string
  track_geojson: unknown
  start_point_geojson: unknown
  end_point_geojson: unknown
  reason?: string
  recalculate: boolean
  equivalence_decision?: 'continue_group' | 'new_group' | 'version_specific'
  equivalence_evidence?: Record<string, unknown>
}

export type SegmentVersionRevision = {
  id: string
  segment_id: string
  operation: string
  reason?: string
  affected_from_at?: string
  affected_until_at?: string
  recalculation_run_id?: string
  rollback_of_revision_id?: string
  before_snapshot?: unknown
  after_snapshot?: unknown
  before_snapshot_sha256?: string
  after_snapshot_sha256?: string
  created_at: string
}

export type SegmentVersionRevisionSummary = Omit<
  SegmentVersionRevision,
  'before_snapshot' | 'after_snapshot' | 'before_snapshot_sha256' | 'after_snapshot_sha256'
> & {
  has_snapshots: boolean
}

export type SegmentVersionRevisionsResponse = {
  revisions: SegmentVersionRevisionSummary[]
  limit: number
  offset: number
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
  elevation_kind?: 'flat' | 'hilly' | 'uphill' | 'downhill'
  visibility?: SegmentVisibility
  access_scope_id?: string | null
  /** null = legacy match-all inside the stored sport group; [] = none; entries = tuple predicates */
  allowed_predicates?: SportPredicate[] | null
  /** nil on wire = use sport-group default (true for most groups) */
  bidirectional?: boolean
  course_kind?: CourseKind
  controls?: CheckpointControl[]
}

export interface CreateCheckpointCoursePayload {
  name: string
  sport_group: string
  description?: string
  visibility?: SegmentVisibility
  access_scope_id?: string | null
  allowed_predicates?: SportPredicate[] | null
  controls: CheckpointControl[]
  source_activity_id?: string
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
  elevation_kind?: 'flat' | 'hilly' | 'uphill' | 'downhill'
  /** comma-separated: "public", "private", "shared", or combinations */
  visibility?: string
  /** "any" = OR between SAC and MTB; "all" (default) = AND */
  difficulty_match?: 'any' | 'all'
  course_kind?: CourseKind
  course_kinds?: CourseKind[]
  include_control_map_features?: boolean
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
  if (params.elevation_kind) qs.set('elevation_kind', params.elevation_kind)
  if (params.visibility) qs.set('visibility', params.visibility)
  if (params.difficulty_match) qs.set('difficulty_match', params.difficulty_match)
  if (params.course_kind) qs.set('course_kind', params.course_kind)
  if (params.course_kinds?.length) qs.set('course_kinds', params.course_kinds.join(','))
  if (params.include_control_map_features) qs.set('include_control_map_features', 'true')
  
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
  elevation_kind?: 'flat' | 'hilly' | 'uphill' | 'downhill'
  visibility?: SegmentVisibility
  access_scope_id?: string | null
  /** absent = unchanged; null = legacy match-all inside the stored sport group; [] = no sports; entries = tuple predicates */
  allowed_predicates?: SportPredicate[] | null
  /** absent = unchanged */
  bidirectional?: boolean
}

export async function fetchSegmentLeaderboard(
  segmentId: string,
  isReverseOrQuery?: boolean | SegmentLeaderboardQuery,
  onlyMine?: boolean,
): Promise<LeaderboardEntry[]> {
  const qs = new URLSearchParams()
  if (typeof isReverseOrQuery === 'object' && isReverseOrQuery !== null) {
    const query = isReverseOrQuery
    if (query.sport_type) qs.set('sport_type', query.sport_type)
    if (query.is_reverse !== undefined) qs.set('is_reverse', String(query.is_reverse))
    if (query.scope) qs.set('scope', query.scope)
    if (query.electric_mode) qs.set('electric_mode', query.electric_mode)
    if (query.gear_id) qs.set('gear_id', query.gear_id)
    if (query.only_mine) qs.set('only_mine', 'true')
    if (query.version_id) qs.set('version_id', query.version_id)
    if (query.leaderboard_equivalence_group_id) {
      qs.set('leaderboard_equivalence_group_id', query.leaderboard_equivalence_group_id)
    }
  } else {
    if (isReverseOrQuery !== undefined) qs.set('is_reverse', String(isReverseOrQuery))
    if (onlyMine) qs.set('only_mine', 'true')
  }
  const q = qs.toString()
  return apiClient<LeaderboardEntry[]>(`/segments/${segmentId}/leaderboard${q ? `?${q}` : ''}`)
}

export async function fetchSegment(id: string, opts: { includeGeoJSON?: boolean; includeControls?: boolean } = {}): Promise<Segment> {
  const qs = new URLSearchParams()
  if (opts.includeGeoJSON) qs.set('include_geojson', 'true')
  if (opts.includeControls) qs.set('include_controls', 'true')
  const query = qs.toString()
  return apiClient<Segment>(`/segments/${id}${query ? `?${query}` : ''}`)
}

export async function updateSegment(id: string, payload: UpdateSegmentPayload): Promise<Segment> {
  return apiClient<Segment>(`/segments/${id}`, { method: 'PUT', body: payload })
}

export async function createSegment(payload: CreateSegmentPayload): Promise<Segment> {
  return apiClient<Segment>('/segments', { method: 'POST', body: payload })
}

export async function createCheckpointCourse(payload: CreateCheckpointCoursePayload): Promise<Segment> {
  return createSegment({ ...payload, course_kind: 'checkpoint_sequence', segment_type: 'standard', geojson_track: '' })
}

export async function importCheckpointCourseGeoJSON(payload: unknown): Promise<Segment> {
  return apiClient<Segment>('/segments/import', { method: 'POST', body: payload })
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

export type SegmentRecalculationRunResponse = {
  status: string
  run_id?: string
  total_activities?: number
}

export async function recomputeSegment(segmentId: string): Promise<SegmentRecalculationRunResponse> {
  return apiClient<SegmentRecalculationRunResponse>(`/segments/${segmentId}/recompute`, {
    method: 'POST',
    body: { operation_id: crypto.randomUUID() },
  })
}

export async function publishSegment(id: string): Promise<SegmentRecalculationRunResponse> {
  return apiClient<SegmentRecalculationRunResponse>(`/segments/${id}/publish`, {
    method: 'POST',
    body: { operation_id: crypto.randomUUID() },
  })
}

export async function updateSegmentVersionValidity(
  segmentId: string,
  versionId: string,
  payload: SegmentVersionValidityUpdate,
): Promise<SegmentVersionRevision> {
  return apiClient<SegmentVersionRevision>(`/segments/${segmentId}/versions/${versionId}/validity`, {
    method: 'PATCH',
    body: payload,
  })
}

export async function updateSegmentVersionGeometry(
  segmentId: string,
  versionId: string,
  payload: SegmentGeometryUpdate,
): Promise<SegmentVersionRevision> {
  return apiClient<SegmentVersionRevision>(`/segments/${segmentId}/versions/${versionId}/geometry`, {
    method: 'PATCH',
    body: payload,
  })
}

export async function fetchSegmentVersionRevisions(
  segmentId: string,
  opts: { limit?: number; offset?: number } = {},
): Promise<SegmentVersionRevisionsResponse> {
  const qs = new URLSearchParams()
  if (opts.limit !== undefined) qs.set('limit', String(opts.limit))
  if (opts.offset !== undefined) qs.set('offset', String(opts.offset))
  const q = qs.toString()
  return apiClient<SegmentVersionRevisionsResponse>(`/segments/${segmentId}/versions/revisions${q ? `?${q}` : ''}`)
}

export async function fetchSegmentVersionRevision(
  segmentId: string,
  revisionId: string,
): Promise<SegmentVersionRevision> {
  return apiClient<SegmentVersionRevision>(`/segments/${segmentId}/versions/revisions/${revisionId}`)
}

export async function rollbackSegmentVersionRevision(
  segmentId: string,
  revisionId: string,
  reason?: string,
): Promise<SegmentVersionRevision> {
  return apiClient<SegmentVersionRevision>(`/segments/${segmentId}/versions/revisions/${revisionId}/rollback`, {
    method: 'POST',
    body: { reason },
  })
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
