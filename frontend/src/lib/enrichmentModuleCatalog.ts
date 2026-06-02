import type { EnrichmentSettings } from '@/api/types'
import type { CapabilityKey } from './deploymentCapabilities'

export type EnrichmentModuleName =
  | 'auto_naming'
  | 'summits'
  | 'forests'
  | 'water'
  | 'settlements'
  | 'borders'
  | 'stops'
  | 'lifts'
  | 'lifts_osm'
  | 'lifts_heuristic'
  | 'surfaces'
  | 'ascents'
  | 'intervals'
  | 'lap_detection'
  | 'embeddings'

export type EnrichmentModuleLabelKey = `enrichmentStatus.modules.${EnrichmentModuleName}.label`
export type EnrichmentModuleDescriptionKey = `enrichmentStatus.modules.${EnrichmentModuleName}.description`

export type ReprocessPolicy =
  | { type: 'standalone'; scope: string; group: 'stream' | 'geo' | 'metadata' }
  | { type: 'coveredBy'; scope: string }
  | { type: 'stage1Covered' }
  | { type: 'inherited' }
  | { type: 'notReprocessable'; reason: string }

export interface AnalysisModule {
  name: EnrichmentModuleName
  parentName?: EnrichmentModuleName
  labelKey: EnrichmentModuleLabelKey
  descriptionKey: EnrichmentModuleDescriptionKey
  field: keyof EnrichmentSettings
  requiresML: boolean
  requiresCapability?: CapabilityKey
  reprocess: ReprocessPolicy
  icon: string
}

function moduleTextKeys(name: EnrichmentModuleName): Pick<AnalysisModule, 'labelKey' | 'descriptionKey'> {
  return {
    labelKey: `enrichmentStatus.modules.${name}.label` as EnrichmentModuleLabelKey,
    descriptionKey: `enrichmentStatus.modules.${name}.description` as EnrichmentModuleDescriptionKey,
  }
}

export const ANALYSIS_MODULES: readonly AnalysisModule[] = [
  {
    name: 'auto_naming',
    ...moduleTextKeys('auto_naming'),
    field: 'enable_auto_naming',
    requiresML: false,
    reprocess: { type: 'standalone', scope: 'auto_title', group: 'metadata' },
    icon: 'M4 6h16M4 12h10M4 18h7',
  },
  {
    name: 'summits',
    ...moduleTextKeys('summits'),
    field: 'enable_summit_detection',
    requiresML: false,
    requiresCapability: 'geo_poi_enrichment',
    reprocess: { type: 'standalone', scope: 'summits', group: 'geo' },
    icon: 'M3 17l4-8 4 4 4-4 4 8H3z',
  },
  {
    name: 'forests',
    ...moduleTextKeys('forests'),
    field: 'enable_forest_detection',
    requiresML: false,
    requiresCapability: 'geo_poi_enrichment',
    reprocess: { type: 'standalone', scope: 'forests', group: 'geo' },
    icon: 'M12 2L7 10h3l-4 6h4l-3 4h10l-3-4h4l-4-6h3L12 2z',
  },
  {
    name: 'water',
    ...moduleTextKeys('water'),
    field: 'enable_water_detection',
    requiresML: false,
    requiresCapability: 'geo_poi_enrichment',
    reprocess: { type: 'standalone', scope: 'water', group: 'geo' },
    icon: 'M2 12c2-3 4-3 6 0s4 3 6 0 4-3 6 0M2 17c2-3 4-3 6 0s4 3 6 0 4-3 6 0',
  },
  {
    name: 'settlements',
    ...moduleTextKeys('settlements'),
    field: 'enable_settlement_detection',
    requiresML: false,
    requiresCapability: 'geo_poi_enrichment',
    reprocess: { type: 'standalone', scope: 'settlements', group: 'geo' },
    icon: 'M3 21h18M5 21V7l7-4 7 4v14M9 21v-6h6v6',
  },
  {
    name: 'borders',
    ...moduleTextKeys('borders'),
    field: 'enable_border_detection',
    requiresML: false,
    requiresCapability: 'geo_poi_enrichment',
    reprocess: { type: 'standalone', scope: 'borders', group: 'geo' },
    icon: 'M4 4v16M4 4h4v4H4M4 12h4v4H4M12 4h4v4h-4M12 12h4v4h-4M20 4v16',
  },
  {
    name: 'stops',
    ...moduleTextKeys('stops'),
    field: 'enable_stop_detection',
    requiresML: false,
    requiresCapability: 'geo_poi_enrichment',
    reprocess: { type: 'standalone', scope: 'stops', group: 'stream' },
    icon: 'M12 2a7 7 0 00-7 7c0 5 7 13 7 13s7-8 7-13a7 7 0 00-7-7zm0 10a3 3 0 110-6 3 3 0 010 6z',
  },
  {
    name: 'lifts',
    ...moduleTextKeys('lifts'),
    field: 'enable_lift_detection',
    requiresML: false,
    reprocess: { type: 'standalone', scope: 'lifts', group: 'geo' },
    icon: 'M4 4v16h2V4H4zm14 0v16h2V4h-2zM8 8l4-4 4 4M12 4v12',
  },
  {
    name: 'lifts_osm',
    parentName: 'lifts',
    ...moduleTextKeys('lifts_osm'),
    field: 'enable_lift_detection_osm',
    requiresML: false,
    requiresCapability: 'osm_lift_snap',
    reprocess: { type: 'inherited' },
    icon: 'M4 4v16h2V4H4zm14 0v16h2V4h-2zM8 8l4-4 4 4M12 4v12',
  },
  {
    name: 'lifts_heuristic',
    parentName: 'lifts',
    ...moduleTextKeys('lifts_heuristic'),
    field: 'enable_lift_detection_heuristic',
    requiresML: false,
    reprocess: { type: 'inherited' },
    icon: 'M4 4v16h2V4H4zm14 0v16h2V4h-2zM8 8l4-4 4 4M12 4v12',
  },
  {
    name: 'surfaces',
    ...moduleTextKeys('surfaces'),
    field: 'enable_surface_detection',
    requiresML: false,
    requiresCapability: 'surface_enrichment',
    reprocess: { type: 'standalone', scope: 'surfaces', group: 'geo' },
    icon: 'M2 20h20M4 20V10l4-4 4 6 4-8 4 6v10',
  },
  {
    name: 'ascents',
    ...moduleTextKeys('ascents'),
    field: 'enable_ascent_detection',
    requiresML: false,
    reprocess: { type: 'standalone', scope: 'ascents', group: 'stream' },
    icon: 'M3 17l5-10 4 6 4-8 5 12',
  },
  {
    name: 'intervals',
    ...moduleTextKeys('intervals'),
    field: 'enable_interval_detection',
    requiresML: false,
    reprocess: { type: 'standalone', scope: 'intervals', group: 'stream' },
    icon: 'M4 18V6M8 18v-8M12 18V4M16 18v-6M20 18v-10',
  },
  {
    name: 'lap_detection',
    ...moduleTextKeys('lap_detection'),
    field: 'enable_lap_detection',
    requiresML: false,
    reprocess: { type: 'coveredBy', scope: 'segments' },
    icon: 'M12 6v6l4 2M5 12a7 7 0 1014 0 7 7 0 00-14 0z',
  },
  {
    name: 'embeddings',
    ...moduleTextKeys('embeddings'),
    field: 'enable_embeddings',
    requiresML: true,
    reprocess: { type: 'notReprocessable', reason: 'Route embeddings are not active in the ML path yet.' },
    icon: 'M12 2a10 10 0 100 20 10 10 0 000-20zm0 4a3 3 0 110 6 3 3 0 010-6z',
  },
] as const

export const MODULE_API_FIELD: Partial<Record<string, keyof EnrichmentSettings>> =
  Object.fromEntries(ANALYSIS_MODULES.map(module => [module.name, module.field]))

export const MODULE_META: Record<string, { labelKey: EnrichmentModuleLabelKey; icon: string }> =
  Object.fromEntries(ANALYSIS_MODULES.map(module => [module.name, { labelKey: module.labelKey, icon: module.icon }]))

export function reprocessScopeForModule(moduleName: string): string[] | null {
  const module = ANALYSIS_MODULES.find(item => item.name === moduleName)
  if (!module) return null

  switch (module.reprocess.type) {
    case 'standalone':
    case 'coveredBy':
      return [module.reprocess.scope]
    case 'stage1Covered':
    case 'notReprocessable':
    case 'inherited':
      return null
  }
}

export function scopeRequiresML(scope: string): boolean {
  return ANALYSIS_MODULES.some(module =>
    module.requiresML &&
    module.reprocess.type === 'standalone' &&
    module.reprocess.scope === scope)
}
