import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { format } from 'date-fns'
import { de } from 'date-fns/locale'
import { Link } from 'react-router-dom'
import { getShadowFSSettings, updateShadowFSSettings, getHotfolderImports, getHeatmapSettings, updateHeatmapSettings } from '@/api/settings'
import SchemaEditor from '@/components/SchemaEditor'
import RenamePreview from '@/components/RenamePreview'

export default function SettingsPage() {
  const queryClient = useQueryClient()
  const [renameOpen, setRenameOpen] = useState(false)
  const [schemaValue, setSchemaValue] = useState<string | null>(null)

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
                       bg-orange-500 hover:bg-orange-600 transition-colors"
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
            style={{ backgroundColor: settings?.hotfolder_enabled ? '#f97316' : '#d4d4d4' }}
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

      {/* Enrichment */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Enrichment</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Automatische Anreicherung von Aktivitäten mit Wetter, Standort und Tags.
          </p>
        </div>
        <Link
          to="/settings/enrichment"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors"
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
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors"
        >
          Automatisierungsregeln
        </Link>
      </section>

      {/* Karte */}
      <HeatmapSettingsSection />

      <RenamePreview open={renameOpen} onClose={() => setRenameOpen(false)} schema={currentSchema} />
    </div>
  )
}

function HeatmapSettingsSection() {
  const queryClient = useQueryClient()
  const [localInterval, setLocalInterval] = useState<number | null>(null)
  const [localThreshold, setLocalThreshold] = useState<number | null>(null)
  const [localFilterMap, setLocalFilterMap] = useState<boolean | null>(null)
  const [localFilterActivity, setLocalFilterActivity] = useState<boolean | null>(null)

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
            style={{ backgroundColor: (currentFilterMap || currentFilterActivity) ? '#f97316' : '#d4d4d4' }}
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
          className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 transition-colors cursor-pointer"
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
