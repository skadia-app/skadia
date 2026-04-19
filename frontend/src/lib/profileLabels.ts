// Centralised UI labels for routing profiles. i18n-Vorbereitung:
// zentral, keine Streuung, keine Default-Werte.
export interface ProfileLabel {
  label: string
  description: string
}

// Explicit ordering — Object.entries preserves insertion order per ES2015
// for string keys, but deriving via a PROFILE_ORDER array is more defensive
// and makes intent explicit.
export const PROFILE_ORDER = ['hiking', 'mtb', 'gravel', 'trekking', 'road'] as const

export type ProfileKey = typeof PROFILE_ORDER[number]

export const PROFILE_LABELS: Record<ProfileKey, ProfileLabel> = {
  hiking:   { label: 'Wandern',  description: 'Alpines Wandern, SAC T1–T4' },
  mtb:      { label: 'MTB',      description: 'Mountainbike, technische Trails' },
  gravel:   { label: 'Gravel',   description: 'Gravel-Bike, Schotter & Forstwege' },
  trekking: { label: 'Trekking', description: 'Trekking-Rad, gemischte Untergründe' },
  road:     { label: 'Rennrad',  description: 'Rennrad, asphaltierte Strassen' },
}
