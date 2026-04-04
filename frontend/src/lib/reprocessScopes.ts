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

// 3-tuple: [key, label, description | undefined]
// description wird in Einzelaktivitäts-Modals angezeigt, ist optional für Bulk-Ansichten.
export const REPROCESS_SCOPES = [
  ['streams',      'Streams & Track',   undefined],
  ['sensors',      'Sensoren',          undefined],
  ['metrics',      'Metriken',          undefined],
  ['details',      'Runden/Splits',     undefined],
  ['segments',     'Segmente & Runden', undefined],
  ['sport_type',   'Sportart',          undefined],
  ['tags',         'Tags',              undefined],
  ['best_efforts', 'Best Efforts',      'Distanz-, Höhenmeter- und Power-Bestleistungen neu berechnen'],
  ['running_form', 'Laufform-Analyse',  'Duty Factor, Vertical Ratio und Fatigue Index neu berechnen'],
] as const

export type ReprocessScopeKey = (typeof REPROCESS_SCOPES)[number][0]
export type AnyReprocessKey = ReprocessScopeKey | GeoScopeKey

// Geo-Keys in "geo" Backend-Scope übersetzen (Stage 2 läuft immer als Ganzes).
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
