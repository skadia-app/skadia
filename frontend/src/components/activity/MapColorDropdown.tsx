// frontend/src/components/activity/MapColorDropdown.tsx
import { useMemo, useRef, useState, useEffect } from 'react'
import type { StreamPoint } from '@/api/types'
import type { Metric } from '@/lib/mapColorUtils'
import { METRIC_LABELS } from '@/lib/colorMetricLabels'
import { MOTION_PROFILE_BY_SPORT_TYPE } from '@/lib/colorScaleRanges'
import { useClickOutside } from '@/hooks/useClickOutside'

const AVAILABILITY_THRESHOLDS: Record<Exclude<Metric, 'sport' | 'pace'>, number> = {
  hr:          0.5,
  speed:       0.5,
  altitude:    0.5,
  cadence:     0.4,
  power:       0.3,
  temperature: 0.4,
}

function pctNonNull(streams: StreamPoint[], field: keyof StreamPoint): number {
  if (streams.length === 0) return 0
  let n = 0
  for (const s of streams) if (s[field] != null) n++
  return n / streams.length
}

interface Props {
  streams: StreamPoint[]
  sportType: string
  metric: Metric
  onChange: (m: Metric) => void
}

export default function MapColorDropdown({ streams, sportType, metric, onChange }: Props) {
  const [open, setOpen] = useState(false)
  const [focusedIdx, setFocusedIdx] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

  useClickOutside(containerRef, () => setOpen(false), open)

  // Close on Escape
  useEffect(() => {
    if (!open) return
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') { setOpen(false); return }
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [open])

  const motion = MOTION_PROFILE_BY_SPORT_TYPE[sportType] ?? MOTION_PROFILE_BY_SPORT_TYPE.generic
  const speedLabel = motion.pace ? METRIC_LABELS.pace : METRIC_LABELS.speed
  const speedMetric: Metric = motion.pace ? 'pace' : 'speed'

  const cadenceLabel =
    motion.cadence === 'bike'   ? METRIC_LABELS.cadence_bike
    : motion.cadence === 'paddle' ? METRIC_LABELS.cadence_paddle
    :                                METRIC_LABELS.cadence_foot

  const availableOptions = useMemo(() => {
    const opts: Array<{ key: Metric; label: string }> = [
      { key: 'sport', label: METRIC_LABELS.sport },
    ]
    if (pctNonNull(streams, 'heart_rate') >= AVAILABILITY_THRESHOLDS.hr) {
      opts.push({ key: 'hr', label: METRIC_LABELS.hr })
    }
    if (pctNonNull(streams, 'speed_ms') >= AVAILABILITY_THRESHOLDS.speed) {
      opts.push({ key: speedMetric, label: speedLabel })
    }
    if (pctNonNull(streams, 'altitude_m') >= AVAILABILITY_THRESHOLDS.altitude) {
      opts.push({ key: 'altitude', label: METRIC_LABELS.altitude })
    }
    if (pctNonNull(streams, 'cadence') >= AVAILABILITY_THRESHOLDS.cadence) {
      opts.push({ key: 'cadence', label: cadenceLabel })
    }
    if (pctNonNull(streams, 'power_w') >= AVAILABILITY_THRESHOLDS.power) {
      opts.push({ key: 'power', label: METRIC_LABELS.power })
    }
    if (pctNonNull(streams, 'temperature_c') >= AVAILABILITY_THRESHOLDS.temperature) {
      opts.push({ key: 'temperature', label: METRIC_LABELS.temperature })
    }
    return opts
  }, [streams, speedLabel, speedMetric, cadenceLabel])

  const current = availableOptions.find(o => o.key === metric) ?? availableOptions[0]

  // When dropdown opens, focus the current selection
  useEffect(() => {
    if (open) {
      const idx = availableOptions.findIndex(o => o.key === metric)
      setFocusedIdx(idx >= 0 ? idx : 0)
    }
  }, [open, availableOptions, metric])

  // Arrow-Keys within the menu
  function handleMenuKeyDown(e: React.KeyboardEvent<HTMLUListElement>) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setFocusedIdx(i => Math.min(availableOptions.length - 1, i + 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setFocusedIdx(i => Math.max(0, i - 1))
    } else if (e.key === 'Home') {
      e.preventDefault(); setFocusedIdx(0)
    } else if (e.key === 'End') {
      e.preventDefault(); setFocusedIdx(availableOptions.length - 1)
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onChange(availableOptions[focusedIdx].key)
      setOpen(false)
    }
  }

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="Einfärbung wählen"
        className="flex items-center gap-1.5 px-2.5 h-8 rounded-lg bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md text-xs font-medium text-neutral-700 dark:text-neutral-300 cursor-pointer hover:bg-white dark:hover:bg-neutral-700 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-1"
      >
        <span>{current.label}</span>
        <svg viewBox="0 0 20 20" fill="currentColor" className="w-3 h-3" aria-hidden="true"><path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.06l3.71-3.83a.75.75 0 111.08 1.04l-4.25 4.4a.75.75 0 01-1.08 0l-4.25-4.4a.75.75 0 01.02-1.06z" clipRule="evenodd" /></svg>
      </button>

      {open && (
        <ul
          ref={listRef}
          role="menu"
          onKeyDown={handleMenuKeyDown}
          tabIndex={-1}
          autoFocus
          className="absolute top-full left-0 mt-1 min-w-[160px] rounded-lg bg-white dark:bg-neutral-800 shadow-lg border border-neutral-200 dark:border-neutral-700 py-1 z-20 focus:outline-none"
        >
          {availableOptions.map((opt, i) => (
            <li
              key={opt.key}
              role="menuitemradio"
              aria-checked={opt.key === metric}
              aria-label={opt.label}
              tabIndex={i === focusedIdx ? 0 : -1}
              onClick={() => { onChange(opt.key); setOpen(false) }}
              onMouseEnter={() => setFocusedIdx(i)}
              className={`block px-3 py-1.5 text-xs cursor-pointer ${
                i === focusedIdx ? 'bg-neutral-100 dark:bg-neutral-700' : ''
              } ${
                opt.key === metric
                  ? 'font-semibold text-blue-600 dark:text-blue-400'
                  : 'text-neutral-700 dark:text-neutral-300'
              }`}
            >
              {opt.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
