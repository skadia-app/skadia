import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { SportIcon } from '@/components/SportIcon'
import { sportLabel, formatDuration, formatDistance } from '@/lib/utils'
import type { Activity } from '@/api/types'

interface Props {
  children: Activity[]
  onHoverChild?: (id: string | null) => void
}

type StatItem = { key: string; node: ReactNode }

function Dot() {
  return <span className="mx-1 text-neutral-300 dark:text-neutral-600">·</span>
}

function ClockIcon() {
  return (
    <svg className="w-3 h-3 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <circle cx="8" cy="8" r="6" />
      <path d="M8 5v3l2 1.5" />
    </svg>
  )
}

function RouteIcon() {
  return (
    <svg className="w-3 h-3 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 8h12M10 5l4 3-4 3" />
    </svg>
  )
}

function ElevUpIcon() {
  return (
    <svg className="w-3 h-3 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M8 13V3M4 7l4-4 4 4" />
    </svg>
  )
}

function ElevDownIcon() {
  return (
    <svg className="w-3 h-3 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M8 3v10M4 9l4 4 4-4" />
    </svg>
  )
}

export default function MultisportSection({ children, onHoverChild }: Props) {
  if (children.length === 0) return null

  return (
    <div className="space-y-2">
      {children.map((child, i) => {
        const stats: StatItem[] = []

        if (child.elapsed_time_s != null)
          stats.push({
            key: 'dur',
            node: (
              <span className="inline-flex items-center gap-0.5">
                <ClockIcon />
                <span>{formatDuration(child.elapsed_time_s)}</span>
              </span>
            ),
          })

        if (child.distance_m != null)
          stats.push({
            key: 'dist',
            node: (
              <span className="inline-flex items-center gap-0.5">
                <RouteIcon />
                <span>{formatDistance(child.distance_m)}</span>
              </span>
            ),
          })

        const hasGain = (child.elevation_gain_m ?? 0) > 0
        const hasLoss = (child.elevation_loss_m ?? 0) > 0
        if (hasGain || hasLoss)
          stats.push({
            key: 'ele',
            node: (
              <span className="inline-flex items-center gap-1">
                {hasGain && (
                  <span className="inline-flex items-center gap-0.5">
                    <ElevUpIcon />
                    <span>{Math.round(child.elevation_gain_m!)}m</span>
                  </span>
                )}
                {hasGain && hasLoss && <span className="text-neutral-300 dark:text-neutral-600">/</span>}
                {hasLoss && (
                  <span className="inline-flex items-center gap-0.5">
                    <ElevDownIcon />
                    <span>{Math.round(child.elevation_loss_m!)}m</span>
                  </span>
                )}
              </span>
            ),
          })

        if (child.avg_heart_rate != null)
          stats.push({
            key: 'hr',
            node: (
              <span className="inline-flex items-center gap-0.5">
                <span className="text-rose-400 text-sm leading-none">♥</span>
                <span>{Math.round(child.avg_heart_rate)} bpm</span>
              </span>
            ),
          })

        if (child.avg_power_w != null)
          stats.push({
            key: 'pwr',
            node: (
              <span className="inline-flex items-center gap-0.5">
                <span className="text-yellow-500">⚡</span>
                <span>{Math.round(child.avg_power_w)}W</span>
              </span>
            ),
          })

        if (child.avg_speed_ms != null && !child.is_indoor)
          stats.push({ key: 'spd', node: <span>{(child.avg_speed_ms * 3.6).toFixed(1)} km/h</span> })

        return (
          <Link
            key={child.id}
            to={`/activities/${child.id}`}
            className="flex items-center gap-3 p-3 rounded-lg border border-neutral-200 dark:border-neutral-800
                       hover:bg-neutral-50 dark:hover:bg-neutral-800/60 transition-colors"
            onMouseEnter={() => onHoverChild?.(child.id)}
            onMouseLeave={() => onHoverChild?.(null)}
          >
            <SportIcon sport={child.sport_type} subType={child.sub_type} className="w-5 h-5 shrink-0" />
            <div className="flex-1 min-w-0">
              <div className="font-medium text-neutral-900 dark:text-neutral-100 truncate">
                {child.title ?? sportLabel(child.sport_type, child.sub_type)}
              </div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5 flex flex-wrap items-center">
                {stats.map((s, idx) => (
                  <span key={s.key} className="inline-flex items-center">
                    {idx > 0 && <Dot />}
                    {s.node}
                  </span>
                ))}
              </div>
            </div>
            <span className="text-xs text-neutral-400 dark:text-neutral-600 shrink-0">
              #{i + 1}
            </span>
          </Link>
        )
      })}
    </div>
  )
}
