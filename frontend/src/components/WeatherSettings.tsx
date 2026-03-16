import { useState, useEffect, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/lib/toast'
import { getWeatherSettings, updateWeatherSettings } from '@/api/settings'
import { apiClient } from '@/api/client'
import type { WeatherPrefs } from '@/api/types'
import { WEATHER_DISPLAY_FIELD_OPTIONS } from '@/api/types'

export function WeatherSettings() {
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const { data: prefs, isLoading } = useQuery({
    queryKey: ['weatherSettings'],
    queryFn: getWeatherSettings,
  })

  const [localPrefs, setLocalPrefs] = useState<WeatherPrefs | null>(null)

  useEffect(() => {
    if (prefs && !localPrefs) {
      setLocalPrefs(prefs)
    }
  }, [prefs, localPrefs])

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const mutation = useMutation({
    mutationFn: updateWeatherSettings,
    onSuccess: (data) => {
      queryClient.setQueryData(['weatherSettings'], data)
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler', message: 'Einstellungen konnten nicht gespeichert werden.', duration: 0 })
    },
  })

  const backfillMutation = useMutation({
    mutationFn: () => apiClient<void>('/admin/backfill-weather', { method: 'POST' }),
    onSuccess: () => {
      addToast({ type: 'success', title: 'Wetter-Backfill gestartet', message: 'Läuft im Hintergrund.' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler', message: 'Backfill konnte nicht gestartet werden.', duration: 0 })
    },
  })

  const update = (patch: Partial<WeatherPrefs>) => {
    if (!localPrefs) return
    const next = { ...localPrefs, ...patch }
    setLocalPrefs(next)
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => mutation.mutate(next), 800)
  }

  if (isLoading || !localPrefs) {
    return (
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <div className="text-sm text-neutral-500 dark:text-neutral-400">Lade Einstellungen…</div>
      </section>
    )
  }

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-6">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Wetterdaten</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Konfiguriere wie Wetterdaten für Aktivitäten abgerufen und angezeigt werden.
        </p>
      </div>

      {/* Section 1: Anzahl Messwerte */}
      <div className="space-y-2">
        <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">Messwerte pro Aktivität</div>
        <div className="space-y-2">
          {[
            {
              value: 'auto' as const,
              label: 'Automatisch',
              hint: 'Basierend auf Höhendifferenz und Dauer',
            },
            {
              value: 'always_one' as const,
              label: 'Immer ein Wert',
              hint: null,
            },
            {
              value: 'always_two' as const,
              label: 'Immer zwei Werte',
              hint: 'Start + Gipfel/Durchschnitt',
            },
          ].map(({ value, label, hint }) => (
            <label key={value} className="flex items-start gap-2.5 cursor-pointer">
              <input
                type="radio"
                name="reading_mode"
                value={value}
                checked={localPrefs.reading_mode === value}
                onChange={() => update({ reading_mode: value })}
                className="mt-0.5 accent-[var(--color-accent-DEFAULT,#f97316)]"
              />
              <span className="text-sm text-neutral-800 dark:text-neutral-200">
                {label}
                {hint && (
                  <span className="text-neutral-500 dark:text-neutral-400 ml-1 text-xs">({hint})</span>
                )}
              </span>
            </label>
          ))}
        </div>
      </div>

      {/* Section 2: Schwellenwerte (nur bei auto oder always_two) */}
      {localPrefs.reading_mode !== 'always_one' && (
        <div className="space-y-4">
          <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">Schwellenwerte</div>

          <div className="space-y-1">
            <label htmlFor="mountain-threshold" className="text-xs text-neutral-500 dark:text-neutral-400">
              Berg/Tal-Wetter ab Höhendifferenz
            </label>
            <div className="flex items-center gap-2">
              <input
                id="mountain-threshold"
                type="number"
                min={50}
                max={3000}
                step={50}
                value={localPrefs.mountain_threshold_m}
                onChange={(e) => update({ mountain_threshold_m: Number(e.target.value) })}
                className="w-24 rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-500 dark:text-neutral-400">m Höhendifferenz</span>
            </div>
            <p className="text-xs text-neutral-400 dark:text-neutral-500">
              Zwei Messwerte wenn Höhendifferenz größer als dieser Wert
            </p>
          </div>

          <div className="space-y-1">
            <label htmlFor="long-duration" className="text-xs text-neutral-500 dark:text-neutral-400">
              Zwei Werte ab Tourdauer
            </label>
            <div className="flex items-center gap-2">
              <input
                id="long-duration"
                type="number"
                min={30}
                max={480}
                step={30}
                value={Math.round(localPrefs.long_duration_s / 60)}
                onChange={(e) => update({ long_duration_s: Number(e.target.value) * 60 })}
                className="w-24 rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 px-2 py-1 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT"
              />
              <span className="text-sm text-neutral-500 dark:text-neutral-400">Minuten Tourdauer</span>
            </div>
            <p className="text-xs text-neutral-400 dark:text-neutral-500">
              Zwei Messwerte bei langen Touren ohne starke Höhenänderung
            </p>
          </div>
        </div>
      )}

      {/* Section 3: Angezeigte Felder */}
      <div className="space-y-2">
        <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">In Aktivitäten angezeigte Werte</div>
        <div className="grid grid-cols-2 gap-2">
          {WEATHER_DISPLAY_FIELD_OPTIONS.map((opt) => {
            const checked = localPrefs.display_fields.includes(opt.key)
            const isLast = localPrefs.display_fields.length === 1 && checked
            return (
              <label key={opt.key} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={isLast}
                  onChange={(e) => {
                    const next = e.target.checked
                      ? [...localPrefs.display_fields, opt.key]
                      : localPrefs.display_fields.filter((f) => f !== opt.key)
                    if (next.length > 0) update({ display_fields: next })
                  }}
                  className="accent-[var(--color-accent-DEFAULT,#f97316)] disabled:opacity-40"
                />
                <span className="text-sm text-neutral-800 dark:text-neutral-200">{opt.label}</span>
              </label>
            )
          })}
        </div>
      </div>

      {/* Backfill-Button */}
      <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800/60">
        <p className="text-xs text-neutral-400 dark:text-neutral-500 mb-3">
          Fehlende Wetterdaten für bereits importierte Aktivitäten nachträglich abrufen.
        </p>
        <button
          onClick={() => backfillMutation.mutate()}
          disabled={backfillMutation.isPending}
          className="inline-flex items-center gap-2 rounded-lg border border-neutral-300 dark:border-neutral-600 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {backfillMutation.isPending ? 'Wird gestartet…' : 'Wetter-Daten nachfüllen'}
        </button>
      </div>
    </section>
  )
}
