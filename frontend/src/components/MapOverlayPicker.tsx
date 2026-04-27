import { useState, useId } from 'react'
import type { CoverageOverlayState, CoverageCompositeState } from '@/hooks/useCoverageOverlays'
import type { MapLayer } from '@/api/mapLayers'
import type { UserMapLayer } from '@/api/mapLayers'

// ── Prop types ────────────────────────────────────────────────────────────────

interface Props {
  /** Coverage composites (Natur, Lärm, Verkehr) from useCoverageOverlays. */
  coverageState: CoverageOverlayState
  onCoverageUpdate: (key: keyof CoverageOverlayState, update: Partial<CoverageCompositeState>) => void

  /**
   * System raster/vector overlay layers from useMapLayersMerged().overlay
   * filtered to is_system=true. Source_kind='vector_style' entries are shown as
   * disabled with an explanatory label — the picker only toggles raster layers.
   */
  systemOverlays: MapLayer[]
  /** Current per-layer enabled state for system overlays (key → enabled). */
  systemOverlayEnabled: Record<string, boolean>
  /** Current per-layer opacity (0-100) for system overlays (key → opacity). */
  systemOverlayOpacity: Record<string, number>
  onToggleSystemOverlay: (key: string, enabled: boolean) => void
  onSetSystemOverlayOpacity: (key: string, opacity: number) => void

  /**
   * User-owned raster overlay layers (role='overlay', source_kind='raster_tiles').
   * vector_style user overlays are excluded before passing — they are not supported
   * as overlays (would require MapLibre style-composition which is out of scope).
   */
  userOverlays: UserMapLayer[]
  onPatchUserLayer: (id: number, patch: Partial<Pick<UserMapLayer, 'enabled' | 'default_opacity'>>) => void
}

// ── Sub-components ────────────────────────────────────────────────────────────

interface OpacitySliderProps {
  value: number
  onChange: (v: number) => void
  labelId: string
  disabled?: boolean
}

function OpacitySlider({ value, onChange, labelId, disabled }: OpacitySliderProps) {
  return (
    <input
      type="range"
      min={0}
      max={100}
      step={5}
      value={value}
      disabled={disabled}
      aria-labelledby={labelId}
      aria-valuetext={`${value}%`}
      onChange={(e) => onChange(Number(e.target.value))}
      className="w-full h-1.5 accent-accent-DEFAULT cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
    />
  )
}

// Label mapping for coverage composites
const COVERAGE_LABELS: Record<keyof CoverageOverlayState, string> = {
  nature: 'Natur',
  noise: 'Lärm',
  trafficUrban: 'Verkehr',
}

// ── Main component ────────────────────────────────────────────────────────────

/**
 * Floating overlay picker panel — top-right corner of the map.
 * Collapsed by default; toggled with a layers icon button.
 *
 * Lists all overlay categories:
 *   1. Coverage composites (nature / noise / trafficUrban)
 *   2. System raster overlays (e.g. hillshade)
 *   3. User-owned raster overlays
 *
 * Stateless: all state lives in the hooks at MapPage level.
 */
export default function MapOverlayPicker({
  coverageState,
  onCoverageUpdate,
  systemOverlays,
  systemOverlayEnabled,
  systemOverlayOpacity,
  onToggleSystemOverlay,
  onSetSystemOverlayOpacity,
  userOverlays,
  onPatchUserLayer,
}: Props) {
  const [open, setOpen] = useState(false)
  const titleId = useId()

  // Derive whether any overlay is currently active (for the button highlight).
  const anyCoverageActive = Object.values(coverageState).some((c) => c.enabled)
  const anySystemActive = systemOverlays.some((l) => systemOverlayEnabled[l.key])
  const anyUserActive = userOverlays.some((ul) => ul.enabled)
  const anyActive = anyCoverageActive || anySystemActive || anyUserActive

  const hasSysOverlays = systemOverlays.length > 0
  const hasUserOverlays = userOverlays.length > 0
  const hasCoverage = true // coverage composites always present (even if no data yet)
  const isEmpty = !hasSysOverlays && !hasUserOverlays && !hasCoverage

  return (
    <div className="absolute top-14 right-4 z-10 flex flex-col items-end gap-1">
      {/* Toggle button */}
      <button
        onClick={() => setOpen((o) => !o)}
        aria-label={open ? 'Ebenen-Auswahl schließen' : 'Ebenen-Auswahl öffnen'}
        aria-expanded={open}
        aria-controls={open ? 'overlay-picker-panel' : undefined}
        title="Ebenen"
        className={`flex items-center justify-center w-8 h-8 rounded-lg border shadow-md backdrop-blur-sm transition-colors cursor-pointer ${
          open || anyActive
            ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
            : 'bg-white/90 dark:bg-neutral-800/90 text-neutral-600 dark:text-neutral-300 border-neutral-300 dark:border-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-700'
        }`}
      >
        {/* Layers / stacked-pages icon */}
        <svg
          className="w-4 h-4"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <polygon points="12 2 2 7 12 12 22 7 12 2" />
          <polyline points="2 17 12 22 22 17" />
          <polyline points="2 12 12 17 22 12" />
        </svg>
      </button>

      {/* Panel */}
      {open && (
        <div
          id="overlay-picker-panel"
          role="region"
          aria-labelledby={titleId}
          className="w-64 rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white/95 dark:bg-neutral-800/95 backdrop-blur-sm shadow-lg overflow-hidden"
        >
          <div className="flex items-center justify-between px-3 py-2 border-b border-neutral-200 dark:border-neutral-700">
            <span
              id={titleId}
              className="text-xs font-semibold text-neutral-700 dark:text-neutral-200 uppercase tracking-wide"
            >
              Ebenen
            </span>
            <button
              onClick={() => setOpen(false)}
              aria-label="Schließen"
              className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
            >
              <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
              </svg>
            </button>
          </div>

          {isEmpty ? (
            <p className="px-3 py-3 text-xs text-neutral-500 dark:text-neutral-400">
              Keine Ebenen verfügbar.
            </p>
          ) : (
            <div className="divide-y divide-neutral-100 dark:divide-neutral-700 max-h-[60vh] overflow-y-auto [scrollbar-width:thin]">

              {/* ── Coverage composites ──────────────────────────────── */}
              <section aria-label="Analyse-Ebenen">
                <div className="px-3 pt-2 pb-0.5">
                  <span className="text-[10px] font-semibold text-neutral-400 dark:text-neutral-500 uppercase tracking-wider">
                    Analyse
                  </span>
                </div>
                {(Object.keys(COVERAGE_LABELS) as (keyof CoverageOverlayState)[]).map((cKey) => {
                  const composite = coverageState[cKey]
                  const rowId = `cov-${cKey}`
                  return (
                    <div key={cKey} className="px-3 py-2 space-y-1.5">
                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id={rowId}
                          checked={composite.enabled}
                          onChange={(e) => onCoverageUpdate(cKey, { enabled: e.target.checked })}
                          className="w-3.5 h-3.5 rounded accent-accent-DEFAULT cursor-pointer"
                          aria-label={COVERAGE_LABELS[cKey]}
                        />
                        <label
                          htmlFor={rowId}
                          className="flex-1 text-xs text-neutral-700 dark:text-neutral-200 cursor-pointer select-none"
                        >
                          {COVERAGE_LABELS[cKey]}
                        </label>
                        <span className="text-[10px] text-neutral-400 tabular-nums w-8 text-right">
                          {composite.opacity}%
                        </span>
                      </div>
                      <OpacitySlider
                        value={composite.opacity}
                        onChange={(v) => onCoverageUpdate(cKey, { opacity: v })}
                        labelId={rowId}
                        disabled={!composite.enabled}
                      />
                    </div>
                  )
                })}
              </section>

              {/* ── System overlays ──────────────────────────────────── */}
              {hasSysOverlays && (
                <section aria-label="System-Ebenen">
                  <div className="px-3 pt-2 pb-0.5">
                    <span className="text-[10px] font-semibold text-neutral-400 dark:text-neutral-500 uppercase tracking-wider">
                      System
                    </span>
                  </div>
                  {systemOverlays.map((layer) => {
                    const isVectorStyle = layer.source_kind === 'vector_style'
                    const enabled = systemOverlayEnabled[layer.key] ?? false
                    const opacity = systemOverlayOpacity[layer.key] ?? layer.default_opacity
                    const rowId = `sysov-${layer.key}`
                    return (
                      <div key={layer.key} className="px-3 py-2 space-y-1.5">
                        <div className="flex items-center gap-2">
                          <input
                            type="checkbox"
                            id={rowId}
                            checked={!isVectorStyle && enabled}
                            disabled={isVectorStyle}
                            onChange={(e) => onToggleSystemOverlay(layer.key, e.target.checked)}
                            className="w-3.5 h-3.5 rounded accent-accent-DEFAULT cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
                            aria-label={layer.name}
                          />
                          <label
                            htmlFor={rowId}
                            className={`flex-1 text-xs cursor-pointer select-none ${
                              isVectorStyle
                                ? 'text-neutral-400 dark:text-neutral-500 line-through'
                                : 'text-neutral-700 dark:text-neutral-200'
                            }`}
                            title={isVectorStyle ? 'Nur als Basiskarte verfügbar' : undefined}
                          >
                            {layer.name}
                          </label>
                          {!isVectorStyle && (
                            <span className="text-[10px] text-neutral-400 tabular-nums w-8 text-right">
                              {opacity}%
                            </span>
                          )}
                          {isVectorStyle && (
                            <span className="text-[10px] text-neutral-400 italic">Karte</span>
                          )}
                        </div>
                        {!isVectorStyle && (
                          <OpacitySlider
                            value={opacity}
                            onChange={(v) => onSetSystemOverlayOpacity(layer.key, v)}
                            labelId={rowId}
                            disabled={!enabled}
                          />
                        )}
                      </div>
                    )
                  })}
                </section>
              )}

              {/* ── User overlays ─────────────────────────────────────── */}
              {hasUserOverlays && (
                <section aria-label="Eigene Ebenen">
                  <div className="px-3 pt-2 pb-0.5">
                    <span className="text-[10px] font-semibold text-neutral-400 dark:text-neutral-500 uppercase tracking-wider">
                      Eigene
                    </span>
                  </div>
                  {userOverlays.map((ul) => {
                    const rowId = `userov-${ul.id}`
                    return (
                      <div key={ul.id} className="px-3 py-2 space-y-1.5">
                        <div className="flex items-center gap-2">
                          <input
                            type="checkbox"
                            id={rowId}
                            checked={ul.enabled}
                            onChange={(e) => onPatchUserLayer(ul.id, { enabled: e.target.checked })}
                            className="w-3.5 h-3.5 rounded accent-accent-DEFAULT cursor-pointer"
                            aria-label={ul.name}
                          />
                          <label
                            htmlFor={rowId}
                            className="flex-1 text-xs text-neutral-700 dark:text-neutral-200 cursor-pointer select-none"
                          >
                            {ul.name}
                          </label>
                          <span className="text-[10px] text-neutral-400 tabular-nums w-8 text-right">
                            {ul.default_opacity}%
                          </span>
                        </div>
                        <OpacitySlider
                          value={ul.default_opacity}
                          onChange={(v) => onPatchUserLayer(ul.id, { default_opacity: v })}
                          labelId={rowId}
                          disabled={!ul.enabled}
                        />
                      </div>
                    )
                  })}
                </section>
              )}

            </div>
          )}
        </div>
      )}
    </div>
  )
}
