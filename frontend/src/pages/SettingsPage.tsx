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
import { fetchSportTypeStats } from '@/api/stats'
import { apiFetch } from '@/lib/auth'
import { fetchEnrichmentSettings, updateEnrichmentSettings } from '@/api/enrichment'
import { useToast } from '@/lib/toast'
import SchemaEditor from '@/components/SchemaEditor'
import RenamePreview from '@/components/RenamePreview'
import LLMSettingsPage from './LLMSettingsPage'
import ChatProfileSettings from '@/components/ChatProfileSettings'

function OSMStatusSection() {
  const [showImportModal, setShowImportModal] = useState(false)

  const { data: osmStatus } = useQuery({
    queryKey: ['osm-status'],
    queryFn: () => apiFetch('/api/v1/admin/osm-status').then(r => r.json()) as Promise<{
      peaks: number; passes: number; lifts: number; settlements: number; amenities?: number; has_data: boolean
    }>,
  })

  const importLifts = useMutation({
    mutationFn: () => apiFetch('/api/v1/segments/import-lifts', { method: 'POST' }).then(r => r.json()),
  })

  const hasData = osmStatus?.has_data ?? false

  return (
    <div className="space-y-3 text-sm">
      {/* Stats grid */}
      <div className={`grid gap-3 text-center ${osmStatus?.amenities !== undefined ? 'grid-cols-5' : 'grid-cols-4'}`}>
        <div>
          <div className="text-lg font-bold">{osmStatus?.peaks?.toLocaleString('de') ?? '–'}</div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">Gipfel</div>
        </div>
        <div>
          <div className="text-lg font-bold">{osmStatus?.passes?.toLocaleString('de') ?? '–'}</div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">Pässe</div>
        </div>
        <div>
          <div className="text-lg font-bold">{osmStatus?.lifts?.toLocaleString('de') ?? '–'}</div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">Lifte</div>
        </div>
        <div>
          <div className="text-lg font-bold">{osmStatus?.settlements?.toLocaleString('de') ?? '–'}</div>
          <div className="text-xs text-neutral-500 dark:text-neutral-400">Ortschaften</div>
        </div>
        {osmStatus?.amenities !== undefined && (
          <div>
            <div className="text-lg font-bold">{osmStatus.amenities.toLocaleString('de')}</div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400">Restaurants/Hütten</div>
          </div>
        )}
      </div>

      {/* Buttons */}
      <div className="flex flex-col gap-2">
        <button
          onClick={() => setShowImportModal(true)}
          className="w-full py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 transition-colors"
        >
          Import-Assistent
        </button>
        <button
          onClick={() => importLifts.mutate()}
          disabled={importLifts.isPending || !hasData}
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

      {showImportModal && <OSMImportModal onClose={() => setShowImportModal(false)} />}
    </div>
  )
}

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

      {/* Lokale Karte (Martin Tile Server) */}
      <LocalMapSection />

      {/* OSM-Geodaten */}
      <section>
        <h2 className="text-base font-semibold mb-3">OSM-Geodaten</h2>
        <div className="rounded-xl border border-border p-4">
          <OSMStatusSection />
        </div>
      </section>

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
              className="mt-0.5 h-4 w-4 rounded border-neutral-300 dark:border-neutral-600 accent-orange-500 cursor-pointer flex-shrink-0"
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

type LocalMapStatus = 'loading' | 'ok' | 'unavailable'

function LocalMapSection() {
  const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? '/tiles'
  const styleUrl = `${martinBase}/style.json`

  const [status, setStatus] = useState<LocalMapStatus>('loading')
  const [sources, setSources] = useState<string[]>([])
  const [checkKey, setCheckKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    setStatus('loading')
    setSources([])
    fetch(styleUrl, { signal: AbortSignal.timeout(5000) })
      .then(r => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((json: { sources?: Record<string, unknown> }) => {
        if (cancelled) return
        setStatus('ok')
        if (json.sources && typeof json.sources === 'object') {
          setSources(Object.keys(json.sources))
        }
      })
      .catch(() => {
        if (!cancelled) setStatus('unavailable')
      })
    return () => { cancelled = true }
  }, [styleUrl, checkKey])

  const statusBadge =
    status === 'loading' ? (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
        <span className="h-2 w-2 rounded-full bg-neutral-400 animate-pulse" />
        Prüfe…
      </span>
    ) : status === 'ok' ? (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
        <span className="h-2 w-2 rounded-full bg-emerald-500" />
        Verbunden
      </span>
    ) : (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
        <span className="h-2 w-2 rounded-full bg-amber-500" />
        Nicht verfügbar
      </span>
    )

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Lokale Karte</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Optionaler Martin Tile Server für Offline-Betrieb und lokale Vektorkacheln.
        </p>
      </div>

      {/* Status-Zeile */}
      <div className="flex items-center gap-3 flex-wrap">
        {statusBadge}
        <span className="text-xs text-neutral-400 dark:text-neutral-500 font-mono break-all">{styleUrl}</span>
        {status !== 'loading' && (
          <button
            onClick={() => setCheckKey(k => k + 1)}
            className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 underline underline-offset-2"
          >
            Erneut prüfen
          </button>
        )}
      </div>

      {/* Info-Box */}
      <div className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200 dark:border-neutral-700 p-4 space-y-2">
        <p className="text-sm text-neutral-700 dark:text-neutral-300 font-medium">Wie funktioniert die lokale Karte?</p>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 leading-relaxed">
          Der Lokal-Stil wird von einem <strong>Martin Tile Server</strong> bereitgestellt, der als eigener
          Dienst in der Docker-Infrastruktur läuft. Er serviert Vektorkacheln aus PostGIS direkt
          und ermöglicht so vollständigen Offline-Betrieb ohne externe Tile-CDNs.
        </p>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 leading-relaxed">
          Die Server-Adresse wird über die Umgebungsvariable <code className="px-1 py-0.5 rounded bg-neutral-200 dark:bg-neutral-700 font-mono text-xs">VITE_MARTIN_URL</code> konfiguriert
          (Standard: <code className="px-1 py-0.5 rounded bg-neutral-200 dark:bg-neutral-700 font-mono text-xs">/tiles</code>).
          Lokale Karten sind in Phase 3 des Entwicklungsplans vorgesehen.
        </p>
      </div>

      {/* Verfügbare Tile-Sets (nur wenn verbunden) */}
      {status === 'ok' && (
        <div>
          <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-2">
            Verfügbare Tile-Quellen ({sources.length})
          </p>
          {sources.length > 0 ? (
            <ul className="space-y-1">
              {sources.map(src => (
                <li key={src} className="flex items-center gap-2 text-xs text-neutral-600 dark:text-neutral-400">
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 flex-shrink-0" />
                  <code className="font-mono">{src}</code>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-xs text-neutral-400">Keine Sources in style.json gefunden.</p>
          )}
        </div>
      )}

      {/* Hinweis auf Standard-Kartenstil */}
      <div className="flex items-start gap-2 rounded-lg bg-accent-100/40 dark:bg-accent-900/10 border border-accent-200/60 dark:border-accent-800/30 px-3 py-2.5">
        <svg className="h-4 w-4 text-accent-DEFAULT mt-0.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M13 16h-1v-4h-1m1-4h.01M12 2a10 10 0 100 20A10 10 0 0012 2z" />
        </svg>
        <p className="text-xs text-neutral-600 dark:text-neutral-400 leading-relaxed">
          Den Lokal-Stil als Standardkarte einrichten: Wähle{' '}
          <strong className="text-neutral-800 dark:text-neutral-200">Lokal (Offline)</strong>{' '}
          im Dropdown <em>Standard-Kartenstil</em> im Abschnitt{' '}
          <strong className="text-neutral-800 dark:text-neutral-200">Karte</strong> oben.{' '}
          {status !== 'ok' && (
            <span className="text-amber-600 dark:text-amber-400">
              Der Martin Server muss erreichbar sein, damit der Stil geladen werden kann.
            </span>
          )}
        </p>
      </div>
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
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Karte</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Einstellungen für die Heatmap und Kartenansicht.
        </p>
      </div>

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

      <div className="border-t border-neutral-100 dark:border-neutral-800/60 pt-4">
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
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-orange-500 cursor-pointer"
        />
        <div className="flex justify-between text-xs text-neutral-400 mt-1">
          <span>10 m (detailliert)</span>
          <span>100 m (schnell)</span>
        </div>
        <p className="text-xs text-neutral-400 mt-2">
          Bestimmt den Abstand zwischen den Sampling-Punkten für die Heatmap. Kleinere Werte erzeugen mehr Punkte und eine detailliertere Heatmap, erhöhen aber die Datenmenge.
        </p>
      </div>

      <div className="border-t border-neutral-100 dark:border-neutral-800/60 pt-4">
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
              className="h-4 w-4 rounded accent-orange-500 cursor-pointer"
            />
            <span className="text-sm text-neutral-700 dark:text-neutral-300">Karte (Heatmap &amp; Tracks)</span>
          </label>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={currentFilterActivity}
              onChange={(e) => setLocalFilterActivity(e.target.checked)}
              className="h-4 w-4 rounded accent-orange-500 cursor-pointer"
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
            className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-orange-500 cursor-pointer"
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
  const { data: settings } = useQuery({
    queryKey: ['import-settings'],
    queryFn: getImportSettings,
  })
  const mut = useMutation({
    mutationFn: updateImportSettings,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['import-settings'] }),
  })

  const s: ImportSettings = settings ?? { conflict_strategy: 'enrich_missing', gps_duplicate_threshold: 75, fusion_time_window_min: 30 }

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
          <span className="tabular-nums font-mono">{s.gps_duplicate_threshold} %</span>
        </div>
        <input
          type="range"
          min={60}
          max={95}
          step={5}
          value={s.gps_duplicate_threshold}
          onChange={e => mut.mutate({ ...s, gps_duplicate_threshold: Number(e.target.value) })}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-orange-500"
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
          <span className="tabular-nums font-mono">{s.fusion_time_window_min} min</span>
        </div>
        <input
          type="range"
          min={5}
          max={120}
          step={5}
          value={s.fusion_time_window_min}
          onChange={e => mut.mutate({ ...s, fusion_time_window_min: Number(e.target.value) })}
          className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-orange-500"
        />
        <div className="flex justify-between text-xs text-neutral-400">
          <span>5 min (strenger)</span>
          <span>120 min (großzügiger)</span>
        </div>
        <p className="text-xs text-neutral-400 mt-1">
          Maximaler Zeitabstand zwischen zwei Aktivitäten damit sie als Fusionskandidaten erkannt werden. 30 Minuten empfohlen — z.B. für eine Wanderung mit Radcomputer + GPS-Gerät gestartet mit kurzem Versatz.
        </p>
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
