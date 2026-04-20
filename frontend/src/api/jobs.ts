import { apiClient } from './client'

export interface ActiveJob {
  id: string
  job_type: 'segment_recompute' | 'activity_recompute_all' | 'activity_recompute' | 'osm_import' | 'osm_enrichment' | 'photo_preview_recompute' | 'photo_thumb_recompute' | 'photo_clip_embed' | 'photo_embed_backfill' | 'garmin_import' | 'strava_import' | 'komoot_import' | 'phash_backfill' | 'recompute_multisport' | 'elevation_reset' | 'elevation_correction' | 'distance_correction' | 'distance_reset' | 'dem_import' | 'apply_rules' | string
  entity_id?: string
  entity_name?: string
  status: 'running' | 'completed' | 'failed'
  phase?:
    // OSM pipeline phases (source: osm/pipeline.go, osm/osmium.go)
    | 'download' | 'filter' | 'import' | 'merge' | 'index' | 'optimize' | 'analyze' | 'enrichment'
    // OSM enrichment sub-phase (source: database/jobs.go comment, ActiveJobsIndicator.tsx)
    | 'splitting'
    // OSM import orchestration phases (source: handler/osm_import.go)
    | 'brouter_segments' | 'dem_download' | 'coverage' | 'tiles'
    // OSM admin/patch pipeline (source: handler/osm_admin.go)
    | 'patch'
    // DEM import phases (source: handler/elevation.go)
    | 'overview' | 'vacuum'
    // TimescaleDB compression phases (source: handler/admin_settings.go)
    | 'enabling compression' | 'removing compression policy' | 'decompressing chunks'
    // Multisport recompute phases — German free-text (source: handler/recompute.go)
    | 'Zusammenfassen'
    // Dynamic: BRouter preprocessing steps e.g. brouter_OsmFastCutter (source: osm/brouter.go)
    | `brouter_${string}`
    // Dynamic: per-region import progress e.g. region_1_europe (source: handler/osm_import.go)
    | `region_${string}`
    // Dynamic: multisport child label e.g. "Disziplin 1/3" (source: handler/recompute.go)
    | `Disziplin ${string}`
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
