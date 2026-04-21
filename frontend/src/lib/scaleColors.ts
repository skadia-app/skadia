// Map-Layer-Farben: 6-stufiger Gradient Grün (T1/S0) → Rot (T6/S5).
// Genutzt von MapPage für die SAC/MTB-gefärbten Segment-Overlays, wo die
// visuelle Differenzierung aller 6 Stufen hilfreich ist. Badges/UI-Dots
// nutzen stattdessen SAC_BADGE_COLORS/MTB_BADGE_COLORS (DAV-Konvention).
export const SAC_COLORS = {
  T1: '#22c55e', T2: '#84cc16', T3: '#eab308',
  T4: '#f97316', T5: '#ef4444', T6: '#991b1b',
} as const

export const MTB_COLORS = {
  S0: '#22c55e', S1: '#84cc16', S2: '#eab308',
  S3: '#f97316', S4: '#ef4444', S5: '#991b1b',
} as const

// DAV-Bayern-Konvention — Ski-Pisten-Farben blau < rot < schwarz.
// Single-Source-of-Truth für SAC/MTB-Schwierigkeits-Badges in UI-Kontexten
// (Route-Planner, Segment-Editor). Segment-Editor importiert die Namens-Map
// via `@/lib/scaleColors` für DB-Save (Tailwind-Klassen-Namen), Route-Planner
// nutzt die Hex-Varianten für inline-style-Dots.
export const DAV_COLORS = {
  blue:  '#2563eb',  // Skipiste blau — einfach
  red:   '#dc2626',  // Skipiste rot — mittel
  black: '#1f2937',  // Skipiste schwarz — schwer
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

// Hex-Varianten für inline styles (Badge-Dots etc.). Consumers die den Namen
// brauchen (z.B. für CSS-Klassen-Komposition), nutzen die *_DAV_COLOR_NAMES.
export const SAC_BADGE_COLORS: Record<string, string> = Object.fromEntries(
  Object.entries(SAC_DAV_COLOR_NAMES).map(([level, name]) => [level, DAV_COLORS[name]]),
)

export const MTB_BADGE_COLORS: Record<string, string> = Object.fromEntries(
  Object.entries(MTB_DAV_COLOR_NAMES).map(([level, name]) => [level, DAV_COLORS[name]]),
)

// Forest / Protected: einheitlicher Highlight-Ton, orthogonal zu SAC/MTB,
// damit bei gleichzeitigem Scale-Pin + Descriptor-Hover beide Layer
// visuell unterscheidbar bleiben.
export const HIGHLIGHT_COLORS = {
  forest:    '#16a34a',  // sattes Grün
  protected: '#8b5cf6',  // Violett
} as const

// Via-Ferrata: Gelb (K1) → Dunkelrot (K6). Kontrast mit MTB/SAC durch anderen
// Farbverlauf-Startpunkt (Gelb statt Grün) — Klettersteige sollen visuell
// als "erhöhtes Risiko" auffallen, auch bei K1.
export const VF_COLORS = {
  K1: '#eab308', K2: '#f97316', K3: '#ef4444',
  K4: '#dc2626', K5: '#991b1b', K6: '#450a0a',
} as const

// K? (unscaled via_ferrata) — dunkelrot, weil Schwierigkeit unbekannt und
// im Worst-Case K6 sein kann. Kein Teil von VF_COLORS weil die Range als
// eigener Layer gerendert wird (dashed line).
export const VF_UNKNOWN_COLOR: string = '#7f1d1d'
