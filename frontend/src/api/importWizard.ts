import { apiClient } from './client'

export type ConflictStrategy = 'enrich_missing' | 'replace_all' | 'skip'

export interface ImportOptions {
  categories?: string[]
  date_from?: string   // ISO date string "YYYY-MM-DD"
  date_to?: string
  conflict_strategy: ConflictStrategy
}

export interface CategoryStats {
  found: number
  existing: number
  new: number
}

export interface ScanResult {
  activities?: CategoryStats
  gear?: CategoryStats
  profile?: CategoryStats
  privacy_zones?: CategoryStats
  health?: CategoryStats
  monitoring?: CategoryStats
  [key: string]: CategoryStats | undefined  // für weitere Kategorien
}

export interface ImportSettings {
  conflict_strategy: ConflictStrategy
  gps_duplicate_threshold: number
  fusion_time_window_min: number
}

export async function scanGarminExport(sourcePath: string): Promise<ScanResult> {
  return apiClient<ScanResult>('/import/garmin/scan', {
    method: 'POST',
    body: { source_path: sourcePath },
  })
}

export async function scanStravaExport(sourcePath: string): Promise<ScanResult> {
  return apiClient<ScanResult>('/import/strava/scan', {
    method: 'POST',
    body: { source_path: sourcePath },
  })
}

export async function startGarminImport(sourcePath: string, options: ImportOptions): Promise<void> {
  return apiClient<void>('/import/garmin/start', {
    method: 'POST',
    body: { source_path: sourcePath, options },
  })
}

export async function startStravaImport(sourcePath: string, options: ImportOptions): Promise<void> {
  return apiClient<void>('/import/strava/start', {
    method: 'POST',
    body: { source_path: sourcePath, options },
  })
}

export async function getImportSettings(): Promise<ImportSettings> {
  return apiClient<ImportSettings>('/settings/import')
}

export async function updateImportSettings(s: ImportSettings): Promise<ImportSettings> {
  return apiClient<ImportSettings>('/settings/import', {
    method: 'PUT',
    body: s,
  })
}

// Fusion API
export interface FusionCandidate {
  id: string
  title: string
  started_at: string
  distance_m: number
  sport_type: string
}

export async function getFusionCandidates(activityId: string): Promise<FusionCandidate[]> {
  return apiClient<FusionCandidate[]>(`/activities/${activityId}/fusion-candidates`)
}

// Fusion Queue (Tier-2 GPS-Similarity)
export interface FusionQueueEntry {
  activity_id_a: string
  activity_id_b: string
  title_a: string
  title_b: string
  started_at_a: string
  started_at_b: string
  sport_type_a: string   // können verschieden sein! (z.B. "cycling" vs "hiking")
  sport_type_b: string
  sub_type_a: string
  sub_type_b: string
  is_electric_a: boolean
  is_electric_b: boolean
  distance_m_a: number
  distance_m_b: number
  overlap_percent: number
  field_richness_a: number
  field_richness_b: number
  time_diff_sec: number
  source_device_a: string
  source_a: string
  original_file_format_a: string
  source_device_b: string
  source_b: string
  original_file_format_b: string
}

export interface FusionQueueResponse {
  items: FusionQueueEntry[]
  computing: boolean
}

export async function getFusionQueue(limit = 20): Promise<FusionQueueResponse> {
  return apiClient<FusionQueueResponse>(`/fusion-queue?limit=${limit}`)
}

export async function dismissFusionCandidate(activityIdA: string, activityIdB: string): Promise<void> {
  return apiClient<void>('/fusion-candidates/dismiss', {
    method: 'POST',
    body: { activity_id_a: activityIdA, activity_id_b: activityIdB },
  })
}

// Gear Merge API (ergänzend zu gear.ts)
export async function getGearDuplicateCandidates(): Promise<[string, string][]> {
  return apiClient<[string, string][]>('/gear/duplicate-candidates')
}

export async function mergeGear(primaryId: string, mergeFromId: string): Promise<void> {
  return apiClient<void>(`/gear/${primaryId}/merge`, {
    method: 'POST',
    body: { merge_from_id: mergeFromId },
  })
}
