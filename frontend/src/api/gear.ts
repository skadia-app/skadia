import { apiClient } from './client'
import type { Gear, MaintenanceAlert } from './types'

export async function fetchGear(includeRetired = false): Promise<Gear[]> {
  const params = includeRetired ? '?retired=true' : ''
  return apiClient<Gear[]>(`/gear${params}`)
}

export async function fetchGearDetail(id: string): Promise<Gear> {
  return apiClient<Gear>(`/gear/${id}`)
}

export async function createGear(data: Partial<Gear>): Promise<Gear> {
  return apiClient<Gear>('/gear', { method: 'POST', body: data })
}

export async function updateGear(id: string, data: Partial<Gear>): Promise<Gear> {
  return apiClient<Gear>(`/gear/${id}`, { method: 'PUT', body: data })
}

export async function deleteGear(id: string): Promise<void> {
  return apiClient(`/gear/${id}`, { method: 'DELETE' })
}

export async function createComponent(gearId: string, data: any): Promise<any> {
  return apiClient(`/gear/${gearId}/components`, { method: 'POST', body: data })
}

export async function updateComponent(id: string, data: any): Promise<any> {
  return apiClient(`/gear/components/${id}`, { method: 'PUT', body: data })
}

export async function deleteComponent(id: string): Promise<void> {
  return apiClient(`/gear/components/${id}`, { method: 'DELETE' })
}

export async function fetchMaintenanceAlerts(): Promise<MaintenanceAlert[]> {
  return apiClient<MaintenanceAlert[]>('/gear/maintenance-alerts')
}
