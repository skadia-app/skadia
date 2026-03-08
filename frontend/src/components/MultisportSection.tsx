import { Link } from 'react-router-dom'
import { SportIcon } from '@/components/SportIcon'
import { sportLabel, formatDuration, formatDistance } from '@/lib/utils'
import type { Activity } from '@/api/types'

interface Props {
  children: Activity[]
}

export default function MultisportSection({ children }: Props) {
  if (children.length === 0) return null

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400">
        Multisport-Disziplinen
      </h3>
      {children.map((child, i) => (
        <Link
          key={child.id}
          to={`/activities/${child.id}`}
          className="flex items-center gap-3 p-3 rounded-lg border border-neutral-200 dark:border-neutral-800
                     hover:bg-neutral-50 dark:hover:bg-neutral-800/60 transition-colors"
        >
          <SportIcon sport={child.sport_type} subType={child.sub_type} className="w-5 h-5 shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="font-medium text-neutral-900 dark:text-neutral-100 truncate">
              {child.title ?? sportLabel(child.sport_type, child.sub_type)}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              {child.elapsed_time_s != null && formatDuration(child.elapsed_time_s)}
              {child.elapsed_time_s != null && child.distance_m != null && ' · '}
              {child.distance_m != null && formatDistance(child.distance_m)}
            </div>
          </div>
          <span className="text-xs text-neutral-400 dark:text-neutral-600 shrink-0">
            #{i + 1}
          </span>
        </Link>
      ))}
    </div>
  )
}
