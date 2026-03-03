import { apiClient, apiUpload } from './client'
import { getToken } from '@/lib/auth'
import type {
  Gear, MaintenanceAlert, GearStatus,
  ComponentAssignment, ChainRotationGroup,
  GearServiceEvent, ComponentTypePreset,
  BikeSetup, GearDocument,
  GearCostSummary,
} from './types'

export async function fetchGear(status?: GearStatus | boolean): Promise<Gear[]> {
  let params = ''
  if (typeof status === 'string') {
    params = `?status=${status}`
  } else if (status === true) {
    // Legacy: showRetired=true means show all statuses
    params = ''
  }
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

export async function updateGearStatus(id: string, status: GearStatus): Promise<Gear> {
  return apiClient<Gear>(`/gear/${id}/status`, { method: 'PATCH', body: { status } })
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

export async function transferComponent(componentId: string, toGearId: string): Promise<void> {
  return apiClient(`/gear/components/${componentId}/transfer`, {
    method: 'POST',
    body: { to_gear_id: toGearId },
  })
}

export async function getComponentHistory(componentId: string): Promise<ComponentAssignment[]> {
  return apiClient<ComponentAssignment[]>(`/gear/components/${componentId}/history`)
}

export async function getChainRotation(gearId: string): Promise<ChainRotationGroup | null> {
  return apiClient<ChainRotationGroup | null>(`/gear/${gearId}/chain-rotation`)
}

export async function createChainRotation(gearId: string, data: {
  name: string
  swap_interval_km: number
  component_ids: string[]
}): Promise<ChainRotationGroup> {
  return apiClient<ChainRotationGroup>(`/gear/${gearId}/chain-rotation`, {
    method: 'POST',
    body: data,
  })
}

export async function swapChain(gearId: string): Promise<ChainRotationGroup> {
  return apiClient<ChainRotationGroup>(`/gear/${gearId}/chain-rotation/swap`, {
    method: 'POST',
  })
}

export async function updateChainRotation(groupId: string, data: Partial<{
  swap_interval_km: number
  swap_interval_hm: number
}>): Promise<void> {
  return apiClient(`/gear/chain-rotation/${groupId}`, {
    method: 'PUT',
    body: data,
  })
}

export async function deleteChainRotation(groupId: string): Promise<void> {
  return apiClient(`/gear/chain-rotation/${groupId}`, { method: 'DELETE' })
}

export async function listServiceEvents(componentId: string): Promise<GearServiceEvent[]> {
  return apiClient<GearServiceEvent[]>(`/gear/components/${componentId}/services`)
}

export async function createServiceEvent(componentId: string, data: {
  performed_at: string
  description?: string | null
  cost?: number | null
  notes?: string | null
}): Promise<GearServiceEvent> {
  return apiClient<GearServiceEvent>(`/gear/components/${componentId}/services`, {
    method: 'POST',
    body: data,
  })
}

export async function deleteServiceEvent(eventId: string): Promise<void> {
  return apiClient(`/gear/services/${eventId}`, { method: 'DELETE' })
}

export async function fetchComponentPresets(gearType?: string): Promise<ComponentTypePreset[]> {
  const params = gearType ? `?gear_type=${gearType}` : ''
  return apiClient<ComponentTypePreset[]>(`/gear/component-presets${params}`)
}

export async function fetchMaintenanceAlerts(): Promise<MaintenanceAlert[]> {
  return apiClient<MaintenanceAlert[]>('/gear/maintenance-alerts')
}

// --- Bike Setups ---

export async function listBikeSetups(gearId: string): Promise<BikeSetup[]> {
  return apiClient<BikeSetup[]>(`/gear/${gearId}/setups`)
}

export async function createBikeSetup(gearId: string, data: Partial<BikeSetup>): Promise<BikeSetup> {
  return apiClient<BikeSetup>(`/gear/${gearId}/setups`, { method: 'POST', body: data })
}

export async function updateBikeSetup(id: string, data: Partial<BikeSetup>): Promise<BikeSetup> {
  return apiClient<BikeSetup>(`/gear/setups/${id}`, { method: 'PUT', body: data })
}

export async function deleteBikeSetup(id: string): Promise<void> {
  return apiClient(`/gear/setups/${id}`, { method: 'DELETE' })
}

export async function activateBikeSetup(id: string): Promise<BikeSetup> {
  return apiClient<BikeSetup>(`/gear/setups/${id}/activate`, { method: 'POST' })
}

// --- Gear Documents ---

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export async function uploadGearDocument(gearId: string, file: File, docType?: string): Promise<GearDocument> {
  const formData = new FormData()
  formData.append('file', file)
  if (docType) formData.append('doc_type', docType)
  return apiUpload<GearDocument>(`/gear/${gearId}/documents`, formData)
}

export async function fetchGearDocuments(gearId: string): Promise<GearDocument[]> {
  return apiClient<GearDocument[]>(`/gear/${gearId}/documents`)
}

export async function deleteGearDocument(docId: string): Promise<void> {
  await apiClient(`/gear/documents/${docId}`, { method: 'DELETE' })
}

export function getGearDocumentUrl(docId: string): string {
  return `${API_BASE}/gear/documents/${docId}/download`
}

/** Fetch a gear document as a Blob using the auth token */
export async function downloadGearDocumentBlob(docId: string): Promise<string> {
  const token = getToken()
  const url = `${API_BASE}/gear/documents/${docId}/download`
  const res = await fetch(url, {
    headers: token ? { 'Authorization': `Bearer ${token}` } : {},
    credentials: 'include',
  })
  if (!res.ok) throw new Error(`Download failed: ${res.status}`)
  const blob = await res.blob()
  return URL.createObjectURL(blob)
}

export async function uploadComponentDocument(componentId: string, file: File): Promise<GearDocument> {
  const formData = new FormData()
  formData.append('file', file)
  return apiUpload<GearDocument>(`/gear/components/${componentId}/documents`, formData)
}

export async function listComponentDocuments(componentId: string): Promise<GearDocument[]> {
  return apiClient<GearDocument[]>(`/gear/components/${componentId}/documents`)
}

export async function getGearCosts(id: string): Promise<GearCostSummary> {
  return apiClient<GearCostSummary>(`/gear/${id}/costs`)
}
