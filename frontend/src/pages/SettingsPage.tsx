import { useState, useMemo, useEffect, useRef, useCallback } from 'react'
import { ChevronDown } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'
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
import { recomputePhotoPreviews, recomputePhotoEmbeddings, backfillPhash } from '@/api/activityPhotos'
import { useToast } from '@/lib/toast'
import FileSchemaSettings from '@/components/FileSchemaSettings'
import LLMSettingsPage from './LLMSettingsPage'
import ChatProfileSettings from '@/components/ChatProfileSettings'
import { WeatherSettings } from '@/components/WeatherSettings'
import AccountSettingsTab from '@/pages/settings/AccountSettingsTab'
import AdminSettingsTab from '@/pages/settings/AdminSettingsTab'
import { MLServiceSection } from '@/pages/settings/MLServiceSection'
import { NotificationSettingsSection } from '@/pages/settings/NotificationSettingsSection'
import { LocalMapAndGeoSection } from '@/pages/settings/LocalMapAndGeoSection'
import Toggle from '@/components/Toggle'
import { SettingsSection } from '@/components/ui/SettingsSection'

// Analysis module configuration — enrichment toggles for the Analysen tab.
interface AnalysisModule {
  label: string
  description: string
  field: keyof EnrichmentSettings
  requiresML: boolean
}

const ANALYSIS_MODULES: AnalysisModule[] = [
  { label: 'Automatische Benennung',  description: 'Generiert automatisch einen Titel basierend auf Route und Aktivitätstyp.',        field: 'enable_auto_naming',         requiresML: false },
  { label: 'Gipfel & Pässe',          description: 'Erkennt besuchte Gipfel und Bergsättel.',                                          field: 'enable_summit_detection',    requiresML: false },
  { label: 'Wälder',                  description: 'Erkennt durchquerte Waldgebiete.',                                                 field: 'enable_forest_detection',    requiresML: false },
  { label: 'Gewässer',                description: 'Erkennt Seen und Fließgewässer entlang der Route.',                               field: 'enable_water_detection',     requiresML: false },
  { label: 'Ortschaften',             description: 'Erkennt durchquerte Siedlungen.',                                                  field: 'enable_settlement_detection',requiresML: false },
  { label: 'Grenzüberquerungen',      description: 'Erkennt überquerte Ländergrenzen.',                                               field: 'enable_border_detection',    requiresML: false },
  { label: 'Stopps & Einkehr',        description: 'Erkennt Aufenthalte und Restaurantbesuche (≥10 min).',                            field: 'enable_stop_detection',      requiresML: false },
  { label: 'Lifte / Seilbahnen',      description: 'Erkennt Gondelbahnen und Sessellifte.',                                           field: 'enable_lift_detection',      requiresML: true  },
  { label: 'Untergrundtypen',         description: 'Erkennt Asphalt, Schotter, Trail etc.',                                           field: 'enable_surface_detection',   requiresML: false },
  { label: 'Anstiege',                description: 'Erkennt und klassifiziert Anstiege.',                                             field: 'enable_ascent_detection',    requiresML: false },
  { label: 'Intervalltraining',       description: 'Erkennt Trainingsintervalle automatisch.',                                        field: 'enable_interval_detection',  requiresML: false },
  { label: 'Rundenerkennung',         description: 'Erkennt automatisch Runden/Laps bei Aktivitäten auf Rundkursen.',                 field: 'enable_lap_detection',       requiresML: false },
  { label: 'Embeddings',              description: 'Berechnet semantische Vektoren für Ähnlichkeitssuche.',                           field: 'enable_embeddings',          requiresML: true  },
]

function AutomatischeAnalysenSection() {
  const { addToast } = useToast()
  const { isAdmin } = useUser()

  const { settings, isLoading, isError, mutation: mut } = useEnrichmentSettings()

  const { data: capabilities } = useQuery({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 30_000,
  })

  const [localThresholds, setLocalThresholds] = useState<Partial<Record<string, number>>>({})
  const [backfillResult, setBackfillResult] = useState<{ processed: number; errors: number; error_details?: string } | null>(null)
  const [backfillRunning, setBackfillRunning] = useState(false)

  async function handleBackfillEnrichment() {
    setBackfillRunning(true)
    setBackfillResult(null)
    try {
      const result = await backfillEnrichment()
      setBackfillResult(result)
      addToast({ type: 'success', title: 'Enrichment abgeschlossen', message: `${result.processed} verarbeitet, ${result.errors} Fehler` })
    } catch {
      addToast({ type: 'error', title: 'Enrichment konnte nicht gestartet werden', duration: 0 })
    } finally {
      setBackfillRunning(false)
    }
  }

  return (
    <SettingsSection
      title="Automatische Analysen"
      description="Analysen, die nach dem Upload automatisch im Hintergrund durchgeführt werden."
      className="space-y-6"
    >

      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">
          Einstellungen konnten nicht geladen werden.
        </p>
      )}

      <div className="space-y-2">
        {ANALYSIS_MODULES.map((module) => {
          const enabled = settings ? (settings[module.field] !== false) : true
          const mlUnavailable = module.requiresML && capabilities !== undefined && !capabilities.ml_service
          return (
            <div key={module.field as string} className="flex items-start justify-between gap-4 py-1">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{module.label}</span>
                {module.requiresML && (
                  <span className="ml-2 inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-violet-100 dark:bg-violet-900/40 text-violet-700 dark:text-violet-300">
                    ML
                  </span>
                )}
                <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {module.description}</span>
                {mlUnavailable && (
                  <p className="text-xs text-amber-600 dark:text-amber-400 mt-0.5">ML-Service nicht erreichbar</p>
                )}
              </div>
              <Toggle
                checked={enabled && !mlUnavailable}
                onChange={() => {
                  if (!settings) return
                  mut.mutate({ [module.field]: !(settings[module.field] ?? true) })
                }}
                disabled={isLoading || mut.isPending || mlUnavailable}
                className="mt-0.5"
                aria-label={`${module.label} aktivieren`}
              />
            </div>
          )
        })}
      </div>

      {/* Thresholds */}
      <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-4">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Schwellenwerte</h3>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          {[
            { key: 'climb_min_gradient_pct' as const, label: 'Min. Anstiegsgradient', unit: '%', step: 0.5 },
            { key: 'climb_min_elevation_m' as const,  label: 'Min. Anstiegshöhe',     unit: 'm', step: 10  },
            { key: 'round_trip_threshold_m' as const, label: 'Rundtour-Schwelle',      unit: 'm', step: 50  },
          ].map(({ key, label, unit, step }) => (
            <div key={key} className="space-y-1">
              <label className="text-xs text-neutral-500 dark:text-neutral-400">{label}</label>
              <div className="flex items-center gap-1">
                <input
                  type="number"
                  step={step}
                  value={localThresholds[key] ?? settings?.[key] ?? ''}
                  onChange={e => setLocalThresholds(prev => ({ ...prev, [key]: Number(e.target.value) }))}
                  onBlur={e => {
                    const val = Number(e.target.value)
                    if (!isNaN(val) && val !== settings?.[key]) {
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
        Wetter-Einstellungen findest du unter{' '}
        <Link to="/settings?tab=general" className="text-accent-DEFAULT hover:underline">Allgemein → Wetterdaten</Link>.
      </p>

      {/* Backfill actions — admin only */}
      {isAdmin && (
        <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Neuberechnung</h3>
          <div className="flex flex-wrap gap-3">
            <button
              onClick={handleBackfillEnrichment}
              disabled={backfillRunning}
              className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {backfillRunning ? 'Läuft…' : 'Enrichment neu berechnen'}
            </button>
          </div>
          {backfillResult && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              Ergebnis: {backfillResult.processed} verarbeitet, {backfillResult.errors} Fehler
              {backfillResult.error_details && <span className="block text-red-500 dark:text-red-400 mt-0.5">{backfillResult.error_details}</span>}
            </p>
          )}
        </div>
      )}

      <p className="text-xs text-neutral-400 dark:text-neutral-500 pt-1 border-t border-neutral-100 dark:border-neutral-800/60">
        Änderungen gelten für neue Aktivitäten sowie bei manueller Neuberechnung.
      </p>
    </SettingsSection>
  )
}

// -----------------------------------------------------------------------
// Photo Preview Size Section
// -----------------------------------------------------------------------

const PREVIEW_SIZE_OPTIONS = [
  { value: 'wqhd', label: 'WQHD' },
  { value: '4k', label: '4K' },
  { value: 'original', label: 'Original' },
] as const

function PhotoPreviewSection() {
  const { addToast } = useToast()
  const { isAdmin } = useUser()

  const { settings, isLoading, mutation } = useEnrichmentSettings()

  const [framesLocal, setFramesLocal] = useState<number | null>(null)

  function mutateFrames(n: number) {
    mutation.mutate({ video_clip_frames: n }, { onSuccess: () => setFramesLocal(null) })
  }

  async function handleRecompute() {
    try {
      await recomputePhotoPreviews()
      addToast({ type: 'info', title: 'Vorschau-Generierung gestartet', message: 'Fortschritt im Header-Indikator sichtbar' })
    } catch {
      addToast({ type: 'error', title: 'Neuberechnung konnte nicht gestartet werden', duration: 0 })
    }
  }

  async function handleRecomputeEmbeddings() {
    try {
      const result = await recomputePhotoEmbeddings()
      addToast({
        type: 'info',
        title: 'Bildsuche-Indexierung gestartet',
        message: `${result.queued} Fotos werden verarbeitet — Fortschritt im Header-Indikator sichtbar`,
      })
    } catch {
      addToast({ type: 'error', title: 'Indexierung konnte nicht gestartet werden', duration: 0 })
    }
  }

  const currentSize = settings?.photo_preview_size ?? 'wqhd'
  const savedFrames = settings?.video_clip_frames ?? 4
  const currentFrames = framesLocal ?? savedFrames

  return (
    <SettingsSection
      title="Foto-Vorschaugröße"
      description="Maximale Auflösung für Galerie-Ansichten. Nur wenn das Original größer ist, wird ein separates Vorschau-Bild erstellt."
    >

      <div className="inline-flex rounded-lg border border-neutral-200 dark:border-neutral-700 overflow-hidden">
        {PREVIEW_SIZE_OPTIONS.map(({ value, label }, idx) => (
          <button
            key={value}
            onClick={() => mutation.mutate({ photo_preview_size: value }, { onSuccess: () => addToast({ type: 'success', title: 'Einstellung gespeichert' }) })}
            disabled={isLoading || mutation.isPending}
            className={`px-4 py-1.5 text-sm font-medium whitespace-nowrap cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
              idx > 0 ? 'border-l border-neutral-200 dark:border-neutral-700' : ''
            } ${
              currentSize === value
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-white dark:bg-neutral-900 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100 mb-1">
            Video-Analyse: Frames für KI-Bildsuche
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-3">
            Anzahl der Frames, die aus einem Video extrahiert werden.
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
            <span>Start</span>
            <span>Ende</span>
          </div>

          <div className="flex items-center gap-4">
            <input
              type="range"
              min={1}
              max={10}
              step={1}
              value={currentFrames}
              disabled={isLoading}
              onChange={e => setFramesLocal(Number(e.target.value))}
              onMouseUp={e => mutateFrames(Number((e.target as HTMLInputElement).value))}
              onTouchEnd={e => mutateFrames(Number((e.target as HTMLInputElement).value))}
              className="flex-1 accent-accent-DEFAULT cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            />
            <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 w-8 text-center tabular-nums">
              {currentFrames}
            </span>
          </div>
        </div>
      </div>

      {isAdmin && (
        <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
          {[
            { label: 'Vorschauen neu erstellen', onClick: handleRecompute, desc: 'Generiert Vorschau-Bilder für alle vorhandenen Fotos mit der aktuellen Einstellung neu.' },
            { label: 'Bildsuche neu indexieren', onClick: handleRecomputeEmbeddings, desc: 'Berechnet KI-Embeddings für alle Fotos (ermöglicht semantische Suche nach Inhalt).' },
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
  { value: 'off',       label: 'Aus' },
  { value: 'nominatim', label: 'Nominatim (OpenStreetMap)' },
  { value: 'photon',    label: 'Photon (Komoot)' },
] as const

type GeocodeProvider = typeof GEOCODE_PROVIDER_OPTIONS[number]['value']

const GEOCODE_PLACEHOLDERS: Record<Exclude<GeocodeProvider, 'off'>, string> = {
  nominatim: 'https://nominatim.openstreetmap.org',
  photon:    'https://photon.komoot.io',
}

const GEOCODE_PROVIDER_NOTES: Record<Exclude<GeocodeProvider, 'off'>, string> = {
  nominatim: 'Strenge Rate-Limits (1 Anfrage/Sekunde global). Für gelegentliche Anfragen ausreichend.',
  photon:    'Community-Dienst ohne offizielle Garantien (Fair-Use).',
}

const GEOCODE_PROVIDER_SERVICES: Record<Exclude<GeocodeProvider, 'off'>, string> = {
  nominatim: 'Dienst: OpenStreetMap Foundation.',
  photon:    'Dienst: Komoot (Photon).',
}

const GEOCODE_STRINGS = {
  sectionTitle:       'Standort-Erkennung',
  sectionDescription: 'Externer Reverse-Geocoding-Dienst als Fallback, wenn lokale OSM-Daten keinen Ortsnamen liefern.',
  providerLabel:      'Dienst',
  privacyNotice:      'Wenn aktiviert, werden Start-Koordinaten (auf ca. 75–110 m gerundet) an den externen Dienst übermittelt. Nur bei Aktivitäten, für die lokale OSM-Daten keine Stadt liefern.',
  urlLabel:           'Server-URL (optional)',
  urlHelp:            'Leer lassen für den öffentlichen Dienst. Eigener Wert nur bei Reverse-Proxy oder Self-Hosting nötig.',
  timeoutLabel:       'Zeitüberschreitung',
  timeoutUnit:        'Sekunden',
  attribution:        'Daten © OpenStreetMap-Mitwirkende (ODbL).',
  saved:              'Einstellung gespeichert',
  invalidTimeout:     'Zeitüberschreitung muss zwischen 1 und 30 Sekunden liegen',

  // "Geocoding nachholen" subsection
  backfillTitle:       'Ortsnamen nachtragen',
  backfillDescription: 'Ältere Aktivitäten, die vor dem Aktivieren des Fallbacks importiert wurden, haben möglicherweise noch keinen Ortsnamen.',
  backfillCountZero:   'Alle Aktivitäten haben einen Ortsnamen.',
  backfillCountOne:    '1 Aktivität ohne Ortsangabe',
  backfillCountMany:   (n: number) => `${n} Aktivitäten ohne Ortsangabe`,
  backfillButton:      'Jetzt nachholen',
  backfillRunning:     'Läuft …',
  backfillProgress:    (done: number, total: number) => `${done} / ${total} bearbeitet`,
  backfillStartError:  'Backfill konnte nicht gestartet werden',
  backfillStartProviderOff:    'Bitte erst einen Geocoding-Dienst aktivieren',
  backfillStartProviderOffMsg: 'Wähle oben einen Fallback-Dienst aus, bevor du Ortsnamen nachträgst.',
  backfillAlreadyRunning:    'Backfill läuft bereits',
  backfillAlreadyRunningMsg: 'Fortschritt im Header-Indikator sichtbar.',
  backfillComplete:    'Geocoding abgeschlossen',
  backfillAbortedProviderOff: 'Geocoding abgebrochen',
  backfillAbortedProviderOffMsg: 'Der Fallback-Dienst wurde während des Backfills deaktiviert.',
  backfillFailed:      'Geocoding fehlgeschlagen',
} as const

function GeocodingFallbackSection() {
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
      { onSuccess: () => addToast({ type: 'success', title: GEOCODE_STRINGS.saved }) },
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
          addToast({ type: 'success', title: GEOCODE_STRINGS.saved })
        },
      },
    )
  }

  function commitTimeout() {
    if (!settings) return
    const n = Math.round(timeoutValue)
    if (!Number.isFinite(n) || n < 1 || n > 30) {
      addToast({ type: 'error', title: GEOCODE_STRINGS.invalidTimeout, duration: 0 })
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
          addToast({ type: 'success', title: GEOCODE_STRINGS.saved })
        },
      },
    )
  }

  const showDetails = provider !== 'off'

  return (
    <SettingsSection
      title={GEOCODE_STRINGS.sectionTitle}
      description={GEOCODE_STRINGS.sectionDescription}
    >
      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">
          Einstellungen konnten nicht geladen werden.
        </p>
      )}

      <div>
        <label
          htmlFor="geocode-provider"
          className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1"
        >
          {GEOCODE_STRINGS.providerLabel}
        </label>
        <select
          id="geocode-provider"
          value={provider}
          disabled={isLoading || mutation.isPending}
          onChange={e => handleProviderChange(e.target.value as GeocodeProvider)}
          className="w-64 px-3 py-1.5 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {GEOCODE_PROVIDER_OPTIONS.map(opt => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>

      {/* Privacy notice — prominent, directly under the dropdown */}
      <div
        role="note"
        className="rounded-lg border border-amber-200 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-900/20 px-4 py-3 text-sm text-amber-800 dark:text-amber-200"
      >
        {GEOCODE_STRINGS.privacyNotice}
      </div>

      {showDetails && (
        <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-4">
          {/* URL override */}
          <div>
            <label
              htmlFor="geocode-url"
              className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1"
            >
              {GEOCODE_STRINGS.urlLabel}
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
              {GEOCODE_STRINGS.urlHelp}
            </p>
          </div>

          {/* Timeout */}
          <div>
            <label
              htmlFor="geocode-timeout"
              className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1"
            >
              {GEOCODE_STRINGS.timeoutLabel}
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
                {GEOCODE_STRINGS.timeoutUnit}
              </span>
            </div>
          </div>

          {/* Backfill historical activities whose start_city is still empty */}
          <GeocodingBackfillBlock />

          {/* Provider-specific notes + common attribution */}
          <div className="text-xs text-neutral-500 dark:text-neutral-400 space-y-1">
            <p>{GEOCODE_PROVIDER_NOTES[provider as Exclude<GeocodeProvider, 'off'>]}</p>
            <p>
              {GEOCODE_STRINGS.attribution}{' '}
              {GEOCODE_PROVIDER_SERVICES[provider as Exclude<GeocodeProvider, 'off'>]}
            </p>
          </div>
        </div>
      )}
    </SettingsSection>
  )
}

/**
 * "Jetzt nachholen"-Button für Aktivitäten ohne start_city.
 *
 * Sichtbar nur innerhalb des GeocodingFallbackSection, also implizit nur wenn
 * der Provider aktiv ist. Polling des Job-Fortschritts läuft über
 * fetchActiveJobs (dasselbe query-key wie der globale Job-Indikator), mit
 * aktivem 2-Sekunden-Intervall, solange wir eine job_id tracken.
 */
function GeocodingBackfillBlock() {
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
  // label stale and the "Jetzt nachholen"-button disabled until staleTime
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
      addToast({ type: 'success', title: GEOCODE_STRINGS.backfillComplete })
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
        title: GEOCODE_STRINGS.backfillAbortedProviderOff,
        message: GEOCODE_STRINGS.backfillAbortedProviderOffMsg,
      })
    } else {
      addToast({
        type: 'error',
        title: GEOCODE_STRINGS.backfillFailed,
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
            title: GEOCODE_STRINGS.backfillStartProviderOff,
            message: GEOCODE_STRINGS.backfillStartProviderOffMsg,
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
            title: GEOCODE_STRINGS.backfillAlreadyRunning,
            message: GEOCODE_STRINGS.backfillAlreadyRunningMsg,
          })
          return
        }
      }
      const message = err instanceof Error ? err.message : String(err)
      addToast({
        type: 'error',
        title: GEOCODE_STRINGS.backfillStartError,
        message,
        duration: 0,
      })
    }
  }

  const gapLabel =
    count === 0
      ? GEOCODE_STRINGS.backfillCountZero
      : count === 1
        ? GEOCODE_STRINGS.backfillCountOne
        : GEOCODE_STRINGS.backfillCountMany(count)

  const current = trackedJob?.progress_current ?? 0
  const total = trackedJob?.progress_total ?? 0
  const percent = total > 0 ? Math.min(100, Math.round((current / total) * 100)) : 0

  return (
    <div className="pt-4 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
      <div>
        <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {GEOCODE_STRINGS.backfillTitle}
        </p>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {GEOCODE_STRINGS.backfillDescription}
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
          {running ? GEOCODE_STRINGS.backfillRunning : GEOCODE_STRINGS.backfillButton}
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
            {GEOCODE_STRINGS.backfillProgress(current, total)}
          </p>
        </div>
      )}
    </div>
  )
}

type SettingsTab = 'general' | 'import' | 'karte' | 'analysen' | 'ai' | 'konto' | 'admin'
const VALID_TABS = new Set<SettingsTab>(['general', 'import', 'karte', 'analysen', 'ai', 'konto', 'admin'])

export default function SettingsPage() {
  const queryClient = useQueryClient()
  const { accent, setAccent } = useAccentColor()
  const { isAdmin } = useUser()
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
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Einstellungen</h1>

      {/* Tab-Navigation */}
      <div
        ref={tabListRef}
        role="tablist"
        aria-label="Einstellungen-Kategorien"
        className="flex gap-1 p-1 bg-neutral-100 dark:bg-neutral-800/50 rounded-lg w-fit flex-wrap"
        onKeyDown={handleTabKeyDown}
      >
        {[
          { id: 'general' as const,  label: 'Allgemein',          show: true      },
          { id: 'import' as const,   label: 'Import',             show: true      },
          { id: 'karte' as const,    label: 'Karte & Geodaten',   show: true      },
          { id: 'konto' as const,    label: 'Konto',              show: true      },
          { id: 'analysen' as const, label: 'Analysen',           show: true      },
          { id: 'ai' as const,       label: 'KI & ML',            show: true      },
          { id: 'admin' as const,    label: 'Admin',              show: isAdmin   },
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
        <div role="tabpanel" id="settings-panel-ai" aria-labelledby="settings-tab-ai">
          {isAdmin && <LLMSettingsPage />}
          {isAdmin && <MLServiceSection />}
          <NotificationSettingsSection />
          <ChatProfileSettings />
        </div>
      )}

      {activeTab === 'general' && (
      <div role="tabpanel" id="settings-panel-general" aria-labelledby="settings-tab-general" className="space-y-8">
      <div className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200 dark:border-neutral-700 px-4 py-3 text-sm text-neutral-600 dark:text-neutral-400">
        Einstellung nicht gefunden? Einstellungen zur Medienverwaltung unter{' '}
        <Link to="/workshop" className="text-accent-DEFAULT hover:text-accent-600 font-medium">Werkstatt</Link>.
      </div>

      {/* Erscheinungsbild */}
      <SettingsSection title="Erscheinungsbild" description="Wähle eine Akzentfarbe für die Benutzeroberfläche.">
        <ColorPicker
          value={accent.shade500}
          onChange={hex => setAccent(deriveAccentEntry(hex))}
        />
        {accent.shade500.toLowerCase() !== DEFAULT_ACCENT_COLOR.toLowerCase() && (
          <button
            onClick={() => setAccent(ACCENT_PALETTE[0])}
            className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 underline cursor-pointer transition-colors"
          >
            Auf Standard zurücksetzen
          </button>
        )}
      </SettingsSection>

      <PhotoPreviewSection />

      {/* Wetterdaten */}
      <WeatherSettings />

      {/* Sportarten */}
      <SportSettingsSection />

      {/* Automatisierung */}
      <SettingsSection title="Automatisierung" description="Regeln für automatische Zuweisungen und Tags.">
        <Link
          to="/settings/rules"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          Automatisierungsregeln
        </Link>
      </SettingsSection>

      {/* Privatsphäre */}
      <SettingsSection title="Privatsphäre" description="Definiere Zonen, in denen GPS-Daten automatisch aus öffentlichen Ansichten ausgeblendet werden.">
        <Link
          to="/settings/privacy"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          Private Zonen verwalten
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
            <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Hotfolder</h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              Automatischer Import neuer Dateien aus einem Verzeichnis.
            </p>
          </div>

          <Toggle
            checked={!!settings?.hotfolder_enabled}
            onChange={() => toggleMutation.mutate(!settings?.hotfolder_enabled)}
            disabled={isLoading || toggleMutation.isPending}
            aria-label="Hotfolder aktivieren"
          />
        </div>

        {isLoading ? (
          <p className="text-sm text-neutral-400">Lade...</p>
        ) : (
          <>
            <div className="flex items-center gap-2 text-sm">
              <span className="text-neutral-500 dark:text-neutral-400">Pfad:</span>
              <span className="font-mono text-neutral-800 dark:text-neutral-200 bg-neutral-50 dark:bg-neutral-800 px-2 py-1 rounded">
                {settings?.hotfolder_path || '(nicht konfiguriert)'}
              </span>
            </div>

            <div className="flex items-center gap-2 text-sm">
              <span className="text-neutral-500 dark:text-neutral-400">Status:</span>
              {settings?.hotfolder_enabled ? (
                <span className="inline-flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-emerald-500" />
                  <span className="text-emerald-600 dark:text-emerald-400">Aktiv</span>
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5">
                  <span className="w-2 h-2 rounded-full bg-neutral-400" />
                  <span className="text-neutral-500 dark:text-neutral-400">Inaktiv</span>
                </span>
              )}
            </div>
          </>
        )}

      </section>

      {/* Datenimport */}
      <SettingsSection title="Datenimport" description="Importiere Aktivitäten, Ausrüstung und Gesundheitsdaten aus einem Garmin-, Strava- oder Komoot-Export.">
        <div className="flex gap-3 flex-wrap">
          <Link
            to="/settings/import?tab=garmin"
            className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
          >
            Garmin-Import
          </Link>
          <Link
            to="/settings/import?tab=strava"
            className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
          >
            Strava-Import
          </Link>
          <Link
            to="/settings/import?tab=komoot"
            className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
          >
            Komoot-Import
          </Link>
        </div>
      </SettingsSection>

      {/* Dateiablage */}
      <FileSchemaSettings />

      {/* Import-Einstellungen */}
      <ImportSettingsSection />

      </div>
      )}

      {activeTab === 'karte' && (
      <div role="tabpanel" id="settings-panel-karte" aria-labelledby="settings-tab-karte" className="space-y-8">

      {/* Karteneinstellungen */}
      <HeatmapSettingsSection />

      {/* Reverse-Geocoding-Fallback (externer Dienst) */}
      <GeocodingFallbackSection />

      {/* Lokale Karten & Geodaten */}
      {isAdmin && <LocalMapAndGeoSection />}

      </div>
      )}
    </div>
  )
}

function SportSettingsSection() {
  const { getSportColor, setSportColor, resetSportColor, customColors, resetAllColors, hideUnused, setHideUnused } =
    useGlobalSportSettings()

  // Aufgeklappt-Status für die Farbliste
  const [colorsOpen, setColorsOpen] = useState(false)

  // Vorschau-Farben (noch nicht gespeichert) — key: sportType
  const [pendingColors, setPendingColors] = useState<Record<string, string>>({})

  const { data: statsData, isLoading: statsLoading } = useQuery({
    queryKey: ['sport-type-stats'],
    queryFn: fetchSportTypeStats,
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

  // Vorschau-Farbe für einen Sport setzen
  function previewSport(sportType: string, hex: string) {
    setPendingColors((prev) => ({ ...prev, [sportType]: hex }))
  }

  // Vorschau bestätigen → dauerhaft speichern
  function confirmSport(sportType: string, hex: string) {
    setSportColor(sportType, hex)
    setPendingColors((prev) => { const n = { ...prev }; delete n[sportType]; return n })
  }

  // Vorschau verwerfen → Picker auf Originalwert zurück
  function cancelSport(sportType: string) {
    setPendingColors((prev) => { const n = { ...prev }; delete n[sportType]; return n })
  }

  // Kategorie-Farbe vorschau: alle Sports in der Kategorie gleichzeitig
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
      {/* Header — immer sichtbar, klappt Farbliste auf/zu */}
      <button
        type="button"
        onClick={() => setColorsOpen((o) => !o)}
        className="w-full flex items-center justify-between px-6 py-4 text-left hover:bg-neutral-50 dark:hover:bg-neutral-800/50 cursor-pointer transition-colors"
      >
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Sportarten</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5">
            Farben pro Sportart, Filtereinstellungen.
          </p>
        </div>
        {/* Chevron */}
        <ChevronDown className={`w-5 h-5 text-neutral-400 flex-shrink-0 transition-transform duration-200 ${colorsOpen ? 'rotate-180' : ''}`} />
      </button>

      {/* Ausgeklappter Inhalt */}
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
                Nicht vorhandene Sportarten in Filtern ausblenden
              </span>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                Nur Sportarten anzeigen, für die bereits Aktivitäten hochgeladen wurden.
              </p>
            </div>
          </label>

          {/* Ladehinweis nur während des ersten Ladens */}
          {hideUnused && statsLoading && (
            <p className="text-xs text-neutral-400">Lade Sportarten-Statistiken…</p>
          )}

          {/* Farbliste */}
          <div className="space-y-5">
            {categoryGroups.map(({ cat, sports }) => {
              // Repräsentative Farbe der Kategorie für den Kategorie-Picker:
              // erster Sport der Kategorie (ggf. mit pending-Vorschau)
              const catPreviewColor = sports.length > 0
                ? (pendingColors[sports[0].sportType] ?? getSportColor(sports[0].sportType))
                : cat.color

              return (
                <div key={cat.key}>
                  {/* Kategorie-Header mit Kategorie-Farbpicker */}
                  <div className="flex items-center gap-2 mb-2">
                    <ColorPicker
                      value={catPreviewColor}
                      onChange={(hex) => previewCategory(cat.key, hex)}
                      onConfirm={(hex) => confirmCategory(cat.key, hex)}
                      onCancel={() => cancelCategory(cat.key)}
                    />
                    <span className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
                      {cat.label}
                    </span>
                    <span className="text-xs text-neutral-400 dark:text-neutral-600 font-normal normal-case tracking-normal">
                      — alle auf diese Farbe setzen
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
                            {s.label}
                            {hasPending && <span className="text-xs ml-1 opacity-70">(Vorschau)</span>}
                          </span>
                          {(isCustom && !hasPending) && (
                            <button
                              onClick={() => resetSportColor(s.sportType)}
                              className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors cursor-pointer"
                            >
                              Zurücksetzen
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
              Alle Farben zurücksetzen
            </button>
          )}
        </div>
      )}
    </section>
  )
}


type BaseStyle = 'osm' | 'topo' | 'cyclosm' | 'satellite' | 'dark' | 'local'

const BASE_STYLE_OPTIONS: { value: BaseStyle; label: string }[] = [
  { value: 'osm', label: 'OSM (Standard)' },
  { value: 'topo', label: 'Topografisch' },
  { value: 'cyclosm', label: 'CyclOSM' },
  { value: 'satellite', label: 'Satellit' },
  { value: 'dark', label: 'Dark' },
  { value: 'local', label: 'Lokal (Offline)' },
]

function HeatmapSettingsSection() {
  const queryClient = useQueryClient()
  const [localRadius, setLocalRadius] = useState<number | null>(null)
  const [localInterval, setLocalInterval] = useState<number | null>(null)
  const [localThreshold, setLocalThreshold] = useState<number | null>(null)
  const [localFilterMap, setLocalFilterMap] = useState<boolean | null>(null)
  const [localFilterActivity, setLocalFilterActivity] = useState<boolean | null>(null)
  const [defaultBaseStyle, setDefaultBaseStyle] = useState<BaseStyle>(() => {
    try {
      const s = localStorage.getItem('skadia-map-defaults')
      if (s) {
        const p = JSON.parse(s)
        if (p.defaultBaseStyle) return p.defaultBaseStyle as BaseStyle
      }
    } catch { /* ignore */ }
    return 'osm'
  })

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
  const fmtRadius = (m: number) => m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${m} m`

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
    <SettingsSection title="Karteneinstellungen" description="Einstellungen für die Heatmap und Kartenansicht.">

      {/* Standard-Kartenstil */}
      <div>
        <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300 block mb-1">
          Standard-Kartenstil
        </label>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mb-2">
          Wird beim ersten Öffnen verwendet. Während der Sitzung wird der zuletzt verwendete Stil gespeichert.
        </p>
        <select
          value={defaultBaseStyle}
          onChange={e => {
            const val = e.target.value as BaseStyle
            setDefaultBaseStyle(val)
            try {
              const stored = JSON.parse(localStorage.getItem('skadia-map-defaults') || '{}')
              localStorage.setItem('skadia-map-defaults', JSON.stringify({ ...stored, defaultBaseStyle: val }))
            } catch { /* ignore */ }
          }}
          className="w-48 px-3 py-1.5 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100"
        >
          {BASE_STYLE_OPTIONS.map(o => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      </div>

      {/* ── Heatmap & Terra Incognita ── */}
      <div className="border-t border-neutral-200 dark:border-neutral-700 pt-5 space-y-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Heatmap &amp; Terra Incognita</h3>

        <div>
          <div className="flex items-center justify-between mb-2">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            Terra Incognita — Sichtweite
          </label>
          <span className="text-sm text-neutral-500 tabular-nums">{fmtRadius(currentRadius)}</span>
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
          <span>100 m (eng am Track)</span>
          <span>10 km (großzügig)</span>
        </div>
        <p className="text-xs text-neutral-400 mt-2">
          Bestimmt den Radius um jeden Track, in dem die Karte sichtbar ist. Kleinere Werte erfordern mehr Erkundung.
        </p>
        {radiusDirty && (
          <button
            onClick={() => radiusMut.mutate(currentRadius)}
            disabled={radiusMut.isPending}
            className="mt-3 px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >
            {radiusMut.isPending ? 'Speichert...' : 'Sichtweite speichern'}
          </button>
        )}
      </div>

        <div className="mt-4">
          <div className="flex items-center justify-between mb-2">
          <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            Heatmap-Sampling-Intervall
          </label>
          <span className="text-sm text-neutral-500 tabular-nums">{currentInterval} m</span>
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
          <span>10 m (detailliert)</span>
          <span>100 m (schnell)</span>
        </div>
        <p className="text-xs text-neutral-400 mt-2">
          Bestimmt den Abstand zwischen den Sampling-Punkten für die Heatmap. Kleinere Werte erzeugen mehr Punkte und eine detailliertere Heatmap, erhöhen aber die Datenmenge.
        </p>
        </div>
      </div>

      {/* ── Teleportationslücken ── */}
      <div className="border-t border-neutral-200 dark:border-neutral-700 pt-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">Teleportationslücken</h3>
        <div className="flex items-center justify-between mb-1">
          <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
            Teleportationslücken ausblenden
          </p>
          <Toggle
            checked={!!(currentFilterMap || currentFilterActivity)}
            onChange={() => {
              const next = !(currentFilterMap || currentFilterActivity)
              setLocalFilterMap(next)
              setLocalFilterActivity(next)
            }}
            aria-label="Teleportationslücken ausblenden"
          />
        </div>
        <p className="text-xs text-neutral-400 mb-4">
          Filtert gerade Linien heraus, die entstehen wenn die GPS-Aufzeichnung gestoppt und an einem anderen Ort wieder gestartet wurde.
        </p>

        <div className="space-y-2 mb-4">
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={currentFilterMap}
              onChange={(e) => setLocalFilterMap(e.target.checked)}
              className="h-4 w-4 rounded accent-accent-DEFAULT cursor-pointer"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">Karte (Heatmap &amp; Tracks)</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={currentFilterActivity}
              onChange={(e) => setLocalFilterActivity(e.target.checked)}
              className="h-4 w-4 rounded accent-accent-DEFAULT cursor-pointer"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">Aktivitätsdetailansicht</span>
          </label>
        </div>

        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Mindestlückenlänge
            </label>
            <span className="text-sm text-neutral-500 tabular-nums">{currentThreshold} m</span>
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
            <span>100 m</span>
            <span>10 km</span>
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
          {updateHeatmapMut.isPending ? 'Speichert...' : 'Speichern'}
        </button>
      )}

      {updateHeatmapMut.isError && (
        <p className="text-sm text-rose-500">
          Fehler: {updateHeatmapMut.error instanceof Error ? updateHeatmapMut.error.message : 'Unbekannt'}
        </p>
      )}
    </SettingsSection>
  )
}

function ImportSettingsSection() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { isAdmin } = useUser()
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
        addToast({ type: 'info', title: 'Backfill läuft bereits', message: 'Fortschritt im Header-Indikator sichtbar' })
      } else if ((result.queued ?? 0) === 0) {
        addToast({ type: 'success', title: 'Alle Fotos haben bereits einen pHash' })
      } else {
        addToast({ type: 'info', title: 'pHash-Backfill gestartet', message: `${result.queued} Fotos werden verarbeitet — Fortschritt im Header-Indikator sichtbar` })
      }
    } catch {
      addToast({ type: 'error', title: 'Backfill konnte nicht gestartet werden', duration: 0 })
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

  useEffect(() => {
    setLocalGpsDup(s.gps_duplicate_threshold)
    setLocalFusionMin(s.fusion_time_window_min)
    setLocalHausdorff(hausdorff)
    setLocalLengthDiff(lengthDiff)
    setLocalPhashBits(phashBits)
  }, [s.gps_duplicate_threshold, s.fusion_time_window_min, hausdorff, lengthDiff, phashBits])

  return (
    <SettingsSection title="Import" className="space-y-5">

      {/* Konflikt-Strategie */}
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Standard-Verhalten bei Duplikaten
        </label>
        <select
          value={s.conflict_strategy}
          onChange={e => mut.mutate({ ...s, conflict_strategy: e.target.value as ConflictStrategy })}
          className="block w-full max-w-xs border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100"
        >
          <option value="enrich_missing">Fehlende Felder ergänzen (empfohlen)</option>
          <option value="replace_all">Immer ersetzen</option>
          <option value="skip">Immer überspringen</option>
        </select>
        <p className="text-xs text-neutral-400 mt-1">
          Gilt für Einzelupload und Hotfolder. Beim Bulk-Import kann pro Import gewählt werden.
        </p>
      </div>

      {/* GPS-Duplikat-Schwellwert */}
      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>GPS-Duplikat-Schwellwert</span>
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
          <span>60 % (mehr Kandidaten)</span>
          <span>95 % (weniger Kandidaten)</span>
        </div>
        <p className="text-xs text-neutral-400 mt-1">
          Aktivitäten werden als Fusion-Kandidaten vorgeschlagen wenn sie innerhalb des eingestellten Zeitfensters liegen und ihre GPS-Spur mindestens so ähnlich ist. Gleiche Strecken an anderen Tagen werden nicht erkannt.
        </p>
      </div>

      {/* Zeitfenster Fusionserkennung */}
      <div className="space-y-2">
        <div className="flex items-center justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300">
          <span>Zeitfenster Fusionserkennung</span>
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
          <span>5 min (strenger)</span>
          <span>120 min (großzügiger)</span>
        </div>
        <p className="text-xs text-neutral-400 mt-1">
          Maximaler Zeitabstand zwischen zwei Aktivitäten damit sie als Fusionskandidaten erkannt werden. 30 Minuten empfohlen — z.B. für eine Wanderung mit Radcomputer + GPS-Gerät gestartet mit kurzem Versatz.
        </p>
      </div>

      {/* Routen-Duplikaterkennung */}
      <div className="space-y-2 pt-2 border-t border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          Routen-Duplikaterkennung (Komoot / Garmin / Strava)
        </h3>
        <p className="text-xs text-neutral-400">
          Beim Import geplanter Routen werden Duplikate über GPS-Ähnlichkeit erkannt. Zwei Routen gelten als Duplikat wenn ihre maximale Abweichung und Längendifferenz unter den Schwellwerten liegen.
        </p>
      </div>

      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>Max. GPS-Abweichung (Hausdorff)</span>
          <span className="tabular-nums font-mono">{localHausdorff} m</span>
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
          <span>10 m (strenger)</span>
          <span>200 m (großzügiger)</span>
        </div>
      </div>

      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>Max. Längendifferenz</span>
          <span className="tabular-nums font-mono">
            {localLengthDiff < 1 ? `${Math.round(localLengthDiff * 1000)} m` : `${Math.round(localLengthDiff * 10) / 10} km`}
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
          <span>100 m (strenger)</span>
          <span>2 km (großzügiger)</span>
        </div>
      </div>

      {/* Foto-Duplikat-Erkennungsschwelle */}
      <div className="space-y-2 pt-2 border-t border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          Foto-Duplikat-Erkennung
        </h3>
        <p className="text-xs text-neutral-400">
          Fotos gelten als Duplikat wenn ihr pHash-Hamming-Abstand unter dem Schwellwert liegt.
          Niedrigere Werte erkennen nur fast identische Fotos; höhere Werte fassen auch leicht
          bearbeitete oder komprimierte Versionen zusammen.
        </p>
      </div>

      <div>
        <div className="flex justify-between text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
          <span>Foto-Duplikat-Erkennungsschwelle</span>
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
          <span>1 Bit (strenger)</span>
          <span>10 Bits (großzügiger)</span>
        </div>
        <p className="text-xs text-neutral-400 mt-1">Standard: 3 Bits</p>
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
              {phashBackfillPending ? 'Startet…' : 'pHash neu berechnen'}
            </button>
            <p className="text-xs text-neutral-400 dark:text-neutral-500">Geschieht automatisch beim Import. Nur nötig für Fotos, die vor Einführung der Duplikatserkennung importiert wurden.</p>
          </div>
        </div>
      )}
    </SettingsSection>
  )
}

