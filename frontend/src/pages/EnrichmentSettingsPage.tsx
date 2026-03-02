import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchEnrichmentSettings, updateEnrichmentSettings, backfillWeather, backfillEnrichment } from '@/api/enrichment'
import type { EnrichmentSettings } from '@/api/types'

export default function EnrichmentSettingsPage() {
  const queryClient = useQueryClient()

  const { data: settings, isLoading } = useQuery({
    queryKey: ['enrichment-settings'],
    queryFn: fetchEnrichmentSettings,
  })

  const updateMut = useMutation({
    mutationFn: updateEnrichmentSettings,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['enrichment-settings'] })
    },
  })

  const weatherMut = useMutation({ mutationFn: backfillWeather })
  const enrichMut = useMutation({ mutationFn: backfillEnrichment })

  function toggle(key: keyof EnrichmentSettings) {
    if (!settings) return
    updateMut.mutate({ [key]: !settings[key] })
  }

  function updateNumber(key: keyof EnrichmentSettings, value: string) {
    const num = parseFloat(value)
    if (!isNaN(num)) {
      updateMut.mutate({ [key]: num })
    }
  }

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade Einstellungen...</div>
  }

  if (!settings) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Einstellungen nicht verfügbar</div>
  }

  const toggles: { key: keyof EnrichmentSettings; label: string; description: string }[] = [
    { key: 'enable_summit_detection', label: 'Gipfelerkennung', description: 'Erkennt überquerte Gipfel und Pässe anhand von OSM-Daten.' },
    { key: 'enable_weather_enrichment', label: 'Wetterdaten', description: 'Lädt historische Wetterdaten für den Startzeitpunkt.' },
    { key: 'enable_stop_detection', label: 'Stopperkennung', description: 'Erkennt längere Pausen und Einkehren.' },
    { key: 'enable_climb_detection', label: 'Anstiege erkennen', description: 'Erkennt und kategorisiert Anstiege nach Gradient und Höhenmeter.' },
    { key: 'enable_commute_detection', label: 'Pendelerkennung', description: 'Erkennt wiederkehrende Pendelstrecken automatisch.' },
    { key: 'enable_auto_naming', label: 'Automatische Benennung', description: 'Generiert automatisch einen Titel basierend auf Route und Aktivitätstyp.' },
  ]

  const thresholds: { key: keyof EnrichmentSettings; label: string; unit: string; step: string }[] = [
    { key: 'climb_min_gradient_pct', label: 'Mindest-Gradient Anstieg', unit: '%', step: '0.5' },
    { key: 'climb_min_elevation_m', label: 'Mindest-Höhenmeter Anstieg', unit: 'm', step: '10' },
    { key: 'round_trip_threshold_m', label: 'Rundtour-Schwellenwert', unit: 'm', step: '50' },
  ]

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-3">
        <Link
          to="/settings"
          className="inline-flex items-center gap-1 text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 transition-colors"
        >
          <span>&#8592;</span> Einstellungen
        </Link>
      </div>

      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Enrichment-Einstellungen</h1>

      {/* Toggles */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Funktionen</h2>
        <div className="space-y-4">
          {toggles.map(({ key, label, description }) => (
            <div key={key} className="flex items-start justify-between gap-4">
              <div>
                <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{label}</div>
                <div className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">{description}</div>
              </div>
              <button
                onClick={() => toggle(key)}
                disabled={updateMut.isPending}
                className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0"
                style={{ backgroundColor: settings[key] ? '#f97316' : '#d4d4d4' }}
                role="switch"
                aria-checked={!!settings[key]}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    settings[key] ? 'translate-x-6' : 'translate-x-1'
                  }`}
                />
              </button>
            </div>
          ))}
        </div>
      </section>

      {/* Thresholds */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Schwellenwerte</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {thresholds.map(({ key, label, unit, step }) => (
            <div key={key}>
              <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
                {label}
              </label>
              <div className="flex items-center gap-2">
                <input
                  type="number"
                  step={step}
                  value={settings[key] as number}
                  onChange={(e) => updateNumber(key, e.target.value)}
                  className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-orange-500 focus:border-transparent"
                />
                <span className="text-sm text-neutral-500 dark:text-neutral-400 flex-shrink-0">{unit}</span>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Actions */}
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Aktionen</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Nachträgliche Verarbeitung aller bestehenden Aktivitäten.
        </p>
        <div className="flex flex-wrap gap-3">
          <button
            onClick={() => weatherMut.mutate()}
            disabled={weatherMut.isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {weatherMut.isPending ? 'Wird geladen...' : 'Wetter nachträglich laden'}
          </button>
          <button
            onClick={() => enrichMut.mutate()}
            disabled={enrichMut.isPending}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-orange-500 hover:bg-orange-600 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {enrichMut.isPending ? 'Wird berechnet...' : 'Enrichment neu berechnen'}
          </button>
        </div>

        {weatherMut.isSuccess && (
          <p className="text-sm text-emerald-600 dark:text-emerald-400">
            Wetter-Backfill abgeschlossen: {weatherMut.data.processed} verarbeitet, {weatherMut.data.errors} Fehler.
          </p>
        )}
        {weatherMut.isError && (
          <p className="text-sm text-rose-500">Fehler beim Wetter-Backfill.</p>
        )}

        {enrichMut.isSuccess && (
          <p className="text-sm text-emerald-600 dark:text-emerald-400">
            Enrichment-Backfill abgeschlossen: {enrichMut.data.processed} verarbeitet, {enrichMut.data.errors} Fehler.
          </p>
        )}
        {enrichMut.isError && (
          <p className="text-sm text-rose-500">Fehler beim Enrichment-Backfill.</p>
        )}
      </section>
    </div>
  )
}
