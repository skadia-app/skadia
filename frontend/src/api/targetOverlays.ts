import { apiClient } from './client'

export type OverlaySubjectKind = 'course' | 'route_publication' | 'place' | 'region' | 'collection'
export type OverlayVisibility = 'private' | 'shared' | 'public'
export type TargetReportType = 'condition' | 'issue' | 'warning' | 'trail_work' | 'closure' | 'metadata'
export type TargetReportStatus = 'open' | 'resolved' | 'rejected' | 'superseded' | 'informational' | 'redacted'
export type TargetUserContentKind = 'comment' | 'rating' | 'save' | 'completion' | 'check_in'

export interface TargetOverlayQuery {
  subject_kind: OverlaySubjectKind
  subject_id: string
  target_version_id?: string
  limit?: number
  offset?: number
}

export interface TargetOverlaySummary {
  subject_kind: OverlaySubjectKind
  subject_id: string
  target_version_id?: string | null
  visibility_scope: 'public'
  public_open_report_count: number
  latest_report_at?: string
  current_condition_state?: string
  current_condition_severity?: number
  current_condition_observed_at?: string
  public_recent_observation_count_30d: number
  last_observation_at?: string
  public_comment_count: number
  public_rating_count: number
  public_rating_mean?: number
  public_completion_count: number
  public_check_in_count: number
  updated_at: string
}

export interface TargetActivityObservation {
  id: string
  subject_kind: OverlaySubjectKind
  subject_id: string
  target_version_id?: string | null
  activity_id?: string | null
  user_id?: string | null
  observation_type: 'matched_traversal' | 'visit' | 'dwell' | 'check_in' | 'completion'
  source_type: string
  source_id: string
  source_occurrence_key: string
  source_effort_id?: string | null
  stats?: Record<string, unknown>
  visibility: OverlayVisibility
  visibility_snapshot: string
  occurred_at: string
  created_at: string
}

export interface TargetReport {
  id: string
  subject_kind: OverlaySubjectKind
  subject_id: string
  target_version_id?: string | null
  author_user_id?: string | null
  report_type: TargetReportType
  visibility: OverlayVisibility
  access_scope_id?: string | null
  attribution_mode: 'named' | 'anonymous_public' | 'moderator_only' | 'hidden'
  condition_state?: string | null
  condition_severity?: number | null
  description: string
  observed_at: string
  valid_from?: string | null
  valid_until?: string | null
  moderation_state: string
  latest_status?: TargetReportStatus
  created_at: string
  updated_at: string
}

export interface TargetReportStatusEvent {
  id: string
  report_id: string
  actor_user_id?: string | null
  status: TargetReportStatus
  reason: string
  created_at: string
}

export interface TargetUserContent {
  id: string
  subject_kind: OverlaySubjectKind
  subject_id: string
  target_version_id?: string | null
  user_id?: string | null
  content_kind: TargetUserContentKind
  payload?: Record<string, unknown>
  visibility: OverlayVisibility
  moderation_state: string
  created_at: string
  updated_at: string
  deleted_at?: string | null
}

export type CreateTargetReportRequest = {
  subject_kind: OverlaySubjectKind
  subject_id: string
  target_version_id?: string
  report_type: TargetReportType
  visibility: OverlayVisibility
  access_scope_id?: string
  attribution_mode: TargetReport['attribution_mode']
  condition_state?: string
  condition_severity?: number
  description: string
  observed_at?: string
  valid_from?: string
  valid_until?: string
}

export type AddTargetReportStatusRequest = {
  status: TargetReportStatus
  reason?: string
}

export type CreateTargetUserContentRequest = {
  subject_kind: OverlaySubjectKind
  subject_id: string
  target_version_id?: string
  content_kind: TargetUserContentKind
  visibility: OverlayVisibility
  access_scope_id?: string
  payload?: Record<string, unknown>
}

export interface TargetOverlaySummaryResponse { summary: TargetOverlaySummary | null }
export interface TargetObservationsResponse { observations: TargetActivityObservation[] }
export interface TargetReportsResponse { reports: TargetReport[] }
export interface TargetReportResponse { report: TargetReport }
export interface TargetReportStatusResponse { status_event: TargetReportStatusEvent }
export interface TargetUserContentResponse { content: TargetUserContent[] }
export interface TargetUserContentSingleResponse { content: TargetUserContent }

export function getTargetOverlaySummary(params: Pick<TargetOverlayQuery, 'subject_kind' | 'subject_id' | 'target_version_id'>): Promise<TargetOverlaySummaryResponse> {
  return apiClient<TargetOverlaySummaryResponse>(withQuery('/target-overlays/summary', targetOverlayQuery(params)))
}

export const listTargetOverlaySummary = getTargetOverlaySummary

export function listTargetObservations(params: TargetOverlayQuery): Promise<TargetObservationsResponse> {
  return apiClient<TargetObservationsResponse>(withQuery('/target-overlays/observations', targetOverlayQuery(params)))
}

export function listTargetReports(params: TargetOverlayQuery & { type?: TargetReportType }): Promise<TargetReportsResponse> {
  return apiClient<TargetReportsResponse>(withQuery('/target-overlays/reports', targetOverlayQuery(params, { type: params.type })))
}

export function createTargetReport(request: CreateTargetReportRequest): Promise<TargetReportResponse> {
  return apiClient<TargetReportResponse>('/target-overlays/reports', { method: 'POST', body: request })
}

export function getTargetReport(id: string): Promise<TargetReportResponse> {
  return apiClient<TargetReportResponse>(`/target-overlays/reports/${id}`)
}

export function addTargetReportStatus(id: string, request: AddTargetReportStatusRequest): Promise<TargetReportStatusResponse> {
  return apiClient<TargetReportStatusResponse>(`/target-overlays/reports/${id}/status`, { method: 'POST', body: request })
}

export function listTargetUserContent(params: TargetOverlayQuery & { kind?: TargetUserContentKind }): Promise<TargetUserContentResponse> {
  return apiClient<TargetUserContentResponse>(withQuery('/target-overlays/content', targetOverlayQuery(params, { kind: params.kind })))
}

export function createTargetUserContent(request: CreateTargetUserContentRequest): Promise<TargetUserContentSingleResponse> {
  return apiClient<TargetUserContentSingleResponse>('/target-overlays/content', { method: 'POST', body: request })
}

export function deleteTargetUserContent(id: string): Promise<{ ok: boolean }> {
  return apiClient<{ ok: boolean }>(`/target-overlays/content/${id}`, { method: 'DELETE' })
}

function targetOverlayQuery(params: Partial<TargetOverlayQuery>, extra: Record<string, unknown> = {}): Record<string, string> {
  return cleanQuery({
    subject_kind: params.subject_kind,
    subject_id: params.subject_id,
    target_version_id: params.target_version_id,
    limit: params.limit,
    offset: params.offset,
    ...extra,
  })
}

function cleanQuery(params: object): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    out[key] = String(value)
  }
  return out
}

function withQuery(path: string, params: Record<string, string>): string {
  const query = new URLSearchParams(params)
  const raw = query.toString()
  return raw ? `${path}?${raw}` : path
}
