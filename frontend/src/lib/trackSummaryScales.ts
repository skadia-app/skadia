import type { CoordRange } from '@/api/routes'
import type { ScaleBadgeLevel } from '@/lib/scaleAggregation'
import { RESAMPLE_SPACING_M } from '@/lib/trackSummaryConstants'

/**
 * Computes per-level distance breakdown from activity track-summary scale ranges.
 *
 * Activity API returns `sac_ranges` / `mtb_ranges` / `vf_ranges` as
 * `Record<level, CoordRange[]>` where each CoordRange indexes into the
 * resampled track (spacing = resampleSpacingM). Distance per range is
 * estimated as `(range.to - range.from) * resampleSpacingM / 1000` km —
 * consistent with how the backend builds the ranges.
 *
 * @param ranges  Record<level, CoordRange[]> from the API (may be null/undefined).
 * @param resampleSpacingM  Resampling interval in metres (from resample_spacing_m field).
 * @param order   Level strings in ascending hardness order.
 * @returns  { maxLevel, breakdown } — maxLevel is the hardest present level,
 *           breakdown lists each level with its total distance in km.
 */
export function computeScaleBreakdown(
  ranges: Record<string, CoordRange[]> | null | undefined,
  resampleSpacingM: number | null | undefined,
  order: readonly string[],
): { maxLevel: string | null; breakdown: ScaleBadgeLevel[] } {
  if (!ranges) return { maxLevel: null, breakdown: [] }

  const spacing = resampleSpacingM ?? RESAMPLE_SPACING_M
  const perLevelKm: Record<string, number> = {}
  let totalKm = 0

  for (const level of order) {
    const levelRanges = ranges[level]
    if (!levelRanges || levelRanges.length === 0) continue
    let sumM = 0
    for (const r of levelRanges) {
      sumM += Math.max(0, r.to - r.from) * spacing
    }
    if (sumM > 0) {
      perLevelKm[level] = sumM / 1000
      totalKm += sumM / 1000
    }
  }

  const breakdown: ScaleBadgeLevel[] = []
  let maxLevel: string | null = null

  for (const level of order) {
    const km = perLevelKm[level]
    if (!km) continue
    breakdown.push({
      level,
      pct: totalKm > 0 ? km / totalKm : 0,
      km,
    })
    maxLevel = level  // ascending order; last present = hardest
  }

  return { maxLevel, breakdown }
}
