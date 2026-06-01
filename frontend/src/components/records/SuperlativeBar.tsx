import { Link } from 'react-router-dom'
import { categoryLabel, formatBestEffortValue } from '@/lib/bestEffortLabels'
import { fmtDateMedium } from '@/lib/format'
import type { SupportedLanguage } from '@/i18n/languages'
import type { BestEffortRanked } from '@/api/bestEfforts'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

interface SuperlativeBarProps {
  records: BestEffortRanked[]
  language?: SupportedLanguage
}

export function SuperlativeBar({ records, language }: SuperlativeBarProps) {
  const unitFormatters = useUnitFormatters()
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
            {categoryLabel(s.category, language)}
          </div>
          <div className="text-base font-bold">
            {formatBestEffortValue(s.category, s.elapsed_time_s, s.value_numeric, unitFormatters.options)}
          </div>
          <div className="text-[9px] text-neutral-500 dark:text-neutral-400">{fmtDateMedium(s.achieved_at)}</div>
        </Link>
      ))}
    </div>
  )
}
