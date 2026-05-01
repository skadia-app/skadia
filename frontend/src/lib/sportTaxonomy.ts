// Sport Taxonomy — Single Source of Truth for all sport-related display logic
// in the frontend. Must match the backend Go taxonomy exactly (same sport_type
// keys, category keys).

import { GENERATED_CATEGORIES, GENERATED_SPORT_DEFS } from './generatedSportTaxonomy'

// ── Types ────────────────────────────────────────────────────────────────────────

export interface SportCategory {
  key: string
  label: string
  color: string
}

export interface SubTypeDef {
  subType: string
  label: string
  detail: string
  movingThreshold?: number
  selectable: boolean
}

export interface SportDef {
  sportType: string
  categoryKey: string
  segmentGroup: 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic'
  label: string
  typicallyHasGPS: boolean
  isIndoor: boolean
  lifecycle: 'canonical' | 'broad_fallback' | 'deprecated'
  selectable: boolean
  subTypes: SubTypeDef[]
}

export interface SportTuple {
  sportType: string
  subType: string
  modifiers: {
    isElectric?: boolean
    isIndoor?: boolean
  }
}

export interface SportLabelOptions {
  mode?: 'compact' | 'withParent'
}

export interface SportOption {
  value: string
  label: string
}

export interface WaterSportUIGroup {
  key: string
  label: string
  segmentGroups: SportDef['segmentGroup'][]
}

export interface RouteSportOption {
  sportType: string
  subType?: string
  label: string
}

// ── Categories ──────────────────────────────────────────────────────────────────

export const CATEGORIES: SportCategory[] = GENERATED_CATEGORIES.map((c) => ({ ...c }))

export const WATER_SPORT_UI_GROUPS: WaterSportUIGroup[] = [
  { key: 'water_swim', label: 'Schwimmen', segmentGroups: ['swim'] },
  { key: 'water_paddle', label: 'Paddeln', segmentGroups: ['paddle'] },
  { key: 'water_surf', label: 'Surfen & Segeln', segmentGroups: ['generic'] },
]

const categoryMap = new Map<string, SportCategory>(CATEGORIES.map((c) => [c.key, c]))

// ── Sport Definitions ───────────────────────────────────────────────────────────

const FRONTEND_SPORT_ORDER = [
  'running',
  'walking',
  'hiking',
  'mountaineering',
  'wheelchair_push',
  'cycling',
  'inline_skating',
  'alpine_skiing',
  'backcountry_skiing',
  'cross_country_skiing',
  'snowboarding',
  'snowshoeing',
  'ice_skating',
  'snowmobiling',
  'sledding',
  'skiing',
  'swimming',
  'paddling',
  'kayaking',
  'stand_up_paddleboarding',
  'rowing',
  'rafting',
  'canoeing',
  'surfing',
  'kitesurfing',
  'windsurfing',
  'wakeboarding',
  'wakesurfing',
  'water_skiing',
  'water_tubing',
  'diving',
  'snorkeling',
  'boating',
  'sailing',
  'fishing',
  'training',
  'fitness_equipment',
  'strength_training',
  'hiit',
  'yoga',
  'pilates',
  'floor_climbing',
  'dance',
  'jump_rope',
  'meditation',
  'soccer',
  'basketball',
  'american_football',
  'rugby',
  'lacrosse',
  'hockey',
  'volleyball',
  'tennis',
  'racket',
  'cricket',
  'baseball',
  'golf',
  'disc_golf',
  'rock_climbing',
  'flying',
  'hang_gliding',
  'sky_diving',
  'jumpmaster',
  'horseback_riding',
  'hunting',
  'motorcycling',
  'driving',
  'boxing',
  'mixed_martial_arts',
  'tactical',
  'multisport',
  'transition',
  'generic',
  'unknown',
]

const generatedSportDefs = new Map<string, (typeof GENERATED_SPORT_DEFS)[number]>(GENERATED_SPORT_DEFS.map((s) => [s.sportType, s]))
const frontendSportOrderSet = new Set<string>(FRONTEND_SPORT_ORDER)

export const SPORT_DEFS: SportDef[] = [
  ...FRONTEND_SPORT_ORDER.map((sportType) => generatedSportDefs.get(sportType)).filter((s): s is (typeof GENERATED_SPORT_DEFS)[number] => Boolean(s)),
  ...GENERATED_SPORT_DEFS.filter((s) => !frontendSportOrderSet.has(s.sportType)),
].map((s) => ({
  sportType: s.sportType,
  categoryKey: s.categoryKey,
  segmentGroup: s.segmentGroup,
  label: s.label,
  typicallyHasGPS: s.typicallyHasGPS,
  isIndoor: s.isIndoor,
  lifecycle: s.lifecycle,
  selectable: s.selectable,
  subTypes: s.subTypes.map((st) => ({ ...st })),
}))

const LEGACY_ALIASES = new Map<string, SportTuple>([
  ['mountain_biking', { sportType: 'cycling', subType: 'mountain', modifiers: {} }],
  ['gravel_cycling', { sportType: 'cycling', subType: 'gravel_cycling', modifiers: {} }],
  ['indoor_cycling', { sportType: 'cycling', subType: 'indoor_cycling', modifiers: { isIndoor: true } }],
  ['trail_running', { sportType: 'running', subType: 'trail', modifiers: {} }],
  ['treadmill_running', { sportType: 'running', subType: 'treadmill', modifiers: { isIndoor: true } }],
  ['lap_swimming', { sportType: 'swimming', subType: 'lap_swimming', modifiers: {} }],
  ['open_water_swimming', { sportType: 'swimming', subType: 'open_water', modifiers: {} }],
  ['road_cycling', { sportType: 'cycling', subType: 'road', modifiers: {} }],
  ['wheelchair_push_walk', { sportType: 'wheelchair_push', subType: 'walk_speed', modifiers: {} }],
  ['wheelchair_push_run', { sportType: 'wheelchair_push', subType: 'run_speed', modifiers: {} }],
  ['strength', { sportType: 'strength_training', subType: '', modifiers: {} }],
  ['e_mountain_biking', { sportType: 'cycling', subType: 'mountain', modifiers: { isElectric: true } }],
  ['e_biking', { sportType: 'cycling', subType: '', modifiers: { isElectric: true } }],
  ['e_bike', { sportType: 'cycling', subType: '', modifiers: { isElectric: true } }],
])

const LEGACY_SUBTYPE_ALIASES = new Map<string, SportTuple>([
  [tupleKey('wheelchair_push_walk', 'indoor_wheelchair_walk'), { sportType: 'wheelchair_push', subType: 'walk_speed', modifiers: { isIndoor: true } }],
  [tupleKey('wheelchair_push_run', 'indoor_wheelchair_run'), { sportType: 'wheelchair_push', subType: 'run_speed', modifiers: { isIndoor: true } }],
])

function tupleKey(sportType: string, subType: string): string {
  return `${sportType}\u0000${subType}`
}

// ── Lookup Maps ─────────────────────────────────────────────────────────────────

const sportMap = new Map<string, SportDef>()
const catSportsMap = new Map<string, SportDef[]>()

for (const s of SPORT_DEFS) {
  sportMap.set(s.sportType, s)
  const list = catSportsMap.get(s.categoryKey)
  if (list) {
    list.push(s)
  } else {
    catSportsMap.set(s.categoryKey, [s])
  }
}

// ── Sub-type promotion set ──────────────────────────────────────────────────────

export const PROMOTE_SUB_TYPE = new Set(['training', 'fitness_equipment', 'racket'])

// ── Fallback humanizer ──────────────────────────────────────────────────────────

function humanize(raw: string): string {
  return raw
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

// ── Lookup Functions ────────────────────────────────────────────────────────────

export function normalizeSportTuple(sportType: string, subType: string | null = null): SportTuple {
  const trimmedSport = sportType.trim()
  const trimmedSub = subType?.trim() ?? ''
  const subtypeAlias = LEGACY_SUBTYPE_ALIASES.get(tupleKey(trimmedSport, trimmedSub))
  if (subtypeAlias) {
    return {
      sportType: subtypeAlias.sportType,
      subType: subtypeAlias.subType,
      modifiers: { ...subtypeAlias.modifiers },
    }
  }
  const alias = LEGACY_ALIASES.get(trimmedSport)
  const tuple: SportTuple = alias
    ? {
        sportType: alias.sportType,
        subType: alias.subType || (trimmedSub && trimmedSub !== 'generic' ? trimmedSub : ''),
        modifiers: { ...alias.modifiers },
      }
    : {
        sportType: trimmedSport,
        subType: trimmedSub,
        modifiers: {},
      }

  if (tuple.sportType === 'running' && tuple.subType === 'treadmill') {
    tuple.modifiers.isIndoor = true
  }
  if (tuple.subType === 'generic') {
    tuple.subType = ''
  }

  return tuple
}

/**
 * Get sport definition. Returns undefined for unknown sport types.
 */
export function getSportDef(sportType: string): SportDef | undefined {
  return sportMap.get(sportType.trim())
}

/**
 * Get the segment group for a sport type. Returns 'generic' for unknown types.
 */
export function getSegmentGroup(sportType: string): 'bike' | 'foot' | 'swim' | 'ski' | 'paddle' | 'generic' {
  return getSportDef(sportType)?.segmentGroup ?? 'generic'
}

/**
 * German display label with sub-type promotion for training, fitness_equipment,
 * and racket sport types.
 *
 * Examples:
 *   sportLabel("running", "trail")             → "Trail"
 *   sportLabel("training", "yoga")             → "Yoga"
 *   sportLabel("training", "strength_training") → "Krafttraining"
 *   sportLabel("racket", "squash")             → "Squash"
 *   sportLabel("cycling")                      → "Radfahren"
 *   sportLabel("some_unknown_type")            → "Some Unknown Type"
 */
export function sportLabel(sportType: string, subType?: string | null, opts?: SportLabelOptions): string {
  const trimmedSport = sportType.trim()
  if (LEGACY_ALIASES.has(trimmedSport)) {
    return `Veraltete Sportart (${humanize(trimmedSport)})`
  }
  const tuple = { sportType: trimmedSport, subType: subType?.trim() ?? '' }
  const d = sportMap.get(tuple.sportType)
  if (!d) return humanize(sportType)

  if (tuple.subType && tuple.subType !== 'generic') {
    const st = d.subTypes.find((s) => s.subType === tuple.subType)
    if (st) {
      if (opts?.mode === 'withParent' && !subTypeLabelCanStandAlone(d)) {
        return `${d.label} (${st.label})`
      }
      return st.label
    }
  }

  return d.label
}

/**
 * Full detail label including both sport and sub-type when applicable.
 *
 * Examples:
 *   sportDetailLabel("alpine_skiing", "backcountry") → "Alpin-Ski (Backcountry)"
 *   sportDetailLabel("training", "yoga")             → "Training (Yoga)"
 *   sportDetailLabel("cycling")                      → "Radfahren"
 *   sportDetailLabel("some_unknown_type")            → "Some Unknown Type"
 */
export function sportDetailLabel(sportType: string, subType?: string | null): string {
  return sportLabel(sportType, subType, { mode: 'withParent' })
}

function subTypeLabelCanStandAlone(d: SportDef): boolean {
  return d.sportType === 'training' || d.sportType === 'fitness_equipment' || d.sportType === 'racket'
}

/**
 * Category color for a sport type. Returns gray (#6B7280) for unknown types.
 */
export function sportColor(sportType: string): string {
  const d = getSportDef(sportType)
  if (!d) return '#6B7280'
  return categoryMap.get(d.categoryKey)?.color ?? '#6B7280'
}

/**
 * Get category for a sport type. Defaults to the 'other' category for unknowns.
 */
export function categoryForSport(sportType: string): SportCategory {
  const d = getSportDef(sportType)
  const cat = d ? categoryMap.get(d.categoryKey) : undefined
  return cat ?? { key: 'other', label: 'Sonstiges', color: '#6B7280' }
}

/**
 * Get all sport definitions in a category.
 */
export function sportsInCategory(categoryKey: string): SportDef[] {
  return catSportsMap.get(categoryKey) ?? []
}

export function canonicalSportOptions(): SportOption[] {
  return SPORT_DEFS.map((s) => ({ value: s.sportType, label: s.label }))
}

const ROUTE_PROMOTED_SUBTYPES: Record<string, string[]> = {
  cycling: ['mountain', 'gravel_cycling', 'road'],
  running: ['trail'],
}

const ROUTE_COMPATIBLE_SPORT_TYPES = [
  'running',
  'walking',
  'hiking',
  'mountaineering',
  'snowshoeing',
  'wheelchair_push',
  'cycling',
  'inline_skating',
  'alpine_skiing',
  'backcountry_skiing',
  'cross_country_skiing',
  'snowboarding',
  'ice_skating',
  'snowmobiling',
  'sledding',
  'skiing',
  'swimming',
  'paddling',
  'kayaking',
  'stand_up_paddleboarding',
  'rowing',
  'rafting',
  'canoeing',
  'surfing',
  'kitesurfing',
  'windsurfing',
  'wakeboarding',
  'wakesurfing',
  'water_skiing',
  'water_tubing',
  'snorkeling',
  'boating',
  'sailing',
  'fishing',
  'cricket',
  'golf',
  'disc_golf',
  'rock_climbing',
  'flying',
  'hang_gliding',
  'sky_diving',
  'jumpmaster',
  'horseback_riding',
  'hunting',
  'motorcycling',
  'driving',
  'multisport',
  'transition',
]

export function routeSportOptions(): RouteSportOption[] {
  return ROUTE_COMPATIBLE_SPORT_TYPES
    .map((sportType) => sportMap.get(sportType))
    .filter((s): s is SportDef => Boolean(s))
    .flatMap((s) => {
      const options: RouteSportOption[] = [{ sportType: s.sportType, label: s.label }]
      for (const subType of ROUTE_PROMOTED_SUBTYPES[s.sportType] ?? []) {
        if (s.subTypes.some((st) => st.subType === subType)) {
          options.push({ sportType: s.sportType, subType, label: sportLabel(s.sportType, subType) })
        }
      }
      return options
    })
}
