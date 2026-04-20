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
