import { useEffect, useRef } from 'react'
import type { RouteSuggestion } from '@/api/routes'
import { variantLabel, loopQualityDisplay, suggestRoutesStrings } from '@/lib/strings/suggest-routes'
import { useToast } from '@/lib/toast'

export const SUGGESTION_COLORS = ['#F97316', '#3B82F6', '#22C55E', '#A855F7', '#EF4444']
// Orange, Blue, Green, Purple, Red — 5 distinct colors for up to 5 variants

interface RouteComparisonPanelProps {
  suggestions: RouteSuggestion[]
  partialSearch?: boolean
  hoveredIndex: number | null
  onHover: (index: number | null) => void
  onSelect: (index: number) => void
  onCancel: () => void
}

function formatDistance(meters: number): string {
  if (meters >= 1000) {
    return `${(meters / 1000).toFixed(1)} km`
  }
  return `${Math.round(meters)} m`
}

function formatElevation(meters: number): string {
  return `${Math.round(meters)} Hm`
}

export default function RouteComparisonPanel({
  suggestions,
  partialSearch,
  hoveredIndex,
  onHover,
  onSelect,
  onCancel,
}: RouteComparisonPanelProps) {
  const { addToast } = useToast()
  const toastShown = useRef(false)

  useEffect(() => {
    if (toastShown.current || !suggestions || suggestions.length === 0) return
    toastShown.current = true

    if (suggestions.some(s => !s.ascent_target_achieved)) {
      addToast({
        type: 'warning',
        ...suggestRoutesStrings.warnings.ascentNotAchieved,
      })
    }

    if (suggestions.some(s => !s.distance_target_achieved)) {
      addToast({
        type: 'warning',
        ...suggestRoutesStrings.warnings.distanceNotAchieved,
      })
    }

    if (partialSearch) {
      addToast({
        type: 'info',
        ...suggestRoutesStrings.warnings.partialSearch,
      })
    }
  }, [suggestions, partialSearch])

  return (
    <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-50 w-auto max-w-[min(36rem,calc(100vw-2rem))] rounded-xl border border-neutral-200 dark:border-neutral-700 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-xl p-3 space-y-2">
      <div className="flex items-center justify-between px-1">
        <span className="text-xs font-semibold text-neutral-900 dark:text-neutral-100">
          Routenvorschlag ({suggestions.length === 1 ? '1 Variante' : `${suggestions.length} Varianten`})
        </span>
      </div>

      <div className="space-y-1">
        {suggestions.map((s, i) => {
          const isHovered = hoveredIndex === i
          const ariaLabel = `Route ${i + 1}: ${formatDistance(s.distance_m)}, ${formatElevation(s.ascent_m)} Aufstieg`
          return (
            <div
              key={i}
              role="button"
              tabIndex={0}
              aria-label={ariaLabel}
              className={`flex items-center gap-3 px-2.5 py-2 rounded-lg transition-colors cursor-pointer ${
                isHovered
                  ? 'bg-neutral-100 dark:bg-neutral-800'
                  : 'hover:bg-neutral-50 dark:hover:bg-neutral-800/50'
              }`}
              onMouseEnter={() => onHover(i)}
              onMouseLeave={() => onHover(null)}
              onFocus={() => onHover(i)}
              onBlur={() => onHover(null)}
              onClick={() => onSelect(i)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  onSelect(i)
                }
              }}
            >
              {/* Color dot */}
              <span
                className="w-3 h-3 rounded-full shrink-0"
                style={{ backgroundColor: SUGGESTION_COLORS[i % SUGGESTION_COLORS.length] }}
              />

              {/* Variant label */}
              <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400 w-20 shrink-0 truncate">
                {variantLabel(s.variant_id || '')}
              </span>

              {/* Distance */}
              <span className="text-xs font-semibold text-neutral-900 dark:text-neutral-100 w-16 text-right tabular-nums">
                {formatDistance(s.distance_m)}
              </span>

              {/* Ascent */}
              <span className="text-xs text-neutral-600 dark:text-neutral-300 w-16 text-right tabular-nums">
                <span className="text-green-600 dark:text-green-400">&#8593;</span> {formatElevation(s.ascent_m)}
              </span>

              {/* Descent */}
              <span className="text-xs text-neutral-600 dark:text-neutral-300 w-16 text-right tabular-nums">
                <span className="text-red-500 dark:text-red-400">&#8595;</span> {formatElevation(s.descent_m)}
              </span>

              {/* Loop quality badge */}
              {s.loop_quality && (() => {
                const quality = loopQualityDisplay(s.loop_quality)
                return (
                  <span
                    className="text-[10px] px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300"
                    title={quality.label}
                  >
                    {quality.icon}
                  </span>
                )
              })()}

              {/* Select button */}
              <button
                onClick={(e) => {
                  e.stopPropagation()
                  onSelect(i)
                }}
                className="ml-auto px-2.5 py-1 text-[11px] font-medium rounded-md bg-accent-DEFAULT text-white hover:bg-accent-600 transition-colors cursor-pointer shrink-0"
              >
                Übernehmen
              </button>
            </div>
          )
        })}
      </div>

      <div className="flex justify-end px-1 pt-1">
        <button
          onClick={onCancel}
          className="px-3 py-1 text-xs font-medium rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >
          Abbrechen
        </button>
      </div>
    </div>
  )
}
