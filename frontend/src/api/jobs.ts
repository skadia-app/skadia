import { apiClient } from './client'

export interface ActiveJob {
  id: string
  job_type: 'segment_recompute' | 'activity_recompute_all' | 'activity_recompute' | 'osm_import' | 'osm_enrichment' | 'photo_preview_recompute' | 'photo_thumb_recompute' | 'photo_clip_embed' | 'photo_embed_backfill' | 'garmin_import' | 'strava_import' | 'komoot_import' | 'phash_backfill' | 'recompute_multisport' | 'elevation_reset' | 'elevation_correction' | 'distance_correction' | 'distance_reset' | 'dem_import' | 'apply_rules' | string
  entity_id?: string
  entity_name?: string
  status: 'running' | 'completed' | 'failed'
  phase?: 'download' | 'import' | 'index' | 'optimize' | 'topology' | 'enrichment' | 'splitting'
  progress_current: number
  progress_total: number
  started_at: string
  finished_at?: string
}

export interface ActiveJobsResponse {
  jobs: ActiveJob[]
  pending_enrichments: number
}

export async function fetchActiveJobs(): Promise<ActiveJobsResponse> {
  return apiClient<ActiveJobsResponse>('/jobs/active')
}
