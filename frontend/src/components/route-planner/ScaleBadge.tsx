import { useState, useRef, useEffect } from 'react'
import { SAC_COLORS, MTB_COLORS, VF_COLORS } from '@/lib/scaleColors'
import type { ScaleBadgeLevel } from '@/lib/scaleAggregation'
import { scaleMaxLabel, scaleAriaLabel } from '@/lib/strings/suggest-routes'

interface Props {
  kind: 'sac' | 'mtb' | 'vf'
  breakdown: ScaleBadgeLevel[]   // ascending hardness order
  maxLevel: string               // hardest level present (for badge dot color)
  isPinned: boolean
  onTogglePin: () => void
  onHover: (active: boolean) => void
}

function colorFor(kind: 'sac' | 'mtb' | 'vf', level: string): string {
  const src = kind === 'sac' ? SAC_COLORS : kind === 'mtb' ? MTB_COLORS : VF_COLORS
  return (src as Record<string, string>)[level] ?? '#71717a'
}

export function ScaleBadge({ kind, breakdown, maxLevel, isPinned, onTogglePin, onHover }: Props) {
  const [open, setOpen] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const openNow = () => {
    if (timer.current) clearTimeout(timer.current)
    setOpen(true)
    onHover(true)
  }

  const closeWithGuard = (relatedTarget: EventTarget | null) => {
    const rt = relatedTarget as HTMLElement | null
    if (rt?.closest?.('.scale-badge')) return  // pointer moving to another badge
    setOpen(false)
    onHover(false)
  }

  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])

  const label = scaleMaxLabel(maxLevel)
  const ariaLabel = scaleAriaLabel(kind, maxLevel)

  return (
    <div className="relative inline-block">
      <button
        type="button"
        className={`scale-badge inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs bg-neutral-100 hover:bg-neutral-200 dark:bg-neutral-800 dark:hover:bg-neutral-700 ${isPinned ? 'ring-2 ring-accent' : ''}`}
        aria-pressed={isPinned}
        aria-label={ariaLabel}
        onMouseEnter={openNow}
        onMouseLeave={(e) => closeWithGuard(e.relatedTarget)}
        onFocus={openNow}
        onBlur={(e) => closeWithGuard(e.relatedTarget)}
        onClick={onTogglePin}
      >
        <span
          className="inline-block w-2 h-2 rounded-full"
          style={{ backgroundColor: colorFor(kind, maxLevel) }}
          aria-hidden="true"
        />
        {label}
      </button>
      {open && breakdown.length > 0 && (
        <div
          className="absolute z-10 top-full left-0 mt-1 bg-white dark:bg-neutral-900 shadow-lg rounded p-2 text-xs min-w-[10rem]"
          role="tooltip"
        >
          {breakdown.map(b => (
            <div key={b.level} className="flex items-center gap-2 py-0.5">
              <span
                className="inline-block w-3 h-3 rounded-sm"
                style={{ backgroundColor: colorFor(kind, b.level) }}
                aria-hidden="true"
              />
              <span className="font-medium">{b.level}</span>
              <span className="ml-auto tabular-nums text-neutral-600 dark:text-neutral-400">
                {b.km < 0.1 ? `${Math.round(b.km * 1000)} m` : `${b.km.toFixed(2)} km`}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
