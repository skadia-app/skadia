import { useState } from 'react'
import { useEscapeKey } from '@/hooks/useEscapeKey'
import { useMutation } from '@tanstack/react-query'
import { suggestRoute, type RouteSuggestionParams, type RouteSuggestion } from '@/api/routes'
import { useAccentColor } from '@/lib/accentColor'
import { useToast } from '@/lib/toast'

// Sport options relevant for route planning (GPS-tracked outdoor sports)
const ROUTE_SPORT_OPTIONS = [
  { value: 'cycling', label: 'Radfahren' },
  { value: 'mountain_biking', label: 'Mountainbike' },
  { value: 'gravel_cycling', label: 'Gravel' },
  { value: 'running', label: 'Laufen' },
  { value: 'trail_running', label: 'Trail Running' },
  { value: 'hiking', label: 'Wandern' },
  { value: 'mountaineering', label: 'Bergsteigen' },
  { value: 'walking', label: 'Gehen' },
]

// BadRouter profile map for sport types
const PROFILE_MAP: Record<string, string> = {
  cycling: 'trekking',
  mountain_biking: 'mtb',
  gravel_cycling: 'gravel',
  running: 'hiking',
  trail_running: 'hiking',
  hiking: 'hiking',
  mountaineering: 'hiking',
  walking: 'hiking',
}

interface PreferenceSliderProps {
  label: string
  value: number
  onChange: (v: number) => void
  accentColor: string
}

function PreferenceSlider({ label, value, onChange, accentColor }: PreferenceSliderProps) {
  return (
    <div className="flex items-center gap-3">
      <span className="w-28 text-xs text-neutral-600 dark:text-neutral-400 shrink-0">{label}</span>
      <input
        type="range"
        min={0}
        max={10}
        step={1}
        value={value}
        onChange={e => onChange(Number(e.target.value))}
        className="flex-1 h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--slider-accent)]"
        style={{ '--slider-accent': accentColor } as React.CSSProperties}
        aria-label={label}
      />
      <span className="w-5 text-right text-xs font-mono text-neutral-700 dark:text-neutral-300 shrink-0">
        {value}
      </span>
    </div>
  )
}

export interface RouteSuggestionDialogProps {
  /** Current map center — used as start point */
  start: [number, number]
  onClose: () => void
  /** Called with the returned route suggestions */
  onResult: (suggestions: RouteSuggestion[]) => void
}

export default function RouteSuggestionDialog({
  start,
  onClose,
  onResult,
}: RouteSuggestionDialogProps) {
  const { accent } = useAccentColor()
  const { addToast } = useToast()

  // Distance: 1–50 km, stored in km for UI, converted to m for API
  const [distanceKm, setDistanceKm] = useState(20)
  const [distanceInput, setDistanceInput] = useState('20')

  // Elevation: 100–3000 Hm
  const [elevationHm, setElevationHm] = useState(500)
  const [elevationInput, setElevationInput] = useState('500')

  // Sport
  const [sportType, setSportType] = useState('cycling')

  // Round-trip toggle
  const [roundTrip, setRoundTrip] = useState(true)

  // Terra Incognita
  const [preferUnknown, setPreferUnknown] = useState(false)

  // Landscape preferences (0–10)
  const [forest, setForest] = useState(5)
  const [water, setWater] = useState(3)
  const [meadow, setMeadow] = useState(3)

  // Surface preferences (0–10)
  const [trail, setTrail] = useState(5)
  const [forestRoad, setForestRoad] = useState(5)
  const [asphalt, setAsphalt] = useState(3)

  // Sync distance slider → text field
  const handleDistanceSlider = (v: number) => {
    setDistanceKm(v)
    setDistanceInput(String(v))
  }

  const handleDistanceInput = (raw: string) => {
    setDistanceInput(raw)
    const n = parseFloat(raw)
    if (!isNaN(n) && n >= 1 && n <= 50) setDistanceKm(Math.round(n))
  }

  // Sync elevation slider → text field
  const handleElevationSlider = (v: number) => {
    setElevationHm(v)
    setElevationInput(String(v))
  }

  const handleElevationInput = (raw: string) => {
    setElevationInput(raw)
    const n = parseInt(raw, 10)
    if (!isNaN(n) && n >= 100 && n <= 3000) setElevationHm(n)
  }

  useEscapeKey(onClose)

  const mutation = useMutation({
    mutationFn: () => {
      const effectiveLandscape = preferUnknown
        ? { forest: Math.min(10, forest + 3), water, meadow: Math.min(10, meadow + 3) }
        : { forest, water, meadow }

      const params: RouteSuggestionParams = {
        start,
        round_trip: roundTrip,
        target_distance_m: distanceKm * 1000,
        target_elevation_gain: elevationHm,
        sport_type: sportType,
        profile: PROFILE_MAP[sportType] ?? 'trekking',
        prefer_unknown: preferUnknown || undefined,
        landscape_prefs: effectiveLandscape,
        surface_prefs: {
          trail,
          forest_road: forestRoad,
          asphalt,
        },
      }
      return suggestRoute(params)
    },
    onSuccess: (result) => {
      onResult(result.suggestions)
      onClose()
    },
    onError: (err) => {
      addToast({
        type: 'error',
        title: 'Vorschlag fehlgeschlagen',
        message: String(err),
        duration: 0,
      })
    },
  })

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={onClose}
      role="presentation"
    >
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" />

      <div
        className="relative bg-white dark:bg-neutral-950 rounded-xl shadow-xl w-[92vw] max-w-md max-h-[90vh] flex flex-col"
        onClick={e => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="route-suggestion-title"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 pt-5 pb-3 border-b border-neutral-200 dark:border-neutral-800 shrink-0">
          <div>
            <h2
              id="route-suggestion-title"
              className="text-base font-semibold text-neutral-900 dark:text-neutral-50"
            >
              Route vorschlagen
            </h2>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              3 Routen werden ab deinem aktuellen Standort berechnet
            </p>
          </div>
          <button
            onClick={onClose}
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 transition-colors"
            aria-label="Dialog schließen"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Scrollable body */}
        <div className="overflow-y-auto flex-1 px-5 py-4 space-y-5">

          {/* Sport */}
          <div>
            <label
              htmlFor="rsd-sport"
              className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
            >
              Sportart
            </label>
            <select
              id="rsd-sport"
              value={sportType}
              onChange={e => setSportType(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            >
              {ROUTE_SPORT_OPTIONS.map(opt => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>

          {/* Distance */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label
                htmlFor="rsd-distance-input"
                className="text-sm font-medium text-neutral-700 dark:text-neutral-300"
              >
                Zieldistanz
              </label>
              <div className="flex items-center gap-1.5">
                <input
                  id="rsd-distance-input"
                  type="number"
                  min={1}
                  max={50}
                  step={1}
                  value={distanceInput}
                  onChange={e => handleDistanceInput(e.target.value)}
                  className="w-16 px-2 py-1 rounded border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-right text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-1"
                  style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
                />
                <span className="text-xs text-neutral-500 dark:text-neutral-400">km</span>
              </div>
            </div>
            <input
              type="range"
              min={1}
              max={50}
              step={1}
              value={distanceKm}
              onChange={e => handleDistanceSlider(Number(e.target.value))}
              className="w-full h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--slider-accent)]"
              style={{ '--slider-accent': accent.shade500 } as React.CSSProperties}
              aria-label="Zieldistanz Slider"
            />
            <div className="flex justify-between text-[10px] text-neutral-400 dark:text-neutral-600 mt-0.5">
              <span>1 km</span>
              <span>50 km</span>
            </div>
          </div>

          {/* Elevation */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label
                htmlFor="rsd-elevation-input"
                className="text-sm font-medium text-neutral-700 dark:text-neutral-300"
              >
                Höhenmeter
              </label>
              <div className="flex items-center gap-1.5">
                <input
                  id="rsd-elevation-input"
                  type="number"
                  min={100}
                  max={3000}
                  step={50}
                  value={elevationInput}
                  onChange={e => handleElevationInput(e.target.value)}
                  className="w-16 px-2 py-1 rounded border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-right text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-1"
                  style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
                />
                <span className="text-xs text-neutral-500 dark:text-neutral-400">Hm</span>
              </div>
            </div>
            <input
              type="range"
              min={100}
              max={3000}
              step={50}
              value={elevationHm}
              onChange={e => handleElevationSlider(Number(e.target.value))}
              className="w-full h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--slider-accent)]"
              style={{ '--slider-accent': accent.shade500 } as React.CSSProperties}
              aria-label="Höhenmeter Slider"
            />
            <div className="flex justify-between text-[10px] text-neutral-400 dark:text-neutral-600 mt-0.5">
              <span>100 Hm</span>
              <span>3000 Hm</span>
            </div>
          </div>

          {/* Toggles */}
          <div className="space-y-3">
            {/* Round-trip toggle */}
            <label className="flex items-center justify-between gap-3 cursor-pointer group">
              <div>
                <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block">
                  Rundtour
                </span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400">
                  Start- und Endpunkt sind identisch
                </span>
              </div>
              <button
                role="switch"
                aria-checked={roundTrip}
                onClick={() => setRoundTrip(v => !v)}
                className="relative inline-flex h-5 w-9 items-center rounded-full transition-colors shrink-0"
                style={{ backgroundColor: roundTrip ? accent.shade500 : undefined }}
                data-state={roundTrip ? 'checked' : 'unchecked'}
              >
                <span
                  className={`
                    inline-block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform
                    ${roundTrip ? 'translate-x-[18px]' : 'translate-x-[3px]'}
                    ${!roundTrip ? 'bg-neutral-300 dark:bg-neutral-600' : ''}
                  `}
                />
                {!roundTrip && (
                  <span className="absolute inset-0 rounded-full bg-neutral-300 dark:bg-neutral-700 -z-10" />
                )}
              </button>
            </label>

            {/* Terra Incognita toggle */}
            <label className="flex items-center justify-between gap-3 cursor-pointer group">
              <div>
                <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block">
                  Terra Incognita bevorzugen
                </span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400">
                  Unbekannte Wege und Gebiete werden stärker gewichtet
                </span>
              </div>
              <button
                role="switch"
                aria-checked={preferUnknown}
                onClick={() => setPreferUnknown(v => !v)}
                className="relative inline-flex h-5 w-9 items-center rounded-full transition-colors shrink-0"
                style={{ backgroundColor: preferUnknown ? accent.shade500 : undefined }}
                data-state={preferUnknown ? 'checked' : 'unchecked'}
              >
                <span
                  className={`
                    inline-block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform
                    ${preferUnknown ? 'translate-x-[18px]' : 'translate-x-[3px]'}
                    ${!preferUnknown ? 'bg-neutral-300 dark:bg-neutral-600' : ''}
                  `}
                />
                {!preferUnknown && (
                  <span className="absolute inset-0 rounded-full bg-neutral-300 dark:bg-neutral-700 -z-10" />
                )}
              </button>
            </label>
          </div>

          {/* Landscape preferences */}
          <div>
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
              Landschaft
            </h3>
            <div className="space-y-2.5">
              <PreferenceSlider label="Wald" value={forest} onChange={setForest} accentColor={accent.shade500} />
              <PreferenceSlider label="Wasser" value={water} onChange={setWater} accentColor={accent.shade500} />
              <PreferenceSlider label="Wiesen" value={meadow} onChange={setMeadow} accentColor={accent.shade500} />
            </div>
          </div>

          {/* Surface preferences */}
          <div>
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
              Untergrund
            </h3>
            <div className="space-y-2.5">
              <PreferenceSlider label="Trail" value={trail} onChange={setTrail} accentColor={accent.shade500} />
              <PreferenceSlider label="Forststraße" value={forestRoad} onChange={setForestRoad} accentColor={accent.shade500} />
              <PreferenceSlider label="Asphalt" value={asphalt} onChange={setAsphalt} accentColor={accent.shade500} />
            </div>
          </div>

        </div>

        {/* Footer */}
        <div className="shrink-0 px-5 py-4 border-t border-neutral-200 dark:border-neutral-800 flex gap-2 justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-md text-sm font-medium bg-neutral-100 hover:bg-neutral-200 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-900 dark:text-neutral-50 transition-colors"
          >
            Abbrechen
          </button>
          <button
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
            className="px-4 py-2 rounded-md text-sm font-medium text-white disabled:opacity-50 disabled:cursor-not-allowed hover:opacity-90 transition-opacity flex items-center gap-2"
            style={{ backgroundColor: accent.shade500 }}
          >
            {mutation.isPending ? (
              <>
                <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
                </svg>
                Wird berechnet…
              </>
            ) : (
              '3 Routen vorschlagen'
            )}
          </button>
        </div>
      </div>
    </div>
  )
}
