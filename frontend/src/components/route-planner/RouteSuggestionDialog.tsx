import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  suggestRoute,
  type RouteSuggestionParams,
  type RouteSuggestionResponse,
} from '@/api/routes'
import { fetchAllProfiles, type RoutingProfileEntry } from '@/api/routingProfiles'
import { useAccentColor } from '@/lib/accentColor'
import { useToast } from '@/lib/toast'
import { ModalShell } from '@/components/ui/ModalShell'
import Toggle from '@/components/Toggle'

// Sport options for route planning (GPS-tracked outdoor sports)
const ROUTE_SPORT_OPTIONS = [
  { value: 'cycling', label: 'Radfahren' },
  { value: 'mountain_biking', label: 'MTB' },
  { value: 'gravel_cycling', label: 'Gravel' },
  { value: 'running', label: 'Laufen' },
  { value: 'trail_running', label: 'Trail Running' },
  { value: 'hiking', label: 'Wandern' },
  { value: 'mountaineering', label: 'Bergsteigen' },
  { value: 'walking', label: 'Gehen' },
] as const

const SURFACE_OPTIONS = [
  { value: 'trails', label: 'Naturwege / Singletrail' },
  { value: 'forest_roads', label: 'Forstwege / Schotter' },
  { value: 'mixed', label: 'Gemischt' },
  { value: 'paved', label: 'Asphalt' },
] as const

const SAC_SCALE_OPTIONS = [
  { value: 0, label: 'Keine Einschränkung' },
  { value: 1, label: 'T1 — Wandern' },
  { value: 2, label: 'T2 — Bergwandern' },
  { value: 3, label: 'T3 — Anspruchsvoll' },
  { value: 4, label: 'T4 — Alpinwandern' },
  { value: 5, label: 'T5 — Anspruchsvolles Alpinwandern' },
  { value: 6, label: 'T6 — Schwieriges Alpinwandern' },
]

const MTB_SCALE_OPTIONS = [
  { value: -1, label: 'Keine Einschränkung' },
  { value: 0, label: 'S0 — Keine technischen Anforderungen' },
  { value: 1, label: 'S1 — Leichte technische Anforderungen' },
  { value: 2, label: 'S2 — Mittlere technische Anforderungen' },
  { value: 3, label: 'S3 — Schwierige technische Anforderungen' },
  { value: 4, label: 'S4 — Sehr schwierige Anforderungen' },
  { value: 5, label: 'S5 — Extrem schwierige Anforderungen' },
]

// Profiles that indicate hiking/trekking base for SAC display
const HIKING_BASE_PROFILES = new Set(['hiking', 'trekking', 'mountaineering', 'walking'])
const MTB_BASE_PROFILES = new Set(['mtb', 'mountain_biking'])

interface PreferenceSliderProps {
  label: string
  value: number
  onChange: (v: number) => void
  accentColor: string
}

function PreferenceSlider({ label, value, onChange, accentColor }: PreferenceSliderProps) {
  return (
    <div className="flex items-center gap-3">
      <span className="w-36 text-xs text-neutral-600 dark:text-neutral-400 shrink-0">{label}</span>
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
  /** Called with the returned route suggestion response */
  onResult: (response: RouteSuggestionResponse) => void
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

  // Sport + Profile selection
  const [sportType, setSportType] = useState('cycling')
  const [selectedProfile, setSelectedProfile] = useState<string>('')

  // Round-trip + Terra Incognita toggles
  const [roundTrip, setRoundTrip] = useState(true)
  const [preferUnknown, setPreferUnknown] = useState(false)

  // Landscape preferences (0–10)
  const [forest, setForest] = useState(5)
  const [water, setWater] = useState(3)
  const [protectedArea, setProtectedArea] = useState(3)
  const [lowNoise, setLowNoise] = useState(5)
  const [lowTraffic, setLowTraffic] = useState(5)

  // Surface tendency
  const [surfaceTendency, setSurfaceTendency] = useState<'trails' | 'forest_roads' | 'mixed' | 'paved'>('mixed')

  // Difficulty scales (0 = no restriction for SAC, -1 = no restriction for MTB)
  const [maxSacScale, setMaxSacScale] = useState<number>(0)
  const [maxMtbScale, setMaxMtbScale] = useState<number>(-1)

  // Track last coverage_available for badge display
  const [lastCoverageAvailable, setLastCoverageAvailable] = useState<boolean | null>(null)

  // Load routing profiles
  const { data: profilesData } = useQuery({
    queryKey: ['routing-profiles'],
    queryFn: fetchAllProfiles,
    staleTime: 60_000,
  })

  // Filter profiles by selected sport
  const filteredProfiles: RoutingProfileEntry[] = (profilesData?.profiles ?? []).filter(p =>
    p.sport_types.includes(sportType)
  )

  // Auto-select first matching profile when sport changes (or when profiles load)
  const effectiveProfile = (() => {
    if (filteredProfiles.length === 0) return selectedProfile
    if (selectedProfile && filteredProfiles.some(p => p.name === selectedProfile)) return selectedProfile
    return filteredProfiles[0]?.name ?? ''
  })()

  // Determine base_profile for current selection (for SAC/MTB scale display)
  const selectedProfileEntry = filteredProfiles.find(p => p.name === effectiveProfile)
  const baseProfile = selectedProfileEntry?.base_profile ?? ''
  const showSacScale = HIKING_BASE_PROFILES.has(baseProfile)
  const showMtbScale = MTB_BASE_PROFILES.has(baseProfile)

  const handleSportChange = (sport: string) => {
    setSportType(sport)
    setSelectedProfile('') // reset so auto-select kicks in
  }

  const handleDistanceSlider = (v: number) => {
    setDistanceKm(v)
    setDistanceInput(String(v))
  }

  const handleDistanceInput = (raw: string) => {
    setDistanceInput(raw)
    const n = parseFloat(raw)
    if (!isNaN(n) && n >= 1 && n <= 50) setDistanceKm(Math.round(n))
  }

  const mutation = useMutation({
    mutationFn: () => {
      const params: RouteSuggestionParams = {
        start,
        round_trip: roundTrip,
        target_distance_m: distanceKm * 1000,
        sport_type: sportType,
        profile: effectiveProfile,
        prefer_unknown: preferUnknown || undefined,
        landscape_prefs: {
          forest,
          water,
          protected: protectedArea,
          low_noise: lowNoise,
          low_traffic: lowTraffic,
        },
        surface_tendency: surfaceTendency,
        max_sac_scale: showSacScale && maxSacScale > 0 ? maxSacScale : undefined,
        max_mtb_scale: showMtbScale && maxMtbScale >= 0 ? maxMtbScale : undefined,
      }
      return suggestRoute(params)
    },
    onSuccess: (response) => {
      setLastCoverageAvailable(response.coverage_available)
      onResult(response)
      // onClose() removed — MapPage handles dialog visibility via onResult
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

  const buttonLabel = mutation.isPending
    ? 'Wird berechnet\u2026'
    : 'Routen vorschlagen'

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="p-0 w-[92vw] max-h-[90vh] flex flex-col">
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
            Routen werden ab deinem aktuellen Standort berechnet
          </p>
        </div>
        <button
          onClick={onClose}
          className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 transition-colors cursor-pointer"
          aria-label="Dialog schließen"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      {/* Scrollable body */}
      <div className="overflow-y-auto flex-1 px-5 py-4 space-y-5">

        {/* Sportart */}
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
            onChange={e => handleSportChange(e.target.value)}
            className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
            style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
          >
            {ROUTE_SPORT_OPTIONS.map(opt => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>

        {/* Profil */}
        <div>
          <label
            htmlFor="rsd-profile"
            className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
          >
            Routing-Profil
          </label>
          {filteredProfiles.length === 0 ? (
            <p className="text-xs text-neutral-400 dark:text-neutral-500 italic">
              Keine Profile für diese Sportart gefunden
            </p>
          ) : (
            <select
              id="rsd-profile"
              value={effectiveProfile}
              onChange={e => setSelectedProfile(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            >
              <optgroup label="BadRouter">
                {filteredProfiles.filter(p => p.engine !== 'brouter').map(p => (
                  <option key={p.name} value={p.name}>
                    {p.name}{p.source === 'user' ? ' (eigenes Profil)' : ''}
                  </option>
                ))}
              </optgroup>
              {filteredProfiles.some(p => p.engine === 'brouter') && (
                <optgroup label="BRouter">
                  {filteredProfiles.filter(p => p.engine === 'brouter').map(p => (
                    <option key={p.name} value={p.name}>
                      {p.name}{p.source === 'user' ? ' (eigenes Profil)' : ''}
                    </option>
                  ))}
                </optgroup>
              )}
            </select>
          )}
        </div>

        {/* Distanz */}
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

        {/* Toggles */}
        <div className="space-y-3">
          <label className="flex items-center justify-between gap-3 cursor-pointer group">
            <div>
              <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block">
                Rundtour
              </span>
              <span className="text-xs text-neutral-500 dark:text-neutral-400">
                Start- und Endpunkt sind identisch
              </span>
            </div>
            <Toggle checked={roundTrip} onChange={setRoundTrip} size="sm" aria-label="Rundtour" />
          </label>

          <label className="flex items-center justify-between gap-3 cursor-pointer group">
            <div>
              <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block">
                Terra Incognita bevorzugen
              </span>
              <span className="text-xs text-neutral-500 dark:text-neutral-400">
                Unbekannte Wege und Gebiete werden stärker gewichtet
              </span>
            </div>
            <Toggle checked={preferUnknown} onChange={setPreferUnknown} size="sm" aria-label="Terra Incognita bevorzugen" />
          </label>
        </div>

        {/* Landschaft */}
        <div>
          <div className="flex items-center gap-2 mb-2">
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Landschaft
            </h3>
            {lastCoverageAvailable === false && (
              <span className="text-[10px] text-neutral-400 dark:text-neutral-500 italic">
                (keine Coverage-Daten — Landschafts-Präferenzen ohne Effekt)
              </span>
            )}
          </div>
          <div className="space-y-2.5">
            <PreferenceSlider label="Wald bevorzugen" value={forest} onChange={setForest} accentColor={accent.shade500} />
            <PreferenceSlider label="Wasser / Gewässer" value={water} onChange={setWater} accentColor={accent.shade500} />
            <PreferenceSlider label="Schutzgebiete" value={protectedArea} onChange={setProtectedArea} accentColor={accent.shade500} />
            <PreferenceSlider label="Lärm vermeiden" value={lowNoise} onChange={setLowNoise} accentColor={accent.shade500} />
            <PreferenceSlider label="Verkehr vermeiden" value={lowTraffic} onChange={setLowTraffic} accentColor={accent.shade500} />
          </div>
        </div>

        {/* Untergrund */}
        <div>
          <label
            htmlFor="rsd-surface"
            className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
          >
            Untergrund
          </label>
          <select
            id="rsd-surface"
            value={surfaceTendency}
            onChange={e => setSurfaceTendency(e.target.value as typeof surfaceTendency)}
            className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
            style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
          >
            {SURFACE_OPTIONS.map(opt => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>

        {/* SAC-Skala (nur für Wandern/Trekking) */}
        {showSacScale && (
          <div>
            <label
              htmlFor="rsd-sac-scale"
              className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
            >
              Maximale SAC-Skala (Schwierigkeit)
            </label>
            <select
              id="rsd-sac-scale"
              value={maxSacScale}
              onChange={e => setMaxSacScale(Number(e.target.value))}
              className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            >
              {SAC_SCALE_OPTIONS.map(opt => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>
        )}

        {/* MTB-Skala (nur für MTB) */}
        {showMtbScale && (
          <div>
            <label
              htmlFor="rsd-mtb-scale"
              className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
            >
              Maximale MTB-Skala (technische Schwierigkeit)
            </label>
            <select
              id="rsd-mtb-scale"
              value={maxMtbScale}
              onChange={e => setMaxMtbScale(Number(e.target.value))}
              className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            >
              {MTB_SCALE_OPTIONS.map(opt => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>
        )}

      </div>

      {/* Footer */}
      <div className="shrink-0 px-5 py-4 border-t border-neutral-200 dark:border-neutral-800 flex gap-2 justify-end">
        <button
          onClick={onClose}
          className="px-4 py-2 rounded-md text-sm font-medium bg-neutral-100 hover:bg-neutral-200 dark:bg-neutral-800 dark:hover:bg-neutral-700 text-neutral-900 dark:text-neutral-50 transition-colors cursor-pointer"
        >
          Abbrechen
        </button>
        <button
          onClick={() => mutation.mutate()}
          disabled={mutation.isPending || filteredProfiles.length === 0}
          className="px-4 py-2 rounded-md text-sm font-medium text-white disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed hover:opacity-90 transition-opacity flex items-center gap-2"
          style={{ backgroundColor: accent.shade500 }}
        >
          {mutation.isPending ? (
            <>
              <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
              </svg>
              {buttonLabel}
            </>
          ) : (
            buttonLabel
          )}
        </button>
      </div>
    </ModalShell>
  )
}
