import { apiClient } from './client'
import type { ActivitiesResponse, Activity, StreamPoint, Sensor, ZoneDistribution, PowerCurveResponse, Lap, Split, SplitSummary, SwimLength, ClimbProEvent } from './types'

export interface ActivityFilterParams {
  sportTypes?: string[]
  dateFrom?: string
  dateTo?: string
  isIndoor?: boolean | null
  hasGPS?: boolean | null
  gearId?: string
  search?: string
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

export async function reprocessActivity(id: string): Promise<Activity> {
  return apiClient<Activity>(`/activities/${id}/recompute`, { method: 'POST' })
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

export type ExportFormat = 'original' | 'gpx' | 'tcx' | 'fit'

export function getActivityExportUrl(id: string, format: ExportFormat): string {
  return `/activities/${id}/export?format=${format}`
}

export async function getZoneDistribution(id: string): Promise<ZoneDistribution> {
  return apiClient<ZoneDistribution>(`/activities/${id}/zone-distribution`)
}

export async function getPowerCurve(period: 'ytd' | '90d' | 'all' = 'ytd'): Promise<PowerCurveResponse> {
  return apiClient<PowerCurveResponse>(`/stats/power-curve?period=${period}`)
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
