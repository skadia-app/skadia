// Map layer colors: six-step green (T1/S0) to red (T6/S5) gradient.
// Used by MapPage for SAC/MTB-colored segment overlays, where visually
// separating all six levels is useful. Badges/UI dots use
// SAC_BADGE_COLORS/MTB_BADGE_COLORS instead (DAV convention).
export const SAC_COLORS = {
  T1: '#22c55e', T2: '#84cc16', T3: '#eab308',
  T4: '#f97316', T5: '#ef4444', T6: '#991b1b',
} as const

export const MTB_COLORS = {
  S0: '#22c55e', S1: '#84cc16', S2: '#eab308',
  S3: '#f97316', S4: '#ef4444', S5: '#991b1b',
} as const

// DAV Bavaria convention: ski-slope colors blue < red < black.
// Single source of truth for SAC/MTB difficulty badges in UI contexts
// (route planner, segment editor). Segment editor imports the name map via
// `@/lib/scaleColors` for DB saves (Tailwind class names); route planner uses
// the hex variants for inline-style dots.
export const DAV_COLORS = {
  blue:  '#2563eb',  // blue ski slope: easy
  red:   '#dc2626',  // red ski slope: medium
  black: '#1f2937',  // black ski slope: hard
} as const

export type DavColorName = keyof typeof DAV_COLORS

export const SAC_DAV_COLOR_NAMES: Record<string, DavColorName> = {
  T1: 'blue',
  T2: 'red',   T3: 'red',
  T4: 'black', T5: 'black', T6: 'black',
}

export const MTB_DAV_COLOR_NAMES: Record<string, DavColorName> = {
  S0: 'blue', S1: 'blue',
  S2: 'red',
  S3: 'black', S4: 'black', S5: 'black',
}

// Hex variants for inline styles (badge dots, etc.). Consumers that need the
// color name, for example CSS class composition, use the *_DAV_COLOR_NAMES maps.
export const SAC_BADGE_COLORS: Record<string, string> = Object.fromEntries(
  Object.entries(SAC_DAV_COLOR_NAMES).map(([level, name]) => [level, DAV_COLORS[name]]),
)

export const MTB_BADGE_COLORS: Record<string, string> = Object.fromEntries(
  Object.entries(MTB_DAV_COLOR_NAMES).map(([level, name]) => [level, DAV_COLORS[name]]),
)

// Forest / protected: unified highlight colors, orthogonal to SAC/MTB, so
// simultaneous scale pins and descriptor hover layers remain visually distinct.
export const HIGHLIGHT_COLORS = {
  forest:    '#16a34a',  // saturated green
  protected: '#8b5cf6',  // violet
} as const

// Via ferrata: yellow (K1) to dark red (K6). The different gradient start
// point (yellow instead of green) contrasts with MTB/SAC and makes via ferratas
// read as elevated risk even at K1.
export const VF_COLORS = {
  K1: '#eab308', K2: '#f97316', K3: '#ef4444',
  K4: '#dc2626', K5: '#991b1b', K6: '#450a0a',
} as const

// K? (unscaled via_ferrata): dark red because difficulty is unknown and may be
// K6 in the worst case. Kept outside VF_COLORS because the range is rendered as
// its own dashed-line layer.
export const VF_UNKNOWN_COLOR: string = '#7f1d1d'
