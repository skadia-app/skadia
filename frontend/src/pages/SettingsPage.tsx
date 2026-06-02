import { useState, useMemo, useEffect, useRef, useCallback } from 'react'
import { ChevronDown } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useUser } from '@/providers/UserProvider'
import { useAccentColor, ACCENT_PALETTE, DEFAULT_ACCENT_COLOR, deriveAccentEntry } from '@/lib/accentColor'
import { getCapabilities } from '@/api/routes'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import { CATEGORIES, SPORT_DEFS } from '@/lib/sportTaxonomy'
import ColorPicker from '@/components/ColorPicker'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useEnrichmentSettings } from '@/lib/useEnrichmentSettings'
import { Link } from 'react-router-dom'
import { getShadowFSSettings, updateShadowFSSettings, getHeatmapSettings, updateHeatmapSettings } from '@/api/settings'
import { getImportSettings, updateImportSettings, type ImportSettings, type ConflictStrategy } from '@/api/importWizard'
import { fetchSportTypeStats } from '@/api/stats'
import { backfillEnrichment, fetchGeocodingGap, startGeocodingBackfill } from '@/api/enrichment'
import { ApiError } from '@/api/client'
import { fetchActiveJobs } from '@/api/jobs'
import type { EnrichmentSettings } from '@/api/types'
import { recomputePhotoPreviews, recomputePhotoThumbnails, recomputePhotoEmbeddings, backfillPhash } from '@/api/activityPhotos'
import { useToast } from '@/lib/toast'
import FileSchemaSettings from '@/components/FileSchemaSettings'
import LLMSettingsPage from './LLMSettingsPage'
import ChatProfileSettings from '@/components/ChatProfileSettings'
import ChatWriteToolSettings from '@/components/ChatWriteToolSettings'
import { WeatherSettings } from '@/components/WeatherSettings'
import AccountSettingsTab from '@/pages/settings/AccountSettingsTab'
import AdminSettingsTab from '@/pages/settings/AdminSettingsTab'
import { MLServiceSection } from '@/pages/settings/MLServiceSection'
import { NotificationSettingsSection } from '@/pages/settings/NotificationSettingsSection'
import { UserCustomLayersSection } from '@/components/UserCustomLayersSection'
import { AutoTitleDisplayPolicySelect } from '@/components/AutoTitleDisplayPolicySelect'
import { useMapLayers } from '@/hooks/useMapLayers'
import Toggle from '@/components/Toggle'
import { SettingsSection } from '@/components/ui/SettingsSection'
import { ANALYSIS_MODULES } from '@/lib/enrichmentModuleCatalog'
import { featureAvailable, featureConfigured } from '@/lib/deploymentCapabilities'
import { useCapabilities } from '@/hooks/useCapabilities'
import { localizeString } from '@/i18n/localizedString'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import {
  distanceFromDisplayValue,
  distanceInputUnit,
  distanceToDisplayValue,
  elevationFromDisplayValue,
  elevationInputUnit,
  elevationToDisplayValue,
  type UnitSystem,
} from '@/lib/units'

function settingsConflictStrategyOptions(t: TFunction): Array<{ value: ConflictStrategy; label: string }> {
  return [
    { value: 'enrich_missing', label: t('settingsPage.import.conflict.enrichMissingRecommended') },
    { value: 'replace_all', label: t('settingsPage.import.conflict.replaceAlways') },
    { value: 'skip', label: t('settingsPage.import.conflict.skipAlways') },
  ]
}

function roundSettingsUnitValue(value: number): number {
  return Number(value.toFixed(3))
}

function settingDistanceDisplayValue(meters: number, units: UnitSystem): number {
  return roundSettingsUnitValue(distanceToDisplayValue(meters, units))
}

function settingDistanceStoredValue(value: string, units: UnitSystem): number | null {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return null
  return roundSettingsUnitValue(distanceFromDisplayValue(numeric, units))
}

function settingElevationDisplayValue(meters: number, units: UnitSystem): number {
  return roundSettingsUnitValue(elevationToDisplayValue(meters, units))
}

function settingElevationStoredValue(value: string, units: UnitSystem): number | null {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return null
  return roundSettingsUnitValue(elevationFromDisplayValue(numeric, units))
}

type ThresholdUnitKind = 'fixed' | 'distance' | 'elevation'

export function AutomatischeAnalysenSection() {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const { isAdmin } = useUser()
  const unitFormatters = useUnitFormatters()

  const { settings, isLoading, isError, mutation: mut } = useEnrichmentSettings()

  // One-shot legacy sync: legacy users may have enable_surface_detection=true but
  // enable_difficulty_scales=false because the scales toggle was added later. On
  // first mount, if the two fields are out of sync, bring enable_difficulty_scales
  // in line with enable_surface_detection. The useRef guard ensures this fires
  // at most once per mount, preventing any re-render loop.
  const legacySyncFiredRef = useRef(false)
  useEffect(() => {
    if (legacySyncFiredRef.current) return
    if (!settings) return
    if (settings.enable_surface_detection !== settings.enable_difficulty_scales) {
      legacySyncFiredRef.current = true
      mut.mutate({ enable_difficulty_scales: settings.enable_surface_detection })
    } else {
      legacySyncFiredRef.current = true
    }
  }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps

  const { data: capabilities } = useQuery({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 30_000,
  })

  const [localThresholds, setLocalThresholds] = useState<Partial<Record<string, number>>>({})
  const [backfillResult, setBackfillResult] = useState<{ processed: number; errors: number; error_details?: string } | null>(null)
  const [backfillRunning, setBackfillRunning] = useState(false)
  const geoPOIAvailable = featureAvailable(capabilities, 'geo_poi_enrichment')
  const thresholdDefinitions: Array<{
    key: 'climb_min_gradient_pct' | 'climb_min_elevation_m' | 'round_trip_threshold_m'
    label: string
    unitKind: ThresholdUnitKind
    unit: string
    step: number
  }> = [
    { key: 'climb_min_gradient_pct', label: t('settingsPage.analysis.climbMinGradient'), unitKind: 'fixed', unit: '%', step: 0.5 },
    {
      key: 'climb_min_elevation_m',
      label: t('settingsPage.analysis.climbMinElevation'),
      unitKind: 'elevation',
      unit: elevationInputUnit(unitFormatters.options.units),
      step: settingElevationDisplayValue(10, unitFormatters.options.units),
    },
    {
      key: 'round_trip_threshold_m',
      label: t('settingsPage.analysis.roundTripThreshold'),
      unitKind: 'distance',
      unit: distanceInputUnit(unitFormatters.options.units),
      step: settingDistanceDisplayValue(50, unitFormatters.options.units),
    },
  ]

  function thresholdDisplayValue(unitKind: ThresholdUnitKind, metersOrPercent: number | undefined): number | string {
    if (metersOrPercent == null) return ''
    if (unitKind === 'distance') return settingDistanceDisplayValue(metersOrPercent, unitFormatters.options.units)
    if (unitKind === 'elevation') return settingElevationDisplayValue(metersOrPercent, unitFormatters.options.units)
    return metersOrPercent
  }

  function thresholdStoredValue(unitKind: ThresholdUnitKind, value: string): number | null {
    if (unitKind === 'distance') return settingDistanceStoredValue(value, unitFormatters.options.units)
    if (unitKind === 'elevation') return settingElevationStoredValue(value, unitFormatters.options.units)
    const numeric = Number(value)
    return Number.isFinite(numeric) ? numeric : null
  }

  async function handleBackfillEnrichment() {
    setBackfillRunning(true)
    setBackfillResult(null)
    try {
      const result = await backfillEnrichment()
      setBackfillResult(result)
      addToast({
        type: 'success',
        title: t('settingsPage.analysis.backfillComplete'),
        message: t('settingsPage.analysis.backfillResult', { processed: result.processed, errors: result.errors }),
      })
    } catch {
      addToast({ type: 'error', title: t('settingsPage.analysis.backfillStartFailed'), duration: 0 })
    } finally {
      setBackfillRunning(false)
    }
  }

  return (
    <SettingsSection
      title={t('settingsPage.analysis.title')}
      description={t('settingsPage.analysis.description')}
      className="space-y-6"
    >
      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">
          {t('settingsPage.analysis.loadFailed')}
        </p>
      )}

      <div className="space-y-2">
        {ANALYSIS_MODULES.filter(m => !m.parentName && (!m.requiresCapability || featureAvailable(capabilities, m.requiresCapability))).map((module) => {
          const enabled = settings ? (settings[module.field] !== false) : true
          const mlUnavailable = module.requiresML && capabilities !== undefined && !capabilities.ml_service
          const moduleLabel = t(`settingsPage.analysis.modules.${module.name}.label`)
          const moduleDescription = t(`settingsPage.analysis.modules.${module.name}.description`)
          // Sub-modules (parentName === module.name) rendered indented below parent
          const children = ANALYSIS_MODULES.filter(m => m.parentName === module.name && (!m.requiresCapability || featureAvailable(capabilities, m.requiresCapability)))
          return (
            <div key={module.field as string}>
              <div className="flex items-start justify-between gap-4 py-1">
                <div className="min-w-0 flex-1">
                  <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{moduleLabel}</span>
                  {module.requiresML && (
                    <span className="ml-2 inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300">
                      ML
                    </span>
                  )}
                  <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {moduleDescription}</span>
                  {mlUnavailable && (
                    <p className="text-xs text-amber-600 dark:text-amber-400 mt-0.5">{t('settingsPage.analysis.mlUnavailable')}</p>
                  )}
                </div>
                <Toggle
                  checked={enabled && !mlUnavailable}
                  onChange={() => {
                    if (!settings) return
                    const next = !(settings[module.field] ?? true)
                    // Difficulty-scales share the same OSM-snap pipeline as surface detection.
                    // Keep enable_difficulty_scales in sync so the backend gate fires correctly.
                    const patch: Partial<EnrichmentSettings> = { [module.field]: next }
                    if (module.field === 'enable_surface_detection') {
                      patch.enable_difficulty_scales = next
                    }
                    mut.mutate(patch)
                  }}
                  disabled={isLoading || mut.isPending || mlUnavailable}
                  className="mt-0.5"
                  aria-label={t('settingsPage.analysis.activateModule', { module: moduleLabel })}
                />
              </div>
              {children.length > 0 && (
                <div className="ml-4 pl-4 border-l border-neutral-200 dark:border-neutral-700 space-y-1 mt-1 mb-1">
                  {children.map((child) => {
                    const parentDisabled = !enabled || mlUnavailable
                    const childEnabled = settings ? (settings[child.field] !== false) : true
                    const childLabel = t(`settingsPage.analysis.modules.${child.name}.label`)
                    const childDescription = t(`settingsPage.analysis.modules.${child.name}.description`)
                    return (
                      <div
                        key={child.field as string}
                        className={`flex items-start justify-between gap-4 py-1 transition-opacity ${parentDisabled ? 'opacity-60' : ''}`}
                      >
                        <div className="min-w-0 flex-1">
                          <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{childLabel}</span>
                          <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {childDescription}</span>
                        </div>
                        <Toggle
                          checked={childEnabled && !parentDisabled}
                          onChange={() => {
                            if (!settings || parentDisabled) return
                            const next = !(settings[child.field] ?? true)
                            mut.mutate({ [child.field]: next })
                          }}
                          // F21: mut.isPending is a single mutation shared across all toggles.
                          // A PUT for any field locks every sub-toggle until the request settles.
                          // Per-field mutation scoping requires restructuring this section into
                          // individual useMutation hooks per field — tracked as a follow-up task.
                          disabled={isLoading || mut.isPending || parentDisabled}
                          className={`mt-0.5 ${parentDisabled ? 'cursor-not-allowed' : ''}`}
                          aria-label={t('settingsPage.analysis.activateModule', { module: childLabel })}
                        />
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
      </div>

      {settings && (
        <AutoTitleDisplayPolicySelect
          value={settings.auto_title_display_policy ?? 'source_unless_generic'}
          onChange={auto_title_display_policy => mut.mutate({ auto_title_display_policy })}
          disabled={isLoading || mut.isPending || settings.enable_auto_naming === false}
        />
      )}

      {/* Summit detection radius slider */}
      {settings && geoPOIAvailable && (
        <div className="flex flex-col gap-2 py-2">
          <div className="flex items-center justify-between">
            <label htmlFor="summit-radius" className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{t('settingsPage.analysis.summitRadius')}</label>
            <span className="tabular-nums text-sm text-neutral-600 dark:text-neutral-400">{unitFormatters.formatDistanceCompact(settings.summit_detection_radius_m)}</span>
          </div>
          <input
            id="summit-radius"
            type="range"
            min={10}
            max={150}
            step={5}
            defaultValue={settings.summit_detection_radius_m}
            onMouseUp={e => mut.mutate({ summit_detection_radius_m: Number((e.target as HTMLInputElement).value) })}
            onKeyUp={e => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') mut.mutate({ summit_detection_radius_m: Number((e.target as HTMLInputElement).value) }) }}
            className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={isLoading || mut.isPending}
          />
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {t('settingsPage.analysis.summitRadiusDescription')}
          </p>
        </div>
      )}

      {/* Thresholds */}
      <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-4">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('settingsPage.analysis.thresholds')}</h3>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          {thresholdDefinitions.map(({ key, label, unitKind, unit, step }) => (
            <div key={key} className="space-y-1">
              <label className="text-xs text-neutral-500 dark:text-neutral-400">{label}</label>
              <div className="flex items-center gap-1">
                <input
                  type="number"
                  step={step}
                  value={thresholdDisplayValue(unitKind, localThresholds[key] ?? settings?.[key])}
                  onChange={e => {
                    const val = thresholdStoredValue(unitKind, e.target.value)
                    setLocalThresholds(prev => ({ ...prev, [key]: val ?? 0 }))
                  }}
                  onBlur={e => {
                    const val = thresholdStoredValue(unitKind, e.target.value)
                    if (val != null && val !== settings?.[key]) {
                      mut.mutate({ [key]: val })
                    }
                    setLocalThresholds(prev => { const next = { ...prev }; delete next[key]; return next })
                  }}
                  className="w-full text-sm bg-neutral-50 dark:bg-neutral-900 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
                />
                <span className="text-xs text-neutral-400 whitespace-nowrap">{unit}</span>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Weather note */}
      <p className="text-xs text-neutral-400 dark:text-neutral-500">
        {t('settingsPage.analysis.weatherHintPrefix')}{' '}
        <Link to="/settings?tab=general" className="text-accent-DEFAULT hover:underline">{t('settingsPage.analysis.weatherHintLink')}</Link>.
      </p>

      {/* Backfill actions — admin only */}
      {isAdmin && (
        <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('settingsPage.analysis.recompute')}</h3>
          <div className="flex flex-wrap gap-3">
            <button
              onClick={handleBackfillEnrichment}
              disabled={backfillRunning}
              className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {backfillRunning ? t('settingsPage.analysis.running') : t('settingsPage.analysis.recomputeEnrichment')}
            </button>
          </div>
          {backfillResult && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              {t('settingsPage.analysis.result', { processed: backfillResult.processed, errors: backfillResult.errors })}
              {backfillResult.error_details && <span className="block text-red-500 dark:text-red-400 mt-0.5">{backfillResult.error_details}</span>}
            </p>
          )}
        </div>
      )}

      <p className="text-xs text-neutral-400 dark:text-neutral-500 pt-1 border-t border-neutral-100 dark:border-neutral-800/60">
        {t('settingsPage.analysis.applyNote')}
      </p>
    </SettingsSection>
  )
}

// -----------------------------------------------------------------------
// Photo Preview Size Section
// -----------------------------------------------------------------------

const PREVIEW_SIZE_OPTIONS = [
  { value: 'wqhd', labelKey: 'settingsPage.photo.previewSizes.wqhd' },
  { value: '4k', labelKey: 'settingsPage.photo.previewSizes.4k' },
  { value: 'original', labelKey: 'settingsPage.photo.previewSizes.original' },
] as const

export function PhotoPreviewSection() {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const { isAdmin } = useUser()
  const { data: capabilities } = useCapabilities()

  const { settings, isLoading, mutation } = useEnrichmentSettings()

  const [framesLocal, setFramesLocal] = useState<number | null>(null)
  const framesInputRef = useRef<HTMLInputElement | null>(null)
  const photoEmbeddingsAvailable = featureAvailable(capabilities, 'photo_embeddings')

  function mutateFrames(n: number) {
    mutation.mutate({ video_clip_frames: n }, { onSuccess: () => setFramesLocal(null) })
  }

  async function handleRecompute() {
    const [previewResult, thumbResult] = await Promise.allSettled([
      recomputePhotoPreviews(),
      recomputePhotoThumbnails(),
    ])
    const anyFailed = previewResult.status === 'rejected' || thumbResult.status === 'rejected'
    const anyStarted = previewResult.status === 'fulfilled' || thumbResult.status === 'fulfilled'
    if (anyStarted) {
      addToast({ type: 'info', title: t('settingsPage.photo.previewGenerationStarted'), message: t('settingsPage.import.progressInHeader') })
    }
    if (anyFailed) {
      addToast({ type: 'error', title: t('settingsPage.photo.previewGenerationPartialFailure'), duration: 0 })
    }
  }

  async function handleRecomputeEmbeddings() {
    try {
      const result = await recomputePhotoEmbeddings()
      addToast({
        type: 'info',
        title: t('settingsPage.photo.embeddingIndexStarted'),
        message: t('settingsPage.import.phashBackfillQueued', { count: result.queued }),
      })
    } catch {
      addToast({ type: 'error', title: t('settingsPage.photo.embeddingIndexFailed'), duration: 0 })
    }
  }

  const currentSize = settings?.photo_preview_size ?? 'wqhd'
  const savedFrames = settings?.video_clip_frames ?? 4
  const currentFrames = framesLocal ?? savedFrames

  return (
    <SettingsSection
      title={t('settingsPage.photo.title')}
      description={t('settingsPage.photo.description')}
    >

      <div className="inline-flex rounded-lg border border-neutral-200 dark:border-neutral-700 overflow-hidden">
        {PREVIEW_SIZE_OPTIONS.map(({ value, labelKey }, idx) => (
          <button
            key={value}
            onClick={() => mutation.mutate({ photo_preview_size: value }, { onSuccess: () => addToast({ type: 'success', title: t('settingsPage.photo.settingSaved') }) })}
            disabled={isLoading || mutation.isPending}
            className={`px-4 py-1.5 text-sm font-medium whitespace-nowrap cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
              idx > 0 ? 'border-l border-neutral-200 dark:border-neutral-700' : ''
            } ${
              currentSize === value
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-white dark:bg-neutral-900 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
          >
            {t(labelKey)}
          </button>
        ))}
      </div>

      {photoEmbeddingsAvailable && (
        <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
          <div>
            <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100 mb-1">
              {t('settingsPage.photo.videoFramesTitle')}
            </h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-3">
              {t('settingsPage.photo.videoFramesDescription')}
            </p>

            {/* Timeline-Visualisierung */}
            <div className="relative h-6 flex items-center mb-1">
              <div className="absolute inset-x-0 top-1/2 -translate-y-1/2 h-px bg-neutral-300 dark:bg-neutral-600" />
              {Array.from({ length: currentFrames }, (_, i) => {
                const pct = currentFrames >= 4
                  ? (i / (currentFrames - 1)) * 100
                  : ((i + 1) / (currentFrames + 1)) * 100
                return (
                  <div
                    key={i}
                    className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-3 h-3 rounded-full bg-accent-DEFAULT shadow-sm"
                    style={{ left: `${pct}%` }}
                  />
                )
              })}
            </div>
            <div className="flex justify-between text-xs text-neutral-400 dark:text-neutral-500 mb-3">
              <span>{t('settingsPage.photo.timelineStart')}</span>
              <span>{t('settingsPage.photo.timelineEnd')}</span>
            </div>

            <div className="flex items-center gap-4">
              <input
                ref={framesInputRef}
                type="range"
                min={1}
                max={10}
                step={1}
                value={currentFrames}
                disabled={isLoading}
                onChange={() => setFramesLocal(Number(framesInputRef.current?.value ?? currentFrames))}
                onMouseUp={() => mutateFrames(Number(framesInputRef.current?.value ?? currentFrames))}
                onTouchEnd={() => mutateFrames(Number(framesInputRef.current?.value ?? currentFrames))}
                className="flex-1 accent-accent-DEFAULT cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              />
              <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 w-8 text-center tabular-nums">
                {currentFrames}
              </span>
            </div>
          </div>
        </div>
      )}

      {isAdmin && (
        <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
          {[
            { label: t('settingsPage.photo.recomputePreviews'), onClick: handleRecompute, desc: t('settingsPage.photo.recomputePreviewsDescription') },
            ...(photoEmbeddingsAvailable
              ? [{ label: t('settingsPage.photo.recomputeEmbeddings'), onClick: handleRecomputeEmbeddings, desc: t('settingsPage.photo.recomputeEmbeddingsDescription') }]
              : []),
          ].map(({ label, onClick, desc }) => (
            <div key={label} className="flex items-center gap-3">
              <button
                onClick={onClick}
                disabled={isLoading}
                className="cursor-pointer whitespace-nowrap px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors min-w-[200px]"
              >
                {label}
              </button>
              <p className="text-xs text-neutral-400 dark:text-neutral-500">{desc}</p>
            </div>
          ))}
        </div>
      )}
    </SettingsSection>
  )
}


// -----------------------------------------------------------------------
// Geocoding Fallback Section (external reverse-geocoding)
// -----------------------------------------------------------------------

const GEOCODE_PROVIDER_OPTIONS = [
  { value: 'off' },
  { value: 'nominatim' },
  { value: 'photon' },
] as const

type GeocodeProvider = typeof GEOCODE_PROVIDER_OPTIONS[number]['value']

const GEOCODE_PLACEHOLDERS: Record<Exclude<GeocodeProvider, 'off'>, string> = {
  nominatim: 'https://nominatim.openstreetmap.org',
  photon:    'https://photon.komoot.io',
}

function GeocodingFallbackSection() {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const { settings, isLoading, isError, mutation } = useEnrichmentSettings()

  const provider: GeocodeProvider = (settings?.geocode_fallback_provider ?? 'off') as GeocodeProvider
  const savedUrl = settings?.geocode_fallback_url ?? ''
  const savedTimeout = settings?.geocode_fallback_timeout_s ?? 5

  const [urlLocal, setUrlLocal] = useState<string | null>(null)
  const [timeoutLocal, setTimeoutLocal] = useState<number | null>(null)

  const urlValue = urlLocal ?? savedUrl
  const timeoutValue = timeoutLocal ?? savedTimeout

  function handleProviderChange(next: GeocodeProvider) {
    if (!settings) return
    if (next === provider) return
    mutation.mutate(
      { geocode_fallback_provider: next },
      { onSuccess: () => addToast({ type: 'success', title: t('settingsPage.geocoding.saved') }) },
    )
  }

  function commitUrl() {
    if (!settings) return
    const trimmed = urlValue.trim()
    const nextVal = trimmed === '' ? null : trimmed
    if (nextVal === (savedUrl || null)) {
      setUrlLocal(null)
      return
    }
    mutation.mutate(
      { geocode_fallback_url: nextVal },
      {
        onSuccess: () => {
          setUrlLocal(null)
          addToast({ type: 'success', title: t('settingsPage.geocoding.saved') })
        },
      },
    )
  }

  function commitTimeout() {
    if (!settings) return
    const n = Math.round(timeoutValue)
    if (!Number.isFinite(n) || n < 1 || n > 30) {
      addToast({ type: 'error', title: t('settingsPage.geocoding.invalidTimeout'), duration: 0 })
      setTimeoutLocal(null)
      return
    }
    if (n === savedTimeout) {
      setTimeoutLocal(null)
      return
    }
    mutation.mutate(
      { geocode_fallback_timeout_s: n },
      {
        onSuccess: () => {
          setTimeoutLocal(null)
          addToast({ type: 'success', title: t('settingsPage.geocoding.saved') })
        },
      },
    )
  }

  const showDetails = provider !== 'off'

  return (
    <SettingsSection
      title={t('settingsPage.geocoding.title')}
      description={t('settingsPage.geocoding.description')}
    >
      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">
          {t('settingsPage.geocoding.loadFailed')}
        </p>
      )}

      <div>
        <label
          htmlFor="geocode-provider"
          className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1"
        >
          {t('settingsPage.geocoding.providerLabel')}
        </label>
        <select
          id="geocode-provider"
          value={provider}
          disabled={isLoading || mutation.isPending}
          onChange={e => handleProviderChange(e.target.value as GeocodeProvider)}
          className="w-64 px-3 py-1.5 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {GEOCODE_PROVIDER_OPTIONS.map(opt => (
            <option key={opt.value} value={opt.value}>{t(`settingsPage.geocoding.providers.${opt.value}`)}</option>
          ))}
        </select>
      </div>

      {/* Privacy notice — prominent, directly under the dropdown */}
      <div
        role="note"
        className="rounded-lg border border-amber-200 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-900/20 px-4 py-3 text-sm text-amber-800 dark:text-amber-200"
      >
        {t('settingsPage.geocoding.privacyNotice')}
      </div>

      {showDetails && (
        <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-4">
          {/* URL override */}
          <div>
            <label
              htmlFor="geocode-url"
              className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1"
            >
              {t('settingsPage.geocoding.urlLabel')}
            </label>
            <input
              id="geocode-url"
              type="url"
              inputMode="url"
              placeholder={GEOCODE_PLACEHOLDERS[provider as Exclude<GeocodeProvider, 'off'>]}
              value={urlValue}
              disabled={isLoading || mutation.isPending}
              onChange={e => setUrlLocal(e.target.value)}
              onBlur={commitUrl}
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  ;(e.target as HTMLInputElement).blur()
                }
              }}
              className="block w-full max-w-md text-sm bg-neutral-50 dark:bg-neutral-900 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
            />
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
              {t('settingsPage.geocoding.urlHelp')}
            </p>
          </div>

          {/* Timeout */}
          <div>
            <label
              htmlFor="geocode-timeout"
              className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1"
            >
              {t('settingsPage.geocoding.timeoutLabel')}
            </label>
            <div className="flex items-center gap-2">
              <input
                id="geocode-timeout"
                type="number"
                min={1}
                max={30}
                step={1}
                value={timeoutValue}
                disabled={isLoading || mutation.isPending}
                onChange={e => setTimeoutLocal(Number(e.target.value))}
                onBlur={commitTimeout}
                onKeyDown={e => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    ;(e.target as HTMLInputElement).blur()
                  }
                }}
                className="w-24 text-sm bg-neutral-50 dark:bg-neutral-900 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed tabular-nums"
              />
              <span className="text-xs text-neutral-500 dark:text-neutral-400">
                {t('settingsPage.geocoding.timeoutUnit')}
              </span>
            </div>
          </div>

          {/* Backfill historical activities whose start_city is still empty */}
          <GeocodingBackfillBlock />

          {/* Provider-specific notes + common attribution */}
          <div className="text-xs text-neutral-500 dark:text-neutral-400 space-y-1">
            <p>{t(`settingsPage.geocoding.notes.${provider}`)}</p>
            <p>
              {t('settingsPage.geocoding.attribution')}{' '}
              {t(`settingsPage.geocoding.services.${provider}`)}
            </p>
          </div>
        </div>
      )}
    </SettingsSection>
  )
}

/**
 * "Run now" button for activities without start_city.
 *
 * Visible only inside GeocodingFallbackSection, so implicitly only while the
 * provider is active. Job progress polling runs through fetchActiveJobs (the
 * same query key as the global job indicator), with an active 2-second interval
 * while we track a job_id.
 */
function GeocodingBackfillBlock() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  // staleTime: 5s — generic refetch cadence that catches cases we don't
  // invalidate explicitly (e.g. a new activity imported in another tab).
  // Direct invalidation after a backfill completes still runs via the
  // useEffect below and is immediate regardless of this value.
  const gapQuery = useQuery({
    queryKey: ['geocoding-gap'],
    queryFn: fetchGeocodingGap,
    staleTime: 5_000,
  })

  // Keep the gap count in sync when new activities appear: the user might
  // import a GPS file elsewhere, which would leave the "0 without start_city"
  // label stale and the run-now button disabled until staleTime
  // expires + a refetch trigger fires. There is no dedicated "activity_imported"
  // event bus in the frontend; react-query's queryCache is the next-best signal.
  // We mark geocoding-gap stale whenever any ['activities', …] query settles.
  useEffect(() => {
    const cache = queryClient.getQueryCache()
    const unsubscribe = cache.subscribe(event => {
      if (event.type !== 'updated') return
      const queryKey = event.query.queryKey
      if (!Array.isArray(queryKey) || queryKey[0] !== 'activities') return
      // Only react to settled fetches (success) — ignore loading/error transitions
      // to avoid double-fetches.
      const action = event.action
      if (action.type !== 'success') return
      queryClient.invalidateQueries({ queryKey: ['geocoding-gap'] })
    })
    return unsubscribe
  }, [queryClient])

  // Tracks the specific backfill job we started — distinguishes "my click"
  // from any other geocoding job that might be running (e.g. a parallel tab).
  const [activeJobId, setActiveJobId] = useState<string | null>(null)

  // Poll while a job is tracked. 2s is aggressive enough for a visible bar
  // without hammering the server, and stops as soon as the job finishes.
  const jobsQuery = useQuery({
    queryKey: ['active-jobs'],
    queryFn: fetchActiveJobs,
    refetchInterval: activeJobId ? 2000 : false,
    enabled: activeJobId !== null,
  })

  const trackedJob = activeJobId
    ? jobsQuery.data?.jobs.find(j => j.id === activeJobId)
    : undefined

  // When the tracked job completes, clear state + invalidate caches that
  // now show stale city data. Toast type differentiates three outcomes:
  //   - completed           → success
  //   - failed + provider-off reason → warning (user-induced, recoverable)
  //   - failed + other error         → error   (unexpected)
  // The backend's BackgroundJob payload exposes an optional `error` string
  // that is only populated when `status === 'failed'`; ActiveJob doesn't yet
  // declare it in the shared TS type, so we read it via a narrow local cast.
  useEffect(() => {
    if (!trackedJob) return
    if (trackedJob.status !== 'completed' && trackedJob.status !== 'failed') return

    setActiveJobId(null)
    queryClient.invalidateQueries({ queryKey: ['geocoding-gap'] })
    queryClient.invalidateQueries({ queryKey: ['activities'] })

    if (trackedJob.status === 'completed') {
      addToast({ type: 'success', title: t('settingsPage.geocoding.backfill.complete') })
      return
    }

    // The backend encodes well-known failure reasons as a structured prefix
    // on the job's `error` string (e.g. "[provider_disabled] <human text>").
    // Checking the prefix is i18n-stable; the suffix remains a human-readable
    // message for DB inspection and future extension.
    const errorMessage = (trackedJob as { error?: string }).error ?? ''
    const providerOff = errorMessage.startsWith('[provider_disabled]')

    if (providerOff) {
      addToast({
        type: 'warning',
        title: t('settingsPage.geocoding.backfill.abortedProviderOff'),
        message: t('settingsPage.geocoding.backfill.abortedProviderOffMessage'),
      })
    } else {
      addToast({
        type: 'error',
        title: t('settingsPage.geocoding.backfill.failed'),
        message: errorMessage || undefined,
        duration: 0,
      })
    }
  }, [trackedJob, queryClient, addToast])

  const running = activeJobId !== null && trackedJob?.status !== 'completed' && trackedJob?.status !== 'failed'
  const count = gapQuery.data?.count ?? 0

  async function handleStart() {
    try {
      const { job_id, total } = await startGeocodingBackfill()
      setActiveJobId(job_id)
      // Seed the active-jobs cache so the progress bar appears immediately —
      // otherwise we wait up to 2s for the first refetch before anything renders.
      //
      // Tradeoff: the seeded job briefly appears in the global ActiveJobsIndicator
      // until the next /jobs/active refetch confirms it (or the SSE `init` frame
      // arrives, whichever is sooner). We accept this minor flicker because the
      // alternative — invalidate + await a round-trip — delays the local progress
      // bar by 100-300ms on every click, which feels sluggish in the common case.
      // The seed is harmless: identical shape, same job_id, so the server's view
      // overwrites it cleanly without UI churn.
      queryClient.setQueryData<{ jobs: unknown[]; pending_enrichments: number } | undefined>(
        ['active-jobs'],
        (prev) => {
          const seed = {
            id: job_id,
            job_type: 'geocoding_backfill',
            status: 'running' as const,
            progress_current: 0,
            progress_total: total,
            started_at: new Date().toISOString(),
          }
          if (!prev) return { jobs: [seed], pending_enrichments: 0 }
          return { ...prev, jobs: [...(prev.jobs as unknown[]), seed] }
        },
      )
    } catch (err) {
      // Structured error_code discriminates user-recoverable conditions from
      // genuine failures. The backend guarantees well-known codes via
      // application/json bodies; see ApiError.json in @/api/client.
      if (err instanceof ApiError && err.json && typeof err.json.error_code === 'string') {
        const code = err.json.error_code
        if (code === 'provider_disabled') {
          addToast({
            type: 'warning',
            title: t('settingsPage.geocoding.backfill.startProviderOff'),
            message: t('settingsPage.geocoding.backfill.startProviderOffMessage'),
          })
          return
        }
        if (code === 'already_running') {
          // Adopt the already-running job so the progress bar reflects it.
          const existingJobId = typeof err.json.job_id === 'string' ? err.json.job_id : ''
          if (existingJobId) {
            setActiveJobId(existingJobId)
          }
          addToast({
            type: 'info',
            title: t('settingsPage.geocoding.backfill.alreadyRunning'),
            message: t('settingsPage.geocoding.backfill.alreadyRunningMessage'),
          })
          return
        }
      }
      const message = err instanceof Error ? err.message : String(err)
      addToast({
        type: 'error',
        title: t('settingsPage.geocoding.backfill.startError'),
        message,
        duration: 0,
      })
    }
  }

  const gapLabel =
    count === 0
      ? t('settingsPage.geocoding.backfill.countZero')
      : count === 1
        ? t('settingsPage.geocoding.backfill.countOne')
        : t('settingsPage.geocoding.backfill.countMany', { count })

  const current = trackedJob?.progress_current ?? 0
  const total = trackedJob?.progress_total ?? 0
  const percent = total > 0 ? Math.min(100, Math.round((current / total) * 100)) : 0

  return (
    <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
      <div>
        <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {t('settingsPage.geocoding.backfill.title')}
        </p>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {t('settingsPage.geocoding.backfill.description')}
        </p>
      </div>

      <div className="flex items-center gap-3 flex-wrap">
        <span className="text-sm tabular-nums text-neutral-700 dark:text-neutral-300">
          {gapLabel}
        </span>
        <button
          type="button"
          onClick={handleStart}
          disabled={running || count === 0 || gapQuery.isLoading}
          className="px-3 py-1.5 text-sm font-medium rounded-lg text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:bg-neutral-300 dark:disabled:bg-neutral-700 disabled:text-neutral-500 dark:disabled:text-neutral-400 disabled:cursor-not-allowed transition-colors"
        >
          {running ? t('settingsPage.geocoding.backfill.running') : t('settingsPage.geocoding.backfill.button')}
        </button>
      </div>

      {running && total > 0 && (
        <div className="space-y-1">
          <div className="h-2 rounded-full bg-neutral-200 dark:bg-neutral-800 overflow-hidden">
            <div
              className="h-full bg-accent-DEFAULT transition-[width] duration-300 ease-out"
              style={{ width: `${percent}%` }}
            />
          </div>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 tabular-nums">
            {t('settingsPage.geocoding.backfill.progress', { current, total })}
          </p>
        </div>
      )}
    </div>
  )
}

type SettingsTab = 'general' | 'import' | 'karte' | 'analysen' | 'ai' | 'konto' | 'admin'
const VALID_TABS = new Set<SettingsTab>(['general', 'import', 'karte', 'analysen', 'ai', 'konto', 'admin'])

export default function SettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { accent, setAccent } = useAccentColor()
  const { isAdmin } = useUser()
  const { data: capabilities } = useCapabilities()
  const [searchParams, setSearchParams] = useSearchParams()
  const rawTab = searchParams.get('tab')
  const activeTab: SettingsTab = VALID_TABS.has(rawTab as SettingsTab) ? (rawTab as SettingsTab) : 'general'

  const setActiveTab = useCallback((tab: SettingsTab) => {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (tab === 'general') next.delete('tab')
      else next.set('tab', tab)
      return next
    }, { replace: true })
  }, [setSearchParams])

  const { data: settings, isLoading } = useQuery({
    queryKey: ['shadow-fs-settings'],
    queryFn: getShadowFSSettings,
  })
  const fullMapAdminConfigured = [
    'dem_correction',
    'offline_map',
    'osm_import',
    'coverage',
    'routing_tiles',
  ].some(key => featureConfigured(capabilities, key))


  const toggleMutation = useMutation({
    mutationFn: (enabled: boolean) => updateShadowFSSettings({ hotfolder_enabled: enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shadow-fs-settings'] })
    },
  })

  const tabListRef = useRef<HTMLDivElement>(null)

  const handleTabKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!tabListRef.current) return
    const tabs = Array.from(
      tabListRef.current.querySelectorAll<HTMLButtonElement>('[role="tab"]')
    )
    const currentIndex = tabs.findIndex(t => t === document.activeElement)
    let nextIndex = currentIndex

    if (e.key === 'ArrowRight') {
      e.preventDefault()
      nextIndex = currentIndex < tabs.length - 1 ? currentIndex + 1 : 0
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault()
      nextIndex = currentIndex > 0 ? currentIndex - 1 : tabs.length - 1
    } else if (e.key === 'Home') {
      e.preventDefault()
      nextIndex = 0
    } else if (e.key === 'End') {
      e.preventDefault()
      nextIndex = tabs.length - 1
    } else {
      return
    }

    tabs[nextIndex].focus()
    tabs[nextIndex].click()
  }, [])

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{t('settingsPage.title')}</h1>

      {/* Tab-Navigation */}
      <div
        ref={tabListRef}
        role="tablist"
        aria-label={t('settingsPage.tabs.ariaLabel')}
        className="flex gap-1 p-1 bg-neutral-100 dark:bg-neutral-800/50 rounded-lg w-fit flex-wrap"
        onKeyDown={handleTabKeyDown}
      >
        {[
          { id: 'general' as const,  label: t('settingsPage.tabs.general'), show: true },
          { id: 'import' as const,   label: t('settingsPage.tabs.import'), show: true },
          { id: 'karte' as const,    label: t('settingsPage.tabs.map'), show: true },
          { id: 'konto' as const,    label: t('settingsPage.tabs.account'), show: true },
          { id: 'analysen' as const, label: t('settingsPage.tabs.analysis'), show: true },
          { id: 'ai' as const,       label: t('settingsPage.tabs.ai'), show: true },
          { id: 'admin' as const,    label: t('settingsPage.tabs.admin'), show: isAdmin },
        ].filter(tab => tab.show).map(tab => (
          <button
            key={tab.id}
            role="tab"
            aria-selected={activeTab === tab.id}
            aria-controls={`settings-panel-${tab.id}`}
            id={`settings-tab-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-1.5 rounded-md text-sm font-medium cursor-pointer transition-colors ${
              activeTab === tab.id
                ? 'bg-white dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100 shadow-sm'
                : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'konto' && (
        <div role="tabpanel" id="settings-panel-konto" aria-labelledby="settings-tab-konto">
          <AccountSettingsTab />
        </div>
      )}

      {activeTab === 'admin' && isAdmin && (
        <div role="tabpanel" id="settings-panel-admin" aria-labelledby="settings-tab-admin">
          <AdminSettingsTab />
        </div>
      )}

      {activeTab === 'analysen' && (
        <div role="tabpanel" id="settings-panel-analysen" aria-labelledby="settings-tab-analysen" className="space-y-8">
          <AutomatischeAnalysenSection />
        </div>
      )}

      {activeTab === 'ai' && (
        <div role="tabpanel" id="settings-panel-ai" aria-labelledby="settings-tab-ai" className="space-y-8">
          {isAdmin && <LLMSettingsPage />}
          {isAdmin && featureConfigured(capabilities, 'ml_service') && <MLServiceSection />}
          <NotificationSettingsSection />
          <ChatProfileSettings />
          <ChatWriteToolSettings />
        </div>
      )}

      {activeTab === 'general' && (
      <div role="tabpanel" id="settings-panel-general" aria-labelledby="settings-tab-general" className="space-y-8">
      <div className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200 dark:border-neutral-700 px-4 py-3 text-sm text-neutral-600 dark:text-neutral-400">
        {t('settingsPage.general.workshopHintPrefix')}{' '}
        <Link to="/workshop" className="text-accent-DEFAULT hover:text-accent-600 font-medium">{t('settingsPage.general.workshopLink')}</Link>.
      </div>

      {/* Erscheinungsbild */}
      <SettingsSection title={t('settingsPage.general.appearanceTitle')} description={t('settingsPage.general.appearanceDescription')}>
        <div className="flex flex-wrap items-center gap-3">
          <ColorPicker
            value={accent.shade500}
            onChange={hex => setAccent(deriveAccentEntry(hex))}
          />
          {accent.shade500.toLowerCase() !== DEFAULT_ACCENT_COLOR.toLowerCase() && (
            <button
              onClick={() => setAccent(ACCENT_PALETTE[0])}
              className="rounded-lg border border-neutral-200 dark:border-neutral-700 px-3 py-1.5 text-sm text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
            >
              {t('settingsPage.general.resetAccent')}
            </button>
          )}
        </div>
      </SettingsSection>

      <PhotoPreviewSection />

      {/* Wetterdaten */}
      <WeatherSettings />

      {/* Sportarten */}
      <SportSettingsSection />

      {/* Automation */}
      <SettingsSection title={t('settingsPage.general.automationTitle')} description={t('settingsPage.general.automationDescription')}>
        <Link
          to="/settings/rules"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          {t('settingsPage.general.automationRules')}
        </Link>
      </SettingsSection>

      {/* Privacy */}
      <SettingsSection title={t('settingsPage.general.privacyTitle')} description={t('settingsPage.general.privacyDescription')}>
        <Link
          to="/settings/privacy"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          {t('settingsPage.general.managePrivateZones')}
        </Link>
      </SettingsSection>

      </div>
      )}

      {activeTab === 'import' && (
      <div role="tabpanel" id="settings-panel-import" aria-labelledby="settings-tab-import" className="space-y-8">

      {/* Hotfolder */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div className="flex items-start justify-between">
          <div>
            <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('settingsPage.import.hotfolderTitle')}</h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              {t('settingsPage.import.hotfolderDescription')}
            </p>
          </div>

          <Toggle
            checked={!!settings?.hotfolder_enabled}
            onChange={() => toggleMutation.mutate(!settings?.hotfolder_enabled)}
            disabled={isLoading || toggleMutation.isPending}
            aria-label={t('settingsPage.import.hotfolderToggle')}
          />
        </div>

        {isLoading ? (
          <p className="text-sm text-neutral-400">{t('settingsPage.loading')}</p>
        ) : (
          <>
            <div className="flex items-center gap-2 text-sm">
              <span className="text-neutral-500 dark:text-neutral-400">{t('settingsPage.import.pathLabel')}</span>
              <span className="font-mono text-neutral-800 dark:text-neutral-200 bg-neutral-50 dark:bg-neutral-800 px-2 py-1 rounded">
                {settings?.hotfolder_path || t('settingsPage.import.notConfigured')}
              </span>
            </div>

            <div className="flex items-center gap-2 text-sm">
              <span className="text-neutral-500 dark:text-neutral-400">{t('settingsPage.import.statusLabel')}</span>
              {settings?.hotfolder_enabled ? (
                <span className="inline-flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-emerald-500" />
                  <span className="text-emerald-600 dark:text-emerald-400">{t('settingsPage.import.active')}</span>
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-neutral-400" />
                  <span className="text-neutral-500 dark:text-neutral-400">{t('settingsPage.import.inactive')}</span>
                </span>
              )}
            </div>
          </>
        )}

      </section>

      {/* File storage */}
      <FileSchemaSettings />

      {/* Import settings */}
      <ImportSettingsSection />

      </div>
      )}

      {activeTab === 'karte' && (
      <div role="tabpanel" id="settings-panel-karte" aria-labelledby="settings-tab-karte" className="space-y-8">

      {/* Map settings */}
      <HeatmapSettingsSection />

      {/* Reverse-geocoding fallback (external service) */}
      <GeocodingFallbackSection />

      {/* Custom map layers (all users) */}
      <UserCustomLayersSection />

      {/* Admin hint: DEM, offline maps, OSM now live under Admin -> Maps & Tiles */}
      {isAdmin && fullMapAdminConfigured && (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {t('settingsPage.map.adminHintPrefix')}{' '}
          <Link
            to="/settings?tab=admin&section=maps"
            className="text-accent-DEFAULT hover:underline"
          >
            {t('settingsPage.map.adminHintLink')}
          </Link>
          .
        </p>
      )}

      </div>
      )}
    </div>
  )
}

function SportSettingsSection() {
  const { t } = useTranslation()
  const { getSportColor, setSportColor, resetSportColor, customColors, resetAllColors, hideUnused, setHideUnused } =
    useGlobalSportSettings()

  // Expanded state for the color list
  const [colorsOpen, setColorsOpen] = useState(false)

  // Preview colors (not persisted yet) — key: sportType
  const [pendingColors, setPendingColors] = useState<Record<string, string>>({})

  const { data: statsData, isLoading: statsLoading } = useQuery({
    queryKey: ['sport-type-stats'],
    queryFn: () => fetchSportTypeStats(),
  })

  const existingSportTypes = useMemo<Set<string>>(() => {
    if (!statsData) return new Set()
    return new Set(statsData.map((s) => s.sport_type))
  }, [statsData])

  const categoryGroups = useMemo(() => {
    return CATEGORIES.map((cat) => {
      let sports = SPORT_DEFS.filter((s) => s.categoryKey === cat.key)
      if (hideUnused && statsData) {
        sports = sports.filter((s) => existingSportTypes.has(s.sportType))
      }
      return { cat, sports }
    }).filter((g) => g.sports.length > 0)
  }, [hideUnused, statsData, existingSportTypes])

  const hasCustomColors = Object.keys(customColors).length > 0

  // Set preview color for a sport
  function previewSport(sportType: string, hex: string) {
    setPendingColors((prev) => ({ ...prev, [sportType]: hex }))
  }

  // Confirm preview and persist it
  function confirmSport(sportType: string, hex: string) {
    setSportColor(sportType, hex)
    setPendingColors((prev) => { const n = { ...prev }; delete n[sportType]; return n })
  }

  // Discard preview and reset picker to the original value
  function cancelSport(sportType: string) {
    setPendingColors((prev) => { const n = { ...prev }; delete n[sportType]; return n })
  }

  // Preview category color: all sports in the category at once
  function previewCategory(catKey: string, hex: string) {
    const sports = SPORT_DEFS.filter((s) => s.categoryKey === catKey)
    setPendingColors((prev) => {
      const n = { ...prev }
      for (const s of sports) n[s.sportType] = hex
      return n
    })
  }

  function confirmCategory(catKey: string, hex: string) {
    const sports = SPORT_DEFS.filter((s) => s.categoryKey === catKey)
    for (const s of sports) setSportColor(s.sportType, hex)
    setPendingColors((prev) => {
      const n = { ...prev }
      for (const s of sports) delete n[s.sportType]
      return n
    })
  }

  function cancelCategory(catKey: string) {
    const sports = SPORT_DEFS.filter((s) => s.categoryKey === catKey)
    setPendingColors((prev) => {
      const n = { ...prev }
      for (const s of sports) delete n[s.sportType]
      return n
    })
  }

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl overflow-hidden">
      {/* Header — always visible, toggles the color list */}
      <button
        type="button"
        onClick={() => setColorsOpen((o) => !o)}
        className="w-full flex items-center justify-between px-6 py-4 text-left hover:bg-neutral-50 dark:hover:bg-neutral-800/50 cursor-pointer transition-colors"
      >
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('settingsPage.sports.title')}</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5">
            {t('settingsPage.sports.description')}
          </p>
        </div>
        {/* Chevron */}
        <ChevronDown className={`w-5 h-5 text-neutral-400 flex-shrink-0 transition-transform duration-200 ${colorsOpen ? 'rotate-180' : ''}`} />
      </button>

      {/* Expanded content */}
      {colorsOpen && (
        <div className="px-6 pb-6 space-y-5 border-t border-neutral-100 dark:border-neutral-800 pt-4">
          {/* Hide unused toggle */}
          <label className="flex items-start gap-3 cursor-pointer group">
            <input
              type="checkbox"
              checked={hideUnused}
              onChange={(e) => setHideUnused(e.target.checked)}
              className="mt-0.5 h-4 w-4 rounded border-neutral-300 dark:border-neutral-600 accent-accent-DEFAULT cursor-pointer flex-shrink-0"
            />
            <div>
              <span className="text-sm font-medium text-neutral-800 dark:text-neutral-200 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                {t('settingsPage.sports.hideUnused')}
              </span>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                {t('settingsPage.sports.hideUnusedDescription')}
              </p>
            </div>
          </label>

          {/* Loading hint only during the initial load */}
          {hideUnused && statsLoading && (
            <p className="text-xs text-neutral-400">{t('settingsPage.sports.loadingStats')}</p>
          )}

          {/* Color list */}
          <div className="space-y-5">
            {categoryGroups.map(({ cat, sports }) => {
              // Representative category color for the category picker:
              // first sport in the category (possibly with a pending preview)
              const catPreviewColor = sports.length > 0
                ? (pendingColors[sports[0].sportType] ?? getSportColor(sports[0].sportType))
                : cat.color

              return (
                <div key={cat.key}>
                  {/* Category header with category color picker */}
                  <div className="flex items-center gap-2 mb-2">
                    <ColorPicker
                      value={catPreviewColor}
                      onChange={(hex) => previewCategory(cat.key, hex)}
                      onConfirm={(hex) => confirmCategory(cat.key, hex)}
                      onCancel={() => cancelCategory(cat.key)}
                    />
                    <span className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
                      {localizeString(cat.labels)}
                    </span>
                    <span className="text-xs text-neutral-400 dark:text-neutral-600 font-normal normal-case tracking-normal">
                      {t('settingsPage.sports.applyColorToCategory')}
                    </span>
                  </div>

                  {/* Einzelne Sportarten */}
                  <div className="space-y-1.5 pl-2 border-l-2 border-neutral-100 dark:border-neutral-800">
                    {sports.map((s) => {
                      const displayColor = pendingColors[s.sportType] ?? getSportColor(s.sportType)
                      const isCustom = !!customColors[s.sportType]
                      const hasPending = s.sportType in pendingColors

                      return (
                        <div key={s.sportType} className="flex items-center gap-3">
                          <ColorPicker
                            value={displayColor}
                            onChange={(hex) => previewSport(s.sportType, hex)}
                            onConfirm={(hex) => confirmSport(s.sportType, hex)}
                            onCancel={() => cancelSport(s.sportType)}
                          />
                          <span className={`flex-1 text-sm ${hasPending ? 'text-[color:var(--color-accent-DEFAULT,#F97316)]' : 'text-neutral-800 dark:text-neutral-200'}`}>
                            {localizeString(s.labels)}
                            {hasPending && <span className="text-xs ml-1 opacity-70">{t('settingsPage.sports.previewSuffix')}</span>}
                          </span>
                          {(isCustom && !hasPending) && (
                            <button
                              onClick={() => resetSportColor(s.sportType)}
                              className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors cursor-pointer"
                            >
                              {t('settingsPage.sports.reset')}
                            </button>
                          )}
                        </div>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>

          {hasCustomColors && (
            <button
              onClick={resetAllColors}
              className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 underline transition-colors cursor-pointer"
            >
              {t('settingsPage.sports.resetAll')}
            </button>
          )}
        </div>
      )}
    </section>
  )
}


function HeatmapSettingsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const unitFormatters = useUnitFormatters()
  const [localRadius, setLocalRadius] = useState<number | null>(null)
  const [localInterval, setLocalInterval] = useState<number | null>(null)
  const [localThreshold, setLocalThreshold] = useState<number | null>(null)
  const [localFilterMap, setLocalFilterMap] = useState<boolean | null>(null)
  const [localFilterActivity, setLocalFilterActivity] = useState<boolean | null>(null)
  const [defaultBaseStyle, setDefaultBaseStyle] = useState<string>(() => {
    try {
      const s = localStorage.getItem('skadia-map-defaults')
      if (s) {
        const p = JSON.parse(s) as { defaultBaseStyle?: string }
        if (p.defaultBaseStyle) return p.defaultBaseStyle
      }
    } catch { /* ignore */ }
    return 'osm'
  })

  // Dynamic base styles from server — replaces the old parallel hardcoded list.
  const { base: baseLayers, disabled: disabledLayers } = useMapLayers()

  // If the stored default refers to a layer that no longer exists (e.g. admin
  // disabled it), fall back to the first available layer and persist that so
  // the next reload doesn't trigger React's "value not in options" warning.
  // While baseLayers is still loading (length === 0), keep the stored value.
  useEffect(() => {
    if (baseLayers.length === 0) return
    if (!defaultBaseStyle) return
    if (baseLayers.some(l => l.key === defaultBaseStyle)) return
    const fallback = baseLayers[0].key
    setDefaultBaseStyle(fallback)
    try {
      const stored = JSON.parse(localStorage.getItem('skadia-map-defaults') || '{}') as Record<string, unknown>
      localStorage.setItem('skadia-map-defaults', JSON.stringify({ ...stored, defaultBaseStyle: fallback }))
    } catch { /* ignore */ }
  }, [baseLayers, defaultBaseStyle])

  const { data: heatmapSettings } = useQuery({
    queryKey: ['heatmap-settings'],
    queryFn: getHeatmapSettings,
  })

  // Terra Incognita Radius aus Enrichment-Settings
  const { settings: enrichSettings, mutation: enrichMutation } = useEnrichmentSettings()
  const radiusMut = {
    mutate: (val: number) => {
      enrichMutation.mutate(
        { terra_incognita_radius_m: val },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['terra-incognita'] })
            setLocalRadius(null)
          },
        },
      )
    },
    isPending: enrichMutation.isPending,
  }
  const currentRadius = localRadius ?? enrichSettings?.terra_incognita_radius_m ?? 1000
  const radiusDirty = localRadius !== null && localRadius !== (enrichSettings?.terra_incognita_radius_m ?? 1000)
  const fmtDistance = unitFormatters.formatDistanceCompact

  const updateHeatmapMut = useMutation({
    mutationFn: updateHeatmapSettings,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['heatmap-settings'] })
      setLocalInterval(null)
      setLocalThreshold(null)
      setLocalFilterMap(null)
      setLocalFilterActivity(null)
    },
  })

  const currentInterval = localInterval ?? heatmapSettings?.sampling_interval_m ?? 20
  const currentThreshold = localThreshold ?? heatmapSettings?.teleport_threshold_m ?? 2000
  const currentFilterMap = localFilterMap ?? heatmapSettings?.teleport_filter_map ?? true
  const currentFilterActivity = localFilterActivity ?? heatmapSettings?.teleport_filter_activity ?? true

  const isDirty =
    (localInterval !== null && localInterval !== heatmapSettings?.sampling_interval_m) ||
    (localThreshold !== null && localThreshold !== heatmapSettings?.teleport_threshold_m) ||
    (localFilterMap !== null && localFilterMap !== heatmapSettings?.teleport_filter_map) ||
    (localFilterActivity !== null && localFilterActivity !== heatmapSettings?.teleport_filter_activity)

  return (
    <SettingsSection title={t('settingsPage.map.title')} description={t('settingsPage.map.description')}>

      {/* Default map style */}
      <div>
        <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1">
          {t('settingsPage.map.defaultStyle')}
        </label>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-2">
          {t('settingsPage.map.defaultStyleDescription')}
        </p>
        <select
          value={defaultBaseStyle}
          onChange={e => {
            const val = e.target.value
            setDefaultBaseStyle(val)
            try {
              const stored = JSON.parse(localStorage.getItem('skadia-map-defaults') || '{}') as Record<string, unknown>
              localStorage.setItem('skadia-map-defaults', JSON.stringify({ ...stored, defaultBaseStyle: val }))
            } catch { /* ignore */ }
          }}
          className="w-48 px-3 py-1.5 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100"
        >
          {baseLayers.map(l => (
            <option key={l.key} value={l.key}>{l.name}</option>
          ))}
          {disabledLayers.filter(d => d.layer.role === 'base').map(({ layer, missingCapability }) => (
            <option key={layer.key} value={layer.key} disabled>
              {t('settingsPage.map.missingCapability', { name: layer.name, capability: missingCapability })}
            </option>
          ))}
        </select>
      </div>

      {/* ── Heatmap & Terra Incognita ── */}
      <div className="border-t border-neutral-200 dark:border-neutral-700 pt-5 space-y-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">{t('settingsPage.map.heatmapTerra')}</h3>

        <div>
          <div className="flex items-center justify-between mb-2">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {t('settingsPage.map.terraRadius')}
          </label>
          <span className="text-sm text-neutral-500 tabular-nums">{fmtDistance(currentRadius)}</span>
        </div>
        <input
          type="range"
          min={100}
          max={10000}
          step={50}
          value={currentRadius}
          onChange={(e) => setLocalRadius(Number(e.target.value))}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT cursor-pointer"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>{fmtDistance(100)} ({t('settingsPage.map.radiusMinHint')})</span>
          <span>{fmtDistance(10000)} ({t('settingsPage.map.radiusMaxHint')})</span>
        </div>
        <p className="text-xs text-neutral-400 mt-2">
          {t('settingsPage.map.radiusDescription')}
        </p>
        {radiusDirty && (
          <button
            onClick={() => radiusMut.mutate(currentRadius)}
            disabled={radiusMut.isPending}
            className="mt-3 px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >
            {radiusMut.isPending ? t('settingsPage.saving') : t('settingsPage.map.saveRadius')}
          </button>
        )}
      </div>

        <div className="mt-4">
          <div className="flex items-center justify-between mb-2">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {t('settingsPage.map.samplingInterval')}
          </label>
          <span className="text-sm text-neutral-500 tabular-nums">{fmtDistance(currentInterval)}</span>
        </div>
        <input
          type="range"
          min={10}
          max={100}
          step={5}
          value={currentInterval}
          onChange={(e) => setLocalInterval(Number(e.target.value))}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT cursor-pointer"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>{fmtDistance(10)} ({t('settingsPage.map.intervalMinHint')})</span>
          <span>{fmtDistance(100)} ({t('settingsPage.map.intervalMaxHint')})</span>
        </div>
        <p className="text-xs text-neutral-400 mt-2">
          {t('settingsPage.map.intervalDescription')}
        </p>
        </div>
      </div>

      {/* ── Teleportation gaps ── */}
      <div className="border-t border-neutral-200 dark:border-neutral-700 pt-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">{t('settingsPage.map.teleportGaps')}</h3>
        <div className="flex items-center justify-between mb-1">
          <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            {t('settingsPage.map.hideTeleportGaps')}
          </p>
          <Toggle
            checked={!!(currentFilterMap || currentFilterActivity)}
            onChange={() => {
              const next = !(currentFilterMap || currentFilterActivity)
              setLocalFilterMap(next)
              setLocalFilterActivity(next)
            }}
            aria-label={t('settingsPage.map.hideTeleportGaps')}
          />
        </div>
        <p className="text-xs text-neutral-400 mb-4">
          {t('settingsPage.map.teleportDescription')}
        </p>

        <div className="space-y-2 mb-4">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={currentFilterMap}
              onChange={(e) => setLocalFilterMap(e.target.checked)}
              className="h-4 w-4 rounded accent-accent-DEFAULT cursor-pointer"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">{t('settingsPage.map.mapTracks')}</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={currentFilterActivity}
              onChange={(e) => setLocalFilterActivity(e.target.checked)}
              className="h-4 w-4 rounded accent-accent-DEFAULT cursor-pointer"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">{t('settingsPage.map.activityDetailView')}</span>
          </label>
        </div>

        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              {t('settingsPage.map.minGapLength')}
            </label>
            <span className="text-sm text-neutral-500 tabular-nums">{fmtDistance(currentThreshold)}</span>
          </div>
          <input
            type="range"
            min={100}
            max={10000}
            step={100}
            value={currentThreshold}
            onChange={(e) => setLocalThreshold(Number(e.target.value))}
            className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT cursor-pointer"
          />
          <div className="flex justify-between text-xs text-neutral-400 mt-1">
            <span>{fmtDistance(100)}</span>
            <span>{fmtDistance(10000)}</span>
          </div>
        </div>
      </div>

      {isDirty && (
        <button
          onClick={() => updateHeatmapMut.mutate({
            sampling_interval_m: currentInterval,
            teleport_threshold_m: currentThreshold,
            teleport_filter_map: currentFilterMap,
            teleport_filter_activity: currentFilterActivity,
          })}
          disabled={updateHeatmapMut.isPending}
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
        >
          {updateHeatmapMut.isPending ? t('settingsPage.saving') : t('settingsPage.save')}
        </button>
      )}

      {updateHeatmapMut.isError && (
        <p className="text-sm text-rose-500">
          {t('settingsPage.errorPrefix')} {updateHeatmapMut.error instanceof Error ? updateHeatmapMut.error.message : t('settingsPage.unknownError')}
        </p>
      )}
    </SettingsSection>
  )
}

function ImportSettingsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { isAdmin } = useUser()
  const unitFormatters = useUnitFormatters()
  const { data: settings } = useQuery({
    queryKey: ['import-settings'],
    queryFn: getImportSettings,
  })
  const mut = useMutation({
    mutationFn: updateImportSettings,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['import-settings'] }),
  })

  const [phashBackfillPending, setPhashBackfillPending] = useState(false)

  async function handlePhashBackfill() {
    setPhashBackfillPending(true)
    try {
      const result = await backfillPhash()
      if (result.status === 'already_running') {
        addToast({ type: 'info', title: t('settingsPage.import.phashAlreadyRunning'), message: t('settingsPage.import.progressInHeader') })
      } else if ((result.queued ?? 0) === 0) {
        addToast({ type: 'success', title: t('settingsPage.import.allPhotosHavePhash') })
      } else {
        addToast({ type: 'info', title: t('settingsPage.import.phashBackfillStarted'), message: t('settingsPage.import.phashBackfillQueued', { count: result.queued }) })
      }
    } catch {
      addToast({ type: 'error', title: t('settingsPage.import.phashBackfillFailed'), duration: 0 })
    } finally {
      setPhashBackfillPending(false)
    }
  }

  const { settings: enrichSettings, mutation: enrichMut } = useEnrichmentSettings()

  const s: ImportSettings = settings ?? { conflict_strategy: 'enrich_missing', gps_duplicate_threshold: 75, fusion_time_window_min: 30 }
  const hausdorff = enrichSettings?.route_duplicate_hausdorff_m ?? 100
  const lengthDiff = enrichSettings?.route_duplicate_length_diff_km ?? 0.5
  const phashBits = enrichSettings?.phash_threshold_bits ?? 3

  // Local state for all sliders — visual feedback during drag, commit on release
  const [localGpsDup, setLocalGpsDup] = useState(s.gps_duplicate_threshold)
  const [localFusionMin, setLocalFusionMin] = useState(s.fusion_time_window_min)
  const [localHausdorff, setLocalHausdorff] = useState(hausdorff)
  const [localLengthDiff, setLocalLengthDiff] = useState(lengthDiff)
  const [localPhashBits, setLocalPhashBits] = useState(phashBits)
  const fmtDistance = unitFormatters.formatDistanceCompact

  useEffect(() => {
    setLocalGpsDup(s.gps_duplicate_threshold)
    setLocalFusionMin(s.fusion_time_window_min)
    setLocalHausdorff(hausdorff)
    setLocalLengthDiff(lengthDiff)
    setLocalPhashBits(phashBits)
  }, [s.gps_duplicate_threshold, s.fusion_time_window_min, hausdorff, lengthDiff, phashBits])

  return (
    <SettingsSection title={t('settingsPage.import.title')} className="space-y-5">

      {/* Konflikt-Strategie */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          {t('settingsPage.import.defaultDuplicateBehavior')}
        </label>
        <select
          value={s.conflict_strategy}
          onChange={e => mut.mutate({ ...s, conflict_strategy: e.target.value as ConflictStrategy })}
          className="block w-full max-w-xs border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100"
        >
          {settingsConflictStrategyOptions(t).map(option => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </select>
        <p className="text-xs text-neutral-400 mt-1">
          {t('settingsPage.import.defaultDuplicateDescription')}
        </p>
      </div>

      {/* GPS-Duplikat-Schwellwert */}
      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>{t('settingsPage.import.gpsDuplicateThreshold')}</span>
          <span className="tabular-nums font-mono">{localGpsDup} %</span>
        </div>
        <input
          type="range"
          min={60}
          max={95}
          step={5}
          value={localGpsDup}
          onChange={e => setLocalGpsDup(Number(e.target.value))}
          onPointerUp={() => mut.mutate({ ...s, gps_duplicate_threshold: localGpsDup })}
          onKeyUp={e => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') mut.mutate({ ...s, gps_duplicate_threshold: localGpsDup }) }}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>{t('settingsPage.import.gpsThresholdMin')}</span>
          <span>{t('settingsPage.import.gpsThresholdMax')}</span>
        </div>
        <p className="text-xs text-neutral-400 mt-1">
          {t('settingsPage.import.gpsDuplicateDescription')}
        </p>
      </div>

      {/* Zeitfenster Fusionserkennung */}
      <div className="space-y-2">
        <div className="flex items-center justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300">
          <span>{t('settingsPage.import.fusionTimeWindow')}</span>
          <span className="tabular-nums font-mono">{localFusionMin} min</span>
        </div>
        <input
          type="range"
          min={5}
          max={120}
          step={5}
          value={localFusionMin}
          onChange={e => setLocalFusionMin(Number(e.target.value))}
          onPointerUp={() => mut.mutate({ ...s, fusion_time_window_min: localFusionMin })}
          onKeyUp={e => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') mut.mutate({ ...s, fusion_time_window_min: localFusionMin }) }}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT"
        />
        <div className="flex justify-between text-xs text-neutral-400">
          <span>{t('settingsPage.import.fusionWindowMin')}</span>
          <span>{t('settingsPage.import.fusionWindowMax')}</span>
        </div>
        <p className="text-xs text-neutral-400 mt-1">
          {t('settingsPage.import.fusionTimeDescription')}
        </p>
      </div>

      {/* Routen-Duplikaterkennung */}
      <div className="space-y-2 pt-2 border-t border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {t('settingsPage.import.routeDuplicateTitle')}
        </h3>
        <p className="text-xs text-neutral-400">
          {t('settingsPage.import.routeDuplicateDescription')}
        </p>
      </div>

      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>{t('settingsPage.import.maxGpsDeviation')}</span>
          <span className="tabular-nums font-mono">{fmtDistance(localHausdorff)}</span>
        </div>
        <input
          type="range"
          min={10}
          max={200}
          step={10}
          value={localHausdorff}
          onChange={e => setLocalHausdorff(Number(e.target.value))}
          onPointerUp={() => enrichMut.mutate({ route_duplicate_hausdorff_m: localHausdorff })}
          onBlur={() => enrichMut.mutate({ route_duplicate_hausdorff_m: localHausdorff })}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>{fmtDistance(10)} ({t('settingsPage.import.hausdorffMinHint')})</span>
          <span>{fmtDistance(200)} ({t('settingsPage.import.hausdorffMaxHint')})</span>
        </div>
      </div>

      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>{t('settingsPage.import.maxLengthDifference')}</span>
          <span className="tabular-nums font-mono">
            {fmtDistance(localLengthDiff * 1000)}
          </span>
        </div>
        <input
          type="range"
          min={0.1}
          max={2}
          step={0.1}
          value={localLengthDiff}
          onChange={e => setLocalLengthDiff(Number(e.target.value))}
          onPointerUp={() => enrichMut.mutate({ route_duplicate_length_diff_km: localLengthDiff })}
          onBlur={() => enrichMut.mutate({ route_duplicate_length_diff_km: localLengthDiff })}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>{fmtDistance(100)} ({t('settingsPage.import.lengthDiffMinHint')})</span>
          <span>{fmtDistance(2000)} ({t('settingsPage.import.lengthDiffMaxHint')})</span>
        </div>
      </div>

      {/* Foto-Duplikat-Erkennungsschwelle */}
      <div className="space-y-2 pt-2 border-t border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {t('settingsPage.import.photoDuplicateTitle')}
        </h3>
        <p className="text-xs text-neutral-400">
          {t('settingsPage.import.photoDuplicateDescription')}
        </p>
      </div>

      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>{t('settingsPage.import.photoDuplicateThreshold')}</span>
          <span className="tabular-nums font-mono">{localPhashBits} Bits</span>
        </div>
        <input
          type="range"
          min={1}
          max={10}
          step={1}
          value={localPhashBits}
          onChange={e => setLocalPhashBits(Number(e.target.value))}
          onPointerUp={() => enrichMut.mutate({ phash_threshold_bits: localPhashBits })}
          onBlur={() => enrichMut.mutate({ phash_threshold_bits: localPhashBits })}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>{t('settingsPage.import.phashMin')}</span>
          <span>{t('settingsPage.import.phashMax')}</span>
        </div>
        <p className="text-xs text-neutral-400 mt-1">{t('settingsPage.import.phashDefault')}</p>
      </div>

      {/* pHash Backfill — admin only */}
      {isAdmin && (
        <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60">
          <div className="flex items-center gap-3">
            <button
              onClick={handlePhashBackfill}
              disabled={phashBackfillPending}
              className="cursor-pointer whitespace-nowrap px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors min-w-[200px]"
            >
              {phashBackfillPending ? t('settingsPage.import.starting') : t('settingsPage.import.recomputePhash')}
            </button>
            <p className="text-xs text-neutral-400 dark:text-neutral-500">{t('settingsPage.import.phashBackfillDescription')}</p>
          </div>
        </div>
      )}
    </SettingsSection>
  )
}
