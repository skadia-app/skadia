import type { CoordRange } from '@/api/routes'

const SAC_ORDER = ['T1', 'T2', 'T3', 'T4', 'T5', 'T6'] as const
const MTB_ORDER = ['S0', 'S1', 'S2', 'S3', 'S4', 'S5'] as const

export interface ScaleBadgeLevel {
  level: string   // "T1".."T6" für SAC, "S0".."S5" für MTB
  pct: number     // 0–1, Anteil an Route-Länge
  km: number      // kumulierte Distanz in km
}

function haversineM(a: number[], b: number[]): number {
  const [lon1, lat1] = a
  const [lon2, lat2] = b
  const R = 6371000
  const toRad = (deg: number) => (deg * Math.PI) / 180
  const dLat = toRad(lat2 - lat1)
  const dLon = toRad(lon2 - lon1)
  const s =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * Math.sin(dLon / 2) ** 2
  return 2 * R * Math.asin(Math.sqrt(s))
}

function rangeLengthM(coords: number[][], range: CoordRange): number {
  let sum = 0
  // range.to is inclusive: sum haversine distances from coords[from] to coords[to],
  // clamped to coords.length-1 to guard against out-of-bounds on the last edge.
  const to = Math.min(range.to, coords.length - 1)
  for (let i = range.from + 1; i <= to; i++) {
    sum += haversineM(coords[i - 1], coords[i])
  }
  return sum
}

/**
 * Aggregates backend Coord-Ranges into per-level distance breakdown.
 * No threshold — outdoor-safety invariant I8 requires every non-zero level
 * to be visible in the badge (a 5m T6 passage may be life-threatening).
 */
export function aggregateScaleLevels(
  ranges: Record<string, CoordRange[]> | undefined,
  coordinates: number[][],
  kind: 'sac' | 'mtb',
): { maxLevel: string | null; breakdown: ScaleBadgeLevel[] } {
  if (!ranges || coordinates.length < 2) {
    return { maxLevel: null, breakdown: [] }
  }
  const order = kind === 'sac' ? SAC_ORDER : MTB_ORDER
  const perLevelM: Record<string, number> = {}
  let totalM = 0

  for (const level of order) {
    const levelRanges = ranges[level]
    if (!levelRanges || levelRanges.length === 0) continue
    let sum = 0
    for (const r of levelRanges) sum += rangeLengthM(coordinates, r)
    if (sum > 0) {
      perLevelM[level] = sum
      totalM += sum
    }
  }

  const breakdown: ScaleBadgeLevel[] = []
  let maxLevel: string | null = null
  for (const level of order) {
    const m = perLevelM[level]
    if (!m) continue
    breakdown.push({
      level,
      pct: totalM > 0 ? m / totalM : 0,
      km: m / 1000,
    })
    maxLevel = level  // order is ascending; last present = hardest
  }

  return { maxLevel, breakdown }
}
