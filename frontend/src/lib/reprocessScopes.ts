import { ANALYSIS_MODULES } from '@/lib/enrichmentModuleCatalog'
import type { Capabilities } from '@/api/routes'
import { featureAvailable, type CapabilityKey } from '@/lib/deploymentCapabilities'
import { i18n } from '@/i18n/i18n'

export type ScopeTuple = readonly [key: string, label: string, description: string | undefined]
const tr = (key: string) => i18n.t(key)

const BASE_REPROCESS_SCOPES: readonly ScopeTuple[] = [
  ['streams',      tr('reprocessScopes.items.streams.label'),      tr('reprocessScopes.items.streams.description')],
  ['sensors',      tr('reprocessScopes.items.sensors.label'),      tr('reprocessScopes.items.sensors.description')],
  ['metrics',      tr('reprocessScopes.items.metrics.label'),      tr('reprocessScopes.items.metrics.description')],
  ['details',      tr('reprocessScopes.items.details.label'),      tr('reprocessScopes.items.details.description')],
  ['segments',     tr('reprocessScopes.items.segments.label'),     tr('reprocessScopes.items.segments.description')],
  ['sport_type',   tr('reprocessScopes.items.sport_type.label'),   tr('reprocessScopes.items.sport_type.description')],
  ['tags',         tr('reprocessScopes.items.tags.label'),         tr('reprocessScopes.items.tags.description')],
  ['best_efforts', tr('reprocessScopes.items.best_efforts.label'), tr('reprocessScopes.items.best_efforts.description')],
  ['running_form', tr('reprocessScopes.items.running_form.label'), tr('reprocessScopes.items.running_form.description')],
] as const

const STREAM_ANALYSIS_SCOPES = ANALYSIS_MODULES
  .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'stream')
  .map(module => [
    module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name,
    tr(`enrichmentStatus.modules.${module.name}.label`),
    tr(`enrichmentStatus.modules.${module.name}.description`),
  ] as const)

const METADATA_ANALYSIS_SCOPES = ANALYSIS_MODULES
  .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'metadata')
  .map(module => [
    module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name,
    tr(`enrichmentStatus.modules.${module.name}.label`),
    tr(`enrichmentStatus.modules.${module.name}.description`),
  ] as const)

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
    .map(module => [module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name, tr(`enrichmentStatus.modules.${module.name}.label`)]),
) as Record<GeoScopeKey, string>

export const GEO_SCOPE_DESCRIPTIONS: Record<GeoScopeKey, string> = Object.fromEntries(
  ANALYSIS_MODULES
    .filter(module => module.reprocess.type === 'standalone' && module.reprocess.group === 'geo')
    .map(module => [module.reprocess.type === 'standalone' ? module.reprocess.scope : module.name, tr(`enrichmentStatus.modules.${module.name}.description`)]),
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
  return GEO_SCOPE_LABELS[key as GeoScopeKey] ?? tr('reprocessScopes.unknown')
}
