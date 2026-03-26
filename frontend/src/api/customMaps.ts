import { apiClient, apiUpload } from './client'

export interface CustomMap {
  id: string
  user_id: string
  name: string
  description?: string
  color: string
  is_visible: boolean
  sort_order: number
  marker_count: number
  created_at: string
  updated_at: string
}

export interface CustomMapMarker {
  id: string
  map_id: string
  user_id: string
  name: string
  description?: string
  lat: number
  lng: number
  color?: string
  image_file_id?: string
  created_at: string
  updated_at: string
}

export async function listCustomMaps(): Promise<CustomMap[]> {
  return apiClient<CustomMap[]>('/custom-maps')
}

export async function createCustomMap(req: { name: string; description?: string; color: string }): Promise<CustomMap> {
  return apiClient<CustomMap>('/custom-maps', { method: 'POST', body: req })
}

export async function updateCustomMap(
  mapID: string,
  req: Partial<{ name: string; description: string; color: string; is_visible: boolean }>,
): Promise<CustomMap> {
  return apiClient<CustomMap>(`/custom-maps/${mapID}`, { method: 'PUT', body: req })
}

export async function deleteCustomMap(mapID: string): Promise<void> {
  return apiClient<void>(`/custom-maps/${mapID}`, { method: 'DELETE' })
}

export async function listCustomMapMarkers(mapID: string): Promise<CustomMapMarker[]> {
  return apiClient<CustomMapMarker[]>(`/custom-maps/${mapID}/markers`)
}

export async function createCustomMapMarker(
  mapID: string,
  req: { name: string; description?: string; lat: number; lng: number; color?: string },
): Promise<CustomMapMarker> {
  return apiClient<CustomMapMarker>(`/custom-maps/${mapID}/markers`, { method: 'POST', body: req })
}

export async function updateCustomMapMarker(
  mapID: string,
  markerID: string,
  req: Partial<{ name: string; description: string; lat: number; lng: number; color: string; image_file_id: string }>,
): Promise<CustomMapMarker> {
  return apiClient<CustomMapMarker>(`/custom-maps/${mapID}/markers/${markerID}`, { method: 'PUT', body: req })
}

export async function uploadMarkerImage(mapId: string, markerId: string, file: File): Promise<CustomMapMarker> {
  const formData = new FormData()
  formData.append('image', file)
  return apiUpload<CustomMapMarker>(`/custom-maps/${mapId}/markers/${markerId}/image`, formData)
}

export async function deleteCustomMapMarker(mapID: string, markerID: string): Promise<void> {
  return apiClient<void>(`/custom-maps/${mapID}/markers/${markerID}`, { method: 'DELETE' })
}
