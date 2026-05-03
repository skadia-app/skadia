import { useState, useId, useCallback, useEffect, useRef } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import type { MapOverlayState } from '@/pages/MapPage'
import type { CoverageOverlayState, CoverageCompositeState } from '@/hooks/useCoverageOverlays'
import type { MapLayer, UserMapLayer } from '@/api/mapLayers'
import type { LayerEntry } from '@/hooks/useLayerOrder'
import { useSystemLayerVisibility } from '@/hooks/useSystemLayerVisibility'

export interface LayerPanelProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  overlayState: MapOverlayState
  onOverlayChange: (key: keyof MapOverlayState, value: boolean) => void
  coverageState: CoverageOverlayState
  onCoverageUpdate: (key: keyof CoverageOverlayState, update: Partial<CoverageCompositeState>) => void
  onCoverageToggleLayer: (composite: keyof CoverageOverlayState, layer: string, enabled: boolean) => void
  systemOverlays: MapLayer[]
  systemOverlayEnabled: Record<string, boolean>
  systemOverlayOpacity: Record<string, number>
  onToggleSystemOverlay: (key: string, enabled: boolean) => void
  onSetSystemOverlayOpacity: (key: string, opacity: number) => void
  userOverlays: UserMapLayer[]
  onPatchUserLayer: (id: number, patch: Partial<Pick<UserMapLayer, 'enabled' | 'default_opacity'>>) => void
  /** Optional live-preview hook: writes opacity into the local query cache
   * (driving the map's setPaintProperty via paint-update) without sending a
   * network PATCH. The slider uses this on every drag step and only calls
   * onPatchUserLayer on pointer-up — otherwise a fast drag spams the backend
   * and trips its rate limiter (HTTP 429), whose onError rollback then snaps
   * the controlled slider back to the pre-mutation value mid-drag. */
  onLiveOpacityPreview?: (id: number, opacity: number) => void
  layerEntries: LayerEntry[]
  onMoveTo: (key: string, toKey: string | null, fallbackZone?: 'above' | 'below') => void
  onChangeZone: (key: string, zone: 'above' | 'below') => void
}

// ---------- Primitives ----------

function DragHandle({ dragging }: { dragging?: boolean }) {
  return (
    <span
      className={`flex-shrink-0 text-neutral-300 dark:text-neutral-600 group-hover:text-neutral-500 dark:group-hover:text-neutral-400 transition-colors ${
        dragging ? 'cursor-grabbing' : 'cursor-grab'
      }`}
      aria-hidden="true"
    >
      <svg className="w-3 h-4" viewBox="0 0 8 14" fill="currentColor">
        <circle cx="2" cy="2"  r="1.2" />
        <circle cx="6" cy="2"  r="1.2" />
        <circle cx="2" cy="7"  r="1.2" />
        <circle cx="6" cy="7"  r="1.2" />
        <circle cx="2" cy="12" r="1.2" />
        <circle cx="6" cy="12" r="1.2" />
      </svg>
    </span>
  )
}

function ColorDot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`flex-shrink-0 w-2 h-2 rounded-full ${className ?? 'bg-transparent'}`}
    />
  )
}

function Chevron({ expanded }: { expanded: boolean }) {
  return (
    <svg
      className={`w-3 h-3 transition-transform ${expanded ? '' : '-rotate-90'}`}
      viewBox="0 0 20 20"
      fill="currentColor"
    >
      <path
        fillRule="evenodd"
        d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
        clipRule="evenodd"
      />
    </svg>
  )
}

function OpacityRow({
  value,
  onChange,
  onCommit,
  labelId,
  disabled,
}: {
  value: number
  /** Fires on every drag step — keep this cheap (local state + cache write).
   *  Network PATCH must NOT happen here; otherwise rapid drags spam the
   *  backend rate limiter, the resulting 429s rollback the cache and the
   *  controlled slider snaps back mid-drag. */
  onChange: (v: number) => void
  /** Fires once when the user releases the slider (pointer-up, key-up, blur).
   *  This is where the actual PATCH belongs. If omitted, onChange handles
   *  everything — used by rows that don't need separate live/commit. */
  onCommit?: (v: number) => void
  labelId: string
  disabled?: boolean
}) {
  return (
    <div className="flex items-center gap-2 pl-7 pr-2 py-1 text-[10px] text-neutral-500 dark:text-neutral-400">
      <span id={labelId} className="whitespace-nowrap">Deckkraft</span>
      <input
        type="range"
        min={0}
        max={100}
        step={5}
        value={value}
        disabled={disabled}
        aria-labelledby={labelId}
        aria-valuetext={`${value}%`}
        onChange={e => onChange(Number(e.target.value))}
        onPointerUp={onCommit ? e => onCommit(Number((e.target as HTMLInputElement).value)) : undefined}
        onKeyUp={onCommit ? e => onCommit(Number((e.target as HTMLInputElement).value)) : undefined}
        onBlur={onCommit ? e => onCommit(Number((e.target as HTMLInputElement).value)) : undefined}
        className="flex-1 h-1 accent-accent-DEFAULT cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
      />
      <span className="w-7 text-right tabular-nums">{value}%</span>
    </div>
  )
}

// ---------- Drag wrapper ----------

interface DragState {
  draggingKey: string | null
  overKey: string | null
}

function useLayerDrag(onMoveTo: LayerPanelProps['onMoveTo']) {
  const [state, setState] = useState<DragState>({ draggingKey: null, overKey: null })

  const onDragStart = useCallback((e: React.DragEvent, key: string) => {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', key)
    setState({ draggingKey: key, overKey: null })
  }, [])

  const onDragOver = useCallback((e: React.DragEvent, key: string) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    setState(s => (s.overKey === key ? s : { ...s, overKey: key }))
  }, [])

  const onDragEnd = useCallback(() => {
    setState({ draggingKey: null, overKey: null })
  }, [])

  const onDrop = useCallback((e: React.DragEvent, targetKey: string) => {
    e.preventDefault()
    const fromKey = e.dataTransfer.getData('text/plain') || state.draggingKey
    if (!fromKey || fromKey === targetKey) {
      setState({ draggingKey: null, overKey: null })
      return
    }
    onMoveTo(fromKey, targetKey)
    setState({ draggingKey: null, overKey: null })
  }, [onMoveTo, state.draggingKey])

  // Drop on an empty zone area → append to that zone.
  const onDropZone = useCallback((e: React.DragEvent, zone: 'above' | 'below') => {
    e.preventDefault()
    const fromKey = e.dataTransfer.getData('text/plain') || state.draggingKey
    if (!fromKey) {
      setState({ draggingKey: null, overKey: null })
      return
    }
    onMoveTo(fromKey, null, zone)
    setState({ draggingKey: null, overKey: null })
  }, [onMoveTo, state.draggingKey])

  const onDragOverZone = useCallback((e: React.DragEvent, zone: 'above' | 'below') => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    const sentinel = `__zone-end-${zone}`
    setState(s => (s.overKey === sentinel ? s : { ...s, overKey: sentinel }))
  }, [])

  return { state, onDragStart, onDragOver, onDragEnd, onDrop, onDragOverZone, onDropZone }
}

// ---------- Specialized rows ----------

interface RowDragProps {
  entryKey: string
  draggable: boolean
  isDragging: boolean
  isOver: boolean
  onDragStart: (e: React.DragEvent, key: string) => void
  onDragOver: (e: React.DragEvent, key: string) => void
  onDragEnd: () => void
  onDrop: (e: React.DragEvent, key: string) => void
}

function rowDragHandlers(p: RowDragProps) {
  return {
    draggable: p.draggable,
    onDragStart: (e: React.DragEvent) => p.onDragStart(e, p.entryKey),
    onDragOver:  (e: React.DragEvent) => p.onDragOver(e, p.entryKey),
    onDragEnd:   p.onDragEnd,
    onDrop:      (e: React.DragEvent) => p.onDrop(e, p.entryKey),
  }
}

function rowClasses(p: RowDragProps): string {
  return [
    'group relative flex items-center gap-2 pl-2 pr-2 py-1.5 rounded transition-colors',
    p.isDragging ? 'opacity-40' : '',
    p.isOver && !p.isDragging ? 'before:absolute before:inset-x-1 before:top-0 before:h-px before:bg-accent-DEFAULT' : '',
    'hover:bg-neutral-100 dark:hover:bg-neutral-700/40',
  ].join(' ')
}

function SimpleVectorRow({
  drag,
  checked,
  onChange,
  label,
}: {
  drag: RowDragProps
  checked: boolean
  onChange: (v: boolean) => void
  label: string
}) {
  return (
    <div className={rowClasses(drag)} {...rowDragHandlers(drag)}>
      <DragHandle dragging={drag.isDragging} />
      <ColorDot />
      <label className="flex items-center gap-2 text-xs cursor-pointer flex-1 min-w-0">
        <input
          type="checkbox"
          checked={checked}
          onChange={e => onChange(e.target.checked)}
          className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer"
        />
        <span className="truncate text-neutral-700 dark:text-neutral-200">{label}</span>
      </label>
    </div>
  )
}

function SummitsRow({
  drag,
  overlayState,
  onOverlayChange,
  label,
}: {
  drag: RowDragProps
  overlayState: MapOverlayState
  onOverlayChange: (key: keyof MapOverlayState, value: boolean) => void
  label: string
}) {
  const [expanded, setExpanded] = useState(false)
  const navigate = useNavigate()

  return (
    <div>
      <div className={rowClasses(drag)} {...rowDragHandlers(drag)}>
        <DragHandle dragging={drag.isDragging} />
        <ColorDot />
        <label className="flex items-center gap-2 text-xs cursor-pointer flex-1 min-w-0">
          <input
            type="checkbox"
            checked={overlayState.showSummits}
            onChange={e => onOverlayChange('showSummits', e.target.checked)}
            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer"
          />
          <span className="truncate text-neutral-700 dark:text-neutral-200">{label}</span>
        </label>
        <button
          onClick={() => setExpanded(e => !e)}
          aria-label={expanded ? 'Optionen einklappen' : 'Optionen ausklappen'}
          aria-expanded={expanded}
          className="p-1 rounded text-neutral-400 hover:text-neutral-700 hover:bg-neutral-200/70 dark:hover:bg-neutral-700/60 cursor-pointer"
        >
          <Chevron expanded={expanded} />
        </button>
      </div>
      {expanded && (
        <div className="ml-9 mr-2 mb-1 border-l border-neutral-200 dark:border-neutral-700 pl-3 py-1 space-y-1">
          {overlayState.showSummits && (
            <label className="flex items-center gap-2 text-[11px] cursor-pointer text-neutral-600 dark:text-neutral-300">
              <input
                type="checkbox"
                checked={overlayState.summitsOnlyVisited}
                onChange={e => onOverlayChange('summitsOnlyVisited', e.target.checked)}
                className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer"
              />
              <span>Nur besuchte</span>
            </label>
          )}
          <button
            onClick={() => navigate('/map/summits')}
            className="text-[11px] text-neutral-600 dark:text-neutral-300 hover:text-accent-DEFAULT hover:underline cursor-pointer"
          >
            Gipfelsammlung öffnen →
          </button>
        </div>
      )}
    </div>
  )
}

const COVERAGE_ENTRY_MAP: Record<string, keyof CoverageOverlayState> = {
  'coverage-nature':  'nature',
  'coverage-noise':   'noise',
  'coverage-traffic': 'trafficUrban',
}

const COVERAGE_META: Record<keyof CoverageOverlayState, {
  label: string
  dot: string
  layerLabels: Record<string, string>
}> = {
  nature: {
    label: 'Natur',
    dot: 'bg-green-500',
    layerLabels: { forest: 'Wald', water: 'Gewässer', riverProximity: 'Fluss-Nähe', protected: 'Schutzgebiete', landuse: 'Landnutzung' },
  },
  noise: {
    label: 'Lärm',
    dot: 'bg-purple-500',
    layerLabels: {},
  },
  trafficUrban: {
    label: 'Verkehr & Urbanisierung',
    dot: 'bg-orange-500',
    layerLabels: { traffic: 'Verkehr', urbanization: 'Urbanisierung' },
  },
}

function CoverageRow({
  drag,
  compositeKey,
  state,
  onUpdate,
  onToggleLayer,
}: {
  drag: RowDragProps
  compositeKey: keyof CoverageOverlayState
  state: CoverageCompositeState
  onUpdate: (key: keyof CoverageOverlayState, update: Partial<CoverageCompositeState>) => void
  onToggleLayer: (composite: keyof CoverageOverlayState, layer: string, enabled: boolean) => void
}) {
  const meta = COVERAGE_META[compositeKey]
  const [expanded, setExpanded] = useState(false)
  const sublayerKeys = Object.keys(meta.layerLabels)
  const enabledSublayerCount = sublayerKeys.filter(k => state.layers[k] ?? true).length
  const sublabelId = useId()

  return (
    <div>
      <div className={rowClasses(drag)} {...rowDragHandlers(drag)}>
        <DragHandle dragging={drag.isDragging} />
        <ColorDot className={meta.dot} />
        <label className="flex items-center gap-2 text-xs cursor-pointer flex-1 min-w-0">
          <input
            type="checkbox"
            checked={state.enabled}
            onChange={e => onUpdate(compositeKey, { enabled: e.target.checked })}
            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer"
          />
          <span className="truncate text-neutral-700 dark:text-neutral-200">
            {meta.label}
            {sublayerKeys.length > 0 && (
              <span className="ml-1.5 text-neutral-400 dark:text-neutral-500 tabular-nums">
                ({enabledSublayerCount}/{sublayerKeys.length})
              </span>
            )}
          </span>
        </label>
        <span className="text-[10px] text-neutral-400 tabular-nums w-7 text-right">
          {state.opacity}%
        </span>
        <button
          onClick={() => setExpanded(e => !e)}
          aria-label={expanded ? 'Optionen einklappen' : 'Optionen ausklappen'}
          aria-expanded={expanded}
          className="p-1 rounded text-neutral-400 hover:text-neutral-700 hover:bg-neutral-200/70 dark:hover:bg-neutral-700/60 cursor-pointer"
        >
          <Chevron expanded={expanded} />
        </button>
      </div>
      {expanded && (
        <div className="mb-1">
          <OpacityRow
            value={state.opacity}
            onChange={v => onUpdate(compositeKey, { opacity: v })}
            labelId={sublabelId}
            disabled={!state.enabled}
          />
          {sublayerKeys.length > 0 && (
            <div className="ml-9 mr-2 border-l border-neutral-200 dark:border-neutral-700 pl-3 py-1 space-y-1">
              {Object.entries(meta.layerLabels).map(([key, name]) => (
                <label key={key} className="flex items-center gap-2 text-[11px] cursor-pointer text-neutral-600 dark:text-neutral-300">
                  <input
                    type="checkbox"
                    checked={state.layers[key] ?? true}
                    onChange={e => onToggleLayer(compositeKey, key, e.target.checked)}
                    className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer"
                    disabled={!state.enabled}
                  />
                  <span>{name}</span>
                </label>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function UserLayerRow({
  drag,
  layer,
  onPatch,
  onLiveOpacity,
}: {
  drag: RowDragProps
  layer: UserMapLayer
  onPatch: (id: number, patch: Partial<Pick<UserMapLayer, 'enabled' | 'default_opacity'>>) => void
  onLiveOpacity?: (id: number, opacity: number) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const opacityId = useId()
  const externalOpacity = layer.default_opacity ?? 100
  // Local slider state: decouples the controlled <input value=…> from the
  // external query cache during a drag. The external cache flips back to the
  // pre-PATCH value whenever the backend rate-limits a mid-burst PATCH (HTTP
  // 429 → onError rollback to ctx.prev), which would otherwise snap the
  // slider thumb back to the old number while the user is still dragging.
  const [opacityLocal, setOpacityLocal] = useState(externalOpacity)
  const draggingRef = useRef(false)
  const lastCommittedRef = useRef(externalOpacity)

  // Pull external updates only when the user isn't actively dragging — e.g.
  // a refetch after the commit-PATCH, or another tab updating the layer.
  useEffect(() => {
    if (!draggingRef.current) {
      setOpacityLocal(externalOpacity)
      lastCommittedRef.current = externalOpacity
    }
  }, [externalOpacity])

  const handleLive = (v: number) => {
    draggingRef.current = true
    setOpacityLocal(v)
    // Cheap optimistic-cache write so MapLibre's paint-update op runs and the
    // map opacity follows the slider in real time. No network PATCH yet.
    onLiveOpacity?.(layer.id, v)
  }

  const handleCommit = (v: number) => {
    draggingRef.current = false
    // Compare against the last committed value, not the live-preview prop:
    // the optimistic preview can already have moved externalOpacity to `v`.
    if (v !== lastCommittedRef.current) {
      lastCommittedRef.current = v
      onPatch(layer.id, { default_opacity: v })
    }
  }

  return (
    <div>
      <div className={rowClasses(drag)} {...rowDragHandlers(drag)}>
        <DragHandle dragging={drag.isDragging} />
        <ColorDot />
        <label className="flex items-center gap-2 text-xs cursor-pointer flex-1 min-w-0">
          <input
            type="checkbox"
            checked={layer.enabled}
            onChange={e => onPatch(layer.id, { enabled: e.target.checked })}
            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer"
          />
          <span className="truncate text-neutral-700 dark:text-neutral-200">{layer.name}</span>
        </label>
        <span className="text-[10px] text-neutral-400 tabular-nums w-7 text-right">{opacityLocal}%</span>
        <button
          onClick={() => setExpanded(e => !e)}
          aria-label={expanded ? 'Optionen einklappen' : 'Optionen ausklappen'}
          aria-expanded={expanded}
          className="p-1 rounded text-neutral-400 hover:text-neutral-700 hover:bg-neutral-200/70 dark:hover:bg-neutral-700/60 cursor-pointer"
        >
          <Chevron expanded={expanded} />
        </button>
      </div>
      {expanded && (
        <OpacityRow
          value={opacityLocal}
          onChange={handleLive}
          onCommit={handleCommit}
          labelId={opacityId}
          disabled={!layer.enabled}
        />
      )}
    </div>
  )
}

function SystemRasterRow({
  drag,
  layer,
  enabled,
  opacity,
  onToggle,
  onSetOpacity,
}: {
  drag: RowDragProps | null
  layer: MapLayer
  enabled: boolean
  opacity: number
  onToggle: (key: string, enabled: boolean) => void
  onSetOpacity: (key: string, opacity: number) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const opacityId = useId()
  const isVectorStyle = layer.source_kind === 'vector_style'

  // System raster overlays aren't part of layerEntries, no drag.
  return (
    <div>
      <div className="group flex items-center gap-2 pl-2 pr-2 py-1.5 rounded hover:bg-neutral-100 dark:hover:bg-neutral-700/40">
        {drag ? <DragHandle /> : <span className="w-3 flex-shrink-0" aria-hidden="true" />}
        <ColorDot />
        <label className="flex items-center gap-2 text-xs cursor-pointer flex-1 min-w-0">
          <input
            type="checkbox"
            checked={!isVectorStyle && enabled}
            disabled={isVectorStyle}
            onChange={e => onToggle(layer.key, e.target.checked)}
            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
          />
          <span
            className={`truncate ${
              isVectorStyle
                ? 'text-neutral-400 dark:text-neutral-500 line-through'
                : 'text-neutral-700 dark:text-neutral-200'
            }`}
            title={isVectorStyle ? 'Nur als Basiskarte verfügbar' : undefined}
          >
            {layer.name}
          </span>
        </label>
        {isVectorStyle ? (
          <span className="text-[10px] text-neutral-400 italic">Karte</span>
        ) : (
          <>
            <span className="text-[10px] text-neutral-400 tabular-nums w-7 text-right">{opacity}%</span>
            <button
              onClick={() => setExpanded(e => !e)}
              aria-label={expanded ? 'Optionen einklappen' : 'Optionen ausklappen'}
              aria-expanded={expanded}
              className="p-1 rounded text-neutral-400 hover:text-neutral-700 hover:bg-neutral-200/70 dark:hover:bg-neutral-700/60 cursor-pointer"
            >
              <Chevron expanded={expanded} />
            </button>
          </>
        )}
      </div>
      {!isVectorStyle && expanded && (
        <OpacityRow
          value={opacity}
          onChange={v => onSetOpacity(layer.key, v)}
          labelId={opacityId}
          disabled={!enabled}
        />
      )}
    </div>
  )
}

// ---------- Section header ----------

function ZoneHeader({ icon, label }: { icon: 'above' | 'below'; label: string }) {
  return (
    <div className="flex items-center gap-1.5 px-2 pt-1 pb-0.5">
      <svg className="w-3 h-3 text-neutral-400 dark:text-neutral-500" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true">
        {icon === 'above' ? (
          <path d="M6 2L1.5 7h3v3h3V7h3z" />
        ) : (
          <path d="M6 10L1.5 5h3V2h3v3h3z" />
        )}
      </svg>
      <span className="text-[10px] font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wider">
        {label}
      </span>
    </div>
  )
}

// ---------- Zones ----------

const SYSTEM_ABOVE_KEYS = ['summits', 'waters', 'forests', 'visitedPois', 'amenities'] as const
type SystemAboveKey = typeof SYSTEM_ABOVE_KEYS[number]

const SYSTEM_ABOVE_META: Record<SystemAboveKey, { fallbackLabel: string; visKey: string; flag: keyof MapOverlayState }> = {
  summits:     { fallbackLabel: 'Gipfel',          visKey: 'summits',         flag: 'showSummits' },
  waters:      { fallbackLabel: 'Gewässer',        visKey: 'waters',          flag: 'showWaters' },
  forests:     { fallbackLabel: 'Wälder',          visKey: 'forests',         flag: 'showForests' },
  visitedPois: { fallbackLabel: 'Besuchte Einkehr',visKey: 'visited_pois',    flag: 'showVisitedPois' },
  amenities:   { fallbackLabel: 'Einkehr (Tiles)', visKey: 'amenities_tiles', flag: 'showAmenities' },
}

type LayerDragApi = ReturnType<typeof useLayerDrag>

const COVERAGE_KEYS = ['coverage-nature', 'coverage-noise', 'coverage-traffic'] as const

const COVERAGE_SUBLAYERS_BY_ENTRY: Record<string, string[]> = {
  'coverage-nature':  ['coverage_landuse', 'coverage_forest', 'coverage_water', 'coverage_river_proximity', 'coverage_protected'],
  'coverage-noise':   ['coverage_noise'],
  'coverage-traffic': ['coverage_urbanization', 'coverage_traffic'],
}

// Shared row renderer used by both zones — the entry's zone is layout-only.
// Any layer type (system vector, coverage raster, user raster) must render in
// whatever zone it currently sits in, otherwise cross-zone drag silently drops it.
function renderLayerRow(
  entry: LayerEntry,
  drag: RowDragProps,
  rest: Pick<LayerPanelProps,
    'overlayState' | 'onOverlayChange' |
    'coverageState' | 'onCoverageUpdate' | 'onCoverageToggleLayer' |
    'userOverlays' | 'onPatchUserLayer' | 'onLiveOpacityPreview'
  >,
  visibility: (key: string) => { enabled: boolean; label: string | null },
): React.ReactNode {
  if ((SYSTEM_ABOVE_KEYS as readonly string[]).includes(entry.key)) {
    const meta = SYSTEM_ABOVE_META[entry.key as SystemAboveKey]
    const vis = visibility(meta.visKey)
    if (!vis.enabled) return null
    if (entry.key === 'summits') {
      return (
        <SummitsRow
          key={entry.key}
          drag={drag}
          overlayState={rest.overlayState}
          onOverlayChange={rest.onOverlayChange}
          label={vis.label ?? meta.fallbackLabel}
        />
      )
    }
    return (
      <SimpleVectorRow
        key={entry.key}
        drag={drag}
        checked={Boolean(rest.overlayState[meta.flag])}
        onChange={v => rest.onOverlayChange(meta.flag, v)}
        label={vis.label ?? meta.fallbackLabel}
      />
    )
  }

  if ((COVERAGE_KEYS as readonly string[]).includes(entry.key)) {
    const sublayers = COVERAGE_SUBLAYERS_BY_ENTRY[entry.key] ?? []
    const anyAdminEnabled = sublayers.length === 0 || sublayers.some(k => visibility(k).enabled)
    if (!anyAdminEnabled) return null
    const compositeKey = COVERAGE_ENTRY_MAP[entry.key]
    if (!compositeKey) return null
    return (
      <CoverageRow
        key={entry.key}
        drag={drag}
        compositeKey={compositeKey}
        state={rest.coverageState[compositeKey]}
        onUpdate={rest.onCoverageUpdate}
        onToggleLayer={rest.onCoverageToggleLayer}
      />
    )
  }

  if (entry.key.startsWith('user-')) {
    const id = Number(entry.key.slice('user-'.length))
    const layer = rest.userOverlays.find(u => u.id === id)
    if (!layer) return null
    return (
      <UserLayerRow
        key={entry.key}
        drag={drag}
        layer={layer}
        onPatch={rest.onPatchUserLayer}
        onLiveOpacity={rest.onLiveOpacityPreview}
      />
    )
  }

  return null
}

function buildRowDrag(entryKey: string, dnd: LayerDragApi): RowDragProps {
  return {
    entryKey,
    draggable: true,
    isDragging: dnd.state.draggingKey === entryKey,
    isOver: dnd.state.overKey === entryKey && dnd.state.draggingKey !== null && dnd.state.draggingKey !== entryKey,
    onDragStart: dnd.onDragStart,
    onDragOver:  dnd.onDragOver,
    onDragEnd:   dnd.onDragEnd,
    onDrop:      dnd.onDrop,
  }
}

function AboveZone(props: Pick<LayerPanelProps,
  'overlayState' | 'onOverlayChange' | 'layerEntries' |
  'coverageState' | 'onCoverageUpdate' | 'onCoverageToggleLayer' |
  'userOverlays' | 'onPatchUserLayer' | 'onLiveOpacityPreview'
> & { dnd: LayerDragApi }) {
  const { visibility } = useSystemLayerVisibility()
  const dnd = props.dnd

  const aboveEntries = props.layerEntries.filter(e => e.zone === 'above')

  return (
    <div className="space-y-0.5">
      <ZoneHeader icon="above" label="Über Aktivitäten" />
      {aboveEntries.map(entry => renderLayerRow(entry, buildRowDrag(entry.key, dnd), props, visibility))}
      <div
        onDragOver={e => dnd.onDragOverZone(e, 'above')}
        onDrop={e => dnd.onDropZone(e, 'above')}
        className={`relative h-2 mx-1 rounded transition-colors ${
          dnd.state.draggingKey && dnd.state.overKey === '__zone-end-above'
            ? 'bg-accent-DEFAULT/20 before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-accent-DEFAULT'
            : ''
        }`}
        aria-hidden="true"
      />
    </div>
  )
}

function BelowZone(props: Pick<LayerPanelProps,
  'overlayState' | 'onOverlayChange' |
  'coverageState' | 'onCoverageUpdate' | 'onCoverageToggleLayer' |
  'layerEntries' |
  'userOverlays' | 'onPatchUserLayer' | 'onLiveOpacityPreview' |
  'systemOverlays' | 'systemOverlayEnabled' | 'systemOverlayOpacity' |
  'onToggleSystemOverlay' | 'onSetSystemOverlayOpacity'
> & { dnd: LayerDragApi }) {
  const { visibility } = useSystemLayerVisibility()
  const dnd = props.dnd

  const belowEntries = props.layerEntries.filter(e => e.zone === 'below')

  return (
    <div className="space-y-0.5 bg-neutral-50/60 dark:bg-neutral-900/30 rounded -mx-1 px-1 pt-1 pb-1">
      <ZoneHeader icon="below" label="Unter Aktivitäten" />
      {belowEntries.map(entry => renderLayerRow(entry, buildRowDrag(entry.key, dnd), props, visibility))}
      <div
        onDragOver={e => dnd.onDragOverZone(e, 'below')}
        onDrop={e => dnd.onDropZone(e, 'below')}
        className={`relative h-2 mx-1 rounded transition-colors ${
          dnd.state.draggingKey && dnd.state.overKey === '__zone-end-below'
            ? 'bg-accent-DEFAULT/20 before:absolute before:inset-x-0 before:top-0 before:h-px before:bg-accent-DEFAULT'
            : ''
        }`}
        aria-hidden="true"
      />

      {props.systemOverlays.length > 0 && (
        <>
          <div className="mt-1 mb-0.5 mx-2 border-t border-neutral-200 dark:border-neutral-700" />
          {props.systemOverlays.map(layer => (
            <SystemRasterRow
              key={layer.key}
              drag={null}
              layer={layer}
              enabled={props.systemOverlayEnabled[layer.key] ?? false}
              opacity={props.systemOverlayOpacity[layer.key] ?? layer.default_opacity}
              onToggle={props.onToggleSystemOverlay}
              onSetOpacity={props.onSetSystemOverlayOpacity}
            />
          ))}
        </>
      )}
    </div>
  )
}

// ---------- Panel ----------

export function LayerPanel(props: LayerPanelProps & { panelAlign?: 'left' | 'right'; buttonRounded?: boolean }) {
  const [localOpen, setLocalOpen] = useState(false)
  const open = props.open !== undefined ? props.open : localOpen
  const setOpen = props.onOpenChange ?? setLocalOpen
  const titleId = useId()
  const dnd = useLayerDrag(props.onMoveTo)
  const align = props.panelAlign ?? 'right'
  const buttonRounded = props.buttonRounded ?? false

  const anyActive =
    Object.values(props.overlayState).some(Boolean) ||
    props.coverageState.nature.enabled ||
    props.coverageState.noise.enabled ||
    props.coverageState.trafficUrban.enabled ||
    props.userOverlays.some(l => l.enabled) ||
    props.systemOverlays.some(l => props.systemOverlayEnabled[l.key])

  return (
    <div className={`relative flex flex-col gap-1 ${align === 'right' ? 'items-end' : 'items-start'}`}>
      {open && (
        <div
          id={`layer-panel-${titleId}`}
          role="region"
          aria-labelledby={titleId}
          className={`absolute bottom-full mb-1 ${align === 'right' ? 'right-0' : 'left-0'} w-[min(18rem,calc(100vw-1rem))] rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white/95 dark:bg-neutral-800/95 backdrop-blur-sm shadow-xl overflow-hidden`}
        >
          <div className="flex items-center justify-between px-3 py-2 border-b border-neutral-200 dark:border-neutral-700">
            <span id={titleId} className="text-xs font-semibold text-neutral-600 dark:text-neutral-300 uppercase tracking-wide">
              Ebenen
            </span>
            <button
              onClick={() => setOpen(false)}
              aria-label="Ebenen schließen"
              className="p-1 rounded text-neutral-400 hover:text-neutral-700 hover:bg-neutral-200/70 dark:hover:text-neutral-200 dark:hover:bg-neutral-700/60 cursor-pointer transition-colors"
            >
              <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>
          <div className="px-2 py-2 space-y-2 max-h-[70vh] overflow-y-auto">
            <AboveZone
              dnd={dnd}
              overlayState={props.overlayState}
              onOverlayChange={props.onOverlayChange}
              coverageState={props.coverageState}
              onCoverageUpdate={props.onCoverageUpdate}
              onCoverageToggleLayer={props.onCoverageToggleLayer}
              layerEntries={props.layerEntries}
              userOverlays={props.userOverlays}
              onPatchUserLayer={props.onPatchUserLayer}
              onLiveOpacityPreview={props.onLiveOpacityPreview}
            />
            <BelowZone
              dnd={dnd}
              overlayState={props.overlayState}
              onOverlayChange={props.onOverlayChange}
              coverageState={props.coverageState}
              onCoverageUpdate={props.onCoverageUpdate}
              onCoverageToggleLayer={props.onCoverageToggleLayer}
              layerEntries={props.layerEntries}
              userOverlays={props.userOverlays}
              onPatchUserLayer={props.onPatchUserLayer}
              onLiveOpacityPreview={props.onLiveOpacityPreview}
              systemOverlays={props.systemOverlays}
              systemOverlayEnabled={props.systemOverlayEnabled}
              systemOverlayOpacity={props.systemOverlayOpacity}
              onToggleSystemOverlay={props.onToggleSystemOverlay}
              onSetSystemOverlayOpacity={props.onSetSystemOverlayOpacity}
            />
          </div>
          <div className="px-3 py-2 border-t border-neutral-200 dark:border-neutral-700">
            <Link
              to="/settings?tab=karte"
              className="text-[11px] text-neutral-500 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-neutral-200 transition-colors"
            >
              Layer verwalten →
            </Link>
          </div>
        </div>
      )}
      <button
        onClick={() => setOpen(!open)}
        aria-label={open ? 'Ebenen schließen' : 'Ebenen öffnen'}
        aria-expanded={open}
        title="Ebenen"
        className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer flex-shrink-0 ${
          buttonRounded
            ? 'rounded-lg border border-neutral-300 dark:border-neutral-600 shadow-md backdrop-blur-sm'
            : 'rounded-r-lg'
        } ${
          open || anyActive
            ? 'bg-accent-DEFAULT text-white border-transparent'
            : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'
        }`}
      >
        <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <polygon points="12 2 2 7 12 12 22 7 12 2" />
          <polyline points="2 17 12 22 22 17" />
          <polyline points="2 12 12 17 22 12" />
        </svg>
        Ebenen
      </button>
    </div>
  )
}
