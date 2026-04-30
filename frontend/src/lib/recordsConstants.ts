// ── Records page constants ────────────────────────────────────────────────────

export const SPORT_GROUP_LABELS: Record<string, string> = {
  running:          'Laufen',
  running_trail:    'Laufen (Trail)',
  running_indoor:   'Laufen (Indoor)',
  running_virtual:  'Laufen (Virtuell)',
  cycling:          'Radfahren',
  cycling_electric: 'E-Bike',
  cycling_virtual:  'Radfahren (Stationär)',
  hiking:           'Wandern',
  swim_pool:        'Schwimmen (Becken)',
  swim_ow:          'Schwimmen (Freiwasser)',
}

export type MainCategory = 'laufen' | 'radfahren' | 'wandern' | 'schwimmen'

export const MAIN_CATEGORIES: Record<MainCategory, string[]> = {
  laufen:     ['running', 'running_trail', 'running_indoor', 'running_virtual'],
  radfahren:  ['cycling', 'cycling_electric', 'cycling_virtual'],
  wandern:    ['hiking'],
  schwimmen:  ['swim_ow', 'swim_pool'],
}

export const MAIN_CATEGORY_LABELS: Record<MainCategory, string> = {
  laufen:     'Laufen',
  radfahren:  'Radfahren',
  wandern:    'Wandern',
  schwimmen:  'Schwimmen',
}

export const MAIN_CATEGORY_COLORS: Record<MainCategory, string> = {
  laufen:     '#3B82F6',
  radfahren:  '#F97316',
  wandern:    '#22C55E',
  schwimmen:  '#06B6D4',
}

export interface SubFilter {
  label: string
  sportGroups: string[]
}

export interface RecordsSportFilterValue {
  sportTypes: string[] | null
  modifiers?: {
    is_electric?: boolean
    is_indoor?: boolean
    is_virtual?: boolean
    is_commute?: boolean
  }
}

export const SUB_FILTERS: Record<MainCategory, SubFilter[]> = {
  laufen: [
    { label: 'Alle', sportGroups: ['running', 'running_trail', 'running_indoor', 'running_virtual'] },
    { label: 'Straße', sportGroups: ['running'] },
    { label: 'Trail', sportGroups: ['running_trail'] },
    { label: 'Stationär', sportGroups: ['running_indoor', 'running_virtual'] },
  ],
  radfahren: [
    { label: 'Alle', sportGroups: ['cycling', 'cycling_electric', 'cycling_virtual'] },
    { label: 'Radfahren', sportGroups: ['cycling'] },
    { label: 'E-Bike', sportGroups: ['cycling_electric'] },
    { label: 'Stationär', sportGroups: ['cycling_virtual'] },
  ],
  schwimmen: [
    { label: 'Alle', sportGroups: ['swim_ow', 'swim_pool'] },
    { label: 'Freiwasser', sportGroups: ['swim_ow'] },
    { label: 'Becken', sportGroups: ['swim_pool'] },
  ],
  wandern: [],  // no sub-filters
}

/** Power curves: which sport_groups to aggregate per main category */
export const POWER_CURVE_GROUPS: Partial<Record<MainCategory, string[]>> = {
  laufen:    ['running', 'running_trail', 'running_indoor', 'running_virtual'],
  radfahren: ['cycling'],  // E-Bike has no power categories in backend, cycling_virtual has no power data
}

export const SPORT_GROUP_ORDER = Object.keys(SPORT_GROUP_LABELS)

export const CATEGORY_PREFIXES = {
  dist: 'dist_',
  elev: 'elev_',
  power: 'power_',
  sup: 'sup_',
} as const

/** Sport-group badge suffix for mixed swimming views */
export const POOL_BADGE = 'B'
export const OW_BADGE = 'FW'

export const SPORT_GROUP_TO_SPORT_TYPES: Record<string, string[]> = {
  running:          ['running'],
  running_trail:    ['running'],
  running_indoor:   ['running'],
  running_virtual:  ['running'],
  cycling:          ['cycling'],
  cycling_electric: ['cycling'],
  cycling_virtual:  ['cycling'],
  hiking:           ['hiking', 'mountaineering', 'snowshoeing'],
  swim_pool:        ['swimming'],
  swim_ow:          ['swimming'],
}

/** Predicate for filtering SegmentEffort / similar records by best-efforts sport_group. */
export function matchesSportGroup(
  sportGroup: string,
  effort: { sport_type: string; sub_type?: string; is_electric?: boolean; is_indoor?: boolean; is_virtual?: boolean },
): boolean {
  const types = SPORT_GROUP_TO_SPORT_TYPES[sportGroup] ?? []
  const matchesType = types.includes(effort.sport_type)

  if (sportGroup === 'running_trail') {
    return matchesType && effort.sub_type === 'trail'
  }
  if (sportGroup === 'running_indoor') {
    return matchesType && (effort.is_indoor === true || effort.sub_type === 'indoor_running' || effort.sub_type === 'treadmill')
  }
  if (sportGroup === 'running') {
    return matchesType && effort.sub_type !== 'trail' && effort.sub_type !== 'indoor_running' && effort.sub_type !== 'treadmill' && effort.is_indoor !== true && effort.is_virtual !== true
  }
  if (sportGroup === 'running_virtual') {
    return matchesType && effort.is_virtual === true
  }
  if (sportGroup === 'cycling_electric') {
    return matchesType && effort.is_electric === true
  }
  if (sportGroup === 'cycling_virtual') {
    return matchesType && (effort.is_virtual === true || effort.is_indoor === true || effort.sub_type === 'indoor_cycling')
  }
  if (sportGroup === 'cycling') {
    return matchesType && effort.is_electric !== true && effort.is_indoor !== true && effort.is_virtual !== true && effort.sub_type !== 'indoor_cycling'
  }
  if (sportGroup === 'swim_pool') {
    return matchesType && effort.sub_type === 'lap_swimming'
  }
  if (sportGroup === 'swim_ow') {
    return matchesType && (effort.sub_type === 'open_water' || !effort.sub_type)
  }
  return matchesType
}

export function recordSportFilterValue(mainCategory: MainCategory | null, subFilter: string | null): RecordsSportFilterValue {
  if (!mainCategory) return { sportTypes: null, modifiers: {} }
  if (mainCategory === 'laufen') {
    if (subFilter === 'Trail') return { sportTypes: ['running:trail'], modifiers: {} }
    if (subFilter === 'Stationär') return { sportTypes: ['running:indoor_running'], modifiers: { is_virtual: true } }
    if (subFilter === 'Straße') return { sportTypes: ['running'], modifiers: {} }
    return { sportTypes: ['running'], modifiers: {} }
  }
  if (mainCategory === 'radfahren') {
    if (subFilter === 'E-Bike') return { sportTypes: ['cycling'], modifiers: { is_electric: true } }
    if (subFilter === 'Stationär') return { sportTypes: ['cycling:indoor_cycling'], modifiers: { is_virtual: true } }
    if (subFilter === 'Radfahren') return { sportTypes: ['cycling'], modifiers: { is_electric: false } }
    return { sportTypes: ['cycling'], modifiers: {} }
  }
  if (mainCategory === 'schwimmen') {
    if (subFilter === 'Freiwasser') return { sportTypes: ['swimming:open_water'], modifiers: {} }
    if (subFilter === 'Becken') return { sportTypes: ['swimming:lap_swimming'], modifiers: {} }
    return { sportTypes: ['swimming'], modifiers: {} }
  }
  return { sportTypes: ['hiking'], modifiers: {} }
}

export function recordFilterSelection(value: RecordsSportFilterValue): { mainCategory: MainCategory | null; subFilter: string | null } {
  const keys = value.sportTypes ?? []
  if (keys.length === 0) return { mainCategory: null, subFilter: null }

  if (keys.includes('running:trail')) return { mainCategory: 'laufen', subFilter: 'Trail' }
  if (keys.includes('running:indoor_running') || (keys.some((key) => key === 'running' || key.startsWith('running:')) && value.modifiers?.is_virtual === true)) {
    return { mainCategory: 'laufen', subFilter: 'Stationär' }
  }
  if (keys.includes('running')) return { mainCategory: 'laufen', subFilter: 'Straße' }

  if (keys.includes('cycling:indoor_cycling') || (keys.some((key) => key === 'cycling' || key.startsWith('cycling:')) && value.modifiers?.is_virtual === true)) {
    return { mainCategory: 'radfahren', subFilter: 'Stationär' }
  }
  if (keys.some((key) => key === 'cycling' || key.startsWith('cycling:'))) {
    if (value.modifiers?.is_electric === true) return { mainCategory: 'radfahren', subFilter: 'E-Bike' }
    if (value.modifiers?.is_electric === false) return { mainCategory: 'radfahren', subFilter: 'Radfahren' }
    return { mainCategory: 'radfahren', subFilter: null }
  }

  if (keys.includes('swimming:open_water')) return { mainCategory: 'schwimmen', subFilter: 'Freiwasser' }
  if (keys.includes('swimming:lap_swimming')) return { mainCategory: 'schwimmen', subFilter: 'Becken' }
  if (keys.includes('swimming')) return { mainCategory: 'schwimmen', subFilter: null }

  if (keys.includes('hiking')) return { mainCategory: 'wandern', subFilter: null }
  return { mainCategory: null, subFilter: null }
}
