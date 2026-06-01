import type { ProfileDefaultsResponse } from '@/api/routes'
import type { ModifierFilters, SportPredicate } from '@/lib/sportPredicates'

// ─── Profile Config Types ──────────────────────────────────────────────────

export interface ProfileConfig {
  profile: string
  sport_types?: string[]
  predicates?: SportPredicate[]
  modifier_filters?: ModifierFilters
  uphill_cost?: number
  downhill_cost?: number
  uphill_cutoff_pct?: number
  downhill_cutoff_pct?: number
  elev_penalty_buffer_m?: number
  elev_max_buffer_m?: number
  elev_buffer_reduce?: number
  max_sac_scale?: number
  sac_preferred?: number
  max_mtb_scale?: number
  max_via_ferrata?: number
  allow_unscaled_via_ferrata?: boolean
  max_trail_visibility?: number         // NEW
  trail_visibility_penalty?: number     // NEW
  allow_steps?: boolean
  allow_ferries?: boolean
  forest_bonus?: number
  noise_penalty?: number
  traffic_penalty?: number
  protected_bonus?: number
  cycleroute_boost?: number
  hikeroute_boost?: number
  wet_weather?: boolean
  path_preference?: number
  turn_cost?: number
  initial_cost_road?: number
  pass0_coefficient?: number
  // Offroad / snap
  snap_threshold_m?: number
  // Highway cost overrides (null from API = blocked type)
  path_cost?: number
  track_cost?: number
  track_grade1_cost?: number
  track_grade2_cost?: number
  track_grade3_cost?: number
  track_grade4_cost?: number
  track_grade5_cost?: number
  footway_cost?: number
  cycleway_cost?: number
  bridleway_cost?: number
  pedestrian_cost?: number
  residential_cost?: number
  service_cost?: number
  unclassified_cost?: number
  tertiary_cost?: number
  secondary_cost?: number
  primary_cost?: number
}

// ─── Field Metadata ────────────────────────────────────────────────────────

export type FieldSection = 'elevation' | 'difficulty' | 'environment' | 'paths' | 'behavior'

export interface FieldMeta {
  key: keyof ProfileConfig
  labelKey: `routePlanner.profileEditor.fields.${string}.label`
  type: 'number' | 'boolean' | 'int'
  min?: number
  max?: number
  step?: number
  unit?: string
  section: FieldSection
  hint?: true
}

export const FIELD_METADATA: FieldMeta[] = [
  // Elevation
  {
    key: 'uphill_cost',
    labelKey: `routePlanner.profileEditor.fields.uphill_cost.label`,
    type: 'number',
    min: 0.0,
    max: 200,
    step: 1.0,
    section: 'elevation',
    hint: true,
  },
  {
    key: 'downhill_cost',
    labelKey: `routePlanner.profileEditor.fields.downhill_cost.label`,
    type: 'number',
    min: 0.0,
    max: 200,
    step: 1.0,
    section: 'elevation',
    hint: true,
  },
  {
    key: 'uphill_cutoff_pct',
    labelKey: `routePlanner.profileEditor.fields.uphill_cutoff_pct.label`,
    type: 'number',
    min: 0,
    max: 30,
    step: 0.5,
    unit: '%',
    section: 'elevation',
    hint: true,
  },
  {
    key: 'downhill_cutoff_pct',
    labelKey: `routePlanner.profileEditor.fields.downhill_cutoff_pct.label`,
    type: 'number',
    min: 0,
    max: 30,
    step: 0.5,
    unit: '%',
    section: 'elevation',
    hint: true,
  },
  {
    key: 'elev_penalty_buffer_m',
    labelKey: `routePlanner.profileEditor.fields.elev_penalty_buffer_m.label`,
    type: 'number',
    min: 0,
    max: 50,
    step: 5,
    unit: 'm',
    section: 'elevation',
    hint: true,
  },
  {
    key: 'elev_max_buffer_m',
    labelKey: `routePlanner.profileEditor.fields.elev_max_buffer_m.label`,
    type: 'number',
    min: 0,
    max: 100,
    step: 5,
    unit: 'm',
    section: 'elevation',
  },
  {
    key: 'elev_buffer_reduce',
    labelKey: `routePlanner.profileEditor.fields.elev_buffer_reduce.label`,
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'elevation',
    hint: true,
  },

  // Difficulty
  {
    key: 'max_sac_scale',
    labelKey: `routePlanner.profileEditor.fields.max_sac_scale.label`,
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: true,
  },
  {
    key: 'sac_preferred',
    labelKey: `routePlanner.profileEditor.fields.sac_preferred.label`,
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: true,
  },
  {
    key: 'max_mtb_scale',
    labelKey: `routePlanner.profileEditor.fields.max_mtb_scale.label`,
    type: 'int',
    min: 0,
    max: 5,
    step: 1,
    section: 'difficulty',
    hint: true,
  },
  {
    key: 'max_via_ferrata',
    labelKey: `routePlanner.profileEditor.fields.max_via_ferrata.label`,
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: true,
  },
  {
    key: 'max_trail_visibility',
    labelKey: `routePlanner.profileEditor.fields.max_trail_visibility.label`,
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: true,
  },
  {
    key: 'trail_visibility_penalty',
    labelKey: `routePlanner.profileEditor.fields.trail_visibility_penalty.label`,
    type: 'number',
    min: 0,
    max: 2,
    step: 0.1,
    section: 'difficulty',
    hint: true,
  },
  {
    key: 'allow_steps',
    labelKey: `routePlanner.profileEditor.fields.allow_steps.label`,
    type: 'boolean',
    section: 'difficulty',
  },
  {
    key: 'allow_ferries',
    labelKey: `routePlanner.profileEditor.fields.allow_ferries.label`,
    type: 'boolean',
    section: 'difficulty',
  },

  // Environment
  {
    key: 'forest_bonus',
    labelKey: `routePlanner.profileEditor.fields.forest_bonus.label`,
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'environment',
    hint: true,
  },
  {
    key: 'noise_penalty',
    labelKey: `routePlanner.profileEditor.fields.noise_penalty.label`,
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'environment',
    hint: true,
  },
  {
    key: 'traffic_penalty',
    labelKey: `routePlanner.profileEditor.fields.traffic_penalty.label`,
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'environment',
    hint: true,
  },
  {
    key: 'protected_bonus',
    labelKey: `routePlanner.profileEditor.fields.protected_bonus.label`,
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'environment',
    hint: true,
  },

  // Paths
  {
    key: 'cycleroute_boost',
    labelKey: `routePlanner.profileEditor.fields.cycleroute_boost.label`,
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'paths',
    hint: true,
  },
  {
    key: 'hikeroute_boost',
    labelKey: `routePlanner.profileEditor.fields.hikeroute_boost.label`,
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'paths',
    hint: true,
  },
  {
    key: 'path_preference',
    labelKey: `routePlanner.profileEditor.fields.path_preference.label`,
    type: 'number',
    min: 0.0,
    max: 30,
    step: 0.5,
    section: 'paths',
    hint: true,
  },

  // Behavior
  {
    key: 'wet_weather',
    labelKey: `routePlanner.profileEditor.fields.wet_weather.label`,
    type: 'boolean',
    section: 'behavior',
    hint: true,
  },
  {
    key: 'turn_cost',
    labelKey: `routePlanner.profileEditor.fields.turn_cost.label`,
    type: 'number',
    min: 0.0,
    max: 150,
    step: 5,
    section: 'behavior',
    hint: true,
  },
  {
    key: 'initial_cost_road',
    labelKey: `routePlanner.profileEditor.fields.initial_cost_road.label`,
    type: 'number',
    min: 0.0,
    max: 50000,
    step: 100,
    section: 'behavior',
  },
  {
    key: 'pass0_coefficient',
    labelKey: `routePlanner.profileEditor.fields.pass0_coefficient.label`,
    type: 'number',
    min: 1.0,
    max: 5.0,
    step: 0.1,
    section: 'behavior',
    hint: true,
  },
]

// ─── Server Defaults Cache ────────────────────────────────────────────────

/**
 * In-memory cache: profile name → server defaults.
 * Populated by fetchAndCacheServerDefaults(), consumed by getProfileDefaults().
 */
const serverDefaultsCache = new Map<string, ProfileConfig>()

/** Convert a server ProfileDefaultsResponse into a ProfileConfig. */
export function serverResponseToConfig(resp: ProfileDefaultsResponse): ProfileConfig {
  const cfg: ProfileConfig = { profile: resp.profile }

  // Map all numeric/boolean fields — skip nulls (leave undefined → fallback to TS defaults)
  const numericKeys: (keyof ProfileDefaultsResponse & keyof ProfileConfig)[] = [
    'uphill_cost', 'downhill_cost', 'uphill_cutoff_pct', 'downhill_cutoff_pct',
    'elev_penalty_buffer_m', 'elev_max_buffer_m', 'elev_buffer_reduce',
    'max_sac_scale', 'sac_preferred', 'max_mtb_scale', 'max_via_ferrata',
    'max_trail_visibility', 'trail_visibility_penalty',
    'forest_bonus', 'noise_penalty', 'traffic_penalty', 'protected_bonus',
    'cycleroute_boost', 'hikeroute_boost',
    'path_preference', 'turn_cost', 'initial_cost_road', 'pass0_coefficient',
    'path_cost', 'track_cost', 'track_grade1_cost', 'track_grade2_cost',
    'track_grade3_cost', 'track_grade4_cost', 'track_grade5_cost',
    'footway_cost', 'cycleway_cost', 'bridleway_cost',
    'pedestrian_cost', 'residential_cost', 'service_cost', 'unclassified_cost',
    'tertiary_cost', 'secondary_cost', 'primary_cost',
  ]
  for (const k of numericKeys) {
    const val = resp[k]
    if (val !== null && val !== undefined) {
      ;(cfg as unknown as Record<string, unknown>)[k] = val
    }
  }

  const boolKeys: (keyof ProfileDefaultsResponse & keyof ProfileConfig)[] = [
    'allow_steps', 'allow_ferries', 'wet_weather', 'allow_unscaled_via_ferrata',
  ]
  for (const k of boolKeys) {
    const val = resp[k]
    if (val !== null && val !== undefined) {
      ;(cfg as unknown as Record<string, unknown>)[k] = val
    }
  }

  return cfg
}

/** Cache server defaults for a profile. Called from ProfileEditor on fetch. */
export function cacheServerDefaults(name: string, resp: ProfileDefaultsResponse): void {
  serverDefaultsCache.set(name, serverResponseToConfig(resp))
}

/** Get cached server defaults, if available. */
export function getCachedServerDefaults(name: string): ProfileConfig | undefined {
  return serverDefaultsCache.get(name)
}

// ─── Helpers ───────────────────────────────────────────────────────────────

/**
 * Get cached server defaults for a profile. Returns null when not yet fetched.
 * Server cache is the single source of truth — no hardcoded TS fallback.
 */
export function getProfileDefaults(name: string): ProfileConfig | null {
  const cached = serverDefaultsCache.get(name)
  if (cached) return { ...cached }
  return null
}

export function mergeProfile(base: ProfileConfig, overrides: Partial<ProfileConfig>): ProfileConfig {
  return { ...base, ...overrides }
}

export const SECTION_LABEL_KEYS: Record<FieldSection, `routePlanner.profileEditor.sections.${FieldSection}`> = {
  elevation: 'routePlanner.profileEditor.sections.elevation',
  difficulty: 'routePlanner.profileEditor.sections.difficulty',
  environment: 'routePlanner.profileEditor.sections.environment',
  paths: 'routePlanner.profileEditor.sections.paths',
  behavior: 'routePlanner.profileEditor.sections.behavior',
}
