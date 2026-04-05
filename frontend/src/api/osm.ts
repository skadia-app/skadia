import { apiClient } from './client'

export interface OSMStatus {
  categories: Record<string, number>  // e.g. { summit: 79076, settlement: 124881, gastronomy: 45000, ... }
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
  summits?: number
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
  // build_topology removed: pgRouting 4.0+ removed pgr_createTopology
  run_enrichment?: boolean  // undefined = true (backwards compatible), false = skip
  build_coverage?: boolean  // opt-in, default false
  build_tiles?: boolean     // opt-in, default false
  download_dem?: boolean    // opt-in, default false
}

export interface StartOSMImportResponse {
  job_id: string
}

export async function fetchOSMStatus(): Promise<OSMStatus> {
  return apiClient<OSMStatus>('/admin/osm-status')
}

export async function importLifts(): Promise<{ imported: number }> {
  return apiClient<{ imported: number }>('/segments/import-lifts', { method: 'POST' })
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
  status: 'importing' | 'imported' | 'failed' | 'needs_redownload'
  data_date: string | null        // Datum der OSM-Kartendaten
  update_interval: string         // 'manual' | 'weekly' | 'monthly'
  replication_url: string | null  // Replication-URL für Updates
  last_updated_at: string | null  // Letzter erfolgreicher Patch/Import
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

export async function retryOSMEnrichment(regions?: string[]): Promise<StartOSMImportResponse> {
  return apiClient<StartOSMImportResponse>('/admin/osm-import/retry-enrichment', {
    method: 'POST',
    ...(regions && regions.length > 0 ? { body: { regions } } : {}),
  })
}

// retryOSMTopology removed: pgr_createTopology removed in pgRouting 4.0.1.
// BadRouter's CSR graph builder computes topology from osm_ways directly.

export interface OSMSchedule {
  interval: string  // 'manual' | 'weekly' | 'monthly'
  weekday: number   // 1=Montag … 7=Sonntag (relevant for weekly)
  day: number       // 1–28 (relevant for monthly)
  hour: number      // 0–23
}

export async function getOSMSchedule(): Promise<OSMSchedule> {
  return apiClient<OSMSchedule>('/admin/osm-schedule')
}

export async function setOSMSchedule(schedule: OSMSchedule): Promise<void> {
  await apiClient<void>('/admin/osm-schedule', { method: 'POST', body: schedule })
}

export async function rebuildTiles(req?: { regions?: string[], force?: boolean }): Promise<{ job_id: string; status: string }> {
  return apiClient<{ job_id: string; status: string }>('/admin/routing/rebuild-tiles', {
    method: 'POST',
    ...(req ? { body: req } : {}),
  })
}

export async function buildBRouterSegments(regions?: string[]): Promise<{ job_id: string; status: string }> {
  return apiClient<{ job_id: string; status: string }>('/admin/routing/build-brouter-segments', {
    method: 'POST',
    ...(regions ? { body: { regions } } : {}),
  })
}

export async function downloadBRouterSegments(regions: string[]): Promise<{ job_id: string; tiles: number; status: string }> {
  return apiClient<{ job_id: string; tiles: number; status: string }>('/admin/routing/download-brouter-segments', {
    method: 'POST',
    body: { regions },
  })
}

export async function buildCoverageGrids(regions?: string[]): Promise<{ job_id: string; status: string }> {
  return apiClient<{ job_id: string; status: string }>('/admin/build-coverage', {
    method: 'POST',
    ...(regions && regions.length > 0 ? { body: { regions } } : {}),
  })
}

export async function triggerOSMRegionUpdate(regionID: string): Promise<{ job_id: string }> {
  return apiClient<{ job_id: string }>(`/admin/osm-regions/${regionID}/update`, {
    method: 'POST',
  })
}

// DEM
export interface DEMRegionStatus {
  id: string
  tiles_needed: number
  tiles_present: number
  complete: boolean
}

export interface DEMStatusResponse {
  sources: { source: string; resolution_m: number; tile_count: number }[]
  total_tiles: number
  regions: DEMRegionStatus[]
  missing_tiles: number
  disk_free_bytes: number
}

export async function getDEMRegionStatus(): Promise<DEMStatusResponse> {
  return apiClient<DEMStatusResponse>('/admin/dem/status')
}

export async function downloadDEM(regions?: string[]): Promise<{ job_id: string }> {
  return apiClient<{ job_id: string }>('/admin/dem/download', {
    method: 'POST',
    ...(regions && regions.length > 0 ? { body: { regions } } : {}),
  })
}

// Batch update
export async function batchUpdateAllRegions(rebuildTiles?: boolean): Promise<{ job_id: string }> {
  return apiClient<{ job_id: string }>('/admin/osm-regions/update-all', {
    method: 'POST',
    body: { rebuild_tiles: rebuildTiles ?? false },
  })
}
