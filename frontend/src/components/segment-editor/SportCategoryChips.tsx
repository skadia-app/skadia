import { useState } from 'react'
import { Check, ChevronDown, ChevronRight, X } from 'lucide-react'
import { Pill } from '../ui/Pill'
import { sportsInCategory, getSegmentGroup } from '@/lib/sportTaxonomy'
import { WINTER_SPORT_WHITELIST, WATER_SPORT_WHITELIST, type SegType } from './constants'
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

// ── Category definitions ───────────────────────────────────────────────────────

export const UI_SPORT_CATEGORIES = [
  { key: 'run',          label: 'Laufen & Wandern', taxonomyKeys: ['run'] },
  { key: 'bike',         label: 'Rad',              taxonomyKeys: ['bike'] },
  { key: 'winter',       label: 'Wintersport',      taxonomyKeys: ['winter'] },
  { key: 'water_swim',   label: 'Schwimmen',        taxonomyKeys: ['water'], segmentGroups: ['swim'] },
  { key: 'water_paddle', label: 'Paddeln',          taxonomyKeys: ['water'], segmentGroups: ['paddle'] },
  { key: 'water_surf',   label: 'Surfen & Segeln',  taxonomyKeys: ['water'], segmentGroups: ['generic'] },
  { key: 'sonstige',     label: 'Sonstige',         taxonomyKeys: ['fitness', 'game', 'climb', 'outdoor', 'multi', 'other'] },
]

// ── Helper exports (used by SegmentSidebar, SegmentEditModal) ─────────────────

export function getVisibleCategories(segType: SegType) {
  if (segType === 'wintersport') return UI_SPORT_CATEGORIES.filter(c => c.key === 'winter')
  if (segType === 'wassersport') return UI_SPORT_CATEGORIES.filter(c => c.key.startsWith('water_'))
  return UI_SPORT_CATEGORIES.filter(c => !c.key.startsWith('water_') && c.key !== 'winter')
}

export function expandCategoryToSportTypes(uiKey: string, whitelist?: readonly string[]): string[] {
  const cat = UI_SPORT_CATEGORIES.find(c => c.key === uiKey)
  if (!cat) return []
  let types = cat.taxonomyKeys.flatMap(tk => sportsInCategory(tk).map(s => s.sportType))
  if (cat.segmentGroups) {
    types = types.filter(t => (cat.segmentGroups as string[]).includes(getSegmentGroup(t)))
  }
  if (whitelist) {
    types = types.filter(t => whitelist.includes(t))
  }
  return types
}

export function isCategoryFullyActive(uiKey: string, active: string[] | null, whitelist?: readonly string[]): boolean {
  if (active === null) return false
  const types = expandCategoryToSportTypes(uiKey, whitelist)
  return types.length > 0 && types.every(t => active.includes(t))
}

export function isCategoryPartiallyActive(uiKey: string, active: string[] | null, whitelist?: readonly string[]): boolean {
  if (active === null) return false
  const types = expandCategoryToSportTypes(uiKey, whitelist)
  return types.some(t => active.includes(t)) && !types.every(t => active.includes(t))
}

// ── Internal DetailPanel ───────────────────────────────────────────────────────

interface DetailPanelProps {
  /** The UI category being expanded */
  category: typeof UI_SPORT_CATEGORIES[number]
  /** All sport types available for this segment type (for filtering) */
  availableSportTypes: Array<{ key: string; label: string; category: string }>
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
      {catSports.map(s => {
        const isActive = selectedSportTypes?.includes(s.sportType) ?? false
        const badges = enrichmentSources?.[s.sportType]

        return (
          <div key={s.sportType} className="flex items-center gap-0.5">
            <Pill
              size="sm"
              role="checkbox"
              ariaChecked={isActive}
              active={isActive}
              onClick={() => onToggleSportType(s.sportType)}
              leftSlot={isActive ? <Check className="h-3 w-3 shrink-0" /> : undefined}
            >
              {s.label}
            </Pill>

            {/* Enrichment source badges — shown only when enrichmentSources provided */}
            {badges && badges.length > 0 && onClearEnrichmentSource && (
              <div className="flex gap-0.5">
                {badges.map((src, i) => (
                  <button
                    key={i}
                    type="button"
                    onClick={() => onClearEnrichmentSource(s.sportType, src)}
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
  const whitelist = segType === 'wintersport' ? WINTER_SPORT_WHITELIST : (segType === 'wassersport' ? WATER_SPORT_WHITELIST : undefined)

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

  return (
    <div className="flex flex-col gap-2">
      {/* ── Category chip row ── */}
      <div className="flex flex-wrap gap-1.5" role="group" aria-label="Sportarten-Filter">
        <Pill
          role="checkbox"
          ariaChecked={activeTypes === null}
          active={activeTypes === null}
          onClick={onSetAll}
          leftSlot={activeTypes === null ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
        >
          Alle
        </Pill>

        {visible.map(cat => {
          const isActive = isCategoryFullyActive(cat.key, activeTypes, whitelist)
          const isPartial = isCategoryPartiallyActive(cat.key, activeTypes, whitelist)

          if (showDetailPicker) {
            // In detail-picker mode: chip click = open/close detail panel
            // The chip itself still shows active/partial state visually.
            const isOpen = openCategoryKey === cat.key
            return (
              <Pill
                key={cat.key}
                role="checkbox"
                ariaChecked={isActive ? true : (isPartial ? 'mixed' : false)}
                active={isActive || isOpen}
                onClick={() => toggleDetail(cat.key)}
                leftSlot={isActive ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
                className={cn(
                  isPartial && !isActive ? 'border-accent-DEFAULT/40 bg-accent-DEFAULT/10' : undefined,
                  isOpen && !isActive ? 'border-neutral-400 dark:border-neutral-500' : undefined,
                )}
              >
                {cat.label}
                {isOpen
                  ? <ChevronDown className="ml-0.5 h-3.5 w-3.5 shrink-0 opacity-60" />
                  : <ChevronRight className="ml-0.5 h-3.5 w-3.5 shrink-0 opacity-40" />
                }
              </Pill>
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
      {showDetailPicker && activeTypes !== null && segType !== 'wintersport' && segType !== 'wassersport' && openCategoryKey && (
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
