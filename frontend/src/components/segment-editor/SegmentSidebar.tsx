// frontend/src/components/segment-editor/SegmentSidebar.tsx
import { useMemo, useState, type ReactNode } from 'react'
import { Check, ChevronDown, ChevronRight, HelpCircle, Link2, Loader2, RefreshCw, X } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SURFACE_DROPDOWN_OPTIONS,
  SAC_SCALE_OPTIONS,
  MTB_SCALE_OPTIONS,
  BIDIRECTIONAL_DEFAULT,
  WINTER_SPORT_WHITELIST,
  SAC_COLOR_MAP,
  MTB_COLOR_MAP,
  deriveTrailDifficultyColor,
  type SegType,
} from './constants'
import { Pill } from './Pill'
import { sportsInCategory } from '@/lib/sportTaxonomy'
import { cn } from '@/lib/utils'

export interface SegmentMetadata {
  name: string
  segType: SegType
  sportTypes: string[] | null  // null = "Alle"
  sacScale: string | null
  mtbScale: string | null
  surface: string | null
  difficultyColor: 'blue' | 'red' | 'black' | null
  bidirectional: boolean
  userEditedBidirectional: boolean
  isPublic: boolean
}

export interface LiveInfo {
  distanceKm: number
  gainM: number
  lossM: number
  elevationMaxM?: number | null
}

export interface SegmentSidebarProps {
  mode: 'track' | 'draw' | 'lap'
  meta: SegmentMetadata
  onMetaChange: (patch: Partial<SegmentMetadata>) => void
  info: LiveInfo | null
  allSportTypes: Array<{ key: string; label: string; category: string }>
  /** Segment group of the source activity (bike/foot/ski/paddle/swim/generic).
   *  Used to auto-expand the matching sport category on mount. */
  defaultSportGroup?: string
  onEnrichmentRequest: () => void
  enrichmentState: { loading: boolean; coverage?: number; lastRequestedAt?: number }
  enrichmentSources: Partial<Record<'surface' | 'sac' | 'mtb', { coverage: number } | null>>
  onClearEnrichmentSource: (field: 'surface' | 'sac' | 'mtb') => void
  enrichmentDisabledReason?: string  // tooltip when button disabled
  enrichmentStatus?: string           // screen-reader status text (loading / success / low-coverage / no-graph)
}

// ── Segment-type labels ────────────────────────────────────────────────────────
const SEG_TYPE_LABELS: Record<SegType, string> = {
  standard: 'Standard',
  trail: 'Trail',
  lap: 'Runde',
  wintersport: 'Winter',
}

// ── Difficulty color labels (accessible, descriptive) ─────────────────────────
const DIFF_COLOR_LABELS: Record<'blue' | 'red' | 'black', string> = {
  blue:  'Blau — einfach',
  red:   'Rot — mittel',
  black: 'Schwarz — schwer',
}

// ── UI-level sport categories (full master list) ───────────────────────────────
// Maps user-visible chips to the underlying taxonomy category keys.
// "Sonstige" covers fitness/game/climb/outdoor/multi/other from the taxonomy.
// Visibility is segType-dependent — see getVisibleCategories() below.
const UI_SPORT_CATEGORIES: Array<{ key: string; label: string; taxonomyKeys: string[] }> = [
  { key: 'run',      label: 'Laufen & Wandern', taxonomyKeys: ['run'] },
  { key: 'bike',     label: 'Rad',              taxonomyKeys: ['bike'] },
  { key: 'winter',   label: 'Wintersport',      taxonomyKeys: ['winter'] },
  { key: 'water',    label: 'Wassersport',       taxonomyKeys: ['water'] },
  { key: 'sonstige', label: 'Sonstige',          taxonomyKeys: ['fitness', 'game', 'climb', 'outdoor', 'multi', 'other'] },
]

/**
 * Returns the sport-category chips that should be shown for the current segType.
 *
 * - wintersport → only the Wintersport chip (existing behaviour)
 * - standard / trail / lap → run + bike + sonstige
 *   (winter and water hidden: winter has its own segType; water gets one in a
 *   future phase — hiding now avoids user confusion and orphaned sport selections)
 */
function getVisibleCategories(segType: SegType): typeof UI_SPORT_CATEGORIES {
  if (segType === 'wintersport') {
    return UI_SPORT_CATEGORIES.filter(c => c.key === 'winter')
  }
  return UI_SPORT_CATEGORIES.filter(c => c.key !== 'winter' && c.key !== 'water')
}

/** Expand a UI category key to all individual sport-type strings from the taxonomy. */
function expandCategoryToSportTypes(uiKey: string, whitelist?: readonly string[]): string[] {
  const cat = UI_SPORT_CATEGORIES.find(c => c.key === uiKey)
  if (!cat) return []
  const types = cat.taxonomyKeys.flatMap(tk => sportsInCategory(tk).map(s => s.sportType))
  if (whitelist) return types.filter(t => whitelist.includes(t))
  return types
}

/** True if ALL sport-types for the given UI category are in `active`. */
function isCategoryFullyActive(uiKey: string, active: string[] | null, whitelist?: readonly string[]): boolean {
  if (active === null) return false // "Alle" mode
  const types = expandCategoryToSportTypes(uiKey, whitelist)
  return types.length > 0 && types.every(t => active.includes(t))
}

/** True if SOME (but not all) sport-types for the given UI category are in `active`. */
function isCategoryPartiallyActive(uiKey: string, active: string[] | null, whitelist?: readonly string[]): boolean {
  if (active === null) return false
  const types = expandCategoryToSportTypes(uiKey, whitelist)
  return types.some(t => active.includes(t)) && !types.every(t => active.includes(t))
}

export function SegmentSidebar(props: SegmentSidebarProps) {
  const {
    meta,
    onMetaChange,
    info,
    mode,
    allSportTypes,
    // defaultSportGroup: no longer used (category chips don't auto-expand)
    onEnrichmentRequest,
    enrichmentState,
    enrichmentSources,
    onClearEnrichmentSource,
    enrichmentDisabledReason,
    enrichmentStatus,
  } = props

  // Advanced "Feinwahl" expansion — which UI categories have detail chips shown.
  const [detailOpenCategories, setDetailOpenCategories] = useState<Set<string>>(new Set())

  const toggleDetailOpen = (uiKey: string) => {
    setDetailOpenCategories(prev => {
      const next = new Set(prev)
      if (next.has(uiKey)) next.delete(uiKey)
      else next.add(uiKey)
      return next
    })
  }

  const showScaleFields = meta.segType !== 'wintersport'
  // For segType ∈ {standard, trail, lap}: difficultyColor is auto-derived from
  // max(SAC, MTB) — handleSacChange/handleMtbChange keep meta.difficultyColor
  // in sync. displayedColor falls back to live derivation for existing segments
  // loaded without a pre-computed difficultyColor.
  // Bei Single-Scale-Delete (nur SAC oder nur MTB gelöscht) asymmetrisch,
  // aber okay in normalem UX-Flow — bleibt das Maximum der gesetzten Skala.
  const displayedColor = meta.segType !== 'wintersport'
    ? (meta.difficultyColor ?? deriveTrailDifficultyColor(meta.sacScale, meta.mtbScale))
    : meta.difficultyColor

  const availableSportTypes = useMemo(() => {
    if (meta.segType === 'wintersport') {
      return allSportTypes.filter(s => WINTER_SPORT_WHITELIST.includes(s.key))
    }
    return allSportTypes
  }, [meta.segType, allSportTypes])

  // Option B (§4.2 Punkt 9): for segType ∈ {standard, trail, lap}, auto-derive
  // difficultyColor from max(SAC-Farbe, MTB-Farbe) whenever either scale changes,
  // so DifficultyBadge and SegmentsOverlay always have a value to render.
  // Wintersport retains the manual picker (FIS colors, no SAC/MTB).
  const handleSacChange = (v: string) => {
    const newSac = v || null
    const patch: Partial<SegmentMetadata> = { sacScale: newSac }
    if (meta.segType !== 'wintersport') {
      patch.difficultyColor = deriveTrailDifficultyColor(newSac, meta.mtbScale)
    }
    onMetaChange(patch)
  }

  const handleMtbChange = (v: string) => {
    const newMtb = v || null
    const patch: Partial<SegmentMetadata> = { mtbScale: newMtb }
    if (meta.segType !== 'wintersport') {
      patch.difficultyColor = deriveTrailDifficultyColor(meta.sacScale, newMtb)
    }
    onMetaChange(patch)
  }

  const handleSegTypeChange = (newType: SegType) => {
    const patch: Partial<SegmentMetadata> = { segType: newType }
    // Default-flipping (§4.2 Item 10): only if user hasn't manually edited.
    if (!meta.userEditedBidirectional) {
      patch.bidirectional = BIDIRECTIONAL_DEFAULT[newType]
    }
    // Wintersport whitelist pruning: drop any now-disallowed sport types.
    if (newType === 'wintersport' && meta.sportTypes) {
      patch.sportTypes = meta.sportTypes.filter(s => WINTER_SPORT_WHITELIST.includes(s))
      if (patch.sportTypes.length === 0) patch.sportTypes = null
    }
    // Scale fields cleared when entering wintersport; difficultyColor stays
    // as the last manual value (user can change it in the wintersport picker).
    if (newType === 'wintersport') {
      patch.sacScale = null
      patch.mtbScale = null
      patch.surface = null
    }
    // Option B: when switching to standard/trail/lap, re-derive difficultyColor
    // from current SAC/MTB values so DifficultyBadge doesn't go blank.
    if (newType !== 'wintersport') {
      patch.difficultyColor = deriveTrailDifficultyColor(meta.sacScale, meta.mtbScale)
    }
    // Part C: when switching to standard/trail/lap, remove any sport types that
    // belong to the now-hidden categories (winter, water) to keep selections clean.
    if (newType !== 'wintersport' && meta.sportTypes) {
      const visibleCats = getVisibleCategories(newType)
      const allowedTypes = visibleCats.flatMap(cat =>
        cat.taxonomyKeys.flatMap(tk => sportsInCategory(tk).map(s => s.sportType))
      )
      const filtered = meta.sportTypes.filter(s => allowedTypes.includes(s))
      patch.sportTypes = filtered.length === 0 ? null : filtered
    }
    onMetaChange(patch)
  }

  const handleBidirectionalToggle = () => {
    // Flag wird bei jedem Toggle-Klick auf true gesetzt — zurückgeklickter
    // Default bleibt 'manuell'-geflagt. Phase 2: optional auto-reset wenn
    // exact auf Default zurück (z.B. User klickt wieder auf den segType-Default).
    onMetaChange({
      bidirectional: !meta.bidirectional,
      userEditedBidirectional: true,
    })
  }

  const toggleSportType = (key: string) => {
    const current = meta.sportTypes
    if (current === null) {
      // Was "Alle" — switch to a single-item list.
      onMetaChange({ sportTypes: [key] })
      return
    }
    const next = current.includes(key) ? current.filter(s => s !== key) : [...current, key]
    onMetaChange({ sportTypes: next.length === 0 ? null : next })
  }

  const setAll = () => onMetaChange({ sportTypes: null })

  const toggleUiCategory = (uiKey: string) => {
    const whitelist = meta.segType === 'wintersport' ? WINTER_SPORT_WHITELIST : undefined
    const categoryTypes = expandCategoryToSportTypes(uiKey, whitelist)
    const current = meta.sportTypes

    if (isCategoryFullyActive(uiKey, current, whitelist)) {
      // Deselect all types in this category.
      const next = current!.filter(t => !categoryTypes.includes(t))
      onMetaChange({ sportTypes: next.length === 0 ? null : next })
    } else {
      // Select all types in this category (merge with existing selections).
      const base = current ?? []
      const next = Array.from(new Set([...base, ...categoryTypes]))
      onMetaChange({ sportTypes: next })
    }
  }

  return (
    <aside className="flex h-full w-full flex-col overflow-y-auto border-r border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900">
      {/* ── Live-Info-Card ──────────────────────────────────────────────────── */}
      {info && (mode === 'track' || (mode === 'draw' && info.distanceKm > 0)) && (
        <div className="mx-3 mt-3 rounded-lg border border-neutral-200 dark:border-neutral-700/60 bg-neutral-50 dark:bg-neutral-800 p-3">
          <div className="grid grid-cols-3 gap-x-2 text-center">
            <MetricCell label="Distanz" value={`${info.distanceKm.toFixed(1)} km`} />
            <MetricCell label="Aufstieg" value={`+${Math.round(info.gainM)} m`} valueClassName="text-red-500" />
            <MetricCell label="Abstieg"  value={`−${Math.round(info.lossM)} m`} valueClassName="text-green-500" />
          </div>
          {info.elevationMaxM != null && (
            <div className="mt-2 border-t border-neutral-200 dark:border-neutral-700/60 pt-2 text-center">
              <MetricCell label="Höchstpunkt" value={`${Math.round(info.elevationMaxM)} m`} />
            </div>
          )}
        </div>
      )}

      {/* ── Scrollable form area ───────────────────────────────────────────── */}
      <div className="flex flex-col gap-4 mx-3 py-3">

        {/* ── Name ─────────────────────────────────────────────────────────── */}
        <Field label="Name" required>
          <input
            type="text"
            value={meta.name}
            onChange={e => onMetaChange({ name: e.target.value })}
            placeholder="Segmentname"
            className={cn(
              'w-full rounded-md border border-neutral-300 dark:border-neutral-700',
              'bg-white dark:bg-neutral-800 px-3 py-1.5',
              'text-sm text-neutral-900 dark:text-neutral-100',
              'placeholder:text-neutral-400 dark:placeholder:text-neutral-500',
              'transition-colors duration-150',
              'hover:border-neutral-400 dark:hover:border-neutral-600',
              'focus:outline-none focus:border-accent-DEFAULT focus:ring-1 focus:ring-accent-DEFAULT',
            )}
            required
          />
        </Field>

        {/* ── Segment-Typ ──────────────────────────────────────────────────── */}
        <Field label="Typ">
          <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Segment-Typ">
            {(['standard', 'trail', 'lap', 'wintersport'] as SegType[]).map(t => (
              <Pill
                key={t}
                role="radio"
                ariaChecked={meta.segType === t}
                active={meta.segType === t}
                onClick={() => handleSegTypeChange(t)}
              >
                {SEG_TYPE_LABELS[t]}
              </Pill>
            ))}
          </div>
        </Field>

        {/* ── Sportarten ───────────────────────────────────────────────────── */}
        <Field label="Sportarten">
          {/* Category chips row — Part C: visible categories depend on segType */}
          <div className="flex flex-wrap gap-1.5" role="group" aria-label="Sportarten-Filter">
            {/* "Alle" chip */}
            <Pill
              role="checkbox"
              ariaChecked={meta.sportTypes === null}
              active={meta.sportTypes === null}
              onClick={setAll}
              leftSlot={meta.sportTypes === null ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
            >
              Alle
            </Pill>

            {/* Category chips — getVisibleCategories filters by segType (Part C) */}
            {getVisibleCategories(meta.segType).map(cat => {
              const whitelist = meta.segType === 'wintersport' ? WINTER_SPORT_WHITELIST : undefined
              const isActive = isCategoryFullyActive(cat.key, meta.sportTypes, whitelist)
              const isPartial = isCategoryPartiallyActive(cat.key, meta.sportTypes, whitelist)
              return (
                <Pill
                  key={cat.key}
                  role="checkbox"
                  ariaChecked={isActive ? true : isPartial ? 'mixed' : false}
                  active={isActive}
                  onClick={() => toggleUiCategory(cat.key)}
                  leftSlot={isActive ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
                  className={isPartial && !isActive
                    ? 'border-accent-DEFAULT/40 bg-accent-DEFAULT/10 text-accent-700 dark:text-accent-300'
                    : undefined}
                >
                  {cat.label}
                </Pill>
              )
            })}
          </div>

          {/* Feinwahl: per category, show individual sport chips on expand.
              Chevron is always clickable — user can add fine-grained picks
              without first activating the entire category (issue #19). */}
          {meta.sportTypes !== null && meta.segType !== 'wintersport' && (
            <div className="mt-2 flex flex-col gap-1.5">
              {getVisibleCategories(meta.segType)
                .map(cat => {
                  const isOpen = detailOpenCategories.has(cat.key)
                  const catSports = cat.taxonomyKeys
                    .flatMap(tk => sportsInCategory(tk))
                    .filter(s => availableSportTypes.some(a => a.key === s.sportType))
                  if (catSports.length === 0) return null
                  return (
                    <div key={cat.key}>
                      <button
                        type="button"
                        onClick={() => toggleDetailOpen(cat.key)}
                        className="flex items-center gap-1 mb-1 group"
                      >
                        {isOpen
                          ? <ChevronDown className="h-3 w-3 text-neutral-400 dark:text-neutral-500 group-hover:text-neutral-600 dark:group-hover:text-neutral-300" />
                          : <ChevronRight className="h-3 w-3 text-neutral-400 dark:text-neutral-500 group-hover:text-neutral-600 dark:group-hover:text-neutral-300" />
                        }
                        <span className="text-[10px] font-semibold tracking-wider uppercase text-neutral-400 dark:text-neutral-500 group-hover:text-neutral-600 dark:group-hover:text-neutral-300">
                          {cat.label} — Feinwahl
                        </span>
                      </button>
                      {isOpen && (
                        <div className="flex flex-wrap gap-1 pl-3">
                          {catSports.map(s => {
                            const isActive = meta.sportTypes?.includes(s.sportType) ?? false
                            return (
                              <Pill
                                key={s.sportType}
                                size="sm"
                                role="checkbox"
                                ariaChecked={isActive}
                                active={isActive}
                                onClick={() => toggleSportType(s.sportType)}
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
                })
              }
            </div>
          )}

          {/* wintersport: individual chips for the whitelisted winter sports */}
          {meta.segType === 'wintersport' && meta.sportTypes !== null && (
            <div className="mt-2 flex flex-wrap gap-1">
              {availableSportTypes.map(s => {
                const isActive = meta.sportTypes?.includes(s.key) ?? false
                return (
                  <Pill
                    key={s.key}
                    size="sm"
                    role="checkbox"
                    ariaChecked={isActive}
                    active={isActive}
                    onClick={() => toggleSportType(s.key)}
                    leftSlot={isActive ? <Check className="h-3 w-3 shrink-0" /> : undefined}
                  >
                    {s.label}
                  </Pill>
                )
              })}
            </div>
          )}
        </Field>

        {/* ── Schwierigkeit & Oberfläche ───────────────────────────────────── */}
        {/* Section header with OSM-Vorschlag button on the right */}
        <div>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
              Schwierigkeit &amp; Oberfläche
            </span>
            <button
              type="button"
              onClick={onEnrichmentRequest}
              disabled={!!enrichmentDisabledReason || enrichmentState.loading}
              title={enrichmentDisabledReason ?? 'Surface, SAC- und MTB-Skala aus OSM-Wegedaten vorschlagen'}
              aria-pressed={!!enrichmentSources.surface || !!enrichmentSources.sac || !!enrichmentSources.mtb}
              className={cn(
                'flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium',
                'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-300',
                'border border-neutral-200 dark:border-neutral-700',
                'transition-colors duration-150',
                'hover:bg-accent-DEFAULT/10 hover:text-accent-DEFAULT hover:border-accent-DEFAULT/30',
                'dark:hover:bg-accent-DEFAULT/10 dark:hover:text-accent-400 dark:hover:border-accent-DEFAULT/30',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1 focus-visible:ring-offset-white dark:focus-visible:ring-offset-neutral-900',
                'disabled:cursor-not-allowed disabled:opacity-40',
              )}
            >
              {enrichmentState.loading
                ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                : <RefreshCw className="h-3.5 w-3.5" />}
              <span className="sr-only">OSM-Vorschlag</span>
            </button>
          </div>

          {showScaleFields ? (
            <div className="flex flex-col gap-2">
              {/* SAC + MTB side-by-side */}
              <div className="grid grid-cols-2 gap-2">
                {/* SAC */}
                <div className="flex flex-col gap-1">
                  <div className="flex items-center gap-1">
                    <span className="text-[10px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">SAC</span>
                    <span
                      tabIndex={0}
                      aria-label="SAC-Wanderskala — offizielle Schweizer Bergweg-Klassifikation (T1–T6). Skadia nutzt die DAV-Bayern-Auslegung."
                      className="cursor-help text-neutral-400 dark:text-neutral-500 hover:text-neutral-600 dark:hover:text-neutral-300 focus-visible:outline-none"
                    >
                      <HelpCircle className="h-3 w-3" aria-hidden="true" />
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <StyledSelect
                      value={meta.sacScale ?? ''}
                      placeholder="—"
                      options={SAC_SCALE_OPTIONS.map(o => ({ value: o.value, label: o.label }))}
                      onChange={handleSacChange}
                    />
                    <ColorSwatch color={meta.sacScale ? SAC_COLOR_MAP[meta.sacScale] : null} />
                    <EnrichmentBadge
                      source={enrichmentSources.sac}
                      onClear={() => onClearEnrichmentSource('sac')}
                    />
                  </div>
                </div>

                {/* MTB */}
                <div className="flex flex-col gap-1">
                  <div className="flex items-center gap-1">
                    <span className="text-[10px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">MTB</span>
                    <span
                      tabIndex={0}
                      aria-label="Singletrail-Skala — offizielle MTB-Technikstufen nach singletrail-skala.de (S0–S5)."
                      className="cursor-help text-neutral-400 dark:text-neutral-500 hover:text-neutral-600 dark:hover:text-neutral-300 focus-visible:outline-none"
                    >
                      <HelpCircle className="h-3 w-3" aria-hidden="true" />
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <StyledSelect
                      value={meta.mtbScale ?? ''}
                      placeholder="—"
                      options={MTB_SCALE_OPTIONS.map(o => ({ value: o.value, label: o.label }))}
                      onChange={handleMtbChange}
                    />
                    <ColorSwatch color={meta.mtbScale ? MTB_COLOR_MAP[meta.mtbScale] : null} />
                    <EnrichmentBadge
                      source={enrichmentSources.mtb}
                      onClear={() => onClearEnrichmentSource('mtb')}
                    />
                  </div>
                </div>
              </div>

              {/* Oberfläche full-width */}
              <div className="flex flex-col gap-1">
                <span className="text-[10px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">Oberfläche</span>
                <div className="flex items-center gap-1.5">
                  <StyledSelect
                    value={meta.surface ?? ''}
                    placeholder="—"
                    options={SURFACE_DROPDOWN_OPTIONS.map(s => ({ value: s, label: s }))}
                    onChange={v => onMetaChange({ surface: v || null })}
                  />
                  <EnrichmentBadge
                    source={enrichmentSources.surface}
                    onClear={() => onClearEnrichmentSource('surface')}
                  />
                </div>
              </div>

              {/* Schwierigkeitsfarbe — nur bei Trail als read-only-Display
                  (abgeleitet aus max(SAC, MTB); Spec §4.6).
                  Für Standard/Lap entfernt — user-feedback: "macht keinen sinn
                  für segmente für alle sportarten", SAC + MTB haben eigene Farben.
                  Wintersport nutzt separaten Zweig unten (FIS manuell). */}
              {meta.segType === 'trail' && (
                <div className="flex flex-col gap-1">
                  <div className="flex items-center gap-1.5">
                    <span className="text-[10px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
                      Schwierigkeit
                    </span>
                    <span
                      title="Abgeleitet aus max(SAC, MTB)"
                      className="text-neutral-400 dark:text-neutral-500"
                    >
                      <Link2 className="h-3.5 w-3.5" aria-hidden="true" />
                      <span className="sr-only">Abgeleitet aus SAC/MTB</span>
                    </span>
                  </div>
                  {displayedColor ? (
                    <div className="inline-flex items-center gap-2 self-start rounded-lg border border-neutral-300 dark:border-neutral-700 px-3 py-1.5 text-sm">
                      <DifficultyDot color={displayedColor} />
                      <span>{DIFF_COLOR_LABELS[displayedColor]}</span>
                    </div>
                  ) : (
                    <span className="text-xs italic text-neutral-400 dark:text-neutral-500">
                      Wähle SAC- oder MTB-Skala
                    </span>
                  )}
                </div>
              )}
            </div>
          ) : (
            /* wintersport: no scale fields, just difficulty */
            // ARIA note: using role="radiogroup" + role="radio" with a "—" (keine)
            // pill so exactly one option is always selected (proper radio semantics).
            // Previously 3 pills toggled off on re-click (checkbox semantics) which
            // is ARIA-misuse for role="radio". The 4th "—" pill acts as the "none"
            // option, keeping the group mutually exclusive.
            <div className="flex flex-col gap-1">
              <span className="text-[10px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">Schwierigkeit</span>
              <div className="flex flex-wrap items-center gap-1.5" role="radiogroup" aria-label="Schwierigkeit">
                {/* "—" = keine Schwierigkeit (radio-group braucht immer eine gewählte Option) */}
                <Pill
                  role="radio"
                  ariaChecked={displayedColor === null}
                  ariaLabel="Keine Schwierigkeit"
                  active={displayedColor === null}
                  onClick={() => onMetaChange({ difficultyColor: null })}
                >
                  —
                </Pill>
                {(['blue', 'red', 'black'] as const).map(c => (
                  <Pill
                    key={c}
                    role="radio"
                    ariaChecked={displayedColor === c}
                    ariaLabel={DIFF_COLOR_LABELS[c]}
                    active={displayedColor === c}
                    disabled={false}
                    onClick={() => onMetaChange({ difficultyColor: c })}
                    leftSlot={<DifficultyDot color={c} />}
                  >
                    {DIFF_COLOR_SHORT[c]}
                  </Pill>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* ── Bidirektional ────────────────────────────────────────────────── */}
        <Field label="Richtung">
          <ToggleRow
            checked={meta.bidirectional}
            onChange={handleBidirectionalToggle}
            label="In beide Richtungen matchen"
          />
        </Field>

        {/* ── Sichtbarkeit ─────────────────────────────────────────────────── */}
        <Field label="Sichtbarkeit">
          <ToggleRow
            checked={meta.isPublic}
            onChange={e => onMetaChange({ isPublic: e.target.checked })}
            label="Öffentlich sichtbar"
          />
        </Field>
      </div>

      {/* Live-region for screen-reader enrichment announcements (§4.14) */}
      <div aria-live="polite" aria-atomic="true" className="sr-only">
        {enrichmentStatus ?? (enrichmentState.loading ? 'Lade OSM-Daten...' : '')}
      </div>
    </aside>
  )
}

// ── Sub-components ─────────────────────────────────────────────────────────────

function MetricCell({ label, value, valueClassName }: { label: string; value: string; valueClassName?: string }) {
  return (
    <div>
      <div className="text-[10px] font-medium tracking-wide text-neutral-500 dark:text-neutral-400 uppercase">{label}</div>
      <div className={cn('mt-0.5 text-sm font-semibold', valueClassName ?? 'text-neutral-900 dark:text-neutral-100')}>{value}</div>
    </div>
  )
}

function Field({
  label,
  children,
  tooltip,
  required,
}: {
  label: string
  children: ReactNode
  tooltip?: string
  required?: boolean
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-1.5 text-[11px] font-semibold tracking-wider text-neutral-500 dark:text-neutral-400 uppercase">
        <span>{label}</span>
        {required && <span className="text-red-500" aria-hidden="true">*</span>}
        {tooltip && (
          <span
            tabIndex={0}
            aria-label={tooltip}
            className={cn(
              'cursor-help text-neutral-400 dark:text-neutral-500',
              'transition-colors hover:text-neutral-600 dark:hover:text-neutral-300',
              'focus-visible:outline-none focus-visible:text-neutral-600 dark:focus-visible:text-neutral-300',
            )}
          >
            <HelpCircle className="h-3 w-3" aria-hidden="true" />
          </span>
        )}
      </div>
      {children}
    </div>
  )
}

// Short display labels for difficulty color pill buttons.
const DIFF_COLOR_SHORT: Record<'blue' | 'red' | 'black', string> = {
  blue:  'Blau',
  red:   'Rot',
  black: 'Schwarz',
}

/**
 * Coloured dot used as the leftSlot in difficulty Pill buttons.
 * The dot communicates which difficulty colour is which; the Pill's own
 * active state always uses the canonical orange accent (not blue/red/black)
 * so all three pill types look visually cohesive.
 */
function DifficultyDot({ color }: { color: 'blue' | 'red' | 'black' }) {
  const cls = color === 'blue'
    ? 'bg-blue-500'
    : color === 'red'
      ? 'bg-red-600'
      : 'bg-neutral-900 dark:bg-neutral-100 border border-neutral-500 dark:border-neutral-400'
  return <span className={cn('inline-block h-3 w-3 rounded-full', cls)} aria-hidden="true" />
}

function ToggleRow({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: React.ChangeEventHandler<HTMLInputElement>
  label: string
}) {
  return (
    <label className="flex cursor-pointer items-center gap-2.5 text-sm text-neutral-700 dark:text-neutral-300 select-none hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors duration-150">
      <input
        type="checkbox"
        checked={checked}
        onChange={onChange}
        className={cn(
          'h-4 w-4 rounded border-neutral-400 dark:border-neutral-600',
          'bg-white dark:bg-neutral-800',
          'accent-[#F97316]',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-1 focus-visible:ring-offset-white dark:focus-visible:ring-offset-neutral-900',
        )}
      />
      {label}
    </label>
  )
}

function ColorSwatch({ color }: { color: 'blue' | 'red' | 'black' | null | undefined }) {
  if (!color) {
    return <span className="inline-block h-6 w-6 flex-shrink-0 rounded border border-neutral-300 dark:border-neutral-700 bg-neutral-100 dark:bg-neutral-800" />
  }
  return (
    <span
      aria-hidden="true"
      className={cn(
        'inline-block h-6 w-6 flex-shrink-0 rounded',
        color === 'blue'  && 'bg-blue-500 border border-blue-600 dark:border-blue-400',
        color === 'red'   && 'bg-red-600 border border-red-700 dark:border-red-400',
        color === 'black' && 'bg-neutral-900 dark:bg-neutral-950 border border-neutral-400 dark:border-neutral-600',
      )}
    />
  )
}

/**
 * Dropdown-based styled select using shadcn DropdownMenu.
 * Replaces native <select> elements for consistent dark-mode styling.
 */
function StyledSelect({
  value,
  placeholder,
  options,
  onChange,
}: {
  value: string
  placeholder: string
  options: ReadonlyArray<{ value: string; label: string }>
  onChange: (v: string) => void
}) {
  // If the stored value is non-empty but not in the current option list
  // (e.g. legacy surface values like 'Beton', 'Naturweg (fest)', 'Holz', 'Metall'
  // that were removed in §4.8 v2), show it as "(veraltet) <value>" so users
  // can see what they have and pick a replacement — no silent data loss.
  const isLegacy = value !== '' && !options.some(o => o.value === value)
  const selectedLabel = isLegacy
    ? `(veraltet) ${value}`
    : (options.find(o => o.value === value)?.label ?? placeholder)

  // Build effective options: for legacy values prepend a "(veraltet)" entry so
  // the dropdown trigger text and the rendered list are consistent.
  const effectiveOptions: ReadonlyArray<{ value: string; label: string }> = isLegacy
    ? [{ value, label: `(veraltet) ${value}` }, ...options]
    : options

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={cn(
            'flex flex-1 min-w-0 items-center justify-between gap-1.5',
            'rounded-md border border-neutral-300 dark:border-neutral-700',
            'bg-white dark:bg-neutral-800 px-2.5 py-1.5',
            'text-sm text-neutral-900 dark:text-neutral-100',
            'transition-colors duration-150',
            'hover:border-neutral-400 dark:hover:border-neutral-600 hover:bg-neutral-50 dark:hover:bg-neutral-700',
            'focus:outline-none focus-visible:border-accent-DEFAULT focus-visible:ring-1 focus-visible:ring-accent-DEFAULT',
            isLegacy && 'border-amber-400 dark:border-amber-600 text-amber-700 dark:text-amber-400',
          )}
        >
          <span className="truncate">{selectedLabel}</span>
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-neutral-400 dark:text-neutral-500" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="z-[300] max-h-60 overflow-y-auto">
        <DropdownMenuItem onClick={() => onChange('')}>
          {placeholder}
        </DropdownMenuItem>
        {effectiveOptions.map(o => (
          <DropdownMenuItem
            key={o.value}
            onClick={() => onChange(o.value === value && isLegacy ? value : o.value)}
            className={o.value === value ? 'font-medium text-accent-DEFAULT dark:text-accent-400' : ''}
          >
            {o.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function EnrichmentBadge({
  source,
  onClear,
}: {
  source?: { coverage: number } | null
  onClear: () => void
}) {
  if (!source) return null
  const pct = Math.round(source.coverage * 100)
  return (
    <span className="flex flex-shrink-0 items-center gap-1 rounded-md border border-accent-DEFAULT/30 bg-accent-DEFAULT/10 px-1.5 py-0.5 text-[10px] font-medium text-accent-DEFAULT dark:text-accent-400">
      <Check className="h-2.5 w-2.5 flex-shrink-0" aria-hidden="true" />
      {pct === 100 ? '100%' : `${pct}%`}
      <button
        type="button"
        onClick={onClear}
        aria-label="OSM-Quelle entfernen"
        className={cn(
          'flex h-3.5 w-3.5 flex-shrink-0 items-center justify-center rounded',
          'text-accent-DEFAULT dark:text-accent-400 transition-colors duration-150',
          'hover:bg-accent-DEFAULT/20 hover:text-accent-600',
          'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent-DEFAULT',
        )}
      >
        <X className="h-2.5 w-2.5" />
      </button>
    </span>
  )
}
