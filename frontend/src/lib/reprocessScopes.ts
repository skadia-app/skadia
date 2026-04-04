// GEO_SCOPE_KEYS: alle Geo-Features triggern dasselbe "geo" Backend-Scope (Stage 2 läuft zusammen).
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
  const result = new Set<string>()
  for (const s of sel) {
    if ((GEO_SCOPE_KEYS as readonly string[]).includes(s)) {
      result.add('geo')
    } else {
      result.add(s)
    }
  }
  return [...result]
}
