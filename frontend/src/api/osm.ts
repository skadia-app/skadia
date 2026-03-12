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
  boundaries: number
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
