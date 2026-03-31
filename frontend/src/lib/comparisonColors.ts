// Central color palette for comparison/fusion/recording-group features.
// Up to 8 sources — cycles if more.

export const SOURCE_COLORS = [
  '#EF4444', '#3B82F6', '#22C55E', '#F59E0B',
  '#8B5CF6', '#EC4899', '#14B8A6', '#F97316',
] as const

export function getSourceColor(index: number): string {
  return SOURCE_COLORS[index % SOURCE_COLORS.length]
}
