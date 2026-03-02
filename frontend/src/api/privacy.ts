import { apiClient } from './client'

export interface PrivacyZone {
  id: string
  name: string
  lat: number
  lng: number
  radius_m: number
  is_active: boolean
  created_at: string
}

export async function listPrivacyZones(): Promise<PrivacyZone[]> {
  return apiClient<PrivacyZone[]>('/privacy-zones')
}

export async function createPrivacyZone(zone: {
  name: string
  lat: number
  lng: number
  radius_m: number
}): Promise<PrivacyZone> {
  return apiClient<PrivacyZone>('/privacy-zones', { method: 'POST', body: zone })
}

export async function updatePrivacyZone(
  id: string,
  update: Partial<Pick<PrivacyZone, 'name' | 'lat' | 'lng' | 'radius_m' | 'is_active'>>,
): Promise<PrivacyZone> {
  return apiClient<PrivacyZone>(`/privacy-zones/${id}`, { method: 'PUT', body: update })
}

export async function deletePrivacyZone(id: string): Promise<void> {
  return apiClient<void>(`/privacy-zones/${id}`, { method: 'DELETE' })
}
