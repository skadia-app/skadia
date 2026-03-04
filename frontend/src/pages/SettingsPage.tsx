import { useState, useMemo } from 'react'
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
import SchemaEditor from '@/components/SchemaEditor'
import RenamePreview from '@/components/RenamePreview'
import LLMSettingsPage from './LLMSettingsPage'

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
          { id: 'ai' as const, label: 'KI & Modelle' },
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

      {activeTab === 'ai' && <LLMSettingsPage />}

      {activeTab === 'general' && <>
      <div className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200 dark:border-neutral-700 px-4 py-3 text-sm text-neutral-600 dark:text-neutral-400">
        Einstellung nicht gefunden? Einstellungen zur Medienverwaltung unter{' '}
        <Link to="/workshop" className="text-orange-500 hover:text-orange-600 font-medium">Werkstatt</Link>.
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

      {/* Datenimport */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Datenimport</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Importiere Aktivitäten, Ausrüstung und Gesundheitsdaten aus einem Garmin-Export.
          </p>
        </div>
        <Link
          to="/settings/import"
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors"
        >
          Garmin-Import
        </Link>
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
          className="inline-block px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 transition-colors"
        >
          Privacy-Zonen verwalten
        </Link>
      </section>

      {/* Import-Einstellungen */}
      <ImportSettingsSection />

      {/* Karte */}
      <HeatmapSettingsSection />

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

  const s: ImportSettings = settings ?? { conflict_strategy: 'enrich_missing', gps_duplicate_threshold: 75 }

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
          Aktivitäten werden als Fusion-Kandidaten vorgeschlagen wenn sie maximal 30 Minuten auseinanderliegen und ihre GPS-Spur mindestens so ähnlich ist. Gleiche Strecken an anderen Tagen werden nicht erkannt.
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
