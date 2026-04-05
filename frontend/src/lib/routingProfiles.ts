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
    max: 200,
    step: 1.0,
    section: 'elevation',
    hint: 'Gewichtung von Höhenmetern bergauf',
  },
  {
    key: 'downhill_cost',
    label: 'Abstiegskosten',
    type: 'number',
    min: 0.0,
    max: 200,
    step: 1.0,
    section: 'elevation',
    hint: 'Gewichtung von Höhenmetern bergab',
  },
  {
    key: 'uphill_cutoff_pct',
    label: 'Aufstieg Cutoff (%)',
    type: 'number',
    min: 0,
    max: 30,
    step: 0.5,
    unit: '%',
    section: 'elevation',
    hint: 'Ab dieser Steigung werden Aufstiegskosten angewendet',
  },
  {
    key: 'downhill_cutoff_pct',
    label: 'Abstieg Cutoff (%)',
    type: 'number',
    min: 0,
    max: 30,
    step: 0.5,
    unit: '%',
    section: 'elevation',
    hint: 'Ab diesem Gefälle werden Abstiegskosten angewendet',
  },
  {
    key: 'elev_penalty_buffer_m',
    label: 'Höhenpuffer Start (m)',
    type: 'number',
    min: 0,
    max: 50,
    step: 5,
    unit: 'm',
    section: 'elevation',
    hint: 'Höhentoleranz bevor Kostenfunktion greift',
  },
  {
    key: 'elev_max_buffer_m',
    label: 'Höhenpuffer Max (m)',
    type: 'number',
    min: 0,
    max: 100,
    step: 5,
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
    max: 5,
    step: 1,
    section: 'difficulty',
    hint: '0=S0, 1=S1, 2=S2, 3=S3, 4=S4, 5=S5',
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
    max: 1.0,
    step: 0.05,
    section: 'environment',
    hint: 'Rabatt auf Kosten für Waldwege (<1 = bevorzugt)',
  },
  {
    key: 'noise_penalty',
    label: 'Lärm-Malus',
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'environment',
    hint: 'Aufpreis für laute Strassen',
  },
  {
    key: 'traffic_penalty',
    label: 'Verkehrs-Malus',
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'environment',
    hint: 'Aufpreis für stark befahrene Strassen',
  },
  {
    key: 'protected_bonus',
    label: 'Schutzgebiet-Bonus',
    type: 'number',
    min: 0.0,
    max: 1.0,
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
    max: 1.0,
    step: 0.05,
    section: 'paths',
    hint: 'Rabatt auf markierte Radrouten (<1 = bevorzugt)',
  },
  {
    key: 'hikeroute_boost',
    label: 'Wanderrouten-Boost',
    type: 'number',
    min: 0.0,
    max: 1.0,
    step: 0.05,
    section: 'paths',
    hint: 'Rabatt auf markierte Wanderwege (<1 = bevorzugt)',
  },
  {
    key: 'path_preference',
    label: 'Pfad-Präferenz',
    type: 'number',
    min: 0.0,
    max: 30,
    step: 0.5,
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
    max: 150,
    step: 5,
    section: 'behavior',
    hint: 'Zusatzkosten pro Abbiegung',
  },
  {
    key: 'initial_cost_road',
    label: 'Startkosten Strasse',
    type: 'number',
    min: 0.0,
    max: 50000,
    step: 100,
    section: 'behavior',
  },
  {
    key: 'pass0_coefficient',
    label: 'Pass-Koeffizient',
    type: 'number',
    min: 1.0,
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
      uphill_cost: 7,
      downhill_cost: 7,
      uphill_cutoff_pct: 3,
      downhill_cutoff_pct: 3,
      elev_penalty_buffer_m: 5,
      elev_max_buffer_m: 10,
      elev_buffer_reduce: 0.01,
      max_sac_scale: 3,
      sac_preferred: 1,
      allow_steps: true,
      forest_bonus: 0.5,
      noise_penalty: 0.4,
      traffic_penalty: 0.6,
      protected_bonus: 0.3,
      hikeroute_boost: 0.2,
      turn_cost: 0,
      // pass0_coefficient intentionally omitted — backend profile default is authoritative
    },
  },
  {
    name: 'mtb',
    label: 'MTB',
    description: 'Mountainbike, technische Trails',
    defaults: {
      profile: 'mtb',
      uphill_cost: 0,
      downhill_cost: 0,
      uphill_cutoff_pct: 1.5,
      downhill_cutoff_pct: 1.5,
      elev_penalty_buffer_m: 5,
      elev_max_buffer_m: 10,
      elev_buffer_reduce: 0,
      max_mtb_scale: 2,
      allow_steps: false,
      path_preference: 20,
      forest_bonus: 0.3,
      noise_penalty: 0.2,
      cycleroute_boost: 0.2,
      turn_cost: 30,
      // pass0_coefficient intentionally omitted — backend profile default is authoritative
    },
  },
  {
    name: 'gravel',
    label: 'Gravel',
    description: 'Gravel-Bike, Schotter & Forstwege',
    defaults: {
      profile: 'gravel',
      uphill_cost: 80,
      downhill_cost: 40,
      uphill_cutoff_pct: 1.5,
      downhill_cutoff_pct: 1.5,
      elev_penalty_buffer_m: 5,
      elev_max_buffer_m: 10,
      elev_buffer_reduce: 0,
      max_sac_scale: 1,
      max_mtb_scale: 0,
      allow_steps: false,
      forest_bonus: 0.3,
      noise_penalty: 0.4,
      traffic_penalty: 0.5,
      cycleroute_boost: 0.3,
      turn_cost: 65,
      // pass0_coefficient intentionally omitted — backend profile default is authoritative
    },
  },
  {
    name: 'trekking',
    label: 'Trekking',
    description: 'Trekking-Rad, gemischte Untergründe',
    defaults: {
      profile: 'trekking',
      uphill_cost: 0,
      downhill_cost: 60,
      uphill_cutoff_pct: 1.5,
      downhill_cutoff_pct: 1.5,
      elev_penalty_buffer_m: 5,
      elev_max_buffer_m: 10,
      elev_buffer_reduce: 0,
      allow_steps: true,
      allow_ferries: true,
      cycleroute_boost: 0.6,
      turn_cost: 90,
      // pass0_coefficient intentionally omitted — backend profile default is authoritative
    },
  },
  {
    name: 'road',
    label: 'Rennrad',
    description: 'Rennrad, asphaltierte Strassen',
    defaults: {
      profile: 'road',
      uphill_cost: 0,
      downhill_cost: 60,
      uphill_cutoff_pct: 1.5,
      downhill_cutoff_pct: 1.5,
      elev_penalty_buffer_m: 5,
      elev_max_buffer_m: 10,
      elev_buffer_reduce: 0,
      allow_steps: false,
      traffic_penalty: 0.1,
      turn_cost: 90,
      // pass0_coefficient intentionally omitted — backend profile default is authoritative
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
    'track_grade3_cost', 'track_grade4_cost', 'track_grade5_cost',
    'footway_cost', 'cycleway_cost', 'bridleway_cost',
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
