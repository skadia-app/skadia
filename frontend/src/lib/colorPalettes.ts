// frontend/src/lib/colorPalettes.ts
/** Missing / undefined metric value segments. */
export const MISSING_COLOR = '#cccccc'

/** Viridis perceptually-uniform sequential palette (9 stops).
 *  Used for: Speed, Pace, Cadence, Altitude, Power-fallback. */
export const VIRIDIS_9 = [
  '#440154', '#482878', '#3e4989',
  '#31688e', '#26828e', '#1f9e89',
  '#35b779', '#6ece58', '#b5de2b',
] as const

/** Diverging palette for temperature — neutral beige at 18 °C (comfort zone). */
export const DIVERGING_TEMP_STOPS: ReadonlyArray<{ value: number; color: string }> = [
  { value: -10, color: '#1e3a8a' },
  { value:   0, color: '#3b82f6' },
  { value:  12, color: '#93c5fd' },
  { value:  18, color: '#fef3c7' },
  { value:  25, color: '#f97316' },
  { value:  35, color: '#7f1d1d' },
]

/** HR-Zone base palette (up to 6 zones). */
export const HR_ZONE_BASE = [
  '#3b82f6', '#10b981', '#eab308',
  '#f97316', '#ef4444', '#7f1d1d',
] as const

/** Power-Zone base palette (Coggan 7 + reserve, up to 8 zones). */
export const POWER_ZONE_BASE = [
  '#3b82f6', '#06b6d4', '#10b981',
  '#eab308', '#f97316', '#ef4444',
  '#7f1d1d', '#581c87',
] as const

/** Speed-Zone aqua palette — visually distinct from HR to avoid confusion
 *  when switching between HR and Speed coloring. Up to 6 zones. */
export const SPEED_ZONE_BASE = [
  '#dbeafe', '#93c5fd', '#3b82f6',
  '#1d4ed8', '#1e3a8a', '#0c1a4a',
] as const

function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '').toLowerCase()
  return [
    parseInt(h.slice(0, 2), 16),
    parseInt(h.slice(2, 4), 16),
    parseInt(h.slice(4, 6), 16),
  ]
}

function rgbToHex(r: number, g: number, b: number): string {
  const h = (n: number) => Math.round(n).toString(16).padStart(2, '0')
  return `#${h(r)}${h(g)}${h(b)}`
}

/** Linear RGB interpolation between two hex colors; t ∈ [0, 1]. */
export function interpolateHex(a: string, b: string, t: number): string {
  const [ar, ag, ab] = hexToRgb(a)
  const [br, bg, bb] = hexToRgb(b)
  return rgbToHex(
    ar + (br - ar) * t,
    ag + (bg - ag) * t,
    ab + (bb - ab) * t,
  )
}
