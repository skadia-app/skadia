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

/**
 * Mapping from sport_group values (used in BestEffortRanked.sport_group) to
 * sport_type values (used in SegmentEffort.sport_type / sportTaxonomy.ts).
 * Used to filter SegmentSection correctly.
 */
export const SPORT_GROUP_TO_SPORT_TYPES: Record<string, string[]> = {
  running:          ['running'],
  running_trail:    ['trail_running'],
  running_indoor:   ['treadmill_running'],
  running_virtual:  ['running'],  // virtual running has no distinct sport_type in taxonomy
  cycling:          ['cycling', 'mountain_biking', 'gravel_cycling'],
  cycling_electric: ['cycling'],  // e-bike has no distinct sport_type in taxonomy
  cycling_virtual:  ['indoor_cycling'],
  hiking:           ['hiking', 'mountaineering', 'snowshoeing'],
  swim_pool:        ['lap_swimming'],
  swim_ow:          ['open_water_swimming', 'swimming'],
}
