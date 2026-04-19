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
    key: 'max_trail_visibility',
    label: 'Max. Sichtbarkeit',
    type: 'int',
    min: 0,
    max: 6,
    step: 1,
    section: 'difficulty',
    hint: '0=deaktiviert, 1=sehr gut sichtbar, 2=gut, 3=mäßig, 4=schlecht, 5=kaum, 6=nicht sichtbar',
  },
  {
    key: 'trail_visibility_penalty',
    label: 'Pfad-Unsichtbarkeit-Bestrafung',
    type: 'number',
    min: 0,
    max: 2,
    step: 0.1,
    section: 'difficulty',
    hint: 'Kosten-Aufschlag pro Stufe (0.0=aus, 0.5=moderat, 1.0=stark)',
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
    'allow_steps', 'allow_ferries', 'wet_weather',
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

export const SECTION_LABELS: Record<FieldSection, string> = {
  elevation: 'Höhenprofil',
  difficulty: 'Schwierigkeit',
  environment: 'Umgebung',
  paths: 'Wegtypen',
  behavior: 'Verhalten',
}
