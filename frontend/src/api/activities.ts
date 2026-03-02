import { apiClient } from './client'
import type { ActivitiesResponse, Activity, StreamPoint, Sensor } from './types'

export async function fetchActivities(
  cursor?: string,
  limit = 20,
  sortBy?: string,
  sortDir?: string,
): Promise<ActivitiesResponse> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('cursor', cursor)
  if (sortBy) params.set('sort_by', sortBy)
  if (sortDir) params.set('sort_dir', sortDir)

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
