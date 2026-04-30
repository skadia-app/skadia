// frontend/src/components/segment-editor/constants.ts

export type SegType = 'standard' | 'trail' | 'lap' | 'wintersport' | 'wassersport' | 'lift'

/**
 * ClimbType maps to the segments.climb_type DB enum.
 * Affects leaderboard partitioning — bergauf/bergab segments get
 * separate leaderboard slots from standard (flat/mixed) segments.
 * Visible for segType ∈ {standard, trail}; hidden for lap/wintersport.
 */
export type ClimbType = 'standard' | 'bergauf' | 'bergab'

export const CLIMB_TYPE_LABELS: Record<ClimbType, string> = {
  standard: 'Eben/Gemischt',
  bergauf:  'Bergauf',
  bergab:   'Bergab',
}

export const BIDIRECTIONAL_DEFAULT: Record<SegType, boolean> = {
  standard: true,
  trail: false,
  lap: true,
  wintersport: false,
  wassersport: false,
  lift: true,
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

// WATER_SPORT_WHITELIST — analog zu WINTER_SPORT_WHITELIST
// Verhindert dass bei segType='wassersport' Bike/Run-Sportarten angeboten werden
// Quelle: sportTaxonomy.ts:129-163 categoryKey='water', ohne diving (hasGPS=false)
export const WATER_SPORT_WHITELIST: readonly string[] = [
  // swim
  'swimming',
  // paddle
  'paddling', 'kayaking', 'stand_up_paddleboarding', 'rowing', 'rafting', 'canoeing',
  // generic water (hasGPS=true only — diving excluded)
  'surfing', 'kitesurfing', 'windsurfing', 'wakeboarding', 'wakesurfing',
  'water_skiing', 'water_tubing', 'snorkeling', 'boating', 'sailing', 'fishing',
]

// DAV-Bayern-Auslegung — Ski-Pisten convention blau < rot < schwarz. §4.6.
// Single-Source-of-Truth: @/lib/scaleColors. Re-Export hier aus Abwärts-
// kompatibilität; Consumer die Hex-Werte brauchen (z.B. Badge-Dots) importieren
// direkt SAC_BADGE_COLORS/MTB_BADGE_COLORS aus @/lib/scaleColors.
export {
  SAC_DAV_COLOR_NAMES as SAC_COLOR_MAP,
  MTB_DAV_COLOR_NAMES as MTB_COLOR_MAP,
} from '@/lib/scaleColors'
import { SAC_DAV_COLOR_NAMES, MTB_DAV_COLOR_NAMES } from '@/lib/scaleColors'
import type { DavColorName } from '@/lib/scaleColors'

const COLOR_ORDER: Record<DavColorName, number> = { blue: 0, red: 1, black: 2 }

export function deriveTrailDifficultyColor(
  sac: string | null | undefined,
  mtb: string | null | undefined,
): DavColorName | null {
  const colors: DavColorName[] = []
  if (sac && SAC_DAV_COLOR_NAMES[sac]) colors.push(SAC_DAV_COLOR_NAMES[sac])
  if (mtb && MTB_DAV_COLOR_NAMES[mtb]) colors.push(MTB_DAV_COLOR_NAMES[mtb])
  if (colors.length === 0) return null
  return colors.reduce((max, c) => (COLOR_ORDER[c] > COLOR_ORDER[max] ? c : max))
}

// Labels that the editor offers in the Surface dropdown. §4.8 (v2 — post-user-feedback):
// - Beton merged into Asphalt (segment-enrichment backend maps concrete* → Asphalt)
// - Metall removed (metal bridges/grids rarely characterize a user-authored segment)
// - Holz removed (boardwalks/chips are incidental, not defining)
// - Naturweg collapsed to one entry (fest/weich sub-types removed — OSM tagging inconsistent)
// Backend surfaceIDToGroupForSegment enforces the same set on enrichment responses.
export const SURFACE_DROPDOWN_OPTIONS: readonly string[] = [
  'Asphalt',
  'Pflaster',
  'Schotter (fein)',
  'Schotter (grob)',
  'Naturweg',
  'Sand',
  'Gras',
  'Fels',
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

// Bewusst ohne Zusätze — verführerische Kurzbeschreibungen führen zu
// Selbstüberschätzung (siehe User-Feedback 2026-04-24). Die offizielle
// Skala ist auf https://www.singletrail-skala.de/ definiert.
export const MTB_SCALE_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: 'S0', label: 'S0' },
  { value: 'S1', label: 'S1' },
  { value: 'S2', label: 'S2' },
  { value: 'S3', label: 'S3' },
  { value: 'S4', label: 'S4' },
  { value: 'S5', label: 'S5' },
]
