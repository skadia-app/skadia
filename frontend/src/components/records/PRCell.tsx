import { Link } from 'react-router-dom'
import { formatBestEffortDisplay, categoryLabel } from '@/lib/bestEffortLabels'
import { fmtDateMedium } from '@/lib/format'
import type { BestEffortRanked } from '@/api/bestEfforts'
import { POOL_BADGE, OW_BADGE } from '@/lib/recordsConstants'
import { cn } from '@/lib/utils'

interface PRCellProps {
  record: BestEffortRanked
  sportGroup?: string
  showGroupBadge?: boolean
  colorHint?: 'blue' | 'orange' | 'green'
  onClick?: () => void
}

function groupBadge(sportGroup: string): string | null {
  if (sportGroup === 'swim_pool') return POOL_BADGE
  if (sportGroup === 'swim_ow') return OW_BADGE
  return null
}

const VALUE_COLOR: Record<string, string> = {
  orange: 'text-orange-600 dark:text-orange-300',
  green:  'text-green-600 dark:text-green-300',
}

export function PRCell({ record, sportGroup, showGroupBadge, colorHint, onClick }: PRCellProps) {
  const display = formatBestEffortDisplay(
    record.category,
    record.elapsed_time_s,
    record.value_numeric,
    sportGroup ?? record.sport_group,
  )
  const label = categoryLabel(record.category)
  const badge = showGroupBadge ? groupBadge(record.sport_group) : null
  const valueColor = colorHint ? (VALUE_COLOR[colorHint] ?? '') : ''

  return (
    <Link
      to={`/activities/${record.activity_id}`}
      onClick={onClick}
      className={cn(
        'bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200/70 dark:border-neutral-700/50 rounded-lg p-2 transition-all',
        'hover:border-orange-400 dark:hover:border-orange-500/40 hover:-translate-y-px cursor-pointer',
      )}
    >
      <div className="text-[10px] text-neutral-500 dark:text-neutral-500">
        {label}
        {badge && <span className="text-cyan-600 dark:text-cyan-500 ml-1">{badge}</span>}
      </div>
      <div className={cn('text-sm font-bold tabular-nums', valueColor)}>{display.primary}</div>
      {display.secondary && (
        <div className="text-[9px] text-neutral-500">{display.secondary}</div>
      )}
      <div className="text-[9px] text-neutral-500 dark:text-neutral-500">{fmtDateMedium(record.achieved_at)}</div>
    </Link>
  )
}
