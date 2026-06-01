import { i18n } from '@/i18n/i18n'
import type { ActiveJob } from '@/api/jobs'

export const JOB_TYPE_LABEL_KEYS: Record<string, string> = {
  osm_import: 'osmImport',
  osm_enrichment: 'osmEnrichment',
  osm_update: 'osmUpdate',
  osm_update_all: 'osmUpdateAll',
  garmin_import: 'garminImport',
  strava_import: 'stravaImport',
  komoot_import: 'komootImport',
  data_import: 'dataImport',
  data_import_enrichment: 'dataImportEnrichment',
  gdpr_export: 'gdprExport',
  gdpr_purge: 'gdprPurge',
  segment_recompute: 'segmentRecompute',
  segment_recalculation: 'segmentRecalculation',
  lift_recompute: 'liftRecompute',
  segment_publish: 'segmentPublish',
  segment_matching: 'segmentMatching',
  activity_recompute: 'activityRecompute',
  activity_recompute_all: 'activityRecomputeAll',
  activity_re_enrich: 'activityReEnrich',
  backfill_activity_enrichments: 'backfillActivityEnrichments',
  recompute_multisport: 'recomputeMultisport',
  photo_preview_recompute: 'photoPreviewRecompute',
  photo_thumb_recompute: 'photoThumbRecompute',
  photo_clip_embed: 'photoClipEmbed',
  photo_embed_backfill: 'photoEmbedBackfill',
  phash_backfill: 'phashBackfill',
  thumbnail_backfill: 'thumbnailBackfill',
  meta_backfill: 'metaBackfill',
  elevation_reset: 'elevationReset',
  elevation_correction: 'elevationCorrection',
  distance_correction: 'distanceCorrection',
  distance_reset: 'distanceReset',
  dem_import: 'demImport',
  dem_download: 'demDownload',
  apply_rules: 'applyRules',
  rule_apply: 'ruleApply',
  routing_graph_build: 'routingGraphBuild',
  tile_build: 'tileBuild',
  brouter_segment_build: 'brouterSegmentBuild',
  brouter_segment_download: 'brouterSegmentDownload',
  coverage_build: 'coverageBuild',
  weather_backfill: 'weatherBackfill',
  geocoding_backfill: 'geocodingBackfill',
  heatmap_rebuild: 'heatmapRebuild',
  pmc_recalc: 'pmcRecalc',
  recalc_best_efforts: 'recalcBestEfforts',
  recalc_best_efforts_all: 'recalcBestEffortsAll',
  best_efforts_recalc: 'bestEffortsRecalc',
  recompute_all_tss: 'recomputeAllTss',
  fusion_queue_compute: 'fusionQueueCompute',
  shadowfs_reconcile: 'shadowfsReconcile',
  shadowfs_rename: 'shadowfsRename',
  compression_toggle: 'compressionToggle',
  device_orphan_cleanup: 'deviceOrphanCleanup',
}

const JOB_STATUS_LABEL_KEYS = new Set(['running', 'completed', 'failed'])

const JOB_PHASE_LABEL_KEYS: Record<string, string> = {
  enrichment: 'enrichment',
  splitting: 'splitting',
  download: 'download',
  filter: 'filter',
  import: 'import',
  merge: 'merge',
  index: 'index',
  optimize: 'optimize',
  analyze: 'analyze',
  brouter_segments: 'brouterSegments',
  dem_download: 'demDownload',
  coverage: 'coverage',
  tiles: 'tiles',
  patch: 'patch',
  overview: 'overview',
  vacuum: 'vacuum',
  'enabling compression': 'enablingCompression',
  'removing compression policy': 'removingCompressionPolicy',
  'decompressing chunks': 'decompressingChunks',
  Multisport: 'multisport',
  Zusammenfassen: 'summarize',
  data_import_files: 'dataImportFiles',
  data_import_profile: 'dataImportProfile',
  data_import_activities: 'dataImportActivities',
  data_import_health: 'dataImportHealth',
}

export function getJobTypeLabel(jobType: string): string {
  const labelKey = JOB_TYPE_LABEL_KEYS[jobType]
  if (!labelKey) return i18n.t('jobs.type.unknownCode')
  return i18n.t(`jobs.type.${labelKey}`)
}

export function getJobStatusLabel(status: string): string {
  if (!JOB_STATUS_LABEL_KEYS.has(status)) {
    return i18n.t('jobs.status.unknownCode')
  }
  return i18n.t(`jobs.status.${status}`)
}

export function getJobPhaseLabel(phase: string): string {
  const labelKey = JOB_PHASE_LABEL_KEYS[phase]
  if (labelKey) return i18n.t(`jobs.phase.${labelKey}`)
  if (phase.startsWith('landmarks:')) return i18n.t('jobs.phase.landmarks')
  if (phase.startsWith('region_')) return i18n.t('jobs.phase.region')
  if (phase.startsWith('brouter_')) return i18n.t('jobs.phase.brouterDynamic')
  const dataImportPhase = phase.match(/^data_import_phase_(\d+)$/)
  if (dataImportPhase) return i18n.t('jobs.phase.dataImportPhase', { phase: dataImportPhase[1] })
  if (phase.startsWith('Disziplin ')) return i18n.t('jobs.phase.discipline')
  return i18n.t('jobs.phase.unknownCode')
}

export function getJobMessageLabel(job: Pick<ActiveJob, 'message_code' | 'params'>): string | null {
  if (!job.message_code || !i18n.exists(job.message_code)) {
    return null
  }
  return i18n.t(job.message_code, job.params ?? {})
}

export function getJobNotificationTitle(
  job: Pick<ActiveJob, 'job_type' | 'entity_name' | 'message_code' | 'params'> & { status: string },
): string {
  const structuredMessage = getJobMessageLabel(job)
  if (structuredMessage) {
    return structuredMessage
  }

  const label = getJobTypeLabel(job.job_type)
  const subject = job.entity_name ? i18n.t('jobs.notification.subjectWithEntity', { label, entity: job.entity_name }) : label
  if (!JOB_STATUS_LABEL_KEYS.has(job.status)) {
    return i18n.t('jobs.notification.unknownStatus', { subject, status: getJobStatusLabel(job.status) })
  }
  return i18n.t(`jobs.notification.${job.status}`, { subject })
}

export const JOB_TYPE_LABELS: Record<string, string> = new Proxy({}, {
  get(_target, prop) {
    return typeof prop === 'string' && prop in JOB_TYPE_LABEL_KEYS ? getJobTypeLabel(prop) : undefined
  },
}) as Record<string, string>
