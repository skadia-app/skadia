import { apiClient } from './client'

export interface BestEffortRanked {
  id: string
  category: string
  sport_group: string
  elapsed_time_s?: number
  value_numeric?: number
  achieved_at: string
  rank: number
}

export interface NearMiss {
  category: string
  sport_group: string
  diff_seconds?: number
  diff_watts?: number
  pr_time_s?: number
  pr_watts?: number
}

export interface ActivityBestEffortsResponse {
  best_efforts: BestEffortRanked[]
  near_misses: NearMiss[]
}

export interface PersonalRecordsParams {
  sport_group?: string
  year?: number
}

export interface PersonalRecord {
  id: string
  category: string
  sport_group: string
  elapsed_time_s?: number
  value_numeric?: number
  achieved_at: string
  activity_id: string
}

export interface PersonalRecordsResponse {
  personal_records: PersonalRecord[]
}

export async function fetchActivityBestEfforts(activityId: string): Promise<ActivityBestEffortsResponse> {
  return apiClient<ActivityBestEffortsResponse>(`/activities/${activityId}/best-efforts`)
}

export async function fetchPersonalRecords(params: PersonalRecordsParams = {}): Promise<PersonalRecordsResponse> {
  const searchParams = new URLSearchParams()
  if (params.sport_group) searchParams.set('sport_group', params.sport_group)
  if (params.year != null) searchParams.set('year', String(params.year))
  const qs = searchParams.toString()
  return apiClient<PersonalRecordsResponse>(`/personal-records${qs ? `?${qs}` : ''}`)
}

export async function addExclusion(activityId: string, category: string): Promise<void> {
  return apiClient<void>('/best-effort-exclusions', {
    method: 'POST',
    body: { activity_id: activityId, category },
  })
}

export async function removeExclusion(activityId: string, category: string): Promise<void> {
  return apiClient<void>(`/best-effort-exclusions/${activityId}/${category}`, {
    method: 'DELETE',
  })
}
