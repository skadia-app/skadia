import { apiClient, apiUpload, API_BASE } from './client'
import { getToken } from '@/lib/auth'
import type {
  Gear, MaintenanceAlert, GearStatus,
  ComponentAssignment, ChainRotationGroup,
  GearServiceEvent, GearServiceInterval, ComponentTypePreset,
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

/**
 * Payload for updateComponent PUT. All fields are optional (nil = unchanged).
 *
 * preset_gear_type_key three-state:
 *   undefined  — leave unchanged
 *   null       — clear the lock (set column to NULL)
 *   string     — set to the given gear-type key
 *
 * The backend clears via an empty string sentinel, not JSON null. The
 * updateComponent function translates null → "" transparently so callers can
 * use the more idiomatic null/undefined distinction.
 */
export interface UpdateComponentPayload {
  name?: string
  type?: string
  brand?: string | null
  model?: string | null
  notes?: string | null
  is_retired?: boolean
  purchase_price?: number | null
  purchase_date?: string | null
  serial_number?: string | null
  weight_g?: number | null
  /** Three-state: undefined = unchanged, null = clear, string = set */
  preset_gear_type_key?: string | null
}

export async function updateComponent(id: string, data: UpdateComponentPayload | Record<string, unknown>): Promise<any> {
  // Translate preset_gear_type_key: null → "" so the backend clears the
  // column. The server treats a nil Go pointer (JSON null) as "unchanged"
  // and an empty-string pointer as "clear to NULL".
  let body: Record<string, unknown> = { ...data }
  if ('preset_gear_type_key' in body && body.preset_gear_type_key === null) {
    body = { ...body, preset_gear_type_key: '' }
  }
  return apiClient(`/gear/components/${id}`, { method: 'PUT', body })
}

export async function deleteComponent(id: string): Promise<void> {
  return apiClient(`/gear/components/${id}`, { method: 'DELETE' })
}

export async function removeComponent(id: string): Promise<void> {
  return apiClient(`/gear/components/${id}/remove`, { method: 'POST' })
}

export async function updateComponentStatus(
  id: string,
  status: 'active' | 'stored' | 'retired',
): Promise<void> {
  return apiClient(`/gear/components/${id}/status`, { method: 'PATCH', body: { status } })
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
  swap_interval_km?: number
  swap_interval_metric_key?: string
  swap_interval_value?: number
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
  name: string
  swap_interval_km: number
  swap_interval_hm: number
  swap_interval_metric_key: string
  swap_interval_value: number
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
  interval_id?: string | null
  description?: string | null
  cost?: number | null
  notes?: string | null
}): Promise<GearServiceEvent> {
  return apiClient<GearServiceEvent>(`/gear/components/${componentId}/services`, {
    method: 'POST',
    body: data,
  })
}

export async function listServiceIntervals(componentId: string): Promise<GearServiceInterval[]> {
  return apiClient<GearServiceInterval[]>(`/gear/components/${componentId}/service-intervals`)
}

export async function createServiceInterval(componentId: string, data: Partial<GearServiceInterval>): Promise<GearServiceInterval> {
  return apiClient<GearServiceInterval>(`/gear/components/${componentId}/service-intervals`, { method: 'POST', body: data })
}

export async function updateServiceInterval(id: string, data: Partial<GearServiceInterval>): Promise<GearServiceInterval> {
  return apiClient<GearServiceInterval>(`/gear/service-intervals/${id}`, { method: 'PUT', body: data })
}

export async function deleteServiceInterval(id: string): Promise<void> {
  return apiClient(`/gear/service-intervals/${id}`, { method: 'DELETE' })
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

/** Payload for createBikeSetup. capture_components is a transient server-side
 *  flag — when true the server builds a component_snapshot from the current
 *  gear_components state. The field is consumed by the API and never stored
 *  as-is; it does not appear on the returned BikeSetup. */
export type BikeSetupCreatePayload = Partial<BikeSetup> & { capture_components?: boolean }

/** Payload for updateBikeSetup. capture_components three-state:
 *  undefined → leave snapshot as-is; true → re-capture; false → clear. */
export type BikeSetupUpdatePayload = Partial<BikeSetup> & { capture_components?: boolean }

export async function createBikeSetup(gearId: string, data: BikeSetupCreatePayload): Promise<BikeSetup> {
  return apiClient<BikeSetup>(`/gear/${gearId}/setups`, { method: 'POST', body: data })
}

export async function updateBikeSetup(id: string, data: BikeSetupUpdatePayload): Promise<BikeSetup> {
  return apiClient<BikeSetup>(`/gear/setups/${id}`, { method: 'PUT', body: data })
}

export async function deleteBikeSetup(id: string): Promise<void> {
  return apiClient(`/gear/setups/${id}`, { method: 'DELETE' })
}

export async function activateBikeSetup(id: string): Promise<BikeSetup> {
  return apiClient<BikeSetup>(`/gear/setups/${id}/activate`, { method: 'POST' })
}

// --- Gear Documents ---

export async function uploadGearDocument(gearId: string, file: File, opts?: {
  doc_type?: string
  title?: string
  document_date?: string
  document_amount?: number
  document_currency?: string
}): Promise<GearDocument> {
  const formData = new FormData()
  formData.append('file', file)
  if (opts?.doc_type) formData.append('doc_type', opts.doc_type)
  if (opts?.title) formData.append('title', opts.title)
  if (opts?.document_date) formData.append('document_date', opts.document_date)
  if (opts?.document_amount != null) formData.append('document_amount', opts.document_amount.toString())
  if (opts?.document_currency) formData.append('document_currency', opts.document_currency)
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
