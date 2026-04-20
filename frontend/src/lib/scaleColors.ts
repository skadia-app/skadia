// SAC-Scale: Grün (T1) → Rot (T6). Kontrastreich auch auf dunklen Basemaps.
export const SAC_COLORS = {
  T1: '#22c55e', T2: '#84cc16', T3: '#eab308',
  T4: '#f97316', T5: '#ef4444', T6: '#991b1b',
} as const

export const MTB_COLORS = {
  S0: '#22c55e', S1: '#84cc16', S2: '#eab308',
  S3: '#f97316', S4: '#ef4444', S5: '#991b1b',
} as const

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
