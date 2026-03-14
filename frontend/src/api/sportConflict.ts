import { apiFetch } from '@/lib/auth'

export interface ResolveSportConflictResponse {
  status: 'resolved' | 'already_resolved'
}

export async function resolveSportConflict(
  activityId: string,
  sportType?: string
): Promise<ResolveSportConflictResponse> {
  const res = await apiFetch(
    `/api/v1/activities/${activityId}/resolve-sport-conflict`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(sportType ? { sport_type: sportType } : {}),
    }
  )
  if (!res.ok) throw new Error(`resolve failed: ${res.status}`)
  return res.json()
}
