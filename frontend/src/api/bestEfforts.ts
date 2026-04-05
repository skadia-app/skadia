import { apiClient } from './client'

// ── Types ──────────────────────────────────────────────────────────────────

/** A single best effort record with a rank among the user's personal bests. */
export interface BestEffortRanked {
  id: string
  user_id?: string
  activity_id: string
  category: string
  sport_group: string
  elapsed_time_s?: number | null
  value_numeric?: number | null
  achieved_at: string
  stream_start_idx?: number | null
  stream_end_idx?: number | null
  rank: number
  suspicious?: boolean
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
  year?: number | string
}

export interface BestEffortExclusion {
  activity_id: string
  category: string
  created_at: string
}

// ── API functions ──────────────────────────────────────────────────────────

export async function fetchActivityBestEfforts(activityId: string): Promise<ActivityBestEffortsResponse> {
  return apiClient<ActivityBestEffortsResponse>(`/activities/${activityId}/best-efforts`)
}

/**
 * Fetch top-3 personal records for the authenticated user.
 * Backend: GET /api/v1/personal-records — returns BestEffortRanked[]
 */
export async function fetchPersonalRecords(params: PersonalRecordsParams = {}): Promise<BestEffortRanked[]> {
  const searchParams = new URLSearchParams()
  if (params.sport_group) searchParams.set('sport_group', params.sport_group)
  if (params.year != null && params.year !== '') searchParams.set('year', String(params.year))
  const qs = searchParams.toString()
  const result = await apiClient<BestEffortRanked[] | { personal_records?: BestEffortRanked[] }>(
    `/personal-records${qs ? `?${qs}` : ''}`
  )
  return Array.isArray(result) ? result : ((result as { personal_records?: BestEffortRanked[] }).personal_records ?? [])
}

/** Exclude an activity from best effort ranking for a specific category. */
export async function addExclusion(activityId: string, category: string): Promise<void> {
  return apiClient<void>('/best-effort-exclusions', {
    method: 'POST',
    body: { activity_id: activityId, category },
  })
}

/** Remove an exclusion so the activity is ranked again. */
export async function removeExclusion(activityId: string, category: string): Promise<void> {
  return apiClient<void>(`/best-effort-exclusions/${activityId}/${category}`, {
    method: 'DELETE',
  })
}

/** Fetch all exclusion records for the authenticated user. */
export async function fetchExclusions(): Promise<BestEffortExclusion[]> {
  return apiClient<BestEffortExclusion[]>('/best-effort-exclusions')
}

/** Trigger a full recalculation of best efforts for the authenticated user. */
export async function recalcBestEfforts(): Promise<{ status: string; job_id?: string }> {
  return apiClient<{ status: string; job_id?: string }>('/best-efforts/recalc', {
    method: 'POST',
  })
}
