import { Link } from 'react-router-dom'
import { formatBestEffortDisplay, categoryLabel } from '@/lib/bestEffortLabels'
import type { BestEffortRanked } from '@/api/bestEfforts'
import { POOL_BADGE, OW_BADGE } from '@/lib/recordsConstants'
import { cn } from '@/lib/utils'

interface PRCellProps {
  record: BestEffortRanked
  sportGroup?: string
  showGroupBadge?: boolean
  onClick?: () => void
}

function groupBadge(sportGroup: string): string | null {
  if (sportGroup === 'swim_pool') return POOL_BADGE
  if (sportGroup === 'swim_ow') return OW_BADGE
  return null
}

export function PRCell({ record, sportGroup, showGroupBadge, onClick }: PRCellProps) {
  const display = formatBestEffortDisplay(
    record.category,
    record.elapsed_time_s,
    record.value_numeric,
    sportGroup ?? record.sport_group,
  )
  const label = categoryLabel(record.category)
  const badge = showGroupBadge ? groupBadge(record.sport_group) : null

  return (
    <Link
      to={`/activities/${record.activity_id}`}
      onClick={onClick}
      className={cn(
        'bg-neutral-100 dark:bg-neutral-800/40 border border-neutral-200 dark:border-neutral-800 rounded-lg p-2 transition-all',
        'hover:border-orange-400 dark:hover:border-orange-500/40 hover:-translate-y-px cursor-pointer',
      )}
    >
      <div className="text-[10px] text-neutral-500 dark:text-neutral-500">
        {label}
        {badge && <span className="text-cyan-600 dark:text-cyan-500 ml-1">{badge}</span>}
      </div>
      <div className="text-sm font-bold tabular-nums">{display.primary}</div>
      {display.secondary && (
        <div className="text-[9px] text-neutral-500">{display.secondary}</div>
      )}
    </Link>
  )
}
