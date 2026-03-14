import { apiClient } from './client'

export interface OSMStatus {
  categories: Record<string, number>  // e.g. { peak: 79076, settlement: 124881, gastronomy: 45000, ... }
  ways: number
  lifts: number
  pistes: number
  waterways: number
  forests: number
  glaciers: number
  wetlands: number
  routes: number
  boundaries: number
  landuse: number
  railways: number
  has_data: boolean
  // Legacy fields for backwards-compat (may come from older API versions)
  peaks?: number
  passes?: number
  settlements?: number
  amenities?: number
}

export interface OSMRegion {
  id: string
  label: string
  continent: string
  url: string
  size_mb: number
}

export interface OSMRegionsResponse {
  regions: OSMRegion[]
}

export interface StartOSMImportRequest {
  regions: string[]
  categories: string[]
  build_brouter?: boolean
  build_topology?: boolean
}

export interface StartOSMImportResponse {
  job_id: string
}

export async function fetchOSMRegions(): Promise<OSMRegionsResponse> {
  return apiClient<OSMRegionsResponse>('/admin/osm-regions')
}

export async function startOSMImport(req: StartOSMImportRequest): Promise<StartOSMImportResponse> {
  return apiClient<StartOSMImportResponse>('/admin/osm-import/start', {
    method: 'POST',
    body: req,
  })
}

export interface OSMImportRegion {
  region_id: string
  label: string
  continent: string
  imported_at: string
  pbf_date: string | null
  pbf_size_bytes: number | null
  import_duration_s: number | null
  categories: string[]
  status: 'importing' | 'imported' | 'failed'
}

export interface OSMDiskUsage {
  tables: { tablename: string; total_bytes: number }[]
  total_bytes: number
}

export async function fetchOSMRegionStatus(): Promise<{ regions: OSMImportRegion[] }> {
  return apiClient<{ regions: OSMImportRegion[] }>('/admin/osm-regions/status')
}

export async function fetchOSMDiskUsage(): Promise<OSMDiskUsage> {
  return apiClient<OSMDiskUsage>('/admin/osm-disk-usage')
}

export async function cancelOSMImport(jobID: string): Promise<void> {
  await apiClient(`/admin/osm-import/cancel/${jobID}`, { method: 'POST' })
}

export async function deleteOSMRegion(regionID: string): Promise<void> {
  await apiClient(`/admin/osm-regions/${regionID}`, { method: 'DELETE' })
}

export async function retryOSMEnrichment(): Promise<StartOSMImportResponse> {
  return apiClient<StartOSMImportResponse>('/admin/osm-import/retry-enrichment', { method: 'POST' })
}

export async function retryOSMTopology(): Promise<StartOSMImportResponse> {
  return apiClient<StartOSMImportResponse>('/admin/osm-import/retry-topology', { method: 'POST' })
}
