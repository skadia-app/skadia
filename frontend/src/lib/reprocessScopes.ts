import { ANALYSIS_MODULES } from '@/lib/enrichmentModuleCatalog'

export type ScopeTuple = readonly [key: string, label: string, description: string | undefined]

const BASE_REPROCESS_SCOPES: readonly ScopeTuple[] = [
  ['streams',      'Streams & Track',   undefined],
  ['sensors',      'Sensoren',          undefined],
  ['metrics',      'Metriken',          undefined],
  ['details',      'Runden/Splits',     undefined],
  ['segments',     'Segmente & Runden', undefined],
  ['sport_type',   'Sportart',          undefined],
  ['tags',         'Tags',              undefined],
  ['best_efforts', 'Bestleistungen',    'Distanz-, Höhenmeter- und Power-Bestleistungen neu berechnen'],
  ['running_form', 'Laufform-Analyse',  'Duty Factor, Vertical Ratio und Fatigue Index neu berechnen'],
] as const

const STREAM_ANALYSIS_SCOPES = ANALYSIS_MODULES
  .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'stream')
  .map(module => [module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name, module.label, module.description] as const)

export const REPROCESS_SCOPES: readonly ScopeTuple[] = [
  ...BASE_REPROCESS_SCOPES,
  ...STREAM_ANALYSIS_SCOPES,
] as const

export const GEO_SCOPE_KEYS = ANALYSIS_MODULES
  .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'geo')
  .map(module => module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name)

export type GeoScopeKey = (typeof GEO_SCOPE_KEYS)[number]

export const GEO_SCOPE_LABELS: Record<GeoScopeKey, string> = Object.fromEntries(
  ANALYSIS_MODULES
    .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'geo')
    .map(module => [module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name, module.label]),
) as Record<GeoScopeKey, string>

export type ReprocessScopeKey = (typeof REPROCESS_SCOPES)[number][0]
export type AnyReprocessKey = ReprocessScopeKey | GeoScopeKey

export const ALL_REPROCESS_SCOPE_KEYS: readonly AnyReprocessKey[] = [
  ...REPROCESS_SCOPES.map(([key]) => key),
  ...GEO_SCOPE_KEYS,
] as const

export const DEFAULT_SCOPES: AnyReprocessKey[] = ['streams', 'sensors', 'metrics', 'details', 'segments']

export function toggleScopeKey(prev: Set<AnyReprocessKey>, key: AnyReprocessKey): Set<AnyReprocessKey> {
  const next = new Set(prev)
  if (next.has(key)) next.delete(key); else next.add(key)
  return next
}

export function toBackendScope(sel: Set<AnyReprocessKey>): string[] {
  return [...sel]
}

export function scopeLabel(key: string): string {
  const base = REPROCESS_SCOPES.find(([scope]) => scope === key)
  if (base) return base[1]
  return GEO_SCOPE_LABELS[key as GeoScopeKey] ?? key
}
