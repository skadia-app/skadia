import type { ActivitiesResponse } from './types'

export async function fetchActivities(cursor?: string, limit = 20): Promise<ActivitiesResponse> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('cursor', cursor)

  const res = await fetch(`/api/v1/activities?${params}`)
  if (!res.ok) throw new Error(`API error ${res.status}`)
  return res.json() as Promise<ActivitiesResponse>
}
