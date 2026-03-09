import { apiClient } from './client'
import { getToken, clearToken } from '@/lib/auth'
import { ApiError } from './client'
import type { ActivitiesResponse, Activity, StreamPoint, Sensor, ZoneDistribution, PowerCurveResponse, Lap, Split, SplitSummary, SwimLength, ClimbProEvent } from './types'

export interface ActivityFilterParams {
  sportTypes?: string[]
  dateFrom?: string
  dateTo?: string
  isIndoor?: boolean | null
  hasGPS?: boolean | null
  gearId?: string
  search?: string
  includeTags?: string[]
  excludeTags?: string[]
  includeGear?: string[]
  excludeGear?: string[]
}

export async function fetchActivities(
  cursor?: string,
  limit = 20,
  sortBy?: string,
  sortDir?: string,
  filters?: ActivityFilterParams,
): Promise<ActivitiesResponse> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('cursor', cursor)
  if (sortBy) params.set('sort_by', sortBy)
  if (sortDir) params.set('sort_dir', sortDir)

  if (filters?.sportTypes?.length) {
    const sports = new Set<string>()
    const sportsWithBareKey = new Set<string>()
    const subs = new Set<string>()
    for (const key of filters.sportTypes) {
      const colonIdx = key.indexOf(':')
      if (colonIdx === -1) {
        // Bare key — sport selected without sub-type restriction
        sports.add(key)
        sportsWithBareKey.add(key)
      } else {
        const sport = key.slice(0, colonIdx)
        const sub = key.slice(colonIdx + 1)
        sports.add(sport)
        subs.add(sub)
      }
    }
    if (sports.size) params.set('sport_type', [...sports].join(','))
    // Only send sub_type filter when every selected sport carries an explicit sub-type.
    // If any sport was selected without a sub-type (bare key), omitting sub_type ensures
    // those activities (e.g. hiking, mountaineering with NULL sub_type) are not excluded
    // by the backend AND-conjunction of sport_type and sub_type conditions.
    if (subs.size > 0 && sportsWithBareKey.size === 0) {
      params.set('sub_type', [...subs].join(','))
    }
  }
  if (filters?.dateFrom) params.set('date_from', filters.dateFrom)
  if (filters?.dateTo) params.set('date_to', filters.dateTo)
  if (filters?.isIndoor !== null && filters?.isIndoor !== undefined) params.set('is_indoor', String(filters.isIndoor))
  if (filters?.hasGPS !== null && filters?.hasGPS !== undefined) params.set('has_gps', String(filters.hasGPS))
  if (filters?.gearId) params.set('gear_id', filters.gearId)
  if (filters?.search) params.set('search', filters.search)
  if (filters?.includeTags?.length) params.set('include_tags', filters.includeTags.join(','))
  if (filters?.excludeTags?.length) params.set('exclude_tags', filters.excludeTags.join(','))
  if (filters?.includeGear?.length) params.set('include_gear', filters.includeGear.join(','))
  if (filters?.excludeGear?.length) params.set('exclude_gear', filters.excludeGear.join(','))

  return apiClient<ActivitiesResponse>(`/activities?${params}`)
}

export async function fetchActivity(id: string): Promise<Activity> {
  return apiClient<Activity>(`/activities/${id}`)
}

export async function fetchActivityStreams(id: string): Promise<StreamPoint[]> {
  return apiClient<StreamPoint[]>(`/activities/${id}/streams`)
}

export async function fetchActivitySensors(id: string): Promise<Sensor[]> {
  return apiClient<Sensor[]>(`/activities/${id}/sensors`)
}

export async function updateActivity(id: string, data: Partial<Pick<Activity, 'title' | 'description' | 'sport_type' | 'sub_type' | 'tags' | 'visibility'>>): Promise<Activity> {
  return apiClient<Activity>(`/activities/${id}`, { method: 'PATCH', body: data })
}

export async function reprocessActivity(id: string): Promise<Activity | { job_id: string }> {
  return apiClient<Activity | { job_id: string }>(`/activities/${id}/recompute`, { method: 'POST' })
}

export interface ReprocessFilter {
  sport_types?: string[]
  format?: string
  before?: string
  after?: string
  ids?: string[]
  scope?: string[]
}

export interface ReprocessResult {
  status?: 'already_running'
  processed: number
  errors: number
  total: number
}

export async function reprocessAllActivities(filter?: ReprocessFilter): Promise<ReprocessResult> {
  return apiClient<ReprocessResult>('/activities/recompute-all', {
    method: 'POST',
    body: filter ?? undefined,
  })
}

export type ExportFormat = 'original' | 'gpx' | 'tcx' | 'fit' | 'zip'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

/**
 * Returns the full URL for a single-activity export endpoint.
 * The URL includes the /api/v1 prefix so it can be used directly as an
 * <a href> or passed to apiDownload() without double-prefixing.
 */
export function getActivityExportUrl(id: string, format: ExportFormat): string {
  return `${API_BASE_URL}/activities/${id}/export?format=${format}`
}

export async function getZoneDistribution(id: string): Promise<ZoneDistribution> {
  return apiClient<ZoneDistribution>(`/activities/${id}/zone-distribution`)
}

export interface PDCParams {
  period?: 'ytd' | '90d' | 'all'
  from?: string   // YYYY-MM-DD
  to?: string     // YYYY-MM-DD
  sport_type?: string
}

export async function getPowerCurve(params: PDCParams = {}): Promise<PowerCurveResponse> {
  const searchParams = new URLSearchParams()
  if (params.from && params.to) {
    searchParams.set('from', params.from)
    searchParams.set('to', params.to)
  } else if (params.period) {
    searchParams.set('period', params.period)
  } else {
    searchParams.set('period', 'ytd')
  }
  if (params.sport_type) searchParams.set('sport_type', params.sport_type)
  return apiClient<PowerCurveResponse>(`/stats/power-curve?${searchParams}`)
}

export async function fetchActivityLaps(id: string): Promise<Lap[]> {
  return apiClient<Lap[]>(`/activities/${id}/laps`)
}

export async function fetchActivitySplits(id: string): Promise<{ splits: Split[]; summaries: SplitSummary[] }> {
  return apiClient<{ splits: Split[]; summaries: SplitSummary[] }>(`/activities/${id}/splits`)
}

export async function fetchActivityLengths(id: string): Promise<SwimLength[]> {
  return apiClient<SwimLength[]>(`/activities/${id}/lengths`)
}

export async function fetchActivityClimbPro(id: string): Promise<ClimbProEvent[]> {
  return apiClient<ClimbProEvent[]>(`/activities/${id}/climb-pro`)
}

// ── Non-destructive edit operations ───────────────────────────────────────────

export interface TrimRequest {
  trim_start?: string // RFC3339
  trim_end?: string   // RFC3339
}

export async function trimActivity(id: string, req: TrimRequest): Promise<Activity> {
  return apiClient<Activity>(`/activities/${id}/trim`, { method: 'POST', body: req })
}

export interface SplitRequest {
  split_at: string // RFC3339
}

export interface SplitResponse {
  activity_id: string
  new_activity_id: string
}

export async function splitActivity(id: string, req: SplitRequest): Promise<SplitResponse> {
  return apiClient<SplitResponse>(`/activities/${id}/split`, { method: 'POST', body: req })
}

export interface MergeRequest {
  source_id: string
  fields: string[]
}

export async function mergeActivities(id: string, req: MergeRequest): Promise<Activity> {
  return apiClient<Activity>(`/activities/${id}/merge`, { method: 'POST', body: req })
}

// ── Delete ────────────────────────────────────────────────────────────────────

/** Delete a single activity. */
export async function deleteActivity(id: string, sources?: 'restore' | 'delete'): Promise<void> {
  const url = sources ? `/activities/${id}?sources=${sources}` : `/activities/${id}`
  await apiClient<void>(url, { method: 'DELETE' })
}

/** Bulk-delete multiple activities. Returns the number deleted. */
export async function deleteActivities(ids: string[]): Promise<{ deleted: number }> {
  return apiClient<{ deleted: number }>('/activities', {
    method: 'DELETE',
    body: { ids },
  })
}

// ── ZIP Export ────────────────────────────────────────────────────────────────

/**
 * Downloads multiple activities as a ZIP archive.
 * @param ids     Activity IDs to export (empty = all activities of the user)
 * @param format  'original' | 'gpx' | 'tcx' | 'fit'
 */
export async function downloadActivitiesZip(
  ids: string[],
  format: ExportFormat = 'original',
): Promise<void> {
  const token = getToken()
  const url = `${API_BASE_URL}/activities/export-zip`
  const res = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
    },
    credentials: 'include',
    body: JSON.stringify({ ids, format }),
  })
  if (res.status === 401) {
    clearToken()
    window.location.href = '/login'
    throw new ApiError(401, 'Unauthorized')
  }
  if (!res.ok) throw new ApiError(res.status, await res.text())
  const blob = await res.blob()
  const cd = res.headers.get('Content-Disposition') ?? ''
  const match = cd.match(/filename[*]?=(?:UTF-8'')?["']?([^"';\r\n]+)["']?/i)
  const filename = match?.[1] ? decodeURIComponent(match[1]) : `skadia-export.zip`
  const objectUrl = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = objectUrl
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // Delay revoking the object URL so the browser has time to initiate the download.
  // Revoking synchronously after click() can race the browser's download initiation
  // and silently abort the download in Firefox and some Chromium versions.
  setTimeout(() => URL.revokeObjectURL(objectUrl), 10_000)
}
