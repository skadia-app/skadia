import type { Capabilities } from '@/api/routes'

export const CAPABILITY_KEYS = [
  'deployment_profile',
  'storage_mode',
  'storage_backend',
  'library_paths',
  'library_schemas',
  'library_sync',
  'import_path_enabled',
  'direct_upload_lifecycle',
  'route_photo_media',
  'gdpr_artifacts',
  'osm_import',
  'osm_enrichment',
  'brouter',
  'brouter_segment_download',
  'brouter_segment_build',
  'badrouter',
  'routing',
  'routing_tiles',
  'dem_correction',
  'coverage',
  'coverage_tiles',
  'system_layers_vector',
  'system_layers_coverage',
  'offline_map',
  'geo_poi_enrichment',
  'surface_enrichment',
  'activity_track_summary_geo',
  'activity_track_summary_surface',
  'segments',
  'segment_recompute',
  'segment_publish',
  'segment_osm_lift_import',
  'segment_enrichment_suggest',
  'osm_lift_snap',
  'isochrone',
  'route_suggestion',
  'local_poi_search',
  'ml_service',
  'document_ml',
  'photo_embeddings',
  'photo_embedding_storage_access',
  'ai_agent',
] as const

export type CapabilityKey = typeof CAPABILITY_KEYS[number]

const CAPABILITY_KEY_SET = new Set<string>(CAPABILITY_KEYS)

export function validateCapabilityKey(key: string): key is CapabilityKey {
  return CAPABILITY_KEY_SET.has(key)
}

export function featureConfigured(caps: Capabilities | null | undefined, key: string): boolean {
  const state = caps?.features?.[key]
  if (state) return state.configured === true
  return caps?.[key] === true
}

export function featureAvailable(caps: Capabilities | null | undefined, key: string): boolean {
  const state = caps?.features?.[key]
  if (state) return state.configured === true && state.available === true
  return caps?.[key] === true
}

export function featureReason(caps: Capabilities | null | undefined, key: string): string | undefined {
  return caps?.features?.[key]?.reason
}

export function legacyCapability(caps: Capabilities | null | undefined, key: string): boolean {
  return featureAvailable(caps, key)
}

export function deploymentProfile(caps: Capabilities | null | undefined): string {
  return typeof caps?.deployment_profile === 'string' ? caps.deployment_profile : 'full'
}

export function storageMode(caps: Capabilities | null | undefined): string {
  return typeof caps?.storage_mode === 'string' ? caps.storage_mode : 'local'
}

export function storageBackend(caps: Capabilities | null | undefined): string {
  return typeof caps?.storage_backend === 'string' ? caps.storage_backend : 'local'
}
