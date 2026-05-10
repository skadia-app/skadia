import { apiClient } from './client'

export interface LiftDetectionConfigResponse {
  effective: { osm: boolean; heuristic: boolean }
  user_setting: { osm: boolean; heuristic: boolean }
  override: { osm: boolean | null; heuristic: boolean | null }
  master_enabled: boolean
}

export async function getLiftDetectionConfig(activityId: string): Promise<LiftDetectionConfigResponse> {
  return apiClient<LiftDetectionConfigResponse>(`/activities/${activityId}/lifts/detection-config`)
}

export async function putLiftDetectionConfig(
  activityId: string,
  body: { osm?: boolean; heuristic?: boolean },
): Promise<void> {
  // Returns 204 — apiClient handles 204 by returning null as T; caller ignores the value.
  await apiClient<void>(`/activities/${activityId}/lifts/detection-config`, {
    method: 'PUT',
    body,
  })
}

export async function deleteLiftDetectionConfig(activityId: string): Promise<void> {
  // Returns 204.
  await apiClient<void>(`/activities/${activityId}/lifts/detection-config`, {
    method: 'DELETE',
  })
}
