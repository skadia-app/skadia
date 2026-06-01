import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  suggestRoute,
  type RouteSuggestionParams,
  type RouteSuggestionResponse,
} from '@/api/routes'
import { fetchAllProfiles, type RoutingProfileEntry } from '@/api/routingProfiles'
import { useAccentColor } from '@/lib/accentColor'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import { modifiersMatch, predicatesMatch, type ActivityModifierState, type ModifierFilters } from '@/lib/sportPredicates'
import { routeSportOptions } from '@/lib/sportTaxonomy'
import { useToast } from '@/lib/toast'
import { distanceFromDisplayValue, distanceInputUnit, distanceToDisplayValue } from '@/lib/units'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { ModalShell } from '@/components/ui/ModalShell'
import Toggle from '@/components/Toggle'

const MIN_TARGET_DISTANCE_M = 1000
const MAX_TARGET_DISTANCE_M = 100_000
const DEFAULT_TARGET_DISTANCE_M = 20_000

const SURFACE_OPTIONS = [
  { value: 'trails', labelKey: 'routePlanner.suggestion.surface.trails' },
  { value: 'forest_roads', labelKey: 'routePlanner.suggestion.surface.forestRoads' },
  { value: 'mixed', labelKey: 'routePlanner.suggestion.surface.mixed' },
  { value: 'paved', labelKey: 'routePlanner.suggestion.surface.paved' },
] as const

const SAC_SCALE_OPTIONS = [
  { value: 0, labelKey: 'routePlanner.suggestion.sac.none' },
  { value: 1, labelKey: 'routePlanner.suggestion.sac.t1' },
  { value: 2, labelKey: 'routePlanner.suggestion.sac.t2' },
  { value: 3, labelKey: 'routePlanner.suggestion.sac.t3' },
  { value: 4, labelKey: 'routePlanner.suggestion.sac.t4' },
  { value: 5, labelKey: 'routePlanner.suggestion.sac.t5' },
  { value: 6, labelKey: 'routePlanner.suggestion.sac.t6' },
]

const MTB_SCALE_OPTIONS = [
  { value: -1, labelKey: 'routePlanner.suggestion.mtb.none' },
  { value: 0, labelKey: 'routePlanner.suggestion.mtb.s0' },
  { value: 1, labelKey: 'routePlanner.suggestion.mtb.s1' },
  { value: 2, labelKey: 'routePlanner.suggestion.mtb.s2' },
  { value: 3, labelKey: 'routePlanner.suggestion.mtb.s3' },
  { value: 4, labelKey: 'routePlanner.suggestion.mtb.s4' },
  { value: 5, labelKey: 'routePlanner.suggestion.mtb.s5' },
]

// Profiles that indicate hiking/trekking base for SAC display
const HIKING_BASE_PROFILES = new Set(['hiking', 'trekking', 'mountaineering', 'walking'])

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

function isMtbProfile(profile: RoutingProfileEntry): boolean {
  if (profile.base_profile === 'mtb') return true
  if (profile.predicates?.some(p => p.sport === 'cycling' && p.sub_type === 'mountain')) return true
  return false
}

export function requiredCyclingSubTypeForBroadSelection(profile: RoutingProfileEntry): string | null {
  if (profile.predicates?.some(p => p.sport === 'cycling' && p.sub_type === '')) return null
  const pred = profile.predicates?.find(p => p.sport === 'cycling' && p.sub_type && p.sub_type !== '')
  return pred?.sub_type ?? null
}

export interface RouteSuggestionDialogProps {
  /** Start coordinate as [lng, lat] — either first planner waypoint or current map center */
  start: [number, number]
  /** Optional label shown in header when start is a user-placed waypoint (falls back to map-position wording otherwise) */
  startLabel?: string
  onClose: () => void
  /** Called with the returned route suggestion response */
  onResult: (response: RouteSuggestionResponse) => void
}

export default function RouteSuggestionDialog({
  start,
  startLabel,
  onClose,
  onResult,
}: RouteSuggestionDialogProps) {
  const { t, i18n } = useTranslation()
  const { accent } = useAccentColor()
  const { addToast } = useToast()
  const { options: unitOptions } = useUnitFormatters()
  const language = supportedLanguageFrom(i18n.language)
  const routeSportOptionsForLanguage = routeSportOptions(language)
  const targetDistanceUnit = distanceInputUnit(unitOptions.units)
  const distanceDisplayDigits = unitOptions.units === 'imperial' ? 3 : 0
  const distanceStep = unitOptions.units === 'imperial' ? 0.001 : 1
  const formatDistanceInputValue = (meters: number) => distanceToDisplayValue(meters, unitOptions.units).toFixed(distanceDisplayDigits)
  const minTargetDistance = formatDistanceInputValue(MIN_TARGET_DISTANCE_M)
  const maxTargetDistance = formatDistanceInputValue(MAX_TARGET_DISTANCE_M)

  // Distance is stored in meters and converted only for user-facing controls.
  const [distanceM, setDistanceM] = useState(DEFAULT_TARGET_DISTANCE_M)
  const [distanceInput, setDistanceInput] = useState(() => formatDistanceInputValue(DEFAULT_TARGET_DISTANCE_M))

  // Sport + Profile selection
  const [sportType, setSportType] = useState('cycling')
  const [subType, setSubType] = useState<string | undefined>(undefined)
  const [isElectric, setIsElectric] = useState(false)
  const [selectedProfile, setSelectedProfile] = useState<string>('')
  const [profileSportOverride, setProfileSportOverride] = useState(false)

  // Terra Incognita toggle
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

  useEffect(() => {
    setDistanceInput(formatDistanceInputValue(distanceM))
  }, [distanceM, unitOptions.units, distanceDisplayDigits])

  // Load routing profiles
  const { data: profilesData } = useQuery({
    queryKey: ['routing-profiles'],
    queryFn: fetchAllProfiles,
    staleTime: 60_000,
  })

  const modifierState: ActivityModifierState = {
    is_electric: isElectric,
    is_indoor: false,
    is_virtual: false,
    is_commute: false,
  }
  const requestModifiers: ModifierFilters | undefined =
    isElectric
      ? { is_electric: true }
      : undefined

  // Filter profiles by selected sport tuple.
  const allProfiles = profilesData?.profiles.filter(p => p.engine !== 'brouter') ?? []
  const strictProfiles: RoutingProfileEntry[] = allProfiles.filter(p => {
    const predicateMatch = p.predicates
      ? predicatesMatch(p.predicates, sportType, subType)
      : p.sport_types.includes(sportType)
    return predicateMatch && modifiersMatch(p.modifier_filters, modifierState)
  })
  const advisoryProfiles: RoutingProfileEntry[] = sportType === 'cycling' && !subType
    ? allProfiles.filter(p =>
        !strictProfiles.some(strict => strict.name === p.name) &&
        requiredCyclingSubTypeForBroadSelection(p) !== null &&
        modifiersMatch(p.modifier_filters, modifierState),
      )
    : []
  const filteredProfiles = [...strictProfiles, ...advisoryProfiles]

  // Auto-select first matching profile when sport changes (or when profiles load)
  const effectiveProfile = (() => {
    if (filteredProfiles.length === 0) return selectedProfile
    if (selectedProfile && filteredProfiles.some(p => p.name === selectedProfile)) return selectedProfile
    return filteredProfiles[0]?.name ?? ''
  })()

  // Determine base_profile for current selection (for SAC/MTB scale display)
  const selectedProfileEntry = filteredProfiles.find(p => p.name === effectiveProfile)
  const selectedProfileRequiresCyclingSubType = selectedProfileEntry ? requiredCyclingSubTypeForBroadSelection(selectedProfileEntry) : null
  const profileNeedsBroadCyclingDecision = sportType === 'cycling' && !subType && selectedProfileRequiresCyclingSubType !== null
  const broadCyclingDecisionIsMountain = selectedProfileRequiresCyclingSubType === 'mountain'
  const baseProfile = selectedProfileEntry?.base_profile ?? ''
  const showSacScale = HIKING_BASE_PROFILES.has(baseProfile)
  const showMtbScale = selectedProfileEntry ? isMtbProfile(selectedProfileEntry) : false

  const selectedSportKey = `${sportType}::${subType ?? ''}`

  const handleSportChange = (key: string) => {
    const [sport, sub = ''] = key.split('::')
    setSportType(sport)
    setSubType(sub || undefined)
    if (sport !== 'cycling') setIsElectric(false)
    setProfileSportOverride(false)
    setSelectedProfile('') // reset so auto-select kicks in
  }

  const handleDistanceSlider = (v: number) => {
    const meters = Math.round(distanceFromDisplayValue(v, unitOptions.units))
    const clamped = Math.max(MIN_TARGET_DISTANCE_M, Math.min(MAX_TARGET_DISTANCE_M, meters))
    setDistanceM(clamped)
    setDistanceInput(formatDistanceInputValue(clamped))
  }

  const handleDistanceInput = (raw: string) => {
    setDistanceInput(raw)
    const n = parseFloat(raw)
    if (!isNaN(n) && n >= Number(minTargetDistance) && n <= Number(maxTargetDistance)) {
      const meters = Math.round(distanceFromDisplayValue(n, unitOptions.units))
      setDistanceM(Math.max(MIN_TARGET_DISTANCE_M, Math.min(MAX_TARGET_DISTANCE_M, meters)))
    }
  }

  const mutation = useMutation({
    mutationFn: () => {
      const params: RouteSuggestionParams = {
        start,
        round_trip: true,
        target_distance_m: distanceM,
        sport_type: sportType,
        sub_type: subType,
        modifiers: requestModifiers,
        profile: effectiveProfile,
        profile_sport_override: profileSportOverride || undefined,
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
        title: t('routePlanner.suggestion.toastFailed'),
        message: String(err),
        duration: 0,
      })
    },
  })

  const buttonLabel = mutation.isPending
    ? t('routePlanner.suggestion.calculating')
    : t('routePlanner.suggestion.submit')

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md" className="p-0 w-[92vw] max-h-[90vh] flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-5 pt-5 pb-3 border-b border-neutral-200 dark:border-neutral-800 shrink-0">
        <div>
          <h2
            id="route-suggestion-title"
            className="text-base font-semibold text-neutral-900 dark:text-neutral-50"
          >
            {t('routePlanner.suggestion.title')}
          </h2>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {startLabel
              ? t('routePlanner.suggestion.startWithLabel', { label: startLabel })
              : t('routePlanner.suggestion.startFromMap')}
          </p>
        </div>
        <button
          onClick={onClose}
          className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 transition-colors cursor-pointer"
          aria-label={t('routePlanner.suggestion.closeDialog')}
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
            {t('routePlanner.suggestion.sport')}
          </label>
          <select
            id="rsd-sport"
            value={selectedSportKey}
            onChange={e => handleSportChange(e.target.value)}
            className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
            style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
          >
            {routeSportOptionsForLanguage.map(opt => (
              <option key={`${opt.sportType}::${opt.subType ?? ''}`} value={`${opt.sportType}::${opt.subType ?? ''}`}>{opt.label}</option>
            ))}
          </select>
        </div>

        {sportType === 'cycling' && (
          <div className="grid grid-cols-2 gap-3">
            <label className="flex items-center justify-between gap-3 cursor-pointer group">
              <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('routePlanner.suggestion.eBike')}</span>
              <Toggle checked={isElectric} onChange={setIsElectric} size="sm" aria-label={t('routePlanner.suggestion.eBike')} />
            </label>
          </div>
        )}

        {/* Profile */}
        <div>
          <label
            htmlFor="rsd-profile"
            className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
          >
            {t('routePlanner.suggestion.routingProfile')}
          </label>
          {filteredProfiles.length === 0 ? (
            <p className="text-xs text-neutral-400 dark:text-neutral-500 italic">
              {t('routePlanner.suggestion.noProfiles')}
            </p>
          ) : (
            <select
              id="rsd-profile"
              value={effectiveProfile}
              onChange={e => {
                setSelectedProfile(e.target.value)
                setProfileSportOverride(false)
              }}
              className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            >
              <optgroup label="BadRouter">
                {filteredProfiles.filter(p => p.engine !== 'brouter').map(p => (
                  <option key={p.name} value={p.name}>
                    {p.name}{p.source === 'user' ? ` (${t('routePlanner.suggestion.customProfile')})` : ''}
                  </option>
                ))}
              </optgroup>
              {filteredProfiles.some(p => p.engine === 'brouter') && (
                <optgroup label="BRouter">
                  {filteredProfiles.filter(p => p.engine === 'brouter').map(p => (
                    <option key={p.name} value={p.name}>
                      {p.name}{p.source === 'user' ? ` (${t('routePlanner.suggestion.customProfile')})` : ''}
                    </option>
                  ))}
                </optgroup>
              )}
            </select>
          )}
        </div>

        {profileNeedsBroadCyclingDecision && (
          <div className="rounded-lg border border-amber-300 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-950/30 p-3 space-y-2">
            <p className="text-sm text-amber-900 dark:text-amber-200">
              {t('routePlanner.suggestion.profileDecision.noSubtype')}<br />
              {broadCyclingDecisionIsMountain
                ? t('routePlanner.suggestion.profileDecision.mtbRequiresMountain')
                : t('routePlanner.suggestion.profileDecision.profileRequiresSubtype', { subType: selectedProfileRequiresCyclingSubType })}
            </p>
            <div className="grid gap-1.5">
              <button
                type="button"
                onClick={() => {
                  setSubType(selectedProfileRequiresCyclingSubType || undefined)
                  setProfileSportOverride(false)
                }}
                className="text-left rounded-md border border-amber-300 dark:border-amber-800 px-2 py-1.5 text-xs text-amber-900 dark:text-amber-100 hover:bg-amber-100 dark:hover:bg-amber-900/40 cursor-pointer"
              >
                {t('routePlanner.suggestion.profileDecision.useSubtype', { subType: selectedProfileRequiresCyclingSubType })}
              </button>
              <button
                type="button"
                onClick={() => {
                  setSelectedProfile('trekking')
                  setProfileSportOverride(false)
                }}
                className="text-left rounded-md border border-amber-300 dark:border-amber-800 px-2 py-1.5 text-xs text-amber-900 dark:text-amber-100 hover:bg-amber-100 dark:hover:bg-amber-900/40 cursor-pointer"
              >
                {t('routePlanner.suggestion.profileDecision.useTrekking')}
              </button>
              <button
                type="button"
                onClick={() => setProfileSportOverride(true)}
                className={`text-left rounded-md border px-2 py-1.5 text-xs cursor-pointer ${
                  profileSportOverride
                    ? 'border-accent-DEFAULT bg-accent-DEFAULT text-white'
                    : 'border-amber-300 dark:border-amber-800 text-amber-900 dark:text-amber-100 hover:bg-amber-100 dark:hover:bg-amber-900/40'
                }`}
              >
                {broadCyclingDecisionIsMountain
                  ? t('routePlanner.suggestion.profileDecision.overrideMtb')
                  : t('routePlanner.suggestion.profileDecision.override')}
              </button>
            </div>
          </div>
        )}

        {/* Distance */}
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <label
              htmlFor="rsd-distance-input"
              className="text-sm font-medium text-neutral-700 dark:text-neutral-300"
            >
              {t('routePlanner.suggestion.targetDistance')}
            </label>
            <div className="flex items-center gap-1.5">
              <input
                id="rsd-distance-input"
                type="number"
                min={minTargetDistance}
                max={maxTargetDistance}
                step={distanceStep}
                value={distanceInput}
                onChange={e => handleDistanceInput(e.target.value)}
                className="w-16 px-2 py-1 rounded border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-right text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-1"
                style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
              />
              <span className="text-xs text-neutral-500 dark:text-neutral-400">{targetDistanceUnit}</span>
            </div>
          </div>
          <input
            type="range"
            min={minTargetDistance}
            max={maxTargetDistance}
            step={distanceStep}
            value={formatDistanceInputValue(distanceM)}
            onChange={e => handleDistanceSlider(Number(e.target.value))}
            className="w-full h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--slider-accent)]"
            style={{ '--slider-accent': accent.shade500 } as React.CSSProperties}
            aria-label={t('routePlanner.suggestion.targetDistanceSlider')}
          />
          <div className="flex justify-between text-[10px] text-neutral-400 dark:text-neutral-600 mt-0.5">
            <span>{minTargetDistance} {targetDistanceUnit}</span>
            <span>{maxTargetDistance} {targetDistanceUnit}</span>
          </div>
        </div>

        {/* Toggles */}
        <div className="space-y-3">
          <label className="flex items-center justify-between gap-3 cursor-pointer group">
            <div>
              <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block">
                {t('routePlanner.suggestion.preferUnknown')}
              </span>
              <span className="text-xs text-neutral-500 dark:text-neutral-400">
                {t('routePlanner.suggestion.preferUnknownDescription')}
              </span>
            </div>
            <Toggle checked={preferUnknown} onChange={setPreferUnknown} size="sm" aria-label={t('routePlanner.suggestion.preferUnknown')} />
          </label>
        </div>

        {/* Landscape */}
        <div>
          <div className="flex items-center gap-2 mb-2">
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {t('routePlanner.suggestion.landscape')}
            </h3>
            {lastCoverageAvailable === false && (
              <span className="text-[10px] text-neutral-400 dark:text-neutral-500 italic">
                {t('routePlanner.suggestion.noCoverageEffect')}
              </span>
            )}
          </div>
          <div className="space-y-2.5">
            <PreferenceSlider label={t('routePlanner.suggestion.landscapePrefs.forest')} value={forest} onChange={setForest} accentColor={accent.shade500} />
            <PreferenceSlider label={t('routePlanner.suggestion.landscapePrefs.water')} value={water} onChange={setWater} accentColor={accent.shade500} />
            <PreferenceSlider label={t('routePlanner.suggestion.landscapePrefs.protectedArea')} value={protectedArea} onChange={setProtectedArea} accentColor={accent.shade500} />
            <PreferenceSlider label={t('routePlanner.suggestion.landscapePrefs.lowNoise')} value={lowNoise} onChange={setLowNoise} accentColor={accent.shade500} />
            <PreferenceSlider label={t('routePlanner.suggestion.landscapePrefs.lowTraffic')} value={lowTraffic} onChange={setLowTraffic} accentColor={accent.shade500} />
          </div>
        </div>

        {/* Surface */}
        <div>
          <label
            htmlFor="rsd-surface"
            className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
          >
            {t('routePlanner.suggestion.surfaceLabel')}
          </label>
          <select
            id="rsd-surface"
            value={surfaceTendency}
            onChange={e => setSurfaceTendency(e.target.value as typeof surfaceTendency)}
            className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
            style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
          >
            {SURFACE_OPTIONS.map(opt => (
              <option key={opt.value} value={opt.value}>{t(opt.labelKey)}</option>
            ))}
          </select>
        </div>

        {/* SAC scale for hiking profiles */}
        {showSacScale && (
          <div>
            <label
              htmlFor="rsd-sac-scale"
              className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
            >
              {t('routePlanner.suggestion.maxSacScale')}
            </label>
            <select
              id="rsd-sac-scale"
              value={maxSacScale}
              onChange={e => setMaxSacScale(Number(e.target.value))}
              className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            >
              {SAC_SCALE_OPTIONS.map(opt => (
                <option key={opt.value} value={opt.value}>{t(opt.labelKey)}</option>
              ))}
            </select>
          </div>
        )}

        {/* MTB scale for MTB profiles */}
        {showMtbScale && (
          <div>
            <label
              htmlFor="rsd-mtb-scale"
              className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1.5"
            >
              {t('routePlanner.suggestion.maxMtbScale')}
            </label>
            <select
              id="rsd-mtb-scale"
              value={maxMtbScale}
              onChange={e => setMaxMtbScale(Number(e.target.value))}
              className="w-full px-3 py-2 rounded-md border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm text-neutral-900 dark:text-neutral-50 focus:outline-none focus:ring-2"
              style={{ '--tw-ring-color': accent.shade400 } as React.CSSProperties}
            >
              {MTB_SCALE_OPTIONS.map(opt => (
                <option key={opt.value} value={opt.value}>{t(opt.labelKey)}</option>
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
          {t('routePlanner.suggestion.cancel')}
        </button>
        <button
          onClick={() => mutation.mutate()}
          disabled={mutation.isPending || filteredProfiles.length === 0 || (profileNeedsBroadCyclingDecision && !profileSportOverride)}
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
