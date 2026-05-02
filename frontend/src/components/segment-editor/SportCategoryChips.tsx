import { useState } from 'react'
import { Check, ChevronDown, ChevronRight, X } from 'lucide-react'
import { Pill } from '../ui/Pill'
import { sportsInCategory } from '@/lib/sportTaxonomy'
import {
  getVisibleCategories,
  isCategoryFullyActive,
  isCategoryPartiallyActive,
  whitelistForSegmentType,
  type UISportCategory,
} from '@/lib/sportCategoryHelpers'
import { type SegType } from './constants'
import { cn } from '@/lib/utils'

// ── Enrichment types ──────────────────────────────────────────────────────────
// These describe per-sport-type enrichment badge data. The source/status types
// are intentionally generic so callers (Sidebar, future Wizard) can shape them.

export interface SportEnrichmentSource {
  /** Short human-readable label (e.g. "OSM") */
  label: string
  /** Optional coverage 0–1 */
  coverage?: number
}

export type SportEnrichmentStatus = 'loading' | 'done' | 'error'

// ── Internal DetailPanel ───────────────────────────────────────────────────────

interface DetailPanelProps {
  /** The UI category being expanded */
  category: UISportCategory
  /** All sport types available for this segment type (for filtering) */
  availableSportTypes: Array<{ key: string; label: string; category: string }>
  allowedSelectionKeys?: readonly string[]
  /** Currently selected sport types (null = all) */
  selectedSportTypes: string[] | null
  onToggleSportType: (key: string) => void
  /** Per-sport-type enrichment sources — optional, shown as badges when present */
  enrichmentSources?: Record<string, SportEnrichmentSource[]>
  onClearEnrichmentSource?: (sportType: string, source: SportEnrichmentSource) => void
  enrichmentStatus?: SportEnrichmentStatus
}

function DetailPanel({
  category,
  availableSportTypes,
  allowedSelectionKeys,
  selectedSportTypes,
  onToggleSportType,
  enrichmentSources,
  onClearEnrichmentSource,
}: DetailPanelProps) {
  const catSports = category.taxonomyKeys
    .flatMap(tk => sportsInCategory(tk))
    .filter(s => availableSportTypes.some(a => a.key === s.sportType))

  if (catSports.length === 0) return null

  return (
    <div className="flex flex-wrap gap-1 pl-3">
      {catSports.flatMap(s => {
        const entries = [{ key: s.sportType, label: s.label }]
        for (const st of s.subTypes) {
          entries.push({ key: `${s.sportType}:${st.subType}`, label: st.label })
        }
        return entries.map(entry => ({ sport: s, entry }))
      }).filter(({ entry }) => {
        if (!allowedSelectionKeys) return true
        if (allowedSelectionKeys.includes(entry.key)) return true
        const [parent, subType] = entry.key.split(':', 2)
        return subType != null && allowedSelectionKeys.includes(parent)
      }).map(({ sport, entry }) => {
        const isActive = selectedSportTypes?.includes(entry.key) ?? false
        const badges = enrichmentSources?.[sport.sportType]

        return (
          <div key={entry.key} className="flex items-center gap-0.5">
            <Pill
              size="sm"
              role="checkbox"
              ariaChecked={isActive}
              active={isActive}
              onClick={() => onToggleSportType(entry.key)}
              leftSlot={isActive ? <Check className="h-3 w-3 shrink-0" /> : undefined}
            >
              {entry.label}
            </Pill>

            {/* Enrichment source badges — shown only when enrichmentSources provided */}
            {badges && badges.length > 0 && onClearEnrichmentSource && (
              <div className="flex gap-0.5">
                {badges.map((src, i) => (
                  <button
                    key={i}
                    type="button"
                    onClick={() => onClearEnrichmentSource(sport.sportType, src)}
                    title={`${src.label}${src.coverage != null ? ` (${Math.round(src.coverage * 100)}%)` : ''} — klicken zum Entfernen`}
                    className={cn(
                      'inline-flex items-center gap-0.5 rounded-full border px-1.5 py-0 text-[10px] font-medium',
                      'border-blue-300 bg-blue-50 text-blue-700',
                      'dark:border-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
                      'hover:border-blue-400 hover:bg-blue-100 dark:hover:bg-blue-900/50',
                      'transition-colors duration-150',
                    )}
                  >
                    {src.label}
                    <X className="h-2.5 w-2.5 shrink-0" />
                  </button>
                ))}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

// ── SportCategoryChips ─────────────────────────────────────────────────────────

interface SportCategoryChipsProps {
  segType: SegType
  activeTypes: string[] | null
  onToggleCategory: (uiKey: string) => void
  onSetAll: () => void
  /**
   * All sport types available for this segment type — required by detail picker
   * to filter sports to only those valid for the current segType.
   * If omitted when showDetailPicker=true, all taxonomy sports are shown.
   */
  availableSportTypes?: Array<{ key: string; label: string; category: string }>
  /**
   * When true, each category chip can be clicked to expand a detail panel
   * showing individual sport type checkboxes. The category toggle (all on/off)
   * moves to a separate checkbox indicator. Default: false.
   */
  showDetailPicker?: boolean
  /** Callback to toggle a single sport type (required when showDetailPicker=true) */
  onToggleSportType?: (key: string) => void
  /** Per-sport-type enrichment source badges — optional */
  enrichmentSources?: Record<string, SportEnrichmentSource[]>
  onClearEnrichmentSource?: (sportType: string, source: SportEnrichmentSource) => void
  enrichmentStatus?: SportEnrichmentStatus
}

export function SportCategoryChips({
  segType,
  activeTypes,
  onToggleCategory,
  onSetAll,
  availableSportTypes,
  showDetailPicker = false,
  onToggleSportType,
  enrichmentSources,
  onClearEnrichmentSource,
  enrichmentStatus,
}: SportCategoryChipsProps) {
  // Internal accordion state — uncontrolled; only used when showDetailPicker=true
  const [openCategoryKey, setOpenCategoryKey] = useState<string | null>(null)

  const visible = getVisibleCategories(segType)
  const whitelist = whitelistForSegmentType(segType)

  // Fall back to "all taxonomy sports" if availableSportTypes not provided
  const resolvedAvailableSportTypes: Array<{ key: string; label: string; category: string }> =
    availableSportTypes ??
    visible.flatMap(cat =>
      cat.taxonomyKeys.flatMap(tk =>
        sportsInCategory(tk).map(s => ({ key: s.sportType, label: s.label, category: tk }))
      )
    )

  const toggleDetail = (key: string) =>
    setOpenCategoryKey(prev => (prev === key ? null : key))

  // #MINOR-9: Hide "Alle" when only 1 category is visible (e.g. Wintersport = 1 category).
  // A single visible category makes "Alle" semantically identical to that category chip.
  const showAllChip = visible.length > 1

  return (
    <div className="flex flex-col gap-2">
      {/* ── Category chip row ── */}
      <div className="flex flex-wrap gap-1.5" role="group" aria-label="Sportarten-Filter">
        {showAllChip && (
          <Pill
            role="checkbox"
            ariaChecked={activeTypes === null}
            active={activeTypes === null}
            onClick={onSetAll}
            leftSlot={activeTypes === null ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
          >
            Alle
          </Pill>
        )}

        {visible.map(cat => {
          const isActive = isCategoryFullyActive(cat.key, activeTypes, whitelist)
          const isPartial = isCategoryPartiallyActive(cat.key, activeTypes, whitelist)

          if (showDetailPicker) {
            // #8a + #8b: Split the chip into two separate interaction zones:
            //   - Pill body click → toggle category on/off (calls onToggleCategory).
            //     Enables true multi-select: "Rad + Laufen" both active simultaneously.
            //   - Chevron button (nested, stopPropagation) → open/close detail panel.
            //     Lets users drill into per-sport fine-tuning independently of toggling.
            const isOpen = openCategoryKey === cat.key
            return (
              <div key={cat.key} className="relative inline-flex">
                <Pill
                  role="checkbox"
                  ariaChecked={isActive ? true : (isPartial ? 'mixed' : false)}
                  active={isActive || isOpen}
                  onClick={() => onToggleCategory(cat.key)}
                  leftSlot={isActive ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
                  className={cn(
                    // Reserve right-side space so the label never overlaps the chevron button
                    'pr-7',
                    isPartial && !isActive ? 'border-accent-DEFAULT/40 bg-accent-DEFAULT/10' : undefined,
                    isOpen && !isActive ? 'border-neutral-400 dark:border-neutral-500' : undefined,
                  )}
                >
                  {cat.label}
                </Pill>
                {/* Chevron as its own focusable button — stopPropagation keeps it independent */}
                <button
                  type="button"
                  aria-label={isOpen ? `${cat.label} zuklappen` : `${cat.label} aufklappen`}
                  onClick={e => { e.stopPropagation(); toggleDetail(cat.key) }}
                  className={cn(
                    'absolute inset-y-0 right-0 flex items-center justify-center w-7 rounded-r-lg',
                    'text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200',
                    'transition-colors duration-150',
                    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1',
                  )}
                >
                  {isOpen
                    ? <ChevronDown className="h-3.5 w-3.5 shrink-0" />
                    : <ChevronRight className="h-3.5 w-3.5 shrink-0" />
                  }
                </button>
              </div>
            )
          }

          // Default (no detail picker): chip click = toggle all category sports
          return (
            <Pill
              key={cat.key}
              role="checkbox"
              ariaChecked={isActive ? true : (isPartial ? 'mixed' : false)}
              active={isActive}
              onClick={() => onToggleCategory(cat.key)}
              leftSlot={isActive ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
              className={isPartial && !isActive ? 'border-accent-DEFAULT/40 bg-accent-DEFAULT/10' : undefined}
            >
              {cat.label}
            </Pill>
          )
        })}
      </div>

      {/* ── Detail panels (showDetailPicker mode, standard segTypes) ── */}
      {showDetailPicker && segType !== 'wintersport' && segType !== 'wassersport' && openCategoryKey && (
        <div className="mt-1 flex flex-col gap-1.5">
          {visible
            .filter(cat => cat.key === openCategoryKey)
            .map(cat => {
              const isOpen = openCategoryKey === cat.key
              if (!isOpen) return null
              return (
                <div key={cat.key}>
                  {/* Category label + toggle-all button */}
                  <div className="flex items-center justify-between mb-1 px-0.5">
                    <span className="text-[10px] font-semibold tracking-wider uppercase text-neutral-400 dark:text-neutral-500">
                      {cat.label} — Feinwahl
                    </span>
                    <button
                      type="button"
                      onClick={() => onToggleCategory(cat.key)}
                      className="text-[10px] text-neutral-400 hover:text-accent-DEFAULT dark:hover:text-accent-300 transition-colors"
                    >
                      {isCategoryFullyActive(cat.key, activeTypes, whitelist) ? 'Alle aus' : 'Alle an'}
                    </button>
                  </div>
	                  <DetailPanel
	                    category={cat}
	                    availableSportTypes={resolvedAvailableSportTypes}
	                    allowedSelectionKeys={whitelist}
	                    selectedSportTypes={activeTypes}
                    onToggleSportType={onToggleSportType ?? (() => {})}
                    enrichmentSources={enrichmentSources}
                    onClearEnrichmentSource={onClearEnrichmentSource}
                    enrichmentStatus={enrichmentStatus}
                  />
                </div>
              )
            })
          }
        </div>
      )}

      {/* ── Detail panel for wintersport/wassersport (flat list, showDetailPicker) ── */}
      {showDetailPicker && activeTypes !== null && (segType === 'wintersport' || segType === 'wassersport') && (
        <div className="flex flex-wrap gap-1">
          {resolvedAvailableSportTypes.map(s => {
            const isActive = activeTypes?.includes(s.key) ?? false
            return (
              <Pill
                key={s.key}
                size="sm"
                role="checkbox"
                ariaChecked={isActive}
                active={isActive}
                onClick={() => (onToggleSportType ?? (() => {}))(s.key)}
                leftSlot={isActive ? <Check className="h-3 w-3 shrink-0" /> : undefined}
              >
                {s.label}
              </Pill>
            )
          })}
        </div>
      )}
    </div>
  )
}
