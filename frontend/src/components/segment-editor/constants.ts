// frontend/src/components/segment-editor/constants.ts

export type SegType = 'standard' | 'trail' | 'lap' | 'wintersport'

export const BIDIRECTIONAL_DEFAULT: Record<SegType, boolean> = {
  standard: true,
  trail: false,
  lap: true,
  wintersport: false,
}

// Hardcoded whitelist (NOT derived from taxonomy Category() === 'winter' —
// snowshoeing is CategoryKey 'run' per sport/taxonomy.go:103). §4.2 Item 5.
export const WINTER_SPORT_WHITELIST: readonly string[] = [
  'alpine_skiing',
  'backcountry_skiing',
  'cross_country_skiing',
  'snowboarding',
  'ice_skating',
  'snowmobiling',
  'sledding',
  'snowshoeing',
]

// DAV-Bayern-Auslegung — Ski-Pisten convention blau < rot < schwarz. §4.6.
export const SAC_COLOR_MAP: Record<string, 'blue' | 'red' | 'black'> = {
  T1: 'blue',
  T2: 'red', T3: 'red',
  T4: 'black', T5: 'black', T6: 'black',
}

export const MTB_COLOR_MAP: Record<string, 'blue' | 'red' | 'black'> = {
  S0: 'blue', S1: 'blue',
  S2: 'red',
  S3: 'black', S4: 'black', S5: 'black',
}

const COLOR_ORDER: Record<'blue' | 'red' | 'black', number> = { blue: 0, red: 1, black: 2 }

export function deriveTrailDifficultyColor(
  sac: string | null | undefined,
  mtb: string | null | undefined,
): 'blue' | 'red' | 'black' | null {
  const colors: Array<'blue' | 'red' | 'black'> = []
  if (sac && SAC_COLOR_MAP[sac]) colors.push(SAC_COLOR_MAP[sac])
  if (mtb && MTB_COLOR_MAP[mtb]) colors.push(MTB_COLOR_MAP[mtb])
  if (colors.length === 0) return null
  return colors.reduce((max, c) => (COLOR_ORDER[c] > COLOR_ORDER[max] ? c : max))
}

// Labels that the editor offers in the Surface dropdown. §4.8 (v2 — post-user-feedback):
// - Beton merged into Asphalt (OSM concrete* tags → 'Asphalt' in SURFACE_GROUPS)
// - Metall removed (not practically relevant for segments)
// - Naturweg collapsed to one entry (fest/weich sub-types removed — OSM tagging inconsistent)
export const SURFACE_DROPDOWN_OPTIONS: readonly string[] = [
  'Asphalt',
  'Pflaster',
  'Schotter (fein)',
  'Schotter (grob)',
  'Naturweg',
  'Sand',
  'Gras',
  'Fels',
  'Holz',
  'Schnee',
  'Sportbelag',
  'Wechselnd',          // editor-only, auto-set on mixed=true from enrichment
]

export const SAC_SCALE_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: 'T1', label: 'T1 — Wandern' },
  { value: 'T2', label: 'T2 — Bergwandern' },
  { value: 'T3', label: 'T3 — anspruchsvolles Bergwandern' },
  { value: 'T4', label: 'T4 — Alpinwandern' },
  { value: 'T5', label: 'T5 — anspruchsvolles Alpinwandern' },
  { value: 'T6', label: 'T6 — schwieriges Alpinwandern' },
]

export const MTB_SCALE_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: 'S0', label: 'S0 — leicht, befestigt' },
  { value: 'S1', label: 'S1 — leicht, kleine Hindernisse' },
  { value: 'S2', label: 'S2 — mittel, größere Hindernisse' },
  { value: 'S3', label: 'S3 — schwer, grobes Gelände' },
  { value: 'S4', label: 'S4 — sehr schwer, enge Spitzkehren' },
  { value: 'S5', label: 'S5 — extrem, Steilpassagen' },
]
