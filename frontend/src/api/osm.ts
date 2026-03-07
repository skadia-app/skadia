import { apiClient } from './client'

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
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
}
