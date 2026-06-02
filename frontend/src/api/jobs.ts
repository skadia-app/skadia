import { apiClient } from './client'

export interface ActiveJob {
  id: string
  job_type:
    // Activity recompute
    | 'segment_recompute' | 'segment_recalculation' | 'lift_recompute' | 'activity_recompute_all' | 'activity_recompute' | 'recompute_multisport'
    // OSM pipeline
    | 'osm_import' | 'osm_enrichment' | 'osm_update' | 'osm_update_all'
    // Routing & coverage
    | 'routing_graph_build' | 'tile_build' | 'coverage_build'
    // BRouter
    | 'brouter_segment_build' | 'brouter_segment_download'
    // Photos
    | 'photo_preview_recompute' | 'photo_thumb_recompute' | 'photo_clip_embed' | 'photo_embed_backfill' | 'phash_backfill'
    // Syncs & imports
    | 'garmin_import' | 'strava_import' | 'komoot_import'
    // Corrections & resets
    | 'elevation_reset' | 'elevation_correction' | 'distance_correction' | 'distance_reset' | 'dem_import' | 'dem_download'
    // Rules & enrichment
    | 'apply_rules' | 'rule_apply' | 'activity_re_enrich' | 'weather_backfill' | 'geocoding_backfill'
    // Storage & files
    | 'shadowfs_reconcile' | 'shadowfs_rename' | 'thumbnail_backfill' | 'meta_backfill'
    // Performance & training
    | 'recalc_best_efforts' | 'recompute_all_tss' | 'fusion_queue_compute'
    // Admin
    | 'device_orphan_cleanup' | 'compression_toggle' | 'segment_publish'
    // GDPR
    | 'gdpr_purge' | 'gdpr_export' | 'data_import'
    // Data import sub-jobs
    | 'data_import_enrichment' | 'heatmap_rebuild' | 'pmc_recalc' | 'best_efforts_recalc' | 'segment_matching'
    | string
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
    // Multisport recompute phases (source: handler/recompute.go)
    | 'Zusammenfassen'
    // GDPR data import phases (source: handler_data_import.go)
    | 'data_import_files' | 'data_import_profile' | 'data_import_activities' | 'data_import_health'
    | `data_import_phase_${number}`
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
  message_code?: string
  status_code?: string
  entity_key?: string
  params?: Record<string, unknown>
}

export interface ActiveJobsResponse {
  jobs: ActiveJob[]
  pending_enrichments: number
}

export async function fetchActiveJobs(): Promise<ActiveJobsResponse> {
  return apiClient<ActiveJobsResponse>('/jobs/active')
}
