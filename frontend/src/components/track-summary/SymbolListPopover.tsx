import { useId } from 'react'
import { Building2, Droplet, Mountain } from 'lucide-react'
import type { SummaryItem, StatsKind } from './StatsRow'
import { useHoverPopover } from '@/hooks/useHoverPopover'

interface Props {
  kind: StatsKind
  icon: React.ReactNode
  label: string
  items: SummaryItem[]
  // Only fired for kind='summit' — water/settlement chips are purely informational.
  onHover?: (kind: StatsKind, items: SummaryItem[]) => void
  onLeave?: () => void
}

function itemIcon(kind: StatsKind) {
  if (kind === 'summit') return <Mountain className="h-3 w-3 shrink-0 text-neutral-400" aria-hidden="true" />
  if (kind === 'water') return <Droplet className="h-3 w-3 shrink-0 text-neutral-400" aria-hidden="true" />
  return <Building2 className="h-3 w-3 shrink-0 text-neutral-400" aria-hidden="true" />
}

export function SymbolListPopover({ kind, icon, label, items, onHover, onLeave }: Props) {
  // Stable ID for aria-describedby wiring (tooltip pattern — no listbox/option semantics
  // because we don't implement keyboard arrow navigation that role="listbox" implies).
  const tooltipId = useId()

  const { open, triggerProps, popoverProps, sentinelAttr } = useHoverPopover({
    onOpen: () => {
      // Only summit chips trigger map highlighting.
      if (kind === 'summit') onHover?.(kind, items)
    },
    onClose: () => {
      if (kind === 'summit') onLeave?.()
    },
  })

  const ariaLabel =
    kind === 'summit'
      ? `${label}, Gipfelliste öffnen`
      : kind === 'water'
        ? `${label}, Gewässerliste öffnen`
        : `${label}, Ortschaftsliste öffnen`

  return (
    <div className="relative inline-block" {...{ [sentinelAttr]: '' }}>
      <button
        type="button"
        className="inline-flex items-center gap-1.5 rounded-md bg-neutral-100 px-2 py-1 text-neutral-700 transition-colors hover:bg-neutral-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT/60 dark:bg-neutral-800 dark:text-neutral-200 dark:hover:bg-neutral-700"
        aria-label={ariaLabel}
        aria-describedby={open ? tooltipId : undefined}
        {...triggerProps}
      >
        {icon}
        <span>{label}</span>
      </button>

      {open && items.length > 0 && (
        <div
          id={tooltipId}
          role="tooltip"
          className="absolute z-20 bottom-full left-0 mb-1 min-w-[200px] max-h-56 overflow-y-auto rounded-lg bg-white/95 dark:bg-neutral-800/95 shadow-lg border border-neutral-200 dark:border-neutral-700 p-2 text-xs"
          {...popoverProps}
        >
          <ul className="space-y-1">
            {items.map((item, idx) => {
              const key = item.osm_id != null && item.osm_id > 0
                ? String(item.osm_id)
                : `${idx}-${item.name}`
              return (
                <li
                  key={key}
                  className="flex items-center justify-between gap-2 text-neutral-700 dark:text-neutral-300"
                >
                  <span className="flex items-center gap-1 min-w-0 shrink">
                    {itemIcon(kind)}
                    <span className="break-words">{item.name || '(kein Name)'}</span>
                  </span>
                  {item.ele_m != null && (
                    <span className="text-right text-neutral-500 dark:text-neutral-400 tabular-nums shrink-0 ml-auto pl-2">
                      {item.ele_m.toLocaleString('de')} m
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </div>
  )
}

export default SymbolListPopover
