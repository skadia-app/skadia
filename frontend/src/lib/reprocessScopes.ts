// GEO_SCOPE_KEYS: individual geo submodule names passed directly to the backend.
// Each key is sent as-is; the legacy aggregate "geo" is no longer sent from the
// frontend — the backend expands it internally for backwards compat.
//
// Keep in sync with services/api/internal/handler/re_enrich.go::geoSubmodules
// and services/api/internal/enrichment/modules.go::CanonicalModules.
// Vocab asymmetries documented there: re_enrich.go's geoSubmodules covers
// summits/settlements/forests/water/borders/surfaces; lifts and stops are
// recompute-only because they require the Stage-1 FIT reparse. 'borders'
// on the frontend collapses borders + settlements (recompute.go wires
// EnableSettlementDetection = doBorders).
export const GEO_SCOPE_KEYS = ['summits', 'forests', 'water', 'stops', 'lifts', 'surfaces', 'borders'] as const
export type GeoScopeKey = (typeof GEO_SCOPE_KEYS)[number]

export const GEO_SCOPE_LABELS: Record<GeoScopeKey, string> = {
  summits:  'Gipfel & Pässe',
  forests:  'Wälder',
  water:    'Gewässer',
  stops:    'Gastronomie / Einkehr',
  lifts:    'Lifte',
  surfaces: 'Oberflächen',
  borders:  'Grenzen & Siedlungen',
}

export type ScopeTuple = readonly [key: string, label: string, description: string | undefined]

export const REPROCESS_SCOPES: readonly ScopeTuple[] = [
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

export type ReprocessScopeKey = (typeof REPROCESS_SCOPES)[number][0]
export type AnyReprocessKey = ReprocessScopeKey | GeoScopeKey

export const DEFAULT_SCOPES: AnyReprocessKey[] = ['streams', 'sensors', 'metrics', 'details', 'segments']

export function toggleScopeKey(prev: Set<AnyReprocessKey>, key: AnyReprocessKey): Set<AnyReprocessKey> {
  const next = new Set(prev)
  if (next.has(key)) next.delete(key); else next.add(key)
  return next
}

export function toBackendScope(sel: Set<AnyReprocessKey>): string[] {
  // Pass each selected scope key directly. Geo submodule keys (summits, forests, …)
  // are sent individually so the backend can run only the selected modules.
  // Previously all geo keys were collapsed to "geo" — that aggregation is removed
  // so that e.g. selecting only "surfaces" actually runs only surface enrichment.
  return [...sel]
}
