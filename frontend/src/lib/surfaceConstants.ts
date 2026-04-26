// Surface grouping: merge similar OSM surface types into display groups
export const SURFACE_GROUPS: Record<string, string> = {
  asphalt: 'Asphalt',
  chipseal: 'Asphalt',
  concrete: 'Beton', concrete_lanes: 'Beton', concrete_plates: 'Beton',
  paved: 'Befestigt',
  paving_stones: 'Pflaster', sett: 'Pflaster', cobblestone: 'Pflaster',
  unhewn_cobblestone: 'Pflaster', bricks: 'Pflaster', brick: 'Pflaster',
  compacted: 'Schotter (fein)',
  fine_gravel: 'Schotter (grob)',
  gravel: 'Schotter (grob)',
  pebblestone: 'Schotter (grob)',
  ground: 'Naturweg', earth: 'Naturweg', dirt: 'Naturweg',
  mud: 'Naturweg', clay: 'Naturweg',
  stepping_stones: 'Naturweg',
  sand: 'Sand', shells: 'Sand',
  grass: 'Gras', grass_paver: 'Gras',
  wood: 'Holz', woodchips: 'Holz',
  rock: 'Fels', stone: 'Fels',
  metal: 'Metall', metal_grid: 'Metall',
  snow: 'Schnee', ice: 'Schnee',
  tartan: 'Sportbelag', artificial_turf: 'Sportbelag', rubber: 'Sportbelag',
  decoturf: 'Sportbelag', carpet: 'Sportbelag', acrylic: 'Sportbelag',
  unpaved: 'Unbefestigt',
  unknown: 'Unbekannt',
  offroad: 'Offroad',
}

export const SURFACE_COLORS: Record<string, string> = {
  'Asphalt': '#5b5b5b',
  'Beton': '#7a7a7a',
  'Befestigt': '#8b8680',
  'Pflaster': '#6b6558',
  'Schotter': '#998b6a',           // legacy fallback — keep during migration window
  'Schotter (fein)': '#a8a07e',
  'Schotter (grob)': '#8e8670',
  'Naturweg': '#8a6b3e',
  'Naturweg (fest)': '#a0784a',
  'Naturweg (weich)': '#6b5430',
  'Sand': '#d4b886',
  'Gras': '#6b8b3e',
  'Holz': '#8b6b3e',
  'Fels': '#6b6b6b',
  'Metall': '#9ba0a8',
  'Schnee': '#d8e8f0',
  'Sportbelag': '#c44532',
  'Unbefestigt': '#8b7a5a',
  'Unbekannt': '#888888',
  'Offroad': '#6b5a3e',
  'Wechselnd': '#b59b5c',
}

export const HIGHWAY_COLORS: Record<string, string> = {
  // Asphaltierte Straßen — Grautöne (dunkel → hell nach Bedeutung)
  motorway: '#1f2937',       // fast schwarz
  trunk: '#374151',          // sehr dunkel
  primary: '#c4a30a',        // gedämpftes gelb (Bundesstraßen-Schild)
  secondary: '#6b7280',      // mittelgrau
  tertiary: '#9ca3af',       // hellgrau
  unclassified: '#9ca3af',
  residential: '#d1d5db',    // sehr hell
  living_street: '#d1d5db',
  service: '#9ca3af',
  motorway_link: '#1f2937',
  trunk_link: '#374151',
  primary_link: '#c4a30a',
  secondary_link: '#6b7280',
  tertiary_link: '#9ca3af',
  // Unbefestigte Wege — Erdtöne
  track: '#c2956a',          // sandig-beige (Feldweg)
  path: '#92400e',           // braun (Wanderpfad)
  bridleway: '#854d0e',      // dunkler braun (Reitweg)
  // Rad-/Fußweg — Blautöne (Verkehrsschilder)
  footway: '#1d4ed8',        // dunkelblau (Fußgänger-Schild)
  cycleway: '#2563eb',       // blau (Radweg-Schild)
  pedestrian: '#3b82f6',     // heller blau (Fußgängerzone)
  // Sonstiges
  steps: '#78716c',          // steingrau
  via_ferrata: '#a21caf',    // lila (Klettersteig)
  offroad: '#b91c1c',        // rot
  unknown: '#d1d5db',
}

export const HIGHWAY_LABELS: Record<string, string> = {
  primary: 'Bundesstraße',
  secondary: 'Landstraße',
  tertiary: 'Kreisstraße',
  unclassified: 'Gemeindestraße',
  residential: 'Wohnstraße',
  living_street: 'Spielstraße',
  service: 'Zufahrtsweg',
  track: 'Feldweg',
  path: 'Pfad',
  footway: 'Fußweg',
  cycleway: 'Radweg',
  bridleway: 'Reitweg',
  steps: 'Treppen',
  pedestrian: 'Fußgängerzone',
  motorway: 'Schnellstraße',
  trunk: 'Schnellstraße',
  offroad: 'Offroad',
  motorway_link: 'Schnellstraße',
  trunk_link: 'Schnellstraße',
  primary_link: 'Bundesstraße',
  secondary_link: 'Landstraße',
  tertiary_link: 'Kreisstraße',
  via_ferrata: 'Klettersteig',
  unknown: 'Unbekannt',
}

export interface AggregatedSurface {
  surface_type: string
  length_m: number
  percentage: number
  /** Raw OSM types that were merged into this display group (only set for '__other__' bucket). */
  overflowRawTypes?: string[]
}

// Always show at least MIN_SHOWN surface types before collapsing to "Sonstige",
// regardless of their percentage. This prevents routes that are 98% asphalt
// from hiding all other surfaces (gravel, compacted, etc.) in a single
// "Sonstige 2%" bucket.
export const MAX_DISPLAYED = 7
export const MIN_SHOWN = 5

export function aggregateSurfaces(
  raw: { surface_type: string; length_m: number }[]
): AggregatedSurface[] {
  // Group by display group (merging similar OSM surface types)
  const map = new Map<string, number>()
  for (const s of raw) {
    const group = SURFACE_GROUPS[s.surface_type] ?? s.surface_type
    map.set(group, (map.get(group) ?? 0) + s.length_m)
  }

  const totalLength = Array.from(map.values()).reduce((a, b) => a + b, 0)
  if (totalLength === 0) return []

  // Build sorted list (descending by length)
  const sorted: AggregatedSurface[] = Array.from(map.entries())
    .map(([surface_type, length_m]) => ({
      surface_type,
      length_m,
      percentage: (length_m / totalLength) * 100,
    }))
    .sort((a, b) => b.length_m - a.length_m)

  // Show at least MIN_SHOWN entries, up to MAX_DISPLAYED — remainder → "Sonstige"
  const showCount = Math.min(Math.max(sorted.length, MIN_SHOWN), MAX_DISPLAYED)
  let displayed = sorted.slice(0, showCount)
  const overflow = sorted.slice(showCount)

  const otherLength = overflow.reduce((a, s) => a + s.length_m, 0)
  if (otherLength > 0) {
    // Collect the raw OSM types that contributed to this display group so that
    // callers can union their ranges for the "Sonstige" hover highlight.
    displayed = [
      ...displayed,
      {
        surface_type: '__other__',
        length_m: otherLength,
        percentage: (otherLength / totalLength) * 100,
        overflowRawTypes: overflow.map(s => s.surface_type),
      },
    ]
  }

  return displayed
}

export function aggregateHighways(
  raw: { path_type: string; length_m: number }[]
): AggregatedSurface[] {
  // Group by path_type, summing length_m
  const map = new Map<string, number>()
  for (const s of raw) {
    map.set(s.path_type, (map.get(s.path_type) ?? 0) + s.length_m)
  }

  const totalLength = Array.from(map.values()).reduce((a, b) => a + b, 0)
  if (totalLength === 0) return []

  const sorted: AggregatedSurface[] = Array.from(map.entries())
    .map(([surface_type, length_m]) => ({
      surface_type,
      length_m,
      percentage: (length_m / totalLength) * 100,
    }))
    .sort((a, b) => b.length_m - a.length_m)

  const showCount = Math.min(Math.max(sorted.length, MIN_SHOWN), MAX_DISPLAYED)
  let displayed = sorted.slice(0, showCount)
  const overflow = sorted.slice(showCount)

  const otherLength = overflow.reduce((a, s) => a + s.length_m, 0)
  if (otherLength > 0) {
    displayed = [
      ...displayed,
      {
        surface_type: '__other__',
        length_m: otherLength,
        percentage: (otherLength / totalLength) * 100,
        // For highways, overflow types are exact OSM highway keys (not display groups),
        // so they can be looked up directly in highway_ranges.
        overflowRawTypes: overflow.map(s => s.surface_type),
      },
    ]
  }

  return displayed
}

export function formatLength(m: number): string {
  return m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${Math.round(m)} m`
}
