// frontend/src/components/segment-editor/SegmentSidebar.tsx
import { useMemo, type ReactNode } from 'react'
import { Check, HelpCircle, Link2, Loader2, RefreshCw } from 'lucide-react'
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
    <aside className="w-[260px] shrink-0 border-r border-neutral-800 bg-neutral-900 p-4 overflow-y-auto">
      {/* Live-Info-Card */}
      {info && (mode === 'track' || (mode === 'draw' && info.distanceKm > 0)) && (
        <div className="mb-4 rounded-md bg-neutral-800 p-3 text-sm">
          <div className="grid grid-cols-3 gap-2 text-center">
            <div>
              <div className="text-xs text-neutral-400">Dist.</div>
              <div className="font-semibold">{info.distanceKm.toFixed(1)} km</div>
            </div>
            <div>
              <div className="text-xs text-neutral-400">↑</div>
              <div className="font-semibold">{Math.round(info.gainM)} m</div>
            </div>
            <div>
              <div className="text-xs text-neutral-400">↓</div>
              <div className="font-semibold">{Math.round(info.lossM)} m</div>
            </div>
          </div>
        </div>
      )}

      {/* Enrichment button */}
      <button
        type="button"
        onClick={onEnrichmentRequest}
        disabled={!!enrichmentDisabledReason || enrichmentState.loading}
        title={enrichmentDisabledReason}
        aria-pressed={!!enrichmentSources.surface || !!enrichmentSources.sac || !!enrichmentSources.mtb}
        className="mb-4 flex w-full items-center justify-center gap-2 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {enrichmentState.loading
          ? <Loader2 className="h-4 w-4 animate-spin" />
          : <RefreshCw className="h-4 w-4" />}
        Aus OSM-Daten holen
      </button>

      {/* Name */}
      <Field label="Name" required>
        <input
          type="text"
          value={meta.name}
          onChange={e => onMetaChange({ name: e.target.value })}
          className="w-full rounded bg-neutral-800 px-2 py-1 text-sm text-white"
          required
        />
      </Field>

      {/* segType */}
      <Field label="Typ">
        <div className="flex flex-col gap-1">
          {(['standard', 'trail', 'lap', 'wintersport'] as SegType[]).map(t => (
            <label key={t} className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="segtype"
                checked={meta.segType === t}
                onChange={() => handleSegTypeChange(t)}
              />
              <span className="capitalize">{t === 'wintersport' ? 'Wintersport' : t}</span>
            </label>
          ))}
        </div>
      </Field>

      {/* Sportarten */}
      <Field label="Sportarten (mehrfach)">
        <div className="flex flex-wrap gap-1">
          <Chip active={meta.sportTypes === null} onClick={setAll}>Alle ✓</Chip>
          {availableSportTypes.map(s => (
            <Chip
              key={s.key}
              active={meta.sportTypes?.includes(s.key) ?? false}
              onClick={() => toggleSportType(s.key)}
            >
              {s.label}
            </Chip>
          ))}
        </div>
      </Field>

      {/* SAC-Skala */}
      {showScaleFields && (
        <Field label="SAC-Skala" tooltip="SAC-Wanderskala — offizielle Schweizer Bergweg-Klassifikation (T1–T6). Skadia nutzt die DAV-Bayern-Auslegung: Ski-Piste-Konvention (blau=einfach, rot=mittel, schwarz=schwer).">
          <div className="flex items-center gap-2">
            <select
              value={meta.sacScale ?? ''}
              onChange={e => onMetaChange({ sacScale: e.target.value || null })}
              className="flex-1 rounded bg-neutral-800 px-2 py-1 text-sm text-white"
            >
              <option value="">—</option>
              {SAC_SCALE_OPTIONS.map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
            <ColorSwatch color={meta.sacScale ? SAC_COLOR_MAP[meta.sacScale] : null} />
            <EnrichmentBadge
              source={enrichmentSources.sac}
              onClear={() => onClearEnrichmentSource('sac')}
            />
          </div>
        </Field>
      )}

      {/* MTB-Skala */}
      {showScaleFields && (
        <Field label="MTB-Skala" tooltip="Singletrail-Skala — offizielle MTB-Technikstufen nach singletrail-skala.de (S0–S5).">
          <div className="flex items-center gap-2">
            <select
              value={meta.mtbScale ?? ''}
              onChange={e => onMetaChange({ mtbScale: e.target.value || null })}
              className="flex-1 rounded bg-neutral-800 px-2 py-1 text-sm text-white"
            >
              <option value="">—</option>
              {MTB_SCALE_OPTIONS.map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
            <ColorSwatch color={meta.mtbScale ? MTB_COLOR_MAP[meta.mtbScale] : null} />
            <EnrichmentBadge
              source={enrichmentSources.mtb}
              onClear={() => onClearEnrichmentSource('mtb')}
            />
          </div>
        </Field>
      )}

      {/* Oberfläche */}
      {showScaleFields && (
        <Field label="Oberfläche">
          <div className="flex items-center gap-2">
            <select
              value={meta.surface ?? ''}
              onChange={e => onMetaChange({ surface: e.target.value || null })}
              className="flex-1 rounded bg-neutral-800 px-2 py-1 text-sm text-white"
            >
              <option value="">—</option>
              {SURFACE_DROPDOWN_OPTIONS.map(s => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
            <EnrichmentBadge
              source={enrichmentSources.surface}
              onClear={() => onClearEnrichmentSource('surface')}
            />
          </div>
        </Field>
      )}

      {/* Difficulty color */}
      <Field label="Schwierigkeitsfarbe">
        <div className="flex items-center gap-2">
          {(['blue', 'red', 'black'] as const).map(c => (
            <button
              key={c}
              type="button"
              aria-label={c}
              aria-pressed={displayedColor === c}
              disabled={difficultyColorReadonly}
              onClick={() => onMetaChange({ difficultyColor: c })}
              className={`h-6 w-6 rounded ${c === 'blue' ? 'bg-blue-500' : c === 'red' ? 'bg-red-500' : 'bg-black border border-neutral-600'} ${displayedColor === c ? 'ring-2 ring-yellow-400' : ''} disabled:cursor-not-allowed`}
            />
          ))}
          {difficultyColorReadonly && (
            <span title="abgeleitet aus SAC/MTB" className="text-neutral-400">
              <Link2 className="h-3 w-3" />
            </span>
          )}
          {!difficultyColorReadonly && displayedColor && (
            <button
              type="button"
              onClick={() => onMetaChange({ difficultyColor: null })}
              className="text-xs text-neutral-500 hover:text-neutral-300"
            >×</button>
          )}
        </div>
      </Field>

      {/* Bidirectional */}
      <Field label="Bidirektional">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={meta.bidirectional}
            onChange={handleBidirectionalToggle}
          />
          <span>In beide Richtungen matchen</span>
        </label>
      </Field>

      {/* Visibility */}
      <Field label="Sichtbarkeit">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={meta.isPublic}
            onChange={e => onMetaChange({ isPublic: e.target.checked })}
          />
          <span>Öffentlich</span>
        </label>
      </Field>

      {/* Live-region for screen-reader enrichment announcements */}
      <div aria-live="polite" aria-atomic="true" className="sr-only">
        {enrichmentState.loading ? 'Lade OSM-Daten...' : ''}
      </div>
    </aside>
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
    <div className="mb-3">
      <div className="mb-1 flex items-center gap-1 text-xs uppercase text-neutral-400">
        {label}{required && <span className="text-red-400">*</span>}
        {tooltip && (
          <span tabIndex={0} aria-describedby={`tip-${label}`} className="cursor-help">
            <HelpCircle className="h-3 w-3" />
            <span id={`tip-${label}`} className="sr-only">{tooltip}</span>
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
      className={`rounded-full px-2 py-0.5 text-xs ${active ? 'bg-blue-600 text-white' : 'bg-neutral-800 text-neutral-300 hover:bg-neutral-700'}`}
    >
      {children}
    </button>
  )
}

function ColorSwatch({ color }: { color: 'blue' | 'red' | 'black' | null | undefined }) {
  if (!color) return <span className="inline-block h-4 w-4 rounded border border-neutral-700 bg-neutral-900" />
  const bg = color === 'blue' ? '#148cf0' : color === 'red' ? '#dc2626' : '#111827'
  return (
    <span
      className="inline-block h-4 w-4 rounded"
      style={{ background: bg, border: color === 'black' ? '1px solid #374151' : undefined }}
    />
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
  return (
    <span className="flex items-center gap-1 rounded bg-blue-900/30 px-1.5 py-0.5 text-[10px] text-blue-200">
      <Check className="h-3 w-3" />
      {Math.round(source.coverage * 100)}%
      <button type="button" onClick={onClear} aria-label="Quelle löschen" className="text-blue-300 hover:text-white">×</button>
    </span>
  )
}
