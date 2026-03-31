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

const LINE_STYLES: SourceLineStyle[] = ['solid', 'dashed', 'dotted']

/** Returns the line style for a source at the given index (cycles every 3). */
export function getSourceLineStyle(idx: number): SourceLineStyle {
  return LINE_STYLES[idx % LINE_STYLES.length]
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

/** Returns the MapLibre line-dasharray for a source (undefined = solid). */
export function getSourceMapDasharray(idx: number): number[] | undefined {
  const style = getSourceLineStyle(idx)
  if (style === 'dashed') return [4, 2]
  if (style === 'dotted') return [2, 2]
  return undefined
}
