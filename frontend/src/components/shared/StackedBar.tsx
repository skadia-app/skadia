import type { AggregatedSurface } from '@/lib/surfaceConstants'
import { formatLength } from '@/lib/surfaceConstants'

export interface StackedBarProps {
  aggregated: AggregatedSurface[]
  colorMap: Record<string, string>
  labelMap?: Record<string, string>
  title: string
  onHover?: (key: string | null) => void
  onPin?: (key: string) => void
  activeKey?: string | null
  compact?: boolean
}

export default function StackedBar({
  aggregated,
  colorMap,
  labelMap,
  title,
  onHover,
  onPin,
  activeKey,
  compact = false,
}: StackedBarProps) {
  function label(surface_type: string): string {
    if (surface_type === '__other__') return 'Sonstige'
    return labelMap?.[surface_type] ?? surface_type
  }

  function color(surface_type: string): string {
    if (surface_type === '__other__') return '#6b7280'
    return colorMap[surface_type] ?? '#9ca3af'
  }

  return (
    <div className={compact ? 'space-y-0.5' : 'space-y-2'}>
      <h3 className={compact
        ? 'text-[10px] font-medium text-neutral-500 dark:text-neutral-400'
        : 'text-sm font-medium text-neutral-500 dark:text-neutral-400'
      }>{title}</h3>
      {/* Stacked bar */}
      <div className={compact
        ? 'flex h-3 rounded-full overflow-hidden gap-0.5'
        : 'flex h-4 rounded-full overflow-hidden gap-0.5'
      }>
        {aggregated.map(s => (
          <div
            key={s.surface_type}
            style={{
              width: `${s.percentage}%`,
              backgroundColor: color(s.surface_type),
              opacity: activeKey != null && activeKey !== s.surface_type ? 0.4 : 1,
            }}
            title={`${label(s.surface_type)}: ${s.percentage.toFixed(0)}%`}
            onMouseEnter={() => onHover?.(s.surface_type)}
            onMouseLeave={() => onHover?.(null)}
            onClick={() => onPin?.(s.surface_type)}
          />
        ))}
      </div>

      {/* Legend */}
      <div className={compact
        ? 'flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] text-neutral-600 dark:text-neutral-400'
        : 'flex flex-wrap gap-x-4 gap-y-1.5 text-xs text-neutral-600 dark:text-neutral-400'
      }>
        {aggregated.map(s => (
          <span
            key={s.surface_type}
            className={compact ? 'flex items-center gap-1' : 'flex items-center gap-1.5'}
            onMouseEnter={() => onHover?.(s.surface_type)}
            onMouseLeave={() => onHover?.(null)}
            onClick={() => onPin?.(s.surface_type)}
          >
            <span
              className={compact ? 'w-2 h-2 rounded-full inline-block shrink-0' : 'w-2.5 h-2.5 rounded-full inline-block shrink-0'}
              style={{
                backgroundColor: color(s.surface_type),
                opacity: activeKey != null && activeKey !== s.surface_type ? 0.4 : 1,
              }}
            />
            <span
              className="text-neutral-800 dark:text-neutral-200"
              style={{ opacity: activeKey != null && activeKey !== s.surface_type ? 0.4 : 1 }}
            >
              {label(s.surface_type)}
            </span>
            <span className="tabular-nums">{s.percentage.toFixed(0)}%</span>
            <span className="text-neutral-400 dark:text-neutral-500">({formatLength(s.length_m)})</span>
          </span>
        ))}
      </div>
    </div>
  )
}
