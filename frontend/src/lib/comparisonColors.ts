// Central color palette for comparison/fusion/recording-group features.
// Up to 8 sources — cycles if more.

export const SOURCE_COLORS = [
  '#EF4444', '#3B82F6', '#22C55E', '#F59E0B',
  '#8B5CF6', '#EC4899', '#14B8A6', '#F97316',
] as const

export function getSourceColor(index: number): string {
  return SOURCE_COLORS[index % SOURCE_COLORS.length]
}

// ── Line styles per source index ──────────────────────────────────────────────
// Shared by MultiTrackMap (MapLibre layers) and StreamChart (ECharts series).

export type SourceLineStyle = 'solid' | 'dashed' | 'dotted'

/** Returns the line style for a source at the given index. Always solid — differentiation by color only. */
export function getSourceLineStyle(_idx: number): SourceLineStyle {
  return 'solid'
}

/** Returns the line width for a source at the given index. */
export function getSourceLineWidth(idx: number): number {
  if (idx === 0) return 4
  if (idx === 1) return 3
  return 2.5
}

/** Returns the ECharts line width for a source (slightly thinner than map lines). */
export function getSourceChartLineWidth(idx: number): number {
  if (idx === 0) return 2.5
  return 1.5
}

/** Returns the MapLibre line-dasharray for a source. Always solid — differentiation by color only. */
export function getSourceMapDasharray(_idx: number): number[] | undefined {
  return undefined
}
