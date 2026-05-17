import { ANALYSIS_MODULES } from '@/lib/enrichmentModuleCatalog'
import type { Capabilities } from '@/api/routes'
import { featureAvailable, type CapabilityKey } from '@/lib/deploymentCapabilities'

export type ScopeTuple = readonly [key: string, label: string, description: string | undefined]

const BASE_REPROCESS_SCOPES: readonly ScopeTuple[] = [
  ['streams',      'Streams & Track',   'GPS-Track, Zeitstempel und alle Sensor-Zeitreihen neu importieren.'],
  ['sensors',      'Sensoren',          'Gerätezuordnungen und Sensor-Metadaten neu auflösen.'],
  ['metrics',      'Metriken',          'Zusammenfassende Kennzahlen (Power, NP, TSS, Pace etc.) neu berechnen.'],
  ['details',      'Runden/Splits',     'Runden und Split-Zeiten aus den FIT-Daten neu auslesen.'],
  ['segments',     'Segmente & Runden', 'Alle Segment-Matches und Rundenzeiten neu berechnen.'],
  ['sport_type',   'Sportart',          'Sportart aus den FIT-Daten neu erkennen und setzen.'],
  ['tags',         'Tags',              'Automatische Tags basierend auf der Aktivität neu vergeben.'],
  ['best_efforts', 'Bestleistungen',    'Distanz-, Höhenmeter- und Power-Bestleistungen neu berechnen.'],
  ['running_form', 'Laufform-Analyse',  'Duty Factor, Vertical Ratio und Fatigue Index neu berechnen.'],
] as const

const STREAM_ANALYSIS_SCOPES = ANALYSIS_MODULES
  .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'stream')
  .map(module => [module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name, module.label, module.description] as const)

const METADATA_ANALYSIS_SCOPES = ANALYSIS_MODULES
  .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'metadata')
  .map(module => [module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name, module.label, module.description] as const)

export const REPROCESS_SCOPES: readonly ScopeTuple[] = [
  ...BASE_REPROCESS_SCOPES,
  ...METADATA_ANALYSIS_SCOPES,
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

export const GEO_SCOPE_DESCRIPTIONS: Record<GeoScopeKey, string> = Object.fromEntries(
  ANALYSIS_MODULES
    .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'geo')
    .map(module => [module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name, module.description]),
) as Record<GeoScopeKey, string>

export type ReprocessScopeKey = (typeof REPROCESS_SCOPES)[number][0]
export type AnyReprocessKey = ReprocessScopeKey | GeoScopeKey

export const ALL_REPROCESS_SCOPE_KEYS: readonly AnyReprocessKey[] = [
  ...REPROCESS_SCOPES.map(([key]) => key),
  ...GEO_SCOPE_KEYS,
] as const

export const DEFAULT_SCOPES: AnyReprocessKey[] = ['streams', 'sensors', 'metrics', 'details', 'segments', 'auto_title']

export function scopeRequiredCapability(key: string): CapabilityKey | undefined {
  const module = ANALYSIS_MODULES.find(item =>
    (item.reprocess.type === 'standalone' || item.reprocess.type === 'coveredBy') &&
    item.reprocess.scope === key)
  return module?.requiresCapability
}

export function scopeAvailable(caps: Capabilities | null | undefined, key: string): boolean {
  const capability = scopeRequiredCapability(key)
  return capability === undefined || featureAvailable(caps, capability)
}

export function availableReprocessScopeKeys(caps: Capabilities | null | undefined): AnyReprocessKey[] {
  return ALL_REPROCESS_SCOPE_KEYS.filter(key => scopeAvailable(caps, key))
}

export function toggleScopeKey(prev: Set<AnyReprocessKey>, key: AnyReprocessKey): Set<AnyReprocessKey> {
  const next = new Set(prev)
  if (next.has(key)) next.delete(key); else next.add(key)
  return next
}

export function toBackendScope(sel: Set<AnyReprocessKey>, allowedKeys?: readonly AnyReprocessKey[]): string[] {
  if (!allowedKeys) return [...sel]
  const allowed = new Set(allowedKeys)
  return [...sel].filter(key => allowed.has(key))
}

export function scopeLabel(key: string): string {
  const base = REPROCESS_SCOPES.find(([scope]) => scope === key)
  if (base) return base[1]
  return GEO_SCOPE_LABELS[key as GeoScopeKey] ?? key
}
