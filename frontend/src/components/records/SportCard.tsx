import type { BestEffortRanked } from '@/api/bestEfforts'
import { CategoryGrid } from './CategoryGrid'
import { MAIN_CATEGORIES, MAIN_CATEGORY_LABELS, type MainCategory } from '@/lib/recordsConstants'
import { cn } from '@/lib/utils'

interface SportCardProps {
  mainCategory: MainCategory
  records: BestEffortRanked[]
  color: string
  onNavigate: () => void
}

const VARIANT_BADGES: Record<string, { label: string; className: string }> = {
  cycling_electric: { label: 'E-Bike', className: 'bg-purple-500/15 text-purple-400 border-purple-500/20' },
  cycling_virtual:  { label: 'Stationär', className: 'bg-neutral-200 dark:bg-neutral-800 text-neutral-500 border-neutral-300 dark:border-neutral-700' },
  swim_ow:          { label: 'Freiwasser', className: 'bg-cyan-500/15 text-cyan-400 border-cyan-500/20' },
  swim_pool:        { label: 'Becken', className: 'bg-cyan-500/10 text-cyan-300 border-cyan-500/15' },
}

export function SportCard({ mainCategory, records, color, onNavigate }: SportCardProps) {
  const rank1Count = records.filter((r) => r.rank === 1).length

  const sportGroups = MAIN_CATEGORIES[mainCategory]
  const presentGroups = new Set(records.map((r) => r.sport_group))
  const badges = sportGroups
    .filter((sg) => VARIANT_BADGES[sg] && presentGroups.has(sg))
    .map((sg) => VARIANT_BADGES[sg])

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
      <div
        className="px-5 py-3 bg-neutral-50 dark:bg-neutral-800/60 border-b border-neutral-200 dark:border-neutral-800 flex items-center gap-2.5 cursor-pointer"
        onClick={onNavigate}
      >
        <span className="w-3 h-3 rounded-full" style={{ backgroundColor: color }} />
        <span className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">
          {MAIN_CATEGORY_LABELS[mainCategory]}
        </span>
        <span className="text-xs text-neutral-500 dark:text-neutral-600">{rank1Count} Rekorde</span>
        {badges.length > 0 && (
          <div className="ml-auto flex items-center gap-1.5">
            {badges.map((b) => (
              <span key={b.label} className={cn('text-[10px] px-1.5 py-0.5 rounded border', b.className)}>
                {b.label}
              </span>
            ))}
            <svg className="w-3 h-3 text-neutral-400 dark:text-neutral-600 ml-1" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path d="M9 5l7 7-7 7" />
            </svg>
          </div>
        )}
        {badges.length === 0 && (
          <span className="ml-auto text-xs text-neutral-400 dark:text-neutral-600 flex items-center gap-1">
            Details
            <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path d="M9 5l7 7-7 7" />
            </svg>
          </span>
        )}
      </div>
      <div className="p-4">
        <CategoryGrid records={records} />
      </div>
    </div>
  )
}
