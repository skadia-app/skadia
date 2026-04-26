import type { BestEffortRanked } from '@/api/bestEfforts'

// Backend ranks best efforts per (user_id, sport_group, category). When a main
// category aggregates several sport_groups (e.g. "Laufen" → running, running_trail,
// running_indoor, running_virtual), each group can contribute its own rank-1 record
// for the same category, producing visible duplicates in the grid/table.
//
// pickBestPerCategory returns one rank-1 record per category — the absolute best
// across all sport_groups present in the input. Selection prefers non-suspicious
// records; among equals, picks by the natural ranking metric:
//   - dist_* / elev_*: lowest elapsed_time_s wins
//   - power_* / sup_* (and any other): highest value_numeric wins
//
// Suspicious records are kept as last resort so that a category never silently
// disappears just because the only candidate is flagged.
export function pickBestPerCategory(records: BestEffortRanked[]): BestEffortRanked[] {
  const byCategory = new Map<string, BestEffortRanked[]>()
  for (const r of records) {
    if (r.rank !== 1) continue
    const arr = byCategory.get(r.category) ?? []
    arr.push(r)
    byCategory.set(r.category, arr)
  }

  const result: BestEffortRanked[] = []
  for (const [category, candidates] of byCategory) {
    const sorted = [...candidates].sort((a, b) => compareForCategory(category, a, b))
    result.push(sorted[0])
  }
  return result
}

function compareForCategory(category: string, a: BestEffortRanked, b: BestEffortRanked): number {
  if (a.suspicious !== b.suspicious) return a.suspicious ? 1 : -1
  if (category.startsWith('dist_') || category.startsWith('elev_')) {
    const ae = a.elapsed_time_s ?? Number.POSITIVE_INFINITY
    const be = b.elapsed_time_s ?? Number.POSITIVE_INFINITY
    return ae - be
  }
  const av = a.value_numeric ?? Number.NEGATIVE_INFINITY
  const bv = b.value_numeric ?? Number.NEGATIVE_INFINITY
  return bv - av
}
