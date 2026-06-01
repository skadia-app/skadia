import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RouteSuggestion } from '@/api/routes'
import type { VariantDescriptor } from '@/api/routes'
import { useToast } from '@/lib/toast'
import { INTERACTIVE_DESCRIPTOR_KEYS } from '@/lib/descriptorFilter'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { descriptorLabel, loopQualityDisplay, variantLabel } from '@/lib/strings/suggest-routes'

export const SUGGESTION_COLORS = ['#F97316', '#3B82F6', '#22C55E', '#A855F7', '#EF4444']
// Orange, Blue, Green, Purple, Red — 5 distinct colors for up to 5 variants

interface RouteComparisonPanelProps {
  suggestions: RouteSuggestion[]
  partialSearch?: boolean
  hoveredIndex: number | null
  onHover: (index: number | null) => void
  onSelect: (index: number) => void
  onCancel: () => void
  onHoverDescriptor?: (key: string | null) => void
  onClickDescriptor?: (key: string) => void
}

export default function RouteComparisonPanel({
  suggestions,
  partialSearch,
  hoveredIndex,
  onHover,
  onSelect,
  onCancel,
  onHoverDescriptor,
  onClickDescriptor,
}: RouteComparisonPanelProps) {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const { formatDistanceCompact, formatElevation } = useUnitFormatters()
  const toastShown = useRef(false)

  const [expandedRoutes, setExpandedRoutes] = useState<Set<number>>(new Set())
  function toggleExpand(routeIdx: number) {
    setExpandedRoutes(prev => {
      const next = new Set(prev)
      if (next.has(routeIdx)) next.delete(routeIdx)
      else next.add(routeIdx)
      return next
    })
  }

  // Reset expanded state when suggestions list changes (stale indices after re-suggest)
  useEffect(() => {
    setExpandedRoutes(new Set())
  }, [suggestions])

  // Outside-click collapse (Spec §6.2)
  const panelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (expandedRoutes.size === 0) return
    function onDocDown(e: MouseEvent) {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setExpandedRoutes(new Set())
      }
    }
    document.addEventListener('mousedown', onDocDown)
    return () => document.removeEventListener('mousedown', onDocDown)
  }, [expandedRoutes.size])

  useEffect(() => {
    if (toastShown.current || !suggestions || suggestions.length === 0) return
    toastShown.current = true

    if (suggestions.some(s => !s.ascent_target_achieved)) {
      addToast({
        type: 'warning',
        title: t('routePlanner.comparison.warnings.ascentNotAchieved.title'),
        message: t('routePlanner.comparison.warnings.ascentNotAchieved.message'),
      })
    }

    if (suggestions.some(s => !s.distance_target_achieved)) {
      addToast({
        type: 'warning',
        title: t('routePlanner.comparison.warnings.distanceNotAchieved.title'),
        message: t('routePlanner.comparison.warnings.distanceNotAchieved.message'),
      })
    }

    if (partialSearch) {
      addToast({
        type: 'info',
        title: t('routePlanner.comparison.warnings.partialSearch.title'),
        message: t('routePlanner.comparison.warnings.partialSearch.message'),
      })
    }
  }, [suggestions, partialSearch, addToast, t])

  return (
    <div ref={panelRef} className="absolute bottom-4 left-1/2 -translate-x-1/2 z-50 w-auto max-w-[min(36rem,calc(100vw-2rem))] rounded-xl border border-neutral-200 dark:border-neutral-700 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-xl p-3 space-y-2">
      <div className="flex items-center justify-between px-1">
        <span className="text-xs font-semibold text-neutral-900 dark:text-neutral-100">
          {t('routePlanner.comparison.title', { count: suggestions.length })}
        </span>
      </div>

      <div className="space-y-1">
        {suggestions.map((s, i) => {
          const isHovered = hoveredIndex === i
          const ariaLabel = t('routePlanner.comparison.routeAria', {
            number: i + 1,
            distance: formatDistanceCompact(s.distance_m),
            ascent: formatElevation(s.ascent_m),
          })
          return (
            <div
              key={i}
              role="button"
              tabIndex={0}
              aria-label={ariaLabel}
              className={`flex flex-col gap-1 px-2.5 py-2 rounded-lg transition-colors cursor-pointer ${
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
              {/* Row 1: Numeric line — Color dot + Label + Distance + Ascent + Descent + Loop badge + Button */}
              <div className="flex items-center gap-3">
                {/* Color dot */}
                <span
                  className="w-3 h-3 rounded-full shrink-0"
                  style={{ backgroundColor: SUGGESTION_COLORS[i % SUGGESTION_COLORS.length] }}
                />

                {/* Variant label (no chips here) */}
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400 w-20 shrink-0 truncate">
                  {variantLabel(s.variant_id || '', t)}
                </span>

                {/* Distance */}
                <span className="text-xs font-semibold text-neutral-900 dark:text-neutral-100 w-16 text-right tabular-nums">
                  {formatDistanceCompact(s.distance_m)}
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
                  const quality = loopQualityDisplay(s.loop_quality, t)
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
                  {t('routePlanner.comparison.apply')}
                </button>
              </div>

              {/* Row 2: Descriptor chips — only rendered when descriptors are present */}
              {s.variant_descriptors && s.variant_descriptors.length > 0 && (() => {
                const sorted = [...s.variant_descriptors].sort((a, b) => b.magnitude - a.magnitude)
                const isExpanded = expandedRoutes.has(i)
                const visible = isExpanded ? sorted : sorted.slice(0, 3)
                const remaining = sorted.length - 3
                return (
                  <div className="flex flex-wrap gap-0.5 pl-6">
                    {/* pl-6 (24px) aligns under the variant label, past the color dot (w-3) + gap-3 (12px) */}
                    {visible.map((d: VariantDescriptor, idx: number) => {
                      const label = descriptorLabel(d.key, d.display_value, t)
                      const interactive = INTERACTIVE_DESCRIPTOR_KEYS.has(d.key)
                      if (interactive) {
                        return (
                          <button
                            key={idx}
                            type="button"
                            className="descriptor-chip text-[9px] leading-tight px-1 py-px rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400 whitespace-nowrap hover:bg-neutral-200 dark:hover:bg-neutral-700"
                            title={label}
                            onMouseEnter={() => onHoverDescriptor?.(d.key)}
                            onMouseLeave={(e) => {
                              // Race-fix: if pointer moves directly to another descriptor chip,
                              // that chip's onMouseEnter has not yet fired; naive null-clear here
                              // would overwrite the incoming chip's hover. Skip clear if
                              // relatedTarget is any other .descriptor-chip in this panel.
                              const rt = e.relatedTarget as HTMLElement | null
                              if (rt && rt.closest?.('.descriptor-chip')) return
                              onHoverDescriptor?.(null)
                            }}
                            onClick={(e) => { e.stopPropagation(); onClickDescriptor?.(d.key) }}
                          >
                            {label}
                          </button>
                        )
                      }
                      return (
                        <span
                          key={idx}
                          className="text-[9px] leading-tight px-1 py-px rounded bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400 whitespace-nowrap"
                          title={label}
                        >
                          {label}
                        </span>
                      )
                    })}
                    {!isExpanded && remaining > 0 && (
                      <button
                        type="button"
                        className="text-[9px] leading-tight px-1 py-px rounded bg-neutral-200 dark:bg-neutral-700 text-neutral-600 dark:text-neutral-300"
                        onClick={(e) => { e.stopPropagation(); toggleExpand(i) }}
                      >
                        {t('routePlanner.comparison.moreDescriptors', { count: remaining })}
                      </button>
                    )}
                  </div>
                )
              })()}
            </div>
          )
        })}
      </div>

      <div className="flex justify-end px-1 pt-1">
        <button
          onClick={onCancel}
          className="px-3 py-1 text-xs font-medium rounded-lg border border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
        >
          {t('routePlanner.comparison.cancel')}
        </button>
      </div>
    </div>
  )
}
