export interface ProfileLabel {
  labelKey: `routePlanner.profileEditor.profiles.${string}.label`
  descriptionKey: `routePlanner.profileEditor.profiles.${string}.description`
}

// Explicit ordering — Object.entries preserves insertion order per ES2015
// for string keys, but deriving via a PROFILE_ORDER array is more defensive
// and makes intent explicit.
export const PROFILE_ORDER = ['hiking', 'mtb', 'gravel', 'trekking', 'road'] as const

export type ProfileKey = typeof PROFILE_ORDER[number]

export const PROFILE_LABELS: Record<ProfileKey, ProfileLabel> = {
  hiking: {
    labelKey: 'routePlanner.profileEditor.profiles.hiking.label',
    descriptionKey: 'routePlanner.profileEditor.profiles.hiking.description',
  },
  mtb: {
    labelKey: 'routePlanner.profileEditor.profiles.mtb.label',
    descriptionKey: 'routePlanner.profileEditor.profiles.mtb.description',
  },
  gravel: {
    labelKey: 'routePlanner.profileEditor.profiles.gravel.label',
    descriptionKey: 'routePlanner.profileEditor.profiles.gravel.description',
  },
  trekking: {
    labelKey: 'routePlanner.profileEditor.profiles.trekking.label',
    descriptionKey: 'routePlanner.profileEditor.profiles.trekking.description',
  },
  road: {
    labelKey: 'routePlanner.profileEditor.profiles.road.label',
    descriptionKey: 'routePlanner.profileEditor.profiles.road.description',
  },
}
