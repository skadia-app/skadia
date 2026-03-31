import type { ProfileDefaultsResponse } from '@/api/routes'

// ─── Profile Config Types ──────────────────────────────────────────────────

export interface ProfileConfig {
  profile: string
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
  label: string
  type: 'number' | 'boolean' | 'int'
  min?: number
  max?: number
  step?: number
  unit?: string
  section: FieldSection
  hint?: string
}

export const FIELD_METADATA: FieldMeta[] = [
  // Elevation
  {
    key: 'uphill_cost',
    label: 'Aufstiegskosten',
    type: 'number',
    min: 0.0,
    max: 5.0,
    step: 0.05,
    section: 'elevation',
    hint: 'Gewichtung von Höhenmetern bergauf',
  },
  {
    key: 'downhill_cost',
    label: 'Abstiegskosten',
    type: 'number',
    min: 0.0,
    max: 5.0,
    step: 0.05,
    section: 'elevation',
    hint: 'Gewichtung von Höhenmetern bergab',
  },
  {
    key: 'uphill_cutoff_pct',
    label: 'Aufstieg Cutoff (%)',
    type: 'number',
    min: 0,
    max: 100,
    step: 1,
    unit: '%',
    section: 'elevation',
    hint: 'Ab dieser Steigung werden Aufstiegskosten angewendet',
  },
  {
    key: 'downhill_cutoff_pct',
    label: 'Abstieg Cutoff (%)',
    type: 'number',
    min: 0,
    max: 100,
    step: 1,
    unit: '%',
    section: 'elevation',
    hint: 'Ab diesem Gefälle werden Abstiegskosten angewendet',
  },
  {
    key: 'elev_penalty_buffer_m',
    label: 'Höhenpuffer Start (m)',
    type: 'number',
    min: 0,
    max: 500,
    step: 10,
    unit: 'm',
    section: 'elevation',
    hint: 'Höhentoleranz bevor Kostenfunktion greift',
  },
  {
    key: 'elev_max_buffer_m',
    label: 'Höhenpuffer Max (m)',
    type: 'number',
    min: 0,
    max: 2000,
    step: 50,
    unit: 'm',
    section: 'elevation',
  },
  {
    key: 'elev_buffer_reduce',
    label: 'Puffer-Reduktion',
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'elevation',
    hint: 'Wie schnell der Puffer abbaut (0=nie, 1=sofort)',
  },

  // Difficulty
  {
    key: 'max_sac_scale',
    label: 'Max. SAC-Skala',
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: '0=Weg, 1=T1, 2=T2, 3=T3, 4=T4, 5=T5, 6=T6',
  },
  {
    key: 'sac_preferred',
    label: 'Bevorzugte SAC-Stufe',
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: 'Wege unter dieser Stufe werden leicht bevorzugt',
  },
  {
    key: 'max_mtb_scale',
    label: 'Max. MTB-Skala',
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: '0=S0, 1=S1, 2=S2, 3=S3, 4=S4, 5=S5, 6=S6',
  },
  {
    key: 'max_via_ferrata',
    label: 'Max. Klettersteig',
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: '0=kein, 1=A, 2=B, 3=C, 4=D, 5=E, 6=F',
  },
  {
    key: 'allow_steps',
    label: 'Treppen erlaubt',
    type: 'boolean',
    section: 'difficulty',
  },
  {
    key: 'allow_ferries',
    label: 'Fähren erlaubt',
    type: 'boolean',
    section: 'difficulty',
  },

  // Environment
  {
    key: 'forest_bonus',
    label: 'Waldbonus',
    type: 'number',
    min: 0.0,
    max: 2.0,
    step: 0.05,
    section: 'environment',
    hint: 'Rabatt auf Kosten für Waldwege (<1 = bevorzugt)',
  },
  {
    key: 'noise_penalty',
    label: 'Lärm-Malus',
    type: 'number',
    min: 0.0,
    max: 3.0,
    step: 0.1,
    section: 'environment',
    hint: 'Aufpreis für laute Strassen',
  },
  {
    key: 'traffic_penalty',
    label: 'Verkehrs-Malus',
    type: 'number',
    min: 0.0,
    max: 3.0,
    step: 0.1,
    section: 'environment',
    hint: 'Aufpreis für stark befahrene Strassen',
  },
  {
    key: 'protected_bonus',
    label: 'Schutzgebiet-Bonus',
    type: 'number',
    min: 0.0,
    max: 2.0,
    step: 0.05,
    section: 'environment',
    hint: 'Rabatt für Wege in Schutzgebieten (<1 = bevorzugt)',
  },

  // Paths
  {
    key: 'cycleroute_boost',
    label: 'Radrouten-Boost',
    type: 'number',
    min: 0.0,
    max: 2.0,
    step: 0.05,
    section: 'paths',
    hint: 'Rabatt auf markierte Radrouten (<1 = bevorzugt)',
  },
  {
    key: 'hikeroute_boost',
    label: 'Wanderrouten-Boost',
    type: 'number',
    min: 0.0,
    max: 2.0,
    step: 0.05,
    section: 'paths',
    hint: 'Rabatt auf markierte Wanderwege (<1 = bevorzugt)',
  },
  {
    key: 'path_preference',
    label: 'Pfad-Präferenz',
    type: 'number',
    min: 0.0,
    max: 2.0,
    step: 0.05,
    section: 'paths',
    hint: 'Multiplikator für Pfade vs. Strassen (<1 = Pfade bevorzugt)',
  },

  // Behavior
  {
    key: 'wet_weather',
    label: 'Nasses Wetter',
    type: 'boolean',
    section: 'behavior',
    hint: 'Rutschige Wege meiden',
  },
  {
    key: 'turn_cost',
    label: 'Abbiegekosten',
    type: 'number',
    min: 0.0,
    max: 5.0,
    step: 0.1,
    section: 'behavior',
    hint: 'Zusatzkosten pro Abbiegung',
  },
  {
    key: 'initial_cost_road',
    label: 'Startkosten Strasse',
    type: 'number',
    min: 0.0,
    max: 5.0,
    step: 0.1,
    section: 'behavior',
  },
  {
    key: 'pass0_coefficient',
    label: 'Pass-Koeffizient',
    type: 'number',
    min: 0.0,
    max: 5.0,
    step: 0.1,
    section: 'behavior',
    hint: 'Kostenfaktor für den ersten Pass der A*-Suche',
  },
]

// ─── Profile Metadata ──────────────────────────────────────────────────────

export interface ProfileMeta {
  name: string
  label: string
  description: string
  defaults: ProfileConfig
}

export const PROFILES: ProfileMeta[] = [
  {
    name: 'hiking',
    label: 'Wandern',
    description: 'Alpines Wandern, SAC T1–T4',
    defaults: {
      profile: 'hiking',
      uphill_cost: 1.5,
      downhill_cost: 0.5,
      uphill_cutoff_pct: 15,
      downhill_cutoff_pct: 20,
      elev_penalty_buffer_m: 50,
      elev_max_buffer_m: 500,
      elev_buffer_reduce: 0.1,
      max_sac_scale: 4,
      sac_preferred: 2,
      max_mtb_scale: 0,
      max_via_ferrata: 0,
      allow_steps: true,
      allow_ferries: true,
      forest_bonus: 0.95,
      noise_penalty: 1.2,
      traffic_penalty: 1.5,
      protected_bonus: 0.9,
      hikeroute_boost: 0.85,
      cycleroute_boost: 1.0,
      wet_weather: false,
      path_preference: 0.8,
      turn_cost: 0.0,
      initial_cost_road: 1.2,
      pass0_coefficient: 1.0,
    },
  },
  {
    name: 'mtb',
    label: 'MTB',
    description: 'Mountainbike, technische Trails',
    defaults: {
      profile: 'mtb',
      uphill_cost: 2.0,
      downhill_cost: 0.3,
      uphill_cutoff_pct: 20,
      downhill_cutoff_pct: 30,
      elev_penalty_buffer_m: 100,
      elev_max_buffer_m: 800,
      elev_buffer_reduce: 0.15,
      max_sac_scale: 2,
      sac_preferred: 1,
      max_mtb_scale: 4,
      max_via_ferrata: 0,
      allow_steps: false,
      allow_ferries: true,
      forest_bonus: 0.9,
      noise_penalty: 1.3,
      traffic_penalty: 1.8,
      protected_bonus: 0.9,
      hikeroute_boost: 0.95,
      cycleroute_boost: 0.8,
      wet_weather: false,
      path_preference: 0.75,
      turn_cost: 0.1,
      initial_cost_road: 1.5,
      pass0_coefficient: 1.0,
    },
  },
  {
    name: 'gravel',
    label: 'Gravel',
    description: 'Gravel-Bike, Schotter & Forstwege',
    defaults: {
      profile: 'gravel',
      uphill_cost: 1.8,
      downhill_cost: 0.4,
      uphill_cutoff_pct: 18,
      downhill_cutoff_pct: 25,
      elev_penalty_buffer_m: 80,
      elev_max_buffer_m: 600,
      elev_buffer_reduce: 0.12,
      max_sac_scale: 1,
      sac_preferred: 0,
      max_mtb_scale: 2,
      max_via_ferrata: 0,
      allow_steps: false,
      allow_ferries: true,
      forest_bonus: 0.92,
      noise_penalty: 1.2,
      traffic_penalty: 1.6,
      protected_bonus: 0.92,
      hikeroute_boost: 0.95,
      cycleroute_boost: 0.85,
      wet_weather: false,
      path_preference: 0.85,
      turn_cost: 0.15,
      initial_cost_road: 1.3,
      pass0_coefficient: 1.0,
    },
  },
  {
    name: 'trekking',
    label: 'Trekking',
    description: 'Trekking-Rad, gemischte Untergründe',
    defaults: {
      profile: 'trekking',
      uphill_cost: 1.6,
      downhill_cost: 0.4,
      uphill_cutoff_pct: 15,
      downhill_cutoff_pct: 20,
      elev_penalty_buffer_m: 60,
      elev_max_buffer_m: 500,
      elev_buffer_reduce: 0.1,
      max_sac_scale: 1,
      sac_preferred: 0,
      max_mtb_scale: 1,
      max_via_ferrata: 0,
      allow_steps: true,
      allow_ferries: true,
      forest_bonus: 0.93,
      noise_penalty: 1.1,
      traffic_penalty: 1.4,
      protected_bonus: 0.93,
      hikeroute_boost: 0.9,
      cycleroute_boost: 0.87,
      wet_weather: false,
      path_preference: 0.88,
      turn_cost: 0.1,
      initial_cost_road: 1.1,
      pass0_coefficient: 1.0,
    },
  },
  {
    name: 'road',
    label: 'Rennrad',
    description: 'Rennrad, asphaltierte Strassen',
    defaults: {
      profile: 'road',
      uphill_cost: 2.5,
      downhill_cost: 0.2,
      uphill_cutoff_pct: 10,
      downhill_cutoff_pct: 15,
      elev_penalty_buffer_m: 30,
      elev_max_buffer_m: 400,
      elev_buffer_reduce: 0.2,
      max_sac_scale: 0,
      sac_preferred: 0,
      max_mtb_scale: 0,
      max_via_ferrata: 0,
      allow_steps: false,
      allow_ferries: true,
      forest_bonus: 1.0,
      noise_penalty: 0.8,
      traffic_penalty: 1.0,
      protected_bonus: 1.0,
      hikeroute_boost: 1.2,
      cycleroute_boost: 0.7,
      wet_weather: false,
      path_preference: 1.5,
      turn_cost: 0.2,
      initial_cost_road: 0.8,
      pass0_coefficient: 1.0,
    },
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
    'forest_bonus', 'noise_penalty', 'traffic_penalty', 'protected_bonus',
    'cycleroute_boost', 'hikeroute_boost',
    'path_preference', 'turn_cost', 'initial_cost_road', 'pass0_coefficient',
    'path_cost', 'track_cost', 'track_grade1_cost', 'track_grade2_cost',
    'track_grade3_cost', 'footway_cost', 'cycleway_cost', 'bridleway_cost',
    'pedestrian_cost', 'residential_cost', 'service_cost', 'unclassified_cost',
    'tertiary_cost', 'secondary_cost', 'primary_cost',
  ]
  for (const k of numericKeys) {
    const val = resp[k]
    if (val !== null && val !== undefined) {
      ;(cfg as Record<string, unknown>)[k] = val
    }
  }

  const boolKeys: (keyof ProfileDefaultsResponse & keyof ProfileConfig)[] = [
    'allow_steps', 'allow_ferries', 'wet_weather',
  ]
  for (const k of boolKeys) {
    const val = resp[k]
    if (val !== null && val !== undefined) {
      ;(cfg as Record<string, unknown>)[k] = val
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
 * Get profile defaults: server cache (single source of truth) with TS PROFILES fallback.
 * When server defaults are cached, they take priority over hardcoded TS values.
 */
export function getProfileDefaults(name: string): ProfileConfig {
  // Prefer server defaults if cached
  const cached = serverDefaultsCache.get(name)
  if (cached) return { ...cached }

  // Fallback to hardcoded TS defaults
  const meta = PROFILES.find(p => p.name === name)
  return meta ? { ...meta.defaults } : { profile: name }
}

export function mergeProfile(base: ProfileConfig, overrides: Partial<ProfileConfig>): ProfileConfig {
  return { ...base, ...overrides }
}

export const SECTION_LABELS: Record<FieldSection, string> = {
  elevation: 'Höhenprofil',
  difficulty: 'Schwierigkeit',
  environment: 'Umgebung',
  paths: 'Wegtypen',
  behavior: 'Verhalten',
}
