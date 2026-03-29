import { apiClient } from './client'

export interface Device {
  id: string
  user_id: string
  type: 'recorder' | 'sensor'
  manufacturer?: string
  manufacturer_id?: number
  product_name?: string
  product_id?: number
  custom_name?: string
  serial_number?: number
  ant_device_number?: number
  device_types?: string[]
  source_type?: string
  first_seen_at: string
  last_seen_at: string
  activity_count: number
  created_at: string
}

export interface DeviceActivity {
  id: string
  title?: string
  sport_type: string
  started_at: string
  distance_m: number
  timezone?: string | null
}

export async function fetchDevices(type?: string): Promise<Device[]> {
  const params = type ? `?type=${type}` : ''
  return apiClient<Device[]>(`/hardware${params}`)
}

export async function fetchDevice(id: string): Promise<Device> {
  return apiClient<Device>(`/hardware/${id}`)
}

export async function updateDeviceCustomName(id: string, customName: string): Promise<Device> {
  return apiClient<Device>(`/hardware/${id}`, {
    method: 'PATCH',
    body: { custom_name: customName },
  })
}

export async function mergeDevices(sourceId: string, targetId: string): Promise<void> {
  return apiClient(`/hardware/merge`, {
    method: 'POST',
    body: { source_id: sourceId, target_id: targetId },
  })
}

export async function deleteDevice(id: string): Promise<void> {
  return apiClient(`/hardware/${id}`, { method: 'DELETE' })
}

export async function fetchDeviceActivities(id: string, after?: string): Promise<DeviceActivity[]> {
  const params = after ? `?after=${encodeURIComponent(after)}` : ''
  return apiClient<DeviceActivity[]>(`/hardware/${id}/activities${params}`)
}
