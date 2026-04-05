import { Link } from 'react-router-dom'
import { formatBestEffortValue } from '@/lib/bestEffortLabels'
import { fmtDateMedium } from '@/lib/format'
import type { BestEffortRanked } from '@/api/bestEfforts'

const SUP_LABELS: Record<string, string> = {
  sup_longest_distance: 'Längste Distanz',
  sup_most_elevation:   'Meiste Höhenmeter',
  sup_biggest_climb:    'Größter Anstieg',
  sup_longest_duration: 'Längste Dauer',
}

interface SuperlativeBarProps {
  records: BestEffortRanked[]
}

export function SuperlativeBar({ records }: SuperlativeBarProps) {
  const sups = records.filter((r) => r.rank === 1 && r.category.startsWith('sup_'))
  if (sups.length === 0) return null

  const best = new Map<string, BestEffortRanked>()
  for (const s of sups) {
    const existing = best.get(s.category)
    if (!existing || (s.value_numeric ?? 0) > (existing.value_numeric ?? 0)) {
      best.set(s.category, s)
    }
  }

  const items = Array.from(best.values())
  if (items.length === 0) return null

  return (
    <div className="grid gap-3" style={{ gridTemplateColumns: `repeat(${Math.min(items.length, 4)}, minmax(0, 1fr))` }}>
      {items.map((s) => (
        <Link
          key={s.id}
          to={`/activities/${s.activity_id}`}
          className="rounded-lg bg-purple-500/5 border border-purple-500/15 p-2.5 hover:border-purple-500/30 transition-colors"
        >
          <div className="text-[10px] text-purple-400 font-medium">
            {SUP_LABELS[s.category] ?? s.category}
          </div>
          <div className="text-base font-bold">
            {formatBestEffortValue(s.category, s.elapsed_time_s, s.value_numeric)}
          </div>
          <div className="text-[9px] text-neutral-500 dark:text-neutral-400">{fmtDateMedium(s.achieved_at)}</div>
        </Link>
      ))}
    </div>
  )
}
