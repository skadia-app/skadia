import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { formatBestEffortValue } from '@/lib/bestEffortLabels'
import { fmtDateMedium } from '@/lib/format'
import { SPORT_GROUP_LABELS } from '@/lib/recordsConstants'
import type { BestEffortRanked } from '@/api/bestEfforts'

interface HighlightBannerProps {
  records: BestEffortRanked[]
}

export function HighlightBanner({ records }: HighlightBannerProps) {
  const cards = useMemo(() => {
    const rank1 = records.filter((r) => r.rank === 1)
    const result: { label: string; value: string; sub: string; activityId: string; gradient: string }[] = []

    // 1. Most recent PR
    const newest = rank1.reduce<BestEffortRanked | null>((best, r) => {
      if (!best || new Date(r.achieved_at) > new Date(best.achieved_at)) return r
      return best
    }, null)
    if (newest) {
      result.push({
        label: 'Aktueller PR',
        value: formatBestEffortValue(newest.category, newest.elapsed_time_s, newest.value_numeric),
        sub: `${SPORT_GROUP_LABELS[newest.sport_group] ?? newest.sport_group} · ${fmtDateMedium(newest.achieved_at)}`,
        activityId: newest.activity_id,
        gradient: 'from-yellow-500/10 to-yellow-600/5 border-yellow-500/20',
      })
    }

    // 2. Longest distance
    const longestDist = rank1.filter((r) => r.category === 'sup_longest_distance')
      .sort((a, b) => (b.value_numeric ?? 0) - (a.value_numeric ?? 0))[0]
    if (longestDist) {
      result.push({
        label: 'Längste Distanz',
        value: formatBestEffortValue(longestDist.category, longestDist.elapsed_time_s, longestDist.value_numeric),
        sub: `${SPORT_GROUP_LABELS[longestDist.sport_group] ?? ''} · ${fmtDateMedium(longestDist.achieved_at)}`,
        activityId: longestDist.activity_id,
        gradient: 'from-purple-500/10 to-purple-600/5 border-purple-500/20',
      })
    }

    // 3. Most elevation
    const mostElev = rank1.filter((r) => r.category === 'sup_most_elevation')
      .sort((a, b) => (b.value_numeric ?? 0) - (a.value_numeric ?? 0))[0]
    if (mostElev) {
      result.push({
        label: 'Meiste Höhenmeter',
        value: formatBestEffortValue(mostElev.category, mostElev.elapsed_time_s, mostElev.value_numeric),
        sub: `${SPORT_GROUP_LABELS[mostElev.sport_group] ?? ''} · ${fmtDateMedium(mostElev.achieved_at)}`,
        activityId: mostElev.activity_id,
        gradient: 'from-green-500/10 to-green-600/5 border-green-500/20',
      })
    }

    // 4. Best 20min power (only if data exists)
    const bestPower = rank1.filter((r) => r.category === 'power_20min')
      .sort((a, b) => (b.value_numeric ?? 0) - (a.value_numeric ?? 0))[0]
    if (bestPower) {
      result.push({
        label: 'Beste Leistung (20 Min)',
        value: formatBestEffortValue(bestPower.category, bestPower.elapsed_time_s, bestPower.value_numeric),
        sub: `${SPORT_GROUP_LABELS[bestPower.sport_group] ?? ''} · ${fmtDateMedium(bestPower.achieved_at)}`,
        activityId: bestPower.activity_id,
        gradient: 'from-orange-500/10 to-orange-600/5 border-orange-500/20',
      })
    }

    return result
  }, [records])

  if (cards.length === 0) return null

  return (
    <div className="grid gap-3 mb-6 grid-cols-2 md:grid-cols-4">
      {cards.map((card) => (
        <Link
          key={card.label}
          to={`/activities/${card.activityId}`}
          className={`rounded-xl bg-gradient-to-br ${card.gradient} border p-3 hover:brightness-110 transition-all`}
        >
          <div className="text-[10px] font-medium mb-1 opacity-80">{card.label}</div>
          <div className="text-lg font-bold">{card.value}</div>
          <div className="text-[10px] text-neutral-500">{card.sub}</div>
        </Link>
      ))}
    </div>
  )
}
