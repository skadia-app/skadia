// frontend/src/components/segment-editor/SegmentSidebar.tsx
import { useMemo, type ReactNode } from 'react'
import { Check, ChevronDown, HelpCircle, Link2, Loader2, RefreshCw, X } from 'lucide-react'
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
  wintersport: 'Wintersport',
}

// ── Difficulty color labels (accessible, descriptive) ─────────────────────────
const DIFF_COLOR_LABELS: Record<'blue' | 'red' | 'black', string> = {
  blue:  'Blau — einfach',
  red:   'Rot — mittel',
  black: 'Schwarz — schwer',
}

export function SegmentSidebar(props: SegmentSidebarProps) {
  const {
    meta,
    onMetaChange,
    info,
    mode,
    allSportTypes,
    onEnrichmentRequest,
    enrichmentState,
    enrichmentSources,
    onClearEnrichmentSource,
    enrichmentDisabledReason,
    enrichmentStatus,
  } = props

  const showScaleFields = meta.segType !== 'wintersport'
  const difficultyColorReadonly = meta.segType === 'trail'
  const displayedColor = difficultyColorReadonly
    ? deriveTrailDifficultyColor(meta.sacScale, meta.mtbScale)
    : meta.difficultyColor

  const availableSportTypes = useMemo(() => {
    if (meta.segType === 'wintersport') {
      return allSportTypes.filter(s => WINTER_SPORT_WHITELIST.includes(s.key))
    }
    return allSportTypes
  }, [meta.segType, allSportTypes])

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
    // Scale fields cleared when entering wintersport.
    if (newType === 'wintersport') {
      patch.sacScale = null
      patch.mtbScale = null
      patch.surface = null
    }
    onMetaChange(patch)
  }

  const handleBidirectionalToggle = () => {
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

  return (
    <aside className="w-[260px] shrink-0 flex flex-col border-r border-neutral-800 bg-neutral-900 overflow-y-auto">
      {/* ── Live-Info-Card ──────────────────────────────────────────────────── */}
      {info && (mode === 'track' || (mode === 'draw' && info.distanceKm > 0)) && (
        <div className="mx-3 mt-3 rounded-lg border border-neutral-700/60 bg-neutral-800 p-3">
          <div className="grid grid-cols-3 gap-x-2 text-center">
            <MetricCell label="Distanz" value={`${info.distanceKm.toFixed(1)} km`} />
            <MetricCell label="Aufstieg" value={`+${Math.round(info.gainM)} m`} />
            <MetricCell label="Abstieg"  value={`−${Math.round(info.lossM)} m`} />
          </div>
          {info.elevationMaxM != null && (
            <div className="mt-2 border-t border-neutral-700/60 pt-2 text-center">
              <MetricCell label="Höchstpunkt" value={`${Math.round(info.elevationMaxM)} m`} />
            </div>
          )}
        </div>
      )}

      {/* ── Scrollable form area ───────────────────────────────────────────── */}
      <div className="flex flex-col gap-4 p-3">
        {/* Enrichment button */}
        <button
          type="button"
          onClick={onEnrichmentRequest}
          disabled={!!enrichmentDisabledReason || enrichmentState.loading}
          title={enrichmentDisabledReason}
          aria-pressed={!!enrichmentSources.surface || !!enrichmentSources.sac || !!enrichmentSources.mtb}
          className={cn(
            'flex w-full items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium',
            'bg-blue-600 text-white',
            'transition-colors duration-150',
            'hover:bg-blue-500',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 focus-visible:ring-offset-2 focus-visible:ring-offset-neutral-900',
            'disabled:cursor-not-allowed disabled:opacity-40',
          )}
        >
          {enrichmentState.loading
            ? <Loader2 className="h-4 w-4 animate-spin" />
            : <RefreshCw className="h-4 w-4" />}
          Aus OSM-Daten holen
        </button>

        {/* ── Name ─────────────────────────────────────────────────────────── */}
        <Field label="Name" required>
          <input
            type="text"
            value={meta.name}
            onChange={e => onMetaChange({ name: e.target.value })}
            placeholder="Segmentname"
            className={cn(
              'w-full rounded-md border border-neutral-700 bg-neutral-800 px-3 py-1.5',
              'text-sm text-neutral-100 placeholder:text-neutral-500',
              'transition-colors duration-150',
              'hover:border-neutral-600',
              'focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500',
            )}
            required
          />
        </Field>

        {/* ── Segment-Typ ──────────────────────────────────────────────────── */}
        <Field label="Typ">
          <div className="grid grid-cols-2 gap-1.5">
            {(['standard', 'trail', 'lap', 'wintersport'] as SegType[]).map(t => (
              <button
                key={t}
                type="button"
                role="radio"
                aria-checked={meta.segType === t}
                onClick={() => handleSegTypeChange(t)}
                className={cn(
                  'rounded-md px-2.5 py-1.5 text-xs font-medium text-left',
                  'transition-colors duration-150',
                  'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 focus-visible:ring-offset-1 focus-visible:ring-offset-neutral-900',
                  meta.segType === t
                    ? 'bg-blue-600 text-white'
                    : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700 hover:text-neutral-100',
                )}
              >
                {SEG_TYPE_LABELS[t]}
              </button>
            ))}
          </div>
        </Field>

        {/* ── Sportarten ───────────────────────────────────────────────────── */}
        <Field label="Sportarten">
          <div className="flex flex-wrap gap-1.5">
            <Chip active={meta.sportTypes === null} onClick={setAll}>
              {meta.sportTypes === null && <Check className="h-3 w-3 shrink-0" />}
              Alle
            </Chip>
            {availableSportTypes.map(s => (
              <Chip
                key={s.key}
                active={meta.sportTypes?.includes(s.key) ?? false}
                onClick={() => toggleSportType(s.key)}
              >
                {meta.sportTypes?.includes(s.key) && <Check className="h-3 w-3 shrink-0" />}
                {s.label}
              </Chip>
            ))}
          </div>
        </Field>

        {/* ── SAC-Skala ────────────────────────────────────────────────────── */}
        {showScaleFields && (
          <Field
            label="SAC-Skala"
            tooltip="SAC-Wanderskala — offizielle Schweizer Bergweg-Klassifikation (T1–T6). Skadia nutzt die DAV-Bayern-Auslegung: Ski-Pisten-Konvention (blau=einfach, rot=mittel, schwarz=schwer)."
          >
            <div className="flex items-center gap-2">
              <StyledSelect
                value={meta.sacScale ?? ''}
                placeholder="—"
                options={SAC_SCALE_OPTIONS.map(o => ({ value: o.value, label: o.label }))}
                onChange={v => onMetaChange({ sacScale: v || null })}
              />
              <ColorSwatch color={meta.sacScale ? SAC_COLOR_MAP[meta.sacScale] : null} />
              <EnrichmentBadge
                source={enrichmentSources.sac}
                onClear={() => onClearEnrichmentSource('sac')}
              />
            </div>
          </Field>
        )}

        {/* ── MTB-Skala ────────────────────────────────────────────────────── */}
        {showScaleFields && (
          <Field
            label="MTB-Skala"
            tooltip="Singletrail-Skala — offizielle MTB-Technikstufen nach singletrail-skala.de (S0–S5)."
          >
            <div className="flex items-center gap-2">
              <StyledSelect
                value={meta.mtbScale ?? ''}
                placeholder="—"
                options={MTB_SCALE_OPTIONS.map(o => ({ value: o.value, label: o.label }))}
                onChange={v => onMetaChange({ mtbScale: v || null })}
              />
              <ColorSwatch color={meta.mtbScale ? MTB_COLOR_MAP[meta.mtbScale] : null} />
              <EnrichmentBadge
                source={enrichmentSources.mtb}
                onClear={() => onClearEnrichmentSource('mtb')}
              />
            </div>
          </Field>
        )}

        {/* ── Oberfläche ───────────────────────────────────────────────────── */}
        {showScaleFields && (
          <Field label="Oberfläche">
            <div className="flex items-center gap-2">
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
          </Field>
        )}

        {/* ── Schwierigkeitsfarbe ──────────────────────────────────────────── */}
        <Field label="Schwierigkeitsfarbe">
          <div className="flex items-center gap-2">
            {(['blue', 'red', 'black'] as const).map(c => (
              <DifficultyButton
                key={c}
                color={c}
                active={displayedColor === c}
                disabled={difficultyColorReadonly}
                onClick={() => onMetaChange({ difficultyColor: c })}
              />
            ))}
            {difficultyColorReadonly && (
              <span
                title="Farbe wird aus SAC/MTB-Skala abgeleitet"
                className="text-neutral-500 transition-colors hover:text-neutral-300"
              >
                <Link2 className="h-3.5 w-3.5" aria-hidden="true" />
                <span className="sr-only">Farbe wird aus SAC/MTB-Skala abgeleitet</span>
              </span>
            )}
            {!difficultyColorReadonly && displayedColor && (
              <button
                type="button"
                onClick={() => onMetaChange({ difficultyColor: null })}
                aria-label="Farbe zurücksetzen"
                className={cn(
                  'flex h-5 w-5 items-center justify-center rounded',
                  'text-neutral-500 transition-colors duration-150',
                  'hover:bg-neutral-700 hover:text-neutral-200',
                  'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-blue-400',
                )}
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
        </Field>

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

function MetricCell({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[10px] font-medium tracking-wide text-neutral-500 uppercase">{label}</div>
      <div className="mt-0.5 text-sm font-semibold text-neutral-100">{value}</div>
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
      <div className="flex items-center gap-1.5 text-[11px] font-semibold tracking-wider text-neutral-400 uppercase">
        <span>{label}</span>
        {required && <span className="text-red-400" aria-hidden="true">*</span>}
        {tooltip && (
          <span
            tabIndex={0}
            aria-label={tooltip}
            className={cn(
              'cursor-help text-neutral-500',
              'transition-colors hover:text-neutral-300',
              'focus-visible:outline-none focus-visible:text-neutral-300',
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

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium',
        'transition-colors duration-150',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 focus-visible:ring-offset-1 focus-visible:ring-offset-neutral-900',
        active
          ? 'bg-blue-600 text-white'
          : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700 hover:text-neutral-100',
      )}
    >
      {children}
    </button>
  )
}

function DifficultyButton({
  color,
  active,
  disabled,
  onClick,
}: {
  color: 'blue' | 'red' | 'black'
  active: boolean
  disabled: boolean
  onClick: () => void
}) {
  const bgClass = color === 'blue'
    ? 'bg-blue-500'
    : color === 'red'
      ? 'bg-red-600'
      : 'bg-neutral-900 border border-neutral-600'

  return (
    <button
      type="button"
      aria-label={DIFF_COLOR_LABELS[color]}
      aria-pressed={active}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'relative h-6 w-6 rounded transition-all duration-150',
        bgClass,
        active && 'ring-2 ring-yellow-400 ring-offset-1 ring-offset-neutral-900',
        !disabled && !active && 'hover:opacity-80 hover:scale-105',
        disabled && 'cursor-not-allowed opacity-60',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-yellow-400 focus-visible:ring-offset-1 focus-visible:ring-offset-neutral-900',
      )}
    />
  )
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
    <label className="flex cursor-pointer items-center gap-2.5 text-sm text-neutral-300 select-none hover:text-neutral-100 transition-colors duration-150">
      <input
        type="checkbox"
        checked={checked}
        onChange={onChange}
        className={cn(
          'h-4 w-4 rounded border-neutral-600 bg-neutral-800',
          'accent-blue-500',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 focus-visible:ring-offset-1 focus-visible:ring-offset-neutral-900',
        )}
      />
      {label}
    </label>
  )
}

function ColorSwatch({ color }: { color: 'blue' | 'red' | 'black' | null | undefined }) {
  if (!color) {
    return <span className="inline-block h-4 w-4 flex-shrink-0 rounded border border-neutral-700 bg-neutral-800" />
  }
  return (
    <span
      aria-hidden="true"
      className={cn(
        'inline-block h-4 w-4 flex-shrink-0 rounded',
        color === 'blue'  && 'bg-blue-500',
        color === 'red'   && 'bg-red-600',
        color === 'black' && 'bg-neutral-950 border border-neutral-600',
      )}
    />
  )
}

/**
 * Dropdown-based styled select using shadcn DropdownMenu.
 * Replaces native <select> elements for consistent dark-mode styling.
 * Note: shadcn <Select> (Radix Select primitive) is not installed in this project,
 * so we use the available DropdownMenu primitive instead.
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
  const selectedLabel = options.find(o => o.value === value)?.label ?? placeholder
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={cn(
            'flex flex-1 min-w-0 items-center justify-between gap-1.5',
            'rounded-md border border-neutral-700 bg-neutral-800 px-2.5 py-1.5',
            'text-sm text-neutral-100',
            'transition-colors duration-150',
            'hover:border-neutral-600 hover:bg-neutral-700',
            'focus:outline-none focus-visible:border-blue-500 focus-visible:ring-1 focus-visible:ring-blue-500',
          )}
        >
          <span className="truncate">{selectedLabel}</span>
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-neutral-400" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-60 overflow-y-auto">
        <DropdownMenuItem onClick={() => onChange('')}>
          {placeholder}
        </DropdownMenuItem>
        {options.map(o => (
          <DropdownMenuItem
            key={o.value}
            onClick={() => onChange(o.value)}
            className={o.value === value ? 'font-medium text-blue-400 dark:text-blue-400' : ''}
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
    <span className="flex flex-shrink-0 items-center gap-1 rounded-md border border-blue-700/50 bg-blue-900/30 px-1.5 py-0.5 text-[10px] font-medium text-blue-300">
      <Check className="h-2.5 w-2.5 flex-shrink-0" aria-hidden="true" />
      {pct === 100 ? '100%' : `${pct}%`}
      <button
        type="button"
        onClick={onClear}
        aria-label="OSM-Quelle entfernen"
        className={cn(
          'flex h-3.5 w-3.5 flex-shrink-0 items-center justify-center rounded',
          'text-blue-400 transition-colors duration-150',
          'hover:bg-blue-700/50 hover:text-white',
          'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-blue-400',
        )}
      >
        <X className="h-2.5 w-2.5" />
      </button>
    </span>
  )
}
