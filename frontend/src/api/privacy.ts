import { apiClient } from './client'

export interface PrivacyZone {
  id: string
  name: string
  lat: number
  lng: number
  radius_m: number
  color: string
  is_active: boolean
  created_at: string
  zone_type: 'circle' | 'polygon'
  polygon?: string // GeoJSON Polygon string, set when zone_type='polygon'
}

export async function listPrivacyZones(): Promise<PrivacyZone[]> {
  return apiClient<PrivacyZone[]>('/privacy-zones')
}

export async function createPrivacyZone(zone: {
  name: string
  lat: number
  lng: number
  radius_m: number
  color: string
  zone_type?: 'circle' | 'polygon'
  polygon?: string
}): Promise<PrivacyZone> {
  return apiClient<PrivacyZone>('/privacy-zones', { method: 'POST', body: zone })
}

export async function updatePrivacyZone(
  id: string,
  update: Partial<Pick<PrivacyZone, 'name' | 'lat' | 'lng' | 'radius_m' | 'color' | 'is_active' | 'zone_type' | 'polygon'>>,
): Promise<PrivacyZone> {
  return apiClient<PrivacyZone>(`/privacy-zones/${id}`, { method: 'PUT', body: update })
}

export async function deletePrivacyZone(id: string): Promise<void> {
  return apiClient<void>(`/privacy-zones/${id}`, { method: 'DELETE' })
}
