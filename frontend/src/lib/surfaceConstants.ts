// Surface grouping: merge similar OSM surface types into display groups
export const SURFACE_GROUPS: Record<string, string> = {
  asphalt: 'Asphalt',
  concrete: 'Beton', concrete_lanes: 'Beton', concrete_plates: 'Beton',
  paved: 'Befestigt',
  paving_stones: 'Pflaster', sett: 'Pflaster', cobblestone: 'Pflaster',
  unhewn_cobblestone: 'Pflaster', bricks: 'Pflaster',
  compacted: 'Schotter', fine_gravel: 'Schotter', gravel: 'Schotter', pebblestone: 'Schotter',
  ground: 'Naturweg', earth: 'Naturweg', dirt: 'Naturweg', mud: 'Naturweg', clay: 'Naturweg',
  sand: 'Sand', shells: 'Sand',
  grass: 'Gras', grass_paver: 'Gras',
  wood: 'Holz', woodchips: 'Holz',
  rock: 'Fels',
  metal: 'Metall', metal_grid: 'Metall',
  snow: 'Schnee', ice: 'Schnee',
  tartan: 'Sportbelag', artificial_turf: 'Sportbelag', rubber: 'Sportbelag',
  unpaved: 'Unbefestigt',
  unknown: 'Unbekannt',
  offroad: 'Offroad',
}

export const SURFACE_COLORS: Record<string, string> = {
  'Asphalt': '#374151',      // dunkelgrau
  'Beton': '#9ca3af',        // hellgrau
  'Befestigt': '#6b7280',    // mittelgrau
  'Pflaster': '#78716c',     // warmgrau/stein
  'Schotter': '#94a3b8',     // blaugrau (Kalkstein/Schiefer)
  'Naturweg': '#8b6c42',     // erdig-braun
  'Sand': '#dbb778',         // sandgelb
  'Gras': '#4d7c0f',         // natürliches grün
  'Holz': '#92400e',         // holzbraun
  'Fels': '#6b7280',         // felsgrau
  'Metall': '#71717a',       // zinkgrau
  'Schnee': '#e2e8f0',       // fast weiß
  'Sportbelag': '#dc2626',   // rot
  'Unbefestigt': '#a8885a',  // unspezifisch braun
  'Unbekannt': '#d1d5db',    // neutralgrau
  'Offroad': '#b91c1c',      // rot
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
    displayed = [
      ...displayed,
      {
        surface_type: '__other__',
        length_m: otherLength,
        percentage: (otherLength / totalLength) * 100,
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
      },
    ]
  }

  return displayed
}

export function formatLength(m: number): string {
  return m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${Math.round(m)} m`
}
