import { apiFetch } from '@/lib/auth'

export interface ResolveSportConflictResponse {
  status: 'resolved' | 'already_resolved'
}

export interface ResolveSportConflictPayload {
  sport_type: string
  sub_type?: string
  is_electric?: boolean
  is_indoor?: boolean
  is_virtual?: boolean
}

export async function resolveSportConflict(
  activityId: string,
  sport?: string | ResolveSportConflictPayload
): Promise<ResolveSportConflictResponse> {
  const body = typeof sport === 'string'
    ? { sport_type: sport }
    : (sport ?? {})
  const res = await apiFetch(
    `/api/v1/activities/${activityId}/resolve-sport-conflict`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }
  )
  if (!res.ok) throw new Error(`resolve failed: ${res.status}`)
  return res.json()
}
