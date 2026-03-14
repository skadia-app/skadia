import { useState, useMemo, useEffect } from 'react'
import OSMImportModal from '@/components/OSMImportModal'
import { useAccentColor, ACCENT_PALETTE, DEFAULT_ACCENT_COLOR, deriveAccentEntry } from '@/lib/accentColor'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import { CATEGORIES, SPORT_DEFS } from '@/lib/sportTaxonomy'
import ColorPicker from '@/components/ColorPicker'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { Link } from 'react-router-dom'
import { getShadowFSSettings, updateShadowFSSettings, getHotfolderImports, getHeatmapSettings, updateHeatmapSettings } from '@/api/settings'
import { getImportSettings, updateImportSettings, type ImportSettings, type ConflictStrategy } from '@/api/importWizard'
import { getCapabilities, type Capabilities } from '@/api/routes'
import { fetchSportTypeStats } from '@/api/stats'
import { apiFetch } from '@/lib/auth'
import { type OSMStatus } from '@/api/osm'
import { OSMRegionManager } from '@/components/OSMRegionManager'
import { DEMSettings } from '@/components/DEMSettings'
import { OfflineMapSettings } from '@/components/OfflineMapSettings'
import { fetchEnrichmentSettings, updateEnrichmentSettings } from '@/api/enrichment'
import { recomputePhotoPreviews, recomputePhotoEmbeddings, backfillPhash } from '@/api/activityPhotos'
import { useToast } from '@/lib/toast'
import SchemaEditor from '@/components/SchemaEditor'
import RenamePreview from '@/components/RenamePreview'
import LLMSettingsPage from './LLMSettingsPage'
import ChatProfileSettings from '@/components/ChatProfileSettings'

// Stage-2 ML enrichment module configuration — backed by /settings/enrichment API.
const STAGE2_MODULES = [
  { key: 'peaks',       apiField: 'enable_peaks'       as const, label: 'Gipfel & Pässe',      description: 'Erkennt besuchte Gipfel und Bergsättel' },
  { key: 'forests',     apiField: 'enable_forests'     as const, label: 'Wälder',              description: 'Erkennt durchquerte Waldgebiete' },
  { key: 'water',       apiField: 'enable_water'       as const, label: 'Gewässer',             description: 'Erkennt Seen und Fließgewässer entlang der Route' },
  { key: 'settlements', apiField: 'enable_settlements' as const, label: 'Ortschaften',          description: 'Erkennt durchquerte Siedlungen' },
  { key: 'borders',     apiField: 'enable_borders'     as const, label: 'Grenzüberquerungen',  description: 'Erkennt überquerte Ländergrenzen' },
  { key: 'stops',       apiField: 'enable_stops'       as const, label: 'Stopps & Einkehr',     description: 'Erkennt Aufenthalte und Restaurantbesuche (≥10 min)' },
  { key: 'lifts',       apiField: 'enable_lifts'       as const, label: 'Lifte / Seilbahnen',  description: 'Erkennt Gondelbahnen und Sessellifte' },
  { key: 'surfaces',    apiField: 'enable_surfaces'    as const, label: 'Untergrundtypen',      description: 'Erkennt Asphalt, Schotter, Trail etc.' },
  { key: 'ascents',     apiField: 'enable_ascents'     as const, label: 'Anstiege',             description: 'Erkennt und klassifiziert Anstiege' },
  { key: 'commute',     apiField: 'enable_commute'     as const, label: 'Pendelrouten',         description: 'Erkennt regelmäßig genutzte Routen' },
] as const

function Stage2EnrichmentSection() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data: settings, isLoading, isError } = useQuery({
    queryKey: ['enrichment-settings'],
    queryFn: fetchEnrichmentSettings,
  })

  const mutation = useMutation({
    mutationFn: (patch: Parameters<typeof updateEnrichmentSettings>[0]) =>
      updateEnrichmentSettings(patch),
    onSuccess: (updated) => {
      queryClient.setQueryData(['enrichment-settings'], updated)
    },
    onError: () => {
      queryClient.invalidateQueries({ queryKey: ['enrichment-settings'] })
      addToast({ type: 'error', title: 'Einstellung konnte nicht gespeichert werden', duration: 0 })
    },
  })

  function toggle(apiField: typeof STAGE2_MODULES[number]['apiField']) {
    if (!settings) return
    const current = settings[apiField] !== false // default true when field absent
    mutation.mutate({ [apiField]: !current })
  }

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Automatische Analysen</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Analysen, die nach dem Upload automatisch im Hintergrund durchgeführt werden.
        </p>
      </div>

      {isError && (
        <p className="text-sm text-red-500 dark:text-red-400">
          Einstellungen konnten nicht geladen werden.
        </p>
      )}

      <div className="space-y-2">
        {STAGE2_MODULES.map(({ apiField, label, description }) => {
          const enabled = settings ? (settings[apiField] !== false) : true
          const isPending = mutation.isPending
          return (
            <div key={apiField} className="flex items-center justify-between gap-4 py-1">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{label}</span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {description}</span>
              </div>
              <button
                onClick={() => toggle(apiField)}
                disabled={isLoading || isPending}
                className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                style={{ backgroundColor: enabled ? 'var(--color-accent-DEFAULT, #f97316)' : '#d4d4d4' }}
                role="switch"
                aria-checked={enabled}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    enabled ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>
          )
        })}
      </div>

      <p className="text-xs text-neutral-400 dark:text-neutral-500 pt-1 border-t border-neutral-100 dark:border-neutral-800/60">
        &Auml;nderungen gelten f&uuml;r neue Aktivit&auml;ten sowie bei manueller Neuberechnung.
      </p>
    </section>
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
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data: settings, isLoading } = useQuery({
    queryKey: ['enrichment-settings'],
    queryFn: fetchEnrichmentSettings,
  })

  const mutation = useMutation({
    mutationFn: (size: string) => updateEnrichmentSettings({ photo_preview_size: size }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['enrichment-settings'], updated)
      addToast({ type: 'success', title: 'Einstellung gespeichert' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Einstellung konnte nicht gespeichert werden', duration: 0 })
    },
  })

  const framesMutation = useMutation({
    mutationFn: (n: number) => updateEnrichmentSettings({ video_clip_frames: n }),
    onSuccess: (updated) => {
      setFramesLocal(null)
      queryClient.setQueryData(['enrichment-settings'], updated)
    },
    onError: () => {
      addToast({ type: 'error', title: 'Einstellung konnte nicht gespeichert werden', duration: 0 })
    },
  })

  const [framesLocal, setFramesLocal] = useState<number | null>(null)

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
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Foto-Vorschaugröße</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Maximale Auflösung für Galerie-Ansichten. Nur wenn das Original größer ist, wird ein separates Vorschau-Bild erstellt.
        </p>
      </div>

      <div className="inline-flex rounded-lg border border-neutral-200 dark:border-neutral-700 overflow-hidden">
        {PREVIEW_SIZE_OPTIONS.map(({ value, label }, idx) => (
          <button
            key={value}
            onClick={() => mutation.mutate(value)}
            disabled={isLoading || mutation.isPending}
            className={`px-4 py-1.5 text-sm font-medium whitespace-nowrap transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
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
              onMouseUp={e => framesMutation.mutate(Number((e.target as HTMLInputElement).value))}
              onTouchEnd={e => framesMutation.mutate(Number((e.target as HTMLInputElement).value))}
              className="flex-1 accent-accent-DEFAULT cursor-pointer disabled:opacity-50"
            />
            <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100 w-8 text-center tabular-nums">
              {currentFrames}
            </span>
          </div>
        </div>
      </div>

      <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60 space-y-3">
        {[
          { label: 'Vorschauen neu erstellen', onClick: handleRecompute, desc: 'Generiert Vorschau-Bilder für alle vorhandenen Fotos mit der aktuellen Einstellung neu.' },
          { label: 'Bildsuche neu indexieren', onClick: handleRecomputeEmbeddings, desc: 'Berechnet KI-Embeddings für alle Fotos (ermöglicht semantische Suche nach Inhalt).' },
        ].map(({ label, onClick, desc }) => (
          <div key={label} className="flex items-center gap-3">
            <button
              onClick={onClick}
              disabled={isLoading}
              className="whitespace-nowrap px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors min-w-[200px]"
            >
              {label}
            </button>
            <p className="text-xs text-neutral-400 dark:text-neutral-500">{desc}</p>
          </div>
        ))}
      </div>
    </section>
  )
}

// -----------------------------------------------------------------------
// ML-Service Section
// -----------------------------------------------------------------------

const NIGHTLY_JOBS = [
  {
    key: 'nightly_feature_engineering',
    label: 'Feature Engineering',
    description: 'NeuroKit2 HRV-Metriken + TSFEL-Features',
  },
  {
    key: 'nightly_tsfm_analysis',
    label: 'TSFM-Analyse',
    description: 'TSPulse Anomalie-Erkennung + TTM Prognosen',
  },
  {
    key: 'nightly_changepoint_detection',
    label: 'Changepoint-Erkennung',
    description: 'ruptures PELT Changepoints in Trainingsdaten',
  },
] as const

type NightlyJobKey = typeof NIGHTLY_JOBS[number]['key']

function MLServiceSection() {
  const { data: mlStatus } = useQuery({
    queryKey: ['ml-status'],
    queryFn: () =>
      apiFetch('/api/v1/ml/status').then(r => r.json()) as Promise<{ available: boolean }>,
    refetchInterval: 30_000,
  })

  const { data: mlSettings, refetch: refetchML } = useQuery({
    queryKey: ['ml-system-settings'],
    queryFn: () =>
      apiFetch('/api/v1/settings/ml').then(r => r.json()) as Promise<
        Record<NightlyJobKey, { enabled: boolean; time: string; description: string }>
      >,
  })

  async function toggleJob(key: NightlyJobKey, enabled: boolean) {
    const current = mlSettings?.[key] ?? { enabled: false, time: '02:00', description: '' }
    await apiFetch(`/api/v1/settings/ml/${key}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...current, enabled }),
    })
    refetchML()
  }

  const isAvailable = mlStatus?.available ?? false

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">ML-Service</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Status des ML-Dienstes und n&auml;chtliche Analysejobs.
          </p>
        </div>
        <span
          className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium flex-shrink-0 ${
            isAvailable
              ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
              : 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
          }`}
        >
          <span
            className={`h-2 w-2 rounded-full ${isAvailable ? 'bg-emerald-500' : 'bg-neutral-400'}`}
          />
          {isAvailable ? 'Erreichbar' : 'Nicht erreichbar'}
        </span>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          N&auml;chtliche Jobs
        </p>
        {NIGHTLY_JOBS.map(({ key, label, description }) => {
          const enabled = mlSettings?.[key]?.enabled ?? false
          const time = mlSettings?.[key]?.time ?? '—'
          return (
            <div key={key} className="flex items-center justify-between gap-4 py-1">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{label}</span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">
                  — {description}
                </span>
                <span className="text-xs text-neutral-400 ml-2">({time} Uhr)</span>
              </div>
              <button
                onClick={() => toggleJob(key, !enabled)}
                className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0 cursor-pointer"
                style={{
                  backgroundColor: enabled ? 'var(--color-accent-DEFAULT, #f97316)' : '#d4d4d4',
                }}
                role="switch"
                aria-checked={enabled}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    enabled ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>
          )
        })}
      </div>
    </section>
  )
}

// -----------------------------------------------------------------------
// Notification Settings Section
// -----------------------------------------------------------------------

const NOTIFICATION_TOGGLES = [
  { key: 'notify_enrichment', label: 'Enrichment abgeschlossen', description: 'Nach jeder Stage-2-Analyse' },
  { key: 'notify_anomaly', label: 'Anomalie erkannt', description: 'Bei ungewöhnlichen Trainingswerten' },
  { key: 'notify_gear', label: 'Ausrüstungs-Alerts', description: 'Wartungshinweise und Verschleiß' },
  { key: 'notify_chat', label: 'Assistent-Antworten', description: 'Wenn der KI-Assistent antwortet' },
] as const

type NotifyKey = typeof NOTIFICATION_TOGGLES[number]['key']

function NotificationSettingsSection() {
  const { data: userSettings, refetch: refetchUser } = useQuery({
    queryKey: ['ml-user-settings'],
    queryFn: () =>
      apiFetch('/api/v1/settings/ml/user').then(r => r.json()) as Promise<
        Record<NotifyKey, boolean>
      >,
  })

  const { data: devices, refetch: refetchDevices } = useQuery({
    queryKey: ['devices'],
    queryFn: () => apiFetch('/api/v1/devices').then(r => r.json()) as Promise<
      { id: string; platform: string; device_name?: string | null; created_at: string }[]
    >,
  })

  async function toggleNotify(key: NotifyKey, value: boolean) {
    await apiFetch(`/api/v1/settings/ml/user/${key}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(value),
    })
    refetchUser()
  }

  async function removeDevice(id: string) {
    await apiFetch(`/api/v1/devices/${id}`, { method: 'DELETE' })
    refetchDevices()
  }

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Benachrichtigungen</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Welche Ereignisse sollen als Benachrichtigung erscheinen.
        </p>
      </div>

      <div className="space-y-2">
        {NOTIFICATION_TOGGLES.map(({ key, label, description }) => {
          const enabled = userSettings?.[key] ?? false
          return (
            <div key={key} className="flex items-center justify-between gap-4 py-1">
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{label}</span>
                <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {description}</span>
              </div>
              <button
                onClick={() => toggleNotify(key, !enabled)}
                className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0 cursor-pointer"
                style={{
                  backgroundColor: enabled ? 'var(--color-accent-DEFAULT, #f97316)' : '#d4d4d4',
                }}
                role="switch"
                aria-checked={enabled}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    enabled ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>
          )
        })}
      </div>

      {/* Registered devices */}
      {devices && devices.length > 0 && (
        <div className="pt-3 border-t border-neutral-100 dark:border-neutral-800/60 space-y-2">
          <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
            Registrierte Ger&auml;te
          </p>
          <div className="space-y-1.5">
            {devices.map(device => (
              <div
                key={device.id}
                className="flex items-center justify-between gap-3 py-1.5 px-3 rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-100 dark:border-neutral-800"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-sm">{device.platform === 'ios' ? '🍎' : '🤖'}</span>
                  <span className="text-sm text-neutral-800 dark:text-neutral-200 truncate">
                    {device.device_name ?? (device.platform === 'ios' ? 'iPhone/iPad' : 'Android-Gerät')}
                  </span>
                  <span className="text-xs text-neutral-400 flex-shrink-0">
                    {new Date(device.created_at).toLocaleDateString('de-DE')}
                  </span>
                </div>
                <button
                  onClick={() => removeDevice(device.id)}
                  className="text-xs text-red-500 hover:text-red-600 dark:hover:text-red-400 transition-colors flex-shrink-0"
                >
                  Entfernen
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  )
}

export default function SettingsPage() {
  const queryClient = useQueryClient()
  const { accent, setAccent } = useAccentColor()
  const [renameOpen, setRenameOpen] = useState(false)
  const [schemaValue, setSchemaValue] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'general' | 'ai'>('general')

  const { data: settings, isLoading } = useQuery({
    queryKey: ['shadow-fs-settings'],
    queryFn: getShadowFSSettings,
  })

  const { data: imports } = useQuery({
    queryKey: ['hotfolder-imports'],
    queryFn: getHotfolderImports,
  })

  // Lokaler Schema-Wert oder vom Server
  const currentSchema = schemaValue ?? settings?.schema ?? ''
  const isDirty = schemaValue !== null && schemaValue !== settings?.schema

  const updateMutation = useMutation({
    mutationFn: updateShadowFSSettings,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shadow-fs-settings'] })
      setSchemaValue(null)
    },
  })

  const toggleMutation = useMutation({
    mutationFn: (enabled: boolean) => updateShadowFSSettings({ hotfolder_enabled: enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['shadow-fs-settings'] })
    },
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Einstellungen...</div>
  }

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Einstellungen</h1>

      {/* Tab-Navigation */}
      <div className="flex gap-1 p-1 bg-neutral-100 dark:bg-neutral-800/50 rounded-lg w-fit">
        {[
          { id: 'general' as const, label: 'Allgemein' },
          { id: 'ai' as const, label: 'KI & ML' },
        ].map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-1.5 rounded-md text-sm font-medium transition-colors ${
              activeTab === tab.id
                ? 'bg-white dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100 shadow-sm'
                : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === 'ai' && <>
        <LLMSettingsPage />
        <Stage2EnrichmentSection />
        <MLServiceSection />
        <NotificationSettingsSection />
        <ChatProfileSettings />
      </>}

      {activeTab === 'general' && <>
      <div className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200 dark:border-neutral-700 px-4 py-3 text-sm text-neutral-600 dark:text-neutral-400">
        Einstellung nicht gefunden? Einstellungen zur Medienverwaltung unter{' '}
        <Link to="/workshop" className="text-accent-DEFAULT hover:text-accent-600 font-medium">Werkstatt</Link>.
      </div>

      {/* Erscheinungsbild */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Erscheinungsbild</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Wähle eine Akzentfarbe für die Benutzeroberfläche.
          </p>
        </div>
        <ColorPicker
          value={accent.shade500}
          onChange={hex => setAccent(deriveAccentEntry(hex))}
        />
        {accent.shade500.toLowerCase() !== DEFAULT_ACCENT_COLOR.toLowerCase() && (
          <button
            onClick={() => setAccent(ACCENT_PALETTE[0])}
            className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 underline transition-colors"
          >
            Auf Standard zurücksetzen
          </button>
        )}
      </section>

      <PhotoPreviewSection />

      {/* Sportarten */}
      <SportSettingsSection />

      {/* Dateiablage */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Dateiablage</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Schema für die Ordnerstruktur im Shadow File System. Variablen werden durch Werte der jeweiligen Aktivität ersetzt.
          </p>
        </div>

        <SchemaEditor
          value={currentSchema}
          onChange={setSchemaValue}
          onSave={() => updateMutation.mutate({ schema: currentSchema })}
          isSaving={updateMutation.isPending}
          isDirty={isDirty}
        />

        {updateMutation.isError && (
          <p className="text-sm text-rose-500">
            Fehler beim Speichern: {updateMutation.error instanceof Error ? updateMutation.error.message : 'Unbekannt'}
          </p>
        )}

        <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60">
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mb-3">
            Bestehende Dateien nach dem aktuellen Schema umbenennen und verschieben.
          </p>
          <button
            onClick={() => setRenameOpen(true)}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white
                       bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
          >
            Vorschau anzeigen
          </button>
        </div>
      </section>

      {/* Hotfolder */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div className="flex items-start justify-between">
          <div>
            <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Hotfolder</h2>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              Automatischer Import neuer Dateien aus einem Verzeichnis.
            </p>
          </div>

          {/* Toggle */}
          <button
            onClick={() => toggleMutation.mutate(!settings?.hotfolder_enabled)}
            disabled={toggleMutation.isPending}
            className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0"
            style={{ backgroundColor: settings?.hotfolder_enabled ? 'var(--color-orange-500)' : '#d4d4d4' }}
            role="switch"
            aria-checked={settings?.hotfolder_enabled}
          >
            <span
              className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                settings?.hotfolder_enabled ? 'translate-x-6' : 'translate-x-1'
              }`}
            />
          </button>
        </div>

        {/* Pfad */}
        <div className="flex items-center gap-2 text-sm">
          <span className="text-neutral-500 dark:text-neutral-400">Pfad:</span>
          <span className="font-mono text-neutral-800 dark:text-neutral-200 bg-neutral-50 dark:bg-neutral-800 px-2 py-1 rounded">
            {settings?.hotfolder_path || '(nicht konfiguriert)'}
          </span>
        </div>

        {/* Status */}
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

        {/* Letzte Imports */}
        {imports && imports.length > 0 && (
          <div>
            <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">Letzte Imports</h3>
            <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-neutral-50 dark:bg-neutral-900/50 border-b border-neutral-200 dark:border-neutral-800">
                    <th className="text-left px-3 py-2 text-neutral-500 dark:text-neutral-400 font-medium">Datei</th>
                    <th className="text-left px-3 py-2 text-neutral-500 dark:text-neutral-400 font-medium">Status</th>
                    <th className="text-left px-3 py-2 text-neutral-500 dark:text-neutral-400 font-medium">Zeitpunkt</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
                  {imports.map((imp) => (
                    <tr key={imp.id} className="even:bg-neutral-50/50 dark:even:bg-neutral-800/20">
                      <td className="px-3 py-2 font-mono text-xs text-neutral-800 dark:text-neutral-200 truncate max-w-xs">
                        {imp.filename}
                      </td>
                      <td className="px-3 py-2">
                        <ImportStatusBadge status={imp.status} />
                      </td>
                      <td className="px-3 py-2 text-neutral-500 dark:text-neutral-400 text-xs">
                        {format(new Date(imp.created_at), 'dd. MMM yyyy HH:mm', { locale: de })}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </section>

      {/* Enrichment Stage 1 */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Enrichment</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Automatische Anreicherung von Aktivitäten mit Wetter, Standort und Tags.
          </p>
        </div>
        <Link
          to="/settings/enrichment"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          Enrichment-Einstellungen
        </Link>
      </section>


      {/* Automatisierung */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Automatisierung</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Regeln für automatische Zuweisungen und Tags.
          </p>
        </div>
        <Link
          to="/settings/rules"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          Automatisierungsregeln
        </Link>
      </section>

      {/* Datenimport */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Datenimport</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Importiere Aktivitäten, Ausrüstung und Gesundheitsdaten aus einem Garmin- oder Strava-Export.
          </p>
        </div>
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
        </div>
      </section>

      {/* Privatsphäre */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Privatsphäre</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Definiere Zonen, in denen GPS-Daten automatisch aus öffentlichen Ansichten ausgeblendet werden.
          </p>
        </div>
        <Link
          to="/settings/privacy"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 transition-colors"
        >
          Private Zonen verwalten
        </Link>
      </section>

      {/* Import-Einstellungen */}
      <ImportSettingsSection />

      {/* Karte */}
      <HeatmapSettingsSection />

      {/* Lokale Karten & Geodaten */}
      <LocalMapAndGeoSection />

      <RenamePreview open={renameOpen} onClose={() => setRenameOpen(false)} schema={currentSchema} />
      </>}
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
        className="w-full flex items-center justify-between px-6 py-4 text-left hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors"
      >
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Sportarten</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-0.5">
            Farben pro Sportart, Filtereinstellungen.
          </p>
        </div>
        {/* Chevron */}
        <svg
          className={`w-5 h-5 text-neutral-400 flex-shrink-0 transition-transform duration-200 ${colorsOpen ? 'rotate-180' : ''}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
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

// ─── Capabilities Badge ──────────────────────────────────────────────────────

function CapBadge({ active, label }: { active: boolean; label: string }) {
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${
      active
        ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
        : 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
    }`}>
      <span className={`h-1.5 w-1.5 rounded-full flex-shrink-0 ${active ? 'bg-emerald-500' : 'bg-neutral-400'}`} />
      {label}
    </span>
  )
}

// ─── Unified "Lokale Karten & Geodaten" Section ──────────────────────────────

function LocalMapAndGeoSection() {
  const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? '/tiles'
  const styleUrl = `${martinBase}/style.json`

  // Martin status
  const [martinStatus, setMartinStatus] = useState<'loading' | 'ok' | 'unavailable'>('loading')
  const [martinSources, setMartinSources] = useState<string[]>([])
  const [checkKey, setCheckKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    setMartinStatus('loading')
    setMartinSources([])
    fetch(styleUrl, { signal: AbortSignal.timeout(5000) })
      .then(r => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((json: { sources?: Record<string, unknown> }) => {
        if (cancelled) return
        setMartinStatus('ok')
        if (json.sources && typeof json.sources === 'object') {
          setMartinSources(Object.keys(json.sources))
        }
      })
      .catch(() => {
        if (!cancelled) setMartinStatus('unavailable')
      })
    return () => { cancelled = true }
  }, [styleUrl, checkKey])

  // Capabilities
  const { data: caps, isLoading: capsLoading } = useQuery<Capabilities>({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 60_000,
  })

  // OSM status
  const [showImportModal, setShowImportModal] = useState(false)
  const { data: osmStatus } = useQuery<OSMStatus>({
    queryKey: ['osm-status'],
    queryFn: () => apiFetch('/api/v1/admin/osm-status').then(r => r.json()),
  })
  const importLifts = useMutation({
    mutationFn: () => apiFetch('/api/v1/segments/import-lifts', { method: 'POST' }).then(r => r.json()),
  })
  const hasOsmData = osmStatus?.has_data ?? false

  // Trail topology
  const { addToast } = useToast()
  const buildTopology = useMutation({
    mutationFn: () => apiFetch('/api/v1/trails/build-topology', { method: 'POST' }).then(r => r.json()),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Trail-Topologie wird aufgebaut', message: 'Der Prozess läuft im Hintergrund.' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Aufbau der Topologie', duration: 0 })
    },
  })

  const martinBadge =
    martinStatus === 'loading' ? (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
        <span className="h-1.5 w-1.5 rounded-full bg-neutral-400 animate-pulse" />
        Prüfe…
      </span>
    ) : martinStatus === 'ok' ? (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
        <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
        Verbunden
      </span>
    ) : (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
        <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
        Nicht verfügbar
      </span>
    )

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Lokale Karten &amp; Geodaten</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Routing, Offline-Karte (Martin), OSM-Daten und Höhenmodell.
        </p>
      </div>

      {/* ── Routing-Dienste ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Routing-Dienste</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3">
          {capsLoading ? (
            <p className="text-xs text-neutral-400">Prüfe Verfügbarkeit…</p>
          ) : (
            <>
              <div className="flex flex-wrap gap-2">
                <CapBadge active={caps?.routing ?? false}           label="Routing (BRouter)" />
                <CapBadge active={caps?.isochrone ?? false}         label="Isochrone" />
                <CapBadge active={caps?.dem_correction ?? false}    label="DEM-Höhenkorrektur" />
                <CapBadge active={caps?.surface_enrichment ?? false} label="Oberflächen-Anreicherung" />
                <CapBadge active={caps?.route_suggestion ?? false}  label="Routen-Vorschläge" />
                <CapBadge active={caps?.offline_map ?? false}       label="Offline-Karte" />
              </div>
              {!(caps?.routing) && (
                <p className="text-xs text-amber-600 dark:text-amber-400 leading-relaxed">
                  Routing ist nicht verfügbar. Stelle sicher, dass der BRouter-Dienst gestartet ist und OSM-Daten importiert wurden.
                </p>
              )}
            </>
          )}
          <button
            onClick={() => buildTopology.mutate()}
            disabled={buildTopology.isPending || !hasOsmData}
            className="mt-1 w-full py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {buildTopology.isPending ? 'Aufbau läuft…' : 'Trail-Topologie aufbauen'}
          </button>
          {!hasOsmData && (
            <p className="text-xs text-neutral-400">OSM-Daten müssen zuerst importiert werden.</p>
          )}
        </div>
      </div>

      {/* ── DEM-Höhendaten ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">DEM-Höhendaten</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-2">
          <CapBadge active={caps?.dem_correction ?? false} label="DEM aktiv" />
          <DEMSettings />
        </div>
      </div>

      {/* ── Offline-Karte (Martin) ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Offline-Karte (Martin Tile Server)</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3">
          {/* Status line */}
          <div className="flex items-center gap-3 flex-wrap">
            {martinBadge}
            <span className="text-xs text-neutral-400 dark:text-neutral-500 font-mono break-all">{styleUrl}</span>
            {martinStatus !== 'loading' && (
              <button
                onClick={() => setCheckKey(k => k + 1)}
                className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 underline underline-offset-2 cursor-pointer"
              >
                Erneut prüfen
              </button>
            )}
          </div>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 leading-relaxed">
            Der Martin Tile Server serviert Vektorkacheln aus PostGIS direkt und ermöglicht vollständigen
            Offline-Betrieb. URL via{' '}
            <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">VITE_MARTIN_URL</code>{' '}
            (Standard: <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">/tiles</code>).
          </p>
          {/* Available tile sources */}
          {martinStatus === 'ok' && martinSources.length > 0 && (
            <div>
              <p className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1.5">
                Tile-Quellen ({martinSources.length})
              </p>
              <ul className="space-y-1">
                {martinSources.map(src => (
                  <li key={src} className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
                    <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 flex-shrink-0" />
                    <code className="font-mono">{src}</code>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {/* Hint */}
          <div className="flex items-start gap-2 rounded-lg bg-accent-100/40 dark:bg-accent-900/10 border border-accent-200/60 dark:border-accent-800/30 px-3 py-2.5">
            <svg className="h-4 w-4 text-accent-DEFAULT mt-0.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M13 16h-1v-4h-1m1-4h.01M12 2a10 10 0 100 20A10 10 0 0012 2z" />
            </svg>
            <p className="text-xs text-neutral-600 dark:text-neutral-400 leading-relaxed">
              Den Lokal-Stil als Standardkarte einrichten: Wähle{' '}
              <strong className="text-neutral-800 dark:text-neutral-200">Lokal (Offline)</strong>{' '}
              im Dropdown <em>Standard-Kartenstil</em> im Abschnitt{' '}
              <strong className="text-neutral-800 dark:text-neutral-200">Karteneinstellungen</strong> oben.
              {martinStatus !== 'ok' && (
                <span className="text-amber-600 dark:text-amber-400">
                  {' '}Der Martin Server muss erreichbar sein.
                </span>
              )}
            </p>
          </div>
        </div>
      </div>

      {/* ── Offline-Karte (Protomaps PMTiles) ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Offline-Karte (Protomaps)</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4">
          <OfflineMapSettings />
        </div>
      </div>

      {/* ── OSM-Geodaten ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">OSM-Geodaten</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3 text-sm">
          {/* Stats grid — POIs */}
          {osmStatus && (
            <div className="space-y-3">
              {/* POI-Kategorien */}
              {osmStatus.categories && Object.keys(osmStatus.categories).length > 0 && (
                <div>
                  <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">POIs</p>
                  <div className="grid grid-cols-3 gap-2 text-center">
                    {Object.entries(osmStatus.categories).map(([key, count]) => (
                      <div key={key} className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{count.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400 capitalize">{key}</div>
                      </div>
                    ))}
                    {/* Legacy fallback when categories is empty */}
                    {Object.keys(osmStatus.categories).length === 0 && (
                      <>
                        {osmStatus.peaks !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.peaks.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Gipfel</div></div>}
                        {osmStatus.passes !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.passes.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Pässe</div></div>}
                        {osmStatus.settlements !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.settlements.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Ortschaften</div></div>}
                        {osmStatus.amenities !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.amenities.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Einkehr</div></div>}
                      </>
                    )}
                  </div>
                </div>
              )}
              {/* Wege */}
              {(osmStatus.lifts > 0 || osmStatus.pistes > 0 || osmStatus.ways > 0 || osmStatus.routes > 0 || osmStatus.railways > 0) && (
                <div>
                  <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">Wege</p>
                  <div className="grid grid-cols-3 gap-2 text-center">
                    {osmStatus.ways > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.ways.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Wege</div>
                      </div>
                    )}
                    {osmStatus.routes > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.routes.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Routen</div>
                      </div>
                    )}
                    {osmStatus.lifts > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.lifts.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Lifte</div>
                      </div>
                    )}
                    {osmStatus.pistes > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.pistes.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Pisten</div>
                      </div>
                    )}
                    {osmStatus.railways > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-3 text-center">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.railways.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Eisenbahn</div>
                      </div>
                    )}
                  </div>
                </div>
              )}
              {/* Flächen */}
              {(osmStatus.waterways > 0 || osmStatus.forests > 0 || osmStatus.glaciers > 0 || osmStatus.wetlands > 0 || osmStatus.boundaries > 0 || osmStatus.landuse > 0) && (
                <div>
                  <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">Flächen</p>
                  <div className="grid grid-cols-3 gap-2 text-center">
                    {osmStatus.waterways > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.waterways.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Gewässer</div>
                      </div>
                    )}
                    {osmStatus.forests > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.forests.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Wälder</div>
                      </div>
                    )}
                    {osmStatus.glaciers > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.glaciers.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Gletscher</div>
                      </div>
                    )}
                    {osmStatus.wetlands > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.wetlands.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Feuchtgebiete</div>
                      </div>
                    )}
                    {osmStatus.boundaries > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.boundaries.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Grenzen</div>
                      </div>
                    )}
                    {osmStatus.landuse > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-3 text-center">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.landuse.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Landnutzung</div>
                      </div>
                    )}
                  </div>
                </div>
              )}
              {/* Fallback: no data yet */}
              {!osmStatus.has_data && (
                <p className="text-xs text-neutral-400 dark:text-neutral-500 text-center py-2">Keine OSM-Daten importiert.</p>
              )}
            </div>
          )}
          {/* Importierte Regionen */}
          <OSMRegionManager hasOsmData={hasOsmData} />

          {/* Buttons */}
          <div className="flex flex-col gap-2">
            <button
              onClick={() => setShowImportModal(true)}
              className="w-full py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 transition-colors cursor-pointer"
            >
              Import-Assistent
            </button>
            <button
              onClick={() => importLifts.mutate()}
              disabled={importLifts.isPending || !hasOsmData}
              className="w-full py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              {importLifts.isPending ? 'Importiere…' : 'Lift-Segmente importieren'}
            </button>
          </div>
          {importLifts.isSuccess && (
            <p className="text-xs text-emerald-600 dark:text-emerald-400">
              {(importLifts.data as { imported: number }).imported} Lift-Segmente importiert.
            </p>
          )}
        </div>
      </div>

      {showImportModal && <OSMImportModal onClose={() => setShowImportModal(false)} />}
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
  const { data: enrichSettings } = useQuery({
    queryKey: ['enrichment-settings'],
    queryFn: fetchEnrichmentSettings,
  })
  const radiusMut = useMutation({
    mutationFn: (val: number) => updateEnrichmentSettings({ terra_incognita_radius_m: val }),
    onSuccess: (updated) => {
      queryClient.setQueryData(['enrichment-settings'], updated)
      queryClient.invalidateQueries({ queryKey: ['terra-incognita'] })
      setLocalRadius(null)
    },
  })
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
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Karteneinstellungen</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Einstellungen für die Heatmap und Kartenansicht.
        </p>
      </div>

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
            className="mt-3 px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 transition-colors cursor-pointer"
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
          <button
            onClick={() => {
              const next = !(currentFilterMap || currentFilterActivity)
              setLocalFilterMap(next)
              setLocalFilterActivity(next)
            }}
            className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0 cursor-pointer"
            style={{ backgroundColor: (currentFilterMap || currentFilterActivity) ? 'var(--color-orange-500)' : '#d4d4d4' }}
            role="switch"
            aria-checked={currentFilterMap || currentFilterActivity}
          >
            <span
              className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                (currentFilterMap || currentFilterActivity) ? 'translate-x-6' : 'translate-x-1'
              }`}
            />
          </button>
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
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 transition-colors cursor-pointer"
        >
          {updateHeatmapMut.isPending ? 'Speichert...' : 'Speichern'}
        </button>
      )}

      {updateHeatmapMut.isError && (
        <p className="text-sm text-rose-500">
          Fehler: {updateHeatmapMut.error instanceof Error ? updateHeatmapMut.error.message : 'Unbekannt'}
        </p>
      )}
    </section>
  )
}

function ImportSettingsSection() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
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

  const { data: enrichSettings } = useQuery({
    queryKey: ['enrichment-settings'],
    queryFn: fetchEnrichmentSettings,
  })
  const enrichMut = useMutation({
    mutationFn: (patch: Parameters<typeof updateEnrichmentSettings>[0]) => updateEnrichmentSettings(patch),
    onSuccess: (updated) => queryClient.setQueryData(['enrichment-settings'], updated),
    onError: () => queryClient.invalidateQueries({ queryKey: ['enrichment-settings'] }),
  })

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

  useEffect(() => { setLocalGpsDup(s.gps_duplicate_threshold) }, [s.gps_duplicate_threshold])
  useEffect(() => { setLocalFusionMin(s.fusion_time_window_min) }, [s.fusion_time_window_min])
  useEffect(() => { setLocalHausdorff(hausdorff) }, [hausdorff])
  useEffect(() => { setLocalLengthDiff(lengthDiff) }, [lengthDiff])
  useEffect(() => { setLocalPhashBits(phashBits) }, [phashBits])

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-5">
      <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Import</h2>

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
          onBlur={() => mut.mutate({ ...s, gps_duplicate_threshold: localGpsDup })}
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
          onBlur={() => mut.mutate({ ...s, fusion_time_window_min: localFusionMin })}
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

      {/* pHash Backfill */}
      <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60">
        <div className="flex items-center gap-3">
          <button
            onClick={handlePhashBackfill}
            disabled={phashBackfillPending}
            className="whitespace-nowrap px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors min-w-[200px]"
          >
            {phashBackfillPending ? 'Startet…' : 'pHash neu berechnen'}
          </button>
          <p className="text-xs text-neutral-400 dark:text-neutral-500">Geschieht automatisch beim Import. Nur nötig für Fotos, die vor Einführung der Duplikatserkennung importiert wurden.</p>
        </div>
      </div>
    </section>
  )
}

function ImportStatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    ok: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
    error: 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-400',
    duplicate: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
  }

  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${styles[status] ?? 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-400'}`}>
      {status}
    </span>
  )
}
