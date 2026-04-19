// frontend/src/lib/colorScaleRanges.ts
/**
 * Motion profiles for sport-dependent absolute scales (Speed, Pace, Cadence).
 *
 * Separate from sportTaxonomy.segmentGroup because routing-group (bike/foot/ski/paddle)
 * is too coarse for motion-physics (Road vs. MTB, Alpin vs. Langlauf, etc.).
 *
 * CI guard in tests ensures every sportType in SPORT_DEFS has an explicit mapping.
 */

export const SPEED_PROFILES = {
  walking:     { minKmh: 0, maxKmh: 8   },
  hiking:      { minKmh: 0, maxKmh: 12  },
  running:     { minKmh: 0, maxKmh: 20  },
  roadCycling: { minKmh: 0, maxKmh: 60  },
  mtbCycling:  { minKmh: 0, maxKmh: 40  },
  alpineSki:   { minKmh: 0, maxKmh: 80  },
  xcSki:       { minKmh: 0, maxKmh: 30  },
  skitour:     { minKmh: 0, maxKmh: 60  },
  paddle:      { minKmh: 0, maxKmh: 15  },
  rowing:      { minKmh: 0, maxKmh: 20  },
  horseback:   { minKmh: 0, maxKmh: 50  },
  water:       { minKmh: 0, maxKmh: 50  }, // surf/wake/sail
  motorized:   { minKmh: 0, maxKmh: 200 },
  generic:     { minKmh: 0, maxKmh: 50  },
} as const

export const PACE_PROFILES = {
  walking: { minMinPerKm: 8,  maxMinPerKm: 20 },
  hiking:  { minMinPerKm: 10, maxMinPerKm: 25 },
  running: { minMinPerKm: 3,  maxMinPerKm: 12 },
  generic: { minMinPerKm: 4,  maxMinPerKm: 15 },
} as const

export const CADENCE_PROFILES = {
  foot_running: { min: 120, max: 200 }, // spm
  foot_walking: { min: 80,  max: 140 }, // spm
  bike:         { min: 0,   max: 120 }, // rpm
  paddle:       { min: 20,  max: 80  }, // spm
  generic:      { min: 0,   max: 150 },
} as const

export type SpeedProfileKey = keyof typeof SPEED_PROFILES
export type PaceProfileKey = keyof typeof PACE_PROFILES
export type CadenceProfileKey = keyof typeof CADENCE_PROFILES

export interface MotionProfile {
  speed: SpeedProfileKey
  pace?: PaceProfileKey    // undefined when pace is meaningless (cycling, skiing, etc.)
  cadence: CadenceProfileKey
}

/**
 * Explicit mapping of every sportType in SPORT_DEFS to a motion profile.
 *
 * New sportTypes in sportTaxonomy.ts WITHOUT an entry here will fail CI
 * (see colorScaleRanges.test.ts).
 */
export const MOTION_PROFILE_BY_SPORT_TYPE: Record<string, MotionProfile> = {
  // Foot: running
  running:             { speed: 'running', pace: 'running', cadence: 'foot_running' },
  trail_running:       { speed: 'running', pace: 'running', cadence: 'foot_running' },
  treadmill_running:   { speed: 'running', pace: 'running', cadence: 'foot_running' },
  wheelchair_push_run: { speed: 'running', pace: 'running', cadence: 'generic' },

  // Foot: walking
  walking:              { speed: 'walking', pace: 'walking', cadence: 'foot_walking' },
  wheelchair_push_walk: { speed: 'walking', pace: 'walking', cadence: 'generic' },

  // Foot: hiking
  hiking:         { speed: 'hiking', pace: 'hiking', cadence: 'foot_walking' },
  mountaineering: { speed: 'hiking', pace: 'hiking', cadence: 'foot_walking' },
  snowshoeing:    { speed: 'hiking', pace: 'hiking', cadence: 'foot_walking' },

  // Climb
  rock_climbing:  { speed: 'hiking', cadence: 'generic' },
  floor_climbing: { speed: 'hiking', cadence: 'generic' },

  // Bike
  cycling:         { speed: 'roadCycling', cadence: 'bike' },
  gravel_cycling:  { speed: 'roadCycling', cadence: 'bike' },
  mountain_biking: { speed: 'mtbCycling',  cadence: 'bike' },
  indoor_cycling:  { speed: 'roadCycling', cadence: 'bike' },

  // Ski / Snowboard
  alpine_skiing:        { speed: 'alpineSki', cadence: 'generic' },
  skiing:               { speed: 'alpineSki', cadence: 'generic' },
  snowboarding:         { speed: 'alpineSki', cadence: 'generic' },
  backcountry_skiing:   { speed: 'skitour',   cadence: 'generic' },
  cross_country_skiing: { speed: 'xcSki',     cadence: 'generic' },
  sledding:             { speed: 'alpineSki', cadence: 'generic' },

  // Water: paddle
  kayaking:                { speed: 'paddle', cadence: 'paddle' },
  canoeing:                { speed: 'paddle', cadence: 'paddle' },
  paddling:                { speed: 'paddle', cadence: 'paddle' },
  stand_up_paddleboarding: { speed: 'paddle', cadence: 'paddle' },
  rafting:                 { speed: 'paddle', cadence: 'paddle' },

  // Water: rowing
  rowing: { speed: 'rowing', cadence: 'paddle' },

  // Water: swim
  swimming:            { speed: 'paddle', cadence: 'generic' },
  lap_swimming:        { speed: 'paddle', cadence: 'generic' },
  open_water_swimming: { speed: 'paddle', cadence: 'generic' },
  snorkeling:          { speed: 'paddle', cadence: 'generic' },
  diving:              { speed: 'paddle', cadence: 'generic' },

  // Water: surf/wake (higher speeds)
  surfing:      { speed: 'water', cadence: 'generic' },
  windsurfing:  { speed: 'water', cadence: 'generic' },
  kitesurfing:  { speed: 'water', cadence: 'generic' },
  wakeboarding: { speed: 'water', cadence: 'generic' },
  wakesurfing:  { speed: 'water', cadence: 'generic' },
  water_skiing: { speed: 'water', cadence: 'generic' },
  water_tubing: { speed: 'water', cadence: 'generic' },

  // Water: boating
  sailing: { speed: 'water',   cadence: 'generic' },
  boating: { speed: 'water',   cadence: 'generic' },
  fishing: { speed: 'generic', cadence: 'generic' },

  // Snow: motorized
  snowmobiling: { speed: 'motorized', cadence: 'generic' },

  // Motorized
  motorcycling: { speed: 'motorized', cadence: 'generic' },
  driving:      { speed: 'motorized', cadence: 'generic' },

  // Air
  flying:       { speed: 'motorized', cadence: 'generic' },
  hang_gliding: { speed: 'motorized', cadence: 'generic' },
  sky_diving:   { speed: 'motorized', cadence: 'generic' },

  // Skate
  ice_skating:    { speed: 'xcSki', cadence: 'generic' },
  inline_skating: { speed: 'xcSki', cadence: 'generic' },

  // Horse
  horseback_riding: { speed: 'horseback', cadence: 'generic' },

  // Ball sports (GPS rare)
  american_football: { speed: 'running', cadence: 'generic' },
  baseball:          { speed: 'running', cadence: 'generic' },
  basketball:        { speed: 'running', cadence: 'generic' },
  soccer:            { speed: 'running', cadence: 'generic' },
  rugby:             { speed: 'running', cadence: 'generic' },
  cricket:           { speed: 'running', cadence: 'generic' },
  hockey:            { speed: 'running', cadence: 'generic' },
  lacrosse:          { speed: 'running', cadence: 'generic' },
  volleyball:        { speed: 'generic', cadence: 'generic' },
  tennis:            { speed: 'generic', cadence: 'generic' },
  racket:            { speed: 'generic', cadence: 'generic' },
  golf:              { speed: 'walking', pace: 'walking', cadence: 'generic' },
  disc_golf:         { speed: 'walking', pace: 'walking', cadence: 'generic' },

  // Fitness / Indoor / Combat
  fitness_equipment:  { speed: 'generic', cadence: 'generic' },
  strength:           { speed: 'generic', cadence: 'generic' },
  strength_training:  { speed: 'generic', cadence: 'generic' },
  hiit:               { speed: 'generic', cadence: 'generic' },
  jump_rope:          { speed: 'generic', cadence: 'generic' },
  yoga:               { speed: 'generic', cadence: 'generic' },
  pilates:            { speed: 'generic', cadence: 'generic' },
  meditation:         { speed: 'generic', cadence: 'generic' },
  dance:              { speed: 'generic', cadence: 'generic' },
  boxing:             { speed: 'generic', cadence: 'generic' },
  mixed_martial_arts: { speed: 'generic', cadence: 'generic' },
  tactical:           { speed: 'generic', cadence: 'generic' },
  jumpmaster:         { speed: 'generic', cadence: 'generic' },

  // Other
  hunting:    { speed: 'hiking',  pace: 'hiking', cadence: 'generic' },
  multisport: { speed: 'generic', cadence: 'generic' },
  transition: { speed: 'walking', cadence: 'generic' },
  training:   { speed: 'generic', cadence: 'generic' },
  generic:    { speed: 'generic', cadence: 'generic' },
  unknown:    { speed: 'generic', cadence: 'generic' },
}

/**
 * Sports allowed to have a `generic` speed profile.
 * Catches forgotten mappings: if a new sportType is added to the mapping
 * with `speed: 'generic'` but not listed here, the CI guard fails.
 */
export const GENERIC_ALLOWLIST: ReadonlyArray<string> = [
  'fishing',
  'volleyball', 'tennis', 'racket',
  'fitness_equipment', 'strength', 'strength_training', 'hiit', 'jump_rope',
  'yoga', 'pilates', 'meditation', 'dance', 'boxing', 'mixed_martial_arts',
  'tactical', 'jumpmaster',
  'multisport', 'training', 'generic', 'unknown',
]

/** Absolute temperature range in °C. Clamped beyond. */
export const TEMPERATURE_RANGE = { min: -10, max: 35 } as const

/** Minimum spread for altitude (activity-relative scale). */
export const ALTITUDE_MIN_SPREAD = 50 // m

/** Minimum spread for HR/Power linear fallback (no user zones configured). */
export const HR_FALLBACK_MIN_SPREAD = 30 // bpm
export const POWER_FALLBACK_MIN_SPREAD = 50 // W

/**
 * Minimum data coverage (as fraction of total stream points) required for a metric
 * to be offered in the dropdown AND retained by the hook auto-reset.
 *
 * 0.3 is a pragmatic trade-off: high enough to filter out streams where a metric
 * is present only as stray artifacts (e.g. strength training with occasional HR pings),
 * low enough to accept sensor warm-up periods on short activities (~16min) without
 * hiding HR/Power/etc.
 */
export const METRIC_AVAILABILITY_THRESHOLDS = {
  hr:          0.3,
  speed:       0.3,
  altitude:    0.3,
  cadence:     0.3,
  power:       0.3,
  temperature: 0.3,
} as const

export type ThresholdMetric = keyof typeof METRIC_AVAILABILITY_THRESHOLDS

/** Expand [min, max] to at least minSpread, centered on midpoint. */
export function expandRange(min: number, max: number, minSpread: number): [number, number] {
  if (max - min >= minSpread) return [min, max]
  const mid = (min + max) / 2
  return [mid - minSpread / 2, mid + minSpread / 2]
}
