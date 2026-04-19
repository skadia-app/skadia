import { apiClient } from './client'
import type { RecordingGroup, RecordingGroupMember } from './types'

export async function fetchRecordingGroups(limit?: number, offset?: number): Promise<RecordingGroup[]> {
  const params = new URLSearchParams()
  if (limit != null) params.set('limit', String(limit))
  if (offset != null) params.set('offset', String(offset))
  const qs = params.toString()
  return apiClient<RecordingGroup[]>(`/recording-groups${qs ? `?${qs}` : ''}`)
}

export async function fetchRecordingGroup(id: string): Promise<RecordingGroup> {
  return apiClient<RecordingGroup>(`/recording-groups/${id}`)
}

export async function fetchRecordingGroupMembers(id: string): Promise<RecordingGroupMember[]> {
  return apiClient<RecordingGroupMember[]>(`/recording-groups/${id}/members`)
}

export async function dissolveRecordingGroup(id: string): Promise<void> {
  return apiClient<void>(`/recording-groups/${id}/dissolve`, { method: 'POST' })
}

export async function setGroupPrimary(groupId: string, activityId: string): Promise<void> {
  return apiClient<void>(`/recording-groups/${groupId}/primary`, {
    method: 'PATCH',
    body: { activity_id: activityId },
  })
}

export async function setGroupSportType(groupId: string, sportType: string): Promise<void> {
  return apiClient<void>(`/recording-groups/${groupId}/sport-type`, {
    method: 'PATCH',
    body: { sport_type: sportType },
  })
}

