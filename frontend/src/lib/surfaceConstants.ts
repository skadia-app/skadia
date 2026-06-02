// Surface grouping: merge similar OSM surface types into display groups
import { i18n } from '@/i18n/i18n'

const sf = (key: string) => i18n.t(`surfaceConstants.surfaces.${key}`)
const hw = (key: string) => i18n.t(`surfaceConstants.highways.${key}`)

export const SURFACE_GROUPS: Record<string, string> = {
  asphalt: sf('asphalt'),
  chipseal: sf('asphalt'),
  concrete: sf('concrete'), concrete_lanes: sf('concrete'), concrete_plates: sf('concrete'),
  paved: sf('paved'),
  paving_stones: sf('pavingStones'), sett: sf('pavingStones'), cobblestone: sf('pavingStones'),
  unhewn_cobblestone: sf('pavingStones'), bricks: sf('pavingStones'), brick: sf('pavingStones'),
  compacted: sf('fineGravel'),
  fine_gravel: sf('coarseGravel'),
  gravel: sf('coarseGravel'),
  pebblestone: sf('coarseGravel'),
  ground: sf('natural'), earth: sf('natural'), dirt: sf('natural'),
  mud: sf('natural'), clay: sf('natural'),
  stepping_stones: sf('natural'),
  sand: sf('sand'), shells: sf('sand'),
  grass: sf('grass'), grass_paver: sf('grass'),
  wood: sf('wood'), woodchips: sf('wood'),
  rock: sf('rock'), stone: sf('rock'),
  metal: sf('metal'), metal_grid: sf('metal'),
  snow: sf('snow'), ice: sf('snow'),
  tartan: sf('sportSurface'), artificial_turf: sf('sportSurface'), rubber: sf('sportSurface'),
  decoturf: sf('sportSurface'), carpet: sf('sportSurface'), acrylic: sf('sportSurface'),
  unpaved: sf('unpaved'),
  unknown: sf('unknown'),
  offroad: sf('offroad'),
}

export const SURFACE_COLORS: Record<string, string> = {
  [sf('asphalt')]: '#5b5b5b',
  [sf('concrete')]: '#7a7a7a',
  [sf('paved')]: '#8b8680',
  [sf('pavingStones')]: '#6b6558',
  [sf('gravel')]: '#998b6a',           // legacy fallback — keep during migration window
  [sf('fineGravel')]: '#a8a07e',
  [sf('coarseGravel')]: '#8e8670',
  [sf('natural')]: '#8a6b3e',
  [sf('naturalFirm')]: '#a0784a',
  [sf('naturalSoft')]: '#6b5430',
  [sf('sand')]: '#d4b886',
  [sf('grass')]: '#6b8b3e',
  [sf('wood')]: '#8b6b3e',
  [sf('rock')]: '#6b6b6b',
  [sf('metal')]: '#9ba0a8',
  [sf('snow')]: '#d8e8f0',
  [sf('sportSurface')]: '#c44532',
  [sf('unpaved')]: '#8b7a5a',
  [sf('unknown')]: '#888888',
  [sf('offroad')]: '#6b5a3e',
  [sf('mixed')]: '#b59b5c',
}

export const HIGHWAY_COLORS: Record<string, string> = {
  // Paved roads — gray shades from dark to light by importance.
  motorway: '#1f2937',       // fast schwarz
  trunk: '#374151',          // sehr dunkel
  primary: '#c4a30a',        // muted yellow
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
  // Unpaved ways — earth tones
  track: '#c2956a',          // sandy beige (field track)
  path: '#92400e',           // brown (hiking path)
  bridleway: '#854d0e',      // dark brown (bridleway)
  // Bike and pedestrian ways — blue tones
  footway: '#1d4ed8',        // dark blue
  cycleway: '#2563eb',       // blue (cycleway sign)
  pedestrian: '#3b82f6',     // lighter blue
  // Other
  steps: '#78716c',          // stone gray
  via_ferrata: '#a21caf',    // purple (via ferrata)
  offroad: '#b91c1c',        // red
  unknown: '#d1d5db',
}

export const HIGHWAY_LABELS: Record<string, string> = {
  primary: hw('primary'),
  secondary: hw('secondary'),
  tertiary: hw('tertiary'),
  unclassified: hw('unclassified'),
  residential: hw('residential'),
  living_street: hw('livingStreet'),
  service: hw('service'),
  track: hw('track'),
  path: hw('path'),
  footway: hw('footway'),
  cycleway: hw('cycleway'),
  bridleway: hw('bridleway'),
  steps: hw('steps'),
  pedestrian: hw('pedestrian'),
  motorway: hw('fastRoad'),
  trunk: hw('fastRoad'),
  offroad: hw('offroad'),
  motorway_link: hw('fastRoad'),
  trunk_link: hw('fastRoad'),
  primary_link: hw('primary'),
  secondary_link: hw('secondary'),
  tertiary_link: hw('tertiary'),
  via_ferrata: hw('viaFerrata'),
  unknown: hw('unknown'),
}

export interface AggregatedSurface {
  surface_type: string
  length_m: number
  percentage: number
  /** Raw OSM types that were merged into this display group (only set for '__other__' bucket). */
  overflowRawTypes?: string[]
}

// Always show at least MIN_SHOWN surface types before collapsing to the other bucket,
// regardless of their percentage. This prevents routes that are 98% asphalt
// from hiding all other surfaces (gravel, compacted, etc.) in a single
// small other bucket.
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

  // Show at least MIN_SHOWN entries, up to MAX_DISPLAYED; the remainder goes into the other bucket.
  const showCount = Math.min(Math.max(sorted.length, MIN_SHOWN), MAX_DISPLAYED)
  let displayed = sorted.slice(0, showCount)
  const overflow = sorted.slice(showCount)

  const otherLength = overflow.reduce((a, s) => a + s.length_m, 0)
  if (otherLength > 0) {
    // Collect the raw OSM types that contributed to this display group so that
    // callers can union their ranges for the other-bucket hover highlight.
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
